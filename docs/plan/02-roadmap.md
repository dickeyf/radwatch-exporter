# 02 — Roadmap

Each phase ends with something that works on its own. Phase IDs map to work items in [07-worklist.md](07-worklist.md).

## Phase 0 — Decisions and bootstrap

**Goal:** a repo you can build and test, with the blocking decisions made.

- Settle the blocking decisions: AD-01 (language), AD-02 (HTTP stack), AD-03 (port layout), PD-01 (name), PD-02 (repo/registry), PD-03 (licence).
- `go mod init`, folder layout, Makefile/Taskfile, `.gitignore`, `golangci-lint` config.
- CI skeleton: lint + test on push.

**Exit:** `go build ./...` and `go test ./...` pass in CI on an empty skeleton.

## Phase 1 — Ingest + metrics (local)

**Goal:** the service runs on a laptop, accepts a counter request, and exposes metrics.

- Ingest handler: parse `AID`, `GID`, `CPM`, `ACPM`, `uSV`; validate; reply 200.
- Metrics: gauges per device, last-report timestamp, request counters by outcome.
- `/healthz` and `/readyz`.
- Config from env vars / file (listen addresses, ingest path, allowlists).
- Structured JSON logging.
- Unit tests with recorded example requests; a `curl` smoke test in the README.

**Exit:** `curl "localhost:8080/log2.asp?AID=1&GID=2&CPM=15&ACPM=13.2&uSV=0.075"` makes `radwatch_cpm{protocol="gmcmap",device_id="2",device="2"} 15` appear on `/metrics`.

## Phase 2 — Real device on the LAN

**Goal:** prove the actual counter talks to the service, before investing in k8s packaging.

- Run the binary (or container) on a LAN machine on port 80.
- Point the GMC-500+ Website/URL at it.
- Capture what the device really sends: headers, query string, method, retry behaviour, what response it expects. Record answers in `06-security-open-questions.md`.

**Exit:** a full hour of readings from the real device in `/metrics`, and the open questions about device behaviour answered or confirmed unanswerable.

## Phase 3 — Container image

**Goal:** a minimal, hardened image.

- Multi-stage Dockerfile, static binary, distroless/scratch, non-root, read-only rootfs compatible.
- Image scan in CI (e.g. Trivy/Grype).
- Push to the chosen registry with semver + git SHA tags.

**Exit:** image runs with `--read-only --user 65532 --cap-drop ALL` and passes the Phase 1 smoke test.

## Phase 4 — Helm chart

**Goal:** deploy into the cluster.

- Chart with Deployment, ClusterIP Service (ingest + metrics ports), optional ingest LoadBalancer Service (counters configured by IP), optional Ingress, ConfigMap, optional ServiceMonitor.
- Chart published to GHCR as an OCI artifact on release tags (IMPL-36).
- TLS secret referenced by name only (created elsewhere in the same namespace).
- Hardened pod security context.
- `helm lint --strict`, `helm template` + kubeconform, and kube-linter in CI.

**Exit:** the chart is installed in the cluster; the device's uploads reach it (through the ingest Service, since the device needs an IP, Q-08) and show up in Prometheus.

## Phase 5 — Observability integration

**Goal:** data in Grafana with alerts.

- Scrape config (ServiceMonitor or Alloy, depending on AD-07).
- Grafana dashboard JSON (CPM, µSv/h, time since last report; ACPM only as a long-term reference), log scales, reference lines (background, 1 mSv/year, and 3.6 R/h "not great, not terrible"), IMPL-31.
- Alert rules: device silent > N minutes; dose rate above threshold.

**Exit:** dashboard live; silence alert fires when the counter is unplugged. (Dashboard since chart 0.3.0, alert rules since chart 0.4.0.) **Met 2026-10-10:** with the counter unplugged, `RadwatchDeviceSilent` fired and was mailed by Alertmanager, then resolved once uploads resumed.

## Phase 6 — Hardening and extras (optional)

- Forward readings to gmcmap.com / radmon.org (PD-05).
- Rate limiting per source IP.
- NetworkPolicy (IMPL-33, chart 0.5.0).
- Chart release automation beyond IMPL-36 (chart versioned separately, `appVersion` = image, PD-08).
- Optional Secret (IMPL-26).

## Milestone summary

| Milestone | After phase | What you have | Reached |
|-----------|-------------|---------------|---------|
| M1 | 1 | Working exporter on a laptop | 2026-10-09 |
| M2 | 2 | Real device confirmed, unknowns answered | 2026-10-09 |
| M3 | 4 | Running in k8s | 2026-10-10 |
| M4 | 5 | Dashboard + alerts | 2026-10-10 |
