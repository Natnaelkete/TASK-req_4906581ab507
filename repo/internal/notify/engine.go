// Package notify implements the offline notification delivery center.
// The engine enforces per-order reminder caps, exponential-backoff
// retries, and one-click unsubscribe semantics before writing each
// send attempt to the audit-backed delivery table.
package notify

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/eaglepoint/harborclass/internal/models"
	"github.com/eaglepoint/harborclass/internal/store"
)

// Defaults from the HarborClass business prompt. Overridable via config.
const (
	DefaultReminderCap  = 3               // max 3 reminders per order per day
	DefaultMaxAttempts  = 5               // retry up to 5 times on transient failure
	DefaultBaseBackoff  = 500 * time.Millisecond
	DefaultJitterBackoff = 100 * time.Millisecond
)

// Errors surfaced to callers / UI.
var (
	ErrRateLimited   = errors.New("reminder cap reached for this order today")
	ErrUnsubscribed  = errors.New("recipient has unsubscribed from this category")
	ErrMaxRetries    = errors.New("maximum retry attempts exhausted")
	ErrTemplateMissing = errors.New("template not found")
)

// Sender is the transport abstraction. In the bundled offline deploy,
// the sender just writes to an in-process audit record; in production
// it would point at an on-prem SMTP/SMS gateway.
type Sender interface {
	Send(ctx context.Context, to models.User, tpl models.NotificationTemplate) error
}

// LocalSender is a deterministic, offline implementation of Sender.
// It can be configured to fail on-demand to exercise retry logic.
type LocalSender struct {
	// FailFirstN, if >0, causes the first N sends to return an error.
	// Useful for tests and demo flows.
	FailFirstN int
	calls      int
}

// Send records the send deterministically and optionally fails.
func (l *LocalSender) Send(_ context.Context, _ models.User, _ models.NotificationTemplate) error {
	l.calls++
	if l.calls <= l.FailFirstN {
		return fmt.Errorf("transient failure on attempt %d", l.calls)
	}
	return nil
}

// Engine orchestrates template lookup, rate limiting, subscription
// checks, retries, and audit writes.
type Engine struct {
	Store        store.Store
	Sender       Sender
	Clock        func() time.Time
	ReminderCap  int
	MaxAttempts  int
	BaseBackoff  time.Duration
	JitterBackoff time.Duration
}

// NewEngine constructs an engine with defaults applied.
func NewEngine(s store.Store, sender Sender) *Engine {
	return &Engine{
		Store:       s,
		Sender:      sender,
		Clock:       time.Now,
		ReminderCap: DefaultReminderCap,
		MaxAttempts: DefaultMaxAttempts,
		BaseBackoff: DefaultBaseBackoff,
		JitterBackoff: DefaultJitterBackoff,
	}
}

// SendRequest is a typed input for Engine.Send.
type SendRequest struct {
	OrderID    string
	UserID     string
	Recipient  models.User
	Category   string
	TemplateID string
}

// Result is returned from Engine.Send describing the outcome.
type Result struct {
	Attempts int
	Success  bool
	Error    string
}

// Send executes the full send pipeline: subscription, rate limit,
// retries with exponential backoff, and an audited delivery_attempts
// record for every attempt.
func (e *Engine) Send(ctx context.Context, req SendRequest) (Result, error) {
	tpl, err := e.Store.TemplateByID(ctx, req.TemplateID)
	if err != nil {
		return Result{}, ErrTemplateMissing
	}
	sub, err := e.Store.Subscription(ctx, req.UserID, req.Category)
	if err != nil {
		return Result{}, err
	}
	if !sub.Subscribed {
		return Result{Success: false, Error: ErrUnsubscribed.Error()}, ErrUnsubscribed
	}
	today := e.Clock()
	count, err := e.Store.CountAttemptsForOrderOn(ctx, req.OrderID, today)
	if err != nil {
		return Result{}, err
	}
	if count >= e.ReminderCap {
		return Result{Success: false, Error: ErrRateLimited.Error()}, ErrRateLimited
	}

	res := Result{}
	for attempt := 1; attempt <= e.MaxAttempts; attempt++ {
		sendErr := e.Sender.Send(ctx, req.Recipient, tpl)
		res.Attempts = attempt
		_ = e.Store.RecordDeliveryAttempt(ctx, models.DeliveryAttempt{
			ID:         fmt.Sprintf("%s-%d-%d", req.OrderID, today.UnixNano(), attempt),
			OrderID:    req.OrderID,
			UserID:     req.UserID,
			Category:   req.Category,
			TemplateID: req.TemplateID,
			Attempt:    attempt,
			SentAt:     e.Clock(),
			Success:    sendErr == nil,
			Error:      errText(sendErr),
		})
		if sendErr == nil {
			res.Success = true
			return res, nil
		}
		if attempt < e.MaxAttempts {
			Sleep(Backoff(e.BaseBackoff, attempt, e.JitterBackoff))
		}
	}
	res.Error = ErrMaxRetries.Error()
	return res, ErrMaxRetries
}

// Backoff computes exponential backoff for the N-th attempt starting at
// attempt==1. Jitter is applied as a flat additive component.
func Backoff(base time.Duration, attempt int, jitter time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := base
	for i := 1; i < attempt; i++ {
		d *= 2
	}
	return d + jitter
}

// Sleep is swapped out in tests.
var Sleep = func(d time.Duration) { time.Sleep(d) }

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
