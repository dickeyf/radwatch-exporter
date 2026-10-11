# 01 — Overview

## Goal

Get radiation readings from a GQ GMC-500+ Geiger counter into the home LGTM stack as Prometheus metrics, so they can be graphed in Grafana, alerted on, and kept long-term in Mimir.

## How the counter talks to a server (known facts)

- The GMC-500+ connects to WiFi and periodically pushes a reading to a configurable server. This is set on the device under **Menu → Server → Website / URL**.
- **Website** is a host name or IP. **URL** is a path (for example `log2.asp` for gmcmap.com).
- The device sends a plain **HTTP GET** with the reading in the query string, about once per minute:

  ```
  GET /<URL>?AID=<user id>&GID=<counter id>&CPM=15&ACPM=13.2&uSV=0.075
  ```

  | Param | Meaning |
  |-------|---------|
  | `AID` | Account / user ID (as configured on the device) |
  | `GID` | Geiger counter ID (as configured on the device) |
  | `CPM` | Counts per minute |
  | `ACPM` | Averaged CPM |
  | `uSV` | Dose rate in µSv/h, computed by the device from its calibration (not energy-compensated) |

- Parameter names are case-sensitive in existing open-source receivers; radwatch matches them in any case (IMPL-05). Some firmware may send extra fields; the receiver must ignore unknown parameters.

Everything else about the device's network behaviour (TLS, ports, auth, retries, response expectations) was unknown before Phase 2; the answers and the remaining open questions are in [06-security-open-questions.md](06-security-open-questions.md).

## Scope

**In scope**

- A small HTTP service that accepts the counter's GET requests and turns them into Prometheus metrics.
- A `/metrics` endpoint scraped by the existing stack (Alloy or Prometheus Operator).
- Health endpoints for Kubernetes probes.
- A minimal, non-root, read-only container image.
- A Helm chart: Deployment, ClusterIP Service, optional ingest LoadBalancer Service, optional Ingress, ConfigMap, optional ServiceMonitor, optional Grafana dashboard ConfigMap and alert rules (PrometheusRule).
- Basic request filtering (allowlist) since the device likely has no real authentication.

**Optional / later**

- Forwarding each reading to gmcmap.com (or radmon.org) so the public map keeps working.

**Out of scope**

- Persisting readings in a database (Mimir is the storage).
- A web UI.
- Reading the counter over USB/serial.
- Any changes to the counter's firmware.

## Glossary

| Term | Meaning |
|------|---------|
| Ingest endpoint | The HTTP path the counter calls (e.g. `/log2.asp`) |
| Metrics endpoint | `/metrics`, scraped by Prometheus/Alloy |
| LGTM | Loki, Grafana, Tempo, Mimir |
| AID / GID | User ID / counter ID sent by the device |
