package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	StatusQueued    = "queued"
	StatusDelivered = "delivered"
	StatusFailed    = "failed"
)

type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store  { return &Store{pool: pool} }
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

type Target struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Provider  string            `json:"provider"`
	Enabled   bool              `json:"enabled"`
	Config    map[string]string `json:"config"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type Rule struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	EventPattern string    `json:"event_pattern"`
	TargetIDs    []string  `json:"target_ids"`
	Enabled      bool      `json:"enabled"`
	Title        string    `json:"title"`
	Body         string    `json:"body"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Delivery struct {
	ID             string         `json:"id"`
	EventName      string         `json:"event_name"`
	TargetID       string         `json:"target_id"`
	Provider       string         `json:"provider"`
	Title          string         `json:"title"`
	Body           string         `json:"body"`
	Payload        map[string]any `json:"payload"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	Status         string         `json:"status"`
	Attempts       int            `json:"attempts"`
	LastError      string         `json:"last_error,omitempty"`
	NextRunAt      time.Time      `json:"next_run_at"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type UserContact struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Kind      string    `json:"kind"`
	Value     string    `json:"value"`
	Label     string    `json:"label"`
	Enabled   bool      `json:"enabled"`
	Verified  bool      `json:"verified"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UserPreference struct {
	UserID        string         `json:"user_id"`
	Enabled       bool           `json:"enabled"`
	Categories    []string       `json:"categories"`
	QuietHours    map[string]any `json:"quiet_hours"`
	PreferredKind string         `json:"preferred_kind"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

type AppConfig struct {
	MaxAttempts int `json:"max_attempts"`
	TimeoutMS   int `json:"timeout_ms"`
}

func DefaultAppConfig() AppConfig { return AppConfig{MaxAttempts: 3, TimeoutMS: 8000} }

func (s *Store) GetAppConfig(ctx context.Context) (AppConfig, error) {
	cfg := DefaultAppConfig()
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT data FROM app_config WHERE id=1`).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	_ = json.Unmarshal(raw, &cfg)
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.TimeoutMS <= 0 {
		cfg.TimeoutMS = 8000
	}
	return cfg, nil
}

func (s *Store) PutAppConfig(ctx context.Context, cfg AppConfig) error {
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO app_config (id, data, updated_at) VALUES (1, $1, now())
ON CONFLICT (id) DO UPDATE SET data=excluded.data, updated_at=now()`, b)
	return err
}

func (s *Store) ListTargets(ctx context.Context) ([]Target, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,name,provider,enabled,config,created_at,updated_at FROM targets ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Target, 0)
	for rows.Next() {
		var t Target
		var raw []byte
		if err := rows.Scan(&t.ID, &t.Name, &t.Provider, &t.Enabled, &raw, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &t.Config)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) GetTarget(ctx context.Context, id string) (Target, error) {
	var t Target
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT id,name,provider,enabled,config,created_at,updated_at FROM targets WHERE id=$1`, id).
		Scan(&t.ID, &t.Name, &t.Provider, &t.Enabled, &raw, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return t, err
	}
	_ = json.Unmarshal(raw, &t.Config)
	return t, nil
}

func (s *Store) UpsertTarget(ctx context.Context, t Target) (Target, error) {
	if t.Name == "" || t.Provider == "" {
		return t, fmt.Errorf("name and provider are required")
	}
	b, err := json.Marshal(t.Config)
	if err != nil {
		return t, err
	}
	if t.ID == "" {
		err = s.pool.QueryRow(ctx, `INSERT INTO targets (name,provider,enabled,config) VALUES ($1,$2,$3,$4)
RETURNING id,created_at,updated_at`, t.Name, t.Provider, t.Enabled, b).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
	} else {
		err = s.pool.QueryRow(ctx, `UPDATE targets SET name=$2, provider=$3, enabled=$4, config=$5, updated_at=now()
WHERE id=$1 RETURNING created_at,updated_at`, t.ID, t.Name, t.Provider, t.Enabled, b).Scan(&t.CreatedAt, &t.UpdatedAt)
	}
	return t, err
}

func (s *Store) DeleteTarget(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM targets WHERE id=$1`, id)
	return err
}

func (s *Store) ListRules(ctx context.Context) ([]Rule, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,name,event_pattern,target_ids,enabled,title,body,created_at,updated_at FROM rules ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Rule, 0)
	for rows.Next() {
		var r Rule
		if err := rows.Scan(&r.ID, &r.Name, &r.EventPattern, &r.TargetIDs, &r.Enabled, &r.Title, &r.Body, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) UpsertRule(ctx context.Context, r Rule) (Rule, error) {
	if r.Name == "" || r.EventPattern == "" || len(r.TargetIDs) == 0 {
		return r, fmt.Errorf("name, event_pattern, and target_ids are required")
	}
	if r.ID == "" {
		err := s.pool.QueryRow(ctx, `INSERT INTO rules (name,event_pattern,target_ids,enabled,title,body) VALUES ($1,$2,$3,$4,$5,$6)
RETURNING id,created_at,updated_at`, r.Name, r.EventPattern, r.TargetIDs, r.Enabled, r.Title, r.Body).Scan(&r.ID, &r.CreatedAt, &r.UpdatedAt)
		return r, err
	}
	err := s.pool.QueryRow(ctx, `UPDATE rules SET name=$2,event_pattern=$3,target_ids=$4,enabled=$5,title=$6,body=$7,updated_at=now()
WHERE id=$1 RETURNING created_at,updated_at`, r.ID, r.Name, r.EventPattern, r.TargetIDs, r.Enabled, r.Title, r.Body).Scan(&r.CreatedAt, &r.UpdatedAt)
	return r, err
}

func (s *Store) DeleteRule(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM rules WHERE id=$1`, id)
	return err
}

func (s *Store) Enqueue(ctx context.Context, d Delivery) (Delivery, error) {
	if d.NextRunAt.IsZero() {
		d.NextRunAt = time.Now()
	}
	b, err := json.Marshal(d.Payload)
	if err != nil {
		return d, err
	}
	err = s.pool.QueryRow(ctx, `INSERT INTO deliveries (event_name,target_id,provider,title,body,payload,status,next_run_at,idempotency_key)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (idempotency_key) WHERE idempotency_key <> ''
DO UPDATE SET idempotency_key=deliveries.idempotency_key
RETURNING id,status,created_at,updated_at`,
		d.EventName, d.TargetID, d.Provider, d.Title, d.Body, b, StatusQueued, d.NextRunAt, d.IdempotencyKey).
		Scan(&d.ID, &d.Status, &d.CreatedAt, &d.UpdatedAt)
	if d.Status == "" {
		d.Status = StatusQueued
	}
	return d, err
}

func (s *Store) DueDeliveries(ctx context.Context, limit int) ([]Delivery, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,event_name,target_id,provider,title,body,payload,idempotency_key,status,attempts,last_error,next_run_at,created_at,updated_at
FROM deliveries WHERE status=$1 AND next_run_at <= now() ORDER BY created_at LIMIT $2`, StatusQueued, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDeliveries(rows)
}

func (s *Store) ListDeliveries(ctx context.Context, limit int) ([]Delivery, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,event_name,target_id,provider,title,body,payload,idempotency_key,status,attempts,last_error,next_run_at,created_at,updated_at
FROM deliveries ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDeliveries(rows)
}

func (s *Store) GetDelivery(ctx context.Context, id string) (Delivery, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,event_name,target_id,provider,title,body,payload,idempotency_key,status,attempts,last_error,next_run_at,created_at,updated_at
FROM deliveries WHERE id=$1`, id)
	if err != nil {
		return Delivery{}, err
	}
	defer rows.Close()
	deliveries, err := scanDeliveries(rows)
	if err != nil {
		return Delivery{}, err
	}
	if len(deliveries) == 0 {
		return Delivery{}, pgx.ErrNoRows
	}
	return deliveries[0], nil
}

func scanDeliveries(rows pgx.Rows) ([]Delivery, error) {
	out := make([]Delivery, 0)
	for rows.Next() {
		var d Delivery
		var raw []byte
		if err := rows.Scan(&d.ID, &d.EventName, &d.TargetID, &d.Provider, &d.Title, &d.Body, &raw, &d.IdempotencyKey, &d.Status, &d.Attempts, &d.LastError, &d.NextRunAt, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &d.Payload)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) ListUserContacts(ctx context.Context, userID string) ([]UserContact, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,user_id,kind,value,label,enabled,verified,created_at,updated_at FROM user_contacts
WHERE ($1='' OR user_id=$1) ORDER BY user_id, kind, value`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]UserContact, 0)
	for rows.Next() {
		var c UserContact
		if err := rows.Scan(&c.ID, &c.UserID, &c.Kind, &c.Value, &c.Label, &c.Enabled, &c.Verified, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) UpsertUserContact(ctx context.Context, c UserContact) (UserContact, error) {
	if c.UserID == "" || c.Kind == "" || c.Value == "" {
		return c, fmt.Errorf("user_id, kind, and value are required")
	}
	if c.ID == "" {
		err := s.pool.QueryRow(ctx, `INSERT INTO user_contacts (user_id,kind,value,label,enabled,verified)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (user_id,kind,value) DO UPDATE SET label=excluded.label, enabled=excluded.enabled, verified=excluded.verified, updated_at=now()
RETURNING id,created_at,updated_at`, c.UserID, c.Kind, c.Value, c.Label, c.Enabled, c.Verified).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
		return c, err
	}
	err := s.pool.QueryRow(ctx, `UPDATE user_contacts SET user_id=$2,kind=$3,value=$4,label=$5,enabled=$6,verified=$7,updated_at=now()
WHERE id=$1 RETURNING created_at,updated_at`, c.ID, c.UserID, c.Kind, c.Value, c.Label, c.Enabled, c.Verified).Scan(&c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (s *Store) DeleteUserContact(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM user_contacts WHERE id=$1`, id)
	return err
}

func (s *Store) GetUserPreference(ctx context.Context, userID string) (UserPreference, error) {
	p := UserPreference{UserID: userID, Enabled: true, QuietHours: map[string]any{}}
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT user_id,enabled,categories,quiet_hours,preferred_kind,updated_at FROM user_preferences WHERE user_id=$1`, userID).
		Scan(&p.UserID, &p.Enabled, &p.Categories, &raw, &p.PreferredKind, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	_ = json.Unmarshal(raw, &p.QuietHours)
	if p.QuietHours == nil {
		p.QuietHours = map[string]any{}
	}
	return p, nil
}

func (s *Store) PutUserPreference(ctx context.Context, p UserPreference) (UserPreference, error) {
	if p.UserID == "" {
		return p, fmt.Errorf("user_id is required")
	}
	if p.QuietHours == nil {
		p.QuietHours = map[string]any{}
	}
	raw, err := json.Marshal(p.QuietHours)
	if err != nil {
		return p, err
	}
	err = s.pool.QueryRow(ctx, `INSERT INTO user_preferences (user_id,enabled,categories,quiet_hours,preferred_kind)
VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (user_id) DO UPDATE SET enabled=excluded.enabled,categories=excluded.categories,quiet_hours=excluded.quiet_hours,preferred_kind=excluded.preferred_kind,updated_at=now()
RETURNING updated_at`, p.UserID, p.Enabled, p.Categories, raw, p.PreferredKind).Scan(&p.UpdatedAt)
	return p, err
}

func (s *Store) MarkDelivered(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE deliveries SET status=$2, attempts=attempts+1, last_error='', updated_at=now() WHERE id=$1`, id, StatusDelivered)
	return err
}

func (s *Store) MarkFailed(ctx context.Context, id, msg string, retry bool) error {
	status := StatusFailed
	next := time.Now()
	if retry {
		status = StatusQueued
		next = time.Now().Add(30 * time.Second)
	}
	_, err := s.pool.Exec(ctx, `UPDATE deliveries SET status=$2, attempts=attempts+1, last_error=$3, next_run_at=$4, updated_at=now() WHERE id=$1`, id, status, msg, next)
	return err
}
