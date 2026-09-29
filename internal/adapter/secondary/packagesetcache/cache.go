// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package packagesetcache

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
	"strings"
	"time"
)

const (
	launchpadUbuntuURL = "https://api.launchpad.net/devel/ubuntu"
	reportBaseURL      = "https://static-reports.ubuntu.com/packagesets"
	maxResponseBytes   = 4 << 20
)

var (
	ErrNotSynced = errors.New("packageset has not been synced")
	validName    = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	validPackage = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]*$`)
)

// Snapshot is one validated, locally cached Launchpad packageset report.
type Snapshot struct {
	Name             string    `json:"name"`
	ConfiguredSeries string    `json:"configured_series"`
	Series           string    `json:"series"`
	Packages         []string  `json:"packages"`
	SyncedAt         time.Time `json:"synced_at"`
}

// Cache downloads packageset reports during explicit sync and reads snapshots
// without network access during package commands.
type Cache struct {
	dir        string
	client     *http.Client
	ubuntuURL  string
	reportsURL string
}

func NewCache(dir string, client *http.Client) *Cache {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Cache{dir: dir, client: client, ubuntuURL: launchpadUbuntuURL, reportsURL: reportBaseURL}
}

func (c *Cache) path(name string) (string, error) {
	if !validName.MatchString(name) {
		return "", fmt.Errorf("invalid packageset name %q", name)
	}
	return filepath.Join(c.dir, name+".json"), nil
}

// Read returns the last successfully synced snapshot for the configured series.
func (c *Cache) Read(name, configuredSeries string) (Snapshot, error) {
	path, err := c.path(name)
	if err != nil {
		return Snapshot{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Snapshot{}, ErrNotSynced
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("reading packageset cache: %w", err)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("decoding packageset cache: %w", err)
	}
	if snapshot.Name != name || snapshot.ConfiguredSeries != configuredSeries || len(snapshot.Packages) == 0 {
		return Snapshot{}, ErrNotSynced
	}
	return snapshot, nil
}

// Sync replaces a snapshot only after the complete report has been validated.
func (c *Cache) Sync(ctx context.Context, name, configuredSeries string) (Snapshot, error) {
	path, err := c.path(name)
	if err != nil {
		return Snapshot{}, err
	}
	series := configuredSeries
	if series == "development" {
		series, err = c.developmentSeries(ctx)
		if err != nil {
			return Snapshot{}, err
		}
	}
	if !validName.MatchString(series) {
		return Snapshot{}, fmt.Errorf("invalid Ubuntu series %q", series)
	}
	reportURL := c.reportsURL + "/" + url.PathEscape(series) + "/" + url.PathEscape(name)
	data, err := c.fetch(ctx, reportURL)
	if err != nil {
		return Snapshot{}, fmt.Errorf("fetching packageset %s for %s: %w", name, series, err)
	}
	packages, err := parseReport(data, name)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot := Snapshot{
		Name: name, ConfiguredSeries: configuredSeries, Series: series,
		Packages: packages, SyncedAt: time.Now().UTC(),
	}
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return Snapshot{}, fmt.Errorf("creating packageset cache: %w", err)
	}
	tmp, err := os.CreateTemp(c.dir, ".packageset-*")
	if err != nil {
		return Snapshot{}, fmt.Errorf("creating packageset snapshot: %w", err)
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return Snapshot{}, err
	}
	if err := json.NewEncoder(tmp).Encode(snapshot); err != nil {
		tmp.Close()
		return Snapshot{}, err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return Snapshot{}, err
	}
	if err := tmp.Close(); err != nil {
		return Snapshot{}, err
	}
	// #nosec G703 -- path is derived from a validated packageset name.
	if err := os.Rename(tmp.Name(), path); err != nil {
		return Snapshot{}, fmt.Errorf("replacing packageset snapshot: %w", err)
	}
	return snapshot, nil
}

func (c *Cache) developmentSeries(ctx context.Context) (string, error) {
	data, err := c.fetch(ctx, c.ubuntuURL)
	if err != nil {
		return "", fmt.Errorf("fetching Ubuntu development series: %w", err)
	}
	var response struct {
		CurrentSeriesLink string `json:"current_series_link"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return "", fmt.Errorf("decoding Ubuntu development series: %w", err)
	}
	link, err := url.Parse(response.CurrentSeriesLink)
	if err != nil || link.Scheme != "https" || link.Host != "api.launchpad.net" || !strings.HasPrefix(link.Path, "/devel/ubuntu/") {
		return "", fmt.Errorf("invalid Ubuntu current_series_link %q", response.CurrentSeriesLink)
	}
	series := strings.TrimPrefix(link.Path, "/devel/ubuntu/")
	if !validName.MatchString(series) {
		return "", fmt.Errorf("invalid Ubuntu development series %q", series)
	}
	return series, nil
}

func (c *Cache) fetch(ctx context.Context, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxResponseBytes {
		return nil, fmt.Errorf("response exceeds 4 MiB")
	}
	return data, nil
}

func parseReport(data []byte, expectedName string) ([]string, error) {
	var name string
	inPackages := false
	seenPackages := false
	var packages []string
	seen := map[string]bool{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Name: ") {
			name = strings.TrimSpace(strings.TrimPrefix(line, "Name: "))
		}
		if line == "Packages:" {
			inPackages = true
			seenPackages = true
			continue
		}
		if !inPackages {
			continue
		}
		if strings.HasPrefix(line, " - ") {
			pkg := strings.TrimSpace(strings.TrimPrefix(line, " - "))
			if !validPackage.MatchString(pkg) || seen[pkg] {
				return nil, fmt.Errorf("invalid or duplicate packageset member %q", pkg)
			}
			seen[pkg] = true
			packages = append(packages, pkg)
			continue
		}
		if line != "" {
			inPackages = false
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading packageset report: %w", err)
	}
	if name != expectedName || !seenPackages || len(packages) == 0 {
		return nil, fmt.Errorf("invalid packageset report for %q", expectedName)
	}
	return packages, nil
}
