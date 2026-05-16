# continuum-plugin-notifications

Native Go notification hub for Continuum. It receives host/plugin events,
matches them against admin-defined rules, queues deliveries, retries failures,
and sends notifications through built-in Go providers.

This plugin does not shell out to Apprise and does not depend on Apprise URL
syntax. Apprise is only the feature benchmark: broad provider coverage, one
admin surface, test sends, retries, and delivery audit.

## Capabilities

| Capability | Notes |
|---|---|
| `event_consumer.v1` (`notification-events`) | Receives Continuum/plugin events and queues matching notifications. |
| `scheduled_task.v1` (`retry-notifications`) | Processes queued deliveries and retries transient failures. |
| `http_routes.v1` (`admin`) | Full admin UI for providers, targets, rules, tests, and audit. |

## Providers

The provider registry now exposes the Apprise-scale surface: 138 catalog
entries, with every catalog entry backed by a Go `Provider` implementation.
Most HTTP, chat, SMS, email, incident, media, and push services send directly
from Go. A few platform-specific or hardware transports use explicit bridge
providers because this plugin currently builds for Linux:

- DBus / GLib / GNOME / macOS / Windows desktop notifications
- Blink(1)
- SMPP
- SNS signed endpoint mode

Those bridge providers still participate in the same rules, retries, test
sends, and delivery audit path.

## Configuration

The host-level plugin config only needs the Postgres DSN for the dedicated
`notifications` schema. Runtime settings, targets, and rules are edited from
the plugin admin UI.

```sql
CREATE ROLE plugin_notifications WITH LOGIN PASSWORD '<chosen by operator>';
CREATE SCHEMA notifications AUTHORIZATION plugin_notifications;
GRANT CONNECT ON DATABASE continuum TO plugin_notifications;
```

Example DSN:

```text
postgres://plugin_notifications:...@host:5432/continuum?search_path=notifications&sslmode=disable
```

## Build & test

```bash
make build
./continuum-plugin-notifications manifest
make test
```
