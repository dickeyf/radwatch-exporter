# radwatch-exporter

Receives readings pushed by radiation counters (first protocol: GQ GMCmap upload, e.g. a GMC-500+) and exposes them as Prometheus metrics.

See [docs/plan](docs/plan/README.md) for the project plan and worklist.

## Run locally

```sh
make build
./bin/radwatch-exporter          # ingest on :8080, metrics on :9090
```

The process logs JSON to stdout. Ctrl-C (or SIGTERM) shuts it down gracefully.

### Smoke test

In another terminal, send a reading the way a GMC counter does, then look at the metrics:

```sh
curl "localhost:8080/log2.asp?AID=1&GID=2&CPM=15&ACPM=13.2&uSV=0.075"
# OK.ERR0
curl -s localhost:9090/metrics | grep '^radwatch_'
# radwatch_cpm{device="2",device_id="2",protocol="gmcmap"} 15
# radwatch_acpm{device="2",device_id="2",protocol="gmcmap"} 13.2
# radwatch_dose_rate_sieverts_per_hour{device="2",device_id="2",protocol="gmcmap"} 7.5e-08
# ...
```

The dose rate is stored in Sv/h (Prometheus base units); show it as µSv/h in Grafana.

### Endpoints

| Port | Path | Purpose |
|------|------|---------|
| 8080 | `GET /log2.asp` (`gmcmap.path`) | GMCmap uploads from the counter |
| 8080 | `GET /healthz` | Liveness: the process is up |
| 8080 | `GET /readyz` | Readiness: both listeners are up and not shutting down |
| 9090 | `GET /metrics` | Prometheus metrics; never route this port through the Ingress |

Rejected uploads get `400`/`414` (bad input) or `403` (allowlists), and are counted in `radwatch_ingest_requests_total{result=...}`.

## Point a GQ counter at it (GMC-500+, GMCmap protocol)

On the counter, under **Menu → Server**:

| Setting | Value |
|---------|-------|
| Website | IP or host name of the machine running the exporter |
| URL | `log2.asp` (or your `gmcmap.path` without the leading `/`) |
| User ID (AID) | anything; becomes `account_id` in the logs |
| Counter ID (GID) | the ID that will appear as `device_id` |

The counter calls port 80, so either run the exporter with `RADWATCH_LISTEN_ADDR=:80` (needs privileges on Linux) or forward port 80 to 8080. Whether the Website field accepts a port is open question Q-03 in [docs/plan/06](docs/plan/06-security-open-questions.md).

To lock it down once it works, set `RADWATCH_ALLOWED_CIDRS` to the counter's IP (give it a DHCP reservation) and `RADWATCH_GMCMAP_ALLOWED_GIDS` to its counter ID. Behind a reverse proxy or Ingress, also set `RADWATCH_TRUSTED_PROXIES` to the proxy's addresses so the real client IP is read from `X-Forwarded-For`.

### Capturing what the device really sends

Set `RADWATCH_DEBUG_REQUESTS=true`. Every request on the ingest port, on any path, is then logged as a `debug request` line with the method, raw request URI, Host, all headers (credentials redacted), source address and reply status. Health probes are skipped. Use it to answer the open questions about device behaviour, then turn it off.

```sh
RADWATCH_DEBUG_REQUESTS=true RADWATCH_LISTEN_ADDR=:80 ./bin/radwatch-exporter
```

## Development

```sh
make build   # bin/radwatch-exporter
make test    # uses -race, which needs cgo (CGO_ENABLED=1 and a C compiler); on Windows without one: go test ./...
make lint    # requires golangci-lint v2
make run
```

Dependencies are kept current by [Renovate](renovate.json), weekly: minor, patch and digest updates (Go modules, GitHub Actions, base images, CI tools) automerge once CI is green; major updates and the Go toolchain (`go.mod` plus the `golang` build image, in one PR) are merged by hand.

## Kubernetes (Helm)

The chart is in [deploy/helm/radwatch-exporter](deploy/helm/radwatch-exporter); every value is documented in its `values.yaml`. For a full walkthrough (Flux, Cilium BGP, pfSense), see the [deploy guide](docs/deploy-guide.md).

**The GMC-500+ cannot resolve hostnames**: its Website field must be an IP. Give the exporter its own LoadBalancer IP with `ingestService` (ingest port only, `externalTrafficPolicy: Local` so the counter's address survives for `allowedCIDRs`):

```yaml
# my-values.yaml
config:
  allowedCIDRs: ["192.168.1.50"]        # the counter (DHCP reservation)
  deviceNames: {"0034021": office}
  gmcmap:
    allowedGIDs: ["0034021"]
ingestService:
  enabled: true
  loadBalancerIP: 10.10.10.20           # the counter's Website setting
```

Optionally, also expose it by name through an Ingress for other clients (curl, a browser), e.g. behind Traefik with external-dns publishing a Cloudflare DNS-only record. Add to the values above:

```yaml
config:
  trustedProxies: ["10.0.0.0/8"]       # Traefik's pod CIDR
ingress:
  enabled: true
  className: traefik
  host: radwatch.example.com           # for other clients; the counter uses the ingest IP
  annotations:
    external-dns.alpha.kubernetes.io/cloudflare-proxied: "false"
    traefik.ingress.kubernetes.io/router.entrypoints: web
```

```sh
# From GHCR (released charts)
helm install radwatch oci://ghcr.io/dickeyf/charts/radwatch-exporter --version <chart version>   -n radwatch --create-namespace -f my-values.yaml
# Or from a checkout
helm install radwatch deploy/helm/radwatch-exporter -n radwatch --create-namespace -f my-values.yaml
```

Counters only speak plain HTTP on port 80: if you route them through an Ingress, serve the path on a plain-HTTP entrypoint without an HTTPS redirect. `/metrics` stays in the cluster on Service port 9090; `ingestService` never exposes it.

### NetworkPolicy

`networkPolicy.enabled: true` limits the ingest port to `config.allowedCIDRs` (plus any `networkPolicy.ingestFrom` peers, such as your ingress controller), the metrics port to `networkPolicy.metricsFrom` (default: any pod in the cluster), and denies egress. Needs a CNI that enforces NetworkPolicy.

### Grafana dashboard

`grafanaDashboard.enabled: true` ships the dashboard as a ConfigMap for the Grafana sidecar (label `grafana_dashboard: "1"` by default). Or import [dashboards/radwatch.json](deploy/helm/radwatch-exporter/dashboards/radwatch.json) by hand. It shows the current dose rate, CPM, 24 h dose and time since the last upload, then log-scale dose-rate and CPM history with reference lines (background, 1 mSv/year, flights, occupational limit, and a perspective panel up to Chernobyl levels, see [reference dose rates](docs/reference-dose-rates.md)).

### Alerts

`prometheusRule.enabled: true` (with `prometheusRule.labels` set to whatever your Prometheus selects rules by) adds alerts for a silent device (no upload for 10 min), a high dose rate (warning above 1 µSv/h for 10 min, critical above 10 µSv/h for 5 min) and a down exporter. Thresholds are values; see the chart's `values.yaml`.

### Forwarding to public maps

Off by default. Set a forward URL to keep a public map fed: each accepted reading is re-sent in the background (10 s timeout, no retries; a failure never affects ingest), counted in `radwatch_forward_requests_total{target,result}`.

- **gmcmap.com:** `config.gmcmap.forwardURL: http://www.gmcmap.com/log2.asp`. The reading goes out with the device's own AID and GID.
- **radmon.org:** `config.radmon.forwardURL: https://radmon.org/radmon.php`, `config.radmon.user`, and the password from an existing Secret (`config.radmon.passwordSecret.name`/`key`), mounted as a file. With several counters, `config.radmon.deviceID` picks the one for that station.

Any compatible server works: the URL is the destination. With `networkPolicy` on, add egress for DNS and the targets (ports 80/443).

## Configuration

Defaults, then an optional YAML file (`RADWATCH_CONFIG_FILE`), then env vars; a set env var always wins. Invalid config stops the process at startup.

| Env var | YAML key | Default |
|---------|----------|---------|
| `RADWATCH_LISTEN_ADDR` | `listen_addr` | `:8080` |
| `RADWATCH_METRICS_ADDR` | `metrics_addr` | `:9090` |
| `RADWATCH_LOG_LEVEL` | `log_level` | `info` |
| `RADWATCH_STALE_AFTER` | `stale_after` | `5m` (`0` = keep values forever) |
| `RADWATCH_DEBUG_REQUESTS` | `debug_requests` | `false` (log every raw ingest request) |
| `RADWATCH_ALLOWED_CIDRS` | `allowed_cidrs` | empty (allow all) |
| `RADWATCH_TRUSTED_PROXIES` | `trusted_proxies` | empty |
| `RADWATCH_DEVICE_NAMES` | `device_names` | empty |
| `RADWATCH_GMCMAP_PATH` | `gmcmap.path` | `/log2.asp` |
| `RADWATCH_GMCMAP_ALLOWED_GIDS` | `gmcmap.allowed_gids` | empty (allow all) |
| `RADWATCH_GMCMAP_ALLOWED_AIDS` | `gmcmap.allowed_aids` | empty (allow all) |
| `RADWATCH_GMCMAP_RESPONSE_STATUS` | `gmcmap.response_status` | `200` |
| `RADWATCH_GMCMAP_RESPONSE_BODY` | `gmcmap.response_body` | `OK.ERR0` |
| `RADWATCH_GMCMAP_FORWARD_URL` | `gmcmap.forward_url` | empty (off). Re-sends each reading there, e.g. `http://www.gmcmap.com/log2.asp` |
| `RADWATCH_RADMON_FORWARD_URL` | `radmon.forward_url` | empty (off). Submits CPM there, e.g. `https://radmon.org/radmon.php` |
| `RADWATCH_RADMON_USER` | `radmon.user` | empty (required with a forward URL) |
| `RADWATCH_RADMON_PASSWORD` | `radmon.password` | empty (required with a forward URL, or use the file below) |
| `RADWATCH_RADMON_PASSWORD_FILE` | `radmon.password_file` | empty. File holding the password, e.g. a mounted Secret; not both |
| `RADWATCH_RADMON_DEVICE_ID` | `radmon.device_id` | empty (forward every device) |

Lists are comma-separated in env vars (`a,b`) and YAML sequences in the file. CIDR lists accept bare IPs. Device names are `id=name,id=name` in env vars and a map in YAML:

```yaml
listen_addr: ":8080"
allowed_cidrs: ["192.168.1.50"]
device_names:
  "0034021": basement
gmcmap:
  allowed_gids: ["0034021"]
```

## Releasing

1. In `deploy/helm/radwatch-exporter/Chart.yaml`, set `appVersion` to the new release (`X.Y.Z`) and bump the chart `version`.
2. Commit, then tag and push: `git tag -a vX.Y.Z -m vX.Y.Z && git push origin vX.Y.Z`.

CI then publishes the image (`X.Y.Z`, `X.Y`, `sha-<short>`) and the chart to GHCR. It fails if `appVersion` doesn't match the tag or the chart version already exists.

## Licence

Apache License 2.0. See [LICENSE](LICENSE).
