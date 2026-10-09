# 07 — Worklist

Status: `TODO` / `DOING` / `DONE` / `DROPPED`. Phase refers to [02-roadmap.md](02-roadmap.md).

---

## A. Implementation worklist

| ID | Phase | Item | Depends on | Status |
|----|-------|------|------------|--------|
| IMPL-01 | 0 | Init Go module, repo layout (`cmd/radwatch-exporter`, `internal/ingest`, `internal/metrics`, `internal/config`), `.gitignore`, `.editorconfig` | AD-01, PD-01, PD-02 | DONE (module renamed to `github.com/dickeyf/radwatch-exporter` per PD-01) |
| IMPL-02 | 0 | Makefile/Taskfile: `build`, `test`, `lint`, `run`, `image`, `helm-lint` | IMPL-01 | DONE (`image` and `helm-lint` wait on IMPL-17 / IMPL-21) |
| IMPL-03 | 0 | `golangci-lint` config; CI pipeline (lint + test) | IMPL-01, PD-02 | DONE (first green run on GitHub Actions, 2026-10-09) |
| IMPL-04 | 1 | Config loader: env vars (+ optional file), defaults, validation, fail fast on bad config | IMPL-01 | DONE (`internal/config`; keys in README and 05-helm-chart) |
| IMPL-05 | 1 | Ingest handler: GET on configured path, parse `AID`/`GID`/`CPM`/`ACPM`/`uSV`, ignore unknown params, numeric bounds, query length cap | IMPL-04, AD-02 | TODO |
| IMPL-06 | 1 | Allowlists: GID, AID, source CIDR with trusted-proxy `X-Forwarded-For` handling | IMPL-05, AD-04 | TODO |
| IMPL-07 | 1 | Metrics registry: gauges, counters per data model in 03-architecture; custom registry + Go/process collectors | IMPL-04, AD-05 | TODO |
| IMPL-08 | 1 | Separate metrics HTTP server on `:9090` serving `/metrics` | IMPL-07, AD-03 | TODO |
| IMPL-09 | 1 | `/healthz` (process up) and `/readyz` (listeners up) | IMPL-05 | TODO |
| IMPL-10 | 1 | Configurable response body/status to the device | IMPL-05 | TODO |
| IMPL-11 | 1 | Structured JSON logging (`log/slog`), one line per accepted/rejected reading | IMPL-05 | TODO |
| IMPL-12 | 1 | Graceful shutdown on SIGTERM; HTTP server timeouts (read/header/write/idle) | IMPL-05 | TODO |
| IMPL-13 | 1 | Unit tests: parser, allowlists, metrics values; table-driven with real example query strings | IMPL-05..07 | TODO |
| IMPL-14 | 1 | README: run locally, curl smoke test, device configuration | IMPL-05 | TODO |
| IMPL-15 | 2 | Debug mode logging raw query + headers to capture real device behaviour | IMPL-11 | TODO |
| IMPL-16 | 2 | Test with the real GMC-500+ on the LAN; fill answers in 06-security-open-questions | IMPL-15 | TODO |
| IMPL-17 | 3 | Multi-stage Dockerfile: static build, distroless/scratch, UID 65532, no shell | IMPL-05, AD-08 | TODO |
| IMPL-18 | 3 | `.dockerignore`; version injected via `-ldflags` | IMPL-17 | TODO |
| IMPL-19 | 3 | CI: build (multi-arch if needed), scan image, push with semver + SHA tags | IMPL-17, PD-02, PD-04 | TODO |
| IMPL-20 | 3 | Verify image runs with `--read-only --cap-drop ALL --user 65532` | IMPL-17 | TODO |
| IMPL-21 | 4 | Helm chart scaffold, `_helpers.tpl`, labels, `values.yaml` | IMPL-17 | TODO |
| IMPL-22 | 4 | Deployment: ports, probes, resources, hardened securityContext, `automountServiceAccountToken: false`, config checksum annotation | IMPL-21 | TODO |
| IMPL-23 | 4 | ClusterIP Service exposing `http` and `metrics` ports | IMPL-21 | TODO |
| IMPL-24 | 4 | Ingress: class, host, ingest path only, annotations passthrough, TLS via existing secret name, HTTP must keep working | IMPL-23, AD-09 | TODO |
| IMPL-25 | 4 | ConfigMap from values, consumed via `envFrom` | IMPL-21, IMPL-04 | TODO |
| IMPL-26 | 4 | Optional Secret (`create` / `existingSecret`), wired via `secretKeyRef` | IMPL-21, AD-04 | TODO |
| IMPL-27 | 4 | `values.schema.json`; `NOTES.txt` printing device Website/URL | IMPL-21 | TODO |
| IMPL-28 | 4 | CI: `helm lint`, `kubeconform`, policy check on rendered manifests | IMPL-21 | TODO |
| IMPL-29 | 4 | Deploy to cluster; point device at Ingress; confirm metrics | IMPL-22..26 | TODO |
| IMPL-30 | 5 | Scrape config (ServiceMonitor template or Alloy snippet) | AD-07 | TODO |
| IMPL-31 | 5 | Grafana dashboard JSON | IMPL-30 | TODO |
| IMPL-32 | 5 | Alert rules: device silent, dose-rate threshold | IMPL-30, PD-07 | TODO |
| IMPL-33 | 6 | Optional NetworkPolicy template | IMPL-23 | TODO |
| IMPL-34 | 6 | Optional forwarding to gmcmap.com/radmon.org (async, timeout, failures don't affect ingest) | PD-05 | TODO |
| IMPL-35 | 6 | Optional in-app rate limiting | AD-04 | TODO |

---

## B. Architectural decisions

| ID | Decision | Options | Notes | Status |
|----|----------|---------|-------|--------|
| AD-01 | Implementation language | Go / Rust | Go (ADR-001): official Prometheus client, stdlib HTTP server | DONE (2026-10-09) |
| AD-02 | HTTP stack | stdlib `net/http` (Go 1.22+ routing) / chi / gin / echo | stdlib `net/http` (ADR-005) — two or three routes, no framework needed | DONE (2026-10-09) |
| AD-03 | Port layout | Separate ports (8080 ingest, 9090 metrics) / single port with path routing | Separate: 8080 ingest, 9090 metrics (ADR-004) | DONE (2026-10-09) |
| AD-04 | How to restrict the ingest endpoint | See layered list in 06 | Depends on Q-01..Q-04, Q-09 | TODO |
| AD-05 | Metric names, units and labels | `gid` only / `gid`+`aid` / friendly `device` name | Avoid unbounded labels; follow Prometheus naming (base units, `_total`) | TODO |
| AD-06 | Replicas and rollout strategy | 1 replica Recreate / 1 replica RollingUpdate | Stateless; >1 replica breaks scrape consistency | TODO |
| AD-07 | How metrics are scraped | ServiceMonitor (Prometheus Operator CRD) / Alloy discovery / pod annotations | Depends on how the LGTM stack scrapes today | TODO |
| AD-08 | Base image | `distroless/static:nonroot` / `scratch` / Chainguard static | distroless gives CA certs + nonroot user for free | TODO |
| AD-09 | Ingress controller specifics | ingress-nginx / Traefik / other | Controls annotations for no-redirect, IP allowlist, rate limit | TODO |
| AD-10 | Protect `/metrics`? | None (in-cluster + NetworkPolicy) / basic auth / TLS via `exporter-toolkit` | Probably none + NetworkPolicy | TODO |
| AD-11 | Stale data handling | Keep last value forever / drop series after N minutes without reports | Prefer explicit `last_report_timestamp` + alert; maybe also expire | TODO |
| AD-12 | Config source | Env vars only / env + YAML file | Env vars + optional YAML file, env wins (ADR-006) | DONE (2026-10-09) |
| AD-13 | Response to rejected requests | Honest 4xx / always 200 (don't leak why) | Depends on Q-05: does the device misbehave on non-200? | TODO |

---

## C. Project decisions

| ID | Decision | Options / notes | Status |
|----|----------|-----------------|--------|
| PD-01 | Project / binary / chart name | `gmc-exporter`, `gmc-ingest`, `geiger-exporter`… | DONE (2026-10-09: `radwatch-exporter` for repo, module, binary, image and chart. Not `gmc…`: GMC is GQ's model prefix and also a GM brand, and other protocols than GMCmap may be added later) |
| PD-02 | Repo host, CI and container registry | GitHub + GHCR / self-hosted Gitea + registry / other | DONE (2026-10-09: GitHub `dickeyf/radwatch-exporter`, public; GitHub Actions; images to GHCR `ghcr.io/dickeyf/…`) |
| PD-03 | Public or private; licence | Private / public with MIT, Apache-2.0, AGPL… | DONE (2026-10-09: public, Apache-2.0; `LICENSE` at repo root) |
| PD-04 | Target architectures | amd64 only / amd64 + arm64 (depends on cluster nodes) | TODO |
| PD-05 | Keep reporting to the public map? | Forward to gmcmap.com / radmon.org / stop public reporting | TODO |
| PD-06 | Exposure and hostname | LAN-only Ingress IP / also reachable from outside; hostname and LAN DNS entry | TODO |
| PD-07 | Alert thresholds and routing | Silence threshold (e.g. 5 min), dose-rate threshold, where alerts go | TODO |
| PD-08 | Versioning and release process | SemVer; chart version vs app version; tag-driven releases | TODO |
| PD-09 | Go version policy | Pin latest stable; update via Renovate/Dependabot | TODO |
| PD-10 | Namespace and how the TLS secret is provisioned | External Secrets / cert-manager / manual; secret name convention | TODO |
| PD-11 | Testing level | Unit only / + integration test with `httptest` / + kind cluster e2e in CI | TODO |
| PD-12 | Support more than one counter? | Single device / multiple (affects labels and allowlist UX) | TODO |
