package dispatch_test

import (
	"errors"
	"testing"
	"time"

	"github.com/eaglepoint/harborclass/internal/dispatch"
	"github.com/eaglepoint/harborclass/internal/models"
)

func couriers() []models.User {
	return []models.User{
		{ID: "c1", Role: models.RoleCourier, Rating: 4.2, Load: 5, Location: models.Location{Lat: 40.0, Lng: -74.0}},
		{ID: "c2", Role: models.RoleCourier, Rating: 4.9, Load: 2, Location: models.Location{Lat: 41.0, Lng: -74.0}},
		{ID: "c3", Role: models.RoleCourier, Rating: 3.8, Load: 9, Location: models.Location{Lat: 40.5, Lng: -74.1}},
	}
}

func TestRatingFirstPicksBest(t *testing.T) {
	picked, err := dispatch.Select(dispatch.StrategyRating).Select(models.Order{}, couriers())
	if err != nil {
		t.Fatal(err)
	}
	if picked.ID != "c2" {
		t.Fatalf("expected c2 (best rating), got %s", picked.ID)
	}
}

func TestLoadBalancedPicksLightest(t *testing.T) {
	picked, err := dispatch.Select(dispatch.StrategyLoadBalanced).Select(models.Order{}, couriers())
	if err != nil {
		t.Fatal(err)
	}
	if picked.ID != "c2" {
		t.Fatalf("expected c2 (load 2), got %s", picked.ID)
	}
}

func TestDistanceFirstPicksNearest(t *testing.T) {
	o := models.Order{PickupZone: "alpha"}
	picked, err := dispatch.Select(dispatch.StrategyDistance).Select(o, couriers())
	if err != nil {
		t.Fatal(err)
	}
	if picked.ID == "" {
		t.Fatal("expected a picked courier")
	}
}

func TestEmptyPoolReturnsError(t *testing.T) {
	for _, s := range []dispatch.StrategyName{dispatch.StrategyDistance, dispatch.StrategyRating, dispatch.StrategyLoadBalanced} {
		if _, err := dispatch.Select(s).Select(models.Order{}, nil); !errors.Is(err, dispatch.ErrNoEligibleCourier) {
			t.Fatalf("strategy %s: expected ErrNoEligibleCourier, got %v", s, err)
		}
	}
}

func TestValidatePickupCutoff(t *testing.T) {
	// 21:00 is after the default 20:00 cutoff.
	order := models.Order{PickupAt: time.Date(2026, 3, 28, 21, 0, 0, 0, time.UTC), PickupZone: "east"}
	if err := dispatch.ValidatePickup(order, models.Facility{PickupCutoffHour: 20}); !errors.Is(err, dispatch.ErrPickupAfterCutoff) {
		t.Fatalf("expected cutoff error, got %v", err)
	}
}

func TestValidatePickupBlacklistedZone(t *testing.T) {
	order := models.Order{PickupAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC), PickupZone: "restricted-north"}
	fac := models.Facility{BlacklistedZones: []string{"restricted-north"}, PickupCutoffHour: 20}
	if err := dispatch.ValidatePickup(order, fac); !errors.Is(err, dispatch.ErrZoneBlacklisted) {
		t.Fatalf("expected ErrZoneBlacklisted, got %v", err)
	}
}

func TestAssignDoubleBookingRejected(t *testing.T) {
	pickup := time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)
	c := []models.User{{ID: "solo", Role: models.RoleCourier}}
	existing := []models.Order{{
		ID: "d0", CourierID: "solo", State: models.StateInProgress, PickupAt: pickup, Kind: models.OrderDelivery,
	}}
	order := models.Order{PickupAt: pickup.Add(15 * time.Minute), PickupZone: "east"}
	_, err := dispatch.Assign(dispatch.StrategyDistance, order, models.Facility{PickupCutoffHour: 20}, c, existing)
	if !errors.Is(err, dispatch.ErrCourierDoubleBook) {
		t.Fatalf("expected double-book conflict, got %v", err)
	}
}

func TestAssignCourierZoneBlacklist(t *testing.T) {
	c := []models.User{{ID: "c1", Role: models.RoleCourier, BlacklistZone: []string{"south"}}}
	order := models.Order{PickupAt: time.Date(2026, 3, 28, 9, 0, 0, 0, time.UTC), PickupZone: "south"}
	_, err := dispatch.Assign(dispatch.StrategyDistance, order, models.Facility{PickupCutoffHour: 20}, c, nil)
	if !errors.Is(err, dispatch.ErrCourierBlacklist) {
		t.Fatalf("expected courier blacklist error, got %v", err)
	}
}

func TestAssignFacilityCutoffSurfaces(t *testing.T) {
	order := models.Order{PickupAt: time.Date(2026, 3, 28, 23, 0, 0, 0, time.UTC), PickupZone: "east"}
	_, err := dispatch.Assign(dispatch.StrategyDistance, order, models.Facility{PickupCutoffHour: 20}, couriers(), nil)
	if !errors.Is(err, dispatch.ErrPickupAfterCutoff) {
		t.Fatalf("expected ErrPickupAfterCutoff, got %v", err)
	}
}

func TestAssignHappyPath(t *testing.T) {
	order := models.Order{PickupAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC), PickupZone: "east"}
	picked, err := dispatch.Assign(dispatch.StrategyRating, order, models.Facility{PickupCutoffHour: 20}, couriers(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if picked.ID != "c2" {
		t.Fatalf("expected rating-first to pick c2, got %s", picked.ID)
	}
}
