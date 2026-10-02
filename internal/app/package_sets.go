// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/packagesetcache"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

var ErrUnknownPackageSet = errors.New("unknown package set")
var ErrPackageSetNotSynced = packagesetcache.ErrNotSynced

func (a *App) packageSetCache() (*packagesetcache.Cache, error) {
	dir, err := cacheSubdir("packagesets")
	if err != nil {
		return nil, err
	}
	return packagesetcache.NewCache(dir, a.upstreamHTTPClient("packagesets", 30*time.Second)), nil
}

// PackageSet resolves static sets and locally cached Launchpad sets without
// fetching from the network. Missing Launchpad snapshots require explicit sync.
func (a *App) PackageSet(name string) ([]string, error) {
	cfg := a.GetConfig().Packages
	static, staticOK := cfg.Sets[name]
	launchpad, launchpadOK := cfg.LaunchpadSets[name]
	if staticOK && launchpadOK {
		return nil, fmt.Errorf("package set %q is configured twice", name)
	}
	if staticOK {
		return append([]string(nil), static...), nil
	}
	if !launchpadOK {
		return nil, fmt.Errorf("%w %q", ErrUnknownPackageSet, name)
	}
	cache, err := a.packageSetCache()
	if err != nil {
		return nil, err
	}
	snapshot, err := cache.Read(name, launchpad.Series)
	if errors.Is(err, packagesetcache.ErrNotSynced) {
		return nil, fmt.Errorf("packageset %q has no cached snapshot; run watchtower cache sync packagesets: %w", name, err)
	}
	if err != nil {
		return nil, err
	}
	return snapshot.Packages, nil
}

// SyncPackageSets refreshes the requested configured Launchpad sets.
func (a *App) SyncPackageSets(ctx context.Context, names []string) ([]packagesetcache.Snapshot, error) {
	cfg := a.GetConfig().Packages
	if len(names) == 0 {
		for name := range cfg.LaunchpadSets {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	// Reject invalid selections before refreshing any snapshot.
	for _, name := range names {
		if _, ok := cfg.LaunchpadSets[name]; !ok {
			return nil, fmt.Errorf("%w %q", ErrUnknownPackageSet, name)
		}
		if _, conflict := cfg.Sets[name]; conflict {
			return nil, fmt.Errorf("package set %q is configured twice", name)
		}
	}
	cache, err := a.packageSetCache()
	if err != nil {
		return nil, err
	}
	var results []packagesetcache.Snapshot
	lastName := ""
	for _, name := range names {
		if name == lastName {
			continue
		}
		lastName = name
		set := cfg.LaunchpadSets[name]
		snapshot, err := cache.Sync(ctx, name, set.Series)
		if err != nil {
			return nil, fmt.Errorf("syncing package set %q: %w", name, err)
		}
		results = append(results, snapshot)
	}
	return results, nil
}

// ClearPackageSets removes locally cached Launchpad packageset snapshots.
func (a *App) ClearPackageSets() error {
	dir, err := cacheSubdir("packagesets")
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

// PackageSetStatuses reports configured Launchpad sets and their cached state.
func (a *App) PackageSetStatuses() ([]dto.PackageSetCacheStatus, error) {
	cfg := a.GetConfig().Packages
	cache, err := a.packageSetCache()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(cfg.LaunchpadSets))
	for name := range cfg.LaunchpadSets {
		names = append(names, name)
	}
	sort.Strings(names)
	statuses := make([]dto.PackageSetCacheStatus, 0, len(names))
	for _, name := range names {
		configured := cfg.LaunchpadSets[name].Series
		status := dto.PackageSetCacheStatus{Name: name, ConfiguredSeries: configured}
		snapshot, err := cache.Read(name, configured)
		if err != nil && !errors.Is(err, packagesetcache.ErrNotSynced) {
			return nil, err
		}
		if err == nil {
			status.Series = snapshot.Series
			status.PackageCount = len(snapshot.Packages)
			status.SyncedAt = snapshot.SyncedAt
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}
