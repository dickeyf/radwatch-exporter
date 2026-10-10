package ingest

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/dickeyf/radwatch-exporter/internal/config"
)

func prefixes(s ...string) config.Prefixes {
	var out config.Prefixes
	for _, p := range s {
		out = append(out, netip.MustParsePrefix(p))
	}
	return out
}

func TestResolve(t *testing.T) {
	g := SourceGuard{Trusted: prefixes("10.0.0.0/8")}
	tests := []struct {
		name, remote string
		xff          []string
		want         string
		wantOK       bool
	}{
		{"direct peer", "192.168.1.50:1234", nil, "192.168.1.50", true},
		{"untrusted peer ignores XFF", "192.168.1.66:1234", []string{"192.168.1.50"}, "192.168.1.66", true},
		{"trusted proxy", "10.1.2.3:1234", []string{"192.168.1.50"}, "192.168.1.50", true},
		{"spoofed leftmost entry is skipped", "10.1.2.3:1234", []string{"1.2.3.4, 192.168.1.50"}, "192.168.1.50", true},
		{"several trusted hops", "10.1.2.3:1234", []string{"192.168.1.50, 10.9.9.9", "10.8.8.8"}, "192.168.1.50", true},
		{"all hops trusted", "10.1.2.3:1234", []string{"10.9.9.9"}, "10.9.9.9", true},
		{"trusted proxy without XFF", "10.1.2.3:1234", nil, "10.1.2.3", true},
		{"garbage XFF", "10.1.2.3:1234", []string{"not-an-ip"}, "", false},
		{"IPv4-mapped IPv6 peer", "[::ffff:192.168.1.50]:1234", nil, "192.168.1.50", true},
		{"IPv6 peer", "[fd00::1]:1234", nil, "fd00::1", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remote
			for _, v := range tt.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			got, ok := g.Resolve(r)
			if ok != tt.wantOK || (ok && got.String() != tt.want) {
				t.Errorf("got %v %v, want %v %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

type countRecorder map[string]int

func (c countRecorder) Record(Reading)                { /* not used */ }
func (c countRecorder) Count(protocol, result string) { c[protocol+"/"+result]++ }

func TestMiddleware(t *testing.T) {
	g := SourceGuard{Allowed: prefixes("192.168.1.50/32"), Trusted: prefixes("10.0.0.0/8")}
	tests := []struct {
		name, remote, xff string
		wantCode          int
	}{
		{"allowed direct", "192.168.1.50:1", "", http.StatusOK},
		{"allowed through proxy", "10.1.2.3:1", "192.168.1.50", http.StatusOK},
		{"other source", "192.168.1.66:1", "", http.StatusForbidden},
		{"spoofed XFF from untrusted peer", "192.168.1.66:1", "192.168.1.50", http.StatusForbidden},
		{"unparseable XFF", "10.1.2.3:1", "junk", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := countRecorder{}
			var seen string
			next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = ClientIP(r.Context()) })
			h := g.Middleware("gmcmap", rec, slog.New(slog.NewTextHandler(io.Discard, nil)))(next)

			r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remote
			if tt.xff != "" {
				r.Header.Set("X-Forwarded-For", tt.xff)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)

			if w.Code != tt.wantCode {
				t.Errorf("got %d, want %d", w.Code, tt.wantCode)
			}
			if tt.wantCode == http.StatusOK && seen != "192.168.1.50" {
				t.Errorf("client IP in context: got %q", seen)
			}
			if tt.wantCode == http.StatusForbidden && rec["gmcmap/rejected_allowlist"] != 1 {
				t.Errorf("rejection not counted: %v", rec)
			}
		})
	}
}

func TestMiddlewareEmptyAllowlistAllowsAll(t *testing.T) {
	h := SourceGuard{}.Middleware("gmcmap", countRecorder{}, slog.New(slog.NewTextHandler(io.Discard, nil)))(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r.RemoteAddr = "203.0.113.9:1"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d", w.Code)
	}
}
