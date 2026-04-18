package http_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/eaglepoint/harborclass/internal/auth"
	"github.com/eaglepoint/harborclass/internal/models"
)

// notify_handler_integration_test.go verifies the /api/notifications
// endpoint group enforces rate limits, unsubscribe, template lookup,
// and cross-tenant recipient protection.

func TestSendNotificationHappyPath(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "teacher", "teacher-pass")
	body := `{"order_id":"ord-a","user_id":"usr-student","category":"booking","template_id":"booking.reminder"}`
	rec := h.do(t, http.MethodPost, "/api/notifications/send", body, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	got := mustJSON(t, rec.Body.Bytes())
	if got["success"] != true {
		t.Fatalf("expected success=true, got %v", got["success"])
	}
}

func TestSendNotificationMissingTemplate(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "teacher", "teacher-pass")
	body := `{"order_id":"ord-a","user_id":"usr-student","category":"booking","template_id":"nope"}`
	rec := h.do(t, http.MethodPost, "/api/notifications/send", body, tok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSendNotificationRateLimit(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "teacher", "teacher-pass")
	body := `{"order_id":"ord-rl","user_id":"usr-student","category":"booking","template_id":"booking.reminder"}`
	for i := 0; i < 3; i++ {
		rec := h.do(t, http.MethodPost, "/api/notifications/send", body, tok)
		if rec.Code != http.StatusOK {
			t.Fatalf("send %d failed: %d %s", i, rec.Code, rec.Body.String())
		}
	}
	rec := h.do(t, http.MethodPost, "/api/notifications/send", body, tok)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after 3/day, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSendNotificationUnsubscribed(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "teacher", "teacher-pass")
	// Student unsubscribes via one-click endpoint first (signed token).
	sig := h.signUnsubscribe("usr-student", "booking")
	un := h.do(t, http.MethodGet, "/api/my/subscriptions/unsubscribe?user=usr-student&category=booking&token="+sig, "", "")
	if un.Code != http.StatusOK {
		t.Fatalf("unsubscribe setup failed: %d %s", un.Code, un.Body.String())
	}
	body := `{"order_id":"ord-u","user_id":"usr-student","category":"booking","template_id":"booking.reminder"}`
	rec := h.do(t, http.MethodPost, "/api/notifications/send", body, tok)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 unsubscribed, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestListTemplatesReturnsSeeded(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "teacher", "teacher-pass")
	rec := h.do(t, http.MethodGet, "/api/notifications/templates", "", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	body := mustJSON(t, rec.Body.Bytes())
	tpls, _ := body["templates"].([]any)
	if len(tpls) < 3 {
		t.Fatalf("expected at least 3 seeded templates, got %d", len(tpls))
	}
}

func TestSendNotificationRequiresSenderRole(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "student", "student-pass")
	body := `{"order_id":"ord-x","user_id":"usr-student","category":"booking","template_id":"booking.reminder"}`
	rec := h.do(t, http.MethodPost, "/api/notifications/send", body, tok)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for student sender, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestSendNotificationCrossOrgRejected proves that a teacher cannot
// target a recipient from another organisation, even if they know the
// recipient's user_id.
func TestSendNotificationCrossOrgRejected(t *testing.T) {
	h := newHarness(t)
	// Insert a user in a different organisation directly via the store.
	foreign := models.User{
		ID:           "usr-foreign",
		Username:     "foreign",
		Role:         models.RoleStudent,
		OrgID:        "org-other",
		DisplayName:  "Foreign Student",
		PasswordHash: auth.HashPassword("nope", "harborclass-demo"),
		CreatedAt:    time.Now(),
	}
	if err := h.store.CreateUser(context.Background(), foreign); err != nil {
		t.Fatalf("seed foreign: %v", err)
	}
	tok := h.loginAs(t, "teacher", "teacher-pass")
	body := `{"order_id":"ord-x","user_id":"usr-foreign","category":"booking","template_id":"booking.reminder"}`
	rec := h.do(t, http.MethodPost, "/api/notifications/send", body, tok)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 cross-org, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestSendNotificationEmitsSignedUnsubscribeURL asserts every successful
// send includes a signed unsubscribe URL the recipient can click.
func TestSendNotificationEmitsSignedUnsubscribeURL(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "teacher", "teacher-pass")
	body := `{"order_id":"ord-sig","user_id":"usr-student","category":"booking","template_id":"booking.reminder"}`
	rec := h.do(t, http.MethodPost, "/api/notifications/send", body, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	got := mustJSON(t, rec.Body.Bytes())
	url, _ := got["unsubscribe_url"].(string)
	if url == "" {
		t.Fatalf("expected unsubscribe_url in response, got %s", rec.Body.String())
	}
	if !strings.Contains(url, "token=") {
		t.Fatalf("unsubscribe_url missing token query: %s", url)
	}
}
