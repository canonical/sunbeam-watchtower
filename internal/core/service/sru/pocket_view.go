// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package sru

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	distro "github.com/canonical/sunbeam-watchtower/pkg/distro/v1"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

var ErrStagingInventoryUnavailable = errors.New("cached staging inventory unavailable")

// CachedVersionPocket selects configured suites/components in one cache group.
type CachedVersionPocket struct {
	Name    string
	Label   string
	Entries []dto.SourceEntry
}

// CachedPocketView reads existing indexes without refreshing or accessing upstream.
// Suite/component evidence is required because a cache group may be partially synced.
func CachedPocketView(ctx context.Context, cache port.DistroCache, series, base, parent string, pockets []CachedVersionPocket) (*dto.SRUPocketView, error) {
	if len(pockets) != 4 {
		return nil, fmt.Errorf("expected parent, staging, proposed and updates selections")
	}
	statuses, err := cache.Status()
	if err != nil {
		return nil, err
	}
	statusByName := map[string]dto.CacheStatus{}
	for _, status := range statuses {
		statusByName[status.Name] = status
	}
	data := map[string][]distro.SourcePackage{}
	usedStatuses := []dto.CacheStatus{}
	for _, pocket := range pockets {
		if _, exists := data[pocket.Name]; exists {
			continue
		}
		if status, exists := statusByName[pocket.Name]; exists && !status.LastUpdated.IsZero() {
			items, err := cache.Query(ctx, pocket.Name, dto.QueryOpts{})
			if err != nil {
				return nil, err
			}
			data[pocket.Name] = items
			usedStatuses = append(usedStatuses, status)
		} else {
			data[pocket.Name] = nil
		}
	}
	versions := make([]map[string]string, len(pockets))
	warnings := make([]string, len(pockets))
	for i, pocket := range pockets {
		versions[i] = map[string]string{}
		if len(pocket.Entries) == 0 {
			warnings[i] = "required suite/component is not configured"
			continue
		}
		for _, entry := range pocket.Entries {
			covered := false
			for _, item := range data[pocket.Name] {
				if item.Suite != entry.Suite || item.Component != entry.Component {
					continue
				}
				covered = true
				if versions[i][item.Package] == "" || distro.CompareVersions(item.Version, versions[i][item.Package]) > 0 {
					versions[i][item.Package] = item.Version
				}
			}
			if !covered {
				warnings[i] = fmt.Sprintf("no cached coverage evidence for %s %s/%s; run cache sync package-index", pocket.Name, entry.Suite, entry.Component)
			}
		}
	}
	if warnings[1] != "" {
		return nil, fmt.Errorf("%w: %s", ErrStagingInventoryUnavailable, warnings[1])
	}
	names := []string{}
	for name := range versions[1] {
		names = append(names, name)
	}
	sort.Strings(names)
	result := &dto.SRUPocketView{ObservedAt: time.Now().UTC(), Series: series, UbuntuBase: base, ParentSeries: parent, CacheStatus: usedStatuses, Rows: []dto.SRUVersions{}}
	for _, name := range names {
		cells := []dto.SRUVersionCell{}
		for i, pocket := range pockets {
			cell := dto.SRUVersionCell{Label: pocket.Label, Version: versions[i][name], Warning: warnings[i]}
			if cell.Warning != "" {
				cell.State = "unknown"
			}
			cells = append(cells, cell)
		}
		result.Rows = append(result.Rows, dto.SRUVersions{ObservedAt: result.ObservedAt, Query: dto.SRUVersionsQuery{Package: name, Series: series}, UbuntuBase: base, ParentSeries: parent, Cells: VersionCells(cells)})
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
