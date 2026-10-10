package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbes(t *testing.T) {
	var c Checker
	mux := http.NewServeMux()
	c.Register(mux)

	get := func(path string) int {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
		return w.Code
	}

	if got := get("/healthz"); got != http.StatusOK {
		t.Errorf("healthz: got %d", got)
	}
	if got := get("/readyz"); got != http.StatusServiceUnavailable {
		t.Errorf("readyz before SetReady: got %d", got)
	}
	c.SetReady(true)
	if got := get("/readyz"); got != http.StatusOK {
		t.Errorf("readyz when ready: got %d", got)
	}
	c.SetReady(false)
	if got := get("/readyz"); got != http.StatusServiceUnavailable {
		t.Errorf("readyz after shutdown starts: got %d", got)
	}
}
