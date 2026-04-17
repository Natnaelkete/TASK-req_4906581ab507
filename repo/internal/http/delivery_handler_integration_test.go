package http_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/eaglepoint/harborclass/internal/models"
)

// delivery_handler_integration_test.go exercises the full dispatch
// pipeline via the HTTP layer: facility lookup, eligibility filtering,
// strategy selection, and audit write.

// morningPickup returns a pickup time at 09:00 UTC a few days in the
// future. Using a fixed hour (well before the 20:00 facility cutoff)
// keeps the tests deterministic regardless of the wall-clock hour the
// suite is executed in.
func morningPickup(offset time.Duration) time.Time {
	base := time.Date(2099, 1, 15, 9, 0, 0, 0, time.UTC)
	return base.Add(offset)
}

func TestAssignCourierHappyPath(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "dispatcher", "dispatch-pass")
	id := seedDelivery(t, h, "east", morningPickup(0))
	body := `{"strategy":"rating-first","facility_id":"fac-main"}`
	rec := h.do(t, http.MethodPost, "/api/deliveries/"+id+"/assign", body, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	got := mustJSON(t, rec.Body.Bytes())
	if got["courier_id"] == nil {
		t.Fatal("expected courier_id in response")
	}
	if got["strategy"] != "rating-first" {
		t.Fatalf("expected strategy echo, got %v", got["strategy"])
	}
}

func TestAssignCourierBlacklistedZoneSurfacesConflict(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "dispatcher", "dispatch-pass")
	id := seedDelivery(t, h, "restricted-north", morningPickup(0))
	rec := h.do(t, http.MethodPost, "/api/deliveries/"+id+"/assign", `{"facility_id":"fac-main"}`, tok)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 zone blacklisted, got %d body=%s", rec.Code, rec.Body.String())
	}
	got := mustJSON(t, rec.Body.Bytes())
	if got["conflict"] != true {
		t.Fatal("expected conflict=true flag in response")
	}
}

func TestAssignCourierAfterCutoffSurfacesConflict(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "dispatcher", "dispatch-pass")
	evening := time.Date(2099, 3, 28, 22, 0, 0, 0, time.UTC)
	id := seedDelivery(t, h, "east", evening)
	rec := h.do(t, http.MethodPost, "/api/deliveries/"+id+"/assign", `{"facility_id":"fac-main"}`, tok)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 after cutoff, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestListDeliveriesEnumerates(t *testing.T) {
	h := newHarness(t)
	tok := h.loginAs(t, "dispatcher", "dispatch-pass")
	// 2-hour spacing so the ids (formatted from time HHMMSS) are unique.
	_ = seedDelivery(t, h, "east", morningPickup(0))
	_ = seedDelivery(t, h, "west", morningPickup(2*time.Hour))
	rec := h.do(t, http.MethodGet, "/api/deliveries", "", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := mustJSON(t, rec.Body.Bytes())
	ds, _ := body["deliveries"].([]any)
	if len(ds) < 2 {
		t.Fatalf("expected >=2 deliveries, got %d body=%s", len(ds), rec.Body.String())
	}
}

// seedDelivery directly persists a delivery order so the test can skip
// the create endpoint and focus on assignment behaviour.
func seedDelivery(t *testing.T, h *harness, zone string, pickup time.Time) string {
	t.Helper()
	o := models.Order{
		ID:         "dlv-" + zone + "-" + pickup.Format("20060102150405"),
		Number:     "HC-TEST-" + zone + "-" + pickup.Format("150405"),
		Kind:       models.OrderDelivery,
		State:      models.StateCreated,
		OrgID:      "org-main",
		PickupZone: zone,
		PickupAt:   pickup,
		CreatedAt:  time.Now(),
	}
	if err := h.store.CreateOrder(context.Background(), o); err != nil {
		t.Fatalf("seed delivery: %v", err)
	}
	return o.ID
}
