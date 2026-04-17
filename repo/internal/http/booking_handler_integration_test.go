package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/eaglepoint/harborclass/internal/models"
)

// booking_handler_integration_test.go drives the real Gin router end
// to end: real handlers, real state-machine, real audit chain, real
// in-memory persistence. No mocks.

func TestListSessionsReturnsSeededSession(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodGet, "/api/sessions", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := mustJSON(t, rec.Body.Bytes())
	sessions, ok := body["sessions"].([]any)
	if !ok || len(sessions) == 0 {
		t.Fatalf("expected at least one seeded session, body=%s", rec.Body.String())
	}
}

func TestCreateBookingHappyPath(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "student", "student-pass")
	rec := h.do(t, http.MethodPost, "/api/bookings", `{"session_id":"sess-demo-1"}`, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := mustJSON(t, rec.Body.Bytes())
	number, _ := body["number"].(string)
	if !strings.HasPrefix(number, "HC-") {
		t.Fatalf("expected HC- prefixed number, got %v", body["number"])
	}
	if body["state"] != string(models.StateConfirmed) {
		t.Fatalf("expected state confirmed, got %v", body["state"])
	}
}

func TestCreateBookingRequiresAuth(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodPost, "/api/bookings", `{"session_id":"sess-demo-1"}`, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestRescheduleEnforcesCap(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "student", "student-pass")
	create := h.do(t, http.MethodPost, "/api/bookings", `{"session_id":"sess-demo-1"}`, tok)
	if create.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", create.Code, create.Body.String())
	}
	body := mustJSON(t, create.Body.Bytes())
	id, _ := body["id"].(string)

	for i := 0; i < 2; i++ {
		rec := h.do(t, http.MethodPost, "/api/bookings/"+id+"/reschedule", `{"new_start":"2026-05-01T10:00:00Z"}`, tok)
		if rec.Code != http.StatusOK {
			t.Fatalf("attempt %d: %d %s", i, rec.Code, rec.Body.String())
		}
	}
	// third reschedule must be rejected.
	rec := h.do(t, http.MethodPost, "/api/bookings/"+id+"/reschedule", `{"new_start":"2026-06-01T10:00:00Z"}`, tok)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCancellationInside24hNeedsApproval(t *testing.T) {
	h := newHarness(t)
	// Inject a near-future session so the default 48h seeded one doesn't apply.
	ctx := context.Background()
	_ = h.store.CreateSession(ctx, models.Session{
		ID: "sess-soon", TeacherID: "usr-teacher", ClassID: "class-default",
		Title: "Tomorrow", StartsAt: time.Now().Add(6 * time.Hour), EndsAt: time.Now().Add(7 * time.Hour),
		Capacity: 5,
	})
	tok := h.loginAs(t, "student", "student-pass")
	create := h.do(t, http.MethodPost, "/api/bookings", `{"session_id":"sess-soon"}`, tok)
	body := mustJSON(t, create.Body.Bytes())
	id, _ := body["id"].(string)
	rec := h.do(t, http.MethodPost, "/api/bookings/"+id+"/cancel", "", tok)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 awaiting approval, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetBookingReturnsTimeline(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "student", "student-pass")
	create := h.do(t, http.MethodPost, "/api/bookings", `{"session_id":"sess-demo-1"}`, tok)
	body := mustJSON(t, create.Body.Bytes())
	id, _ := body["id"].(string)
	rec := h.do(t, http.MethodGet, "/api/bookings/"+id, "", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	got := mustJSON(t, rec.Body.Bytes())
	if got["timeline"] == nil {
		t.Fatal("expected timeline in response")
	}
}

func TestSubscriptionUnsubscribeIsOneClick(t *testing.T) {
	h := newHarness(t)
	// /api/my/subscriptions/unsubscribe is deliberately auth-less.
	rec := h.do(t, http.MethodGet, "/api/my/subscriptions/unsubscribe?user=usr-student&category=booking", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	sub, _ := h.store.Subscription(context.Background(), "usr-student", "booking")
	if sub.Subscribed {
		t.Fatal("expected subscription to be off after one-click unsubscribe")
	}
}

func TestTeacherAnalyticsFormat(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "teacher", "teacher-pass")
	rec := h.do(t, http.MethodGet, "/api/teacher/analytics", "", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	for _, key := range []string{"window_7d", "window_30d", "window_90d"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("missing %s in analytics payload: %s", key, rec.Body.String())
		}
	}
}
