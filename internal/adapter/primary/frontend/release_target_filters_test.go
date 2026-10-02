// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"testing"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/config"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func TestResolveReleaseTargetProfile(t *testing.T) {
	cfg := &config.Config{
		Releases: config.ReleasesConfig{
			DefaultTargetProfile: "global",
			TargetProfiles: map[string]config.ReleaseTargetProfileConfig{
				"global": {
					Include: []config.ReleaseTargetMatcherConfig{{BaseNames: []string{"ubuntu"}, MinBaseChannel: "24.04"}},
				},
				"explicit": {
					Include: []config.ReleaseTargetMatcherConfig{{Architectures: []string{"arm64"}}},
				},
			},
		},
		Projects: []config.ProjectConfig{{
			Name: "openstack",
			Release: &config.ProjectReleaseConfig{
				TargetProfile: "global",
				TargetProfileOverrides: &config.ReleaseTargetProfileConfig{
					Exclude: []config.ReleaseTargetMatcherConfig{{Architectures: []string{"s390x"}}},
				},
			},
		}},
	}

	profile, err := ResolveReleaseTargetProfile(cfg, "openstack", "", false)
	if err != nil {
		t.Fatalf("ResolveReleaseTargetProfile() error = %v", err)
	}
	if profile == nil || profile.Name != "global" {
		t.Fatalf("ResolveReleaseTargetProfile() = %+v, want global profile", profile)
	}
	if len(profile.Include) != 1 || len(profile.Exclude) != 1 {
		t.Fatalf("resolved profile = %+v, want layered include/exclude", profile)
	}

	profile, err = ResolveReleaseTargetProfile(cfg, "openstack", "explicit", false)
	if err != nil {
		t.Fatalf("ResolveReleaseTargetProfile(explicit) error = %v", err)
	}
	if profile == nil || profile.Name != "explicit" {
		t.Fatalf("ResolveReleaseTargetProfile(explicit) = %+v, want explicit profile", profile)
	}
	if len(profile.Include) != 1 || len(profile.Exclude) != 1 {
		t.Fatalf("explicit layered profile = %+v, want explicit include plus project exclude", profile)
	}
}

func TestFilterReleaseListEntries(t *testing.T) {
	cfg := &config.Config{
		Releases: config.ReleasesConfig{
			DefaultTargetProfile: "noble-and-newer",
			TargetProfiles: map[string]config.ReleaseTargetProfileConfig{
				"noble-and-newer": {
					Include: []config.ReleaseTargetMatcherConfig{{BaseNames: []string{"ubuntu"}, MinBaseChannel: "24.04"}},
				},
			},
		},
	}

	entries := []dto.ReleaseListEntry{{
		Project: "openstack",
		Name:    "openstack",
		Targets: []dto.ReleaseTargetSnapshot{
			{Architecture: "amd64", Base: dto.ReleaseBase{Name: "ubuntu", Channel: "22.04"}, Revision: 40, Version: "1.2.2"},
			{Architecture: "amd64", Base: dto.ReleaseBase{Name: "ubuntu", Channel: "24.04"}, Revision: 41, Version: "1.2.3"},
		},
	}}

	filtered, err := FilterReleaseListEntries(cfg, entries, "", false)
	if err != nil {
		t.Fatalf("FilterReleaseListEntries() error = %v", err)
	}
	if got := len(filtered); got != 1 {
		t.Fatalf("len(filtered) = %d, want 1", got)
	}
	if got := len(filtered[0].Targets); got != 1 {
		t.Fatalf("len(filtered[0].Targets) = %d, want 1", got)
	}
	if got := filtered[0].Targets[0].Base.Channel; got != "24.04" {
		t.Fatalf("filtered target base channel = %q, want 24.04", got)
	}

	allTargets, err := FilterReleaseListEntries(cfg, entries, "", true)
	if err != nil {
		t.Fatalf("FilterReleaseListEntries(allTargets) error = %v", err)
	}
	if got := len(allTargets[0].Targets); got != 2 {
		t.Fatalf("len(allTargets[0].Targets) = %d, want 2", got)
	}
}

func TestFilterReleaseListEntriesNormalizesSnapBasesForMinBaseChannel(t *testing.T) {
	cfg := &config.Config{
		Releases: config.ReleasesConfig{
			DefaultTargetProfile: "noble-and-newer",
			TargetProfiles: map[string]config.ReleaseTargetProfileConfig{
				"noble-and-newer": {
					Include: []config.ReleaseTargetMatcherConfig{{BaseNames: []string{"ubuntu"}, MinBaseChannel: "24.04"}},
				},
			},
		},
	}

	entries := []dto.ReleaseListEntry{{
		Project: "openstack-hypervisor",
		Name:    "openstack-hypervisor",
		Targets: []dto.ReleaseTargetSnapshot{
			{Architecture: "amd64", Base: dto.ReleaseBase{Name: "core22"}, Revision: 40, Version: "1.2.2"},
			{Architecture: "amd64", Base: dto.ReleaseBase{Name: "core24"}, Revision: 41, Version: "1.2.3"},
		},
	}}

	filtered, err := FilterReleaseListEntries(cfg, entries, "", false)
	if err != nil {
		t.Fatalf("FilterReleaseListEntries() error = %v", err)
	}
	if got := len(filtered); got != 1 {
		t.Fatalf("len(filtered) = %d, want 1", got)
	}
	if got := len(filtered[0].Targets); got != 1 {
		t.Fatalf("len(filtered[0].Targets) = %d, want 1", got)
	}
	if got := filtered[0].Targets[0].Base.Name; got != "core24" {
		t.Fatalf("filtered target base = %q, want core24", got)
	}
}

func TestFilterReleaseListEntriesKeepsBaseLessTargetsVisible(t *testing.T) {
	cfg := &config.Config{
		Releases: config.ReleasesConfig{
			DefaultTargetProfile: "noble-and-newer",
			TargetProfiles: map[string]config.ReleaseTargetProfileConfig{
				"noble-and-newer": {
					Include: []config.ReleaseTargetMatcherConfig{{BaseNames: []string{"ubuntu"}, MinBaseChannel: "24.04"}},
				},
			},
		},
	}

	entries := []dto.ReleaseListEntry{{
		Project: "openstack-hypervisor",
		Name:    "openstack-hypervisor",
		Targets: []dto.ReleaseTargetSnapshot{
			{Architecture: "amd64", Revision: 41, Version: "1.2.3"},
		},
	}}

	filtered, err := FilterReleaseListEntries(cfg, entries, "", false)
	if err != nil {
		t.Fatalf("FilterReleaseListEntries() error = %v", err)
	}
	if got := len(filtered); got != 1 {
		t.Fatalf("len(filtered) = %d, want 1", got)
	}
	if got := len(filtered[0].Targets); got != 1 {
		t.Fatalf("len(filtered[0].Targets) = %d, want 1", got)
	}
}

func TestFilterReleaseShowResultDropsHiddenChannels(t *testing.T) {
	cfg := &config.Config{
		Releases: config.ReleasesConfig{
			TargetProfiles: map[string]config.ReleaseTargetProfileConfig{
				"noble-and-newer": {
					Include: []config.ReleaseTargetMatcherConfig{{BaseNames: []string{"ubuntu"}, MinBaseChannel: "24.04"}},
				},
			},
		},
	}

	result := &dto.ReleaseShowResult{
		Project: "openstack",
		Name:    "openstack",
		Channels: []dto.ReleaseChannelSnapshot{
			{
				Channel: "2024.1/stable",
				Targets: []dto.ReleaseTargetSnapshot{{Architecture: "amd64", Base: dto.ReleaseBase{Name: "ubuntu", Channel: "22.04"}, Revision: 40}},
			},
			{
				Channel: "2025.1/stable",
				Targets: []dto.ReleaseTargetSnapshot{{Architecture: "amd64", Base: dto.ReleaseBase{Name: "ubuntu", Channel: "24.04"}, Revision: 41}},
			},
		},
	}

	filtered, err := FilterReleaseShowResult(cfg, result.Project, result, "noble-and-newer", false)
	if err != nil {
		t.Fatalf("FilterReleaseShowResult() error = %v", err)
	}
	if got := len(filtered.Channels); got != 1 {
		t.Fatalf("len(filtered.Channels) = %d, want 1", got)
	}
	if got := filtered.Channels[0].Channel; got != "2025.1/stable" {
		t.Fatalf("filtered channel = %q, want 2025.1/stable", got)
	}
}

func TestFormatReleaseTargetAndTargets(t *testing.T) {
	target := dto.ReleaseTargetSnapshot{
		Architecture: "amd64",
		Base:         dto.ReleaseBase{Name: "ubuntu", Channel: "24.04"},
		Revision:     41,
		Version:      "1.2.3",
		ReleasedAt:   time.Now(),
	}

	if got := FormatReleaseTarget(target); got != "amd64@ubuntu/24.04:r41/1.2.3" {
		t.Fatalf("FormatReleaseTarget() = %q", got)
	}
	if got := FormatReleaseTargetCompact(target); got != "amd64@ubuntu/24.04:r41" {
		t.Fatalf("FormatReleaseTargetCompact() = %q", got)
	}
	if got := FormatReleaseTargets([]dto.ReleaseTargetSnapshot{target}); got != "amd64@ubuntu/24.04:r41/1.2.3" {
		t.Fatalf("FormatReleaseTargets() = %q", got)
	}

	snapTarget := dto.ReleaseTargetSnapshot{
		Architecture: "amd64",
		Base:         dto.ReleaseBase{Name: "core24"},
		Revision:     7,
	}
	if got := FormatReleaseTargetCompact(snapTarget); got != "amd64@core24:r7" {
		t.Fatalf("FormatReleaseTargetCompact(snap) = %q", got)
	}

	duplicateVersionTarget := dto.ReleaseTargetSnapshot{
		Architecture: "amd64",
		Base:         dto.ReleaseBase{Name: "ubuntu", Channel: "24.04"},
		Revision:     347,
		Version:      "347",
	}
	if got := FormatReleaseTarget(duplicateVersionTarget); got != "amd64@ubuntu/24.04:r347" {
		t.Fatalf("FormatReleaseTarget(duplicate version) = %q", got)
	}

	snapDetailedTarget := dto.ReleaseTargetSnapshot{
		Architecture: "amd64",
		Base:         dto.ReleaseBase{Name: "core24"},
		Revision:     527,
		Version:      "2024.1",
	}
	if got := FormatReleaseTarget(snapDetailedTarget); got != "amd64@core24:r527/2024.1" {
		t.Fatalf("FormatReleaseTarget(snap) = %q", got)
	}
}
