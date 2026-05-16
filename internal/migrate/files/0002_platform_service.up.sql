ALTER TABLE deliveries
  ADD COLUMN IF NOT EXISTS idempotency_key TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS deliveries_idempotency_key_idx
  ON deliveries (idempotency_key)
  WHERE idempotency_key <> '';

CREATE TABLE IF NOT EXISTS user_contacts (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    TEXT NOT NULL,
  kind       TEXT NOT NULL,
  value      TEXT NOT NULL,
  label      TEXT NOT NULL DEFAULT '',
  enabled    BOOLEAN NOT NULL DEFAULT true,
  verified   BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (user_id, kind, value)
);

CREATE INDEX IF NOT EXISTS user_contacts_user_idx ON user_contacts (user_id);
CREATE INDEX IF NOT EXISTS user_contacts_kind_idx ON user_contacts (kind);

CREATE TABLE IF NOT EXISTS user_preferences (
  user_id        TEXT PRIMARY KEY,
  enabled        BOOLEAN NOT NULL DEFAULT true,
  categories     TEXT[] NOT NULL DEFAULT '{}',
  quiet_hours    JSONB NOT NULL DEFAULT '{}'::jsonb,
  preferred_kind TEXT NOT NULL DEFAULT '',
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
