package http_test

import (
	"net/http"
	"testing"
)

// notify_handler_integration_test.go verifies the /api/notifications
// endpoint group enforces rate limits, unsubscribe, and template lookup.

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
	// Student unsubscribes via one-click endpoint first.
	un := h.do(t, http.MethodGet, "/api/my/subscriptions/unsubscribe?user=usr-student&category=booking", "", "")
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
