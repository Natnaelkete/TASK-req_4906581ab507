# HarborClass Architecture & Design

## Goals

HarborClass must deliver every capability in the business brief
(multi-role scheduling, creator content, offline notifications,
tamper-evident auditing, dispatch with conflict detection, managed-
device policies) with zero third-party runtime dependencies.

## Runtime model

- `docker compose up` is the only supported start command.
- The `app` container is a single Go binary built from
  `cmd/harborclass/main.go`.
- The `db` container is PostgreSQL 16.
- The `app` container reads `internal/store/migrations.sql` on startup
  and applies it idempotently, then seeds demo data via
  `internal/bootstrap.Seed`.

## Module layout

Domain packages deliberately stay small and self-contained:

- `internal/models` — shared Go types (Users, Sessions, Orders,
  Facilities, AuditEntries, Devices). No business behaviour.
- `internal/store` — persistence boundary. `Store` is the interface.
  `Postgres` is the SQL implementation; `Memory` is a real, full,
  in-memory implementation that tests use without mocking.
- `internal/auth` — password hashing (SHA-256 with salt — a static
  decision to keep the suite dependency-free; bcrypt is a drop-in
  replacement in `internal/auth/crypto.go`), AES-GCM encryption for PII
  at rest, phone-number masking, and the `auth.Can` decision function
  for org/class scope checks.
- `internal/order` — the *shared* booking/delivery state machine.
  It owns the "max 2 reschedules", "24h approval window", "7-day
  refund window", "rollback" rules, and the human-readable order
  number format `HC-MMDDYYYY-NNNNNN`.
- `internal/dispatch` — courier selection with three strategies
  (distance-first, rating-first, load-balanced) and the eligibility
  checks (facility cutoff, facility-level blacklist, courier-level
  zone blacklist, same-window double booking).
- `internal/notify` — reminder cap (default 3 per order per day),
  retries up to 5 with exponential backoff, subscription-aware
  delivery, delivery-attempt audit rows.
- `internal/audit` — hash-chain `Append`/`Verify`/`Search`/`ExportTo`.
  The chain is tamper-evident: each entry's hash is `SHA-256` of its
  payload concatenated with the prior entry's hash.
- `internal/http/router.go` — Gin router. Every route is registered
  here; the static auditor can read it top-to-bottom.
- `internal/http/handlers/*.go` — one file per domain, each holding
  thin Gin handlers that delegate to the services above.
- `internal/http/middleware/` — auth, metrics, request logging.
- `webtpl/` — compiled Templ components used by server-rendered pages,
  plus their unit tests.
- `web/templates/*.templ` — Templ source files that mirror `webtpl/`.

## Request lifecycle

1. Gin receives a request and runs `RequestLogger` and `Observability`
   middleware.
2. Authenticated routes pass through `middleware.RequireAuth`, which
   resolves the bearer token against the in-memory session table.
3. The handler runs its business call (state-machine transition,
   dispatch assignment, notification send, etc).
4. Every non-trivial handler appends one entry to the audit chain so
   admins have a complete tamper-evident trail.
5. Response body is serialised JSON or (for the `/student`,
   `/teacher`, `/dispatcher`, `/admin` paths) server-rendered HTML
   emitted by `webtpl`.

## State machine (bookings & deliveries)

The same Go type (`models.Order`) represents both bookings and
deliveries. `internal/order.Machine` enforces:

- `created -> confirmed` on booking creation (auto-confirms after
  capacity is reserved).
- `confirmed -> rescheduled` up to `MaxReschedules = 2`.
- `confirmed -> cancelled` requires teacher/admin approval if the
  session starts within 24 hours, otherwise the student may cancel.
- `confirmed/rescheduled/in_progress -> completed`.
- `completed -> refund_review -> refunded`, with refund requests
  limited to 7 days post-completion.
- Any non-`rolled_back` state can be rolled back by admin.

Each transition appends a timeline entry and an audit log entry.

## Dispatch pipeline

`dispatch.Assign` is the single entry point. It runs, in order:

1. `ValidatePickup` (facility cutoff, facility-level zone blacklist).
2. `EligibleCouriers` (drops courier-level blacklisted zones and any
   courier already working a delivery whose pickup falls within a
   sliding 60-minute window).
3. `Select(strategy).Select` — picks one of the remaining couriers
   using the configured strategy.

Conflicts are surfaced as typed errors (`ErrPickupAfterCutoff`,
`ErrZoneBlacklisted`, `ErrCourierBlacklist`, `ErrCourierDoubleBook`,
`ErrNoEligibleCourier`) and map to 409 or 503 with a `conflict:true`
flag the UI renders in the conflict panel.

## Notification engine

`notify.Engine.Send` is a self-contained pipeline:

1. Template lookup (404-equivalent if missing).
2. Subscription lookup; unsubscribed recipients abort with
   `ErrUnsubscribed`.
3. Count today's attempts for this order; abort with
   `ErrRateLimited` if `>= ReminderCap` (default 3).
4. Send via the `Sender`. On failure, record the attempt, sleep
   `Backoff(base, attempt) + jitter`, and retry up to `MaxAttempts`
   times (default 5). Every attempt, successful or not, is persisted
   to `delivery_attempts` so admins have a full audit trail.

`LocalSender` is the bundled offline sender. Production deployments
swap in an on-prem SMTP/SMS transport that still satisfies the
`Sender` interface.

## Tamper-evident audit

The chain is append-only. `Append` computes
`SHA-256(id | at | actor | action | resource | detail | prev_hash)`
and writes the row. `Verify` recomputes each entry's hash and
compares with `prev_hash` of the next row — any mutation is detected
at the index of the first broken link.

`ExportTo` writes RFC-4180-ish CSV with quoting for fields that
contain commas, newlines, or quotes. The admin console's export link
streams this CSV with `Content-Disposition: attachment`.

## Authorisation

`auth.Can(subject, action, target)` centralises role and
org/class scope. Admins are scoped to their org; teachers further
scoped to classes they belong to for cancel-approval decisions.
Students can only modify orders they own. The `internal/auth`
package contains all decisions in one place so a static auditor can
verify each action.

## PII & masking

Passwords are salted SHA-256 (swap-in bcrypt in the same file if
policy requires). Phone numbers are encrypted with AES-GCM using a
key derived from `HARBORCLASS_ENCRYPTION_KEY` and never returned in
API payloads — masked display values only.

## Observability & device policies

- `/api/health` returns a fast JSON health check.
- `/api/metrics` returns request and error counters.
- `/api/alerts` surfaces recent `notify.send.failed` audit entries as
  an offline admin alert feed.
- `/api/crash-reports` lets managed devices POST a crash report which
  is persisted to the audit chain.
- `/api/devices/register` + `/api/devices/policy` implement canary and
  forced-upgrade decisions.

## Testing posture

- **Unit tests** live next to their packages (`*_test.go`).
- **HTTP integration tests** live in `internal/http/*_integration_test.go`
  and boot a full Gin router wired to the real services and the real
  in-memory store.
- **Templ render tests** live in `webtpl/*_test.go` and assert that
  each role dashboard contains the required text, badges, analytics
  numbers, and conflict messages.

No mocks of controllers, services, or repositories are used
anywhere. The in-memory store is a full `Store` implementation —
choosing it for tests is orthogonal to the mocking question.
