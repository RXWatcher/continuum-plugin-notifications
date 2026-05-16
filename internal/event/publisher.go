// Package event publishes named events into continuum's event hub via the
// SDK's RuntimeHost client. The host stamps `plugin.<plugin_id>.` in front of
// the supplied name, so callers pass the unprefixed leaf (e.g. "submitted",
// "cancelled"). Failures are logged but never bubble up to the caller —
// persisted state is the source of truth.
package event

import (
	"context"

	"github.com/hashicorp/go-hclog"

	"github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginsdk/runtimehost"
)

// Publisher wraps a *runtimehost.Client. Construct once at plugin startup; safe
// for concurrent use.
type Publisher struct {
	host   *runtimehost.Client
	logger hclog.Logger
}

func New(host *runtimehost.Client, logger hclog.Logger) *Publisher {
	if logger == nil {
		logger = hclog.NewNullLogger()
	}
	return &Publisher{host: host, logger: logger}
}

// Publish fires an event into the host. If host is nil (broker not yet
// bound — very brief startup window) or the publish fails, the failure is
// logged and Publish returns. Callers do not need to handle errors.
func (p *Publisher) Publish(ctx context.Context, name string, payload map[string]any) {
	if p.host == nil {
		p.logger.Warn("host not bound; skipping event", "name", name)
		return
	}
	if err := p.host.PublishEvent(ctx, name, payload); err != nil {
		p.logger.Warn("publish event", "name", name, "err", err)
	}
}
