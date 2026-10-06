// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"

	"github.com/canonical/sunbeam-watchtower/internal/config"
	"github.com/canonical/sunbeam-watchtower/internal/core/service/sru"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func (a *App) SRUPocketView(ctx context.Context, series string) (*dto.SRUPocketView, error) {
	if !sruSeriesName.MatchString(series) {
		return nil, fmt.Errorf("%w: expected a UCA series", ErrSRUMigrationQuery)
	}
	cfg := a.GetConfig()
	if cfg == nil {
		return nil, fmt.Errorf("no configuration loaded")
	}
	base, parent := "", ""
	for name, release := range cfg.Packages.Distros["ubuntu"].Releases {
		if backport, exists := release.Backports[series]; exists {
			if base != "" {
				return nil, fmt.Errorf("%w: ambiguous UCA series %s", ErrSRUMigrationQuery, series)
			}
			base = name
			parent = backport.ParentRelease
		}
	}
	if base == "" {
		return nil, fmt.Errorf("%w: UCA series %s is not configured", ErrSRUMigrationTarget, series)
	}
	sources := buildPackageSources(cfg.Packages, []string{"ubuntu"}, nil, []string{"release", "updates", "security"}, []string{series}, a.Logger)
	pockets := []sru.CachedVersionPocket{{Name: "ubuntu", Label: "Ubuntu " + parent}, {Name: "ubuntu/" + series, Label: "staging"}, {Name: "ubuntu/" + series, Label: "proposed"}, {Name: "ubuntu/" + series, Label: "updates"}}
	if _, configured := cfg.Packages.Distros["ubuntu"].Releases[parent]; configured && parent != "" {
		for _, suite := range []string{parent, parent + "-updates", parent + "-security"} {
			for _, component := range cfg.Packages.Distros["ubuntu"].Components {
				pockets[0].Entries = append(pockets[0].Entries, dto.SourceEntry{Suite: suite, Component: component})
			}
		}
	}
	for _, source := range sources {
		for _, entry := range source.Entries {
			if source.Name == "ubuntu/"+series {
				switch entry.Suite {
				case config.ExpandBackportSuiteType(base, series, "release"):
					pockets[1].Entries = append(pockets[1].Entries, entry)
				case config.ExpandBackportSuiteType(base, series, "proposed"):
					pockets[2].Entries = append(pockets[2].Entries, entry)
				case config.ExpandBackportSuiteType(base, series, "updates"):
					pockets[3].Entries = append(pockets[3].Entries, entry)
				}

			}
		}
	}
	if parent == "" {
		pockets[0].Label = "Ubuntu parent"
	}
	cache, err := a.DistroCache()
	if err != nil {
		return nil, err
	}
	return sru.CachedPocketView(ctx, cache, series, base, parent, pockets)
}
