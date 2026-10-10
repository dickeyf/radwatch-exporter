// Package health serves the liveness and readiness probes.
package health

import (
	"net/http"
	"sync/atomic"
)

// Checker answers /healthz (the process is up) and /readyz (the listeners
// are up and the service is not shutting down).
type Checker struct {
	ready atomic.Bool
}

// SetReady flips the readiness state.
func (c *Checker) SetReady(ready bool) { c.ready.Store(ready) }

// Register adds GET /healthz and GET /readyz to mux.
func (c *Checker) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		write(w, http.StatusOK, "ok")
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if c.ready.Load() {
			write(w, http.StatusOK, "ready")
			return
		}
		write(w, http.StatusServiceUnavailable, "not ready")
	})
}

func write(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body + "\n"))
}
