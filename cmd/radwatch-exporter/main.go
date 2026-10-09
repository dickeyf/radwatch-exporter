// Command radwatch-exporter receives readings pushed by radiation counters
// (first protocol: GQ GMCmap upload) and exposes them as Prometheus metrics.
package main

import (
	"log/slog"
	"os"

	"github.com/dickeyf/radwatch-exporter/internal/config"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	var level slog.Level
	_ = level.UnmarshalText([]byte(cfg.LogLevel)) // validated by config.Load
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	logger.Info("starting radwatch-exporter",
		"version", version,
		"listen_addr", cfg.ListenAddr,
		"metrics_addr", cfg.MetricsAddr,
		"gmcmap_path", cfg.GMCMap.Path,
	)
}
