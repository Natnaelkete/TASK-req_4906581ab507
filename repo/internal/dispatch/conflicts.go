package dispatch

import (
	"time"

	"github.com/eaglepoint/harborclass/internal/models"
)

// Facility-level cutoff default (may be overridden per facility).
const defaultCutoffHour = 20 // no pickups after 8pm

// EligibilityCheck is a single check applied during courier assignment.
// Callers run each check in order; the first failure surfaces to the UI
// as a visible conflict message.
type EligibilityCheck func() error

// EligibleCouriers filters the candidate pool by the zone, facility,
// and double-booking rules, returning the couriers that can still be
// considered by a selection strategy.
func EligibleCouriers(candidates []models.User, order models.Order, facility models.Facility, existing []models.Order) []models.User {
	out := []models.User{}
	for _, c := range candidates {
		if couriersBlacklistsZone(c, order.PickupZone) {
			continue
		}
		if overlapsExisting(c.ID, order.PickupAt, existing) {
			continue
		}
		_ = facility
		out = append(out, c)
	}
	return out
}

func couriersBlacklistsZone(c models.User, zone string) bool {
	for _, z := range c.BlacklistZone {
		if z == zone {
			return true
		}
	}
	return false
}

func overlapsExisting(courierID string, pickup time.Time, existing []models.Order) bool {
	for _, o := range existing {
		if o.CourierID != courierID {
			continue
		}
		if o.State == models.StateCancelled || o.State == models.StateCompleted {
			continue
		}
		// 60-minute sliding window around each pickup.
		if pickup.Sub(o.PickupAt).Abs() < time.Hour {
			return true
		}
	}
	return false
}

// ValidatePickup returns the first visible conflict on the order's own
// pickup parameters (cutoff hour, facility blacklist).
func ValidatePickup(order models.Order, facility models.Facility) error {
	cutoff := facility.PickupCutoffHour
	if cutoff == 0 {
		cutoff = defaultCutoffHour
	}
	if order.PickupAt.Hour() >= cutoff {
		return ErrPickupAfterCutoff
	}
	for _, z := range facility.BlacklistedZones {
		if z == order.PickupZone {
			return ErrZoneBlacklisted
		}
	}
	return nil
}

// Assign runs the full pipeline: validate pickup, filter candidates,
// then apply the selected strategy. The return value captures both the
// selected courier and any visible conflict to display in the UI.
func Assign(name StrategyName, order models.Order, facility models.Facility, candidates []models.User, existing []models.Order) (models.User, error) {
	if err := ValidatePickup(order, facility); err != nil {
		return models.User{}, err
	}
	eligible := EligibleCouriers(candidates, order, facility, existing)
	if len(eligible) == 0 {
		// Distinguish zone-level vs pure no-courier for the UI.
		for _, c := range candidates {
			if couriersBlacklistsZone(c, order.PickupZone) {
				return models.User{}, ErrCourierBlacklist
			}
			if overlapsExisting(c.ID, order.PickupAt, existing) {
				return models.User{}, ErrCourierDoubleBook
			}
		}
		return models.User{}, ErrNoEligibleCourier
	}
	return Select(name).Select(order, eligible)
}
