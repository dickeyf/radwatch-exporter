package ingest

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDebugRequests(t *testing.T) {
	var buf bytes.Buffer
	mux := http.NewServeMux()
	mux.HandleFunc("GET /log2.asp", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("OK.ERR0")) })
	mux.HandleFunc("GET /healthz", func(http.ResponseWriter, *http.Request) {})
	h := DebugRequests(slog.New(slog.NewJSONHandler(&buf, nil)), mux)

	do := func(target string, header http.Header) {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
		for k, v := range header {
			r.Header[k] = v
		}
		h.ServeHTTP(httptest.NewRecorder(), r)
	}
	do("/log2.asp?AID=1&GID=2&CPM=15&Extra=x", http.Header{"User-Agent": {"GMC"}, "Authorization": {"Basic c2VjcmV0"}})
	do("/wrong/path?GID=2", nil)
	do("/healthz", nil)

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2 (probes skipped):\n%s", len(lines), buf.String())
	}
	var first, second struct {
		RequestURI    string            `json:"request_uri"`
		Status        int               `json:"status"`
		ResponseBytes int               `json:"response_bytes"`
		Headers       map[string]string `json:"headers"`
	}
	if err := json.Unmarshal(lines[0], &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(lines[1], &second); err != nil {
		t.Fatal(err)
	}
	if first.RequestURI != "/log2.asp?AID=1&GID=2&CPM=15&Extra=x" || first.Status != 200 || first.ResponseBytes != 7 {
		t.Errorf("first line: %+v", first)
	}
	if first.Headers["User-Agent"] != "GMC" || first.Headers["Authorization"] != "[redacted]" {
		t.Errorf("headers: %v", first.Headers)
	}
	if second.RequestURI != "/wrong/path?GID=2" || second.Status != http.StatusNotFound {
		t.Errorf("wrong-path line: %+v", second)
	}
}
