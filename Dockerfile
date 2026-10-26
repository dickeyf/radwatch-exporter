# syntax=docker/dockerfile:1

# Base images are pinned by digest (ADR-010) and kept current by Renovate (PD-09),
# which bumps the golang tag together with the go directive in go.mod.
FROM --platform=$BUILDPLATFORM golang:1.27-bookworm@sha256:972b4110ba0f47539a975c31e52c64765e1ff19f2d74469ae4a595c6676e80a5 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
      -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/radwatch-exporter ./cmd/radwatch-exporter

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /out/radwatch-exporter /radwatch-exporter
USER 65532:65532
EXPOSE 8080 9090
ENTRYPOINT ["/radwatch-exporter"]
