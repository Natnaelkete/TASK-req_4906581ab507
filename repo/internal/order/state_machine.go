// Package order implements the shared booking/delivery state machine
// with the strict transition rules described in the HarborClass prompt.
package order

import (
	"errors"
	"fmt"
	"time"

	"github.com/eaglepoint/harborclass/internal/models"
)

// MaxReschedules is the upper bound on reschedules for a confirmed order.
const MaxReschedules = 2

// CancelRequiresApprovalWindow is the window before session start where a
// cancellation requires teacher approval (24 hours per spec).
const CancelRequiresApprovalWindow = 24 * time.Hour

// RefundWindow is the post-completion window in which a refund may be
// filed (7 days per spec).
const RefundWindow = 7 * 24 * time.Hour

// Transition errors.
var (
	ErrInvalidState       = errors.New("invalid state for this action")
	ErrRescheduleLimit    = errors.New("reschedule limit reached")
	ErrRefundWindowClosed = errors.New("refund window closed")
	ErrNeedsApproval      = errors.New("cancellation requires teacher approval")
)

// Machine drives state changes. It is stateless itself; all state
// lives on the Order value and is returned alongside the decision.
type Machine struct {
	Now func() time.Time
}

// NewMachine returns a machine using time.Now.
func NewMachine() *Machine { return &Machine{Now: time.Now} }

// GenerateNumber builds a human-readable HC-MMDDYYYY-NNNNNN number.
func GenerateNumber(now time.Time, seq int) string {
	return fmt.Sprintf("HC-%02d%02d%04d-%06d", now.Month(), now.Day(), now.Year(), seq)
}

// Create seeds a fresh order in the "created" state.
func (m *Machine) Create(o models.Order) models.Order {
	o.State = models.StateCreated
	o.Timeline = append(o.Timeline, models.OrderEvent{
		At: m.Now(), State: models.StateCreated, Actor: "system", Message: "order created",
	})
	return o
}

// Confirm transitions created/pending -> confirmed.
func (m *Machine) Confirm(o models.Order, actor string) (models.Order, error) {
	switch o.State {
	case models.StateCreated, models.StatePending:
		o.State = models.StateConfirmed
		o.Timeline = append(o.Timeline, models.OrderEvent{At: m.Now(), State: o.State, Actor: actor, Message: "confirmed"})
		return o, nil
	}
	return o, ErrInvalidState
}

// Reschedule moves a confirmed order to rescheduled, enforcing the cap
// of two reschedules. The caller is expected to pass the new pickup/
// session time and update the order accordingly on success.
func (m *Machine) Reschedule(o models.Order, actor string, newStart time.Time) (models.Order, error) {
	if o.State != models.StateConfirmed && o.State != models.StateRescheduled {
		return o, ErrInvalidState
	}
	if o.RescheduleCount >= MaxReschedules {
		return o, ErrRescheduleLimit
	}
	o.RescheduleCount++
	o.PickupAt = newStart
	o.State = models.StateRescheduled
	o.Timeline = append(o.Timeline, models.OrderEvent{At: m.Now(), State: o.State, Actor: actor, Message: "rescheduled"})
	return o, nil
}

// Cancel transitions to cancelled. If inside the 24h approval window
// and the actor is not a teacher/admin, the call fails with
// ErrNeedsApproval; the caller can then queue the cancellation for
// approval instead.
func (m *Machine) Cancel(o models.Order, actor string, approver bool, sessionStart time.Time) (models.Order, error) {
	if o.State == models.StateCancelled || o.State == models.StateCompleted || o.State == models.StateRefunded {
		return o, ErrInvalidState
	}
	if sessionStart.Sub(m.Now()) < CancelRequiresApprovalWindow && !approver {
		return o, ErrNeedsApproval
	}
	o.State = models.StateCancelled
	o.Timeline = append(o.Timeline, models.OrderEvent{At: m.Now(), State: o.State, Actor: actor, Message: "cancelled"})
	return o, nil
}

// Complete transitions in_progress/confirmed -> completed.
func (m *Machine) Complete(o models.Order, actor string) (models.Order, error) {
	switch o.State {
	case models.StateConfirmed, models.StateInProgress, models.StateRescheduled:
		o.State = models.StateCompleted
		o.CompletedAt = m.Now()
		o.Timeline = append(o.Timeline, models.OrderEvent{At: m.Now(), State: o.State, Actor: actor, Message: "completed"})
		return o, nil
	}
	return o, ErrInvalidState
}

// RequestRefund moves a completed order into refund_review. It must be
// called within 7 days of completion.
func (m *Machine) RequestRefund(o models.Order, actor string) (models.Order, error) {
	if o.State != models.StateCompleted {
		return o, ErrInvalidState
	}
	if m.Now().Sub(o.CompletedAt) > RefundWindow {
		return o, ErrRefundWindowClosed
	}
	o.State = models.StateRefundReview
	o.Payment = models.PayRefundPending
	o.Timeline = append(o.Timeline, models.OrderEvent{At: m.Now(), State: o.State, Actor: actor, Message: "refund requested"})
	return o, nil
}

// ApproveRefund moves refund_review -> refunded.
func (m *Machine) ApproveRefund(o models.Order, actor string) (models.Order, error) {
	if o.State != models.StateRefundReview {
		return o, ErrInvalidState
	}
	o.State = models.StateRefunded
	o.Payment = models.PayRefunded
	o.Timeline = append(o.Timeline, models.OrderEvent{At: m.Now(), State: o.State, Actor: actor, Message: "refund approved"})
	return o, nil
}

// Rollback is an admin-visible compensating transition, recorded in the
// timeline and audit log. It transitions the order into rolled_back.
func (m *Machine) Rollback(o models.Order, actor, reason string) (models.Order, error) {
	if o.State == models.StateRolledBack {
		return o, ErrInvalidState
	}
	o.State = models.StateRolledBack
	o.Timeline = append(o.Timeline, models.OrderEvent{At: m.Now(), State: o.State, Actor: actor, Message: "rolled back: " + reason})
	return o, nil
}
