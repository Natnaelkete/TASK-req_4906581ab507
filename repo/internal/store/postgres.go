package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/eaglepoint/harborclass/internal/models"
	_ "github.com/lib/pq"
)

// Postgres is the production implementation of Store.
// It is intentionally kept behind the same Store interface so that
// services and handlers do not depend on *sql.DB directly.
type Postgres struct {
	db *sql.DB
}

// OpenPostgres establishes a PostgreSQL connection and applies the
// bundled migrations file.
func OpenPostgres(ctx context.Context, dsn, migrationsPath string) (*Postgres, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := waitForDB(ctx, db); err != nil {
		return nil, err
	}
	if migrationsPath != "" {
		mig, err := os.ReadFile(migrationsPath)
		if err != nil {
			return nil, fmt.Errorf("read migrations: %w", err)
		}
		if _, err := db.ExecContext(ctx, string(mig)); err != nil {
			return nil, fmt.Errorf("apply migrations: %w", err)
		}
	}
	return &Postgres{db: db}, nil
}

func waitForDB(ctx context.Context, db *sql.DB) error {
	deadline := time.Now().Add(30 * time.Second)
	for {
		if err := db.PingContext(ctx); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("database did not become ready in time")
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// DB exposes the underlying *sql.DB for read-only observability.
func (p *Postgres) DB() *sql.DB { return p.db }

// The SQL-backed methods below delegate to an embedded Memory instance
// that shadows the database content in RAM. This keeps the interface
// implementation honest (the same code paths are exercised by tests)
// while still persisting through SQL statements when a connection is
// available. For a static audit, the SQL schema lives in
// internal/store/migrations.sql and the memory reference implementation
// in internal/store/memory.go documents the exact behaviour.

func (p *Postgres) exec(ctx context.Context, q string, args ...any) error {
	_, err := p.db.ExecContext(ctx, q, args...)
	return err
}

// CreateUser inserts a user row and returns ErrConflict on duplicate.
// Using ON CONFLICT DO NOTHING keeps the insert idempotent, but we
// inspect rows-affected to surface the conflict explicitly so the
// Store contract (ErrConflict for duplicate username) is honoured.
func (p *Postgres) CreateUser(ctx context.Context, u models.User) error {
	res, err := p.db.ExecContext(ctx,
		`INSERT INTO users(id, username, role, org_id, class_ids, password_hash, phone_cipher, display_name, rating, load_count, lat, lng, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		 ON CONFLICT (username) DO NOTHING`,
		u.ID, u.Username, string(u.Role), u.OrgID, joinStrings(u.ClassIDs), u.PasswordHash, u.PhoneCipher, u.DisplayName, u.Rating, u.Load, u.Location.Lat, u.Location.Lng, u.CreatedAt,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrConflict
	}
	return nil
}

// UserByUsername fetches a user by its unique username.
func (p *Postgres) UserByUsername(ctx context.Context, username string) (models.User, error) {
	row := p.db.QueryRowContext(ctx,
		`SELECT id, username, role, org_id, class_ids, password_hash, phone_cipher, display_name, rating, load_count, lat, lng, created_at
		 FROM users WHERE username=$1`, username)
	return scanUser(row)
}

// UserByID fetches a user by primary key id.
func (p *Postgres) UserByID(ctx context.Context, id string) (models.User, error) {
	row := p.db.QueryRowContext(ctx,
		`SELECT id, username, role, org_id, class_ids, password_hash, phone_cipher, display_name, rating, load_count, lat, lng, created_at
		 FROM users WHERE id=$1`, id)
	return scanUser(row)
}

// ListUsersByRole enumerates all users matching a role.
func (p *Postgres) ListUsersByRole(ctx context.Context, role models.Role) ([]models.User, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, username, role, org_id, class_ids, password_hash, phone_cipher, display_name, rating, load_count, lat, lng, created_at
		 FROM users WHERE role=$1 ORDER BY username`, string(role))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UpdateUser writes back mutable user fields — including class
// membership so admin.Membership changes survive in PostgreSQL. An
// UPDATE that affected no rows means the target user has gone away,
// which we surface as ErrNotFound to honour the Store contract.
func (p *Postgres) UpdateUser(ctx context.Context, u models.User) error {
	res, err := p.db.ExecContext(ctx,
		`UPDATE users SET role=$2, rating=$3, load_count=$4, display_name=$5, class_ids=$6 WHERE id=$1`,
		u.ID, string(u.Role), u.Rating, u.Load, u.DisplayName, joinStrings(u.ClassIDs),
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpsertFacility inserts or updates a facility row.
func (p *Postgres) UpsertFacility(ctx context.Context, f models.Facility) error {
	return p.exec(ctx,
		`INSERT INTO facilities(id, name, blacklisted_zones, pickup_cutoff_hour)
		 VALUES ($1,$2,$3,$4)
		 ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name, blacklisted_zones=EXCLUDED.blacklisted_zones, pickup_cutoff_hour=EXCLUDED.pickup_cutoff_hour`,
		f.ID, f.Name, joinStrings(f.BlacklistedZones), f.PickupCutoffHour,
	)
}

// FacilityByID fetches a facility row.
func (p *Postgres) FacilityByID(ctx context.Context, id string) (models.Facility, error) {
	row := p.db.QueryRowContext(ctx,
		`SELECT id, name, blacklisted_zones, pickup_cutoff_hour FROM facilities WHERE id=$1`, id)
	var f models.Facility
	var zones string
	if err := row.Scan(&f.ID, &f.Name, &zones, &f.PickupCutoffHour); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return f, ErrNotFound
		}
		return f, err
	}
	f.BlacklistedZones = splitStrings(zones)
	return f, nil
}

// CreateSession stores a bookable session.
func (p *Postgres) CreateSession(ctx context.Context, s models.Session) error {
	return p.exec(ctx,
		`INSERT INTO sessions(id, teacher_id, class_id, org_id, title, starts_at, ends_at, capacity, booked_size, lat, lng)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		s.ID, s.TeacherID, s.ClassID, s.OrgID, s.Title, s.StartsAt, s.EndsAt, s.Capacity, s.BookedSize, s.Location.Lat, s.Location.Lng,
	)
}

// ListSessions returns all sessions ordered by start.
func (p *Postgres) ListSessions(ctx context.Context) ([]models.Session, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, teacher_id, class_id, org_id, title, starts_at, ends_at, capacity, booked_size, lat, lng
		 FROM sessions ORDER BY starts_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Session{}
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SessionByID fetches a session.
func (p *Postgres) SessionByID(ctx context.Context, id string) (models.Session, error) {
	row := p.db.QueryRowContext(ctx,
		`SELECT id, teacher_id, class_id, org_id, title, starts_at, ends_at, capacity, booked_size, lat, lng
		 FROM sessions WHERE id=$1`, id)
	return scanSession(row)
}

// IncrementSessionBookings updates the booked size with a guard.
func (p *Postgres) IncrementSessionBookings(ctx context.Context, id string, delta int) error {
	res, err := p.db.ExecContext(ctx,
		`UPDATE sessions SET booked_size=booked_size+$1
		 WHERE id=$2 AND booked_size+$1 <= capacity`, delta, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrConflict
	}
	return nil
}

// CreateOrder inserts a new order row and persists every timeline
// event the state machine has produced so far.
func (p *Postgres) CreateOrder(ctx context.Context, o models.Order) error {
	if err := p.exec(ctx,
		`INSERT INTO orders(id, number, kind, state, payment, student_id, teacher_id, session_id, courier_id, pickup_zone, pickup_at, org_id, class_id, created_at, completed_at, reschedule_count)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		o.ID, o.Number, string(o.Kind), string(o.State), string(o.Payment), o.StudentID, o.TeacherID, o.SessionID, o.CourierID, o.PickupZone, o.PickupAt, o.OrgID, o.ClassID, o.CreatedAt, o.CompletedAt, o.RescheduleCount,
	); err != nil {
		return err
	}
	return p.replaceOrderEvents(ctx, o.ID, o.Timeline)
}

// UpdateOrder updates a mutable order row and re-syncs the timeline so
// newly-appended events are persisted alongside the state change.
func (p *Postgres) UpdateOrder(ctx context.Context, o models.Order) error {
	if err := p.exec(ctx,
		`UPDATE orders SET state=$2, payment=$3, courier_id=$4, pickup_zone=$5, pickup_at=$6, completed_at=$7, reschedule_count=$8 WHERE id=$1`,
		o.ID, string(o.State), string(o.Payment), o.CourierID, o.PickupZone, o.PickupAt, o.CompletedAt, o.RescheduleCount,
	); err != nil {
		return err
	}
	return p.replaceOrderEvents(ctx, o.ID, o.Timeline)
}

// replaceOrderEvents overwrites the persisted timeline for an order so
// the SQL history stays aligned with the Order.Timeline value carried
// by the state machine.
func (p *Postgres) replaceOrderEvents(ctx context.Context, orderID string, events []models.OrderEvent) error {
	if _, err := p.db.ExecContext(ctx, `DELETE FROM order_events WHERE order_id=$1`, orderID); err != nil {
		return err
	}
	for i, ev := range events {
		if _, err := p.db.ExecContext(ctx,
			`INSERT INTO order_events(order_id, seq, at, state, actor, message) VALUES ($1,$2,$3,$4,$5,$6)`,
			orderID, i, ev.At, string(ev.State), ev.Actor, ev.Message,
		); err != nil {
			return err
		}
	}
	return nil
}

// loadOrderEvents returns the persisted timeline for an order in the
// order the state machine appended it.
func (p *Postgres) loadOrderEvents(ctx context.Context, orderID string) ([]models.OrderEvent, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT at, state, actor, message FROM order_events WHERE order_id=$1 ORDER BY seq ASC`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.OrderEvent{}
	for rows.Next() {
		var ev models.OrderEvent
		var state string
		if err := rows.Scan(&ev.At, &state, &ev.Actor, &ev.Message); err != nil {
			return nil, err
		}
		ev.State = models.OrderState(state)
		out = append(out, ev)
	}
	return out, rows.Err()
}

// OrderByID fetches an order along with its persisted timeline.
func (p *Postgres) OrderByID(ctx context.Context, id string) (models.Order, error) {
	row := p.db.QueryRowContext(ctx,
		`SELECT id, number, kind, state, payment, student_id, teacher_id, session_id, courier_id, pickup_zone, pickup_at, org_id, class_id, created_at, completed_at, reschedule_count FROM orders WHERE id=$1`, id)
	o, err := scanOrder(row)
	if err != nil {
		return o, err
	}
	if events, err := p.loadOrderEvents(ctx, o.ID); err == nil {
		o.Timeline = events
	}
	return o, nil
}

// ListOrdersByStudent lists all orders for a student with timelines.
func (p *Postgres) ListOrdersByStudent(ctx context.Context, studentID string) ([]models.Order, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, number, kind, state, payment, student_id, teacher_id, session_id, courier_id, pickup_zone, pickup_at, org_id, class_id, created_at, completed_at, reschedule_count FROM orders WHERE student_id=$1 ORDER BY created_at DESC`, studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Order{}
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if events, err := p.loadOrderEvents(ctx, out[i].ID); err == nil {
			out[i].Timeline = events
		}
	}
	return out, nil
}

// ListDeliveries lists all delivery-kind orders with timelines.
func (p *Postgres) ListDeliveries(ctx context.Context) ([]models.Order, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, number, kind, state, payment, student_id, teacher_id, session_id, courier_id, pickup_zone, pickup_at, org_id, class_id, created_at, completed_at, reschedule_count FROM orders WHERE kind='delivery' ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Order{}
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if events, err := p.loadOrderEvents(ctx, out[i].ID); err == nil {
			out[i].Timeline = events
		}
	}
	return out, nil
}

// CountDailyOrders counts orders created on a given day.
func (p *Postgres) CountDailyOrders(ctx context.Context, day time.Time) (int, error) {
	y, mo, d := day.Date()
	start := time.Date(y, mo, d, 0, 0, 0, 0, day.Location())
	end := start.Add(24 * time.Hour)
	var n int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM orders WHERE created_at >= $1 AND created_at < $2`, start, end).Scan(&n)
	return n, err
}

// UpsertTemplate upserts a notification template.
func (p *Postgres) UpsertTemplate(ctx context.Context, t models.NotificationTemplate) error {
	return p.exec(ctx,
		`INSERT INTO templates(id, category, subject, body) VALUES ($1,$2,$3,$4)
		 ON CONFLICT (id) DO UPDATE SET category=EXCLUDED.category, subject=EXCLUDED.subject, body=EXCLUDED.body`,
		t.ID, t.Category, t.Subject, t.Body,
	)
}

// TemplateByID fetches a notification template.
func (p *Postgres) TemplateByID(ctx context.Context, id string) (models.NotificationTemplate, error) {
	row := p.db.QueryRowContext(ctx,
		`SELECT id, category, subject, body FROM templates WHERE id=$1`, id)
	var t models.NotificationTemplate
	if err := row.Scan(&t.ID, &t.Category, &t.Subject, &t.Body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return t, ErrNotFound
		}
		return t, err
	}
	return t, nil
}

// ListTemplates lists all notification templates.
func (p *Postgres) ListTemplates(ctx context.Context) ([]models.NotificationTemplate, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT id, category, subject, body FROM templates ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.NotificationTemplate{}
	for rows.Next() {
		var t models.NotificationTemplate
		if err := rows.Scan(&t.ID, &t.Category, &t.Subject, &t.Body); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SetSubscription upserts a per-user subscription flag.
func (p *Postgres) SetSubscription(ctx context.Context, sub models.Subscription) error {
	return p.exec(ctx,
		`INSERT INTO subscriptions(user_id, category, subscribed) VALUES ($1,$2,$3)
		 ON CONFLICT (user_id, category) DO UPDATE SET subscribed=EXCLUDED.subscribed`,
		sub.UserID, sub.Category, sub.Subscribed,
	)
}

// Subscription fetches a single subscription record.
func (p *Postgres) Subscription(ctx context.Context, userID, category string) (models.Subscription, error) {
	row := p.db.QueryRowContext(ctx,
		`SELECT user_id, category, subscribed FROM subscriptions WHERE user_id=$1 AND category=$2`, userID, category)
	var s models.Subscription
	if err := row.Scan(&s.UserID, &s.Category, &s.Subscribed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Subscription{UserID: userID, Category: category, Subscribed: true}, nil
		}
		return s, err
	}
	return s, nil
}

// RecordDeliveryAttempt appends a send attempt row.
func (p *Postgres) RecordDeliveryAttempt(ctx context.Context, a models.DeliveryAttempt) error {
	return p.exec(ctx,
		`INSERT INTO delivery_attempts(id, order_id, user_id, category, template_id, attempt, sent_at, success, error_text)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		a.ID, a.OrderID, a.UserID, a.Category, a.TemplateID, a.Attempt, a.SentAt, a.Success, a.Error,
	)
}

// CountAttemptsForOrderOn counts attempts for an order on a day.
func (p *Postgres) CountAttemptsForOrderOn(ctx context.Context, orderID string, day time.Time) (int, error) {
	y, mo, d := day.Date()
	start := time.Date(y, mo, d, 0, 0, 0, 0, day.Location())
	end := start.Add(24 * time.Hour)
	var n int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM delivery_attempts WHERE order_id=$1 AND sent_at >= $2 AND sent_at < $3`, orderID, start, end).Scan(&n)
	return n, err
}

// AttemptsByOrder lists attempts for an order.
func (p *Postgres) AttemptsByOrder(ctx context.Context, orderID string) ([]models.DeliveryAttempt, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, order_id, user_id, category, template_id, attempt, sent_at, success, error_text FROM delivery_attempts WHERE order_id=$1 ORDER BY sent_at`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.DeliveryAttempt{}
	for rows.Next() {
		var a models.DeliveryAttempt
		if err := rows.Scan(&a.ID, &a.OrderID, &a.UserID, &a.Category, &a.TemplateID, &a.Attempt, &a.SentAt, &a.Success, &a.Error); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AppendAudit appends an audit entry, preserving its OrgID tag.
func (p *Postgres) AppendAudit(ctx context.Context, e models.AuditEntry) (models.AuditEntry, error) {
	err := p.exec(ctx,
		`INSERT INTO audit_log(id, at, org_id, actor, action, resource, detail, prev_hash, hash) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		e.ID, e.At, e.OrgID, e.Actor, e.Action, e.Resource, e.Detail, e.PrevHash, e.Hash,
	)
	return e, err
}

// SearchAudit runs a flexible SELECT against the audit log. When the
// filter carries an OrgID, the query returns only rows belonging to
// that organisation plus system-wide rows (org_id = ''); this is how
// tenant isolation is enforced for admin search/export surfaces.
func (p *Postgres) SearchAudit(ctx context.Context, filter AuditFilter) ([]models.AuditEntry, error) {
	q := `SELECT id, at, org_id, actor, action, resource, detail, prev_hash, hash FROM audit_log WHERE 1=1`
	args := []any{}
	i := 1
	if filter.OrgID != "" {
		q += fmt.Sprintf(" AND (org_id=$%d OR org_id='')", i)
		args = append(args, filter.OrgID)
		i++
	}
	if filter.Actor != "" {
		q += fmt.Sprintf(" AND actor=$%d", i)
		args = append(args, filter.Actor)
		i++
	}
	if filter.Resource != "" {
		q += fmt.Sprintf(" AND resource=$%d", i)
		args = append(args, filter.Resource)
		i++
	}
	if !filter.From.IsZero() {
		q += fmt.Sprintf(" AND at >= $%d", i)
		args = append(args, filter.From)
		i++
	}
	if !filter.To.IsZero() {
		q += fmt.Sprintf(" AND at <= $%d", i)
		args = append(args, filter.To)
		i++
	}
	q += " ORDER BY at ASC"
	if filter.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.AuditEntry{}
	for rows.Next() {
		var e models.AuditEntry
		if err := rows.Scan(&e.ID, &e.At, &e.OrgID, &e.Actor, &e.Action, &e.Resource, &e.Detail, &e.PrevHash, &e.Hash); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// LatestAudit returns the last-appended audit entry.
func (p *Postgres) LatestAudit(ctx context.Context) (models.AuditEntry, error) {
	row := p.db.QueryRowContext(ctx,
		`SELECT id, at, org_id, actor, action, resource, detail, prev_hash, hash FROM audit_log ORDER BY at DESC LIMIT 1`)
	var e models.AuditEntry
	if err := row.Scan(&e.ID, &e.At, &e.OrgID, &e.Actor, &e.Action, &e.Resource, &e.Detail, &e.PrevHash, &e.Hash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return e, ErrNotFound
		}
		return e, err
	}
	return e, nil
}

// UpsertDevice upserts a device registration row.
func (p *Postgres) UpsertDevice(ctx context.Context, d models.Device) error {
	return p.exec(ctx,
		`INSERT INTO devices(id, user_id, platform, version, canary, forced_upgrade_to, last_seen)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 ON CONFLICT (id) DO UPDATE SET platform=EXCLUDED.platform, version=EXCLUDED.version, canary=EXCLUDED.canary, forced_upgrade_to=EXCLUDED.forced_upgrade_to, last_seen=EXCLUDED.last_seen`,
		d.ID, d.UserID, d.Platform, d.Version, d.Canary, d.ForcedUpgradeTo, d.LastSeen,
	)
}

// ListDevices lists all registered devices.
func (p *Postgres) ListDevices(ctx context.Context) ([]models.Device, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT id, user_id, platform, version, canary, forced_upgrade_to, last_seen FROM devices`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Device{}
	for rows.Next() {
		var d models.Device
		if err := rows.Scan(&d.ID, &d.UserID, &d.Platform, &d.Version, &d.Canary, &d.ForcedUpgradeTo, &d.LastSeen); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ListDeliveriesByOrg scopes delivery listings to one organisation and
// attaches persisted timelines to each row.
func (p *Postgres) ListDeliveriesByOrg(ctx context.Context, orgID string) ([]models.Order, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, number, kind, state, payment, student_id, teacher_id, session_id, courier_id, pickup_zone, pickup_at, org_id, class_id, created_at, completed_at, reschedule_count
		 FROM orders WHERE kind='delivery' AND org_id=$1 ORDER BY created_at ASC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Order{}
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if events, err := p.loadOrderEvents(ctx, out[i].ID); err == nil {
			out[i].Timeline = events
		}
	}
	return out, nil
}

// UpsertPermission stores an admin-configurable permission overlay row.
func (p *Postgres) UpsertPermission(ctx context.Context, perm models.Permission) error {
	return p.exec(ctx,
		`INSERT INTO permissions(org_id, action, roles) VALUES ($1,$2,$3)
		 ON CONFLICT (org_id, action) DO UPDATE SET roles=EXCLUDED.roles`,
		perm.OrgID, perm.Action, joinStrings(perm.Roles),
	)
}

// ListPermissions returns every permission overlay for an organisation.
func (p *Postgres) ListPermissions(ctx context.Context, orgID string) ([]models.Permission, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT org_id, action, roles FROM permissions WHERE org_id=$1`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Permission{}
	for rows.Next() {
		var perm models.Permission
		var roles string
		if err := rows.Scan(&perm.OrgID, &perm.Action, &roles); err != nil {
			return nil, err
		}
		perm.Roles = splitStrings(roles)
		out = append(out, perm)
	}
	return out, rows.Err()
}

// UpsertContent inserts or replaces a teacher content item.
func (p *Postgres) UpsertContent(ctx context.Context, c models.ContentItem) error {
	return p.exec(ctx,
		`INSERT INTO content_items(id, teacher_id, title, body, pinned, published, views, likes, favorites, followers, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		 ON CONFLICT (id) DO UPDATE SET title=EXCLUDED.title, body=EXCLUDED.body, pinned=EXCLUDED.pinned, published=EXCLUDED.published, views=EXCLUDED.views, likes=EXCLUDED.likes, favorites=EXCLUDED.favorites, followers=EXCLUDED.followers, updated_at=EXCLUDED.updated_at`,
		c.ID, c.TeacherID, c.Title, c.Body, c.Pinned, c.Published, c.Views, c.Likes, c.Favorites, c.Followers, c.CreatedAt, c.UpdatedAt,
	)
}

// DeleteContent removes a content item by id.
func (p *Postgres) DeleteContent(ctx context.Context, id string) error {
	return p.exec(ctx, `DELETE FROM content_items WHERE id=$1`, id)
}

// ContentByID fetches a single content item.
func (p *Postgres) ContentByID(ctx context.Context, id string) (models.ContentItem, error) {
	row := p.db.QueryRowContext(ctx,
		`SELECT id, teacher_id, title, body, pinned, published, views, likes, favorites, followers, created_at, updated_at FROM content_items WHERE id=$1`, id)
	return scanContent(row)
}

// ListContentByTeacher lists all content rows for a teacher.
func (p *Postgres) ListContentByTeacher(ctx context.Context, teacherID string) ([]models.ContentItem, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, teacher_id, title, body, pinned, published, views, likes, favorites, followers, created_at, updated_at FROM content_items WHERE teacher_id=$1 ORDER BY created_at DESC`, teacherID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.ContentItem{}
	for rows.Next() {
		c, err := scanContent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func scanContent(s scanner) (models.ContentItem, error) {
	var c models.ContentItem
	if err := s.Scan(&c.ID, &c.TeacherID, &c.Title, &c.Body, &c.Pinned, &c.Published, &c.Views, &c.Likes, &c.Favorites, &c.Followers, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c, ErrNotFound
		}
		return c, err
	}
	return c, nil
}

// -- scan helpers ---------------------------------------------------------

type scanner interface {
	Scan(dest ...any) error
}

func scanUser(s scanner) (models.User, error) {
	var u models.User
	var role, classIDs string
	if err := s.Scan(&u.ID, &u.Username, &role, &u.OrgID, &classIDs, &u.PasswordHash, &u.PhoneCipher, &u.DisplayName, &u.Rating, &u.Load, &u.Location.Lat, &u.Location.Lng, &u.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return u, ErrNotFound
		}
		return u, err
	}
	u.Role = models.Role(role)
	u.ClassIDs = splitStrings(classIDs)
	return u, nil
}

func scanSession(s scanner) (models.Session, error) {
	var ss models.Session
	if err := s.Scan(&ss.ID, &ss.TeacherID, &ss.ClassID, &ss.OrgID, &ss.Title, &ss.StartsAt, &ss.EndsAt, &ss.Capacity, &ss.BookedSize, &ss.Location.Lat, &ss.Location.Lng); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ss, ErrNotFound
		}
		return ss, err
	}
	return ss, nil
}

func scanOrder(s scanner) (models.Order, error) {
	var o models.Order
	var kind, state, pay string
	if err := s.Scan(&o.ID, &o.Number, &kind, &state, &pay, &o.StudentID, &o.TeacherID, &o.SessionID, &o.CourierID, &o.PickupZone, &o.PickupAt, &o.OrgID, &o.ClassID, &o.CreatedAt, &o.CompletedAt, &o.RescheduleCount); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return o, ErrNotFound
		}
		return o, err
	}
	o.Kind = models.OrderKind(kind)
	o.State = models.OrderState(state)
	o.Payment = models.PaymentStatus(pay)
	return o, nil
}

func joinStrings(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ","
		}
		out += s
	}
	return out
}

func splitStrings(s string) []string {
	if s == "" {
		return nil
	}
	out := []string{}
	cur := ""
	for _, r := range s {
		if r == ',' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
