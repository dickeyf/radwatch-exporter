# 03 — Architecture

## Component view

```
GMC-500+ (WiFi)
   │  HTTP GET /<ingest-path>?AID=..&GID=..&CPM=..&ACPM=..&uSV=..
   ▼
Ingress controller (LAN IP, :80 / :443)
   │  only the ingest path is routed
   ▼
Service radwatch-exporter (ClusterIP)
   ├─ port "http"    → container :8080  (ingest, /healthz, /readyz)
   └─ port "metrics" → container :9090  (/metrics)
   ▼
Pod: radwatch-exporter (single static Go binary)
   ├─ ingest handler → validate → allowlist check → update gauges
   ├─ (optional) forwarder → gmcmap.com / radmon.org
   └─ Prometheus registry → /metrics
   ▲
Alloy / Prometheus (scrapes "metrics" port in-cluster)
   ▼
Mimir → Grafana
```

## Data model (initial proposal)

| Metric | Type | Labels | Notes |
|--------|------|--------|-------|
| `gmc_cpm` | gauge | `gid` | Last CPM received |
| `gmc_acpm` | gauge | `gid` | Last averaged CPM |
| `gmc_dose_rate_usv_per_hour` | gauge | `gid` | Last µSv/h from the device |
| `gmc_last_report_timestamp_seconds` | gauge | `gid` | Unix time of last accepted reading |
| `gmc_reports_total` | counter | `gid` | Accepted readings |
| `gmc_ingest_requests_total` | counter | `result` (`accepted`, `rejected_allowlist`, `bad_request`) | No `gid` on rejects, to avoid cardinality abuse |
| standard `go_*` / `process_*` | — | — | From client_golang collectors |

Whether `aid` is also a label, and whether any label should be a friendly name instead of the raw ID, is AD-05.

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

**Status:** Proposed (confirm as AD-06).

No persistence; on restart the gauges are empty until the next report (≤ 1 min). One replica: with more, a scrape hits only one pod and values would flip between "stale" and "fresh". Use `strategy: Recreate` or `RollingUpdate` with `maxSurge: 1`; a gap of one reading on deploy is acceptable.

### ADR-004 — Separate ports for ingest and metrics

**Status:** Accepted (AD-03, confirmed 2026-10-09).

Ingest on `:8080`, metrics on `:9090`, both exposed by the ClusterIP Service. Only the ingest port is routed by the Ingress, so `/metrics` is never reachable from outside the cluster even if the Ingress is misconfigured on paths. Alternative: one port, path-based Ingress rules — simpler but easier to expose `/metrics` by accident.

### ADR-005 — Standard library `net/http`, no web framework

**Status:** Accepted (AD-02, confirmed 2026-10-09).

The service has two or three routes per listener (ingest path, `/healthz`, `/readyz`; `/metrics`). Go 1.22+ `http.ServeMux` supports method and path patterns, which covers this. chi/gin/echo would add dependencies without adding anything needed. Consequence: handlers are plain `http.Handler`s, testable with `httptest`; middleware (allowlist, logging) is hand-written wrapping.

## Decisions still open

See the `AD-*` items in [07-worklist.md](07-worklist.md).
