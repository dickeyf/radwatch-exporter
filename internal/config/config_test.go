package config

import (
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg, Default()) {
		t.Errorf("got %+v, want defaults %+v", cfg, Default())
	}
}

func TestLoadEnv(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"RADWATCH_LISTEN_ADDR":            "0.0.0.0:80",
		"RADWATCH_LOG_LEVEL":              "debug",
		"RADWATCH_STALE_AFTER":            "90s",
		"RADWATCH_DEBUG_REQUESTS":         "true",
		"RADWATCH_ALLOWED_CIDRS":          "192.168.1.50, 10.1.2.3/8",
		"RADWATCH_TRUSTED_PROXIES":        "",
		"RADWATCH_DEVICE_NAMES":           "0034021=basement",
		"RADWATCH_GMCMAP_PATH":            "/ingest/abc",
		"RADWATCH_GMCMAP_ALLOWED_GIDS":    "0034021,,0034022 ",
		"RADWATCH_GMCMAP_RESPONSE_STATUS": "204",
		"RADWATCH_GMCMAP_RESPONSE_BODY":   " OK ",
		"RADWATCH_GMCMAP_FORWARD_URL":     "http://www.gmcmap.com/log2.asp",
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := Default()
	want.ListenAddr = "0.0.0.0:80"
	want.LogLevel = "debug"
	want.StaleAfter = 90 * time.Second
	want.DebugRequests = true
	want.AllowedCIDRs = Prefixes{netip.MustParsePrefix("192.168.1.50/32"), netip.MustParsePrefix("10.0.0.0/8")}
	want.TrustedProxies = nil
	want.DeviceNames = map[string]string{"0034021": "basement"}
	want.GMCMap.Path = "/ingest/abc"
	want.GMCMap.AllowedGIDs = []string{"0034021", "0034022"}
	want.GMCMap.ResponseStatus = 204
	want.GMCMap.ResponseBody = " OK "
	want.GMCMap.ForwardURL = "http://www.gmcmap.com/log2.asp"
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("got  %+v\nwant %+v", cfg, want)
	}
}

func TestLoadFileThenEnvOverrides(t *testing.T) {
	path := writeFile(t, `
metrics_addr: ":9191"
stale_after: 10m
allowed_cidrs: ["192.168.1.50"]
device_names:
  "0034021": basement
gmcmap:
  path: /from-file
  allowed_gids: ["0034021"]
`)
	cfg, err := Load(env(map[string]string{
		EnvConfigFile:          path,
		"RADWATCH_GMCMAP_PATH": "/from-env",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MetricsAddr != ":9191" {
		t.Errorf("metrics_addr from file: got %q", cfg.MetricsAddr)
	}
	if cfg.StaleAfter != 10*time.Minute {
		t.Errorf("stale_after from file: got %s", cfg.StaleAfter)
	}
	if cfg.GMCMap.Path != "/from-env" {
		t.Errorf("env should override file: got %q", cfg.GMCMap.Path)
	}
	if !cfg.AllowedCIDRs.Contains(netip.MustParseAddr("192.168.1.50")) {
		t.Errorf("allowed_cidrs from file: got %v", cfg.AllowedCIDRs)
	}
	if cfg.DeviceNames["0034021"] != "basement" || len(cfg.GMCMap.AllowedGIDs) != 1 {
		t.Errorf("file values lost: %+v", cfg)
	}
	if cfg.ListenAddr != ":8080" {
		t.Errorf("default should survive file: got %q", cfg.ListenAddr)
	}
}

func TestLoadEmptyFile(t *testing.T) {
	if _, err := Load(env(map[string]string{EnvConfigFile: writeFile(t, "")})); err != nil {
		t.Fatal(err)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		file    string
		wantErr string
	}{
		{"missing file", map[string]string{EnvConfigFile: "/does/not/exist.yaml"}, "", "config file"},
		{"unknown file key", nil, "listen_adr: ':1'\n", "listen_adr"},
		{"bad file cidr", nil, "allowed_cidrs: [nope]\n", "nope"},
		{"bad listen addr", map[string]string{"RADWATCH_LISTEN_ADDR": "8080"}, "", "listen_addr"},
		{"bad port", map[string]string{"RADWATCH_METRICS_ADDR": ":99999"}, "", "invalid port"},
		{"same ports", map[string]string{"RADWATCH_METRICS_ADDR": ":8080"}, "", "must differ"},
		{"bad stale_after", map[string]string{"RADWATCH_STALE_AFTER": "5"}, "", "RADWATCH_STALE_AFTER"},
		{"negative stale_after", map[string]string{"RADWATCH_STALE_AFTER": "-1m"}, "", "stale_after"},
		{"bad debug flag", map[string]string{"RADWATCH_DEBUG_REQUESTS": "maybe"}, "", "RADWATCH_DEBUG_REQUESTS"},
		{"bad log level", map[string]string{"RADWATCH_LOG_LEVEL": "loud"}, "", "log_level"},
		{"bad cidr", map[string]string{"RADWATCH_ALLOWED_CIDRS": "1.2.3.4/40"}, "", "RADWATCH_ALLOWED_CIDRS"},
		{"bad device names", map[string]string{"RADWATCH_DEVICE_NAMES": "0034021"}, "", "id=name"},
		{"relative path", map[string]string{"RADWATCH_GMCMAP_PATH": "log2.asp"}, "", "start with /"},
		{"health path", map[string]string{"RADWATCH_GMCMAP_PATH": "/healthz"}, "", "health"},
		{"path with query", map[string]string{"RADWATCH_GMCMAP_PATH": "/a?b"}, "", "plain path"},
		{"bad status", map[string]string{"RADWATCH_GMCMAP_RESPONSE_STATUS": "42"}, "", "response_status"},
		{"non-numeric status", map[string]string{"RADWATCH_GMCMAP_RESPONSE_STATUS": "ok"}, "", "RESPONSE_STATUS"},
		{"bad forward url", map[string]string{"RADWATCH_GMCMAP_FORWARD_URL": "ftp://x"}, "", "forward_url"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := tt.env
			if tt.file != "" {
				e = map[string]string{EnvConfigFile: writeFile(t, tt.file)}
			}
			_, err := Load(env(e))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got error %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateReportsAllErrors(t *testing.T) {
	cfg := Default()
	cfg.LogLevel = "loud"
	cfg.GMCMap.ResponseStatus = 0
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "log_level") || !strings.Contains(err.Error(), "response_status") {
		t.Errorf("want both errors, got %v", err)
	}
}
