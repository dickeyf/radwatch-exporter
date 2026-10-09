BINARY  := radwatch-exporter
PKG     := ./cmd/$(BINARY)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
IMAGE   ?= $(BINARY):$(VERSION)

.PHONY: all build test lint fmt vet run tidy image helm-lint clean

all: lint test build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) $(PKG)

test:
	go test -race -cover ./...

lint:
	golangci-lint run ./...

fmt:
	golangci-lint fmt ./...

vet:
	go vet ./...

run:
	go run $(PKG)

tidy:
	go mod tidy

image:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE) .

helm-lint:
	helm lint deploy/helm/$(BINARY)

clean:
	rm -rf bin dist
