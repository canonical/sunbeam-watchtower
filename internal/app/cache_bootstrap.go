// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/bugcache"
	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/distrocache"
	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/excusescache"
	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/gitcache"
	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/reviewcache"
	"github.com/canonical/sunbeam-watchtower/internal/config"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// ResolveCacheDir returns the cache directory for sunbeam-watchtower.
// It uses $XDG_CACHE_HOME/sunbeam-watchtower if set, otherwise ~/.cache/sunbeam-watchtower.
func ResolveCacheDir() (string, error) {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("determining home directory: %w", err)
		}
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "sunbeam-watchtower"), nil
}

func cacheSubdir(name string) (string, error) {
	cacheDir, err := ResolveCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, name), nil
}

// DistroCache returns a lazy-initialized distro cache singleton.
func (a *App) DistroCache() (*distrocache.Cache, error) {
	a.distroOnce.Do(func() {
		path, err := cacheSubdir("distro")
		if err != nil {
			a.distroErr = err
			return
		}
		a.distroCache, a.distroErr = distrocache.NewCache(path, a.Logger, a.upstreamHTTPClient("packages", 5*time.Minute))
	})
	return a.distroCache, a.distroErr
}

// GitCache returns a lazy-initialized git cache singleton.
func (a *App) GitCache() (*gitcache.Cache, error) {
	a.gitOnce.Do(func() {
		reposDir, err := cacheSubdir("repos")
		if err != nil {
			a.gitErr = err
			return
		}
		a.Logger.Debug("initializing git cache", "cacheDir", reposDir)
		a.gitCache = gitcache.NewCache(reposDir, a.Logger)
	})
	return a.gitCache, a.gitErr
}

// BugCache returns a lazy-initialized bug cache singleton.
func (a *App) BugCache() (*bugcache.Cache, error) {
	a.bugCacheOnce.Do(func() {
		path, err := cacheSubdir("bugs")
		if err != nil {
			a.bugCacheErr = err
			return
		}
		a.bugCache, a.bugCacheErr = bugcache.NewCache(path, a.Logger)
	})
	return a.bugCache, a.bugCacheErr
}

// ReviewCache returns a lazy-initialized review cache singleton.
func (a *App) ReviewCache() (*reviewcache.Cache, error) {
	a.reviewCacheOnce.Do(func() {
		path, err := cacheSubdir("reviews")
		if err != nil {
			a.reviewCacheErr = err
			return
		}
		a.reviewCache, a.reviewCacheErr = reviewcache.NewCache(path)
	})
	return a.reviewCache, a.reviewCacheErr
}

// ExcusesCache returns a lazy-initialized excuses cache singleton.
func (a *App) ExcusesCache() (*excusescache.Cache, error) {
	a.excusesOnce.Do(func() {
		path, err := cacheSubdir("excuses")
		if err != nil {
			a.excusesErr = err
			return
		}
		a.excusesCache, a.excusesErr = excusescache.NewCache(path, a.ExcusesSources(), a.Logger, a.upstreamHTTPClient("excuses", 2*time.Minute))
	})
	return a.excusesCache, a.excusesErr
}

// ExcusesSources returns the configured excuses trackers, with built-in Ubuntu/Debian
// defaults synthesized for legacy configs during migration.
func (a *App) ExcusesSources() []dto.ExcusesSource {
	if a == nil || a.GetConfig() == nil {
		return dto.KnownExcusesSources()
	}
	return configuredExcusesSources(a.GetConfig())
}

// DefaultExcusesTracker returns the preferred default tracker for list/show commands.
func (a *App) DefaultExcusesTracker() string {
	sources := a.ExcusesSources()
	if _, ok := dto.ExcusesSourceByTracker(sources, dto.ExcusesTrackerUbuntu); ok {
		return dto.ExcusesTrackerUbuntu
	}
	if len(sources) == 0 {
		return dto.ExcusesTrackerUbuntu
	}
	return sources[0].Tracker
}

func configuredExcusesSources(_ *config.Config) []dto.ExcusesSource {
	// URLs are owned by providers; no config override needed.
	return dto.KnownExcusesSources()
}
