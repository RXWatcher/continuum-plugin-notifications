# Notifications Setup, Debugging, And Flows

Plugin ID: `continuum.notifications`
Version documented: `0.1.0`

## Purpose

event consumer and retrying outbound notification queue for Continuum/plugin events.

## Runtime Dependencies

- Continuum plugin host
- Postgres database/schema
- Events emitted by Continuum core or other plugins
- Outbound webhook/notification destinations configured in the admin UI

## Setup Checklist

1. Configure database_url.
2. Install and open the Notifications admin UI.
3. Create destinations and rules for the events you care about.
4. Trigger a known event, such as a request approval or playback event.
5. Confirm delivery, retry state, and logs.

## Configuration Reference

- `connection`
- `database_url`

Use the plugin manifest/admin form as the source of truth for field validation and defaults. Keep database credentials scoped to the plugin schema unless a plugin explicitly needs read access to Continuum core tables.

## Exposed Routes

- `* /api/admin/* [admin]`
- `* /api/v1/* [authenticated]`
- `GET /assets/* [public]`
- `GET /admin/* [admin]`

## Capabilities

- `event_consumer.v1 (notification-events) - Receives Continuum and plugin events, applies rules, and queues outbound notifications.`
- `scheduled_task.v1 (retry-notifications) - Retries queued or failed notification deliveries.`
- `http_routes.v1 (admin) - Admin UI for targets, rules, provider settings, delivery audit, retries, and test sends.`

## Operational Flows

### Notification delivery

1. Continuum or a plugin emits an event.
2. Notifications consumes the event and evaluates rules.
3. Matching rules enqueue outbound deliveries.
4. The retry task sends pending deliveries and records success/failure history.

## How This Plugin Communicates

- Consumes events from Continuum core and plugins.
- Does not fulfill requests itself.
- Sends outbound HTTP/webhook notifications and records delivery state.

## Debugging Runbook

- If no events arrive, confirm the source plugin emits events and Notifications is enabled.
- If rules do not match, inspect event type/name and payload fields.
- If delivery fails, check destination URL/auth and retry task logs.
- Use the admin route for delivery history and failure details.

## Log And Health Checks

- Start with Continuum Admin -> Plugins and confirm the installation is enabled.
- Check the plugin process logs around startup for manifest loading, migration, and route registration.
- Check scheduled task logs when a workflow depends on polling or reconciliation.
- Confirm the plugin routes are reachable through Continuum using the access level shown above.
- For database-backed plugins, verify the configured role can connect, create/migrate tables in its schema, and read/write expected rows.

## Common Failure Patterns

- Wrong installation ID selected in a portal or router setting after reinstalling a plugin.
- Plugin database URL points at the public schema instead of the dedicated plugin schema.
- Reverse proxy forwards the SPA route but not `/api/*`, `/api/v1/*`, `/assets/*`, or provider-specific public routes.
- Network checks are run from the operator laptop instead of from the Continuum/plugin runtime network.
- Secrets are regenerated during restart, invalidating signed URLs, encrypted fields, or login state.

## Verification After Changes

1. Restart or reload the plugin installation.
2. Open the plugin route or admin page in Continuum.
3. Exercise the smallest workflow that crosses a plugin boundary.
4. Confirm both the source plugin and destination plugin record the same request/session/login identifier.
5. Leave the scheduled reconciler enough time to run, then confirm terminal state or a useful error.
