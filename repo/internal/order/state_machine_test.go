package order_test

import (
	"errors"
	"testing"
	"time"

	"github.com/eaglepoint/harborclass/internal/models"
	"github.com/eaglepoint/harborclass/internal/order"
)

// fixed clock used throughout the table to make assertions deterministic.
var clock = time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)

func machine() *order.Machine {
	return &order.Machine{Now: func() time.Time { return clock }}
}

func TestCreateInitialState(t *testing.T) {
	o := machine().Create(models.Order{ID: "o1"})
	if o.State != models.StateCreated {
		t.Fatalf("expected created, got %s", o.State)
	}
	if len(o.Timeline) == 0 {
		t.Fatal("expected timeline entry")
	}
}

func TestGenerateNumberFormat(t *testing.T) {
	got := order.GenerateNumber(clock, 742)
	want := "HC-03282026-000742"
	if got != want {
		t.Fatalf("want %s, got %s", want, got)
	}
}

func TestConfirmTransitions(t *testing.T) {
	tests := []struct {
		in  models.OrderState
		ok  bool
	}{
		{models.StateCreated, true},
		{models.StatePending, true},
		{models.StateCompleted, false},
		{models.StateCancelled, false},
	}
	for _, tc := range tests {
		t.Run(string(tc.in), func(t *testing.T) {
			out, err := machine().Confirm(models.Order{State: tc.in}, "system")
			if tc.ok && (err != nil || out.State != models.StateConfirmed) {
				t.Fatalf("expected confirmed, got %v err=%v", out.State, err)
			}
			if !tc.ok && err == nil {
				t.Fatal("expected error on invalid state")
			}
		})
	}
}

func TestRescheduleCapAndRecordsCount(t *testing.T) {
	o := models.Order{State: models.StateConfirmed}
	m := machine()
	for i := 0; i < order.MaxReschedules; i++ {
		var err error
		o, err = m.Reschedule(o, "student", clock.Add(24*time.Hour))
		if err != nil {
			t.Fatalf("unexpected err on attempt %d: %v", i, err)
		}
	}
	if _, err := m.Reschedule(o, "student", clock.Add(48*time.Hour)); !errors.Is(err, order.ErrRescheduleLimit) {
		t.Fatalf("expected ErrRescheduleLimit, got %v", err)
	}
	if o.RescheduleCount != order.MaxReschedules {
		t.Fatalf("expected count=%d, got %d", order.MaxReschedules, o.RescheduleCount)
	}
}

func TestCancelRequires24hApprovalWindow(t *testing.T) {
	o := models.Order{State: models.StateConfirmed}
	// Session starts in 12h — inside the 24h window, student cannot cancel.
	_, err := machine().Cancel(o, "student", false, clock.Add(12*time.Hour))
	if !errors.Is(err, order.ErrNeedsApproval) {
		t.Fatalf("expected ErrNeedsApproval, got %v", err)
	}
	// Teacher may approve, transition succeeds.
	out, err := machine().Cancel(o, "teacher", true, clock.Add(12*time.Hour))
	if err != nil || out.State != models.StateCancelled {
		t.Fatalf("teacher cancel: state=%s err=%v", out.State, err)
	}
	// Beyond 24h, anyone may cancel.
	out, err = machine().Cancel(o, "student", false, clock.Add(48*time.Hour))
	if err != nil || out.State != models.StateCancelled {
		t.Fatalf("outside window: state=%s err=%v", out.State, err)
	}
}

func TestRefundWindow(t *testing.T) {
	o := models.Order{State: models.StateCompleted, CompletedAt: clock.Add(-3 * 24 * time.Hour)}
	if _, err := machine().RequestRefund(o, "student"); err != nil {
		t.Fatalf("3 days inside 7-day window: %v", err)
	}
	o.CompletedAt = clock.Add(-10 * 24 * time.Hour)
	if _, err := machine().RequestRefund(o, "student"); !errors.Is(err, order.ErrRefundWindowClosed) {
		t.Fatalf("expected ErrRefundWindowClosed, got %v", err)
	}
}

func TestRefundApproval(t *testing.T) {
	o := models.Order{State: models.StateRefundReview}
	out, err := machine().ApproveRefund(o, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if out.State != models.StateRefunded || out.Payment != models.PayRefunded {
		t.Fatalf("bad state/payment: %s / %s", out.State, out.Payment)
	}
}

func TestRollbackTimeline(t *testing.T) {
	o := models.Order{State: models.StateConfirmed}
	out, err := machine().Rollback(o, "admin", "manual intervention")
	if err != nil {
		t.Fatal(err)
	}
	if out.State != models.StateRolledBack {
		t.Fatalf("bad state: %s", out.State)
	}
	if len(out.Timeline) == 0 || out.Timeline[len(out.Timeline)-1].Message == "" {
		t.Fatal("expected rollback timeline entry with message")
	}
}

func TestCompleteRequiresActiveState(t *testing.T) {
	for _, bad := range []models.OrderState{models.StateCreated, models.StateCancelled, models.StateRefunded} {
		if _, err := machine().Complete(models.Order{State: bad}, "teacher"); err == nil {
			t.Fatalf("expected error completing from %s", bad)
		}
	}
	out, err := machine().Complete(models.Order{State: models.StateConfirmed}, "teacher")
	if err != nil || out.State != models.StateCompleted {
		t.Fatalf("valid completion failed: state=%s err=%v", out.State, err)
	}
}
