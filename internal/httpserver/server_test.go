package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	cfgpkg "github.com/taoyao-code/iot-server/internal/config"
	appmetrics "github.com/taoyao-code/iot-server/internal/metrics"
)

func TestHealthzReadyzMetrics(t *testing.T) {
	cfg := cfgpkg.HTTPConfig{Addr: ":0", ReadTimeout: time.Second, WriteTimeout: time.Second}
	reg := appmetrics.NewRegistry()
	handler := appmetrics.Handler(reg)
	srv := New(cfg, "/metrics", handler, func() bool { return true })

	// healthz
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	srv.srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("/healthz code=%d", rr.Code)
	}

	// readyz ok
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	srv.srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("/readyz code=%d", rr.Code)
	}

	// metrics
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	srv.srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("/metrics code=%d", rr.Code)
	}
}

func TestReadyzNotReady(t *testing.T) {
	cfg := cfgpkg.HTTPConfig{Addr: ":0", ReadTimeout: time.Second, WriteTimeout: time.Second}
	reg := appmetrics.NewRegistry()
	handler := appmetrics.Handler(reg)
	srv := New(cfg, "/metrics", handler, func() bool { return false })

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	srv.srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("/readyz not-ready code=%d", rr.Code)
	}
}
