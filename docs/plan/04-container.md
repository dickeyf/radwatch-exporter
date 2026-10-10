# 04 — Container image

## Requirements

- Minimal: only the static binary (plus CA certificates if outbound forwarding to gmcmap.com over HTTPS is ever enabled).
- Runs as a non-root user (numeric UID so `runAsNonRoot` can be verified).
- Works with a **read-only root filesystem**: the service writes nothing to disk. Logs go to stdout.
- No shell, no package manager in the final image.
- Listens on unprivileged ports only (8080, 9090), so no `NET_BIND_SERVICE` capability is needed. Port 80/443 is the Ingress's job.

## Dockerfile

The [Dockerfile](../../Dockerfile) at the repo root (IMPL-17, ADR-010):

- **Build stage:** `golang:1.26-bookworm`, run on the build machine's platform and cross-compiling to `TARGETOS`/`TARGETARCH`, so multi-arch builds (PD-04) need no emulation. Static binary (`CGO_ENABLED=0`, `-trimpath`, `-s -w`), version injected with `--build-arg VERSION=...` into `main.version` (IMPL-18). BuildKit cache mounts for modules and the build cache.
- **Final stage:** `gcr.io/distroless/static-debian12:nonroot`, only the binary at `/radwatch-exporter`, `USER 65532:65532`, ports 8080 and 9090.
- Both base images are pinned by digest. `GO_VERSION` must match `go.mod` (PD-09).
- [.dockerignore](../../.dockerignore) is an allowlist: only `go.mod`, `go.sum`, `cmd/` and `internal/`, without tests.

Build locally with `make image` (passes `VERSION` from `git describe`).

Notes:

- `distroless/static:nonroot` includes CA certs, `/etc/passwd` with the `nonroot` user and tzdata, and nothing else (chosen in ADR-010 over `scratch` and Chainguard static).
- Base images are pinned by digest; Renovate/Dependabot should keep them current (PD-09).
- Build for `linux/amd64` and `linux/arm64` if any cluster node is ARM (PD-04).
- No `HEALTHCHECK` instruction: Kubernetes probes handle health, and there is no shell or curl in the image anyway. If a probe-from-inside is ever wanted, add a `healthcheck` subcommand to the binary.

## Runtime contract (enforced by the Helm chart)

```yaml
securityContext:              # container
  runAsNonRoot: true
  runAsUser: 65532
  runAsGroup: 65532
  readOnlyRootFilesystem: true
  allowPrivilegeEscalation: false
  capabilities:
    drop: ["ALL"]
  seccompProfile:
    type: RuntimeDefault
```

The pod also sets `automountServiceAccountToken: false` (the service never talks to the API server).

## Verification

The CI `image` job (IMPL-20) builds the image on every push, then:

- runs it with `--read-only --user 65532:65532 --cap-drop ALL --security-opt no-new-privileges`;
- runs the Phase 1 smoke test against it (upload accepted, `radwatch_cpm` on `/metrics`);
- fails if the image is 20 MiB or larger, or its user is not `65532:65532`.

Still to add with IMPL-19: image scan (Trivy/Grype, fail on fixable HIGH/CRITICAL), multi-arch build and push to GHCR.
