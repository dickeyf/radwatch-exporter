# 03 — Architecture

## Component view

```
GMC-500+ (WiFi, configured with an IP: it cannot resolve names, Q-08)
   │  HTTP GET http://<ingest IP>/<ingest-path>?AID=..&GID=..&CPM=..&ACPM=..&uSV=..
   ▼
Service radwatch-exporter-ingest (LoadBalancer, fixed private IP, :80 only,
   │  externalTrafficPolicy: Local so the counter's address survives; ADR-009, IMPL-38)
   │
   │        other clients (optional) → Ingress controller → only the ingest path
   ▼
Service radwatch-exporter (ClusterIP)
   ├─ port "http"    → container :8080  (ingest, /healthz, /readyz)
   └─ port "metrics" → container :9090  (/metrics, in-cluster only)
   ▼
Pod: radwatch-exporter (single static Go binary)
   ├─ ingest: source-IP check → validate → GID/AID check → update gauges (ADR-009)
   ├─ (optional, not implemented yet: IMPL-34) forwarder → gmcmap.com / radmon.org
   └─ Prometheus registry → /metrics
   ▲
Alloy / Prometheus (scrapes "metrics" port in-cluster)
   ▼
Mimir → Grafana
```

## Data model

Decided in ADR-007 (AD-05). Every device metric carries the same three labels:

- `protocol`: the upload protocol that delivered the reading (`gmcmap` for now).
- `device_id`: the counter's ID in that protocol (GMCmap `GID`).
- `device`: the friendly name from `device_names`, or `device_id` when none is set.

| Metric | Type | Labels | Notes |
|--------|------|--------|-------|
| `radwatch_cpm` | gauge | `protocol`, `device_id`, `device` | Last counts per minute |
| `radwatch_acpm` | gauge | `protocol`, `device_id`, `device` | Last averaged CPM (only if the device sends it) |
| `radwatch_dose_rate_sieverts_per_hour` | gauge | `protocol`, `device_id`, `device` | Last dose rate as computed by the device, converted from µSv/h to Sv/h (only if sent) |
| `radwatch_last_report_timestamp_seconds` | gauge | `protocol`, `device_id`, `device` | Unix time of the last accepted reading |
| `radwatch_reports_total` | counter | `protocol`, `device_id`, `device` | Accepted readings |
| `radwatch_ingest_requests_total` | counter | `protocol`, `result` (`accepted`, `bad_request`, `rejected_allowlist`) | No device labels, so rejected requests can't create series |
| standard `go_*` / `process_*` | — | — | From client_golang collectors |

A device that has not reported for `stale_after` (default 5 min) loses its `cpm`, `acpm` and dose-rate series; its timestamp and `reports_total` stay (ADR-008). A gauge is also dropped when a reading stops including that field.

## Decisions taken

### ADR-001 — Implement the service in Go

**Status:** Accepted (AD-01, confirmed 2026-10-09).

**Context.** The service is an HTTP server that receives one small GET per minute per device and exposes Prometheus metrics. Requirements: small static binary, tiny image, low memory, strong Prometheus support, easy to maintain for a single developer working in GoLand.

**Options considered.**

| Criterion | Go | Rust |
|-----------|----|------|
| HTTP server | `net/http` in the standard library is production-grade; no framework needed | Needs a crate (`axum`/`hyper` + `tokio`); excellent but more moving parts |
| Prometheus client | `prometheus/client_golang` is the reference implementation, maintained by the Prometheus project, used by Prometheus itself and most exporters | `prometheus-client` (official Rust client) and `prometheus` crate are good, but smaller ecosystem and fewer examples |
| Exporter ecosystem | Almost every official and community exporter is Go; patterns (`promhttp`, collectors, `exporter-toolkit` for TLS/basic-auth on `/metrics`) are well-trodden | Fewer exporters to copy patterns from |
| Static binary / image | `CGO_ENABLED=0` gives a static binary; `scratch`/distroless images around 10 MB | Static musl builds work; similar image size; slower and more involved builds |
| Build/iteration speed | Very fast compiles | Slower compiles, steeper learning curve |
| Runtime footprint | ~10–20 MB RSS | Lower (~a few MB) — irrelevant at one request per minute |
| Tooling | GoLand first-class; `go test`, `golangci-lint` | RustRover/rust-analyzer good |
| Safety | Memory-safe (GC) | Memory-safe without GC; stronger compile-time guarantees |

**Decision.** Go. Its standard library covers the HTTP server and the official Prometheus client library is Go. Rust's advantages (lower memory, no GC) don't matter at this load, and its ecosystem for exporters is smaller.

**Consequences.** Use `net/http` + `prometheus/client_golang`; aim for zero or near-zero other dependencies. Go module, Go version pinned in `go.mod` and the Dockerfile.

### ADR-002 — Pull model: expose `/metrics`, don't push

**Status:** Accepted.

The counter pushes to us; we keep the last value in memory and let Prometheus/Alloy scrape it. No remote-write or Pushgateway. Simpler, stateless, idiomatic exporter pattern. Trade-off: a reading between two scrapes can be overwritten if the device ever reports faster than the scrape interval (not the case at 1/min with a 15–30 s scrape).

### ADR-003 — Stateless, single replica

**Status:** Accepted (AD-06, confirmed 2026-10-09).

No persistence; on restart the gauges are empty until the next report (within the device's upload interval, 1–2 min, Q-07). One replica: with more, uploads and scrapes would land on different pods and values would flip between "stale" and "fresh". `strategy: Recreate`, so two pods never receive uploads at once; a gap of one reading on deploy is acceptable. The chart hard-codes both.

### ADR-004 — Separate ports for ingest and metrics

**Status:** Accepted (AD-03, confirmed 2026-10-09).

Ingest on `:8080`, metrics on `:9090`, both exposed by the ClusterIP Service. Only the ingest port is routed by the Ingress or exposed by the ingest LoadBalancer Service (`ingestService`); neither ever carries the metrics port, so `/metrics` is never reachable from outside the cluster even if the Ingress is misconfigured on paths. Alternative: one port, path-based Ingress rules — simpler but easier to expose `/metrics` by accident.

### ADR-005 — Standard library `net/http`, no web framework

**Status:** Accepted (AD-02, confirmed 2026-10-09).

The service has two or three routes per listener (ingest path, `/healthz`, `/readyz`; `/metrics`). Go 1.22+ `http.ServeMux` supports method and path patterns, which covers this. chi/gin/echo would add dependencies without adding anything needed. Consequence: handlers are plain `http.Handler`s, testable with `httptest`; middleware (allowlist, logging) is hand-written wrapping.

### ADR-006 — Configuration from env vars plus an optional YAML file

**Status:** Accepted (AD-12, confirmed 2026-10-09).

Order of precedence: built-in defaults, then the YAML file named by `RADWATCH_CONFIG_FILE` (if set), then `RADWATCH_*` env vars. A variable that is set, even to an empty string, overrides the file. Unknown YAML keys are an error. All problems are reported at once and the process exits non-zero before opening any listener.

Settings for one upload protocol live in their own section (`gmcmap:` in YAML, `RADWATCH_GMCMAP_*` in env), so more protocols can be added later without renaming keys. Env vars keep the simple case simple (a ConfigMap via `envFrom`); the file suits per-device maps and longer lists. Reference: the README's Configuration section and `internal/config`.

### ADR-007 — Metric names and labels

**Status:** Accepted (AD-05, confirmed 2026-10-09).

Prefix `radwatch_`. Device labels are `protocol`, `device_id` and `device` (see Data model). `protocol` keeps IDs from different upload protocols apart. `device` lets dashboards show friendly names without a Grafana lookup; it falls back to the ID so the label is never empty. The GMCmap account ID (`AID`) is not a label: it identifies the gmcmap.com account, not the counter, and adds nothing to the series. Device series can only be created by a reading that passed validation and the allowlists, which bounds cardinality.

Units: Prometheus base units, no SI prefixes. The device reports µSv/h; the exporter converts to Sv/h (`_sieverts_per_hour`, typical values around 1e-7). Hours rather than seconds because dose rates are always quoted per hour. Dashboards choose the display unit (e.g. µSv/h). Accumulated dose is left to queries (e.g. `avg_over_time(...[1d]) * 24` gives Sv per day); an in-exporter `_sieverts_total` counter was considered and not adopted for now.

### ADR-008 — Expire value gauges of silent devices

**Status:** Accepted (AD-11, confirmed 2026-10-09).

When a device has not reported for `stale_after` (`RADWATCH_STALE_AFTER`, default `5m`, `0` disables), the exporter drops its value gauges (`radwatch_cpm`, `radwatch_acpm`, `radwatch_dose_rate_sieverts_per_hour`) at the next scrape. `radwatch_last_report_timestamp_seconds` and `radwatch_reports_total` are kept. Dashboards then show a gap instead of a flat, stale line, and the "device silent" alert stays a simple `time() - radwatch_last_report_timestamp_seconds > 600` (PD-07) with no `absent()`. The next accepted reading brings the gauges back. Alternatives rejected: keeping the last value forever (misleading graphs), dropping every series (alerting needs `absent()` per device).

### ADR-009 — Layered ingest restrictions, honest 4xx on rejection

**Status:** Accepted (AD-04 and AD-13, confirmed 2026-10-09).

In-app checks, in order, each skipped when its list is empty:

1. **Source IP** against `allowed_cidrs` (`internal/ingest.SourceGuard`, shared by all protocols). The client address is the TCP peer, unless the peer is in `trusted_proxies`: then `X-Forwarded-For` is read right to left and the first hop outside `trusted_proxies` is the client. An entry added by the client itself (leftmost) is therefore never trusted. An unparseable hop rejects the request when the allowlist is set.
2. **Input validation** (400 / 414), so junk never reaches the ID checks.
3. **GID** against `gmcmap.allowed_gids`, then **AID** against `gmcmap.allowed_aids` (a missing AID fails a non-empty AID list).

Rejections answer honestly: 403 for allowlists, 400/414 for bad input. They are counted as `rejected_allowlist` or `bad_request` and logged at WARN with `client_ip` and `reason`. The resolved `client_ip` is logged for accepted readings too.

Outside the app, the private LoadBalancer IP of the ingest Service (with the router's firewall rule and optionally `loadBalancerSourceRanges`), or the Ingress for other clients (strict path, LAN-only exposure), and the optional NetworkPolicy (ADR-011) add the outer layers listed in 06. Path-as-secret waits for Q-04. AD-13 is revisited if Phase 2 shows the device misbehaves on non-200 replies (Q-05); the reply would then become configurable.

### ADR-010 — Container base image: distroless static, nonroot

**Status:** Accepted (AD-08, confirmed 2026-10-09).

Final stage `gcr.io/distroless/static-debian12:nonroot`: CA certificates (for future HTTPS forwarding, IMPL-34), a `nonroot` user with UID/GID 65532, tzdata, no shell or package manager. `scratch` would be a few hundred KB smaller but needs CA certs and a passwd entry copied in by hand; Chainguard's static image is equivalent but only `latest` is free to pull by tag. Build stage `golang:<Go version>-bookworm`. Both are pinned by digest and updated by Renovate (PD-09). The build cross-compiles on the build platform, so arm64 images (PD-04) need no QEMU.

### ADR-011 — No authentication on `/metrics`; optional NetworkPolicy

**Status:** Accepted (AD-10, 2026-10-11).

`/metrics` gets no basic auth or TLS. It listens on its own port (ADR-004), which neither the ingest Service nor the Ingress ever carries, so it is only reachable inside the cluster, and it exposes nothing secret (readings, request counts, Go runtime stats). `exporter-toolkit` would add a dependency, a credential to rotate, and matching configuration in every scraper, for little gain.

The chart instead ships an optional NetworkPolicy (`networkPolicy.enabled`, IMPL-33) for clusters whose CNI enforces it:

- **Ingest port:** from `config.allowedCIDRs` as `ipBlock`s (a bare IP becomes /32 or /128), plus extra peers such as the ingress controller (`ingestFrom`). Through the ingest Service with `externalTrafficPolicy: Local` the counter keeps its address, so the same list works at both layers. With neither, the port stays open.
- **Metrics port:** from `metricsFrom` peers (e.g. the `monitoring` namespace); empty means any pod in the cluster, never outside.
- **Egress:** denied unless listed (`egress`). The exporter makes no outbound calls; forwarding (IMPL-34) will need DNS and its target here.

Off by default: a wrong peer silently breaks scraping or ingest, and kubelet probe handling differs by CNI. Revisit if `/metrics` ever has to leave the cluster.

## Decisions still open

See the `AD-*` items in [07-worklist.md](07-worklist.md).
