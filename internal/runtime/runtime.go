package runtime

import (
	"context"
	"fmt"
	"sync"

	pluginv1 "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginsdk/runtimedefault"
)

type Config struct {
	DatabaseURL string `json:"database_url"`
}

type Server struct {
	runtimedefault.Server
	manifest *pluginv1.PluginManifest
	onCfg    func(Config) error
	mu       sync.RWMutex
	cfg      Config
}

func New(manifest *pluginv1.PluginManifest, onConfig func(Config) error) *Server {
	return &Server{manifest: manifest, onCfg: onConfig}
}

func (s *Server) GetManifest(_ context.Context, _ *pluginv1.GetManifestRequest) (*pluginv1.GetManifestResponse, error) {
	return &pluginv1.GetManifestResponse{Manifest: s.manifest}, nil
}

func (s *Server) Configure(_ context.Context, req *pluginv1.ConfigureRequest) (*pluginv1.ConfigureResponse, error) {
	cfg := Config{}
	for _, e := range req.GetConfig() {
		if e.GetKey() != "connection" || e.GetValue() == nil {
			continue
		}
		m := e.GetValue().AsMap()
		if v, _ := m["database_url"].(string); v != "" {
			cfg.DatabaseURL = v
		}
		if v, _ := m["value"].(string); v != "" {
			cfg.DatabaseURL = v
		}
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("database_url is required")
	}
	if s.onCfg != nil {
		if err := s.onCfg(cfg); err != nil {
			return nil, err
		}
	}
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	return &pluginv1.ConfigureResponse{}, nil
}

func (s *Server) Snapshot() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}
