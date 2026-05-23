package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	goruntime "runtime"
	"sync/atomic"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/jackc/pgx/v5/pgxpool"

	pluginv1 "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	publicmanifest "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginsdk/manifest"
	sdkruntime "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginsdk/runtime"

	"github.com/RXWatcher/silo-plugin-notifications/internal/consumer"
	"github.com/RXWatcher/silo-plugin-notifications/internal/httproutes"
	"github.com/RXWatcher/silo-plugin-notifications/internal/migrate"
	"github.com/RXWatcher/silo-plugin-notifications/internal/notify"
	"github.com/RXWatcher/silo-plugin-notifications/internal/poll"
	pluginrt "github.com/RXWatcher/silo-plugin-notifications/internal/runtime"
	"github.com/RXWatcher/silo-plugin-notifications/internal/server"
	"github.com/RXWatcher/silo-plugin-notifications/internal/store"
	"github.com/RXWatcher/silo-plugin-notifications/web"
)

//go:embed manifest.json
var manifestRaw []byte

func main() {
	logger := hclog.New(&hclog.LoggerOptions{Name: "silo-plugin-notifications"})
	manifest, err := loadManifest()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load manifest: %v\n", err)
		os.Exit(1)
	}

	httpSrv := httproutes.NewServer()
	var (
		poolPtr     atomic.Pointer[pgxpool.Pool]
		storePtr    atomic.Pointer[store.Store]
		registryPtr atomic.Pointer[notify.Registry]
	)

	poller := poll.New(func() *poll.Deps {
		s := storePtr.Load()
		reg := registryPtr.Load()
		if s == nil || reg == nil {
			return nil
		}
		return &poll.Deps{Store: s, Registry: reg}
	}, logger.Named("worker"))
	pollMgr := &poll.Manager{}

	rt := pluginrt.New(manifest, func(cfg pluginrt.Config) error {
		ctx := context.Background()
		pcfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
		if err != nil {
			return fmt.Errorf("parse db: %w", err)
		}
		if pcfg.MaxConns < 16 {
			pcfg.MaxConns = 16
		}
		p, err := pgxpool.NewWithConfig(ctx, pcfg)
		if err != nil {
			return fmt.Errorf("pgxpool: %w", err)
		}
		if err := migrate.Run(ctx, cfg.DatabaseURL); err != nil {
			p.Close()
			return fmt.Errorf("migrate: %w", err)
		}

		s := store.New(p)
		appCfg, _ := s.GetAppConfig(ctx)
		reg := notify.NewRegistry(time.Duration(appCfg.TimeoutMS) * time.Millisecond)

		storePtr.Store(s)
		registryPtr.Store(reg)

		srv := server.New(server.Deps{
			Store:  s,
			WebFS:  web.FS(),
			Logger: logger.Named("admin"),
			Registry: func() *notify.Registry {
				if r := registryPtr.Load(); r != nil {
					return r
				}
				return notify.NewRegistry(8 * time.Second)
			},
			RunDue: poller.Tick,
		})
		httpSrv.SetHandler(srv.Handler())
		pollMgr.Start(context.Background(), poller, 30*time.Second)

		if old := poolPtr.Swap(p); old != nil {
			old.Close()
		}
		return nil
	})

	cons := consumer.New(func() *consumer.Deps {
		s := storePtr.Load()
		if s == nil {
			return nil
		}
		return &consumer.Deps{Store: s}
	}, logger.Named("consumer"))

	sdkruntime.Serve(sdkruntime.ServeConfig{
		Logger: logger,
		Servers: sdkruntime.CapabilityServers{
			Runtime:       rt,
			HttpRoutes:    httpSrv,
			EventConsumer: cons,
			ScheduledTask: &poll.ScheduledServer{Poller: poller},
		},
	})
}

func loadManifest() (*pluginv1.PluginManifest, error) {
	manifest, err := publicmanifest.Load(manifestRaw)
	if err != nil {
		return nil, fmt.Errorf("load embedded manifest: %w", err)
	}
	executablePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable path: %w", err)
	}
	binaryData, err := os.ReadFile(executablePath)
	if err != nil {
		return nil, fmt.Errorf("read executable %q: %w", executablePath, err)
	}
	checksum := sha256.Sum256(binaryData)
	manifest.Checksum = hex.EncodeToString(checksum[:])
	if len(manifest.GetSupportedPlatforms()) == 0 {
		manifest.SupportedPlatforms = []*pluginv1.SupportedPlatform{{Os: goruntime.GOOS, Arch: goruntime.GOARCH}}
	}
	return manifest, nil
}
