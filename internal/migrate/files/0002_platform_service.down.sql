DROP TABLE IF EXISTS user_preferences;
DROP TABLE IF EXISTS user_contacts;
DROP INDEX IF EXISTS deliveries_idempotency_key_idx;
ALTER TABLE deliveries DROP COLUMN IF EXISTS idempotency_key;
