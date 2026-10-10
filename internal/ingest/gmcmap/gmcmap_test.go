package gmcmap

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dickeyf/radwatch-exporter/internal/config"
	"github.com/dickeyf/radwatch-exporter/internal/ingest"
)

type fakeRecorder struct {
	readings []ingest.Reading
	counts   map[string]int
}

func (f *fakeRecorder) Record(r ingest.Reading) { f.readings = append(f.readings, r) }
func (f *fakeRecorder) Count(protocol, result string) {
	if f.counts == nil {
		f.counts = map[string]int{}
	}
	f.counts[protocol+"/"+result]++
}

func ptr(f float64) *float64 { return &f }

func TestParse(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  ingest.Reading
	}{
		{
			"full GMC-500+ upload",
			"AID=0230111&GID=0034021&CPM=15&ACPM=13.2&uSV=0.075",
			ingest.Reading{Protocol: "gmcmap", AccountID: "0230111", DeviceID: "0034021", CPM: 15, ACPM: ptr(13.2), SvPerH: ptr(0.075e-6)},
		},
		{
			"only required fields",
			"GID=0034021&CPM=0",
			ingest.Reading{Protocol: "gmcmap", DeviceID: "0034021"},
		},
		{
			"case-insensitive names, unknown params ignored",
			"aid=1&gid=2&cpm=15&usv=0.1&tmp=22.5&foo=bar",
			ingest.Reading{Protocol: "gmcmap", AccountID: "1", DeviceID: "2", CPM: 15, SvPerH: ptr(0.1e-6)},
		},
		{
			"empty optional values treated as absent",
			"GID=2&CPM=15&ACPM=&uSV=",
			ingest.Reading{Protocol: "gmcmap", DeviceID: "2", CPM: 15},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.query)
			if err != nil {
				t.Fatal(err)
			}
			if got.Protocol != tt.want.Protocol || got.DeviceID != tt.want.DeviceID ||
				got.AccountID != tt.want.AccountID || got.CPM != tt.want.CPM ||
				!eqPtr(got.ACPM, tt.want.ACPM) || !eqPtr(got.SvPerH, tt.want.SvPerH) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func eqPtr(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return math.Abs(*a-*b) <= 1e-12*math.Max(1, math.Abs(*b))
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name, query, wantErr string
	}{
		{"missing GID", "CPM=15", "gid is required"},
		{"missing CPM", "GID=2", "cpm is required"},
		{"bad GID", "GID=../etc&CPM=1", "gid: invalid ID"},
		{"GID too long", "GID=" + strings.Repeat("1", 33) + "&CPM=1", "gid: invalid ID"},
		{"bad AID", "AID=a%20b&GID=2&CPM=1", "aid: invalid ID"},
		{"non-numeric CPM", "GID=2&CPM=lots", "cpm: not a number"},
		{"NaN", "GID=2&CPM=NaN", "cpm: not a number"},
		{"Inf", "GID=2&CPM=Inf", "cpm: not a number"},
		{"negative", "GID=2&CPM=-1", "out of range"},
		{"CPM too high", "GID=2&CPM=1e9", "out of range"},
		{"uSV too high", "GID=2&CPM=1&uSV=1e6", "usv: out of range"},
		{"duplicate", "GID=2&CPM=1&CPM=2", "more than once"},
		{"duplicate across case", "GID=2&CPM=1&cpm=2", "more than once"},
		{"malformed escape", "GID=2&CPM=%zz", "malformed query"},
		{"reports every error", "GID=&CPM=x", "gid is required\ncpm: not a number"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.query)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func newTestHandler() (*fakeRecorder, *http.ServeMux) {
	rec := &fakeRecorder{}
	h := New(config.Default().GMCMap, rec)
	h.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	mux := http.NewServeMux()
	h.Register(mux)
	return rec, mux
}

func TestHandlerAccepts(t *testing.T) {
	rec, mux := newTestHandler()
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/log2.asp?AID=1&GID=2&CPM=15&ACPM=13.2&uSV=0.075", nil))

	if w.Code != http.StatusOK || w.Body.String() != "OK.ERR0" {
		t.Errorf("got %d %q, want 200 OK.ERR0", w.Code, w.Body.String())
	}
	if len(rec.readings) != 1 || rec.readings[0].DeviceID != "2" || rec.readings[0].Time.Unix() != 1_700_000_000 {
		t.Errorf("readings: %+v", rec.readings)
	}
	if rec.counts["gmcmap/accepted"] != 1 {
		t.Errorf("counts: %v", rec.counts)
	}
}

func TestHandlerRejects(t *testing.T) {
	tests := []struct {
		name, method, target string
		wantCode             int
		wantCounted          bool
	}{
		{"bad request", http.MethodGet, "/log2.asp?GID=2", http.StatusBadRequest, true},
		{"query too long", http.MethodGet, "/log2.asp?GID=2&CPM=1&x=" + strings.Repeat("a", MaxQueryLen), http.StatusRequestURITooLong, true},
		{"wrong method", http.MethodPost, "/log2.asp?GID=2&CPM=1", http.StatusMethodNotAllowed, false},
		{"wrong path", http.MethodGet, "/other?GID=2&CPM=1", http.StatusNotFound, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, mux := newTestHandler()
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), tt.method, tt.target, nil))
			if w.Code != tt.wantCode {
				t.Errorf("got %d, want %d", w.Code, tt.wantCode)
			}
			if len(rec.readings) != 0 {
				t.Errorf("unexpected readings: %+v", rec.readings)
			}
			if got := rec.counts["gmcmap/bad_request"] == 1; got != tt.wantCounted {
				t.Errorf("bad_request counted = %v, want %v", got, tt.wantCounted)
			}
		})
	}
}
