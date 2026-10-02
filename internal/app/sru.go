// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/config"
	"github.com/canonical/sunbeam-watchtower/internal/core/service/sru"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

var ErrSRUNotSynced = errors.New("SRU monitor has no snapshot; run watchtower cache sync sru")

// SRUCacheStatus reports whether a complete local snapshot is available.
func (a *App) SRUCacheStatus() (dto.SRUCacheStatus, error) {
	path, err := sruSnapshotPath()
	if err != nil {
		return dto.SRUCacheStatus{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return dto.SRUCacheStatus{}, nil
	}
	if err != nil {
		return dto.SRUCacheStatus{}, err
	}
	var snapshot dto.SRUSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return dto.SRUCacheStatus{}, fmt.Errorf("decoding SRU snapshot: %w", err)
	}
	if snapshot.SyncedAt.IsZero() {
		return dto.SRUCacheStatus{}, fmt.Errorf("invalid SRU snapshot: missing sync time")
	}
	return dto.SRUCacheStatus{SyncedAt: snapshot.SyncedAt, Targets: len(snapshot.Rows)}, nil
}

// ClearSRUCache removes only the local SRU snapshot.
func (a *App) ClearSRUCache() error {
	path, err := sruSnapshotPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// SRUSnapshot reads the last complete monitoring snapshot without upstream IO.
func (a *App) SRUSnapshot(filter dto.SRUFilter) (*dto.SRUSnapshot, error) {
	path, err := sruSnapshotPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrSRUNotSynced
	}
	if err != nil {
		return nil, err
	}
	var snapshot dto.SRUSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("decoding SRU snapshot: %w", err)
	}
	if snapshot.SyncedAt.IsZero() {
		return nil, fmt.Errorf("invalid SRU snapshot: missing sync time")
	}
	filtered := sru.Filter(snapshot, filter)
	return &filtered, nil
}

func sruSnapshotPath() (string, error) {
	dir, err := cacheSubdir("sru")
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "snapshot.json"), nil
}

// RefreshSRU builds a complete new snapshot and publishes it atomically.
// An upstream failure leaves the previous snapshot available.
func (a *App) RefreshSRU(ctx context.Context) (*dto.SRUSnapshot, error) {
	cfg := a.GetConfig()
	if cfg == nil {
		return nil, fmt.Errorf("no configuration loaded")
	}
	packages, err := sruPackages(a)
	if err != nil {
		return nil, err
	}
	client := newLaunchpadClient(a.LaunchpadCredentialStore(), a.Logger, a.upstreamHTTPClient("launchpad", 2*time.Minute))
	if client == nil {
		client = lp.NewClient(nil, a.Logger, a.upstreamHTTPClient("launchpad", 2*time.Minute))
	}
	ids, err := discoverSRUBugIDs(ctx, client, packages, cfg.Packages.Distros["ubuntu"].Releases)
	if err != nil {
		return nil, err
	}
	bugs, err := loadSRUBugs(ctx, client, ids)
	if err != nil {
		return nil, err
	}
	rows := makeSRURows(bugs, packages, cfg.Packages.Distros["ubuntu"].Releases)
	changesClient := sruChangesClient(a)
	evidence, err := collectSRUEvidence(ctx, client, changesClient, rows)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		sru.ApplyEvidence(&rows[i], evidence[sruEvidenceKey(rows[i])][rows[i].BugID])
	}
	applySRUParentGates(rows)
	snapshot := &dto.SRUSnapshot{SyncedAt: time.Now().UTC(), Rows: rows}
	if err := writeSRUSnapshot(*snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func sruPackages(a *App) (map[string][]string, error) {
	cfg := a.GetConfig().Packages
	names := make([]string, 0, len(cfg.Sets)+len(cfg.LaunchpadSets))
	for name := range cfg.Sets {
		names = append(names, name)
	}
	for name := range cfg.LaunchpadSets {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("no package sets configured for SRU monitoring")
	}
	result := make(map[string][]string)
	for _, name := range names {
		packages, err := a.PackageSet(name)
		if err != nil {
			return nil, fmt.Errorf("resolving package set %s: %w", name, err)
		}
		for _, pkg := range packages {
			result[pkg] = append(result[pkg], name)
		}
	}
	return result, nil
}

func discoverSRUBugIDs(ctx context.Context, client *lp.Client, packages map[string][]string, releases map[string]config.ReleaseConfig) ([]int, error) {
	names := make([]string, 0, len(packages))
	for name := range packages {
		names = append(names, name)
	}
	sort.Strings(names)
	series := make([]string, 0, len(releases))
	ucaSeries := make(map[string]bool)
	for name, release := range releases {
		series = append(series, name)
		for backport := range release.Backports {
			ucaSeries[backport] = true
		}
	}
	sort.Strings(series)
	targets := make([]string, 0, len(series)*len(names)+len(ucaSeries))
	for _, name := range names {
		for _, release := range series {
			targets = append(targets, "ubuntu/"+release+"/+source/"+name)
		}
	}
	for release := range ucaSeries {
		targets = append(targets, "cloud-archive/"+release)
	}
	sort.Strings(targets)
	ids := make(map[int]bool)
	var mu sync.Mutex
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	var firstErr error
	for _, target := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			tasks, err := client.SearchBugTasks(ctx, target, lp.BugTaskSearchOpts{OmitDuplicates: true})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("discovering %s bugs: %w", target, err)
				}
				return
			}
			for _, task := range tasks {
				var id int
				if _, err := fmt.Sscanf(strings.TrimPrefix(task.BugLink, lp.APIBaseURL+"/bugs/"), "%d", &id); err == nil && id > 0 {
					ids[id] = true
				}
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	result := make([]int, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	sort.Ints(result)
	return result, nil
}

type sruBug struct {
	Bug   lp.Bug
	Tasks []lp.BugTask
}

func loadSRUBugs(ctx context.Context, client *lp.Client, ids []int) ([]sruBug, error) {
	result := make([]sruBug, len(ids))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for i, id := range ids {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			bug, err := client.GetBug(ctx, id)
			if err == nil {
				var tasks []lp.BugTask
				tasks, err = client.GetBugTasks(ctx, id)
				if err == nil && !bug.Private && !bug.SecurityRelated && bug.InformationType == "Public" {
					result[i] = sruBug{Bug: bug, Tasks: tasks}
				}
			}
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("loading bug %d: %w", id, err)
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return result, firstErr
}

func makeSRURows(bugs []sruBug, packages map[string][]string, releases map[string]config.ReleaseConfig) []dto.SRURow {
	var rows []dto.SRURow
	for _, entry := range bugs {
		if entry.Bug.ID == 0 {
			continue
		}
		bugID := fmt.Sprint(entry.Bug.ID)
		affectedPackages := make(map[string]bool)
		for _, task := range entry.Tasks {
			pkg, _, ok := ubuntuTaskTarget(task.TargetLink)
			if ok && len(packages[pkg]) > 0 {
				affectedPackages[pkg] = true
			}
		}
		for _, task := range entry.Tasks {
			if pkg, series, ok := ubuntuTaskTarget(task.TargetLink); ok && series != "" && len(packages[pkg]) > 0 {
				if _, configured := releases[series]; !configured {
					continue
				}
				row := newSRURow(entry.Bug, task, bugID, pkg, "ubuntu", series, "", false)
				row.PackageSets = append([]string(nil), packages[pkg]...)
				rows = append(rows, row)
				continue
			}
			series, ok := cloudArchiveTaskTarget(task.TargetLink)
			if !ok {
				continue
			}
			for base, release := range releases {
				backport, configured := release.Backports[series]
				if !configured {
					continue
				}
				for pkg := range affectedPackages {
					row := newSRURow(entry.Bug, task, bugID, pkg, "uca", series, base, backport.SRUParentRequired)
					if len(affectedPackages) > 1 {
						row.Warning = sru.SharedCloudArchiveTaskWarning
					}
					row.PackageSets = append([]string(nil), packages[pkg]...)
					row.ParentSeries = backport.ParentRelease
					rows = append(rows, row)
				}
			}
		}
	}
	return rows
}

func newSRURow(bug lp.Bug, task lp.BugTask, bugID, pkg, archive, series, base string, parentRequired bool) dto.SRURow {
	verification, tag := sru.Verification(bug.Tags, archive, series)
	row := dto.SRURow{BugID: bugID, Title: bug.Title, BugURL: bug.WebLink,
		Package: pkg, Archive: archive, Series: series, UbuntuBase: base,
		TaskStatus: task.Status, TaskURL: task.WebLink, Verification: verification,
		VerificationTag: tag, Stage: "not observed", ParentRequired: parentRequired}
	if bug.DateLastUpdated != nil {
		row.UpdatedAt = bug.DateLastUpdated.Time
	}
	return row
}

func ubuntuTaskTarget(raw string) (pkg, series string, ok bool) {
	path := strings.TrimPrefix(raw, lp.APIBaseURL+"/")
	parts := strings.Split(path, "/")
	if len(parts) == 3 && parts[0] == "ubuntu" && parts[1] == "+source" {
		return parts[2], "", true
	}
	if len(parts) == 4 && parts[0] == "ubuntu" && parts[2] == "+source" {
		return parts[3], parts[1], true
	}
	return "", "", false
}

func cloudArchiveTaskTarget(raw string) (string, bool) {
	path := strings.TrimPrefix(raw, lp.APIBaseURL+"/")
	parts := strings.Split(path, "/")
	returnValue := len(parts) == 2 && parts[0] == "cloud-archive" && parts[1] != ""
	if returnValue {
		return parts[1], true
	}
	return "", false
}

func sruEvidenceKey(row dto.SRURow) string {
	return row.Archive + ":" + row.UbuntuBase + ":" + row.Series + ":" + row.Package
}

var fixedBugField = regexp.MustCompile(`^[0-9]+$`)

func collectSRUEvidence(ctx context.Context, client *lp.Client, changesClient *http.Client, rows []dto.SRURow) (map[string]map[string][]sru.Evidence, error) {
	unique := make(map[string]dto.SRURow)
	for _, row := range rows {
		unique[sruEvidenceKey(row)] = row
	}
	result := make(map[string]map[string][]sru.Evidence, len(unique))
	var mu sync.Mutex
	var firstErr error
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for key, row := range unique {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			items, err := fetchSRUTargetEvidence(ctx, client, changesClient, row)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("checking %s: %w", key, err)
				}
				return
			}
			result[key] = items
		}()
	}
	wg.Wait()
	return result, firstErr
}

func fetchSRUTargetEvidence(ctx context.Context, client *lp.Client, changesClient *http.Client, row dto.SRURow) (map[string][]sru.Evidence, error) {
	result := make(map[string][]sru.Evidence)
	versionBugs := make(map[string]map[string]bool)
	series := row.Series
	if row.Archive == "uca" {
		series = row.UbuntuBase
	}
	seriesLink := lp.APIBaseURL + "/ubuntu/" + series
	if row.Archive == "ubuntu" {
		uploads, err := client.GetPackageUploads(ctx, seriesLink, row.Package, "Unapproved")
		if err != nil {
			return nil, err
		}
		for _, upload := range uploads {
			if upload.PackageName != row.Package || upload.ChangesFileURL == "" {
				continue
			}
			ids, err := readFixedBugs(ctx, changesClient, upload.ChangesFileURL)
			if err != nil {
				return nil, err
			}
			for id := range ids {
				result[id] = append(result[id], sru.Evidence{Stage: "unapproved", Version: upload.PackageVersion, URL: upload.SelfLink})
			}
			versionBugs[upload.PackageVersion] = ids
		}
	}
	stages := []string{"proposed", "updates"}
	if row.Archive == "uca" {
		stages = append([]string{"staging"}, stages...)
	}
	for _, stage := range stages {
		archive := lp.APIBaseURL + "/ubuntu/+archive/primary"
		options := lp.PublishedSourceOpts{SourceName: row.Package, ExactMatch: true,
			DistroSeries: seriesLink}
		if row.Archive == "ubuntu" {
			options.Pocket = strings.ToUpper(stage[:1]) + stage[1:]
		} else {
			archive = lp.APIBaseURL + "/~ubuntu-cloud-archive/+archive/ubuntu/" + row.Series + "-" + stage
		}
		publications, err := client.GetPublishedSources(ctx, archive, options)
		if err != nil {
			return nil, err
		}
		for _, publication := range publications {
			if publication.SourcePackageName != row.Package || publication.DistroSeriesLink != seriesLink ||
				(publication.Status != "Published" && publication.Status != "Superseded") {
				continue
			}
			changesURL, err := client.GetPublicationChangesURL(ctx, publication.SelfLink)
			if err != nil {
				return nil, err
			}
			ids := versionBugs[publication.SourcePackageVersion]
			if changesURL != "" {
				ids, err = readFixedBugs(ctx, changesClient, changesURL)
				if err != nil {
					return nil, err
				}
				versionBugs[publication.SourcePackageVersion] = ids
			}
			for id := range ids {
				result[id] = append(result[id], sru.Evidence{Stage: stage, Version: publication.SourcePackageVersion, URL: publication.SelfLink})
			}
		}
	}
	return result, nil
}

func sruChangesClient(a *App) *http.Client {
	base := a.upstreamHTTPClient("launchpad", 30*time.Second)
	client := *base
	client.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
		if !validSRUChangesURL(req.URL) {
			return fmt.Errorf("untrusted changes-file redirect")
		}
		return nil
	}
	return &client
}

func validSRUChangesURL(raw *url.URL) bool {
	if raw == nil || raw.Scheme != "https" || raw.User != nil || raw.Port() != "" {
		return false
	}
	host := strings.ToLower(raw.Hostname())
	return host == "launchpad.net" || host == "launchpadlibrarian.net" || strings.HasSuffix(host, ".launchpadlibrarian.net")
}

func readFixedBugs(ctx context.Context, client *http.Client, rawURL string) (map[string]bool, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || !validSRUChangesURL(parsed) {
		return nil, fmt.Errorf("untrusted changes-file URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("changes file returned HTTP %d", response.StatusCode)
	}
	const maxChangesBytes = 2 << 20
	contents, err := io.ReadAll(io.LimitReader(response.Body, maxChangesBytes+1))
	if err != nil {
		return nil, err
	}
	if len(contents) > maxChangesBytes {
		return nil, fmt.Errorf("changes file exceeds %d bytes", maxChangesBytes)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(contents)))
	scanner.Buffer(make([]byte, 4096), 2<<20)
	result := make(map[string]bool)
	inFixed := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Launchpad-Bugs-Fixed:") {
			inFixed = true
			line = strings.TrimPrefix(line, "Launchpad-Bugs-Fixed:")
		} else if len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
			inFixed = false
		}
		if !inFixed {
			continue
		}
		for _, field := range strings.Fields(line) {
			if fixedBugField.MatchString(field) {
				result[field] = true
			}
		}
	}
	return result, scanner.Err()
}

func applySRUParentGates(rows []dto.SRURow) {
	for i := range rows {
		if !rows[i].ParentRequired {
			continue
		}
		for _, candidate := range rows {
			if candidate.BugID == rows[i].BugID && candidate.Package == rows[i].Package &&
				candidate.Archive == "ubuntu" && candidate.Series == rows[i].ParentSeries && candidate.Stage == "updates" {
				rows[i].ParentReady = true
				break
			}
		}
		if !rows[i].ParentReady {
			if rows[i].Warning != "" {
				rows[i].Warning += "; "
			}
			rows[i].Warning += "parent Ubuntu SRU is not observed in updates"
		}
	}
}

func writeSRUSnapshot(snapshot dto.SRUSnapshot) error {
	path, err := sruSnapshotPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".snapshot-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err := temp.Chmod(0600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	// #nosec G703 -- path is the caller's configured XDG cache directory,
	// joined with fixed "sru/snapshot.json" components.
	return os.Rename(temp.Name(), path)
}
