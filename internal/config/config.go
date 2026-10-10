// Package config loads and validates the service configuration.
//
// Values are layered: built-in defaults, then an optional YAML file (path in
// RADWATCH_CONFIG_FILE), then RADWATCH_* environment variables. A variable
// that is set, even to the empty string, overrides the file. Settings for a
// specific upload protocol live in their own section (gmcmap for now).
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// EnvConfigFile names the variable holding the optional YAML config path.
const EnvConfigFile = "RADWATCH_CONFIG_FILE"

// Config is the full service configuration.
type Config struct {
	ListenAddr     string            `yaml:"listen_addr"`
	MetricsAddr    string            `yaml:"metrics_addr"`
	LogLevel       string            `yaml:"log_level"`
	StaleAfter     time.Duration     `yaml:"stale_after"`
	AllowedCIDRs   Prefixes          `yaml:"allowed_cidrs"`
	TrustedProxies Prefixes          `yaml:"trusted_proxies"`
	DeviceNames    map[string]string `yaml:"device_names"`
	GMCMap         GMCMap            `yaml:"gmcmap"`
}

// GMCMap configures the GQ GMCmap upload protocol (GET log2.asp?AID=..&GID=..).
type GMCMap struct {
	Path           string   `yaml:"path"`
	AllowedGIDs    []string `yaml:"allowed_gids"`
	AllowedAIDs    []string `yaml:"allowed_aids"`
	ResponseStatus int      `yaml:"response_status"`
	ResponseBody   string   `yaml:"response_body"`
	ForwardURL     string   `yaml:"forward_url"`
}

// Default returns the configuration used when nothing is set.
func Default() Config {
	return Config{
		ListenAddr:  ":8080",
		MetricsAddr: ":9090",
		LogLevel:    "info",
		StaleAfter:  5 * time.Minute,
		GMCMap: GMCMap{
			Path:           "/log2.asp",
			ResponseStatus: 200,
			ResponseBody:   "OK.ERR0",
		},
	}
}

// Load builds the configuration from defaults, the optional file and the
// environment, then validates it. lookup is usually os.LookupEnv.
func Load(lookup func(string) (string, bool)) (Config, error) {
	cfg := Default()
	if path, ok := lookup(EnvConfigFile); ok && path != "" {
		if err := loadFile(path, &cfg); err != nil {
			return Config{}, err
		}
	}
	if err := applyEnv(lookup, &cfg); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func loadFile(path string, cfg *Config) error {
	data, err := os.ReadFile(path) //nolint:gosec // path comes from the operator
	if err != nil {
		return fmt.Errorf("config file: %w", err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("config file %s: %w", path, err)
	}
	return nil
}

func applyEnv(lookup func(string) (string, bool), cfg *Config) error {
	var errs []error
	str := func(key string, dst *string) {
		if v, ok := lookup(key); ok {
			*dst = strings.TrimSpace(v)
		}
	}
	list := func(key string, dst *[]string) {
		if v, ok := lookup(key); ok {
			*dst = splitList(v)
		}
	}
	prefixes := func(key string, dst *Prefixes) {
		if v, ok := lookup(key); ok {
			p, err := parsePrefixes(splitList(v))
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", key, err))
				return
			}
			*dst = p
		}
	}

	str("RADWATCH_LISTEN_ADDR", &cfg.ListenAddr)
	str("RADWATCH_METRICS_ADDR", &cfg.MetricsAddr)
	str("RADWATCH_LOG_LEVEL", &cfg.LogLevel)
	if v, ok := lookup("RADWATCH_STALE_AFTER"); ok {
		d, err := time.ParseDuration(strings.TrimSpace(v))
		if err != nil {
			errs = append(errs, fmt.Errorf("RADWATCH_STALE_AFTER: %w", err))
		} else {
			cfg.StaleAfter = d
		}
	}
	prefixes("RADWATCH_ALLOWED_CIDRS", &cfg.AllowedCIDRs)
	prefixes("RADWATCH_TRUSTED_PROXIES", &cfg.TrustedProxies)
	if v, ok := lookup("RADWATCH_DEVICE_NAMES"); ok {
		names, err := parsePairs(splitList(v))
		if err != nil {
			errs = append(errs, fmt.Errorf("RADWATCH_DEVICE_NAMES: %w", err))
		} else {
			cfg.DeviceNames = names
		}
	}

	str("RADWATCH_GMCMAP_PATH", &cfg.GMCMap.Path)
	list("RADWATCH_GMCMAP_ALLOWED_GIDS", &cfg.GMCMap.AllowedGIDs)
	list("RADWATCH_GMCMAP_ALLOWED_AIDS", &cfg.GMCMap.AllowedAIDs)
	if v, ok := lookup("RADWATCH_GMCMAP_RESPONSE_STATUS"); ok {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			errs = append(errs, fmt.Errorf("RADWATCH_GMCMAP_RESPONSE_STATUS: %w", err))
		} else {
			cfg.GMCMap.ResponseStatus = n
		}
	}
	if v, ok := lookup("RADWATCH_GMCMAP_RESPONSE_BODY"); ok {
		cfg.GMCMap.ResponseBody = v // kept verbatim: whitespace may matter to the device
	}
	str("RADWATCH_GMCMAP_FORWARD_URL", &cfg.GMCMap.ForwardURL)

	return errors.Join(errs...)
}

// Validate reports every problem in the configuration at once.
func (c Config) Validate() error {
	var errs []error
	addErr := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	for name, addr := range map[string]string{"listen_addr": c.ListenAddr, "metrics_addr": c.MetricsAddr} {
		if _, port, err := net.SplitHostPort(addr); err != nil {
			addErr("%s %q: %w", name, addr, err)
		} else if n, err := strconv.Atoi(port); err != nil || n < 0 || n > 65535 {
			addErr("%s %q: invalid port", name, addr)
		}
	}
	if c.ListenAddr == c.MetricsAddr {
		addErr("listen_addr and metrics_addr must differ (ADR-004)")
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		addErr("log_level %q: want debug, info, warn or error", c.LogLevel)
	}
	if c.StaleAfter < 0 {
		addErr("stale_after %s: must not be negative (0 disables expiry)", c.StaleAfter)
	}
	for id, name := range c.DeviceNames {
		if id == "" || name == "" {
			addErr("device_names: empty ID or name in %q=%q", id, name)
		}
	}

	g := c.GMCMap
	switch {
	case !strings.HasPrefix(g.Path, "/"):
		addErr("gmcmap.path %q: must start with /", g.Path)
	case g.Path == "/healthz" || g.Path == "/readyz":
		addErr("gmcmap.path %q: clashes with a health endpoint", g.Path)
	case strings.ContainsAny(g.Path, "?# "):
		addErr("gmcmap.path %q: must be a plain path", g.Path)
	}
	if g.ResponseStatus < 100 || g.ResponseStatus > 599 {
		addErr("gmcmap.response_status %d: not an HTTP status", g.ResponseStatus)
	}
	if g.ForwardURL != "" {
		u, err := url.Parse(g.ForwardURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			addErr("gmcmap.forward_url %q: want an absolute http(s) URL", g.ForwardURL)
		}
	}
	return errors.Join(errs...)
}

// Prefixes is a list of CIDRs; bare IPs are accepted as single-host prefixes.
type Prefixes []netip.Prefix

// UnmarshalYAML applies the same parsing as the environment variables.
func (p *Prefixes) UnmarshalYAML(node *yaml.Node) error {
	var items []string
	if err := node.Decode(&items); err != nil {
		return err
	}
	parsed, err := parsePrefixes(items)
	if err != nil {
		return err
	}
	*p = parsed
	return nil
}

// Contains reports whether addr is in any of the prefixes.
func (p Prefixes) Contains(addr netip.Addr) bool {
	addr = addr.Unmap()
	for _, prefix := range p {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func splitList(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// parsePrefixes accepts CIDRs and bare IPs (treated as /32 or /128).
func parsePrefixes(items []string) (Prefixes, error) {
	var out Prefixes
	for _, s := range items {
		if !strings.Contains(s, "/") {
			addr, err := netip.ParseAddr(s)
			if err != nil {
				return nil, err
			}
			out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
			continue
		}
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, err
		}
		out = append(out, p.Masked())
	}
	return out, nil
}

func parsePairs(items []string) (map[string]string, error) {
	out := make(map[string]string, len(items))
	for _, s := range items {
		k, v, ok := strings.Cut(s, "=")
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if !ok || k == "" || v == "" {
			return nil, fmt.Errorf("%q: want id=name", s)
		}
		out[k] = v
	}
	return out, nil
}
