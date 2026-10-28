CREATE EXTENSION IF NOT EXISTS citext;
CREATE SCHEMA IF NOT EXISTS platform;
CREATE SCHEMA IF NOT EXISTS auth;
CREATE SCHEMA IF NOT EXISTS itinerary;
CREATE SCHEMA IF NOT EXISTS analytics;

CREATE TABLE IF NOT EXISTS platform.schema_migrations (
    version     text PRIMARY KEY,
    applied_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS auth.users (
    id             uuid PRIMARY KEY,
    email          citext NOT NULL UNIQUE,
    full_name      text NOT NULL,
    password_hash  text NOT NULL,
    role           text NOT NULL DEFAULT 'traveler' CHECK (role IN ('traveler','analyst','admin')),
    locale         text NOT NULL DEFAULT 'es',
    region         text NOT NULL DEFAULT 'AR',
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS itinerary.trips (
    id           uuid PRIMARY KEY,
    user_id      uuid NOT NULL,
    name         text NOT NULL,
    destination  text NOT NULL DEFAULT '',
    start_date   date,
    end_date     date,
    status       text NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS trips_user_status_idx ON itinerary.trips (user_id, status);
CREATE INDEX IF NOT EXISTS trips_user_destination_idx ON itinerary.trips (user_id, lower(destination));

CREATE TABLE IF NOT EXISTS itinerary.reservations (
    id                uuid PRIMARY KEY,
    user_id           uuid NOT NULL,
    trip_id           uuid NOT NULL REFERENCES itinerary.trips(id) ON DELETE CASCADE,
    reservation_type  text NOT NULL CHECK (reservation_type IN ('flight','hotel','car','other')),
    provider          text NOT NULL,
    provider_locator  text NOT NULL DEFAULT '',
    title             text NOT NULL DEFAULT '',
    destination       text NOT NULL DEFAULT '',
    start_at          timestamptz,
    end_at            timestamptz,
    status            text NOT NULL DEFAULT 'confirmed' CHECK (status IN ('confirmed','canceled')),
    source            text NOT NULL CHECK (source IN ('email','provider','manual')),
    source_ref        text NOT NULL DEFAULT '',
    details           jsonb NOT NULL DEFAULT '{}'::jsonb,
    last_synced_at    timestamptz,
    stale             boolean NOT NULL DEFAULT false,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS reservations_user_trip_idx ON itinerary.reservations (user_id, trip_id, start_at);
CREATE INDEX IF NOT EXISTS reservations_user_status_idx ON itinerary.reservations (user_id, status);
CREATE UNIQUE INDEX IF NOT EXISTS reservations_dedup_idx
    ON itinerary.reservations (user_id, provider, provider_locator)
    WHERE provider_locator <> '';

CREATE TABLE IF NOT EXISTS itinerary.reservation_changes (
    id              uuid PRIMARY KEY,
    user_id         uuid NOT NULL,
    reservation_id  uuid NOT NULL REFERENCES itinerary.reservations(id) ON DELETE CASCADE,
    source          text NOT NULL,
    changed_fields  text[] NOT NULL DEFAULT '{}',
    previous        jsonb NOT NULL DEFAULT '{}'::jsonb,
    current         jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS changes_reservation_idx ON itinerary.reservation_changes (reservation_id, created_at);

CREATE TABLE IF NOT EXISTS itinerary.ingestion_events (
    id                   uuid PRIMARY KEY,
    user_id              uuid NOT NULL,
    mailbox_id           uuid,
    channel              text NOT NULL,
    message_id           text NOT NULL,
    payload              jsonb NOT NULL,
    status               text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','processed','ignored','error')),
    attempts             integer NOT NULL DEFAULT 0,
    last_error           text NOT NULL DEFAULT '',
    next_attempt_at      timestamptz NOT NULL DEFAULT now(),
    received_at          timestamptz,
    processing_started_at timestamptz,
    processed_at         timestamptz,
    created_at           timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, message_id)
);
CREATE INDEX IF NOT EXISTS events_pending_idx ON itinerary.ingestion_events (status, next_attempt_at);

CREATE TABLE IF NOT EXISTS itinerary.mail_accounts (
    id               uuid PRIMARY KEY,
    user_id          uuid NOT NULL,
    provider         text NOT NULL,
    email            citext NOT NULL,
    scopes           text[] NOT NULL DEFAULT '{}',
    token_cipher     text NOT NULL DEFAULT '',
    status           text NOT NULL DEFAULT 'active' CHECK (status IN ('active','revoked')),
    last_scanned_at  timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    revoked_at       timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS mail_accounts_active_idx
    ON itinerary.mail_accounts (user_id, email)
    WHERE status = 'active';

CREATE TABLE IF NOT EXISTS itinerary.mail_whitelist (
    id       uuid PRIMARY KEY,
    user_id  uuid NOT NULL,
    pattern  text NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS mail_whitelist_unique_idx
    ON itinerary.mail_whitelist (user_id, lower(pattern));

CREATE TABLE IF NOT EXISTS itinerary.mail_filters (
    id        uuid PRIMARY KEY,
    user_id   uuid NOT NULL,
    field     text NOT NULL CHECK (field IN ('sender','subject','body')),
    operator  text NOT NULL CHECK (operator IN ('contains','equals','not_contains','starts_with')),
    value     text NOT NULL,
    active    boolean NOT NULL DEFAULT true
);
CREATE INDEX IF NOT EXISTS mail_filters_user_idx ON itinerary.mail_filters (user_id, active);

CREATE TABLE IF NOT EXISTS itinerary.trip_shares (
    id            uuid PRIMARY KEY,
    trip_id       uuid NOT NULL REFERENCES itinerary.trips(id) ON DELETE CASCADE,
    owner_user_id uuid NOT NULL,
    guest_email   citext NOT NULL,
    guest_user_id uuid,
    created_at    timestamptz NOT NULL DEFAULT now(),
    revoked_at    timestamptz
);
CREATE INDEX IF NOT EXISTS trip_shares_trip_idx ON itinerary.trip_shares (trip_id, revoked_at);
CREATE INDEX IF NOT EXISTS trip_shares_guest_email_idx ON itinerary.trip_shares (guest_email, revoked_at);
CREATE INDEX IF NOT EXISTS trip_shares_guest_user_idx ON itinerary.trip_shares (guest_user_id, revoked_at);

CREATE TABLE IF NOT EXISTS itinerary.notifications (
    id             uuid PRIMARY KEY,
    user_id        uuid NOT NULL,
    notification_type text NOT NULL,
    title          text NOT NULL,
    body           text NOT NULL DEFAULT '',
    resource_type  text NOT NULL DEFAULT '',
    resource_id    text NOT NULL DEFAULT '',
    read_at        timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS notifications_user_idx ON itinerary.notifications (user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS itinerary.audit_logs (
    id           uuid PRIMARY KEY,
    user_id      uuid,
    action       text NOT NULL,
    resource     text NOT NULL,
    resource_id  text NOT NULL DEFAULT '',
    ip           text NOT NULL DEFAULT '',
    details      jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS audit_user_idx ON itinerary.audit_logs (user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS itinerary.provider_snapshots (
    provider   text NOT NULL,
    locator    text NOT NULL,
    payload    jsonb NOT NULL,
    fetched_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, locator)
);

CREATE TABLE IF NOT EXISTS itinerary.agencies (
    id           uuid PRIMARY KEY,
    name         text NOT NULL,
    email        text NOT NULL,
    phone        text NOT NULL DEFAULT '',
    description  text NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS itinerary.agency_recommendations (
    id           uuid PRIMARY KEY,
    agency_id    uuid NOT NULL REFERENCES itinerary.agencies(id) ON DELETE CASCADE,
    title        text NOT NULL,
    destination  text NOT NULL,
    description  text NOT NULL DEFAULT '',
    price_cents  bigint NOT NULL DEFAULT 0,
    currency     text NOT NULL DEFAULT 'USD',
    start_date   date,
    end_date     date,
    active       boolean NOT NULL DEFAULT true
);
CREATE INDEX IF NOT EXISTS recommendations_agency_idx ON itinerary.agency_recommendations (agency_id, active);

CREATE TABLE IF NOT EXISTS itinerary.agency_favorites (
    user_id     uuid PRIMARY KEY,
    agency_id   uuid NOT NULL REFERENCES itinerary.agencies(id),
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS itinerary.agency_requests (
    id                 uuid PRIMARY KEY,
    user_id            uuid NOT NULL,
    agency_id          uuid NOT NULL REFERENCES itinerary.agencies(id),
    reservation_id     uuid REFERENCES itinerary.reservations(id) ON DELETE SET NULL,
    recommendation_id  uuid REFERENCES itinerary.agency_recommendations(id) ON DELETE SET NULL,
    message            text NOT NULL DEFAULT '',
    status             text NOT NULL DEFAULT 'open' CHECK (status IN ('open','closed')),
    context            jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at         timestamptz NOT NULL DEFAULT now(),
    closed_at          timestamptz
);
CREATE INDEX IF NOT EXISTS agency_requests_user_idx ON itinerary.agency_requests (user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS itinerary.recommendation_responses (
    id                 uuid PRIMARY KEY,
    user_id            uuid NOT NULL,
    recommendation_id  uuid NOT NULL REFERENCES itinerary.agency_recommendations(id) ON DELETE CASCADE,
    action             text NOT NULL CHECK (action IN ('discarded','inquiry')),
    created_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, recommendation_id)
);

CREATE TABLE IF NOT EXISTS analytics.reservation_facts (
    id              uuid PRIMARY KEY,
    destination     text NOT NULL DEFAULT '',
    start_date      date,
    end_date        date,
    status          text NOT NULL,
    provider        text NOT NULL,
    fact_type       text NOT NULL,
    agency_id       uuid,
    occurred_at     timestamptz NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS facts_occurred_idx ON analytics.reservation_facts (occurred_at);
CREATE INDEX IF NOT EXISTS facts_destination_idx ON analytics.reservation_facts (lower(destination));
CREATE INDEX IF NOT EXISTS facts_status_idx ON analytics.reservation_facts (status);
