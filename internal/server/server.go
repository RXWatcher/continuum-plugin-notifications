package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hashicorp/go-hclog"

	"github.com/RXWatcher/continuum-plugin-notifications/internal/auth"
	"github.com/RXWatcher/continuum-plugin-notifications/internal/notify"
	"github.com/RXWatcher/continuum-plugin-notifications/internal/store"
)

func errString(msg string) error { return errors.New(msg) }

type Deps struct {
	Store    *store.Store
	WebFS    http.FileSystem
	Logger   hclog.Logger
	Registry func() *notify.Registry
	RunDue   func(context.Context) (int, error)
}

type Server struct{ deps Deps }

type PublicTarget struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Provider     string              `json:"provider"`
	Enabled      bool                `json:"enabled"`
	Capabilities notify.Capabilities `json:"capabilities"`
}

type SendRequest struct {
	TargetID       string              `json:"target_id"`
	TargetName     string              `json:"target_name"`
	To             string              `json:"to"`
	Recipient      RecipientRef        `json:"recipient"`
	Recipients     []RecipientRef      `json:"recipients"`
	Category       string              `json:"category"`
	IdempotencyKey string              `json:"idempotency_key"`
	Title          string              `json:"title"`
	Body           string              `json:"body"`
	EventName      string              `json:"event_name"`
	Payload        map[string]any      `json:"payload"`
	Attachments    []notify.Attachment `json:"attachments"`
	SendNow        bool                `json:"send_now"`
}

type RecipientRef struct {
	Kind        string `json:"kind"`
	Value       string `json:"value"`
	UserID      string `json:"user_id"`
	ContactKind string `json:"contact_kind"`
}

func New(d Deps) *Server { return &Server{deps: d} }

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Route("/api/admin", func(r chi.Router) {
		r.Use(s.requireConfigured)
		r.Get("/status", s.handleStatus)
		r.Get("/providers", s.handleProviders)
		r.Get("/config", s.handleGetConfig)
		r.Put("/config", s.handlePutConfig)
		r.Get("/targets", s.handleListTargets)
		r.Post("/targets", s.handleUpsertTarget)
		r.Put("/targets/{id}", s.handleUpsertTarget)
		r.Delete("/targets/{id}", s.handleDeleteTarget)
		r.Post("/targets/{id}/test", s.handleTestTarget)
		r.Get("/rules", s.handleListRules)
		r.Post("/rules", s.handleUpsertRule)
		r.Put("/rules/{id}", s.handleUpsertRule)
		r.Delete("/rules/{id}", s.handleDeleteRule)
		r.Get("/deliveries", s.handleListDeliveries)
		r.Get("/deliveries/{id}", s.handleGetDelivery)
		r.Post("/deliveries/run", s.handleRunDue)
		r.Get("/contacts", s.handleListContacts)
		r.Post("/contacts", s.handleUpsertContact)
		r.Put("/contacts/{id}", s.handleUpsertContact)
		r.Delete("/contacts/{id}", s.handleDeleteContact)
		r.Get("/preferences/{userID}", s.handleGetPreference)
		r.Put("/preferences/{userID}", s.handlePutPreference)
	})
	r.Route("/api/v1", func(r chi.Router) {
		r.With(s.requireConfigured).Get("/capabilities", s.handleCapabilities)
		r.With(s.requireConfigured).Get("/targets", s.handlePublicTargets)
		r.With(s.requireAdmin, s.requireConfigured).Post("/send", s.handleSend)
		r.With(s.requireConfigured).Get("/deliveries/{id}", s.handleGetDelivery)
	})
	r.Get("/admin", s.handleSPA)
	r.Get("/admin/*", s.handleSPA)
	if s.deps.WebFS != nil {
		r.Get("/assets/*", http.FileServer(s.deps.WebFS).ServeHTTP)
	}
	return r
}

func (s *Server) requireConfigured(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.deps.Store == nil {
			writeError(w, http.StatusServiceUnavailable, "notifications plugin is not configured")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := auth.FromRequest(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing identity")
			return
		}
		if !id.IsAdmin {
			writeError(w, http.StatusForbidden, "admin required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	targets, _ := s.publicTargets(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"providers": s.registry().Catalog(),
		"targets":   targets,
		"recipient_kinds": []string{
			"email", "user", "discord_user", "discord_channel", "telegram_chat", "phone", "target",
		},
		"attachment_contract": map[string]any{
			"data_base64": "small payloads",
			"url":         "planned for large files",
			"storage_ref": "planned for plugin/base-owned files",
		},
	})
}

func (s *Server) handlePublicTargets(w http.ResponseWriter, r *http.Request) {
	out, err := s.publicTargets(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"targets": out})
}

func (s *Server) publicTargets(ctx context.Context) ([]PublicTarget, error) {
	rows, err := s.deps.Store.ListTargets(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PublicTarget, 0, len(rows))
	for _, row := range rows {
		caps := notify.Capabilities{RecipientMode: "configured_target"}
		if p, ok := s.registry().Provider(row.Provider); ok {
			caps = p.Capabilities()
		}
		out = append(out, PublicTarget{ID: row.ID, Name: row.Name, Provider: row.Provider, Enabled: row.Enabled, Capabilities: caps})
	}
	return out, nil
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	var req SendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	target, err := s.resolveSendTarget(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !target.Enabled {
		writeError(w, http.StatusBadRequest, "target disabled")
		return
	}
	target, err = s.overrideTargetForSend(r.Context(), target, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	eventName := req.EventName
	if eventName == "" {
		eventName = "plugin.continuum.notifications.direct"
	}
	payload := req.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	if len(req.Attachments) > 0 {
		payload["attachments"] = req.Attachments
	}
	if to := target.Config["to"]; to != "" {
		payload["target_config"] = map[string]string{"to": to}
	}
	msg := notify.Message{EventName: eventName, Title: req.Title, Body: req.Body, Payload: payload, Attachments: req.Attachments}
	if req.SendNow {
		if err := s.registry().Send(r.Context(), target, msg); err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "delivered"})
		return
	}
	row, err := s.deps.Store.Enqueue(r.Context(), store.Delivery{
		EventName:      eventName,
		TargetID:       target.ID,
		Provider:       target.Provider,
		Title:          req.Title,
		Body:           req.Body,
		Payload:        payload,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "queued", "delivery_id": row.ID})
}

func (s *Server) resolveSendTarget(ctx context.Context, req SendRequest) (store.Target, error) {
	if req.TargetID != "" {
		return s.deps.Store.GetTarget(ctx, req.TargetID)
	}
	if req.TargetName == "" {
		return store.Target{}, errString("target_id or target_name is required")
	}
	rows, err := s.deps.Store.ListTargets(ctx)
	if err != nil {
		return store.Target{}, err
	}
	for _, row := range rows {
		if row.Name == req.TargetName {
			return row, nil
		}
	}
	return store.Target{}, errString("target not found")
}

func (s *Server) overrideTargetForSend(ctx context.Context, target store.Target, req SendRequest) (store.Target, error) {
	if target.Config == nil {
		target.Config = map[string]string{}
	}
	to := req.To
	if to == "" {
		var err error
		to, err = s.resolveRecipients(ctx, req)
		if err != nil {
			return target, err
		}
	}
	if to != "" {
		target.Config["to"] = to
	}
	return target, nil
}

func (s *Server) resolveRecipients(ctx context.Context, req SendRequest) (string, error) {
	refs := req.Recipients
	if req.Recipient != (RecipientRef{}) {
		refs = append(refs, req.Recipient)
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
			contacts, err := s.deps.Store.ListUserContacts(ctx, ref.UserID)
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

func (s *Server) registry() *notify.Registry {
	if s.deps.Registry != nil {
		return s.deps.Registry()
	}
	return notify.NewRegistry(8 * time.Second)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	targets, _ := s.deps.Store.ListTargets(r.Context())
	rules, _ := s.deps.Store.ListRules(r.Context())
	deliveries, _ := s.deps.Store.ListDeliveries(r.Context(), 20)
	writeJSON(w, http.StatusOK, map[string]any{
		"configured":        true,
		"providers":         len(s.registry().Catalog()),
		"targets":           len(targets),
		"rules":             len(rules),
		"recent_deliveries": len(deliveries),
	})
}

func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.registry().Catalog())
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.deps.Store.GetAppConfig(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	var cfg store.AppConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.deps.Store.PutAppConfig(r.Context(), cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handleListTargets(w http.ResponseWriter, r *http.Request) {
	rows, err := s.deps.Store.ListTargets(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) handleUpsertTarget(w http.ResponseWriter, r *http.Request) {
	var t store.Target
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if id := chi.URLParam(r, "id"); id != "" {
		t.ID = id
	}
	row, err := s.deps.Store.UpsertTarget(r.Context(), t)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) handleDeleteTarget(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Store.DeleteTarget(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTestTarget(w http.ResponseWriter, r *http.Request) {
	t, err := s.deps.Store.GetTarget(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	msg := notify.Message{EventName: "plugin.continuum.notifications.test", Title: "Continuum test notification", Body: "This test was sent from the Notifications admin UI.", Payload: map[string]any{"test": true}}
	if err := s.registry().Send(r.Context(), t, msg); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleListRules(w http.ResponseWriter, r *http.Request) {
	rows, err := s.deps.Store.ListRules(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) handleUpsertRule(w http.ResponseWriter, r *http.Request) {
	var row store.Rule
	if err := json.NewDecoder(r.Body).Decode(&row); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if id := chi.URLParam(r, "id"); id != "" {
		row.ID = id
	}
	out, err := s.deps.Store.UpsertRule(r.Context(), row)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Store.DeleteRule(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListDeliveries(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.deps.Store.ListDeliveries(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) handleGetDelivery(w http.ResponseWriter, r *http.Request) {
	row, err := s.deps.Store.GetDelivery(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) handleListContacts(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	rows, err := s.deps.Store.ListUserContacts(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) handleUpsertContact(w http.ResponseWriter, r *http.Request) {
	var row store.UserContact
	if err := json.NewDecoder(r.Body).Decode(&row); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if id := chi.URLParam(r, "id"); id != "" {
		row.ID = id
	}
	out, err := s.deps.Store.UpsertUserContact(r.Context(), row)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteContact(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Store.DeleteUserContact(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetPreference(w http.ResponseWriter, r *http.Request) {
	row, err := s.deps.Store.GetUserPreference(r.Context(), chi.URLParam(r, "userID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) handlePutPreference(w http.ResponseWriter, r *http.Request) {
	var row store.UserPreference
	if err := json.NewDecoder(r.Body).Decode(&row); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	row.UserID = chi.URLParam(r, "userID")
	out, err := s.deps.Store.PutUserPreference(r.Context(), row)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleRunDue(w http.ResponseWriter, r *http.Request) {
	if s.deps.RunDue == nil {
		writeJSON(w, http.StatusOK, map[string]any{"processed": 0})
		return
	}
	n, err := s.deps.RunDue(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"processed": n})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"message": msg}})
}
