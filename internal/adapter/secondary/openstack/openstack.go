// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package openstack

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	"gopkg.in/yaml.v3"
)

var _ port.UpstreamProvider = (*Provider)(nil)

// Provider implements port.UpstreamProvider by reading data from locally
// cloned bare git repos (openstack/releases and openstack/requirements).
type Provider struct {
	releasesDir     string
	requirementsDir string
	mapping         map[string]string
}

// NewProvider creates an OpenStack upstream provider that reads from the given
// bare repo paths.
func NewProvider(releasesDir, requirementsDir string) *Provider {
	return &Provider{
		releasesDir:     releasesDir,
		requirementsDir: requirementsDir,
		mapping:         defaultMapping,
	}
}

// deliverableFile represents the YAML structure of a deliverables file in the
// openstack/releases repository.
type deliverableFile struct {
	Team     string               `yaml:"team"`
	Type     string               `yaml:"type"`
	Releases []deliverableRelease `yaml:"releases"`
}

type deliverableRelease struct {
	Version string `yaml:"version"`
}

type seriesStatus struct {
	Name      string `yaml:"name"`
	ReleaseID string `yaml:"release-id"`
	Status    string `yaml:"status"`
}

// DefaultRelease returns the newest non-development OpenStack release series.
func (p *Provider) DefaultRelease(ctx context.Context) (string, error) {
	series, err := p.seriesStatus(ctx)
	if err != nil {
		return "", err
	}
	for _, s := range series {
		if strings.EqualFold(s.Status, "development") {
			continue
		}
		if s.Name != "" {
			return s.Name, nil
		}
	}
	return "", fmt.Errorf("no default upstream release found")
}

// ResolveRelease maps either an OpenStack series name or release-id to the
// deliverables directory name used by openstack/releases.
func (p *Provider) ResolveRelease(ctx context.Context, release string) (string, error) {
	if release == "" {
		return p.DefaultRelease(ctx)
	}
	series, err := p.seriesStatus(ctx)
	if err != nil {
		return release, err
	}
	for _, s := range series {
		if s.Name == release || s.ReleaseID == release {
			return s.Name, nil
		}
	}
	return release, nil
}

func (p *Provider) seriesStatus(ctx context.Context) ([]seriesStatus, error) {
	content, err := gitShow(ctx, p.releasesDir, "HEAD:data/series_status.yaml")
	if err != nil {
		return nil, fmt.Errorf("reading series status: %w", err)
	}
	return parseSeriesStatus([]byte(content))
}

func parseSeriesStatus(data []byte) ([]seriesStatus, error) {
	var series []seriesStatus
	if err := yaml.Unmarshal(data, &series); err != nil {
		return nil, fmt.Errorf("parsing series status: %w", err)
	}
	return series, nil
}

// ListDeliverables returns known deliverables for the given release by reading
// YAML files from deliverables/<release>/ in the releases repo.
func (p *Provider) ListDeliverables(ctx context.Context, release string) ([]dto.Deliverable, error) {
	if release == "" {
		return p.listLatestDeliverables(ctx)
	}
	return p.listDeliverablesForRelease(ctx, release)
}

func (p *Provider) listLatestDeliverables(ctx context.Context) ([]dto.Deliverable, error) {
	series, err := p.seriesStatus(ctx)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	var result []dto.Deliverable
	// Independent deliverables no longer appear in named series. Prefer their
	// current release records over historical series entries.
	independent, _ := p.listDeliverablesForRelease(ctx, "_independent")
	for _, d := range independent {
		if d.Version == "" {
			continue
		}
		seen[p.MapPackageName(d.Name, d.Type)] = true
		result = append(result, d)
	}
	for _, s := range series {
		if strings.EqualFold(s.Status, "development") || s.Name == "" {
			continue
		}
		deliverables, err := p.listDeliverablesForRelease(ctx, s.Name)
		if err != nil {
			continue
		}
		for _, d := range deliverables {
			pkgName := p.MapPackageName(d.Name, d.Type)
			if seen[pkgName] || d.Version == "" {
				continue
			}
			seen[pkgName] = true
			result = append(result, d)
		}
	}
	return result, nil
}

func (p *Provider) listDeliverablesForRelease(ctx context.Context, release string) ([]dto.Deliverable, error) {
	treePath := fmt.Sprintf("HEAD:deliverables/%s/", release)
	listing, err := gitShow(ctx, p.releasesDir, treePath)
	if err != nil {
		return nil, fmt.Errorf("listing deliverables for %s: %w", release, err)
	}

	var result []dto.Deliverable
	for _, line := range strings.Split(strings.TrimSpace(listing), "\n") {
		name := strings.TrimSpace(line)
		if name == "" || !strings.HasSuffix(name, ".yaml") {
			continue
		}

		filePath := fmt.Sprintf("HEAD:deliverables/%s/%s", release, name)
		content, err := gitShow(ctx, p.releasesDir, filePath)
		if err != nil {
			continue
		}

		d, err := parseDeliverable(name, []byte(content))
		if err != nil {
			continue
		}
		result = append(result, d)
	}
	return result, nil
}

// parseDeliverable parses a single deliverable YAML file and returns a
// Deliverable. The fileName is used to derive the deliverable name (without
// the .yaml suffix).
func parseDeliverable(fileName string, data []byte) (dto.Deliverable, error) {
	var df deliverableFile
	if err := yaml.Unmarshal(data, &df); err != nil {
		return dto.Deliverable{}, fmt.Errorf("parsing %s: %w", fileName, err)
	}

	name := strings.TrimSuffix(fileName, ".yaml")
	dtype := mapDeliverableType(df.Type)
	version := latestVersion(df.Releases)

	return dto.Deliverable{
		Name:    name,
		Type:    dtype,
		Version: version,
		Team:    df.Team,
	}, nil
}

// latestVersion returns the latest non-lifecycle version from the releases
// list, iterating backwards. Lifecycle markers are not package versions.
func latestVersion(releases []deliverableRelease) string {
	for i := len(releases) - 1; i >= 0; i-- {
		v := releases[i].Version
		if isLifecycleVersion(v) {
			continue
		}
		return v
	}
	return ""
}

// isLifecycleVersion returns true for end-of-life, end-of-maintenance, and
// extended-maintenance markers.
func isLifecycleVersion(v string) bool {
	return strings.HasSuffix(v, "-eol") || strings.HasSuffix(v, "-eom") || strings.HasSuffix(v, "-em")
}

// mapDeliverableType converts the string type from the releases YAML to a
// dto.DeliverableType.
func mapDeliverableType(t string) dto.DeliverableType {
	switch strings.ToLower(t) {
	case "service":
		return dto.DeliverableService
	case "library":
		return dto.DeliverableLibrary
	case "client-library":
		return dto.DeliverableClient
	default:
		return dto.DeliverableOther
	}
}

// GetConstraints returns upper version constraints for the given release from
// the requirements repo. Named series are mapped to their release IDs, which
// are used by the requirements stable branches.
func (p *Provider) GetConstraints(ctx context.Context, release string) (map[string]string, error) {
	ref := "HEAD:upper-constraints.txt"
	if release != "" {
		series, err := p.seriesStatus(ctx)
		if err != nil {
			return nil, err
		}
		for _, s := range series {
			if s.Name == release || s.ReleaseID == release {
				release = s.ReleaseID
				break
			}
		}
		ref = fmt.Sprintf("stable/%s:upper-constraints.txt", release)
	}
	content, err := gitShow(ctx, p.requirementsDir, ref)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", ref, err)
	}
	return parseConstraints([]byte(content))
}

// parseConstraints parses an upper-constraints.txt file. Each line has the
// format name===version (possibly followed by ;env-markers).
func parseConstraints(data []byte) (map[string]string, error) {
	result := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Strip environment markers (everything after ';').
		if idx := strings.Index(line, ";"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		parts := strings.SplitN(line, "===", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		version := strings.TrimSpace(parts[1])
		if name != "" && version != "" {
			result[name] = version
		}
	}
	return result, scanner.Err()
}

// MapPackageName maps an upstream deliverable name to a distro source package
// name. It first checks the override mapping, then applies a type-based
// heuristic.
func (p *Provider) MapPackageName(deliverable string, dtype dto.DeliverableType) string {
	// Normalise: dots → hyphens.
	name := strings.ReplaceAll(deliverable, ".", "-")

	// Check explicit mapping first.
	if mapped, ok := p.mapping[deliverable]; ok {
		return mapped
	}

	// oslo.* libraries → python-oslo-*
	if strings.HasPrefix(name, "oslo-") {
		return "python-" + name
	}

	switch dtype {
	case dto.DeliverableLibrary:
		return "python-" + name
	case dto.DeliverableService, dto.DeliverableClient:
		return name
	default:
		return name
	}
}

// gitShow runs `git -C <dir> show <ref>` and returns the output as a string.
func gitShow(ctx context.Context, dir, ref string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "show", ref)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git show %s in %s: %w", ref, dir, err)
	}
	return string(out), nil
}
