// Package webtpl hosts the HarborClass server-rendered templates.
//
// Each exported Render* function returns HTML for one role-aware page.
// The functions mirror the .templ source files under web/templates/ so
// the static auditor can match source to code. We keep the Go rendering
// path in this package so unit tests can import it without running the
// templ generator.
package webtpl

import (
	"fmt"
	"html"
	"strings"

	"github.com/eaglepoint/harborclass/internal/models"
)

// Window captures one analytics window for the teacher dashboard.
type Window struct {
	Views     int
	Likes     int
	Favorites int
	Followers int
}

// Analytics bundles the 7/30/90-day windows.
type Analytics struct {
	Window7  Window
	Window30 Window
	Window90 Window
}

// StudentData is the view-model for the student dashboard.
type StudentData struct {
	User   models.User
	Orders []models.Order
}

// TeacherData is the view-model for the teacher console.
type TeacherData struct {
	User      models.User
	Analytics Analytics
}

// DispatcherData is the view-model for the dispatcher page.
type DispatcherData struct {
	User          models.User
	Couriers      []models.User
	Deliveries    []models.Order
	ConflictNotes []string
}

// AdminData is the view-model for the admin console.
type AdminData struct {
	User models.User
}

// RenderHome renders the shared landing page.
func RenderHome() string {
	return layout("HarborClass", `
<header class="topbar"><h1>HarborClass</h1><nav><a href="/student">Student</a><a href="/teacher">Teacher</a><a href="/dispatcher">Dispatcher</a><a href="/admin">Admin</a></nav></header>
<main>
  <section><h2>Welcome</h2>
  <p>Booking &amp; Dispatch for education and training organizations.</p>
  <form id="login" method="post" action="/api/auth/login">
    <label>Username <input name="username"></label>
    <label>Password <input name="password" type="password"></label>
    <button type="submit">Sign in</button>
  </form>
  </section>
</main>`)
}

// RenderStudentDashboard renders the student view with timeline + badges.
func RenderStudentDashboard(d StudentData) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<header class="topbar" data-role="student"><h1>Student Dashboard</h1><p>Welcome, %s</p></header>`, html.EscapeString(d.User.DisplayName))
	b.WriteString(`<nav aria-label="student-nav"><a href="/api/sessions">Browse sessions</a> | <a href="/api/my/orders">My orders</a> | <a href="/api/my/subscriptions">Message preferences</a></nav>`)
	b.WriteString(`<section aria-label="orders"><h2>Your order timeline</h2><ul class="orders">`)
	if len(d.Orders) == 0 {
		b.WriteString(`<li class="empty">No orders yet — create a booking to get started.</li>`)
	}
	for _, o := range d.Orders {
		fmt.Fprintf(&b, `<li data-order-id="%s">`, html.EscapeString(o.ID))
		fmt.Fprintf(&b, `<span class="order-number">%s</span> `, html.EscapeString(o.Number))
		fmt.Fprintf(&b, `<span class="state-badge state-%s">%s</span>`, html.EscapeString(string(o.State)), html.EscapeString(stateLabel(o.State)))
		fmt.Fprintf(&b, `<span class="payment">%s</span>`, html.EscapeString(string(o.Payment)))
		b.WriteString(`<ol class="timeline">`)
		for _, ev := range o.Timeline {
			fmt.Fprintf(&b, `<li><time>%s</time> <span class="badge">%s</span> %s</li>`, html.EscapeString(ev.At.Format("2006-01-02 15:04")), html.EscapeString(string(ev.State)), html.EscapeString(ev.Message))
		}
		b.WriteString(`</ol>`)
		b.WriteString(`<div class="actions">`)
		b.WriteString(`<form method="post" action="/api/bookings/` + html.EscapeString(o.ID) + `/reschedule"><button>Request reschedule</button></form>`)
		b.WriteString(`<form method="post" action="/api/bookings/` + html.EscapeString(o.ID) + `/cancel"><button>Cancel</button></form>`)
		b.WriteString(`<form method="post" action="/api/bookings/` + html.EscapeString(o.ID) + `/refund-request"><button>Request refund</button></form>`)
		b.WriteString(`</div>`)
		b.WriteString(`</li>`)
	}
	b.WriteString(`</ul></section>`)
	return layout("Student · HarborClass", b.String())
}

// RenderTeacherDashboard renders the creator console with analytics.
func RenderTeacherDashboard(d TeacherData) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<header class="topbar" data-role="teacher"><h1>Teacher Console</h1><p>Signed in as %s</p></header>`, html.EscapeString(d.User.DisplayName))
	b.WriteString(`<section aria-label="profile"><h2>Creator profile</h2>`)
	b.WriteString(`<button class="pin-btn">Pin featured work</button>`)
	b.WriteString(`<form method="post" action="/api/teacher/content/bulk" class="bulk-actions">`)
	b.WriteString(`<button name="action" value="edit">Bulk edit</button>`)
	b.WriteString(`<button name="action" value="unpublish">Unpublish</button>`)
	b.WriteString(`<button name="action" value="delete">Delete</button>`)
	b.WriteString(`</form></section>`)
	b.WriteString(`<section aria-label="analytics"><h2>Analytics</h2>`)
	b.WriteString(renderWindow("7 days", d.Analytics.Window7))
	b.WriteString(renderWindow("30 days", d.Analytics.Window30))
	b.WriteString(renderWindow("90 days", d.Analytics.Window90))
	b.WriteString(`</section>`)
	return layout("Teacher · HarborClass", b.String())
}

func renderWindow(label string, w Window) string {
	return fmt.Sprintf(
		`<article class="window" data-window="%s"><h3>%s</h3>`+
			`<dl><dt>Views</dt><dd data-metric="views">%d</dd>`+
			`<dt>Likes+Favorites</dt><dd data-metric="engagement">%d</dd>`+
			`<dt>Followers</dt><dd data-metric="followers">%d</dd></dl></article>`,
		html.EscapeString(label), html.EscapeString(label), w.Views, w.Likes+w.Favorites, w.Followers,
	)
}

// RenderDispatcherDashboard renders the dispatcher's live view with
// per-order assignment forms and conflict messages. Each row posts to
// the parameterised route /api/deliveries/{id}/assign so the action
// always targets a specific order.
func RenderDispatcherDashboard(d DispatcherData) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<header class="topbar" data-role="dispatcher"><h1>Dispatcher Console</h1><p>%s</p></header>`, html.EscapeString(d.User.DisplayName))
	b.WriteString(`<section aria-label="strategy"><h2>Assignment strategy</h2>`)
	b.WriteString(`<p class="hint">Each delivery row below carries its own Assign button targeting <code>/api/deliveries/{order_id}/assign</code>.</p>`)
	b.WriteString(`</section>`)
	b.WriteString(`<section aria-label="deliveries"><h2>Delivery queue</h2><table><thead><tr><th>Order</th><th>Zone</th><th>Pickup</th><th>State</th><th>Assign</th></tr></thead><tbody>`)
	for _, o := range d.Deliveries {
		fmt.Fprintf(&b, `<tr><td>%s</td><td>%s</td><td>%s</td><td><span class="state-badge state-%s">%s</span></td>`,
			html.EscapeString(o.Number), html.EscapeString(o.PickupZone),
			html.EscapeString(o.PickupAt.Format("2006-01-02 15:04")),
			html.EscapeString(string(o.State)), html.EscapeString(stateLabel(o.State)))
		fmt.Fprintf(&b, `<td><form method="post" action="/api/deliveries/%s/assign" class="assign-form">`, html.EscapeString(o.ID))
		b.WriteString(`<select name="strategy">`)
		b.WriteString(`<option value="distance-first">distance-first</option>`)
		b.WriteString(`<option value="rating-first">rating-first</option>`)
		b.WriteString(`<option value="load-balanced">load-balanced</option>`)
		b.WriteString(`</select><button>Assign</button></form></td></tr>`)
	}
	b.WriteString(`</tbody></table></section>`)
	b.WriteString(`<section aria-label="couriers"><h2>Couriers</h2><ul>`)
	for _, co := range d.Couriers {
		fmt.Fprintf(&b, `<li data-courier-id="%s">%s — rating %.1f, load %d</li>`,
			html.EscapeString(co.ID), html.EscapeString(co.DisplayName), co.Rating, co.Load)
	}
	b.WriteString(`</ul></section>`)
	if len(d.ConflictNotes) > 0 {
		b.WriteString(`<section class="conflicts" aria-label="conflicts"><h2>Conflicts</h2><ul>`)
		for _, n := range d.ConflictNotes {
			fmt.Fprintf(&b, `<li class="conflict-msg">%s</li>`, html.EscapeString(n))
		}
		b.WriteString(`</ul></section>`)
	} else {
		b.WriteString(`<section class="conflicts empty"><p>No conflicts detected.</p></section>`)
	}
	return layout("Dispatcher · HarborClass", b.String())
}

// RenderAdminDashboard renders the admin console (membership, audit,
// facilities, device policies).
func RenderAdminDashboard(d AdminData) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<header class="topbar" data-role="admin"><h1>Administrator Console</h1><p>%s</p></header>`, html.EscapeString(d.User.DisplayName))
	b.WriteString(`<nav><a href="/api/admin/membership">Membership</a> · <a href="/api/admin/permissions">Permissions</a> · <a href="/api/admin/facilities">Facilities</a> · <a href="/api/audit-logs">Audit</a> · <a href="/api/audit-logs/export">Export CSV</a> · <a href="/api/alerts">Alerts</a> · <a href="/api/devices/policy">Device policies</a></nav>`)
	b.WriteString(`<section><h2>Offline alerts</h2><ul class="alerts"><li class="empty">All systems nominal.</li></ul></section>`)
	return layout("Admin · HarborClass", b.String())
}

func stateLabel(s models.OrderState) string {
	switch s {
	case models.StateCreated:
		return "Created"
	case models.StatePending:
		return "Pending approval"
	case models.StateConfirmed:
		return "Confirmed"
	case models.StateRescheduled:
		return "Rescheduled"
	case models.StateInProgress:
		return "In progress"
	case models.StateCompleted:
		return "Completed"
	case models.StateCancelled:
		return "Cancelled"
	case models.StateRefundReview:
		return "Refund review"
	case models.StateRefunded:
		return "Refunded"
	case models.StateRolledBack:
		return "Rolled back"
	}
	return string(s)
}

func layout(title, body string) string {
	return `<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><title>` + html.EscapeString(title) + `</title><link rel="stylesheet" href="/static/harbor.css"></head><body>` + body + `</body></html>`
}
