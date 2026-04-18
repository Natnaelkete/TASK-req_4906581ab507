package store

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/eaglepoint/harborclass/internal/models"
)

// Memory is an in-memory implementation of Store. It is a full
// implementation (not a mock): every method maintains real state and
// respects the same invariants as PostgreSQL. Integration tests rely
// on it to exercise the full service and handler stack offline.
type Memory struct {
	mu         sync.RWMutex
	users      map[string]models.User
	facilities map[string]models.Facility
	sessions   map[string]models.Session
	orders     map[string]models.Order
	templates  map[string]models.NotificationTemplate
	subs       map[string]models.Subscription
	attempts   []models.DeliveryAttempt
	audit      []models.AuditEntry
	devices    map[string]models.Device
	perms      map[string]models.Permission // key = orgID|action
	content    map[string]models.ContentItem
}

// NewMemory creates an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{
		users:      map[string]models.User{},
		facilities: map[string]models.Facility{},
		sessions:   map[string]models.Session{},
		orders:     map[string]models.Order{},
		templates:  map[string]models.NotificationTemplate{},
		subs:       map[string]models.Subscription{},
		devices:    map[string]models.Device{},
		perms:      map[string]models.Permission{},
		content:    map[string]models.ContentItem{},
	}
}

// CreateUser persists a new user row keyed by username.
func (m *Memory) CreateUser(_ context.Context, u models.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[u.Username]; ok {
		return ErrConflict
	}
	m.users[u.Username] = u
	return nil
}

// UserByUsername looks up a user by their unique username.
func (m *Memory) UserByUsername(_ context.Context, username string) (models.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.users[username]
	if !ok {
		return models.User{}, ErrNotFound
	}
	return u, nil
}

// UserByID performs a linear scan because ids are user-supplied uuids.
func (m *Memory) UserByID(_ context.Context, id string) (models.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return models.User{}, ErrNotFound
}

// ListUsersByRole returns users matching the requested role.
func (m *Memory) ListUsersByRole(_ context.Context, role models.Role) ([]models.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []models.User{}
	for _, u := range m.users {
		if u.Role == role {
			out = append(out, u)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Username < out[j].Username })
	return out, nil
}

// UpdateUser overwrites the row for the given username.
func (m *Memory) UpdateUser(_ context.Context, u models.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[u.Username]; !ok {
		return ErrNotFound
	}
	m.users[u.Username] = u
	return nil
}

// UpsertFacility inserts or replaces a facility row.
func (m *Memory) UpsertFacility(_ context.Context, f models.Facility) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.facilities[f.ID] = f
	return nil
}

// FacilityByID fetches a facility by id.
func (m *Memory) FacilityByID(_ context.Context, id string) (models.Facility, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	f, ok := m.facilities[id]
	if !ok {
		return models.Facility{}, ErrNotFound
	}
	return f, nil
}

// CreateSession stores a bookable session.
func (m *Memory) CreateSession(_ context.Context, s models.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.ID] = s
	return nil
}

// ListSessions returns all sessions sorted by start time.
func (m *Memory) ListSessions(_ context.Context) ([]models.Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]models.Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartsAt.Before(out[j].StartsAt) })
	return out, nil
}

// SessionByID returns the session with the given id.
func (m *Memory) SessionByID(_ context.Context, id string) (models.Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	if !ok {
		return models.Session{}, ErrNotFound
	}
	return s, nil
}

// IncrementSessionBookings adjusts the booked size atomically.
func (m *Memory) IncrementSessionBookings(_ context.Context, id string, delta int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return ErrNotFound
	}
	if s.BookedSize+delta > s.Capacity {
		return ErrConflict
	}
	s.BookedSize += delta
	m.sessions[id] = s
	return nil
}

// CreateOrder adds a new order row.
func (m *Memory) CreateOrder(_ context.Context, o models.Order) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.orders[o.ID]; ok {
		return ErrConflict
	}
	m.orders[o.ID] = o
	return nil
}

// UpdateOrder overwrites an existing order row.
func (m *Memory) UpdateOrder(_ context.Context, o models.Order) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.orders[o.ID]; !ok {
		return ErrNotFound
	}
	m.orders[o.ID] = o
	return nil
}

// OrderByID fetches an order by its uuid id.
func (m *Memory) OrderByID(_ context.Context, id string) (models.Order, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	o, ok := m.orders[id]
	if !ok {
		return models.Order{}, ErrNotFound
	}
	return o, nil
}

// ListOrdersByStudent returns orders owned by a specific student.
func (m *Memory) ListOrdersByStudent(_ context.Context, studentID string) ([]models.Order, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []models.Order{}
	for _, o := range m.orders {
		if o.StudentID == studentID {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// ListDeliveries returns all delivery-type orders.
func (m *Memory) ListDeliveries(_ context.Context) ([]models.Order, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []models.Order{}
	for _, o := range m.orders {
		if o.Kind == models.OrderDelivery {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// CountDailyOrders counts orders created on the given calendar day.
func (m *Memory) CountDailyOrders(_ context.Context, day time.Time) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	y, mo, d := day.Date()
	n := 0
	for _, o := range m.orders {
		oy, om, od := o.CreatedAt.Date()
		if y == oy && mo == om && d == od {
			n++
		}
	}
	return n, nil
}

// UpsertTemplate stores a reusable notification template.
func (m *Memory) UpsertTemplate(_ context.Context, t models.NotificationTemplate) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.templates[t.ID] = t
	return nil
}

// TemplateByID fetches a notification template.
func (m *Memory) TemplateByID(_ context.Context, id string) (models.NotificationTemplate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.templates[id]
	if !ok {
		return models.NotificationTemplate{}, ErrNotFound
	}
	return t, nil
}

// ListTemplates lists all notification templates.
func (m *Memory) ListTemplates(_ context.Context) ([]models.NotificationTemplate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]models.NotificationTemplate, 0, len(m.templates))
	for _, t := range m.templates {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func subKey(userID, category string) string { return userID + "|" + category }

// SetSubscription stores the per-user, per-category subscription flag
// used to enforce one-click unsubscribe semantics.
func (m *Memory) SetSubscription(_ context.Context, sub models.Subscription) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subs[subKey(sub.UserID, sub.Category)] = sub
	return nil
}

// Subscription returns the stored subscription row (default: subscribed).
func (m *Memory) Subscription(_ context.Context, userID, category string) (models.Subscription, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.subs[subKey(userID, category)]
	if !ok {
		return models.Subscription{UserID: userID, Category: category, Subscribed: true}, nil
	}
	return s, nil
}

// RecordDeliveryAttempt appends a delivery attempt to the audit stream.
func (m *Memory) RecordDeliveryAttempt(_ context.Context, a models.DeliveryAttempt) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.attempts = append(m.attempts, a)
	return nil
}

// CountAttemptsForOrderOn returns the number of attempts for a given
// order on the given calendar day; used for the 3-per-day rate limit.
func (m *Memory) CountAttemptsForOrderOn(_ context.Context, orderID string, day time.Time) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	y, mo, d := day.Date()
	n := 0
	for _, a := range m.attempts {
		ay, am, ad := a.SentAt.Date()
		if a.OrderID == orderID && y == ay && mo == am && d == ad {
			n++
		}
	}
	return n, nil
}

// AttemptsByOrder returns every attempt for an order, used for retry
// decisions and admin visibility.
func (m *Memory) AttemptsByOrder(_ context.Context, orderID string) ([]models.DeliveryAttempt, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []models.DeliveryAttempt{}
	for _, a := range m.attempts {
		if a.OrderID == orderID {
			out = append(out, a)
		}
	}
	return out, nil
}

// AppendAudit appends a new audit entry.
func (m *Memory) AppendAudit(_ context.Context, e models.AuditEntry) (models.AuditEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.audit = append(m.audit, e)
	return e, nil
}

// SearchAudit filters the audit log by org/actor/resource/time range.
// When OrgID is set, cross-tenant rows are excluded; rows without an
// OrgID (system-wide events) remain visible to every admin.
func (m *Memory) SearchAudit(_ context.Context, filter AuditFilter) ([]models.AuditEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []models.AuditEntry{}
	for _, e := range m.audit {
		if filter.OrgID != "" && e.OrgID != "" && e.OrgID != filter.OrgID {
			continue
		}
		if filter.Actor != "" && !strings.EqualFold(e.Actor, filter.Actor) {
			continue
		}
		if filter.Resource != "" && !strings.EqualFold(e.Resource, filter.Resource) {
			continue
		}
		if !filter.From.IsZero() && e.At.Before(filter.From) {
			continue
		}
		if !filter.To.IsZero() && e.At.After(filter.To) {
			continue
		}
		out = append(out, e)
	}
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

// LatestAudit returns the last appended audit entry, or ErrNotFound if
// the chain is empty.
func (m *Memory) LatestAudit(_ context.Context) (models.AuditEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.audit) == 0 {
		return models.AuditEntry{}, ErrNotFound
	}
	return m.audit[len(m.audit)-1], nil
}

// UpsertDevice inserts or updates a device registration.
func (m *Memory) UpsertDevice(_ context.Context, d models.Device) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.devices[d.ID] = d
	return nil
}

// ListDevices returns all registered devices.
func (m *Memory) ListDevices(_ context.Context) ([]models.Device, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]models.Device, 0, len(m.devices))
	for _, d := range m.devices {
		out = append(out, d)
	}
	return out, nil
}

// ListDeliveriesByOrg scopes deliveries to a single organisation for
// object-level authorisation on the dispatcher queue.
func (m *Memory) ListDeliveriesByOrg(_ context.Context, orgID string) ([]models.Order, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []models.Order{}
	for _, o := range m.orders {
		if o.Kind != models.OrderDelivery {
			continue
		}
		if orgID != "" && o.OrgID != orgID {
			continue
		}
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func permKey(orgID, action string) string { return orgID + "|" + action }

// UpsertPermission stores or replaces the admin-configurable role list
// for a single (org, action) pair.
func (m *Memory) UpsertPermission(_ context.Context, p models.Permission) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.perms[permKey(p.OrgID, p.Action)] = p
	return nil
}

// ListPermissions returns the full permission overlay for an org.
func (m *Memory) ListPermissions(_ context.Context, orgID string) ([]models.Permission, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []models.Permission{}
	for _, p := range m.perms {
		if p.OrgID == orgID {
			out = append(out, p)
		}
	}
	return out, nil
}

// UpsertContent inserts or replaces a teacher content item.
func (m *Memory) UpsertContent(_ context.Context, c models.ContentItem) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.content[c.ID] = c
	return nil
}

// DeleteContent removes a content item by id.
func (m *Memory) DeleteContent(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.content, id)
	return nil
}

// ContentByID fetches a content item.
func (m *Memory) ContentByID(_ context.Context, id string) (models.ContentItem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.content[id]
	if !ok {
		return models.ContentItem{}, ErrNotFound
	}
	return c, nil
}

// ListContentByTeacher returns every content item authored by a teacher
// in reverse-chronological order.
func (m *Memory) ListContentByTeacher(_ context.Context, teacherID string) ([]models.ContentItem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []models.ContentItem{}
	for _, c := range m.content {
		if c.TeacherID == teacherID {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
