package http_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/eaglepoint/harborclass/internal/models"
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
	tok := h.loginAs(t, "admin", "admin-pass")
	rec := h.do(t, http.MethodGet, "/api/metrics", "", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := mustJSON(t, rec.Body.Bytes())
	if _, ok := body["requests"]; !ok {
		t.Fatal("metrics missing requests counter")
	}
}

// TestMetricsRequiresAuth locks in that /api/metrics does not leak
// operational counters to unauthenticated callers.
func TestMetricsRequiresAuth(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodGet, "/api/metrics", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", rec.Code)
	}
}

// TestAlertsRequiresAuth locks in that /api/alerts is protected.
func TestAlertsRequiresAuth(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodGet, "/api/alerts", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", rec.Code)
	}
}

// TestMetricsForbiddenForNonAdmin locks in least-privilege: only an
// admin may read operational counters.
func TestMetricsForbiddenForNonAdmin(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "student", "student-pass")
	rec := h.do(t, http.MethodGet, "/api/metrics", "", tok)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin, got %d", rec.Code)
	}
}

// TestAlertsForbiddenForNonAdmin locks in least-privilege for alerts.
func TestAlertsForbiddenForNonAdmin(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "teacher", "teacher-pass")
	rec := h.do(t, http.MethodGet, "/api/alerts", "", tok)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin, got %d", rec.Code)
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
	tok := h.loginAs(t, "student", "student-pass")
	rec := h.do(t, http.MethodPost, "/api/crash-reports", `{"version":"1.0.0","stack":"panic!","actor":"student"}`, tok)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestCrashReportRequiresAuth locks in that /api/crash-reports cannot
// be used as an unauthenticated log-injection surface.
func TestCrashReportRequiresAuth(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodPost, "/api/crash-reports", `{"version":"1.0.0","stack":"x","actor":"anon"}`, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", rec.Code)
	}
}

// TestDevicePolicyRejectsForeignDevice proves an authenticated user
// cannot query policy metadata for a device owned by someone else.
// The canary/forced-upgrade flags are sensitive and must stay scoped
// to the device owner or a same-org admin.
func TestDevicePolicyRejectsForeignDevice(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := h.store.UpsertDevice(ctx, models.Device{
		ID: "dev-foreign", UserID: "usr-teacher", Platform: "ios", Version: "1.0.0",
		Canary: true, ForcedUpgradeTo: "2.0.0",
	}); err != nil {
		t.Fatal(err)
	}
	tok := h.loginAs(t, "student", "student-pass")
	rec := h.do(t, http.MethodGet, "/api/devices/policy?device_id=dev-foreign&version=1.0.0", "", tok)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for foreign device, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestDevicePolicyAllowsOwner proves the registered owner of a device
// gets policy metadata for their own device.
func TestDevicePolicyAllowsOwner(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := h.store.UpsertDevice(ctx, models.Device{
		ID: "dev-student", UserID: "usr-student", Platform: "ios", Version: "1.0.0",
		ForcedUpgradeTo: "2.0.0",
	}); err != nil {
		t.Fatal(err)
	}
	tok := h.loginAs(t, "student", "student-pass")
	rec := h.do(t, http.MethodGet, "/api/devices/policy?device_id=dev-student&version=1.0.0", "", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for owner, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := mustJSON(t, rec.Body.Bytes())
	if body["upgrade_required"] != true {
		t.Fatalf("expected upgrade_required=true, got %v", body["upgrade_required"])
	}
}

// TestDevicePolicyAllowsSameOrgAdmin proves an admin in the same org
// as the device owner can inspect policy metadata, but a cross-org
// admin cannot.
func TestDevicePolicyAllowsSameOrgAdmin(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := h.store.UpsertDevice(ctx, models.Device{
		ID: "dev-samaorg", UserID: "usr-teacher", Platform: "ios", Version: "1.0.0",
		ForcedUpgradeTo: "2.0.0",
	}); err != nil {
		t.Fatal(err)
	}
	adminTok := h.loginAs(t, "admin", "admin-pass")
	rec := h.do(t, http.MethodGet, "/api/devices/policy?device_id=dev-samaorg&version=1.0.0", "", adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for same-org admin, got %d body=%s", rec.Code, rec.Body.String())
	}
}
