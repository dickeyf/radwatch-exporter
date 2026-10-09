// Package gmcmap implements the GQ GMCmap upload protocol: the counter sends
// GET <path>?AID=<account>&GID=<counter>&CPM=<n>[&ACPM=<n>][&uSV=<n>].
package gmcmap

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dickeyf/radwatch-exporter/internal/config"
	"github.com/dickeyf/radwatch-exporter/internal/ingest"
)

// Protocol is the value of the protocol label for readings from this handler.
const Protocol = "gmcmap"

// Limits on what a request may contain.
const (
	MaxQueryLen = 1024
	maxCPM      = 10_000_000 // far above any GM tube's saturation
	maxUSvPerH  = 100_000    // 100 mSv/h
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

// Handler serves the GMCmap upload endpoint.
type Handler struct {
	cfg config.GMCMap
	rec ingest.Recorder
	now func() time.Time
}

// New returns a handler that reports outcomes to rec.
func New(cfg config.GMCMap, rec ingest.Recorder) *Handler {
	return &Handler{cfg: cfg, rec: rec, now: time.Now}
}

// Register adds the handler to mux for GET (and so HEAD) on the configured path.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("GET "+h.cfg.Path, h)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if len(r.URL.RawQuery) > MaxQueryLen {
		h.reject(w, http.StatusRequestURITooLong, "query too long")
		return
	}
	reading, err := Parse(r.URL.RawQuery)
	if err != nil {
		h.reject(w, http.StatusBadRequest, err.Error())
		return
	}
	reading.Time = h.now()
	h.rec.Record(reading)
	h.rec.Count(Protocol, ingest.ResultAccepted)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(h.cfg.ResponseStatus)
	_, _ = w.Write([]byte(h.cfg.ResponseBody))
}

// reject answers a request that failed validation. Replying with an honest
// 4xx is a placeholder until AD-13 decides what the device should see.
func (h *Handler) reject(w http.ResponseWriter, status int, msg string) {
	h.rec.Count(Protocol, ingest.ResultBadRequest)
	http.Error(w, msg, status)
}

// Parse validates a GMCmap query string. Parameter names are matched case-
// insensitively, unknown parameters are ignored, and a known parameter given
// twice is an error. GID and CPM are required; AID, ACPM and uSV are optional.
func Parse(rawQuery string) (ingest.Reading, error) {
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return ingest.Reading{}, fmt.Errorf("malformed query: %w", err)
	}
	params := map[string]string{}
	for name, vs := range values {
		key := strings.ToLower(name)
		switch key {
		case "aid", "gid", "cpm", "acpm", "usv":
		default:
			continue
		}
		if _, dup := params[key]; dup || len(vs) != 1 {
			return ingest.Reading{}, fmt.Errorf("parameter %s given more than once", name)
		}
		params[key] = strings.TrimSpace(vs[0])
	}

	r := ingest.Reading{Protocol: Protocol}
	var errs []error
	if r.DeviceID, err = id(params, "gid", true); err != nil {
		errs = append(errs, err)
	}
	if r.AccountID, err = id(params, "aid", false); err != nil {
		errs = append(errs, err)
	}
	cpm, err := number(params, "cpm", maxCPM, true)
	if err != nil {
		errs = append(errs, err)
	} else {
		r.CPM = *cpm
	}
	if r.ACPM, err = number(params, "acpm", maxCPM, false); err != nil {
		errs = append(errs, err)
	}
	if r.USvPerH, err = number(params, "usv", maxUSvPerH, false); err != nil {
		errs = append(errs, err)
	}
	if err := errors.Join(errs...); err != nil {
		return ingest.Reading{}, err
	}
	return r, nil
}

func id(params map[string]string, key string, required bool) (string, error) {
	v, ok := params[key]
	if !ok || v == "" {
		if required {
			return "", fmt.Errorf("%s is required", key)
		}
		return "", nil
	}
	if !idPattern.MatchString(v) {
		return "", fmt.Errorf("%s: invalid ID", key)
	}
	return v, nil
}

func number(params map[string]string, key string, maxVal float64, required bool) (*float64, error) {
	v, ok := params[key]
	if !ok || v == "" {
		if required {
			return nil, fmt.Errorf("%s is required", key)
		}
		return nil, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return nil, fmt.Errorf("%s: not a number", key)
	}
	if f < 0 || f > maxVal {
		return nil, fmt.Errorf("%s: out of range [0, %g]", key, maxVal)
	}
	return &f, nil
}
