package ingest

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/dickeyf/radwatch-exporter/internal/config"
)

type clientIPKey struct{}

// ClientIP returns the client address resolved by SourceGuard, or "" when
// the request did not go through it.
func ClientIP(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPKey{}).(string)
	return ip
}

// SourceGuard resolves the real client address and enforces the source-IP
// allowlist (ADR-009). X-Forwarded-For is only trusted when the direct peer
// is in Trusted, and is read right to left past every trusted hop, so a
// client cannot spoof its address by sending its own header.
type SourceGuard struct {
	Allowed config.Prefixes // empty allows every source
	Trusted config.Prefixes
}

// Resolve returns the client address of r, or false if it can't be parsed.
func (g SourceGuard) Resolve(r *http.Request) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	client, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	client = client.Unmap()
	if !g.Trusted.Contains(client) {
		return client, true
	}

	var hops []string
	for _, h := range r.Header.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(h, ",")...)
	}
	for i := len(hops) - 1; i >= 0; i-- {
		hop, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			return netip.Addr{}, false
		}
		client = hop.Unmap()
		if !g.Trusted.Contains(client) {
			break
		}
	}
	return client, true
}

// Middleware rejects requests from sources outside Allowed with 403 and
// stores the resolved client address in the request context.
func (g SourceGuard) Middleware(protocol string, rec Recorder, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			client, ok := g.Resolve(r)
			clientStr := ""
			if ok {
				clientStr = client.String()
			}
			if len(g.Allowed) > 0 && (!ok || !g.Allowed.Contains(client)) {
				rec.Count(protocol, ResultRejectedAllowlist)
				log.LogAttrs(r.Context(), slog.LevelWarn, "reading rejected",
					slog.String("protocol", protocol),
					slog.String("result", ResultRejectedAllowlist),
					slog.Int("status", http.StatusForbidden),
					slog.String("reason", "source not allowed"),
					slog.String("client_ip", clientStr),
					slog.String("remote_addr", r.RemoteAddr),
				)
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientIPKey{}, clientStr)))
		})
	}
}
