package ingest

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// redactedHeaders are never logged in clear, even in debug mode.
var redactedHeaders = map[string]bool{
	"Authorization":       true,
	"Proxy-Authorization": true,
	"Cookie":              true,
}

// DebugRequests logs every request on the ingest port, whatever its path, with
// the raw request line, all headers and the reply status (IMPL-15). It exists
// to capture what a real device sends (Q-06, Q-08 in docs/plan/06), so it
// wraps the whole mux and also shows requests to wrong paths. Probe paths are
// skipped to keep the output readable.
func DebugRequests(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)

		headers := make([]any, 0, len(r.Header))
		for name, values := range r.Header {
			v := strings.Join(values, ", ")
			if redactedHeaders[name] {
				v = "[redacted]"
			}
			headers = append(headers, slog.String(name, v))
		}
		log.LogAttrs(r.Context(), slog.LevelInfo, "debug request",
			slog.String("method", r.Method),
			slog.String("request_uri", r.RequestURI),
			slog.String("proto", r.Proto),
			slog.String("host", r.Host),
			slog.Group("headers", headers...),
			slog.Int64("content_length", r.ContentLength),
			slog.String("remote_addr", r.RemoteAddr),
			slog.Int("status", sw.status),
			slog.Int("response_bytes", sw.bytes),
			slog.Duration("duration", time.Since(start)),
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}
