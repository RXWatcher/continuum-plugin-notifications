package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/hashicorp/go-hclog"

	"github.com/RXWatcher/silo-plugin-notifications/internal/notify"
	"github.com/RXWatcher/silo-plugin-notifications/internal/store"
	pluginv1 "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
)

type Deps struct {
	Store *store.Store
}

type Consumer struct {
	pluginv1.UnimplementedEventConsumerServer
	deps   func() *Deps
	logger hclog.Logger
}

func New(deps func() *Deps, logger hclog.Logger) *Consumer {
	if logger == nil {
		logger = hclog.NewNullLogger()
	}
	return &Consumer{deps: deps, logger: logger}
}

func (c *Consumer) HandleEvent(ctx context.Context, req *pluginv1.HandleEventRequest) (*pluginv1.HandleEventResponse, error) {
	d := c.deps()
	if d == nil || d.Store == nil {
		return &pluginv1.HandleEventResponse{}, nil
	}
	event := req.GetEventName()
	payload := map[string]any{}
	if req.GetPayload() != nil {
		payload = req.GetPayload().AsMap()
	}
	if isDirectSendEvent(event) {
		if err := handleDirectSend(ctx, d.Store, payload); err != nil {
			return nil, err
		}
		return &pluginv1.HandleEventResponse{}, nil
	}
	rules, err := d.Store.ListRules(ctx)
	if err != nil {
		return nil, err
	}
	queued := 0
	for _, rule := range rules {
		if !rule.Enabled || !notify.Match(rule.EventPattern, event) {
			continue
		}
		title := notify.Render(rule.Title, event, payload)
		body := notify.Render(rule.Body, event, payload)
		for _, targetID := range rule.TargetIDs {
			target, err := d.Store.GetTarget(ctx, targetID)
			if err != nil || !target.Enabled {
				continue
			}
			if _, err := d.Store.Enqueue(ctx, store.Delivery{
				EventName: event,
				TargetID:  target.ID,
				Provider:  target.Provider,
				Title:     title,
				Body:      body,
				Payload:   payload,
			}); err != nil {
				return nil, err
			}
			queued++
		}
	}
	if queued > 0 {
		c.logger.Info("queued notifications", "event", event, "count", queued)
	}
	return &pluginv1.HandleEventResponse{}, nil
}

func isDirectSendEvent(event string) bool {
	return event == "notifications.send" ||
		event == "plugin.silo.notifications.send" ||
		strings.HasSuffix(event, ".notifications.send") ||
		strings.HasSuffix(event, ".notification.send")
}

type recipientRef struct {
	Kind        string `json:"kind"`
	Value       string `json:"value"`
	UserID      string `json:"user_id"`
	ContactKind string `json:"contact_kind"`
}

func handleDirectSend(ctx context.Context, s *store.Store, payload map[string]any) error {
	raw, _ := json.Marshal(payload)
	var req struct {
		TargetID       string              `json:"target_id"`
		TargetName     string              `json:"target_name"`
		To             string              `json:"to"`
		Recipient      recipientRef        `json:"recipient"`
		Recipients     []recipientRef      `json:"recipients"`
		Category       string              `json:"category"`
		IdempotencyKey string              `json:"idempotency_key"`
		Title          string              `json:"title"`
		Body           string              `json:"body"`
		EventName      string              `json:"event_name"`
		Payload        map[string]any      `json:"payload"`
		Attachments    []notify.Attachment `json:"attachments"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return err
	}
	if req.Title == "" {
		req.Title = "Silo notification"
	}
	if req.EventName == "" {
		req.EventName = "plugin.silo.notifications.direct"
	}
	if req.Payload == nil {
		req.Payload = map[string]any{}
	}
	if req.Category != "" {
		req.Payload["category"] = req.Category
	}
	if len(req.Attachments) > 0 {
		req.Payload["attachments"] = req.Attachments
	}
	target, err := resolveTarget(ctx, s, req.TargetID, req.TargetName)
	if err != nil {
		return err
	}
	if !target.Enabled {
		return errors.New("target disabled")
	}
	to := req.To
	if to == "" {
		to, err = resolveRecipients(ctx, s, req.Recipient, req.Recipients)
		if err != nil {
			return err
		}
	}
	if to != "" {
		req.Payload["target_config"] = map[string]string{"to": to}
	}
	_, err = s.Enqueue(ctx, store.Delivery{
		EventName:      req.EventName,
		TargetID:       target.ID,
		Provider:       target.Provider,
		Title:          req.Title,
		Body:           req.Body,
		Payload:        req.Payload,
		IdempotencyKey: req.IdempotencyKey,
	})
	return err
}

func resolveRecipients(ctx context.Context, s *store.Store, single recipientRef, many []recipientRef) (string, error) {
	refs := many
	if single != (recipientRef{}) {
		refs = append(refs, single)
	}
	var out []string
	for _, ref := range refs {
		switch ref.Kind {
		case "", "email":
			if ref.Value != "" {
				out = append(out, ref.Value)
			}
		case "user":
			kind := ref.ContactKind
			if kind == "" {
				kind = "email"
			}
			contacts, err := s.ListUserContacts(ctx, ref.UserID)
			if err != nil {
				return "", err
			}
			for _, contact := range contacts {
				if contact.Enabled && contact.Kind == kind && contact.Value != "" {
					out = append(out, contact.Value)
				}
			}
		default:
			if ref.Value != "" {
				out = append(out, ref.Value)
			}
		}
	}
	return strings.Join(out, ", "), nil
}

func resolveTarget(ctx context.Context, s *store.Store, targetID, targetName string) (store.Target, error) {
	if targetID != "" {
		return s.GetTarget(ctx, targetID)
	}
	if targetName == "" {
		return store.Target{}, jsonErr("target_id or target_name is required")
	}
	rows, err := s.ListTargets(ctx)
	if err != nil {
		return store.Target{}, err
	}
	for _, row := range rows {
		if row.Name == targetName {
			return row, nil
		}
	}
	return store.Target{}, jsonErr("target not found")
}

func jsonErr(msg string) error { return errors.New(msg) }
