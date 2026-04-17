package notify

import (
	"context"

	"github.com/eaglepoint/harborclass/internal/models"
	"github.com/eaglepoint/harborclass/internal/store"
)

// SeedTemplates installs the default notification templates. It is safe
// to call on every boot; duplicates are upserted in place.
func SeedTemplates(ctx context.Context, s store.Store) error {
	defaults := []models.NotificationTemplate{
		{ID: "booking.reminder", Category: "booking", Subject: "Booking reminder", Body: "Your booking {{.Number}} is coming up."},
		{ID: "booking.cancelled", Category: "booking", Subject: "Booking cancelled", Body: "Your booking {{.Number}} was cancelled."},
		{ID: "delivery.assigned", Category: "delivery", Subject: "Courier assigned", Body: "Courier {{.CourierID}} assigned to order {{.Number}}."},
		{ID: "refund.updated", Category: "refund", Subject: "Refund update", Body: "Refund for order {{.Number}} is now {{.State}}."},
	}
	for _, t := range defaults {
		if err := s.UpsertTemplate(ctx, t); err != nil {
			return err
		}
	}
	return nil
}
