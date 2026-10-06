// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/core/service/sru"
	distro "github.com/canonical/sunbeam-watchtower/pkg/distro/v1"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

// SRUVersions observes one configured UCA series and its native Ubuntu parent.
// It does not need monitoring sets, bugs, changes files or upstream ordering.
func (a *App) SRUVersions(ctx context.Context, query dto.SRUVersionsQuery) (*dto.SRUVersions, error) {
	if !sruSourceName.MatchString(query.Package) || !sruSeriesName.MatchString(query.Series) {
		return nil, fmt.Errorf("%w: expected a source package and UCA series", ErrSRUMigrationQuery)
	}
	cfg := a.GetConfig()
	if cfg == nil {
		return nil, fmt.Errorf("no configuration loaded")
	}
	target := dto.SRUMigrationTarget{Archive: "uca", Series: query.Series}
	for base, release := range cfg.Packages.Distros["ubuntu"].Releases {
		if backport, ok := release.Backports[query.Series]; ok {
			if target.UbuntuBase != "" {
				return nil, fmt.Errorf("%w: ambiguous UCA series %s", ErrSRUMigrationQuery, query.Series)
			}
			target.UbuntuBase = base
			target.ParentSeries = backport.ParentRelease
		}
	}
	if target.UbuntuBase == "" {
		return nil, fmt.Errorf("%w: UCA series %s is not configured", ErrSRUMigrationTarget, query.Series)
	}
	client := newLaunchpadClient(a.LaunchpadCredentialStore(), a.Logger, a.upstreamHTTPClient("launchpad", 30*time.Second))
	if client == nil {
		client = lp.NewClient(nil, a.Logger, a.upstreamHTTPClient("launchpad", 30*time.Second))
	}
	parent := dto.SRUVersionCell{Label: "Ubuntu parent", State: "unconfigured", Warning: "no parent_release configured"}
	if target.ParentSeries != "" {
		observed := fetchSRUPublicationTarget(ctx, client, nil, query.Package, dto.SRUMigrationTarget{Archive: "ubuntu", Series: target.ParentSeries, UbuntuBase: target.ParentSeries}, []string{"release", "updates", "security"})
		parent = sruVersionCell("Ubuntu "+target.ParentSeries, observed.Pockets)
	}
	observed := fetchSRUPublicationTarget(ctx, client, nil, query.Package, target, []string{"staging", "proposed", "updates"})
	cells := []dto.SRUVersionCell{parent}
	for _, pocket := range observed.Pockets {
		cells = append(cells, sruVersionCell(pocket.Pocket, []dto.SRUPocketObservation{pocket}))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &dto.SRUVersions{ObservedAt: time.Now().UTC(), Query: query, UbuntuBase: target.UbuntuBase, ParentSeries: target.ParentSeries, Cells: sru.VersionCells(cells)}, nil
}

func sruVersionCell(label string, pockets []dto.SRUPocketObservation) dto.SRUVersionCell {
	cell := dto.SRUVersionCell{Label: label}
	for _, pocket := range pockets {
		if !pocket.Known {
			cell.State = "unknown"
			cell.Warning = pocket.Pocket + ": " + pocket.Warning
		}
		for _, publication := range pocket.Publications {
			if cell.Version == "" || distro.CompareVersions(publication.Version, cell.Version) > 0 {
				cell.Version = publication.Version
			}
		}
	}
	return cell
}

// SRUAllVersions lists every configured series in stable name order.
func (a *App) SRUAllVersions(ctx context.Context, source string) (*dto.SRUVersionList, error) {
	if !sruSourceName.MatchString(source) {
		return nil, fmt.Errorf("%w: expected a source package", ErrSRUMigrationQuery)
	}
	cfg := a.GetConfig()
	if cfg == nil {
		return nil, fmt.Errorf("no configuration loaded")
	}
	names := []string{}
	seen := map[string]bool{}
	for _, release := range cfg.Packages.Distros["ubuntu"].Releases {
		for name := range release.Backports {
			if seen[name] {
				return nil, fmt.Errorf("%w: ambiguous UCA series %s", ErrSRUMigrationQuery, name)
			}
			seen[name] = true
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("%w: no UCA series configured", ErrSRUMigrationTarget)
	}
	sort.Strings(names)
	result := &dto.SRUVersionList{Package: source, Rows: []dto.SRUVersions{}}
	for _, name := range names {
		row, err := a.SRUVersions(ctx, dto.SRUVersionsQuery{Package: source, Series: name})
		if err != nil {
			return nil, err
		}
		result.Rows = append(result.Rows, *row)
	}
	result.ObservedAt = time.Now().UTC()
	return result, nil
}
