package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/dickeyf/radwatch-exporter/internal/ingest"
)

var deviceMetrics = []string{
	"radwatch_cpm", "radwatch_acpm", "radwatch_dose_rate_sieverts_per_hour",
	"radwatch_last_report_timestamp_seconds", "radwatch_reports_total",
}

func ptr(f float64) *float64 { return &f }

func reading(t time.Time) ingest.Reading {
	return ingest.Reading{
		Protocol: "gmcmap", DeviceID: "0034021", AccountID: "0230111",
		CPM: 15, ACPM: ptr(13.2), SvPerH: ptr(0.075e-6), Time: t,
	}
}

func TestRecord(t *testing.T) {
	m := New(map[string]string{"0034021": "basement"}, 5*time.Minute)
	m.Record(reading(time.Unix(1_700_000_000, 0)))

	want := `
# HELP radwatch_acpm Last averaged counts per minute reported by the device.
# TYPE radwatch_acpm gauge
radwatch_acpm{device="basement",device_id="0034021",protocol="gmcmap"} 13.2
# HELP radwatch_cpm Last counts per minute reported by the device.
# TYPE radwatch_cpm gauge
radwatch_cpm{device="basement",device_id="0034021",protocol="gmcmap"} 15
# HELP radwatch_dose_rate_sieverts_per_hour Last dose rate computed by the device, in Sv/h.
# TYPE radwatch_dose_rate_sieverts_per_hour gauge
radwatch_dose_rate_sieverts_per_hour{device="basement",device_id="0034021",protocol="gmcmap"} 7.5e-08
# HELP radwatch_last_report_timestamp_seconds Unix time of the last accepted reading.
# TYPE radwatch_last_report_timestamp_seconds gauge
radwatch_last_report_timestamp_seconds{device="basement",device_id="0034021",protocol="gmcmap"} 1.7e+09
# HELP radwatch_reports_total Accepted readings.
# TYPE radwatch_reports_total counter
radwatch_reports_total{device="basement",device_id="0034021",protocol="gmcmap"} 1
`
	m.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	if err := testutil.GatherAndCompare(m.reg, strings.NewReader(want), deviceMetrics...); err != nil {
		t.Error(err)
	}
}

func TestDeviceFallsBackToID(t *testing.T) {
	m := New(nil, 0)
	m.Record(reading(time.Now()))
	if got := testutil.ToFloat64(m.cpm.WithLabelValues("gmcmap", "0034021", "0034021")); got != 15 {
		t.Errorf("cpm with device=ID: got %v", got)
	}
}

func TestMissingFieldDropsSeries(t *testing.T) {
	m := New(nil, 0)
	m.Record(reading(time.Now()))
	r := reading(time.Now())
	r.ACPM = nil
	m.Record(r)
	if n := testutil.CollectAndCount(m.acpm); n != 0 {
		t.Errorf("acpm series: got %d, want 0", n)
	}
	if n := testutil.CollectAndCount(m.doseRate); n != 1 {
		t.Errorf("dose rate series: got %d, want 1", n)
	}
}

func TestStaleValuesExpire(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	m := New(nil, 5*time.Minute)
	m.Record(reading(start))

	count := func(at time.Time) map[string]int {
		m.now = func() time.Time { return at }
		mfs, err := m.reg.Gather()
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]int{}
		for _, mf := range mfs {
			got[mf.GetName()] = len(mf.GetMetric())
		}
		return got
	}

	if got := count(start.Add(4 * time.Minute)); got["radwatch_cpm"] != 1 {
		t.Errorf("before stale_after: %v", got)
	}
	got := count(start.Add(6 * time.Minute))
	for _, name := range []string{"radwatch_cpm", "radwatch_acpm", "radwatch_dose_rate_sieverts_per_hour"} {
		if got[name] != 0 {
			t.Errorf("%s should have expired: %v", name, got)
		}
	}
	for _, name := range []string{"radwatch_last_report_timestamp_seconds", "radwatch_reports_total"} {
		if got[name] != 1 {
			t.Errorf("%s should be kept: %v", name, got)
		}
	}

	m.Record(reading(start.Add(7 * time.Minute)))
	if got := count(start.Add(7 * time.Minute)); got["radwatch_cpm"] != 1 {
		t.Errorf("a new reading should bring values back: %v", got)
	}
}

func TestStaleAfterZeroKeepsValues(t *testing.T) {
	m := New(nil, 0)
	m.Record(reading(time.Unix(0, 0)))
	if n := testutil.CollectAndCount(m, "radwatch_cpm"); n != 1 {
		t.Errorf("got %d cpm series, want 1", n)
	}
}

func TestCount(t *testing.T) {
	m := New(nil, 0)
	m.InitProtocol("gmcmap")
	m.Count("gmcmap", ingest.ResultBadRequest)
	want := `
# HELP radwatch_ingest_requests_total Ingest requests by protocol and result.
# TYPE radwatch_ingest_requests_total counter
radwatch_ingest_requests_total{protocol="gmcmap",result="accepted"} 0
radwatch_ingest_requests_total{protocol="gmcmap",result="bad_request"} 1
radwatch_ingest_requests_total{protocol="gmcmap",result="rejected_allowlist"} 0
`
	if err := testutil.GatherAndCompare(m.reg, strings.NewReader(want), "radwatch_ingest_requests_total"); err != nil {
		t.Error(err)
	}
}
