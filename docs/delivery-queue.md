# Delivery Queue and Retries

The queue lives in the `deliveries` table. Every successful enqueue (from rule match, direct send, or test send via the queued path) inserts one row. The scheduled task and an admin "Run due now" button drive the same poller in [`internal/poll/poll.go`](../internal/poll/poll.go).

## Row lifecycle

| State | Set by | Meaning |
| --- | --- | --- |
| `queued` | `Enqueue` and `MarkFailed(retry=true)` | Waiting for `next_run_at <= now()`. Picked up by the poller. |
| `delivered` | `MarkDelivered` after a provider returns nil. | Terminal success. Attempts incremented, `last_error` cleared. |
| `failed` | `MarkFailed(retry=false)` once `attempts+1 >= max_attempts`, or when the target is gone / disabled. | Terminal failure. Stays in the table for audit. |

The check constraint on `deliveries.status` only allows these three.

## Poller tick

`Poller.Tick`:

1. Reads `app_config` for the active `max_attempts` (does not read `timeout_ms` directly — the registry is constructed with the timeout at startup; see below).
2. `SELECT ... WHERE status='queued' AND next_run_at <= now() ORDER BY created_at LIMIT 25`. Hard cap of 25 rows per tick.
3. For each row:
   - Fetches the target. Missing target -> `MarkFailed` with retry only if attempts remain.
   - Disabled target -> `MarkFailed` with `retry=false`, regardless of attempts. A disabled target is treated as permanently unreachable.
   - Applies `target_config` overrides from the payload (see below).
   - Calls `Registry.Send(ctx, target, msg)`. The registry wraps `ctx` in `context.WithTimeout(timeout)` from `NewRegistry` (set to `app_config.timeout_ms` at plugin startup).
   - Success -> `MarkDelivered`. Failure -> `MarkFailed(retry = attempts+1 < max_attempts)`.

The scheduled task interval is driven by the Continuum host based on the manifest's `scheduled_task.v1` declaration. Out-of-band ticks are triggered manually via `POST /api/admin/deliveries/run`.

## Retry policy

Default `max_attempts=3`, `timeout_ms=8000` (set in `0001_init.up.sql` and re-applied by `store.DefaultAppConfig`).

`MarkFailed(retry=true)` pushes `next_run_at` out by **30 seconds**. This is hard-coded — there is no exponential backoff, no jitter, no per-provider override. A flapping destination retried three times sees the queue try at T+0s, T+30s, T+60s, then give up.

`attempts` increments on both success and failure (see `MarkDelivered`'s `attempts=attempts+1`), so a row that succeeds on the second try shows `attempts=2`.

Bumping `max_attempts` affects all in-flight rows on the next tick — the value is read every tick. Reducing it can turn a still-retryable row into a `failed` row at the next failure.

`timeout_ms` is **read once** when the notify Registry is built at startup (see `runtime.go` / plugin init). Changing it requires a plugin restart to take effect. Until then, the UI shows the new value but the registry still enforces the old one.

## Per-delivery `target_config` overrides

The poller honours `payload["target_config"]` as a map of `{string -> string}` that is merged into `target.Config` for this one send only. This is how direct sends inject `to` recipients without permanently editing the target row.

```go
target = applyTargetConfigOverrides(target, row.Payload)
```

Empty values are skipped — they cannot blank out a stored config key. The override is non-persistent; the actual row in `targets` is untouched.

This is the only way to vary provider config per delivery. Rule-driven deliveries never set `target_config`, so they always send against the stored target config exactly.

## Attachments

`payload["attachments"]` is decoded back into `[]notify.Attachment` (`filename`, `content_type`, `data_base64`). Providers that opt-in (Discord webhook, SMTP) handle them; others ignore them. There is no separate attachments table — the base64 payload sits inside `deliveries.payload`. Big attachments make for big rows.

The capabilities map on each provider (`MaxAttachmentBytes`, `ContentTypes`) is advisory metadata for the UI; the providers themselves do not enforce limits before sending.

## Idempotency at enqueue

The deliveries table has `UNIQUE INDEX deliveries_idempotency_key_idx ON (idempotency_key) WHERE idempotency_key <> ''`. The insert uses an `ON CONFLICT ... DO UPDATE SET idempotency_key=deliveries.idempotency_key` no-op — the returning clause yields the existing row's id and timestamps so the caller can't tell whether it was the original insert or a duplicate.

Only direct sends populate the key today; rule-driven deliveries always insert with an empty key and never collide.

## Manual interventions

There is no admin action to:

- Force-retry a `failed` row. Either set `status='queued'`, `attempts=0`, `next_run_at=now()` by hand in SQL or re-trigger the source event.
- Cancel a pending row. Either delete it or set `status='failed'`.
- Reorder the queue. `created_at ASC` ordering is hard-coded; raise priorities by lowering `created_at` if you must, but the field is normally `DEFAULT now()`.

The poller does not take a row-level lock — two poll ticks running concurrently could pick the same row. In practice the scheduled task is single-instance and the manual "Run due now" button is rare, so this has not been a problem. Be aware before adding additional pollers.

## Useful SQL

Pending work:

```sql
SELECT id, event_name, provider, attempts, last_error, next_run_at
FROM deliveries
WHERE status = 'queued'
ORDER BY next_run_at;
```

Recent terminal failures with their last error:

```sql
SELECT id, provider, target_id, last_error, updated_at
FROM deliveries
WHERE status = 'failed'
ORDER BY updated_at DESC
LIMIT 50;
```

Reset a failed row for one more pass (raises `max_attempts` headroom by also resetting `attempts`):

```sql
UPDATE deliveries
SET status='queued', attempts=0, last_error='', next_run_at=now()
WHERE id = '...';
```
