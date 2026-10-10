// Command radwatch-exporter receives readings pushed by radiation counters
// (first protocol: GQ GMCmap upload) and exposes them as Prometheus metrics.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dickeyf/radwatch-exporter/internal/config"
	"github.com/dickeyf/radwatch-exporter/internal/ingest/gmcmap"
	"github.com/dickeyf/radwatch-exporter/internal/metrics"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

const shutdownTimeout = 10 * time.Second

func main() {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	var level slog.Level
	_ = level.UnmarshalText([]byte(cfg.LogLevel)) // validated by config.Load
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg, logger); err != nil {
		logger.Error("exiting", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	m := metrics.New(cfg.DeviceNames, cfg.StaleAfter)
	m.InitProtocol(gmcmap.Protocol)

	ingestMux := http.NewServeMux()
	gmcmap.New(cfg.GMCMap, m).Register(ingestMux)
	metricsMux := http.NewServeMux()
	metricsMux.Handle("GET /metrics", m.Handler())

	servers := []*http.Server{newServer(cfg.ListenAddr, ingestMux), newServer(cfg.MetricsAddr, metricsMux)}
	errc := make(chan error, len(servers))
	for _, srv := range servers {
		go func() {
			if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
				errc <- err
			}
		}()
	}
	logger.Info("started radwatch-exporter",
		"version", version,
		"listen_addr", cfg.ListenAddr,
		"metrics_addr", cfg.MetricsAddr,
		"gmcmap_path", cfg.GMCMap.Path,
	)

	var runErr error
	select {
	case <-ctx.Done():
		logger.Info("shutting down")
	case runErr = <-errc:
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	for _, srv := range servers {
		if err := srv.Shutdown(shutdownCtx); err != nil {
			runErr = errors.Join(runErr, err)
		}
	}
	return runErr
}

// newServer applies timeouts suited to tiny requests from LAN devices.
func newServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
}
