// Package ingest holds the protocol-neutral types shared by the upload
// protocol handlers (one subpackage per protocol, e.g. gmcmap).
package ingest

import "time"

// Request outcomes, used as the result label on the ingest request counter.
const (
	ResultAccepted          = "accepted"
	ResultBadRequest        = "bad_request"
	ResultRejectedAllowlist = "rejected_allowlist"
)

// Reading is one accepted measurement from a device.
type Reading struct {
	Protocol  string // upload protocol, e.g. "gmcmap"
	DeviceID  string // counter ID within the protocol (GMCmap GID)
	AccountID string // account ID if the protocol has one (GMCmap AID); not a metric label
	CPM       float64
	ACPM      *float64 // nil when the device did not send it
	SvPerH    *float64 // dose rate in Sv/h (base unit); nil when not sent
	Time      time.Time
}

// Recorder receives the outcome of every ingest request.
type Recorder interface {
	// Record stores an accepted reading.
	Record(r Reading)
	// Count counts a request outcome (one of the Result* constants).
	Count(protocol, result string)
}
