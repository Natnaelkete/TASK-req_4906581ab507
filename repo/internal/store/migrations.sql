-- HarborClass PostgreSQL schema.
-- Applied at boot by internal/store.OpenPostgres. Idempotent.

CREATE TABLE IF NOT EXISTS users (
    id               TEXT PRIMARY KEY,
    username         TEXT NOT NULL UNIQUE,
    role             TEXT NOT NULL,
    org_id           TEXT NOT NULL DEFAULT '',
    class_ids        TEXT NOT NULL DEFAULT '',
    password_hash    TEXT NOT NULL,
    phone_cipher     TEXT NOT NULL DEFAULT '',
    display_name     TEXT NOT NULL DEFAULT '',
    rating           DOUBLE PRECISION NOT NULL DEFAULT 0,
    load_count       INTEGER NOT NULL DEFAULT 0,
    lat              DOUBLE PRECISION NOT NULL DEFAULT 0,
    lng              DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- class_ids is stored as a comma-separated list on the user row.
-- Guarded ALTER keeps repeat migrations against an existing db safe.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'users' AND column_name = 'class_ids'
    ) THEN
        ALTER TABLE users ADD COLUMN class_ids TEXT NOT NULL DEFAULT '';
    END IF;
END$$;

CREATE TABLE IF NOT EXISTS permissions (
    org_id   TEXT NOT NULL,
    action   TEXT NOT NULL,
    roles    TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (org_id, action)
);

CREATE TABLE IF NOT EXISTS content_items (
    id          TEXT PRIMARY KEY,
    teacher_id  TEXT NOT NULL,
    title       TEXT NOT NULL,
    body        TEXT NOT NULL DEFAULT '',
    pinned      BOOLEAN NOT NULL DEFAULT FALSE,
    published   BOOLEAN NOT NULL DEFAULT TRUE,
    views       INTEGER NOT NULL DEFAULT 0,
    likes       INTEGER NOT NULL DEFAULT 0,
    favorites   INTEGER NOT NULL DEFAULT 0,
    followers   INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS content_items_teacher_created
    ON content_items (teacher_id, created_at);

CREATE TABLE IF NOT EXISTS facilities (
    id                 TEXT PRIMARY KEY,
    name               TEXT NOT NULL,
    blacklisted_zones  TEXT NOT NULL DEFAULT '',
    pickup_cutoff_hour INTEGER NOT NULL DEFAULT 20
);

CREATE TABLE IF NOT EXISTS sessions (
    id           TEXT PRIMARY KEY,
    teacher_id   TEXT NOT NULL,
    class_id     TEXT NOT NULL DEFAULT '',
    org_id       TEXT NOT NULL DEFAULT '',
    title        TEXT NOT NULL,
    starts_at    TIMESTAMPTZ NOT NULL,
    ends_at      TIMESTAMPTZ NOT NULL,
    capacity     INTEGER NOT NULL,
    booked_size  INTEGER NOT NULL DEFAULT 0,
    lat          DOUBLE PRECISION NOT NULL DEFAULT 0,
    lng          DOUBLE PRECISION NOT NULL DEFAULT 0
);

-- Idempotent org_id backfill for existing session tables.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'sessions' AND column_name = 'org_id'
    ) THEN
        ALTER TABLE sessions ADD COLUMN org_id TEXT NOT NULL DEFAULT '';
    END IF;
END$$;

CREATE TABLE IF NOT EXISTS orders (
    id                TEXT PRIMARY KEY,
    number            TEXT NOT NULL UNIQUE,
    kind              TEXT NOT NULL,
    state             TEXT NOT NULL,
    payment           TEXT NOT NULL DEFAULT 'unpaid',
    student_id        TEXT NOT NULL DEFAULT '',
    teacher_id        TEXT NOT NULL DEFAULT '',
    session_id        TEXT NOT NULL DEFAULT '',
    courier_id        TEXT NOT NULL DEFAULT '',
    pickup_zone       TEXT NOT NULL DEFAULT '',
    pickup_at         TIMESTAMPTZ,
    org_id            TEXT NOT NULL DEFAULT '',
    class_id          TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at      TIMESTAMPTZ,
    reschedule_count  INTEGER NOT NULL DEFAULT 0
);

-- order_events persists the per-order timeline the state machine
-- produces on every transition. The handlers and UI read this back on
-- OrderByID so the SQL-backed runtime exposes the same timeline the
-- in-memory store returns.
CREATE TABLE IF NOT EXISTS order_events (
    order_id   TEXT NOT NULL,
    seq        INTEGER NOT NULL,
    at         TIMESTAMPTZ NOT NULL,
    state      TEXT NOT NULL,
    actor      TEXT NOT NULL DEFAULT '',
    message    TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (order_id, seq)
);
CREATE INDEX IF NOT EXISTS order_events_order_at
    ON order_events (order_id, at);

CREATE TABLE IF NOT EXISTS templates (
    id        TEXT PRIMARY KEY,
    category  TEXT NOT NULL,
    subject   TEXT NOT NULL,
    body      TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS subscriptions (
    user_id     TEXT NOT NULL,
    category    TEXT NOT NULL,
    subscribed  BOOLEAN NOT NULL DEFAULT TRUE,
    PRIMARY KEY (user_id, category)
);

CREATE TABLE IF NOT EXISTS delivery_attempts (
    id          TEXT PRIMARY KEY,
    order_id    TEXT NOT NULL,
    user_id     TEXT NOT NULL,
    category    TEXT NOT NULL,
    template_id TEXT NOT NULL,
    attempt     INTEGER NOT NULL,
    sent_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    success     BOOLEAN NOT NULL,
    error_text  TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS delivery_attempts_order_day
    ON delivery_attempts (order_id, sent_at);

CREATE TABLE IF NOT EXISTS audit_log (
    id         TEXT PRIMARY KEY,
    at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    org_id     TEXT NOT NULL DEFAULT '',
    actor      TEXT NOT NULL,
    action     TEXT NOT NULL,
    resource   TEXT NOT NULL,
    detail     TEXT NOT NULL DEFAULT '',
    prev_hash  TEXT NOT NULL DEFAULT '',
    hash       TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS audit_log_at ON audit_log (at);
CREATE INDEX IF NOT EXISTS audit_log_actor ON audit_log (actor);
CREATE INDEX IF NOT EXISTS audit_log_resource ON audit_log (resource);
CREATE INDEX IF NOT EXISTS audit_log_org ON audit_log (org_id, at);

-- Guarded ALTER keeps repeat migrations against an existing db safe.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'audit_log' AND column_name = 'org_id'
    ) THEN
        ALTER TABLE audit_log ADD COLUMN org_id TEXT NOT NULL DEFAULT '';
    END IF;
END$$;

CREATE TABLE IF NOT EXISTS devices (
    id                 TEXT PRIMARY KEY,
    user_id            TEXT NOT NULL,
    platform           TEXT NOT NULL,
    version            TEXT NOT NULL,
    canary             BOOLEAN NOT NULL DEFAULT FALSE,
    forced_upgrade_to  TEXT NOT NULL DEFAULT '',
    last_seen          TIMESTAMPTZ NOT NULL DEFAULT now()
);
