# 04 — Container image

## Requirements

- Minimal: only the static binary (plus CA certificates if outbound forwarding to gmcmap.com over HTTPS is ever enabled).
- Runs as a non-root user (numeric UID so `runAsNonRoot` can be verified).
- Works with a **read-only root filesystem**: the service writes nothing to disk. Logs go to stdout.
- No shell, no package manager in the final image.
- Listens on unprivileged ports only (8080, 9090), so no `NET_BIND_SERVICE` capability is needed. Port 80/443 is the Ingress's job.

## Planned Dockerfile (sketch)

```dockerfile
# syntax=docker/dockerfile:1

ARG GO_VERSION=1.xx   # pin in AD/PD decision; keep in sync with go.mod

FROM golang:${GO_VERSION} AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
      -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/radwatch-exporter ./cmd/radwatch-exporter

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/radwatch-exporter /radwatch-exporter
USER 65532:65532
EXPOSE 8080 9090
ENTRYPOINT ["/radwatch-exporter"]
```

Notes:

- `distroless/static:nonroot` includes CA certs, `/etc/passwd` with the `nonroot` user and tzdata, and nothing else. `scratch` is the alternative (AD-08): smaller, but you must copy CA certs yourself.
- Pin the base images by digest once chosen (Renovate/Dependabot can keep them current).
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

- `docker run --rm --read-only --user 65532:65532 --cap-drop ALL -p 8080:8080 -p 9090:9090 <image>` then the Phase 1 curl smoke test.
- Image scan (Trivy/Grype) in CI; fail on fixable HIGH/CRITICAL.
- Check final image size (target: < 20 MB).
