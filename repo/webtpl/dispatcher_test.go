package webtpl_test

import (
	"strings"
	"testing"
	"time"

	"github.com/eaglepoint/harborclass/internal/models"
	"github.com/eaglepoint/harborclass/webtpl"
)

func TestDispatcherDashboardShowsStrategyAndDeliveries(t *testing.T) {
	d := webtpl.DispatcherData{
		User: models.User{DisplayName: "Dana Dispatcher", Role: models.RoleDispatcher},
		Couriers: []models.User{
			{ID: "c1", DisplayName: "Casey Courier", Rating: 4.9, Load: 2},
		},
		Deliveries: []models.Order{
			{Number: "HC-03282026-000011", PickupZone: "east", PickupAt: time.Date(2026, 3, 28, 9, 0, 0, 0, time.UTC), State: models.StateInProgress},
		},
	}
	html := webtpl.RenderDispatcherDashboard(d)
	for _, need := range []string{
		`data-role="dispatcher"`,
		"Dispatcher Console",
		"Dana Dispatcher",
		"distance-first",
		"rating-first",
		"load-balanced",
		"HC-03282026-000011",
		"Casey Courier",
		"east",
		"state-badge state-in_progress",
	} {
		if !strings.Contains(html, need) {
			t.Fatalf("dispatcher HTML missing %q", need)
		}
	}
}

func TestDispatcherDashboardRendersConflictMessages(t *testing.T) {
	d := webtpl.DispatcherData{
		User:          models.User{DisplayName: "Dana", Role: models.RoleDispatcher},
		ConflictNotes: []string{"pickup falls after facility cutoff", "courier cannot service the requested zone"},
	}
	html := webtpl.RenderDispatcherDashboard(d)
	for _, need := range []string{
		"Conflicts",
		"pickup falls after facility cutoff",
		"courier cannot service the requested zone",
		"conflict-msg",
	} {
		if !strings.Contains(html, need) {
			t.Fatalf("conflicts HTML missing %q", need)
		}
	}
}

func TestDispatcherEmptyConflictsIsClean(t *testing.T) {
	d := webtpl.DispatcherData{User: models.User{DisplayName: "Dana", Role: models.RoleDispatcher}}
	html := webtpl.RenderDispatcherDashboard(d)
	if !strings.Contains(html, "No conflicts detected") {
		t.Fatalf("expected empty-conflicts placeholder, got %s", html)
	}
}
