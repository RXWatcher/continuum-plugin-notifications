# Rule Matching and Template Expansion

The matcher lives in [`internal/notify/template.go`](../internal/notify/template.go) and is exercised by the event consumer in [`internal/consumer/consumer.go`](../internal/consumer/consumer.go). It is deliberately small — there is no regex, no payload predicate, no priority, no rate limiting.

## Match semantics

`Match(pattern, event)` returns true when:

| Pattern form | Behaviour | Example |
| --- | --- | --- |
| `"*"` | Matches every event. | `*` matches `library.media_added` and `plugin.continuum.requests.submitted`. |
| Exact string | Matches when `pattern == event`. | `library.media_added` only matches `library.media_added`. |
| Trailing `*` | Matches when `event` has `strings.TrimSuffix(pattern, "*")` as a prefix. | `plugin.continuum.requests.*` matches `plugin.continuum.requests.submitted` and `plugin.continuum.requests.fulfilled` but **not** `plugin.continuum.requests` (no trailing dot in the event). |

There is **no** mid-string `*`, no `?`, and no character class. `plugin.*.requests.submitted` is treated as an exact string and will not match anything in practice.

## Event names the consumer sees

The host only routes events whose names appear in the subscription list in `manifest.json`. The list is broad and ends in `plugin.*`, so any future `plugin.<vendor>.<plugin>.<event>` flows through without redeploying. Host events (`library.*`, `continuum.*`) only arrive if explicitly listed.

A rule pattern of `*` is therefore effectively "every event the manifest subscribes to". If you need to subscribe to a new host event, add it to the manifest — the rule alone is not enough.

## Direct sends bypass matching

Events named `notifications.send`, `plugin.continuum.notifications.send`, or anything ending in `.notifications.send` / `.notification.send` are routed straight to `handleDirectSend` and never consult the rules table. See [direct-send.md](direct-send.md).

## Template expansion

Title and body are run through `notify.Render(template, event, payload)`:

1. Replace every occurrence of `{{event}}` with the event name.
2. Replace every occurrence of `{{summary}}` with the payload "summary" (see below).
3. For every top-level payload key `k`, replace `{{k}}` with `fmt.Sprint(payload[k])`.

The replacement is literal `strings.ReplaceAll`, not parsed. Consequences:

- Nested values are not addressable. `{{user.email}}` is one literal placeholder — there is no key called `user.email`. If you need nested data, flatten it in the publisher.
- Non-string payload values render with Go's `%v`. Maps and slices render in Go's default form (`map[...]` / `[...]`) — useful for debugging, ugly for end users.
- Unknown placeholders are left in the output verbatim. A template body of `{{title}}` against a payload that has no `title` key ships `{{title}}` to the destination.
- An empty template renders as the event name (see `Render`'s early return). To send an actually blank body, set the rule body to a single space.

### `{{summary}}` heuristic

`notify.Summary(payload)` returns the first non-empty value among these payload keys, in order: `summary`, `message`, `title`, `name`, `status`, `reason`. If none are present it falls back to the JSON encoding of the entire payload — which is the right thing during early debugging and the wrong thing once a rule is in production. Override with a concrete template once you know which payload key carries the human-readable text.

## Multiple rules and overlap

Each enabled rule is evaluated independently. Overlapping patterns each enqueue their own delivery rows. There is no de-duplication across rules — that is the operator's responsibility. If you want exactly-once-per-event-per-channel, keep patterns disjoint.

Within a single rule, every target ID in `target_ids` produces one delivery row. A disabled target is silently skipped at enqueue time (the consumer reads the target, sees `Enabled=false`, and continues). Targets that have been deleted return an error from `GetTarget` and are also skipped.

## What does **not** happen at match time

- The consumer does not check user preferences or quiet hours.
- It does not consult contacts. Contacts only apply to direct sends.
- It does not de-duplicate against recent deliveries. The idempotency key index (`deliveries_idempotency_key_idx`) only deduplicates direct sends that supply a non-empty key — rule-driven deliveries write empty idempotency keys.
- It does not throttle or rate limit.

If any of those matter for a given destination, model them upstream (in the publisher) or downstream (in the provider's own rate limits — Slack and Discord will 429 you).
