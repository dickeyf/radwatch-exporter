# radwatch-exporter — Project Plan

A lightweight service that receives readings pushed by a GQ GMC-500+ Geiger counter over WiFi and exposes them as Prometheus metrics for an LGTM (Loki / Grafana / Tempo / Mimir) stack running in Kubernetes.

> Name: `radwatch-exporter` (PD-01). The first supported upload protocol is GQ's GMCmap (`log2.asp`); more protocols may follow.

## Documents

| File | Content |
|------|---------|
| [01-overview.md](01-overview.md) | Goal, scope, how the counter talks to a server, glossary |
| [02-roadmap.md](02-roadmap.md) | Phased roadmap with exit criteria |
| [03-architecture.md](03-architecture.md) | Architecture overview and initial decisions (ADRs) |
| [04-container.md](04-container.md) | Container image plan (minimal, non-root, read-only rootfs) |
| [05-helm-chart.md](05-helm-chart.md) | Helm chart (Deployment, Services incl. the optional ingest LoadBalancer, optional Ingress, ServiceMonitor, Grafana dashboard and alert rules, ConfigMap) |
| [06-security-open-questions.md](06-security-open-questions.md) | Endpoint restriction, TLS, auth, open questions |
| [07-worklist.md](07-worklist.md) | Work items: implementation, architectural decisions, project decisions |

See also the [deploy guide](../deploy-guide.md) (the reference deployment, verified end to end) and the [reference dose rates](../reference-dose-rates.md) for dashboards.

## Using this plan with Claude Code

- Start from `07-worklist.md`. Take one item at a time.
- Before taking an `AD-*` or `PD-*` item, write the decision in `03-architecture.md` (ADR format) or in the worklist itself, then mark it done.
- Implementation items (`IMPL-*`) list what they depend on. Don't start an item until those are done or explicitly stubbed.
- Keep this folder as the source of truth: when a decision changes, update the doc in the same commit as the code.
