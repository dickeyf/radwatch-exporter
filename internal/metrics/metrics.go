// Package metrics holds the Prometheus registry and implements
// ingest.Recorder (data model and ADR-007 in docs/plan/03-architecture.md).
package metrics

import (
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/dickeyf/radwatch-exporter/internal/ingest"
)

const namespace = "radwatch"

var deviceLabels = []string{"protocol", "device_id", "device"}

// Metrics records readings and serves them. Value gauges of a device that
// has not reported for staleAfter are dropped (ADR-008); its timestamp and
// counter are kept so "device silent" alerts keep working.
type Metrics struct {
	reg         *prometheus.Registry
	deviceNames map[string]string
	staleAfter  time.Duration
	now         func() time.Time

	cpm, acpm, doseRate, lastReport *prometheus.GaugeVec
	reports, requests, forwards     *prometheus.CounterVec

	mu       sync.Mutex
	lastSeen map[[3]string]time.Time // device label values -> last accepted reading
}

// New builds the registry with Go and process collectors. deviceNames maps
// device IDs to friendly names; staleAfter 0 keeps values forever.
func New(deviceNames map[string]string, staleAfter time.Duration) *Metrics {
	gauge := func(name, help string) *prometheus.GaugeVec {
		return prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: namespace, Name: name, Help: help}, deviceLabels)
	}
	m := &Metrics{
		reg:         prometheus.NewRegistry(),
		deviceNames: deviceNames,
		staleAfter:  staleAfter,
		now:         time.Now,
		cpm:         gauge("cpm", "Last counts per minute reported by the device."),
		acpm:        gauge("acpm", "Last averaged counts per minute reported by the device."),
		doseRate:    gauge("dose_rate_sieverts_per_hour", "Last dose rate computed by the device, in Sv/h."),
		lastReport:  gauge("last_report_timestamp_seconds", "Unix time of the last accepted reading."),
		reports: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "reports_total", Help: "Accepted readings.",
		}, deviceLabels),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "ingest_requests_total", Help: "Ingest requests by protocol and result.",
		}, []string{"protocol", "result"}),
		forwards: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "forward_requests_total", Help: "Readings forwarded to public maps, by target and result.",
		}, []string{"target", "result"}),
		lastSeen: map[[3]string]time.Time{},
	}
	m.reg.MustRegister(
		m,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return m
}

// InitProtocol creates the request counters of a protocol at zero, so rate()
// works from the first rejected or accepted request.
func (m *Metrics) InitProtocol(protocol string) {
	for _, result := range []string{ingest.ResultAccepted, ingest.ResultBadRequest, ingest.ResultRejectedAllowlist} {
		m.requests.WithLabelValues(protocol, result)
	}
}

// InitForward creates the forward counters of a target at zero.
func (m *Metrics) InitForward(target string, results ...string) {
	for _, result := range results {
		m.forwards.WithLabelValues(target, result)
	}
}

// CountForward counts one forward attempt (forward.Counter).
func (m *Metrics) CountForward(target, result string) {
	m.forwards.WithLabelValues(target, result).Inc()
}

// Record implements ingest.Recorder.
func (m *Metrics) Record(r ingest.Reading) {
	name := m.deviceNames[r.DeviceID]
	if name == "" {
		name = r.DeviceID
	}
	labels := [3]string{r.Protocol, r.DeviceID, name}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.cpm.WithLabelValues(labels[:]...).Set(r.CPM)
	setOrDelete(m.acpm, labels, r.ACPM)
	setOrDelete(m.doseRate, labels, r.SvPerH)
	m.lastReport.WithLabelValues(labels[:]...).Set(float64(r.Time.UnixNano()) / 1e9)
	m.reports.WithLabelValues(labels[:]...).Inc()
	m.lastSeen[labels] = r.Time
}

// setOrDelete drops the series when the device stops sending that field, so
// an old value is not shown as current.
func setOrDelete(g *prometheus.GaugeVec, labels [3]string, v *float64) {
	if v == nil {
		g.DeleteLabelValues(labels[:]...)
		return
	}
	g.WithLabelValues(labels[:]...).Set(*v)
}

// Count implements ingest.Recorder.
func (m *Metrics) Count(protocol, result string) {
	m.requests.WithLabelValues(protocol, result).Inc()
}

// Describe implements prometheus.Collector.
func (m *Metrics) Describe(ch chan<- *prometheus.Desc) {
	for _, c := range m.collectors() {
		c.Describe(ch)
	}
}

// Collect implements prometheus.Collector, expiring stale values first.
func (m *Metrics) Collect(ch chan<- prometheus.Metric) {
	m.mu.Lock()
	m.expire()
	m.mu.Unlock()
	for _, c := range m.collectors() {
		c.Collect(ch)
	}
}

func (m *Metrics) collectors() []prometheus.Collector {
	return []prometheus.Collector{m.cpm, m.acpm, m.doseRate, m.lastReport, m.reports, m.requests, m.forwards}
}

// expire must be called with mu held.
func (m *Metrics) expire() {
	if m.staleAfter <= 0 {
		return
	}
	cutoff := m.now().Add(-m.staleAfter)
	for labels, seen := range m.lastSeen {
		if seen.Before(cutoff) {
			m.cpm.DeleteLabelValues(labels[:]...)
			m.acpm.DeleteLabelValues(labels[:]...)
			m.doseRate.DeleteLabelValues(labels[:]...)
			delete(m.lastSeen, labels)
		}
	}
}

// Handler serves the registry in the Prometheus exposition format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{Registry: m.reg})
}
