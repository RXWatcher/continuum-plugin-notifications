# Direct Sends

A direct send addresses a specific target by ID or name and bypasses the rules table. There are two transports:

1. **Event** — publish `notifications.send` (or any of the suffix variants below). Consumed by `internal/consumer/consumer.go::handleDirectSend`.
2. **HTTP** — `POST /api/v1/send` on this plugin's route surface. Handled by `internal/server/server.go::handleSend`.

Both share the same payload shape and most of the same resolution logic. The HTTP endpoint adds an admin auth requirement and an optional `send_now` flag that skips the queue entirely.

## Event names that count as direct sends

The consumer recognises:

- `notifications.send`
- `plugin.silo.notifications.send`
- Anything ending in `.notifications.send`
- Anything ending in `.notification.send` (singular)

Any other event name is treated as a regular event and runs through the rules engine.

## Request shape

```json
{
  "target_id": "uuid",            // or:
  "target_name": "ops-discord",
  "to": "alice@example.com",      // optional override; comma-separated for multi-rcpt
  "recipient": { "kind": "user", "user_id": "U1", "contact_kind": "email" },
  "recipients": [ { "kind": "email", "value": "ops@example.com" } ],
  "category": "system",
  "idempotency_key": "request-1234-fulfilled",
  "title": "...",                 // required for HTTP; defaulted for event
  "body": "...",
  "event_name": "plugin.foo.bar", // optional; defaults to plugin.silo.notifications.direct
  "payload": { "anything": "here" },
  "attachments": [ { "filename": "x.pdf", "content_type": "application/pdf", "data_base64": "..." } ],
  "send_now": false               // HTTP only; bypasses queue when true
}
```

### Target resolution

`target_id` wins if present. Otherwise the server lists targets and picks the first row whose `name` matches `target_name` exactly. Names are not indexed unique — duplicates are operator error and the resolver picks whichever row Postgres returns first.

A disabled target rejects the send (HTTP 400 / event-side `errors.New("target disabled")`).

### Recipient resolution

`to` (a raw, comma-separated string) wins. Otherwise the server walks `recipient` plus `recipients`:

- `kind: "email"` or empty: takes `value` as-is.
- `kind: "user"`: looks up `user_contacts` for `user_id` and filters by `contact_kind` (defaults to `email`). Every enabled, matching, non-empty contact value is appended.
- Anything else: takes `value` as-is.

Multiple values are joined with `", "`. The resolved string lands in `target.Config["to"]` for that one delivery via the per-delivery override mechanism (see [delivery-queue.md](delivery-queue.md#per-delivery-target_config-overrides)).

A direct send with no `to` and no resolvable recipients still goes through. The provider receives the target's stored `to`. That is fine for shared-channel targets (Discord, Slack) but will fail for SMTP if the target has no `to` configured.

### Idempotency keys

If `idempotency_key` is non-empty, the insert uses `ON CONFLICT (idempotency_key) WHERE idempotency_key <> ''` to make duplicates a no-op. The handler returns the existing row's ID, not a fresh row.

Use this to make publishers safe to retry. A good key is `<publisher>.<entity>.<event>.<id>`. The unique index is partial (`WHERE idempotency_key <> ''`), so rule-driven deliveries that always carry an empty key never collide.

There is no TTL — keys live forever in the deliveries table. If a publisher re-uses a key six months later it still de-duplicates. Pick keys that are naturally unique over the retention window you care about.

### `send_now` (HTTP only)

`POST /api/v1/send` with `send_now: true` calls the provider synchronously, returning `200 {"status":"delivered"}` or `502 {"error":...}` from the provider. No row is written to `deliveries`, so this path leaves no audit trail and gets no retries. Useful for "did the credential work right now" probes from a script; bad for anything you want to debug after the fact.

The queued path (default) returns `202 {"status":"queued","delivery_id":"..."}` and behaves identically to a rule-driven enqueue from then on.

## HTTP auth

`/api/v1/send` requires `IsAdmin` on the resolved identity. The other `/api/v1/*` routes (`/capabilities`, `/targets`, `GET /deliveries/:id`) only require authentication. The manifest enforces this with two route entries — `service_send` is admin-only and ordered before the catch-all `service_api`.

## What direct sends do not give you

- No template rendering. `title`, `body`, and `event_name` are stored as-is. If you want `{{event}}` expansion you have to do it in the caller.
- No per-recipient fan-out. Multi-recipient sends become a single delivery row whose provider builds one outbound message addressed to several recipients (SMTP `To:` list, Discord channel message, etc.). One provider failure fails the row for everyone.
- No user-preference checks. Quiet hours and `enabled=false` on `user_preferences` are not consulted.
