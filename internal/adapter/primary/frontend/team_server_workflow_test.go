// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/canonical/sunbeam-watchtower/internal/config"
	"github.com/canonical/sunbeam-watchtower/internal/core/service/artifactdiscovery"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

type fakeRepoCache struct {
	paths map[string]string // cloneURL -> repoPath
	err   error
}

func (f *fakeRepoCache) EnsureRepo(_ context.Context, cloneURL string, _ *dto.SyncOptions) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	if p, ok := f.paths[cloneURL]; ok {
		return p, nil
	}
	return "/repos/" + cloneURL, nil
}

type fakeDiscoverer struct {
	byPath map[string][]artifactdiscovery.DiscoveredArtifact
	errs   map[string]error
}

func (f *fakeDiscoverer) Discover(_ context.Context, repoPath string, _ dto.ArtifactType) ([]artifactdiscovery.DiscoveredArtifact, error) {
	if err, ok := f.errs[repoPath]; ok {
		return nil, err
	}
	return f.byPath[repoPath], nil
}

type fakeTeamSyncer struct {
	gotTeam    string
	gotTargets []dto.SyncTarget
	gotDryRun  bool
	ret        *dto.TeamSyncResult
	err        error
}

func (f *fakeTeamSyncer) Sync(_ context.Context, teamName string, targets []dto.SyncTarget, dryRun bool) (*dto.TeamSyncResult, error) {
	f.gotTeam = teamName
	f.gotTargets = append([]dto.SyncTarget(nil), targets...)
	f.gotDryRun = dryRun
	if f.err != nil {
		return nil, f.err
	}
	if f.ret != nil {
		return f.ret, nil
	}
	artifacts := make([]dto.ArtifactSyncResult, len(targets))
	for i, t := range targets {
		artifacts[i] = dto.ArtifactSyncResult{
			Project:      t.Project,
			ArtifactType: t.ArtifactType,
			StoreName:    t.StoreName,
			AlreadySync:  true,
		}
	}
	return &dto.TeamSyncResult{Artifacts: artifacts}, nil
}

func snapProject(name string) config.ProjectConfig {
	return config.ProjectConfig{
		Name:         name,
		ArtifactType: "snap",
		Code:         config.CodeConfig{Forge: "github", Owner: "canonical", Project: name},
	}
}

func charmProject(name string) config.ProjectConfig {
	return config.ProjectConfig{
		Name:         name,
		ArtifactType: "charm",
		Code:         config.CodeConfig{Forge: "github", Owner: "canonical", Project: name},
	}
}

func sortedTargetStoreNames(ts []dto.SyncTarget) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.StoreName
	}
	sort.Strings(out)
	return out
}

func TestTeamServerWorkflowSyncSingleSnap(t *testing.T) {
	cloneURL, _ := snapProject("openstack").Code.CloneURL()
	repoPath := "/repos/openstack"
	cfg := &config.Config{
		Collaborators: &config.CollaboratorsConfig{LaunchpadTeam: "team-sunbeam"},
		Projects:      []config.ProjectConfig{snapProject("openstack")},
	}
	cache := &fakeRepoCache{paths: map[string]string{cloneURL: repoPath}}
	disc := &fakeDiscoverer{byPath: map[string][]artifactdiscovery.DiscoveredArtifact{
		repoPath: {{Name: "openstack", ArtifactType: dto.ArtifactSnap}},
	}}
	syncer := &fakeTeamSyncer{}

	w := &TeamServerWorkflow{cfg: cfg, cache: cache, discoverer: disc, syncer: syncer}
	result, err := w.Sync(context.Background(), dto.TeamSyncRequest{DryRun: true})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if syncer.gotTeam != "team-sunbeam" {
		t.Errorf("team = %q, want team-sunbeam", syncer.gotTeam)
	}
	if !syncer.gotDryRun {
		t.Errorf("dryRun not propagated")
	}
	if len(syncer.gotTargets) != 1 {
		t.Fatalf("targets = %d, want 1", len(syncer.gotTargets))
	}
	got := syncer.gotTargets[0]
	want := dto.SyncTarget{Project: "openstack", ArtifactType: dto.ArtifactSnap, StoreName: "openstack"}
	if got != want {
		t.Errorf("target = %+v, want %+v", got, want)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", result.Warnings)
	}
}

func TestTeamServerWorkflowSyncMonorepoCharms(t *testing.T) {
	cloneURL, _ := charmProject("sunbeam-charms").Code.CloneURL()
	repoPath := "/repos/sunbeam-charms"
	cfg := &config.Config{
		Collaborators: &config.CollaboratorsConfig{LaunchpadTeam: "team-sunbeam"},
		Projects:      []config.ProjectConfig{charmProject("sunbeam-charms")},
	}
	charms := []artifactdiscovery.DiscoveredArtifact{
		{Name: "cinder-volume-hitachi", RelPath: "charms/cinder-volume-hitachi", ArtifactType: dto.ArtifactCharm},
		{Name: "keystone-k8s", RelPath: "charms/keystone-k8s", ArtifactType: dto.ArtifactCharm},
		{Name: "nova-k8s", RelPath: "charms/nova-k8s", ArtifactType: dto.ArtifactCharm},
	}
	cache := &fakeRepoCache{paths: map[string]string{cloneURL: repoPath}}
	disc := &fakeDiscoverer{byPath: map[string][]artifactdiscovery.DiscoveredArtifact{repoPath: charms}}
	syncer := &fakeTeamSyncer{}

	w := &TeamServerWorkflow{cfg: cfg, cache: cache, discoverer: disc, syncer: syncer}
	if _, err := w.Sync(context.Background(), dto.TeamSyncRequest{}); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	got := sortedTargetStoreNames(syncer.gotTargets)
	want := []string{"cinder-volume-hitachi", "keystone-k8s", "nova-k8s"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("store names = %v, want %v", got, want)
	}
	for _, target := range syncer.gotTargets {
		if target.Project != "sunbeam-charms" {
			t.Errorf("target %q project = %q, want sunbeam-charms", target.StoreName, target.Project)
		}
		if target.ArtifactType != dto.ArtifactCharm {
			t.Errorf("target %q type = %v, want charm", target.StoreName, target.ArtifactType)
		}
	}
}

func TestTeamServerWorkflowSyncProjectsFilter(t *testing.T) {
	cfg := &config.Config{
		Collaborators: &config.CollaboratorsConfig{LaunchpadTeam: "team-sunbeam"},
		Projects: []config.ProjectConfig{
			snapProject("openstack"),
			charmProject("sunbeam-charms"),
		},
	}
	openstackURL, _ := cfg.Projects[0].Code.CloneURL()
	sunbeamURL, _ := cfg.Projects[1].Code.CloneURL()

	cache := &fakeRepoCache{paths: map[string]string{
		openstackURL: "/repos/openstack",
		sunbeamURL:   "/repos/sunbeam-charms",
	}}
	disc := &fakeDiscoverer{byPath: map[string][]artifactdiscovery.DiscoveredArtifact{
		"/repos/openstack":      {{Name: "openstack", ArtifactType: dto.ArtifactSnap}},
		"/repos/sunbeam-charms": {{Name: "keystone-k8s", ArtifactType: dto.ArtifactCharm}},
	}}
	syncer := &fakeTeamSyncer{}

	w := &TeamServerWorkflow{cfg: cfg, cache: cache, discoverer: disc, syncer: syncer}
	_, err := w.Sync(context.Background(), dto.TeamSyncRequest{Projects: []string{"sunbeam-charms"}})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if len(syncer.gotTargets) != 1 || syncer.gotTargets[0].Project != "sunbeam-charms" {
		t.Errorf("targets = %+v, want only sunbeam-charms", syncer.gotTargets)
	}
}

func TestTeamServerWorkflowSyncDiscoveryError(t *testing.T) {
	cfg := &config.Config{
		Collaborators: &config.CollaboratorsConfig{LaunchpadTeam: "team-sunbeam"},
		Projects: []config.ProjectConfig{
			charmProject("broken-charms"),
			snapProject("openstack"),
		},
	}
	brokenURL, _ := cfg.Projects[0].Code.CloneURL()
	openstackURL, _ := cfg.Projects[1].Code.CloneURL()
	cache := &fakeRepoCache{paths: map[string]string{
		brokenURL:    "/repos/broken",
		openstackURL: "/repos/openstack",
	}}
	disc := &fakeDiscoverer{
		errs: map[string]error{"/repos/broken": errors.New("walk failed")},
		byPath: map[string][]artifactdiscovery.DiscoveredArtifact{
			"/repos/openstack": {{Name: "openstack", ArtifactType: dto.ArtifactSnap}},
		},
	}
	syncer := &fakeTeamSyncer{}

	w := &TeamServerWorkflow{cfg: cfg, cache: cache, discoverer: disc, syncer: syncer}
	result, err := w.Sync(context.Background(), dto.TeamSyncRequest{})
	if err != nil {
		t.Fatalf("Sync() err = %v, want nil (discovery error must not fail the whole sync)", err)
	}
	if len(syncer.gotTargets) != 1 || syncer.gotTargets[0].Project != "openstack" {
		t.Errorf("targets = %+v, want only openstack", syncer.gotTargets)
	}
	if len(result.Warnings) == 0 {
		t.Fatalf("expected a warning for broken-charms, got none")
	}
	foundBroken := false
	for _, w := range result.Warnings {
		if contains(w, "broken-charms") && contains(w, "walk failed") {
			foundBroken = true
			break
		}
	}
	if !foundBroken {
		t.Errorf("warnings = %v, want one mentioning broken-charms and walk failed", result.Warnings)
	}
}

func TestTeamServerWorkflowSyncNoArtifactsWarning(t *testing.T) {
	cfg := &config.Config{
		Collaborators: &config.CollaboratorsConfig{LaunchpadTeam: "team-sunbeam"},
		Projects:      []config.ProjectConfig{charmProject("empty")},
	}
	emptyURL, _ := cfg.Projects[0].Code.CloneURL()
	cache := &fakeRepoCache{paths: map[string]string{emptyURL: "/repos/empty"}}
	disc := &fakeDiscoverer{byPath: map[string][]artifactdiscovery.DiscoveredArtifact{"/repos/empty": nil}}
	syncer := &fakeTeamSyncer{}

	w := &TeamServerWorkflow{cfg: cfg, cache: cache, discoverer: disc, syncer: syncer}
	result, err := w.Sync(context.Background(), dto.TeamSyncRequest{})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if len(syncer.gotTargets) != 0 {
		t.Errorf("targets = %+v, want none", syncer.gotTargets)
	}
	if len(result.Warnings) == 0 || !contains(result.Warnings[0], "empty") {
		t.Errorf("warnings = %v, want one mentioning empty", result.Warnings)
	}
}

func TestTeamServerWorkflowSyncSkipArtifactsFilter(t *testing.T) {
	proj := charmProject("sunbeam-charms")
	proj.Team = &config.ProjectTeamConfig{SkipArtifacts: []string{"sunbeam-libs"}}
	cfg := &config.Config{
		Collaborators: &config.CollaboratorsConfig{LaunchpadTeam: "team-sunbeam"},
		Projects:      []config.ProjectConfig{proj},
	}
	cloneURL, _ := proj.Code.CloneURL()
	cache := &fakeRepoCache{paths: map[string]string{cloneURL: "/repos/sunbeam-charms"}}
	disc := &fakeDiscoverer{byPath: map[string][]artifactdiscovery.DiscoveredArtifact{
		"/repos/sunbeam-charms": {
			{Name: "keystone-k8s", ArtifactType: dto.ArtifactCharm},
			{Name: "sunbeam-libs", ArtifactType: dto.ArtifactCharm},
		},
	}}
	syncer := &fakeTeamSyncer{}

	w := &TeamServerWorkflow{cfg: cfg, cache: cache, discoverer: disc, syncer: syncer}
	result, err := w.Sync(context.Background(), dto.TeamSyncRequest{})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	got := sortedTargetStoreNames(syncer.gotTargets)
	if !reflect.DeepEqual(got, []string{"keystone-k8s"}) {
		t.Errorf("store names = %v, want [keystone-k8s]", got)
	}
	skipWarned := false
	for _, w := range result.Warnings {
		if contains(w, "sunbeam-libs") && contains(w, "team.skip_artifacts") {
			skipWarned = true
			break
		}
	}
	if !skipWarned {
		t.Errorf("warnings = %v, want one mentioning sunbeam-libs skip", result.Warnings)
	}
}

func TestTeamServerWorkflowSyncSkipsNonArtifactProjects(t *testing.T) {
	cfg := &config.Config{
		Collaborators: &config.CollaboratorsConfig{LaunchpadTeam: "team-sunbeam"},
		Projects: []config.ProjectConfig{
			{Name: "deb-only", ArtifactType: "deb", Code: config.CodeConfig{Forge: "github", Owner: "canonical", Project: "deb"}},
			snapProject("openstack"),
		},
	}
	openstackURL, _ := cfg.Projects[1].Code.CloneURL()
	cache := &fakeRepoCache{paths: map[string]string{openstackURL: "/repos/openstack"}}
	disc := &fakeDiscoverer{byPath: map[string][]artifactdiscovery.DiscoveredArtifact{
		"/repos/openstack": {{Name: "openstack", ArtifactType: dto.ArtifactSnap}},
	}}
	syncer := &fakeTeamSyncer{}

	w := &TeamServerWorkflow{cfg: cfg, cache: cache, discoverer: disc, syncer: syncer}
	if _, err := w.Sync(context.Background(), dto.TeamSyncRequest{}); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if len(syncer.gotTargets) != 1 || syncer.gotTargets[0].Project != "openstack" {
		t.Errorf("targets = %+v, want only openstack", syncer.gotTargets)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
