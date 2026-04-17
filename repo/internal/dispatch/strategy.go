// Package dispatch contains courier assignment strategies and the
// conflict-detection rules enforced on delivery orders.
package dispatch

import (
	"errors"
	"math"
	"sort"

	"github.com/eaglepoint/harborclass/internal/models"
)

// StrategyName identifies a courier selection strategy.
type StrategyName string

const (
	StrategyDistance     StrategyName = "distance-first"
	StrategyRating       StrategyName = "rating-first"
	StrategyLoadBalanced StrategyName = "load-balanced"
)

// Dispatch errors exposed over the API as user-visible conflicts.
var (
	ErrNoEligibleCourier = errors.New("no eligible courier available")
	ErrPickupAfterCutoff = errors.New("pickup falls after facility cutoff")
	ErrZoneBlacklisted   = errors.New("pickup zone is blacklisted at facility level")
	ErrCourierBlacklist  = errors.New("courier cannot service the requested zone")
	ErrCourierDoubleBook = errors.New("courier already assigned to an overlapping delivery")
)

// Strategy selects a courier for a delivery order.
type Strategy interface {
	Select(order models.Order, couriers []models.User) (models.User, error)
}

// Select returns the implementation for a strategy name.
func Select(name StrategyName) Strategy {
	switch name {
	case StrategyRating:
		return ratingFirst{}
	case StrategyLoadBalanced:
		return loadBalanced{}
	default:
		return distanceFirst{}
	}
}

type distanceFirst struct{}

func (distanceFirst) Select(o models.Order, couriers []models.User) (models.User, error) {
	if len(couriers) == 0 {
		return models.User{}, ErrNoEligibleCourier
	}
	ref := orderLocation(o)
	sorted := make([]models.User, len(couriers))
	copy(sorted, couriers)
	sort.SliceStable(sorted, func(i, j int) bool {
		return haversine(sorted[i].Location, ref) < haversine(sorted[j].Location, ref)
	})
	return sorted[0], nil
}

type ratingFirst struct{}

func (ratingFirst) Select(_ models.Order, couriers []models.User) (models.User, error) {
	if len(couriers) == 0 {
		return models.User{}, ErrNoEligibleCourier
	}
	sorted := make([]models.User, len(couriers))
	copy(sorted, couriers)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Rating > sorted[j].Rating })
	return sorted[0], nil
}

type loadBalanced struct{}

func (loadBalanced) Select(_ models.Order, couriers []models.User) (models.User, error) {
	if len(couriers) == 0 {
		return models.User{}, ErrNoEligibleCourier
	}
	sorted := make([]models.User, len(couriers))
	copy(sorted, couriers)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Load < sorted[j].Load })
	return sorted[0], nil
}

// orderLocation is a tiny helper. We use the order's pickup zone
// converted into a stable pseudo-location so distance sorting behaves
// deterministically even for demo data.
func orderLocation(o models.Order) models.Location {
	// Encode the zone string into a deterministic lat/lng. In production
	// this is a real warehouse lookup; here it is intentionally simple
	// and exposed so tests can reason about ordering.
	var lat, lng float64
	for i, r := range o.PickupZone {
		if i%2 == 0 {
			lat += float64(r)
		} else {
			lng += float64(r)
		}
	}
	return models.Location{Lat: lat, Lng: lng}
}

// haversine computes the great-circle distance between two locations.
func haversine(a, b models.Location) float64 {
	const R = 6371.0
	la1, la2 := a.Lat*math.Pi/180, b.Lat*math.Pi/180
	dlat := (b.Lat - a.Lat) * math.Pi / 180
	dlng := (b.Lng - a.Lng) * math.Pi / 180
	h := math.Sin(dlat/2)*math.Sin(dlat/2) + math.Cos(la1)*math.Cos(la2)*math.Sin(dlng/2)*math.Sin(dlng/2)
	return 2 * R * math.Asin(math.Sqrt(h))
}
