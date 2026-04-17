package notify_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eaglepoint/harborclass/internal/models"
	"github.com/eaglepoint/harborclass/internal/notify"
	"github.com/eaglepoint/harborclass/internal/store"
)

var baseTime = time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)

// newEngine wires a store+engine with deterministic settings; Sleep is
// swapped to a no-op so retries don't slow the suite.
func newEngine(t *testing.T) (*store.Memory, *notify.Engine, *notify.LocalSender) {
	t.Helper()
	s := store.NewMemory()
	ctx := context.Background()
	if err := notify.SeedTemplates(ctx, s); err != nil {
		t.Fatal(err)
	}
	sender := &notify.LocalSender{}
	e := notify.NewEngine(s, sender)
	e.Clock = func() time.Time { return baseTime }
	e.ReminderCap = notify.DefaultReminderCap
	e.MaxAttempts = notify.DefaultMaxAttempts
	e.BaseBackoff = 0
	e.JitterBackoff = 0
	notify.Sleep = func(time.Duration) {}
	return s, e, sender
}

func TestBackoffDoubles(t *testing.T) {
	base := 100 * time.Millisecond
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 100 * time.Millisecond},
		{2, 200 * time.Millisecond},
		{3, 400 * time.Millisecond},
		{4, 800 * time.Millisecond},
		{5, 1600 * time.Millisecond},
	}
	for _, tc := range cases {
		got := notify.Backoff(base, tc.attempt, 0)
		if got != tc.want {
			t.Fatalf("attempt %d: want %v got %v", tc.attempt, tc.want, got)
		}
	}
}

func TestSendHappyPath(t *testing.T) {
	s, e, sender := newEngine(t)
	recipient := models.User{ID: "u1", Username: "alex", Role: models.RoleStudent}
	_ = s.CreateUser(context.Background(), recipient)
	res, err := e.Send(context.Background(), notify.SendRequest{
		OrderID: "o1", UserID: "u1", Recipient: recipient, Category: "booking", TemplateID: "booking.reminder",
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Success || res.Attempts != 1 {
		t.Fatalf("expected 1 successful attempt, got %+v", res)
	}
	_ = sender
}

func TestRateLimitThreePerDay(t *testing.T) {
	s, e, _ := newEngine(t)
	ctx := context.Background()
	recipient := models.User{ID: "u1"}
	for i := 0; i < notify.DefaultReminderCap; i++ {
		if _, err := e.Send(ctx, notify.SendRequest{
			OrderID: "o1", UserID: "u1", Recipient: recipient, Category: "booking", TemplateID: "booking.reminder",
		}); err != nil {
			t.Fatalf("unexpected err on send %d: %v", i, err)
		}
	}
	if _, err := e.Send(ctx, notify.SendRequest{
		OrderID: "o1", UserID: "u1", Recipient: recipient, Category: "booking", TemplateID: "booking.reminder",
	}); !errors.Is(err, notify.ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
	// ensure attempts were recorded
	atts, _ := s.AttemptsByOrder(ctx, "o1")
	if len(atts) != notify.DefaultReminderCap {
		t.Fatalf("expected %d recorded attempts, got %d", notify.DefaultReminderCap, len(atts))
	}
}

func TestUnsubscribeBlocksSend(t *testing.T) {
	s, e, _ := newEngine(t)
	ctx := context.Background()
	_ = s.SetSubscription(ctx, models.Subscription{UserID: "u1", Category: "booking", Subscribed: false})
	_, err := e.Send(ctx, notify.SendRequest{
		OrderID: "o1", UserID: "u1", Recipient: models.User{ID: "u1"}, Category: "booking", TemplateID: "booking.reminder",
	})
	if !errors.Is(err, notify.ErrUnsubscribed) {
		t.Fatalf("expected ErrUnsubscribed, got %v", err)
	}
}

func TestRetriesUpToMaxWithBackoff(t *testing.T) {
	s, e, sender := newEngine(t)
	sender.FailFirstN = 3 // 3 failures, 4th succeeds
	_, err := e.Send(context.Background(), notify.SendRequest{
		OrderID: "o1", UserID: "u1", Recipient: models.User{ID: "u1"}, Category: "booking", TemplateID: "booking.reminder",
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	atts, _ := s.AttemptsByOrder(context.Background(), "o1")
	if len(atts) != 4 {
		t.Fatalf("expected 4 recorded attempts (3 fail + 1 success), got %d", len(atts))
	}
}

func TestRetriesExhaustAtFive(t *testing.T) {
	_, e, sender := newEngine(t)
	sender.FailFirstN = 99 // never succeeds
	res, err := e.Send(context.Background(), notify.SendRequest{
		OrderID: "o1", UserID: "u1", Recipient: models.User{ID: "u1"}, Category: "booking", TemplateID: "booking.reminder",
	})
	if !errors.Is(err, notify.ErrMaxRetries) {
		t.Fatalf("expected ErrMaxRetries, got %v", err)
	}
	if res.Attempts != notify.DefaultMaxAttempts {
		t.Fatalf("expected %d attempts, got %d", notify.DefaultMaxAttempts, res.Attempts)
	}
}

func TestTemplateMissing(t *testing.T) {
	_, e, _ := newEngine(t)
	_, err := e.Send(context.Background(), notify.SendRequest{
		OrderID: "o1", UserID: "u1", Recipient: models.User{ID: "u1"}, Category: "booking", TemplateID: "missing",
	})
	if !errors.Is(err, notify.ErrTemplateMissing) {
		t.Fatalf("expected ErrTemplateMissing, got %v", err)
	}
}
