package http_test

import (
	"net/http"
	"testing"
)

// health_handler_integration_test.go hits operational endpoints.

func TestHealthEndpoint(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodGet, "/api/health", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := mustJSON(t, rec.Body.Bytes())
	if body["status"] != "ok" {
		t.Fatalf("expected ok, got %v", body["status"])
	}
}

func TestMetricsReachable(t *testing.T) {
	h := newHarness(t)
	// warm up the counter by issuing a few requests first.
	_ = h.do(t, http.MethodGet, "/api/health", "", "")
	rec := h.do(t, http.MethodGet, "/api/metrics", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := mustJSON(t, rec.Body.Bytes())
	if _, ok := body["requests"]; !ok {
		t.Fatal("metrics missing requests counter")
	}
}

func TestLoginHome(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodGet, "/", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") == "" {
		t.Fatal("expected a content-type on home page")
	}
}

func TestCrashReportAccepted(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodPost, "/api/crash-reports", `{"version":"1.0.0","stack":"panic!","actor":"student"}`, "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}
}
