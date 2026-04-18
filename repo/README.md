Project Type: fullstack
Backend: Go (Gin)
Frontend: Go Templ (server-rendered HTML)

# HarborClass Booking & Dispatch

HarborClass is a multi-role booking and dispatch platform for education
and training organisations that run in-person sessions. It implements
the full HarborClass business brief - multi-role scheduling, creator
content management, delivery dispatch, an offline notification centre,
tamper-evident auditing, and device/upgrade policies - without any
third-party service dependencies at runtime.

## Stack and architecture

- **Backend:** Go (Gin) for the decoupled REST API.
- **Frontend:** Go Templ server-rendered HTML (see `web/templates/` for
  sources and `webtpl/` for the compiled Go renderers the router uses).
- **Database:** PostgreSQL, schema in `internal/store/migrations.sql`.
- **Persistence boundary:** one `store.Store` interface with both a
  PostgreSQL and an in-memory implementation (the in-memory store is a
  real implementation, not a mock, and is used by integration tests).
- **State machine:** `internal/order` - shared between bookings and
  deliveries, enforces reschedule caps, 24h approval window, 7-day
  refund window, and rollback.
- **Dispatch:** `internal/dispatch` - distance-first, rating-first, and
  load-balanced strategies with facility cutoff, blacklisted zones,
  zone-blacklisted couriers, and double-booking detection.
- **Notifications:** `internal/notify` - reminder cap of 3 per order
  per day, 5 retries with exponential backoff, unsubscribe flag
  enforcement, delivery attempts written to an audit table.
- **Audit:** `internal/audit` - tamper-evident hash chain with search
  and CSV export. Admin UI reads and exports via this package.
- **Authentication & authorisation:** `internal/auth` - salted password
  hashing, AES-GCM PII encryption, masked views, role/org/class scope
  checks.

## Running with Docker

The only supported runtime is Docker Compose. The bundled
`docker-compose.yml` provisions the application and PostgreSQL.

```bash
docker-compose up
```

After the `db` service is healthy, the API becomes available at:

API base: http://localhost:8080/api
Web UI: http://localhost:8080/

Seed data (demo org, class, one session, five role accounts, and the
default notification templates) is installed deterministically on first
boot - controlled by the `HARBORCLASS_SEED` environment variable.

## Tests

Run the full test suite:

```bash
./run_tests.sh
```

`run_tests.sh` is Docker-only. It builds the `tests` stage of the
bundled Dockerfile - which resolves every module from the committed
`go.sum` during `docker build` (no `go mod tidy`, no runtime install
path) - and then runs `go test -count=1 ./...` inside a container
with `--network none` so the tests run fully offline. No host Go
toolchain, package manager, or database setup is required. It covers:

- Backend unit tests for the state machine, dispatch strategies,
  notification engine, audit hash chain, and scope/crypto helpers.
- Real HTTP integration tests (no mocked handlers, services, or
  repositories) across auth, bookings, deliveries, notifications,
  audit, admin, teacher console, devices, ops, and the server-rendered
  role pages.
- Templ rendering tests for the Student, Teacher, Dispatcher, and
  Admin dashboards.

## Demo credentials

| Role        | Username     | Password        | Display name      |
|-------------|--------------|-----------------|-------------------|
| Student     | `student`    | `student-pass`  | Alex Student      |
| Teacher     | `teacher`    | `teacher-pass`  | Taylor Teacher    |
| Courier     | `courier`    | `courier-pass`  | Casey Courier     |
| Dispatcher  | `dispatcher` | `dispatch-pass` | Dana Dispatcher   |
| Administrator | `admin`    | `admin-pass`    | Avery Admin       |

These accounts are created at startup by `internal/bootstrap.Seed`.

## Minimal verification scenario

1. `docker-compose up` - start the stack.
2. `curl -s -X POST http://localhost:8080/api/auth/login \
   -H 'Content-Type: application/json' \
   -d '{"username":"student","password":"student-pass"}'`
   returns a bearer token.
3. `curl -s http://localhost:8080/api/sessions \
   -H "Authorization: Bearer $TOKEN"` lists the seeded session
   `sess-demo-1`.
4. `curl -s -X POST http://localhost:8080/api/bookings \
   -H 'Content-Type: application/json' \
   -H "Authorization: Bearer $TOKEN" \
   -d '{"session_id":"sess-demo-1"}'` creates an auto-confirmed order
   with a `HC-MMDDYYYY-NNNNNN` number and a visible `state` badge
   (`confirmed`).
5. Visit `http://localhost:8080/student` in a browser - the
   server-rendered dashboard displays the order timeline and state
   badge.
6. Log in as `dispatcher` and `POST /api/deliveries/:id/assign` with
   `{"strategy":"rating-first"}` to assign a courier; try a
   blacklisted zone to see the UI conflict message.

## Endpoint groups

Full API inventory is in `docs/api-spec.md` and mirrored in
`internal/http/router.go`. The major groups are:

- `/api/auth/*` - login/logout/whoami
- `/api/sessions`, `/api/bookings/*`, `/api/my/*` - student/teacher flows
- `/api/deliveries/*` - dispatch and courier assignment (with
  per-order completion)
- `/api/notifications/*` - notification centre
- `/api/audit-logs`, `/api/audit-logs/export` - admin audit
- `/api/admin/*` - org/class membership, permissions, rollback
- `/api/devices/*` - device registration and upgrade policies
- `/api/health`, `/api/metrics`, `/api/alerts`, `/api/crash-reports` - ops

## Environment variables

Every environment variable below is wired in `docker-compose.yml`; the
defaults shown are the values used by the supported Docker runtime.

| Variable                          | Default                                                                           |
|-----------------------------------|-----------------------------------------------------------------------------------|
| `HARBORCLASS_HTTP_ADDR`           | `:8080`                                                                           |
| `HARBORCLASS_DB_URL`              | `postgres://harbor:harbor@db:5432/harborclass?sslmode=disable`                    |
| `HARBORCLASS_MIGRATIONS`          | `/app/migrations.sql`                                                             |
| `HARBORCLASS_SEED`                | `true`                                                                            |
| `HARBORCLASS_ENCRYPTION_KEY`      | `dev-only-encryption-key-change-me-32`                                            |
| `HARBORCLASS_REMINDER_CAP`        | `3`                                                                               |
| `HARBORCLASS_RETRY_MAX`           | `5`                                                                               |
| `HARBORCLASS_RETRY_BASE_MS`       | `500`                                                                             |
| `HARBORCLASS_PICKUP_CUTOFF_HOUR`  | `20`                                                                              |
| `HARBORCLASS_REQUIRE_DB`          | `true` - missing Postgres is a fatal startup error under Docker Compose.          |

## Project layout

```
cmd/harborclass/main.go        # entrypoint, wiring
internal/http/router.go        # Gin router (all routes listed)
internal/http/handlers/        # handler code per domain
internal/http/middleware/      # auth, logging, metrics
internal/order/                # shared booking/delivery state machine
internal/dispatch/             # courier assignment + conflicts
internal/notify/               # notification engine + templates
internal/audit/                # hash chain + search + export
internal/auth/                 # auth, scope, crypto, masking
internal/store/                # Store interface + Postgres + memory + migrations.sql
internal/bootstrap/            # demo data seeder
internal/config/               # config loader (env vars)
internal/models/               # shared domain types
web/templates/*.templ          # Templ sources
webtpl/                        # compiled Go renderers + unit tests
docs/design.md                 # architecture notes
docs/api-spec.md               # full endpoint inventory
questions.md                   # assumptions and applied answers
metadata.json                  # project metadata (incl. full prompt)
Dockerfile, docker-compose.yml # container runtime
run_tests.sh                   # go test ./... inside the app image
```
