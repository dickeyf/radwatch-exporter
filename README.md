# radwatch-exporter

Receives readings pushed by radiation counters (first protocol: GQ GMCmap upload, e.g. a GMC-500+) and exposes them as Prometheus metrics.

See [docs/plan](docs/plan/README.md) for the project plan and worklist.

## Development

```sh
make build   # bin/radwatch-exporter
make test
make lint    # requires golangci-lint v2
make run
```
