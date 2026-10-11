package forward

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/dickeyf/radwatch-exporter/internal/config"
	"github.com/dickeyf/radwatch-exporter/internal/ingest"
	"github.com/dickeyf/radwatch-exporter/internal/ingest/gmcmap"
)

// Targets returns the forwarding targets enabled in cfg (a non-empty forward_url).
func Targets(cfg config.Config) []Target {
	var out []Target
	if cfg.GMCMap.ForwardURL != "" {
		out = append(out, GMCMap{URL: cfg.GMCMap.ForwardURL})
	}
	if cfg.Radmon.ForwardURL != "" {
		out = append(out, Radmon{Config: cfg.Radmon})
	}
	return out
}

// GMCMap re-sends GMCmap readings in the GMCmap upload format, with the
// device's own AID and GID, e.g. to http://www.gmcmap.com/log2.asp.
type GMCMap struct {
	URL string
}

// Name implements Target.
func (GMCMap) Name() string { return "gmcmap" }

// Request implements Target. Readings from other protocols are skipped: they
// have no GMCmap IDs.
func (g GMCMap) Request(ctx context.Context, r ingest.Reading) (*http.Request, error) {
	if r.Protocol != gmcmap.Protocol {
		return nil, nil
	}
	q := url.Values{}
	if r.AccountID != "" {
		q.Set("AID", r.AccountID)
	}
	q.Set("GID", r.DeviceID)
	q.Set("CPM", num(r.CPM))
	if r.ACPM != nil {
		q.Set("ACPM", num(*r.ACPM))
	}
	if r.SvPerH != nil {
		q.Set("uSV", num(*r.SvPerH*1e6))
	}
	return get(ctx, g.URL, q)
}

// Check implements Target. gmcmap.com answers 200 with "OK.ERR0" on success
// and an ERRn code otherwise (e.g. unknown account or counter ID).
func (GMCMap) Check(status int, body string) error {
	if status < 200 || status > 299 {
		return errors.New("unexpected status")
	}
	if strings.Contains(body, "ERR") && !strings.Contains(body, "ERR0") {
		return errors.New("upload refused")
	}
	return nil
}

// Radmon submits CPM readings to radmon.org's submit API:
// GET <url>?function=submit&user=..&password=..&value=<cpm>&unit=CPM.
type Radmon struct {
	Config config.Radmon
}

// Name implements Target.
func (Radmon) Name() string { return "radmon" }

// Request implements Target. A radmon account is one station, so with
// device_id set only that device is forwarded.
func (t Radmon) Request(ctx context.Context, r ingest.Reading) (*http.Request, error) {
	if t.Config.DeviceID != "" && r.DeviceID != t.Config.DeviceID {
		return nil, nil
	}
	q := url.Values{}
	q.Set("function", "submit")
	q.Set("user", t.Config.User)
	q.Set("password", t.Config.Password)
	q.Set("value", num(r.CPM))
	q.Set("unit", "CPM")
	return get(ctx, t.Config.ForwardURL, q)
}

// Check implements Target. radmon.org answers 200 even for a bad login, with
// the reason in the body.
func (Radmon) Check(status int, body string) error {
	if status < 200 || status > 299 {
		return errors.New("unexpected status")
	}
	lower := strings.ToLower(body)
	for _, bad := range []string{"incorrect", "error", "invalid", "denied"} {
		if strings.Contains(lower, bad) {
			return errors.New("upload refused")
		}
	}
	return nil
}

func get(ctx context.Context, base string, q url.Values) (*http.Request, error) {
	u, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	if u.RawQuery != "" {
		u.RawQuery += "&" + q.Encode()
	} else {
		u.RawQuery = q.Encode()
	}
	return http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
}

// num formats a value like the device does: shortest form, no exponent for
// everyday values, float noise from unit conversion rounded away.
func num(v float64) string {
	s := strconv.FormatFloat(v, 'g', 10, 64)
	if strings.ContainsAny(s, "e") {
		s = strconv.FormatFloat(v, 'f', -1, 64)
	}
	return s
}
