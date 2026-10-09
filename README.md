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

## Configuration

Defaults, then an optional YAML file (`RADWATCH_CONFIG_FILE`), then env vars; a set env var always wins. Invalid config stops the process at startup.

| Env var | YAML key | Default |
|---------|----------|---------|
| `RADWATCH_LISTEN_ADDR` | `listen_addr` | `:8080` |
| `RADWATCH_METRICS_ADDR` | `metrics_addr` | `:9090` |
| `RADWATCH_LOG_LEVEL` | `log_level` | `info` |
| `RADWATCH_ALLOWED_CIDRS` | `allowed_cidrs` | empty (allow all) |
| `RADWATCH_TRUSTED_PROXIES` | `trusted_proxies` | empty |
| `RADWATCH_DEVICE_NAMES` | `device_names` | empty |
| `RADWATCH_GMCMAP_PATH` | `gmcmap.path` | `/log2.asp` |
| `RADWATCH_GMCMAP_ALLOWED_GIDS` | `gmcmap.allowed_gids` | empty (allow all) |
| `RADWATCH_GMCMAP_ALLOWED_AIDS` | `gmcmap.allowed_aids` | empty (allow all) |
| `RADWATCH_GMCMAP_RESPONSE_STATUS` | `gmcmap.response_status` | `200` |
| `RADWATCH_GMCMAP_RESPONSE_BODY` | `gmcmap.response_body` | `OK.ERR0` |
| `RADWATCH_GMCMAP_FORWARD_URL` | `gmcmap.forward_url` | empty (off) |

Lists are comma-separated in env vars (`a,b`) and YAML sequences in the file. CIDR lists accept bare IPs. Device names are `id=name,id=name` in env vars and a map in YAML:

```yaml
listen_addr: ":8080"
allowed_cidrs: ["192.168.1.50"]
device_names:
  "0034021": basement
gmcmap:
  allowed_gids: ["0034021"]
```

## Licence

Apache License 2.0. See [LICENSE](LICENSE).
