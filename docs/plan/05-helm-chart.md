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
│   ├── ingress.yaml
│   ├── configmap.yaml
│   ├── secret.yaml           # not yet (IMPL-26)
│   ├── servicemonitor.yaml   # optional, serviceMonitor.enabled (AD-07)
│   └── NOTES.txt             # prints the Website / URL to enter on the counter
├── ci/full-values.yaml       # every optional value turned on, for CI (placeholders only)
└── .kube-linter.yaml         # policy for the rendered manifests
```

Implemented in IMPL-21–25 and 27; `values.yaml` documents every value. The chart's `appVersion` is the image tag used when `image.tag` is empty, so a `v<appVersion>` release must exist on GHCR (or set `image.tag` to an `edge`/`sha-` tag).

Values map to the config with `config.*` (e.g. `config.gmcmap.allowedGIDs`), rendered as `RADWATCH_*` env vars in the ConfigMap. Ports are fixed inside the container (8080, 9090); `service.port` and `service.metricsPort` set the Service side. Validation: `helm lint` plus the schema; a missing `ingress.host` (or `tls.secretName` with TLS on) fails the render.

## Resources

### Deployment

- 1 replica, `strategy: Recreate` (ADR-003, AD-06). Not configurable.
- Container ports `http` (8080) and `metrics` (9090).
- Config: `envFrom` the ConfigMap; secret values via `env.valueFrom.secretKeyRef` only if the Secret is enabled.
- Probes: `livenessProbe` and `readinessProbe` on `http:/healthz` and `http:/readyz`.
- `podAnnotations` and `podLabels`: free-form maps passed through to the pod template.
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

`service.annotations` and `service.labels` are free-form maps passed through as-is (e.g. Cilium LB IP pool or BGP announcement labels, or external-dns when the Service is exposed directly).

### Ingress

- `ingressClassName` configurable (no default assumed).
- Host configurable (e.g. `gmc.home.lan`) — it must be resolvable by the counter on the LAN (PD-06).
- Routes **only** the ingest path (configurable, default `/log2.asp`) to Service port `http`. `/metrics` is never routed.
- **TLS from an existing Secret** (PD-10: cert-manager creates it). The chart does not create or manage certificates. It takes the name of a `kubernetes.io/tls` Secret that already exists in the same namespace (e.g. created by External Secrets Operator or cert-manager):

  ```yaml
  ingress:
    enabled: true
    className: ""
    host: gmc.home.lan
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
| `RADWATCH_GMCMAP_FORWARD_URL` | `http://www.gmcmap.com/log2.asp` | Optional forwarding (PD-05). Not implemented yet (IMPL-34): validated, then ignored |

Keys are final (IMPL-04, ADR-006). The chart may also mount a YAML file and set `RADWATCH_CONFIG_FILE`; env vars override it.

### Secret (optional)

- Rendered only if `secret.create: true`; or reference an existing one with `secret.existingSecret`.
- The GMC-500+ is not known to support credentials (Q-02). AID/GID are sent in clear text and are not secrets.
- Plausible future uses: a shared token appended to the URL path (e.g. URL = `ingest/<random-token>`) if the device lets you set an arbitrary path (AD-04), or gmcmap.com account IDs for forwarding if you'd rather not keep them in a ConfigMap.

### ServiceMonitor (optional)

Rendered only if `serviceMonitor.enabled: true` (AD-07: the cluster has the Prometheus Operator CRDs; Alloy's `prometheus.operator.servicemonitors` can read it too). Scrapes the `metrics` port at `/metrics`. Values: `interval`, `scrapeTimeout` (empty = scraper default), free-form `labels` (for whatever label your Prometheus or Alloy selects ServiceMonitors by) and `annotations`, `relabelings`, `metricRelabelings`.

### Metadata on every resource

`commonLabels` is added to every rendered resource. Together with `podAnnotations`/`podLabels`, `service.annotations`/`service.labels` and `ingress.annotations`, no metadata change should ever need a chart fork. The schema types all of these as string-to-string maps.

## Chart quality gates

The CI `chart` job (IMPL-28) runs on every push, for the default values and for `ci/full-values.yaml`:

- `helm lint --strict`, which also enforces `values.schema.json`.
- `helm template | kubeconform -strict`, with the datreeio CRDs catalog for the ServiceMonitor.
- `kube-linter` with all built-in checks (non-root, read-only rootfs, dropped caps, probes, resources...). Exclusions are listed with their reason in `.kube-linter.yaml` (single replica by design, no CPU limit, no ServiceAccount).
