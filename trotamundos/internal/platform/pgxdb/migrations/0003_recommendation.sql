CREATE SCHEMA IF NOT EXISTS recommendation;

CREATE TABLE IF NOT EXISTS recommendation.user_preferences (
    user_id                 uuid PRIMARY KEY,
    max_budget_cents        bigint NOT NULL DEFAULT 0,
    lodging_types           text[] NOT NULL DEFAULT '{}',
    transport_types         text[] NOT NULL DEFAULT '{}',
    preferred_destinations  text[] NOT NULL DEFAULT '{}',
    preferred_start_date    date,
    preferred_end_date      date,
    updated_at              timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS recommendation.user_interactions (
    id                uuid NOT NULL,
    user_id           uuid NOT NULL,
    recommendation_id uuid NOT NULL,
    action            text NOT NULL CHECK (action IN ('interested','discarded','adjust','inquiry')),
    adjustments       jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at        timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id, user_id)
) PARTITION BY HASH (user_id);

CREATE TABLE IF NOT EXISTS recommendation.user_interactions_0 PARTITION OF recommendation.user_interactions FOR VALUES WITH (MODULUS 4, REMAINDER 0);
CREATE TABLE IF NOT EXISTS recommendation.user_interactions_1 PARTITION OF recommendation.user_interactions FOR VALUES WITH (MODULUS 4, REMAINDER 1);
CREATE TABLE IF NOT EXISTS recommendation.user_interactions_2 PARTITION OF recommendation.user_interactions FOR VALUES WITH (MODULUS 4, REMAINDER 2);
CREATE TABLE IF NOT EXISTS recommendation.user_interactions_3 PARTITION OF recommendation.user_interactions FOR VALUES WITH (MODULUS 4, REMAINDER 3);

CREATE INDEX IF NOT EXISTS interactions_user_idx ON recommendation.user_interactions (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS interactions_recommendation_idx ON recommendation.user_interactions (recommendation_id);

CREATE TABLE IF NOT EXISTS recommendation.anonymized_signals (
    id                  uuid PRIMARY KEY,
    user_bucket         text NOT NULL,
    destination         text NOT NULL DEFAULT '',
    recommendation_type text NOT NULL DEFAULT '',
    action              text NOT NULL,
    price_bucket        text NOT NULL DEFAULT '',
    created_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS anonymized_signals_created_idx ON recommendation.anonymized_signals (created_at);
CREATE INDEX IF NOT EXISTS anonymized_signals_affinity_idx ON recommendation.anonymized_signals (destination, recommendation_type, action);

CREATE TABLE IF NOT EXISTS recommendation.agency_deliveries (
    request_id uuid PRIMARY KEY,
    agency_id  uuid NOT NULL,
    status     text NOT NULL,
    detail     text NOT NULL DEFAULT '',
    sent_at    timestamptz NOT NULL DEFAULT now()
);
