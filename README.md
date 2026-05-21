# Notifications for Continuum

`continuum.notifications` is Continuum's notification hub. It receives host and plugin events, matches them against operator-defined rules, queues outbound deliveries across a wide provider catalog, and retries transient failures while keeping a delivery audit trail.

The plugin is passive. Other plugins publish their normal events whether or not Notifications is installed; if it is absent, those workflows continue unchanged and no notification subscriber receives the events.

## Category

Lives under **Operations**.

## Capabilities

| Type | ID | Purpose |
| --- | --- | --- |
| `event_consumer.v1` | `notification-events` | Receives Continuum and plugin events, applies rules, and queues outbound notifications. |
| `scheduled_task.v1` | `retry-notifications` | Drives the delivery queue: sends due notifications and retries transient failures. |
| `http_routes.v1` | `admin` | Admin SPA and JSON API for providers, targets, rules, delivery audit, retries, and test sends. |

## Dependencies

Passive cross-plugin consumer. The plugin subscribes to a broad set of host and plugin events, but no other plugin depends on it. In that sense it is standalone: removing Notifications does not break any publisher.

Host: [`ContinuumApp/continuum`](https://github.com/ContinuumApp/continuum). SDK: [`ContinuumApp/continuum-plugin-sdk`](https://github.com/ContinuumApp/continuum-plugin-sdk).

## External services

- Postgres, using a dedicated `notifications` schema for targets, rules, deliveries, user contacts, and preferences.
- Outbound HTTP, SMTP, and provider-specific protocols to whatever delivery endpoints are configured (chat webhooks, SMS gateways, transactional email APIs, incident platforms, push services, media servers, and so on).

## Providers

The provider registry targets Apprise-scale coverage with roughly a hundred entries grouped into core webhooks, HTTP/API services, mobile push, social and chat, SMS gateways, ops and media systems, and a long tail of remaining push and parity providers (see `internal/notify/`). Most providers send directly from Go using `net/http`, `net/smtp`, and provider-specific JSON payloads.

A small number of platform-specific or hardware transports are represented as explicit bridge providers. These require an external bridge service (typically a small HTTP shim the operator runs alongside the plugin) and are marked `delivery_kind: bridge` in the catalog so the admin UI can surface that requirement. Everything else is `delivery_kind: native`.

## Rule matching

Each rule has an `event_pattern`, a list of target IDs, optional templated `title` and `body`, and an `enabled` flag. The matcher supports an exact match, a single `*` wildcard, or a trailing-`*` prefix match. Title and body templates expand `{{event}}`, `{{summary}}`, and any top-level payload key.

For each matched rule, the consumer enqueues one delivery per enabled target. Direct sends arrive as `notifications.send` (or a `.notifications.send` suffix) and bypass rule matching, addressing a specific target by ID or name with optional dynamic recipients resolved from the user contact book.

## Delivery queue and retries

Deliveries are persisted to Postgres with `queued`, `delivered`, or `failed` status and a `next_run_at` timestamp. The scheduled task ticks the poller, which pulls up to 25 due rows, looks up the target and provider, applies any per-delivery `target_config` overrides (used by direct sends to inject recipients), and calls the provider's `Send`.

On success the row is marked `delivered`. On failure the row is marked failed and, while `attempts < max_attempts`, requeued with `next_run_at` pushed out by 30 seconds; once attempts are exhausted it stays `failed`. `max_attempts` and per-send `timeout_ms` are stored in `app_config` and default to 3 attempts and 8 seconds. Idempotency keys deduplicate direct sends at enqueue time.

## Test send capability

The admin UI exposes a test-send action so operators can validate provider credentials, target configuration, and recipient resolution against a canned payload before turning a rule loose on production events. Use it after editing credentials or before enabling a new provider for live traffic.

## Event subscriptions

The consumer subscribes to a wide list of host and plugin event names, including a broad `plugin.*` catch-all so newly installed plugins start flowing through rules without redeploying Notifications:

```text
continuum.notifications.send
notifications.send
library.media_added
plugin.continuum.requests.{submitted,approved,denied,cancelled,fulfilled}
plugin.continuum.arrproxy.{submitted,downloading,imported,failed,cancelled,unrouted}
plugin.continuum.arrouter.{submitted,downloading,imported,failed,cancelled,unrouted}
plugin.continuum.audiobooks.request_submitted
plugin.continuum.ebooks.request_submitted
plugin.continuum.bookwarehouse-audio.{request_acknowledged,request_status_changed,request_fulfilled,request_failed}
plugin.continuum.bookwarehouse-ebook.{request_acknowledged,request_status_changed,request_fulfilled,request_failed}
plugin.continuum.ebook-requests.{request_acknowledged,request_status_changed,request_fulfilled,request_failed}
plugin.*
```

See `cmd/continuum-plugin-notifications/manifest.json` for the canonical, fully-expanded list.

## Configuration

| Key | Required | Description |
| --- | --- | --- |
| `connection` | yes | Postgres DSN for the dedicated `notifications` schema. |

Runtime settings — provider credentials, targets, rules, user contacts and preferences, and the `max_attempts` / `timeout_ms` knobs — live in the plugin's own tables and are managed from the admin UI rather than the host-level manifest form.

Example DSN:

```text
postgres://plugin_notifications:password@postgres:5432/continuum?search_path=notifications&sslmode=disable
```

Database setup:

```sql
CREATE ROLE plugin_notifications WITH LOGIN PASSWORD '<chosen>';
CREATE SCHEMA notifications AUTHORIZATION plugin_notifications;
GRANT CONNECT ON DATABASE continuum TO plugin_notifications;
```

## Detailed docs

- [Operations](docs/operations.md) — admin UI map, target and rule lifecycles, app config.
- [Rule matching](docs/rule-matching.md) — pattern semantics and template expansion.
- [Direct sends](docs/direct-send.md) — `notifications.send` event and `POST /api/v1/send`.
- [Delivery queue](docs/delivery-queue.md) — queue states, retries, idempotency, manual interventions.
- [Provider catalog](docs/provider-catalog.md) — native vs bridge, recipient modes, common failure categories.
- [Debugging runbook](docs/debugging.md) — symptom-driven diagnosis.

## Build and release

```bash
make build
make test
```

CI builds linux-amd64 binaries on push to main via the reusable workflow in [RXWatcher/continuum-plugin-repository](https://github.com/RXWatcher/continuum-plugin-repository) and publishes them to the catalog at [`./binaries/`](https://github.com/RXWatcher/continuum-plugin-repository/tree/main/binaries).
