// Package store defines the persistence boundary. The production build
// uses PostgreSQL; tests use an in-memory implementation of the same
// interface (not a mock) so HTTP integration tests exercise real
// handler, service, and repository code.
package store

import (
	"context"
	"errors"
	"time"

	"github.com/eaglepoint/harborclass/internal/models"
)

// ErrNotFound is returned when a lookup does not match a row.
var ErrNotFound = errors.New("not found")

// ErrConflict indicates a conflict that cannot be repaired by retry,
// for example a booking collision on an already-full session.
var ErrConflict = errors.New("conflict")

// Store is the full persistence interface used by services.
// PostgreSQL and the in-memory reference implementation both satisfy it.
type Store interface {
	// Users & auth
	CreateUser(ctx context.Context, u models.User) error
	UserByUsername(ctx context.Context, username string) (models.User, error)
	UserByID(ctx context.Context, id string) (models.User, error)
	ListUsersByRole(ctx context.Context, role models.Role) ([]models.User, error)
	UpdateUser(ctx context.Context, u models.User) error

	// Facilities
	UpsertFacility(ctx context.Context, f models.Facility) error
	FacilityByID(ctx context.Context, id string) (models.Facility, error)

	// Sessions
	CreateSession(ctx context.Context, s models.Session) error
	ListSessions(ctx context.Context) ([]models.Session, error)
	SessionByID(ctx context.Context, id string) (models.Session, error)
	IncrementSessionBookings(ctx context.Context, id string, delta int) error

	// Orders (bookings + deliveries)
	CreateOrder(ctx context.Context, o models.Order) error
	UpdateOrder(ctx context.Context, o models.Order) error
	OrderByID(ctx context.Context, id string) (models.Order, error)
	ListOrdersByStudent(ctx context.Context, studentID string) ([]models.Order, error)
	ListDeliveries(ctx context.Context) ([]models.Order, error)
	CountDailyOrders(ctx context.Context, day time.Time) (int, error)

	// Notifications
	UpsertTemplate(ctx context.Context, t models.NotificationTemplate) error
	TemplateByID(ctx context.Context, id string) (models.NotificationTemplate, error)
	ListTemplates(ctx context.Context) ([]models.NotificationTemplate, error)
	SetSubscription(ctx context.Context, sub models.Subscription) error
	Subscription(ctx context.Context, userID, category string) (models.Subscription, error)
	RecordDeliveryAttempt(ctx context.Context, a models.DeliveryAttempt) error
	CountAttemptsForOrderOn(ctx context.Context, orderID string, day time.Time) (int, error)
	AttemptsByOrder(ctx context.Context, orderID string) ([]models.DeliveryAttempt, error)

	// Audit
	AppendAudit(ctx context.Context, e models.AuditEntry) (models.AuditEntry, error)
	SearchAudit(ctx context.Context, filter AuditFilter) ([]models.AuditEntry, error)
	LatestAudit(ctx context.Context) (models.AuditEntry, error)

	// Devices
	UpsertDevice(ctx context.Context, d models.Device) error
	ListDevices(ctx context.Context) ([]models.Device, error)
}

// AuditFilter narrows audit log searches.
type AuditFilter struct {
	Actor    string
	Resource string
	From     time.Time
	To       time.Time
	Limit    int
}
