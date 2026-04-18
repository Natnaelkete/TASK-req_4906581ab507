// Package models contains the shared domain types used across store,
// services, and HTTP handlers. Keeping them in one place lets the
// static auditor quickly map requirements to concrete Go structs.
package models

import "time"

// Role enumerates the five user roles recognised by HarborClass.
type Role string

const (
	RoleStudent    Role = "student"
	RoleTeacher    Role = "teacher"
	RoleCourier    Role = "courier"
	RoleDispatcher Role = "dispatcher"
	RoleAdmin      Role = "admin"
)

// User is a HarborClass account. PII (phone/password) is stored encrypted.
type User struct {
	ID            string
	Username      string
	Role          Role
	OrgID         string
	ClassIDs      []string
	PasswordHash  string
	PhoneCipher   string // AES-GCM ciphertext of the real phone number
	DisplayName   string
	Rating        float64
	Load          int
	Location      Location
	BlacklistZone []string
	CreatedAt     time.Time
}

// Location is a simple latitude/longitude coordinate pair used for
// distance calculations in dispatch strategies.
type Location struct {
	Lat float64
	Lng float64
}

// Session is a scheduled class/event that students can book into.
// OrgID is the tenant boundary: booking creation must verify the
// session belongs to the student's organisation, otherwise sessions
// could be booked across tenants by guessing session ids.
type Session struct {
	ID         string
	TeacherID  string
	ClassID    string
	OrgID      string
	Title      string
	StartsAt   time.Time
	EndsAt     time.Time
	Capacity   int
	BookedSize int
	Location   Location
}

// OrderKind distinguishes a booking from a delivery.
type OrderKind string

const (
	OrderBooking  OrderKind = "booking"
	OrderDelivery OrderKind = "delivery"
)

// OrderState is the visible state badge on each order.
type OrderState string

const (
	StateCreated        OrderState = "created"
	StatePending        OrderState = "pending_approval"
	StateConfirmed      OrderState = "confirmed"
	StateRescheduled    OrderState = "rescheduled"
	StateInProgress     OrderState = "in_progress"
	StateCompleted      OrderState = "completed"
	StateCancelled      OrderState = "cancelled"
	StateRefundReview   OrderState = "refund_review"
	StateRefunded       OrderState = "refunded"
	StateRolledBack     OrderState = "rolled_back"
)

// PaymentStatus is the reconciliation field tracked per order.
// HarborClass never touches real money; this is a bookkeeping marker.
type PaymentStatus string

const (
	PayUnpaid        PaymentStatus = "unpaid"
	PayWaived        PaymentStatus = "waived"
	PayCashRecorded  PaymentStatus = "cash_recorded"
	PayRefundPending PaymentStatus = "refund_pending"
	PayRefunded      PaymentStatus = "refunded"
)

// Order represents either a booking (student -> session) or a delivery
// (dispatcher -> courier). They share a state machine.
type Order struct {
	ID              string
	Number          string // e.g. HC-03282026-000742
	Kind            OrderKind
	State           OrderState
	Payment         PaymentStatus
	StudentID       string
	TeacherID       string
	SessionID       string
	CourierID       string
	PickupZone      string
	PickupAt        time.Time
	OrgID           string
	ClassID         string
	CreatedAt       time.Time
	CompletedAt     time.Time
	RescheduleCount int
	Timeline        []OrderEvent
}

// OrderEvent is one step on the order's timeline and is shown on the
// student's booking detail page as well as the dispatcher's view.
type OrderEvent struct {
	At      time.Time
	State   OrderState
	Actor   string
	Message string
}

// Facility represents a HarborClass site. It carries the blacklist
// used by dispatch to refuse pickups in restricted zones.
type Facility struct {
	ID              string
	Name            string
	BlacklistedZones []string
	PickupCutoffHour int
}

// NotificationTemplate describes a reusable message template.
type NotificationTemplate struct {
	ID       string
	Category string
	Subject  string
	Body     string
}

// Subscription represents a per-user, per-category subscription flag
// supporting the one-click unsubscribe requirement.
type Subscription struct {
	UserID     string
	Category   string
	Subscribed bool
}

// DeliveryAttempt records every send attempt for audit and retry.
type DeliveryAttempt struct {
	ID         string
	OrderID    string
	UserID     string
	Category   string
	TemplateID string
	Attempt    int
	SentAt     time.Time
	Success    bool
	Error      string
}

// AuditEntry is one entry in the tamper-evident audit chain. OrgID is
// the tenant the action occurred in; search/export paths filter by
// this so admins cannot read entries belonging to other organisations.
// A blank OrgID denotes a system-wide event (e.g. unauthenticated
// failures) and is visible to every tenant admin.
type AuditEntry struct {
	ID        string
	At        time.Time
	OrgID     string
	Actor     string
	Action    string
	Resource  string
	Detail    string
	PrevHash  string
	Hash      string
}

// Device represents a managed device subject to upgrade policies.
type Device struct {
	ID             string
	UserID         string
	Platform       string
	Version        string
	Canary         bool
	ForcedUpgradeTo string
	LastSeen       time.Time
}

// Permission is a per-org overlay on top of the static authorisation
// matrix. Roles listed here are granted the named action within OrgID.
type Permission struct {
	OrgID  string
	Action string
	Roles  []string
}

// ContentItem is a teacher-authored piece of content (post or class
// material). Analytics roll up from the Views/Likes/Favorites/Followers
// fields and CreatedAt is used to bucket into 7/30/90-day windows.
type ContentItem struct {
	ID        string
	TeacherID string
	Title     string
	Body      string
	Pinned    bool
	Published bool
	Views     int
	Likes     int
	Favorites int
	Followers int
	CreatedAt time.Time
	UpdatedAt time.Time
}
