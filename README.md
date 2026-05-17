# Notifications for Continuum

`continuum.notifications` is Continuum's notification hub. It receives host and
plugin events, matches them against operator-defined rules, queues deliveries,
retries transient failures, and records delivery history.

The plugin is passive: other plugins publish their normal events whether or not
Notifications is installed. If it is absent, workflows continue normally and no
notification subscriber receives those events.

## Features

- Admin UI for providers, targets, rules, test sends, delivery audit, and
  retries.
- Event consumer that can match Continuum and plugin events.
- Persistent delivery queue with retry handling.
- Large provider catalog implemented directly in Go where possible.
- Bridge providers for platform-specific or hardware transports when native
  Linux operation is not available.
- Scheduled retry worker for queued and transiently failed notifications.

## Providers

The provider registry targets Apprise-scale coverage: HTTP services, chat,
SMS, email, incident platforms, media systems, and push providers. Most send
directly from Go. A small set of desktop or hardware transports are represented
as explicit bridge providers.

## Configuration

| Key | Required | Description |
|---|---|---|
| `connection` | yes | Postgres DSN for the dedicated `notifications` schema. |

Runtime provider settings, targets, and rules are managed in the plugin admin
UI rather than the host-level manifest form.

Example DSN:

```text
postgres://plugin_notifications:password@postgres:5432/continuum?search_path=notifications&sslmode=disable
```

## Database Setup

```sql
CREATE ROLE plugin_notifications WITH LOGIN PASSWORD '<chosen>';
CREATE SCHEMA notifications AUTHORIZATION plugin_notifications;
GRANT CONNECT ON DATABASE continuum TO plugin_notifications;
```

## Operations

- Start with a single test target and rule, then broaden event filters.
- Use test sends before enabling a new provider for production events.
- Review the delivery audit after changing provider credentials.
- Keep notification rules specific enough to avoid duplicate alerts.

## Build And Test

```bash
make build
./continuum-plugin-notifications manifest
make test
```
