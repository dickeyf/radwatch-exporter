# 05 — Helm chart

## Layout

```
deploy/helm/radwatch-exporter/
├── Chart.yaml
├── values.yaml
├── values.schema.json        # types every value; annotation/label maps are string-to-string
├── templates/
│   ├── _helpers.tpl
│   ├── deployment.yaml
│   ├── service.yaml
│   ├── service-ingest.yaml   # optional, ingestService.enabled (IMPL-38)
│   ├── ingress.yaml
│   ├── configmap.yaml
│   ├── servicemonitor.yaml   # optional, serviceMonitor.enabled (AD-07)
│   ├── dashboard-configmap.yaml # optional, grafanaDashboard.enabled (IMPL-31)
│   ├── prometheusrule.yaml   # optional, prometheusRule.enabled (IMPL-32)
│   ├── networkpolicy.yaml    # optional, networkPolicy.enabled (IMPL-33)
│   └── NOTES.txt             # prints the Website / URL to enter on the counter
├── dashboards/radwatch.json  # the Grafana dashboard
├── ci/full-values.yaml       # every optional value turned on, for CI (placeholders only)
└── .kube-linter.yaml         # policy for the rendered manifests
```

Implemented in IMPL-21–25, 27, 30–33 and 38; `values.yaml` documents every value. The chart's `appVersion` is the image tag used when `image.tag` is empty, so a `v<appVersion>` release must exist on GHCR (or set `image.tag` to an `edge`/`sha-` tag).

Values map to the config with `config.*` (e.g. `config.gmcmap.allowedGIDs`), rendered as `RADWATCH_*` env vars in the ConfigMap. Ports are fixed inside the container (8080, 9090); `service.port` and `service.metricsPort` set the Service side. Validation: `helm lint` plus the schema; a missing `ingress.host` (or `tls.secretName` with TLS on) fails the render.

## Resources

### Deployment

- 1 replica, `strategy: Recreate` (ADR-003, AD-06). Not configurable.
- Container ports `http` (8080) and `metrics` (9090).
- Config: `envFrom` the ConfigMap; secret values via `env.valueFrom.secretKeyRef` only if the Secret is enabled.
- Probes: `livenessProbe` and `readinessProbe` on `http:/healthz` and `http:/readyz`.
- `podAnnotations` and `podLabels`: free-form maps passed through to the pod template.
- Resources: small requests/limits (e.g. 10m CPU / 16Mi request, 64Mi limit) — tune after the first deploy (IMPL-29).
- Pod and container security context as in [04-container.md](04-container.md).
- `automountServiceAccountToken: false`.
- Annotation `checksum/config` from the ConfigMap so config changes roll the pod.

### Service (ClusterIP)

Exposes **both** ports:

```yaml
spec:
  type: ClusterIP
  ports:
    - name: http
      port: 80
      targetPort: http
    - name: metrics
      port: 9090
      targetPort: metrics
```

The Ingress uses `http`; the in-cluster scraper uses `metrics`.

`service.annotations` and `service.labels` are free-form maps passed through as-is (e.g. Cilium LB IP pool or BGP announcement labels, or external-dns when the Service is exposed directly).

### Ingest Service (optional, IMPL-38)

`ingestService.enabled: true` adds a second Service, `<fullname>-ingest`, exposing **only** the ingest port, by default as a LoadBalancer with `externalTrafficPolicy: Local`. It exists because the GMC-500+ cannot resolve hostnames (Q-08): it must be given an IP, and an Ingress cannot route by IP. Values: `type`, `port` (80), `externalTrafficPolicy`, `loadBalancerIP`, `loadBalancerClass`, `loadBalancerSourceRanges`, free-form `annotations` (e.g. `lbipam.cilium.io/ips` to pin the address with Cilium) and `labels` (e.g. a Cilium IP-pool selector). `Local` keeps the client's address, so `config.allowedCIDRs` works without trusted proxies. The metrics port is never part of it.

### Ingress

- `ingressClassName` configurable (no default assumed).
- Host configurable (e.g. `radwatch.example.com`). Optional since the counter cannot use it (it needs an IP, Q-08): use `ingestService` for the counter and the Ingress for other clients.
- Routes **only** the ingest path (configurable, default `/log2.asp`) to Service port `http`. `/metrics` is never routed.
- **TLS from an existing Secret** (PD-10: cert-manager creates it). The chart does not create or manage certificates. It takes the name of a `kubernetes.io/tls` Secret that already exists in the same namespace (e.g. created by External Secrets Operator or cert-manager):

  ```yaml
  ingress:
    enabled: true
    className: ""
    host: radwatch.example.com
    path: /log2.asp
    pathType: Exact
    annotations: {}           # free-form, passed through as-is
    tls:
      enabled: false
      secretName: ""          # existing TLS secret in this namespace
  ```

- `ingress.annotations` is a free-form map passed through unchanged, for external-dns and controller-specific settings. Example for a private hostname published by external-dns to Cloudflare:

  ```yaml
  ingress:
    annotations:
      external-dns.alpha.kubernetes.io/cloudflare-proxied: "false"   # DNS-only: the IP is private
  ```

- **Traefik (AD-09).** Route the host on the plain-HTTP entrypoint, and on the TLS one too if TLS is on, e.g. `traefik.ingress.kubernetes.io/router.entrypoints: web,websecure`. If Traefik redirects `web` to `websecure` globally (an entrypoint-level redirect), the device breaks: exclude this host, or serve it from an entrypoint without the redirect. Client IP: set `externalTrafficPolicy: Local` on Traefik's LoadBalancer Service and put Traefik's pod CIDR in `config.trustedProxies`.
- **Both HTTP and HTTPS must work.** The counter very likely only speaks plain HTTP (open question Q-01). If TLS is enabled, the chart must **not** force an HTTP→HTTPS redirect on the ingest path, or the device will break. Controller-specific annotation to disable redirects is supplied through `ingress.annotations` (AD-09).
- Source-IP restriction can be done at the Ingress layer via controller annotations (e.g. an allowlist of the counter's LAN IP) — see AD-04 and [06-security-open-questions.md](06-security-open-questions.md).

### ConfigMap

Holds non-secret configuration, consumed as env vars:

| Key | Example | Purpose |
|-----|---------|---------|
| `RADWATCH_LISTEN_ADDR` | `:8080` | Ingest listener |
| `RADWATCH_METRICS_ADDR` | `:9090` | Metrics listener |
| `RADWATCH_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `RADWATCH_STALE_AFTER` | `5m` | Drop value gauges of a silent device (ADR-008) |
| `RADWATCH_DEBUG_REQUESTS` | `false` | Log every raw ingest request with headers (IMPL-15); for device debugging only |
| `RADWATCH_ALLOWED_CIDRS` | `192.168.1.50/32` | Source-IP allowlist (see trusted-proxy note below) |
| `RADWATCH_TRUSTED_PROXIES` | `10.0.0.0/8` | CIDRs whose `X-Forwarded-For` is trusted |
| `RADWATCH_DEVICE_NAMES` | `0034021=basement` | Optional friendly-name mapping (AD-05) |
| `RADWATCH_GMCMAP_PATH` | `/log2.asp` | Path the device calls |
| `RADWATCH_GMCMAP_ALLOWED_GIDS` | `0034021` | Comma-separated allowlist of counter IDs (empty = allow all) |
| `RADWATCH_GMCMAP_ALLOWED_AIDS` | `0230111` | Comma-separated allowlist of user IDs |
| `RADWATCH_GMCMAP_RESPONSE_STATUS` | `200` | HTTP status replied to the device (Q-05) |
| `RADWATCH_GMCMAP_RESPONSE_BODY` | `OK.ERR0` | Body replied to the device (Q-05) |
| `RADWATCH_GMCMAP_FORWARD_URL` | `http://www.gmcmap.com/log2.asp` | Optional forwarding to a GMCmap server (PD-05, IMPL-34); empty = off |
| `RADWATCH_RADMON_FORWARD_URL` | `https://radmon.org/radmon.php` | Optional forwarding to radmon.org (PD-05, IMPL-34); empty = off |
| `RADWATCH_RADMON_USER` | `station` | radmon.org user |
| `RADWATCH_RADMON_DEVICE_ID` | `0034021` | Only this device is forwarded to radmon; empty = all |

Keys are final (IMPL-04, ADR-006). The chart may also mount a YAML file and set `RADWATCH_CONFIG_FILE`; env vars override it.

### Secret (optional)

The chart never creates a Secret (IMPL-26): secrets do not belong in values. The one secret value, the radmon.org password, comes from an existing Secret (`config.radmon.passwordSecret.name` / `key`, e.g. from External Secrets or sealed-secrets) as `RADWATCH_RADMON_PASSWORD` via `secretKeyRef`, never through the ConfigMap. Setting `config.radmon.forwardURL` without it fails the render.

- The GMC-500+ is not known to support credentials (Q-02). AID/GID are sent in clear text and are not secrets.
- A path-as-secret token (Q-04) would use the same pattern if ever needed.

### ServiceMonitor (optional)

Rendered only if `serviceMonitor.enabled: true` (AD-07: the cluster has the Prometheus Operator CRDs; Alloy's `prometheus.operator.servicemonitors` can read it too). Scrapes the `metrics` port at `/metrics`. Values: `interval`, `scrapeTimeout` (empty = scraper default), free-form `labels` (for whatever label your Prometheus or Alloy selects ServiceMonitors by) and `annotations`, `relabelings`, `metricRelabelings`.

### Metadata on every resource

`commonLabels` is added to every rendered resource. Together with `podAnnotations`/`podLabels`, `service.annotations`/`service.labels` and `ingress.annotations`, no metadata change should ever need a chart fork. The schema types all of these as string-to-string maps.

### Alert rules (optional, IMPL-32)

`prometheusRule.enabled: true` renders a `PrometheusRule` (Prometheus Operator CRDs) with free-form `labels` (whatever your Prometheus selects rules by, e.g. `release: kube-prometheus-stack`) and these alerts, thresholds from PD-07:

| Alert | Condition | Severity |
|-------|-----------|----------|
| `RadwatchExporterDown` | scrape target down for 5 min | warning |
| `RadwatchDeviceSilent` | no upload for `deviceSilent.seconds` (600) plus `for` (2m) | warning |
| `RadwatchDoseRateWarning` | dose rate > `doseRate.warning.sievertsPerHour` (1e-6 Sv/h = 1 µSv/h) for 10 min | warning |
| `RadwatchDoseRateCritical` | dose rate > `doseRate.critical.sievertsPerHour` (1e-5 Sv/h = 10 µSv/h) for 5 min | critical |

Every alert also carries `alertLabels` (default `service: radwatch`) for Alertmanager routing. Deliberate tests with a source will fire the dose-rate alerts: silence them in Alertmanager for the duration.

### NetworkPolicy (optional, IMPL-33)

`networkPolicy.enabled: true` renders a NetworkPolicy for the pod (ADR-011), for CNIs that enforce it:

| Value | Default | Effect |
|-------|---------|--------|
| `ingestFromAllowedCIDRs` | `true` | `config.allowedCIDRs` become `ipBlock` peers of the ingest port (bare IPs get /32 or /128) |
| `ingestFrom` | `[]` | Extra peers for the ingest port, e.g. the ingress controller's pods. No CIDRs and no peers: open |
| `metricsFrom` | `[]` | Peers allowed to scrape; empty = any pod in the cluster |
| `egress` | `[]` | Egress rules; empty denies all (the exporter makes no outbound calls) |

Kubelet probes come from the node; Cilium and Calico allow them by default. With an Ingress, add the controller to `ingestFrom`, or its traffic is dropped.

### Grafana dashboard (optional, IMPL-31)

`grafanaDashboard.enabled: true` renders `dashboards/radwatch.json` into a ConfigMap labelled for the Grafana sidecar (`grafanaDashboard.labels`, default `grafana_dashboard: "1"`, kube-prometheus-stack's default). `namespace` puts it elsewhere if the sidecar only watches one namespace; `annotations` can set a folder. The dashboard has a data-source and a device selector; dose-rate panels use log scales and the reference lines from [reference-dose-rates.md](../reference-dose-rates.md).

## Publishing (IMPL-36)

On a `vX.Y.Z` tag, the CI `publish-chart` job runs after the chart checks and the image push. It requires `appVersion` in `Chart.yaml` to equal `X.Y.Z` (so the default image exists), refuses to overwrite a chart `version` that is already published, then pushes the packaged chart to `oci://ghcr.io/dickeyf/charts/radwatch-exporter`. The chart `version` is independent (PD-08): bump it for every published change to the chart.

## Chart quality gates

The CI `chart` job (IMPL-28) runs on every push, for the default values and for `ci/full-values.yaml`:

- Helm 4 (version pinned in the workflow, updated by Renovate). `helm lint --strict`, which also enforces `values.schema.json` and rejects non-standard `Chart.yaml` fields (so the licence is the `artifacthub.io/license` annotation).
- `helm template | kubeconform -strict`, with the datreeio CRDs catalog for the ServiceMonitor and PrometheusRule.
- `kube-linter` with its default checks plus explicit security checks (non-root, read-only rootfs, privilege escalation, probe ports, memory requests...). Exclusions are listed with their reason in `.kube-linter.yaml` (single replica by design, no CPU limit, no ServiceAccount).
