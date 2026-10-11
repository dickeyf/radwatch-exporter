package forward

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/dickeyf/radwatch-exporter/internal/config"
	"github.com/dickeyf/radwatch-exporter/internal/ingest"
)

type fakeRecorder struct {
	mu       sync.Mutex
	readings int
	forwards map[string]int
}

func (f *fakeRecorder) Record(ingest.Reading) { f.mu.Lock(); f.readings++; f.mu.Unlock() }
func (f *fakeRecorder) Count(string, string)  {}
func (f *fakeRecorder) CountForward(target, result string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.forwards == nil {
		f.forwards = map[string]int{}
	}
	f.forwards[target+"/"+result]++
}

func ptr(f float64) *float64 { return &f }

var reading = ingest.Reading{
	Protocol: "gmcmap", AccountID: "0230111", DeviceID: "0034021",
	CPM: 15, ACPM: ptr(13.2), SvPerH: ptr(0.05e-6),
}

// server records the query of every request and answers with body.
func server(t *testing.T, status int, body string) (*httptest.Server, *[]url.Values) {
	t.Helper()
	var mu sync.Mutex
	var got []url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.URL.Query())
		mu.Unlock()
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func run(t *testing.T, targets []Target, readings ...ingest.Reading) (*fakeRecorder, string) {
	t.Helper()
	var logs bytes.Buffer
	rec := &fakeRecorder{}
	f := New(rec, rec, slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})), targets...)
	f.Start(t.Context())
	for _, r := range readings {
		f.Record(r)
	}
	f.Close()
	return rec, logs.String()
}

func TestGMCMapForward(t *testing.T) {
	srv, got := server(t, http.StatusOK, "OK.ERR0")
	rec, _ := run(t, []Target{GMCMap{URL: srv.URL + "/log2.asp"}}, reading)
	if rec.readings != 1 {
		t.Errorf("reading not recorded locally: %d", rec.readings)
	}
	if rec.forwards["gmcmap/ok"] != 1 {
		t.Errorf("forwards: %v", rec.forwards)
	}
	want := url.Values{"AID": {"0230111"}, "GID": {"0034021"}, "CPM": {"15"}, "ACPM": {"13.2"}, "uSV": {"0.05"}}
	if len(*got) != 1 || (*got)[0].Encode() != want.Encode() {
		t.Errorf("query: got %v, want %v", *got, want)
	}
}

func TestGMCMapRefusedAndSkipped(t *testing.T) {
	srv, _ := server(t, http.StatusOK, "ERR1. User is not found.")
	other := reading
	other.Protocol = "other"
	rec, logs := run(t, []Target{GMCMap{URL: srv.URL}}, reading, other)
	if rec.forwards["gmcmap/error"] != 1 || rec.forwards["gmcmap/skipped"] != 1 {
		t.Errorf("forwards: %v", rec.forwards)
	}
	if !strings.Contains(logs, "User is not found") {
		t.Errorf("refusal body not logged: %s", logs)
	}
}

func TestRadmonForward(t *testing.T) {
	srv, got := server(t, http.StatusOK, "OK")
	cfg := config.Radmon{ForwardURL: srv.URL + "/radmon.php", User: "station", Password: "s3cret", DeviceID: "0034021"}
	other := reading
	other.DeviceID = "0034022"
	rec, _ := run(t, []Target{Radmon{Config: cfg}}, reading, other)
	if rec.forwards["radmon/ok"] != 1 || rec.forwards["radmon/skipped"] != 1 {
		t.Errorf("forwards: %v", rec.forwards)
	}
	want := url.Values{"function": {"submit"}, "user": {"station"}, "password": {"s3cret"}, "value": {"15"}, "unit": {"CPM"}}
	if len(*got) != 1 || (*got)[0].Encode() != want.Encode() {
		t.Errorf("query: got %v, want %v", *got, want)
	}
}

func TestRadmonBadLoginAndNoPasswordInLogs(t *testing.T) {
	srv, _ := server(t, http.StatusOK, "Incorrect login.")
	cfg := config.Radmon{ForwardURL: srv.URL, User: "station", Password: "s3cret"}
	rec, logs := run(t, []Target{Radmon{Config: cfg}}, reading)
	if rec.forwards["radmon/error"] != 1 {
		t.Errorf("forwards: %v", rec.forwards)
	}

	// A connection error must not log the URL, which carries the password.
	srv.Close()
	rec, logs2 := run(t, []Target{Radmon{Config: cfg}}, reading)
	if rec.forwards["radmon/error"] != 1 {
		t.Errorf("forwards after close: %v", rec.forwards)
	}
	if strings.Contains(logs+logs2, "s3cret") {
		t.Errorf("password in logs: %s%s", logs, logs2)
	}
}

func TestForwardURLWithQueryAndStatus(t *testing.T) {
	srv, got := server(t, http.StatusServiceUnavailable, "")
	rec, _ := run(t, []Target{GMCMap{URL: srv.URL + "/log2.asp?src=radwatch"}}, reading)
	if rec.forwards["gmcmap/error"] != 1 {
		t.Errorf("forwards: %v", rec.forwards)
	}
	if len(*got) != 1 || (*got)[0].Get("src") != "radwatch" || (*got)[0].Get("GID") != "0034021" {
		t.Errorf("query: %v", *got)
	}
}

// blockingTarget holds its sender until released, to fill the queue.
type blockingTarget struct{ release chan struct{} }

func (blockingTarget) Name() string { return "slow" }
func (b blockingTarget) Request(context.Context, ingest.Reading) (*http.Request, error) {
	<-b.release
	return nil, nil
}
func (blockingTarget) Check(int, string) error { return nil }

func TestQueueFullDrops(t *testing.T) {
	b := blockingTarget{release: make(chan struct{})}
	rec := &fakeRecorder{}
	f := New(rec, rec, slog.New(slog.NewTextHandler(io.Discard, nil)), b)
	f.Start(t.Context())
	// One reading is held by the sender, queueSize wait in the queue, the rest drop.
	for range queueSize + 5 {
		f.Record(reading)
	}
	close(b.release)
	f.Close()
	if rec.readings != queueSize+5 {
		t.Errorf("all readings must be recorded locally: %d", rec.readings)
	}
	if d := rec.forwards["slow/dropped"]; d < 4 || d > 5 {
		t.Errorf("dropped: %d (forwards %v)", d, rec.forwards)
	}
}

func TestTargets(t *testing.T) {
	cfg := config.Default()
	if len(Targets(cfg)) != 0 {
		t.Error("forwarding must be off by default")
	}
	cfg.GMCMap.ForwardURL = "http://www.gmcmap.com/log2.asp"
	cfg.Radmon = config.Radmon{ForwardURL: "https://radmon.org/radmon.php", User: "u", Password: "p"}
	ts := Targets(cfg)
	if len(ts) != 2 || ts[0].Name() != "gmcmap" || ts[1].Name() != "radmon" {
		t.Errorf("targets: %v", ts)
	}
}

func TestNum(t *testing.T) {
	for in, want := range map[float64]string{15: "15", 13.2: "13.2", 0.05e-6 * 1e6: "0.05", 0.07 * 1e-6 * 1e6: "0.07", 1234567: "1234567"} {
		if got := num(in); got != want {
			t.Errorf("num(%v) = %q, want %q", in, got, want)
		}
	}
}
