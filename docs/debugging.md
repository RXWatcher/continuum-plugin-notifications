# Notifications Debugging Runbook

Read this with [delivery-queue.md](delivery-queue.md), [rule-matching.md](rule-matching.md), and [provider-catalog.md](provider-catalog.md) open — most concrete signals come from the deliveries table.

The single most useful query is "what does Postgres think happened for this event":

```sql
SELECT id, status, attempts, last_error, provider, target_id, next_run_at, created_at
FROM deliveries
WHERE event_name = $1
ORDER BY created_at DESC
LIMIT 20;
```

If the row exists you are debugging delivery. If it does not, you are debugging matching.

## Symptom: no delivery row for an event you expected

Walk the path the consumer takes:

1. **Is Notifications consuming the event?** Confirm the event name appears in `cmd/silo-plugin-notifications/manifest.json` under `subscriptions`, or matches the `plugin.*` catch-all. Host events (`library.*`, `silo.*`) must be enumerated explicitly.
2. **Did the publisher actually emit?** Check the publishing plugin's logs. If Notifications is only one of several subscribers, the host log will show fan-out. A misspelled event name is a common cause — `plugin.silo.requests.fulfilled` (correct) vs `plugin.silo.request.fulfilled` (silently dropped).
3. **Does a rule match?** Pull rules and test the matcher mentally:
   - `*` matches anything subscribed.
   - Exact match — case-sensitive, no trimming.
   - Trailing-`*` — `plugin.silo.requests.*` matches `plugin.silo.requests.submitted`; it does not match `plugin.silo.requests` (missing dot).
   - No mid-string globs. `plugin.*.requests.*` matches nothing.
4. **Is the rule `enabled`?** Disabled rules are skipped silently.
5. **Are the target IDs still valid and enabled?** Deleted or disabled targets produce no row.
6. **Is this a direct send?** Events ending in `.notifications.send` skip the rules table — they need a resolvable target, not a matching rule.

If steps 1–6 look right and there is still no row, raise the consumer log level. The consumer logs `queued notifications event=... count=...` at info on every event that produced at least one delivery; absence is itself a signal.

## Symptom: row exists but is stuck `queued`

1. **Is `next_run_at` in the future?** Failed-and-retrying rows are pushed 30 seconds out.
2. **Is the scheduled task running?** Confirm the host has scheduled `retry-notifications` and is invoking it. Hit `POST /api/admin/deliveries/run` to force a tick — if it processes the row, the scheduler is the problem; if it does not, the row itself has an issue.
3. **Is the target still present and enabled?** A row whose target was disabled after enqueue stays `queued` until the next tick, at which point it becomes terminal `failed` ("target disabled").
4. **Is `attempts >= max_attempts`?** Should not happen with status `queued`, but a manual SQL edit can leave it inconsistent. Inspect with `SELECT attempts, status FROM deliveries WHERE id=...`.
5. **More than 25 due rows?** The poller processes at most 25 per tick. A large backlog drains 25-per-interval until clear.

## Symptom: row keeps cycling failed -> queued -> failed

The retry loop is doing its job. Inspect `last_error`:

- HTTP status with body snippet -> see [provider-catalog.md](provider-catalog.md#common-provider-failure-categories).
- `target disabled` or `target_id` lookup error -> fix the target.
- `decode attachment ...` -> the publisher sent malformed base64; the row will keep failing until manually `failed`-ed or attempts exhaust. Edit the publisher.
- Context deadline exceeded -> raise `timeout_ms` if the provider is slow but healthy. Remember the change only takes effect after a plugin restart.

After `attempts` reaches `max_attempts` the row becomes terminal `failed` and stops retrying. Bumping `max_attempts` only helps rows still in `queued`.

## Symptom: test-send works, production sends fail

Most common causes:

- **Template / payload mismatch.** Test-send uses a fixed `{"test": true}` payload and a fixed title/body. Production events carry whatever shape the publisher emits; templates may render unexpectedly. Inspect `deliveries.payload` and the rendered `title`/`body` on the row.
- **Recipient resolution.** Test-send does not exercise `to` / `recipient` / `recipients`. A direct-send path that uses `recipient.kind=user` only fails in production when the user has no enabled contacts of the requested kind.
- **Provider rate limit.** A green test does not promise headroom under real volume — Slack/Discord 429s only appear once traffic ramps.
- **Different identity.** `POST /api/v1/send` requires admin; if a service plugin is configured to call it with a non-admin token, prod fails with 403 while tests from the admin UI succeed.

## Symptom: duplicate notifications

- **Overlapping rules.** Each enabled matching rule enqueues independently. List rules and check that two patterns are not catching the same event for the same target.
- **Publisher retries without an idempotency key.** Direct sends without an `idempotency_key` are not de-duplicated. The publisher should set a stable key (`<publisher>.<entity>.<event>.<id>`).
- **Re-delivery from manual SQL.** Setting a delivered row back to `queued` reships.
- **Host fan-out duplication.** Verify the publisher emits the event exactly once per state transition by reading its own logs.

## Symptom: nothing in `last_error` for a failed row

`MarkFailed` writes whatever the provider returned. An empty `last_error` plus `status='failed'` usually means the row was force-failed via SQL or by the "target disabled" path before any send attempt. Look at `attempts` — if it is `0`, no send was tried.

## Database checks

Confirm the role can reach its schema:

```sql
SET search_path TO notifications;
SELECT count(*) FROM targets;
SELECT count(*) FROM rules;
SELECT count(*) FROM deliveries WHERE status='queued';
```

The migration files in `internal/migrate/files/` are the source of truth for table shape. `0001_init.up.sql` creates `app_config`, `targets`, `rules`, `deliveries`. `0002_platform_service.up.sql` adds `idempotency_key`, `user_contacts`, `user_preferences`. `0003_book_request_rule_templates.up.sql` seeds rule presets.

If a fresh installation reports `relation "..." does not exist`, the migration runner did not execute — usually because the configured role lacks `CREATE` on the schema. See the README for the canonical `CREATE ROLE / CREATE SCHEMA ... AUTHORIZATION` setup.

## Logs to grep

- `consumer.go` -> `queued notifications event=<name> count=<n>` — proof a rule matched.
- `poll.go` -> `notification delivery failed id=<uuid> target=<name> err=<text>` — primary failure signal.
- `httproutes/server.go` returns plain HTTP errors with a wrapped JSON body `{"error":{"message":"..."}}` — useful when calling `/api/v1/send` from a service plugin.

The admin SPA also exposes Deliveries directly; for most failures the admin UI is faster than tailing logs.

## When the plugin route is unreachable

`GET /admin/...` returns 503 with `{"error":{"code":"not_ready","message":"plugin not configured"}}` until the host has called `Configure` (i.e. supplied a `database_url`). The wrapping HTTP handler from `httproutes/server.go` uses an atomically-swappable handler — until it is set, every request is a 503. Cure: ensure the host-level `connection.database_url` is set, then re-enable the plugin install.

`/api/admin/*` returns 401/403 from the host gateway, not from this plugin, when the caller is not an admin. Check the host auth chain, not the plugin code, for those.
