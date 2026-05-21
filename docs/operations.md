# Notifications Operations

Operator runbook for the Notifications admin UI. The plugin is admin-only — there is no user-facing surface beyond contact entries that admins curate on behalf of users.

This document covers day-to-day workflows. For deeper topics see:

- [rule-matching.md](rule-matching.md) — event patterns and template expansion
- [direct-send.md](direct-send.md) — `notifications.send` and the `POST /api/v1/send` endpoint
- [delivery-queue.md](delivery-queue.md) — queue states, retries, idempotency
- [provider-catalog.md](provider-catalog.md) — native vs bridge providers and capability fields
- [debugging.md](debugging.md) — failure diagnosis

## Admin UI map

The SPA mounts at `/admin/*` behind admin auth. The same backend services `/api/admin/*` (admin-only JSON) and `/api/v1/*` (authenticated read; `POST /api/v1/send` is admin-gated). Routes are declared in `cmd/continuum-plugin-notifications/manifest.json`.

| Section | What it manages | Backing endpoint |
| --- | --- | --- |
| Providers | Read-only catalog of every registered provider, including `delivery_kind` and field schema. | `GET /api/admin/providers` |
| Targets | A named instance of a provider with its credentials and per-target config (one Discord webhook, one SMTP relay, etc.). | `/api/admin/targets[/:id][/test]` |
| Rules | Event pattern -> target IDs, with optional title/body templates and `enabled` flag. | `/api/admin/rules[/:id]` |
| Contacts | Per-user address book (email, phone, push handle, etc.) consumed by direct sends with `recipient.kind=user`. | `/api/admin/contacts[/:id]` |
| Preferences | Per-user `enabled`, categories, quiet hours, preferred contact kind. Stored but not yet enforced by the matcher — informational. | `/api/admin/preferences/:userID` |
| Deliveries | Recent delivery audit (status, attempts, last_error, payload). Ordered newest first; default limit 100, max 500 via `?limit=`. | `/api/admin/deliveries[/:id]` |
| Retries | "Run due now" button kicks the poller out-of-band. | `POST /api/admin/deliveries/run` |
| Config | `max_attempts` and `timeout_ms` (defaults 3 and 8000). | `GET/PUT /api/admin/config` |

`GET /api/admin/status` returns counts (providers, targets, rules, recent_deliveries) and is the cheapest health check.

## Target lifecycle

1. Create a target with `name`, `provider` (id from the catalog), `enabled`, and a `config` map keyed by the provider's field keys.
2. Click **Test** in the UI (or `POST /api/admin/targets/:id/test`). The server constructs a canned message and calls the provider's `Send` synchronously. See [debugging.md](debugging.md#test-send-scope) for what test-send does and does not validate.
3. Reference the target's UUID in one or more rules, or address it directly from a publisher via `notifications.send`.

Disable a target instead of deleting it if rules still reference it — deletions cascade nothing; rules will continue to enqueue against a missing target ID and every delivery will fail with `ERROR: ...` from the store on dequeue.

## Rule lifecycle

1. Pick an `event_pattern`. Use exact match for one event, a trailing-`*` prefix for a family (e.g. `plugin.continuum.requests.*`), or `*` to catch every subscribed event.
2. Add the target IDs that should receive deliveries when the rule matches.
3. Optional: customise `title` and `body` templates. Defaults are `{{event}}` and `{{summary}}`. Available placeholders: `{{event}}`, `{{summary}}`, and any top-level key in the event payload. See [rule-matching.md](rule-matching.md).
4. Toggle `enabled`. Disabled rules are skipped by the consumer.

Rules do not have priorities. Every enabled rule whose pattern matches the event enqueues one delivery per enabled target listed on that rule, so overlapping rules duplicate deliveries by design — useful when, say, one rule fans an event out to an ops chat and a separate rule emails the requester.

## Contacts and preferences

Contacts feed direct sends only (`recipient.kind=user`). The rule matcher does not look at contacts or preferences today — the consumer queues one delivery per `target_id` listed on the rule regardless of who triggered the event.

`(user_id, kind, value)` is unique. Use the same `kind` strings you intend to address in send calls (`email`, `phone`, etc.) — the matcher in [`consumer.go`](../internal/consumer/consumer.go) does an exact-string compare against `contact.Kind`.

`enabled=false` contacts are excluded. `verified` is informational; no logic gates on it yet.

User preferences (`quiet_hours`, `categories`, `preferred_kind`) are stored but unused by the delivery path. Persist them if you want, but do not assume they suppress anything.

## App config

`GET /api/admin/config` and `PUT /api/admin/config` manipulate the singleton row in `app_config`:

```json
{ "max_attempts": 3, "timeout_ms": 8000 }
```

`max_attempts` is the total attempts before a delivery becomes terminal `failed`. `timeout_ms` is the per-send context timeout — applied uniformly to every provider including SMTP. Bumping it is the right knob when an external API is slow but not actually broken.

## Verifying after changes

1. Open `/admin/` and confirm Status shows non-zero counts and `configured=true`.
2. Run **Test** on the changed target. Confirm a 200 and a real message at the destination.
3. Trigger the smallest real event that should match the rule (a request submission, a library media add, etc.) and watch Deliveries for the new row.
4. If the row stays `queued` for longer than the scheduled task interval, hit **Run due now** to force a tick — useful for confirming the issue is the queue, not the matcher.
