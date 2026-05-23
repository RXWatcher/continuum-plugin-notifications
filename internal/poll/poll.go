package poll

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hashicorp/go-hclog"

	"github.com/RXWatcher/silo-plugin-notifications/internal/notify"
	"github.com/RXWatcher/silo-plugin-notifications/internal/store"
)

type Deps struct {
	Store    *store.Store
	Registry *notify.Registry
}

type Poller struct {
	deps   func() *Deps
	logger hclog.Logger
}

func New(deps func() *Deps, logger hclog.Logger) *Poller {
	if logger == nil {
		logger = hclog.NewNullLogger()
	}
	return &Poller{deps: deps, logger: logger}
}

func (p *Poller) Tick(ctx context.Context) (int, error) {
	d := p.deps()
	if d == nil || d.Store == nil || d.Registry == nil {
		return 0, nil
	}
	cfg, _ := d.Store.GetAppConfig(ctx)
	rows, err := d.Store.DueDeliveries(ctx, 25)
	if err != nil {
		return 0, err
	}
	for _, row := range rows {
		target, err := d.Store.GetTarget(ctx, row.TargetID)
		if err != nil {
			_ = d.Store.MarkFailed(ctx, row.ID, err.Error(), row.Attempts+1 < cfg.MaxAttempts)
			continue
		}
		if !target.Enabled {
			_ = d.Store.MarkFailed(ctx, row.ID, "target disabled", false)
			continue
		}
		target = applyTargetConfigOverrides(target, row.Payload)
		msg := notify.Message{EventName: row.EventName, Title: row.Title, Body: row.Body, Payload: row.Payload}
		if raw, ok := row.Payload["attachments"]; ok {
			b, _ := json.Marshal(raw)
			_ = json.Unmarshal(b, &msg.Attachments)
		}
		if err := d.Registry.Send(ctx, target, msg); err != nil {
			p.logger.Warn("notification delivery failed", "id", row.ID, "target", target.Name, "err", err)
			_ = d.Store.MarkFailed(ctx, row.ID, err.Error(), row.Attempts+1 < cfg.MaxAttempts)
			continue
		}
		_ = d.Store.MarkDelivered(ctx, row.ID)
	}
	return len(rows), nil
}

func applyTargetConfigOverrides(target store.Target, payload map[string]any) store.Target {
	raw, ok := payload["target_config"]
	if !ok {
		return target
	}
	b, _ := json.Marshal(raw)
	overrides := map[string]string{}
	if err := json.Unmarshal(b, &overrides); err != nil {
		return target
	}
	if target.Config == nil {
		target.Config = map[string]string{}
	}
	for key, value := range overrides {
		if value != "" {
			target.Config[key] = value
		}
	}
	return target
}

type Manager struct {
	cancel context.CancelFunc
}

func (m *Manager) Start(ctx context.Context, p *Poller, every time.Duration) {
	if m.cancel != nil {
		m.cancel()
	}
	if every <= 0 {
		every = 30 * time.Second
	}
	cctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			_, _ = p.Tick(cctx)
			select {
			case <-cctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}
