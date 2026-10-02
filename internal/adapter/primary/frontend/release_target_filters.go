// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/canonical/sunbeam-watchtower/internal/config"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// ReleaseTargetProfile holds resolved release target visibility rules.
type ReleaseTargetProfile struct {
	Name    string
	Include []ReleaseTargetMatcher
	Exclude []ReleaseTargetMatcher
}

// ReleaseTargetMatcher matches one target by base and architecture attributes.
type ReleaseTargetMatcher struct {
	BaseNames      []string
	BaseChannels   []string
	MinBaseChannel string
	Architectures  []string
}

var snapBasePattern = regexp.MustCompile(`^core(\d{2})$`)

// ResolveReleaseTargetProfile resolves the active target profile for one project.
func ResolveReleaseTargetProfile(cfg *config.Config, project string, selectedProfile string, allTargets bool) (*ReleaseTargetProfile, error) {
	if allTargets {
		return nil, nil
	}
	if cfg == nil {
		cfg = &config.Config{}
	}

	projectCfg := projectConfigByName(cfg, project)
	profileName := strings.TrimSpace(selectedProfile)
	if profileName == "" && projectCfg != nil && projectCfg.Release != nil {
		profileName = strings.TrimSpace(projectCfg.Release.TargetProfile)
	}
	if profileName == "" {
		profileName = strings.TrimSpace(cfg.Releases.DefaultTargetProfile)
	}

	var resolved ReleaseTargetProfile
	if profileName != "" {
		baseProfile, ok := cfg.Releases.TargetProfiles[profileName]
		if !ok {
			return nil, fmt.Errorf("unknown release target profile %q", profileName)
		}
		resolved = releaseTargetProfileFromConfig(profileName, &baseProfile)
	}

	if projectCfg != nil && projectCfg.Release != nil && projectCfg.Release.TargetProfileOverrides != nil {
		if resolved.Name == "" {
			resolved.Name = profileName
		}
		overrides := releaseTargetProfileFromConfig(profileName, projectCfg.Release.TargetProfileOverrides)
		resolved.Include = append(resolved.Include, overrides.Include...)
		resolved.Exclude = append(resolved.Exclude, overrides.Exclude...)
	}

	if resolved.Name == "" && len(resolved.Include) == 0 && len(resolved.Exclude) == 0 {
		return nil, nil
	}
	return &resolved, nil
}

// FilterReleaseListEntries filters release list rows using the active per-project profile.
func FilterReleaseListEntries(cfg *config.Config, entries []dto.ReleaseListEntry, selectedProfile string, allTargets bool) ([]dto.ReleaseListEntry, error) {
	filtered := make([]dto.ReleaseListEntry, 0, len(entries))
	for _, entry := range entries {
		profile, err := ResolveReleaseTargetProfile(cfg, entry.Project, selectedProfile, allTargets)
		if err != nil {
			return nil, err
		}
		targets := filterReleaseTargets(entry.Targets, profile)
		if len(entry.Targets) > 0 && len(targets) == 0 {
			continue
		}
		entry.Targets = targets
		filtered = append(filtered, entry)
	}
	return filtered, nil
}

// FilterReleaseShowResult filters release show channels and targets using the active profile.
func FilterReleaseShowResult(cfg *config.Config, project string, result *dto.ReleaseShowResult, selectedProfile string, allTargets bool) (*dto.ReleaseShowResult, error) {
	if result == nil {
		return nil, nil
	}
	profile, err := ResolveReleaseTargetProfile(cfg, project, selectedProfile, allTargets)
	if err != nil {
		return nil, err
	}

	filtered := *result
	filtered.Tracks = append([]string(nil), result.Tracks...)
	filtered.Channels = make([]dto.ReleaseChannelSnapshot, 0, len(result.Channels))
	for _, channel := range result.Channels {
		channelCopy := channel
		channelCopy.Targets = filterReleaseTargets(channel.Targets, profile)
		channelCopy.Resources = append([]dto.ReleaseResourceSnapshot(nil), channel.Resources...)
		if len(channel.Targets) > 0 && len(channelCopy.Targets) == 0 {
			continue
		}
		filtered.Channels = append(filtered.Channels, channelCopy)
	}
	return &filtered, nil
}

// FormatReleaseTarget renders one target with architecture, base, revision, and version.
func FormatReleaseTarget(target dto.ReleaseTargetSnapshot) string {
	return formatReleaseTarget(target, true)
}

// FormatReleaseTargetCompact renders one target without the optional version suffix.
func FormatReleaseTargetCompact(target dto.ReleaseTargetSnapshot) string {
	return formatReleaseTarget(target, false)
}

// FormatReleaseTargets renders a full target list using the canonical target-aware format.
func FormatReleaseTargets(targets []dto.ReleaseTargetSnapshot) string {
	if len(targets) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(targets))
	for _, target := range targets {
		parts = append(parts, FormatReleaseTarget(target))
	}
	return strings.Join(parts, ", ")
}

func releaseTargetProfileFromConfig(name string, cfg *config.ReleaseTargetProfileConfig) ReleaseTargetProfile {
	profile := ReleaseTargetProfile{Name: name}
	if cfg == nil {
		return profile
	}
	profile.Include = make([]ReleaseTargetMatcher, 0, len(cfg.Include))
	for _, matcher := range cfg.Include {
		profile.Include = append(profile.Include, releaseTargetMatcherFromConfig(matcher))
	}
	profile.Exclude = make([]ReleaseTargetMatcher, 0, len(cfg.Exclude))
	for _, matcher := range cfg.Exclude {
		profile.Exclude = append(profile.Exclude, releaseTargetMatcherFromConfig(matcher))
	}
	return profile
}

func releaseTargetMatcherFromConfig(cfg config.ReleaseTargetMatcherConfig) ReleaseTargetMatcher {
	return ReleaseTargetMatcher{
		BaseNames:      append([]string(nil), cfg.BaseNames...),
		BaseChannels:   append([]string(nil), cfg.BaseChannels...),
		MinBaseChannel: cfg.MinBaseChannel,
		Architectures:  append([]string(nil), cfg.Architectures...),
	}
}

func filterReleaseTargets(targets []dto.ReleaseTargetSnapshot, profile *ReleaseTargetProfile) []dto.ReleaseTargetSnapshot {
	if profile == nil {
		return append([]dto.ReleaseTargetSnapshot(nil), targets...)
	}
	filtered := make([]dto.ReleaseTargetSnapshot, 0, len(targets))
	for _, target := range targets {
		if !releaseTargetVisible(target, profile) {
			continue
		}
		filtered = append(filtered, target)
	}
	return filtered
}

func releaseTargetVisible(target dto.ReleaseTargetSnapshot, profile *ReleaseTargetProfile) bool {
	included := true
	if len(profile.Include) > 0 {
		included = false
		for _, matcher := range profile.Include {
			if matcher.matches(target) {
				included = true
				break
			}
		}
	}
	if !included {
		return false
	}
	for _, matcher := range profile.Exclude {
		if matcher.matches(target) {
			return false
		}
	}
	return true
}

func (m ReleaseTargetMatcher) matches(target dto.ReleaseTargetSnapshot) bool {
	baseName, baseChannel := canonicalReleaseTargetBase(target)
	hasBaseMetadata := baseName != "" || baseChannel != ""
	if len(m.BaseNames) > 0 && hasBaseMetadata && !slices.Contains(m.BaseNames, baseName) {
		return false
	}
	if len(m.BaseChannels) > 0 && hasBaseMetadata && !slices.Contains(m.BaseChannels, baseChannel) {
		return false
	}
	if len(m.Architectures) > 0 {
		arch := target.Architecture
		if arch == "" {
			arch = target.Base.Architecture
		}
		if !slices.Contains(m.Architectures, arch) {
			return false
		}
	}
	if m.MinBaseChannel != "" && hasBaseMetadata {
		if compareBaseChannels(baseChannel, m.MinBaseChannel) < 0 {
			return false
		}
	}
	return true
}

func canonicalReleaseTargetBase(target dto.ReleaseTargetSnapshot) (string, string) {
	baseName := strings.TrimSpace(target.Base.Name)
	baseChannel := strings.TrimSpace(target.Base.Channel)
	if normalizedChannel, ok := normalizeSnapBaseVersion(baseName); ok {
		return "ubuntu", normalizedChannel
	}
	if normalizedChannel, ok := normalizeSnapBaseVersion(baseChannel); ok {
		return "ubuntu", normalizedChannel
	}
	return baseName, baseChannel
}

func compareBaseChannels(left string, right string) int {
	leftParts, err := parseBaseChannelVersion(left)
	if err != nil {
		return -1
	}
	rightParts, err := parseBaseChannelVersion(right)
	if err != nil {
		return -1
	}
	maxLen := max(len(leftParts), len(rightParts))
	for idx := 0; idx < maxLen; idx++ {
		leftValue := 0
		if idx < len(leftParts) {
			leftValue = leftParts[idx]
		}
		rightValue := 0
		if idx < len(rightParts) {
			rightValue = rightParts[idx]
		}
		switch {
		case leftValue < rightValue:
			return -1
		case leftValue > rightValue:
			return 1
		}
	}
	return 0
}

func parseBaseChannelVersion(raw string) ([]int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty base channel")
	}
	if normalized, ok := normalizeSnapBaseVersion(raw); ok {
		raw = normalized
	}
	parts := strings.Split(raw, ".")
	values := make([]int, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			return nil, fmt.Errorf("invalid base channel %q", raw)
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid base channel %q", raw)
		}
		values = append(values, value)
	}
	return values, nil
}

func normalizeSnapBaseVersion(raw string) (string, bool) {
	matches := snapBasePattern.FindStringSubmatch(strings.TrimSpace(raw))
	if len(matches) != 2 {
		return "", false
	}
	return matches[1] + ".04", true
}

func formatReleaseTarget(target dto.ReleaseTargetSnapshot, includeVersion bool) string {
	label := target.Architecture
	if label == "" {
		label = target.Base.Architecture
	}
	if label == "" {
		label = "default"
	}
	if target.Base.Name != "" || target.Base.Channel != "" {
		baseName := target.Base.Name
		baseChannel := target.Base.Channel
		switch {
		case baseName != "" && baseChannel != "":
			label += "@" + baseName + "/" + baseChannel
		case baseName != "":
			label += "@" + baseName
		default:
			label += "@base/" + baseChannel
		}
	}
	if target.Revision > 0 {
		label += fmt.Sprintf(":r%d", target.Revision)
	}
	if includeVersion && shouldRenderReleaseVersion(target) {
		label += "/" + target.Version
	}
	return label
}

func shouldRenderReleaseVersion(target dto.ReleaseTargetSnapshot) bool {
	if target.Version == "" {
		return false
	}
	if target.Revision > 0 && target.Version == strconv.Itoa(target.Revision) {
		return false
	}
	return true
}

func projectConfigByName(cfg *config.Config, project string) *config.ProjectConfig {
	if cfg == nil {
		return nil
	}
	for idx := range cfg.Projects {
		if cfg.Projects[idx].Name == project {
			return &cfg.Projects[idx]
		}
	}
	return nil
}
