// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/core/service/sru"

	"github.com/canonical/sunbeam-watchtower/internal/config"
	distro "github.com/canonical/sunbeam-watchtower/pkg/distro/v1"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

// sruMigrationTargets uses upstream order only for configured targets. Native
// Ubuntu targets are associated through configured UCA parent releases.
func sruMigrationTargets(releases map[string]config.ReleaseConfig, series []dto.UpstreamSeries, selected string) ([]dto.SRUMigrationTarget, error) {
	metadata := make(map[string]dto.UpstreamSeries)
	ranks := make(map[string]int)
	for i, item := range series {
		if _, exists := metadata[item.Name]; exists {
			return nil, fmt.Errorf("cached upstream series %s is duplicated", item.Name)
		}
		metadata[item.Name] = item
		ranks[item.Name] = i
	}
	targets := make(map[string]dto.SRUMigrationTarget)
	targetRanks := make(map[string]int)
	selectedFound := false
	for base, release := range releases {
		for name, backport := range release.Backports {
			item, ok := metadata[name]
			if !ok {
				return nil, fmt.Errorf("configured UCA series %s has no cached upstream order; sync the upstream cache", name)
			}
			key := "uca/" + name
			if _, exists := targets[key]; exists {
				return nil, fmt.Errorf("UCA series %s is configured for multiple Ubuntu bases", name)
			}
			targets[key] = dto.SRUMigrationTarget{Archive: "uca", Series: name, UbuntuBase: base, ReleaseID: item.ReleaseID, ParentSeries: backport.ParentRelease, ParentRequired: backport.SRUParentRequired}
			targetRanks[key] = ranks[name]
			selectedFound = selectedFound || name == selected
			if backport.ParentRelease != "" {
				if _, exists := releases[backport.ParentRelease]; !exists {
					return nil, fmt.Errorf("parent Ubuntu release %s is not configured", backport.ParentRelease)
				}
				key = "ubuntu/" + backport.ParentRelease
				if _, exists := targets[key]; exists && targetRanks[key] != ranks[name] {
					return nil, fmt.Errorf("parent Ubuntu release %s has ambiguous upstream release order", backport.ParentRelease)
				}
				targets[key] = dto.SRUMigrationTarget{Archive: "ubuntu", Series: backport.ParentRelease, UbuntuBase: backport.ParentRelease, ReleaseID: item.ReleaseID}
				targetRanks[key] = ranks[name]
			}
		}
	}
	if !selectedFound {
		return nil, fmt.Errorf("UCA series %s is not configured", selected)
	}
	result := make([]dto.SRUMigrationTarget, 0, len(targets))
	for _, target := range targets {
		result = append(result, target)
	}
	seenReleaseIDs := make(map[string]bool)
	for _, item := range metadata {
		if item.ReleaseID == "" {
			continue
		}
		if seenReleaseIDs[item.ReleaseID] {
			return nil, fmt.Errorf("cached upstream release ID %s has ambiguous series order", item.ReleaseID)
		}
		seenReleaseIDs[item.ReleaseID] = true
	}

	sort.Slice(result, func(i, j int) bool {
		a, b := targetRanks[result[i].Archive+"/"+result[i].Series], targetRanks[result[j].Archive+"/"+result[j].Series]
		if a != b {
			return a < b
		}
		// Native Ubuntu precedes its UCA backport when ordering is equal.
		if result[i].Archive != result[j].Archive {
			return result[i].Archive == "ubuntu"
		}
		return result[i].Series < result[j].Series
	})
	return result, nil
}

func fetchSRUMigrationTarget(ctx context.Context, client *lp.Client, changesClient *http.Client, source string, target dto.SRUMigrationTarget) dto.SRUMigrationTarget {
	pockets := []string{"proposed", "updates"}
	if target.Archive == "uca" {
		pockets = append([]string{"staging"}, pockets...)
	}
	return fetchSRUPublicationTarget(ctx, client, changesClient, source, target, pockets)
}

func fetchSRUPublicationTarget(ctx context.Context, client *lp.Client, changesClient *http.Client, source string, target dto.SRUMigrationTarget, pockets []string) dto.SRUMigrationTarget {
	target.Pockets = make([]dto.SRUPocketObservation, 0, len(pockets))
	for _, pocket := range pockets {
		observation := dto.SRUPocketObservation{Pocket: pocket, Publications: []dto.SRUPublication{}}
		archive := lp.APIBaseURL + "/ubuntu/+archive/primary"
		options := lp.PublishedSourceOpts{SourceName: source, ExactMatch: true, Status: "Published", DistroSeries: lp.APIBaseURL + "/ubuntu/" + target.UbuntuBase}
		if target.Archive == "uca" {
			archive = lp.APIBaseURL + "/~ubuntu-cloud-archive/+archive/ubuntu/" + target.Series + "-" + pocket
		} else {
			options.Pocket = strings.ToUpper(pocket[:1]) + pocket[1:]
		}
		publications, err := client.GetPublishedSources(ctx, archive, options)
		if err != nil {
			observation.Warning = err.Error()
			target.Pockets = append(target.Pockets, observation)
			continue
		}
		observation.Known = true
		for _, publication := range publications {
			if publication.Status != "Published" || publication.SourcePackageName != source || publication.DistroSeriesLink != options.DistroSeries {
				continue
			}
			item := dto.SRUPublication{Version: publication.SourcePackageVersion, URL: publication.SelfLink, BugIDs: []string{}}
			if changesClient == nil {
				observation.Publications = append(observation.Publications, item)
				continue
			}
			changesURL, err := client.GetPublicationChangesURL(ctx, publication.SelfLink)
			if err != nil {
				item.Warning = err.Error()
			} else if changesURL == "" {
				item.Warning = "publication has no changes file; fix association is unknown"
			} else {
				ids, err := readFixedBugs(ctx, changesClient, changesURL)
				if err != nil {
					item.Warning = err.Error()
				} else {
					item.BugsKnown = len(ids) > 0
					if len(ids) == 0 {
						item.Warning = "no Launchpad bug references; fix association is unknown"
					}
					for id := range ids {
						item.BugIDs = append(item.BugIDs, id)
					}
					sort.Strings(item.BugIDs)
				}
			}
			observation.Publications = append(observation.Publications, item)
		}
		sort.Slice(observation.Publications, func(i, j int) bool {
			return distro.CompareVersions(observation.Publications[i].Version, observation.Publications[j].Version) > 0
		})
		target.Pockets = append(target.Pockets, observation)
	}
	return target
}

var ErrSRUMigrationQuery = errors.New("invalid SRU migration query")
var ErrSRUMigrationTarget = errors.New("SRU migration target not found")
var ErrSRUMigrationUnavailable = errors.New("SRU migration ordering unavailable")
var ErrSRUMigrationPrivate = errors.New("SRU migration bug is not public")

// SRUMigration performs a read-only live inspection. It never changes archive
// state or the existing SRU snapshot. The upstream ordering cache must exist.
func (a *App) SRUMigration(ctx context.Context, query dto.SRUMigrationQuery) (*dto.SRUMigrationChain, error) {
	if err := validateSRUMigrationQuery(query); err != nil {
		return nil, err
	}
	cfg := a.GetConfig()
	if cfg == nil {
		return nil, fmt.Errorf("no configuration loaded")
	}
	selectedFound := false
	for _, release := range cfg.Packages.Distros["ubuntu"].Releases {
		if _, ok := release.Backports[query.Series]; ok {
			selectedFound = true
		}
	}
	if !selectedFound {
		return nil, fmt.Errorf("%w: UCA series %s is not configured", ErrSRUMigrationTarget, query.Series)
	}
	provider, err := a.BuildUpstreamProvider()
	if err != nil {
		return nil, err
	}
	if provider == nil {
		return nil, fmt.Errorf("%w: configure and sync the OpenStack upstream cache to resolve release order", ErrSRUMigrationUnavailable)
	}
	metadata, err := provider.ListSeries(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: reading cached upstream order (run cache sync upstream): %w", ErrSRUMigrationUnavailable, err)
	}
	targets, err := sruMigrationTargets(cfg.Packages.Distros["ubuntu"].Releases, metadata, query.Series)
	if err != nil {
		return nil, fmt.Errorf("resolving configured migration targets: %w", err)
	}
	client := newLaunchpadClient(a.LaunchpadCredentialStore(), a.Logger, a.upstreamHTTPClient("launchpad", 30*time.Second))
	if client == nil {
		client = lp.NewClient(nil, a.Logger, a.upstreamHTTPClient("launchpad", 30*time.Second))
	}
	id, _ := strconv.Atoi(query.BugID)
	bug, err := client.GetBug(ctx, id)
	if err != nil {
		var httpErr *lp.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("%w: desired bug %s", ErrSRUMigrationTarget, query.BugID)
		}
		return nil, fmt.Errorf("checking desired bug: %w", err)
	}
	if bug.Private || bug.SecurityRelated || bug.InformationType != "Public" {
		return nil, fmt.Errorf("%w: only public, non-security bugs are monitored", ErrSRUMigrationPrivate)
	}
	changesClient := sruChangesClient(a)
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func(index int) {
			defer wg.Done()
			defer func() { <-sem }()
			targets[index] = fetchSRUMigrationTarget(ctx, client, changesClient, query.Package, targets[index])
		}(i)
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := sru.MigrationChain(query, targets, time.Now().UTC())
	return &result, nil
}

func validateSRUMigrationQuery(query dto.SRUMigrationQuery) error {
	id, err := strconv.Atoi(query.BugID)
	if err != nil || id < 1 || strconv.Itoa(id) != query.BugID {
		return fmt.Errorf("%w: expected a positive numeric bug ID", ErrSRUMigrationQuery)
	}
	if !sruSourceName.MatchString(query.Package) || !sruSeriesName.MatchString(query.Series) {
		return fmt.Errorf("%w: expected a source package and UCA series", ErrSRUMigrationQuery)
	}
	return nil
}

var sruSourceName = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]+$`)
var sruSeriesName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
