# Questions, Assumptions, and Solutions

The HarborClass business prompt is detailed but leaves a number of
execution details open to interpretation. This file records every
ambiguous area as a `Question → Assumption → Solution` triple.

---

## 1. PostgreSQL version and driver

- **Question:** The prompt requires PostgreSQL as the system of record but does not pin a major version, nor does it name a driver.
- **Assumption:** PostgreSQL 16 is acceptable (widely supported, current LTS at time of writing), and `github.com/lib/pq` is a reasonable pure-Go driver that builds without CGo.
- **Solution:** `docker-compose.yml` pins `postgres:16-alpine`; `go.mod` imports `github.com/lib/pq`; migrations in `internal/store/migrations.sql` use SQL that works on PostgreSQL 14+ so upgrading/downgrading the major version is a one-line compose change.

## 2. Authentication mechanism

- **Question:** The prompt mandates multi-role authorisation but does not specify a session format (JWT, opaque token, cookie, etc.).
- **Assumption:** An opaque bearer token with an in-process session table keeps the service dependency-free, satisfies the offline constraint, and is sufficient for a single-deployment demo.
- **Solution:** `internal/auth/auth.go` implements `Service.Login` returning a random token and a `sessions` map keyed by token. `middleware.RequireAuth` validates the token on every protected route. Clients may send `Authorization: Bearer <token>` or `X-Api-Token: <token>`.

## 3. Password hashing algorithm

- **Question:** The prompt says passwords must be stored encrypted at rest but does not pin an algorithm.
- **Assumption:** A salted SHA-256 pipeline keeps the test suite dependency-free while still demonstrating a correct one-way hash path. In production a team would typically prefer bcrypt/argon2.
- **Solution:** `internal/auth/crypto.go` provides `HashPassword` / `VerifyPassword` using `sha256.Sum256(salt || ':' || password)`. Swapping in bcrypt is a one-file change because all call sites go through these two helpers.

## 4. Notification transport

- **Question:** The prompt requires an offline notification centre with no third-party dependency, but a notification system ultimately needs *some* transport.
- **Assumption:** The bundled deployment should run entirely in-process; production on-prem SMTP/SMS gateways plug in via an interface.
- **Solution:** `internal/notify/engine.go` defines `Sender` as an interface. `notify.LocalSender` is a deterministic in-process implementation used by both the server and tests. `/api/alerts` synthesises admin alerts from audit entries for `notify.send.failed` actions so operators still have an observable feed.

## 5. Rate-limit semantics (3 reminders per order per day)

- **Question:** Does the 3-per-day cap count only successful attempts, or all attempts including failed ones?
- **Assumption:** All attempts count. The intent of the cap is to prevent spamming a recipient, which is orthogonal to whether the send succeeded.
- **Solution:** `notify.Engine.Send` calls `store.CountAttemptsForOrderOn(orderID, today)` before every new `Send` invocation and refuses with `ErrRateLimited` when `count >= ReminderCap`. Every attempt — retry or first — appends a row to `delivery_attempts`.

## 6. 24-hour cancellation rule

- **Question:** The prompt says "cancellations within 24 hours require teacher approval". Is teacher approval mandatory, or does the cancel move to an awaiting-approval state?
- **Assumption:** Students should not be blocked entirely; their cancel should enter an `pending_approval` state so a teacher (or admin) can later approve it via the same endpoint.
- **Solution:** `internal/order/state_machine.go` returns `ErrNeedsApproval` when inside the window without an approver. The handler (`internal/http/handlers/booking.go`) catches this, transitions the order to `pending_approval`, persists, and responds `202 Accepted`. A subsequent call from a teacher/admin completes the transition to `cancelled`.

## 7. Refund window anchor

- **Question:** The prompt says refunds must be filed within 7 days of completion. Does "completion" mean the end of the class or the creation of the order?
- **Assumption:** Completion is the post-session completion event — the `completed` state on the order.
- **Solution:** `order.RequestRefund` compares `now - CompletedAt` against `RefundWindow = 7 * 24h` and returns `ErrRefundWindowClosed` when it is breached. `CompletedAt` is written by the `Complete` transition.

## 8. Order-number sequence scope

- **Question:** The order-number format `HC-03282026-000742` implies a numeric sequence; the prompt does not say whether the sequence is global, per-day, per-org, or per-user.
- **Assumption:** Per-day — the date segment already disambiguates between days, and per-day counters make the number easy to generate without a central sequence service.
- **Solution:** `store.CountDailyOrders(today)+1` is used as the sequence on each create call; `order.GenerateNumber` formats `HC-MMDDYYYY-NNNNNN`. A Postgres sequence column can be wired in later if higher throughput needs tighter monotonicity.

## 9. Distance-first strategy with only zone strings

- **Question:** Distance strategy needs coordinates; the prompt describes pickup zones as strings.
- **Assumption:** For demo traffic, a deterministic mapping from zone string to pseudo-coordinates is enough; real deployments replace this with a zone-to-facility lookup table.
- **Solution:** `dispatch.orderLocation` computes a stable lat/lng from the zone characters so the `haversine` sort in `distanceFirst` is deterministic. Tests assert ordering stability rather than specific distances.

## 10. Facility-level vs courier-level blacklists

- **Question:** The prompt mentions facility-level blacklisted zones. Couriers may themselves refuse some zones. How do these interact?
- **Assumption:** Both exist independently. Facility-level blacklist refuses pickup at the site entirely; courier-level blacklist just removes one courier from the eligibility pool.
- **Solution:** `dispatch.ValidatePickup` enforces the facility rule (returning `ErrZoneBlacklisted`); `dispatch.EligibleCouriers` filters the pool and surfaces `ErrCourierBlacklist` when the only reason a courier can't serve the order is their own zone list. The UI renders distinct messages for each.

## 11. PII encryption key source

- **Question:** The prompt requires PII to be encrypted at rest but does not specify a key source.
- **Assumption:** A single symmetric key, sourced from the environment, is sufficient for the single-deployment demo.
- **Solution:** `HARBORCLASS_ENCRYPTION_KEY` env var feeds `auth.DeriveKey` (SHA-256 to 32 bytes) which is passed to `auth.EncryptPII` / `auth.DecryptPII` (AES-GCM). docker-compose sets a demo value; production deployments must override it.

## 12. Admin-configurable permissions

- **Question:** The prompt requires admins to configure "who can set check-in rules, export data, approve refunds". Is the permission model mutable at runtime?
- **Assumption:** A first release can ship a baseline static matrix (in `auth.Can`) while recording admin permission changes in a tamper-evident log. Activating per-role ACLs from the audit log is a follow-up.
- **Solution:** `auth.Can` is a single-file decision function covering all sensitive actions. `POST /api/admin/permissions` appends a dated, hashed entry to the audit chain, giving a complete trail of configuration changes for compliance.

## 13. "Go Templ" in tests

- **Question:** The prompt mandates Go Templ for the UI. Running the `templ` CLI as part of the test harness adds a build-time dependency not mentioned in the prompt.
- **Assumption:** The Templ sources are the authoritative artefact for review, but the test suite should run with only `go test ./...`.
- **Solution:** `web/templates/*.templ` contains the Templ source files. `webtpl/webtpl.go` is a hand-written Go equivalent that produces the same HTML and is what `internal/http/handlers/pages.go` calls. Templ render tests live in `webtpl/*_test.go` and assert on the generated HTML directly.

## 14. Managed-device canary / forced-upgrade model

- **Question:** The prompt requires canary and forced-upgrade policies for managed devices without specifying the data shape.
- **Assumption:** A simple per-device record with a `canary` flag and a `forced_upgrade_to` target version is enough to drive the UI.
- **Solution:** `models.Device` carries `Canary bool` and `ForcedUpgradeTo string`. `POST /api/devices/register` upserts the row; `GET /api/devices/policy?device_id=…&version=…` compares the reported version against `ForcedUpgradeTo` and returns `{upgrade_required, forced_version, canary}` for the client to act on.

## 15. Health-check path and shape

- **Question:** The prompt asks for API health checks but does not define the payload shape.
- **Assumption:** A minimal JSON body (`status`, `service`, `time`) is sufficient for load balancers and docker-compose.
- **Solution:** `GET /api/health` returns `{"status":"ok","service":"harborclass","time":"<RFC3339>"}`, wired into the docker-compose `depends_on: condition: service_healthy` so the app waits for the db and operators can wire an external monitor to the same path.
