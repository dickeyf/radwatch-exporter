# 05 — Helm chart

## Layout

```
deploy/helm/radwatch-exporter/
├── Chart.yaml
├── values.yaml
├── values.schema.json        # optional but recommended
├── templates/
│   ├── _helpers.tpl
│   ├── deployment.yaml
│   ├── service.yaml
│   ├── ingress.yaml
│   ├── configmap.yaml
│   ├── secret.yaml           # rendered only if enabled
│   ├── servicemonitor.yaml   # optional, see AD-07
│   └── NOTES.txt
└── README.md
```

## Resources

### Deployment

- 1 replica (ADR-003), `strategy` per AD-06.
- Container ports `http` (8080) and `metrics` (9090).
- Config: `envFrom` the ConfigMap; secret values via `env.valueFrom.secretKeyRef` only if the Secret is enabled.
- Probes: `livenessProbe` and `readinessProbe` on `http:/healthz` and `http:/readyz`.
- Resources: small requests/limits (e.g. 10m CPU / 16Mi request, 64Mi limit) — tune after Phase 2.
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

### Ingress

- `ingressClassName` configurable (no default assumed).
- Host configurable (e.g. `gmc.home.lan`) — it must be resolvable by the counter on the LAN (PD-06).
- Routes **only** the ingest path (configurable, default `/log2.asp`) to Service port `http`. `/metrics` is never routed.
- **TLS from an existing Secret.** The chart does not create or manage certificates. It takes the name of a `kubernetes.io/tls` Secret that already exists in the same namespace (e.g. created by External Secrets Operator or cert-manager):

  ```yaml
  ingress:
    enabled: true
    className: ""
    host: gmc.home.lan
    path: /log2.asp
    pathType: Exact
    annotations: {}
    tls:
      enabled: false
      secretName: ""          # existing TLS secret in this namespace
  ```

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
| `RADWATCH_GMCMAP_FORWARD_URL` | `http://www.gmcmap.com/log2.asp` | Optional forwarding (PD-05). Not implemented yet (IMPL-34): validated, then ignored |

Keys are final (IMPL-04, ADR-006). The chart may also mount a YAML file and set `RADWATCH_CONFIG_FILE`; env vars override it.

### Secret (optional)

- Rendered only if `secret.create: true`; or reference an existing one with `secret.existingSecret`.
- The GMC-500+ is not known to support credentials (Q-02). AID/GID are sent in clear text and are not secrets.
- Plausible future uses: a shared token appended to the URL path (e.g. URL = `ingest/<random-token>`) if the device lets you set an arbitrary path (AD-04), or gmcmap.com account IDs for forwarding if you'd rather not keep them in a ConfigMap.

### ServiceMonitor (optional)

Rendered only if `serviceMonitor.enabled: true` and the CRD exists. Points at the `metrics` port. Whether to use this or an Alloy scrape rule is AD-07.

## Chart quality gates

- `helm lint` and `helm template | kubeconform -strict`.
- `values.schema.json` validates types and required fields.
- Rendered manifests pass a policy check (e.g. Kyverno/Polaris CLI) for non-root, read-only rootfs, dropped caps.
- `NOTES.txt` prints the Website/URL values to enter on the counter.
