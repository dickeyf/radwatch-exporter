// Command radwatch-exporter receives readings pushed by a GQ GMC-500+ Geiger
// counter and exposes them as Prometheus metrics.
package main

import (
	"log/slog"
	"os"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger.Info("starting radwatch-exporter", "version", version)
}
