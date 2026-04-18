# HarborClass API Specification

All routes are registered in `internal/http/router.go`. The list below
is the authoritative inventory the static auditor should cross-check
against the router code and the handler files under
`internal/http/handlers/`.

Unless noted otherwise, authenticated endpoints require a bearer token
(`Authorization: Bearer <token>` or `X-Api-Token: <token>`) obtained
from `POST /api/auth/login`.

## Auth

| Method | Path | Description |
|--------|------|-------------|
| POST   | `/api/auth/login` | Exchange username/password for a bearer token. Writes `login` to the audit chain. |
| POST   | `/api/auth/logout` | Invalidate the current token. |
| GET    | `/api/auth/whoami` | Return the authenticated user's safe profile (phone masked). |

## Sessions and bookings (Student / Teacher)

| Method | Path | Description |
|--------|------|-------------|
| GET    | `/api/sessions` | List bookable sessions. Authenticated; scoped to the caller's organisation (shared catalog entries with empty `OrgID` are still visible). |
| POST   | `/api/bookings` | Create a booking for the authenticated student. Auto-confirms and reserves capacity. Rejects cross-org session ids with `403`. |
| GET    | `/api/bookings/:id` | Fetch a booking with its timeline. |
| POST   | `/api/bookings/:id/reschedule` | Reschedule a confirmed booking (max 2). |
| POST   | `/api/bookings/:id/cancel` | Cancel a booking. Inside 24h returns `202` awaiting teacher approval. |
| POST   | `/api/bookings/:id/complete` | Mark a booking completed. Restricted to the session's teacher or a same-org admin. Opens the 7-day refund window. |
| POST   | `/api/bookings/:id/refund-request` | File a refund within 7 days of completion. Moves state to `refund_review`. |
| GET    | `/api/my/orders` | List the student's own orders. |
| POST   | `/api/my/subscriptions` | Toggle a subscription category for the current user. |
| GET    | `/api/my/subscriptions/unsubscribe` | One-click unsubscribe. Public, takes `user=`, `category=`, and `token=` query params. The `token` is an HMAC-SHA256 signature over `user|category|expiry` issued with every outbound reminder; requests with a missing, forged, or expired token return `403`. |

## Teacher content console

| Method | Path | Description |
|--------|------|-------------|
| GET    | `/api/teacher/profile` | Return the creator profile and pinned works. |
| POST   | `/api/teacher/pin` | Pin a featured content item. |
| POST   | `/api/teacher/content/bulk` | Bulk edit / unpublish / delete content. |
| GET    | `/api/teacher/analytics` | 7/30/90-day views, likes+favorites, follower growth. |

## Dispatch (Dispatcher)

| Method | Path | Description |
|--------|------|-------------|
| GET    | `/api/deliveries` | List delivery-kind orders scoped to the caller's organisation. |
| POST   | `/api/deliveries` | Create a delivery order (dispatcher only). |
| POST   | `/api/deliveries/:id/assign` | Assign a courier using the configured strategy. Returns `409` with `conflict:true` for blacklists, cutoffs, and double-bookings; `503` when no eligible courier exists. |
| POST   | `/api/deliveries/:id/complete` | Mark a delivery completed. Allowed for the assigned courier or a same-org dispatcher/admin. Opens the 7-day refund window. |

## Notifications

| Method | Path | Description |
|--------|------|-------------|
| POST   | `/api/notifications/send` | Dispatch a template to a recipient. Enforces reminder cap (`429`), unsubscribe (`403`), retries/backoff. |
| GET    | `/api/notifications/templates` | List notification templates. |
| POST   | `/api/notifications/templates` | Upsert a notification template (admin). |

## Admin console

| Method | Path | Description |
|--------|------|-------------|
| POST   | `/api/admin/membership` | Adjust org/class membership on a user. |
| POST   | `/api/admin/permissions` | Store per-role permissions (recorded in audit). |
| POST   | `/api/admin/refunds/:id/approve` | Approve a pending refund. |
| POST   | `/api/admin/orders/:id/rollback` | Roll back any order; records a compensating timeline event. |
| POST   | `/api/admin/facilities` | Upsert facility (zone blacklists and pickup cutoff hour). |

## Audit

| Method | Path | Description |
|--------|------|-------------|
| GET    | `/api/audit-logs` | Search audit entries by `actor`, `resource`, `from`, `to`. Admin only. Results are always scoped to the caller's organisation (plus system-wide rows). |
| GET    | `/api/audit-logs/export` | Export filtered entries as CSV. Admin only. Same org-scoping as search. |

## Devices and upgrades

| Method | Path | Description |
|--------|------|-------------|
| POST   | `/api/devices/register` | Register or update a managed device. |
| GET    | `/api/devices/policy` | Fetch the upgrade decision for a device (`device_id`, `version`). Authenticated; only the registered owner or a same-org admin may call. Returns `403` for foreign devices. |

## Observability / ops

| Method | Path | Description |
|--------|------|-------------|
| GET    | `/api/health` | Public liveness probe for load balancers and docker-compose. |
| GET    | `/api/metrics` | Request counters (`requests`, `errors`). Admin-only. |
| GET    | `/api/alerts` | Offline alerts synthesised from audit entries. Admin-only and scoped to caller's organisation. |
| POST   | `/api/crash-reports` | Persist a client-side crash into the audit log. Authenticated; actor is taken from the session and the request body is size-capped. |

## Server-rendered UI

| Method | Path | Role |
|--------|------|------|
| GET    | `/`             | Public landing page with login form. |
| GET    | `/student`      | Student dashboard (timeline + state badges). |
| GET    | `/teacher`      | Teacher console (profile, bulk actions, analytics). |
| GET    | `/dispatcher`   | Dispatcher view (strategies, queue, conflicts). |
| GET    | `/admin`        | Admin console (membership, permissions, alerts). |

## Status codes summary

| Status | Meaning |
|--------|---------|
| 200    | Success. |
| 201    | Resource created (booking / delivery). |
| 202    | Accepted but awaiting approval (e.g. cancel inside 24h). |
| 400    | Invalid request body. |
| 401    | Missing or invalid bearer token. |
| 403    | Forbidden by role or scope, or recipient unsubscribed. |
| 404    | Resource not found (session, order, template). |
| 409    | State-machine conflict (reschedule cap, booking full, dispatch conflict). |
| 429    | Reminder cap exceeded. |
| 502    | Retries exhausted (notification). |
| 503    | No eligible courier for the delivery. |
