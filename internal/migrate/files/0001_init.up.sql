CREATE TABLE IF NOT EXISTS app_config (
  id         INT PRIMARY KEY DEFAULT 1,
  data       JSONB NOT NULL DEFAULT '{}'::jsonb,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT app_config_singleton CHECK (id = 1)
);

INSERT INTO app_config (id, data)
VALUES (1, '{"max_attempts":3,"timeout_ms":8000}'::jsonb)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS targets (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name       TEXT NOT NULL,
  provider   TEXT NOT NULL,
  enabled    BOOLEAN NOT NULL DEFAULT true,
  config     JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS targets_provider_idx ON targets (provider);

CREATE TABLE IF NOT EXISTS rules (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name          TEXT NOT NULL,
  event_pattern TEXT NOT NULL,
  target_ids    TEXT[] NOT NULL DEFAULT '{}',
  enabled       BOOLEAN NOT NULL DEFAULT true,
  title         TEXT NOT NULL DEFAULT '{{event}}',
  body          TEXT NOT NULL DEFAULT '{{summary}}',
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS rules_enabled_idx ON rules (enabled);

CREATE TABLE IF NOT EXISTS deliveries (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  event_name  TEXT NOT NULL,
  target_id   TEXT NOT NULL,
  provider    TEXT NOT NULL,
  title       TEXT NOT NULL,
  body        TEXT NOT NULL,
  payload     JSONB NOT NULL DEFAULT '{}'::jsonb,
  status      TEXT NOT NULL CHECK (status IN ('queued','delivered','failed')),
  attempts    INT NOT NULL DEFAULT 0,
  last_error  TEXT NOT NULL DEFAULT '',
  next_run_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS deliveries_due_idx
  ON deliveries (next_run_at)
  WHERE status = 'queued';

CREATE INDEX IF NOT EXISTS deliveries_created_idx ON deliveries (created_at DESC);
