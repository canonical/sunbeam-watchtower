// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/bugcache"
	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/distrocache"
	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/excusescache"
	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/gitcache"
	oteladapter "github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/otel"
	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/releasecache"
	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/reviewcache"
	"github.com/canonical/sunbeam-watchtower/internal/config"
	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	"github.com/canonical/sunbeam-watchtower/internal/core/service/artifactdiscovery"
	opsvc "github.com/canonical/sunbeam-watchtower/internal/core/service/operation"
	"github.com/canonical/sunbeam-watchtower/internal/core/service/teamsync"
)

var ErrLaunchpadAuthRequired = errors.New("launchpad authentication required")

// RuntimeMode selects how frontend/server state should be managed.
type RuntimeMode string

const (
	// RuntimeModePersistent is intended for long-lived server processes.
	RuntimeModePersistent RuntimeMode = "persistent"
	// RuntimeModeEphemeral is intended for short-lived embedded frontends.
	RuntimeModeEphemeral RuntimeMode = "ephemeral"
)

// Options configures runtime-specific application behavior.
type Options struct {
	RuntimeMode RuntimeMode
	ConfigPath  string
}

// App holds shared application state and provides lazy-initialized factories
// for services and adapters. Both the CLI and HTTP API use this layer.
type App struct {
	config     *config.Config
	configMu   sync.RWMutex
	configPath string

	Logger      *slog.Logger
	runtimeMode RuntimeMode

	distroOnce  sync.Once
	distroCache *distrocache.Cache
	distroErr   error

	gitOnce  sync.Once
	gitCache *gitcache.Cache
	gitErr   error

	bugCacheOnce sync.Once
	bugCache     *bugcache.Cache
	bugCacheErr  error

	lpCredsOnce  sync.Once
	lpCredsStore port.LaunchpadCredentialStore

	ghCredsOnce  sync.Once
	ghCredsStore port.GitHubCredentialStore

	snapStoreCredsOnce  sync.Once
	snapStoreCredsStore port.SnapStoreCredentialStore

	charmhubCredsOnce  sync.Once
	charmhubCredsStore port.CharmhubCredentialStore

	lpFlowOnce  sync.Once
	lpFlowStore port.LaunchpadPendingAuthFlowStore

	ghFlowOnce  sync.Once
	ghFlowStore port.GitHubPendingAuthFlowStore

	excusesOnce  sync.Once
	excusesCache *excusescache.Cache
	excusesErr   error

	releaseCacheOnce sync.Once
	releaseCache     *releasecache.Cache
	releaseCacheErr  error

	reviewCacheOnce sync.Once
	reviewCache     *reviewcache.Cache
	reviewCacheErr  error

	operationStoreOnce sync.Once
	operationStore     port.OperationStore

	operationServiceOnce sync.Once
	operationService     *opsvc.Service

	telemetryOnce sync.Once
	telemetry     *oteladapter.Telemetry
	telemetryErr  error

	teamSyncServiceOnce sync.Once
	teamSyncService     *teamsync.Service
	teamSyncServiceErr  error

	artifactDiscoveryOnce sync.Once
	artifactDiscoverySvc  *artifactdiscovery.Service
}

// NewApp creates a new App instance.
func NewApp(cfg *config.Config, logger *slog.Logger) *App {
	return NewAppWithOptions(cfg, logger, Options{RuntimeMode: RuntimeModePersistent})
}

// NewAppWithOptions creates a new App instance with explicit runtime behavior.
func NewAppWithOptions(cfg *config.Config, logger *slog.Logger, opts Options) *App {
	mode := opts.RuntimeMode
	if mode == "" {
		mode = RuntimeModePersistent
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &App{config: cfg, configPath: opts.ConfigPath, Logger: logger, runtimeMode: mode}
}

// GetConfig returns the current configuration, safe for concurrent use.
func (a *App) GetConfig() *config.Config {
	a.configMu.RLock()
	defer a.configMu.RUnlock()
	return a.config
}

// ReloadConfig loads, validates, and atomically swaps the active configuration.
func (a *App) ReloadConfig(path string) error {
	cfg, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("loading config from %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validating config from %s: %w", path, err)
	}

	a.configMu.Lock()
	old := a.config
	a.config = cfg
	a.configMu.Unlock()

	a.Logger.Info("configuration reloaded",
		"path", path,
		"projects_before", len(projectNames(old)),
		"projects_after", len(projectNames(cfg)),
	)
	return nil
}

// ConfigPath returns the filesystem path used to load the configuration.
func (a *App) ConfigPath() string {
	return a.configPath
}

// projectNames returns project names from a config, or nil if cfg is nil.
func projectNames(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	names := make([]string, len(cfg.Projects))
	for i, p := range cfg.Projects {
		names[i] = p.Name
	}
	return names
}

// Close releases resources held by the App (e.g. distro cache).
func (a *App) Close() error {
	var errs []error
	if a.distroCache != nil {
		if err := a.distroCache.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if a.bugCache != nil {
		if err := a.bugCache.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if a.excusesCache != nil {
		if err := a.excusesCache.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if a.releaseCache != nil {
		if err := a.releaseCache.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if a.reviewCache != nil {
		if err := a.reviewCache.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if closer, ok := a.lpFlowStore.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if closer, ok := a.ghFlowStore.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if closer, ok := a.operationStore.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if a.telemetry != nil {
		if err := a.telemetry.Shutdown(context.Background()); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
