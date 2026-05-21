# Provider Catalog

The catalog is built once at startup in `notify.NewRegistry`. Eight provider groups feed it (`coreProviders`, `httpAPIProviders`, `mobilePushProviders`, `socialChatProviders`, `smsGatewayProviders`, `opsMediaProviders`, `remainingPushProviders`, `finalParityProviders`). Source lives in `internal/notify/`.

Each provider exposes:

- `ID()` — stable string used as `targets.provider`.
- `DisplayName()` — for the admin UI.
- `Fields()` — declared field schema. The UI uses `Field.Control`, `Default`, `Options`, `Help`, etc. to render the editor and validate input.
- `Capabilities()` — runtime hints (`recipient_mode`, `dynamic_recipients`, `attachments`, `max_attachment_bytes`, `content_types`).
- `Send(ctx, target, msg)` — actually performs the delivery.

`GET /api/admin/providers` returns the catalog including the synthesised `delivery_kind` and `implementation_note` per provider. The endpoint is sorted by display name and includes every registered provider — there is no per-installation enable/disable for providers (you enable/disable targets, not providers).

## Field normalisation

`normalizeFields` and `enrichField` in `provider.go` add sensible defaults for common field keys: secret fields auto-become `password` controls; `port` becomes `number`; `tls_mode`, `priority`, and `severity` become `options` with a canned set; URLs get placeholder text. Providers can override any of this by setting `Control`, `Placeholder`, `Help`, etc. explicitly.

If you add a new provider and its admin field renders as a plain text input when it should be a URL or password, check `enrichField` — adding the new key there is usually the fix.

## `delivery_kind`: native vs bridge

`providerDeliveryKind` classifies each provider:

- **`native`** (`implementation_note: Implemented directly by this plugin.`) — the provider's `Send` runs in-process. Most of the catalog is native: anything that speaks HTTP/HTTPS, SMTP, or a documented provider API directly from Go.
- **`bridge`** (`implementation_note: Requires an external bridge service configured by the operator.`) — the provider has a `bridge_url` field, **or** its id/name contains "bridge". `Send` POSTs JSON to the operator-managed shim, which is responsible for talking the actual protocol.

You cannot tell from the catalog alone what shape the bridge expects. Today the bridge providers post one of two payloads:

```json
{ "title": "...", "body": "...", "event": "..." }                  // desktopBridgeProvider
{ "color": "#aaccff", "title": "...", "body": "..." }              // blink1 bridge
{ "from": "+15555550100", "to": "+15555550199", "text": "..." }    // smpp bridge
```

Run your shim on a URL only reachable from the plugin container, terminate TLS at your reverse proxy if you care, and authenticate via an mTLS or shared-secret header on the proxy — the plugin sends no auth headers to bridges.

If you intend to model a hardware or platform-specific transport as a bridge, follow the same pattern (one field `bridge_url`, postJSON the message dict) so `providerDeliveryKind` classifies it automatically.

## Recipient modes

`Capabilities.RecipientMode` is purely informational metadata for the admin UI today. The runtime does not branch on it. Current values:

| Mode | Meaning | Example providers |
| --- | --- | --- |
| `direct_addressable` | Per-message recipient — `to` can be set per send via `target_config` override. | `smtp`, `sendgrid`, `twilio`, most SMS gateways. |
| `broadcast_channel` | The target represents a fixed channel/room. No per-send recipient. | `slack`, `discord`, `teams`. |
| `configured_target` | Provider-specific addressing baked into the target's config (device tokens, topic names, etc.). | Most push and ops providers. |

`dynamic_recipients: true` tells the UI it is safe to expose the per-send `to` / `recipients` fields in the direct-send composer for that provider.

## Common provider failure categories

Failures are surfaced as `last_error` on the delivery row. The text is whatever the provider's `Send` returned.

- **HTTP 4xx** (`http 401`, `http 403`, `http 404`) — almost always auth or addressing. 401/403 means the credential (token, API key, webhook signing secret) is wrong, expired, or revoked. 404 on a webhook usually means the webhook was deleted on the provider side or the URL is malformed. Test-send will reproduce this immediately.
- **HTTP 4xx with a body snippet** — for most providers the error message includes the first 512 bytes of the response body (`http 400: invalid_recipient` etc.). That snippet is the actual provider validation error; treat it as the primary signal.
- **HTTP 429** — rate limited. Retries fire 30s later; if the channel is genuinely too hot, raise `max_attempts` or fan out across multiple targets.
- **HTTP 5xx** — provider-side outage. Retries usually clear within the retry window. A persistent 502/503 is provider downtime, not configuration.
- **SMTP `dial tcp: i/o timeout`** — firewall blocks the port, or the host is unreachable from the plugin container. Check from inside the container, not from your laptop.
- **SMTP `535 ... Authentication failed`** — username/password wrong, or the provider requires an app password / OAuth token rather than the account password.
- **SMTP `starttls: x509: certificate signed by unknown authority`** — the relay presents a private CA. Either trust it in the container or switch `tls_mode` to `tls` against a public-CA-served port.
- **SMTP attachment errors** — the row contains a base64 attachment that fails to decode (`decode attachment: illegal base64 data`). The publisher is malformed; do not re-queue.
- **OAuth expiry** (Office365/Graph, SES with role tokens, anything using bearer tokens) — `http 401`. The plugin does not refresh tokens; rotate the stored credential and re-test.
- **Provider-specific signature failures** (Slack, Pushover, Twilio) — `http 400: invalid_signature` or similar. Caused by a credential pair that has drifted (key rotated on one side, not the other).

## Test-send vs production

`POST /api/admin/targets/:id/test` constructs:

```json
{
  "event_name": "plugin.continuum.notifications.test",
  "title": "Continuum test notification",
  "body": "This test was sent from the Notifications admin UI.",
  "payload": {"test": true}
}
```

It validates:

- The provider id is registered.
- Required credential fields are non-empty (per the provider's own `Send`; the registry does not pre-validate).
- The endpoint accepts the canned shape and returns 2xx.
- The recipient is reachable when the target stores its own `to`.

It does **not** validate:

- Template rendering — there is no template, just the literal title/body above.
- Per-send recipient resolution — there is no `target_config["to"]` override applied.
- Payload-driven providers (anything that branches on `payload` keys) against the real event payload shape — the test payload is just `{"test": true}`.
- Rate limits at production traffic. A clean test does not promise the next thousand sends will work.
- Idempotency keys. Test-sends are not queued.

Always follow a green test-send with one production event before declaring victory.
