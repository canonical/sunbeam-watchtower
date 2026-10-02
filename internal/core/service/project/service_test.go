// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"context"
	"fmt"
	"testing"

	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

// mockProjectManager implements port.ProjectManager for testing.
type mockProjectManager struct {
	projects      map[string]*forge.Project
	series        map[string][]forge.ProjectSeries // project → series
	createdSeries []seriesCreate
	devFocusSet   []devFocusUpdate
	createErr     error
	devFocusErr   error
}

type seriesCreate struct {
	Project string
	Name    string
	Summary string
}

type devFocusUpdate struct {
	Project        string
	SeriesSelfLink string
}

func (m *mockProjectManager) GetProject(_ context.Context, name string) (*forge.Project, error) {
	p, ok := m.projects[name]
	if !ok {
		return nil, fmt.Errorf("project %q not found", name)
	}
	return p, nil
}

func (m *mockProjectManager) GetProjectSeries(_ context.Context, name string) ([]forge.ProjectSeries, error) {
	return m.series[name], nil
}

func (m *mockProjectManager) CreateSeries(_ context.Context, projectName, seriesName, summary string) (*forge.ProjectSeries, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	m.createdSeries = append(m.createdSeries, seriesCreate{Project: projectName, Name: seriesName, Summary: summary})
	selfLink := fmt.Sprintf("https://api.launchpad.net/devel/%s/%s", projectName, seriesName)
	ps := &forge.ProjectSeries{Name: seriesName, SelfLink: selfLink, Active: true}
	m.series[projectName] = append(m.series[projectName], *ps)
	return ps, nil
}

func (m *mockProjectManager) SetDevelopmentFocus(_ context.Context, projectName, seriesSelfLink string) error {
	if m.devFocusErr != nil {
		return m.devFocusErr
	}
	m.devFocusSet = append(m.devFocusSet, devFocusUpdate{Project: projectName, SeriesSelfLink: seriesSelfLink})
	return nil
}

func TestSync_CreatesMissingSeries(t *testing.T) {
	mgr := &mockProjectManager{
		projects: map[string]*forge.Project{
			"sunbeam": {Name: "sunbeam", DevelopmentFocusLink: ""},
		},
		series: map[string][]forge.ProjectSeries{
			"sunbeam": {
				{Name: "2024.1", SelfLink: "https://api.launchpad.net/devel/sunbeam/2024.1", Active: true},
			},
		},
	}

	svc := NewService(mgr, map[string]ProjectSyncConfig{
		"sunbeam": {Series: []string{"2024.1", "2024.2", "2025.1"}},
	}, nil)
	result, err := svc.Sync(context.Background(), SyncOptions{})
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// 2024.1 exists; 2024.2 and 2025.1 should be created.
	if len(mgr.createdSeries) != 2 {
		t.Fatalf("expected 2 series created, got %d", len(mgr.createdSeries))
	}
	if mgr.createdSeries[0].Name != "2024.2" {
		t.Errorf("first created series = %q, want 2024.2", mgr.createdSeries[0].Name)
	}
	if mgr.createdSeries[1].Name != "2025.1" {
		t.Errorf("second created series = %q, want 2025.1", mgr.createdSeries[1].Name)
	}

	createActions := 0
	for _, a := range result.Actions {
		if a.ActionType == ActionCreateSeries {
			createActions++
		}
	}
	if createActions != 2 {
		t.Errorf("expected 2 create_series actions, got %d", createActions)
	}
}

func TestSync_SetsDevelopmentFocus(t *testing.T) {
	mgr := &mockProjectManager{
		projects: map[string]*forge.Project{
			"sunbeam": {
				Name:                 "sunbeam",
				DevelopmentFocusLink: "https://api.launchpad.net/devel/sunbeam/trunk",
			},
		},
		series: map[string][]forge.ProjectSeries{
			"sunbeam": {
				{Name: "2025.1", SelfLink: "https://api.launchpad.net/devel/sunbeam/2025.1", Active: true},
			},
		},
	}

	svc := NewService(mgr, map[string]ProjectSyncConfig{
		"sunbeam": {Series: []string{"2025.1"}, DevelopmentFocus: "2025.1"},
	}, nil)
	result, err := svc.Sync(context.Background(), SyncOptions{})
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	if len(mgr.devFocusSet) != 1 {
		t.Fatalf("expected 1 dev focus update, got %d", len(mgr.devFocusSet))
	}
	if mgr.devFocusSet[0].SeriesSelfLink != "https://api.launchpad.net/devel/sunbeam/2025.1" {
		t.Errorf("dev focus set to %q, want sunbeam/2025.1", mgr.devFocusSet[0].SeriesSelfLink)
	}

	var devFocusActions int
	for _, a := range result.Actions {
		if a.ActionType == ActionSetDevFocus {
			devFocusActions++
		}
	}
	if devFocusActions != 1 {
		t.Errorf("expected 1 set_dev_focus action, got %d", devFocusActions)
	}
}

func TestSync_DevFocusUnchanged(t *testing.T) {
	mgr := &mockProjectManager{
		projects: map[string]*forge.Project{
			"sunbeam": {
				Name:                 "sunbeam",
				DevelopmentFocusLink: "https://api.launchpad.net/devel/sunbeam/2025.1",
			},
		},
		series: map[string][]forge.ProjectSeries{
			"sunbeam": {
				{Name: "2025.1", SelfLink: "https://api.launchpad.net/devel/sunbeam/2025.1", Active: true},
			},
		},
	}

	svc := NewService(mgr, map[string]ProjectSyncConfig{
		"sunbeam": {Series: []string{"2025.1"}, DevelopmentFocus: "2025.1"},
	}, nil)
	result, err := svc.Sync(context.Background(), SyncOptions{})
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	if len(mgr.devFocusSet) != 0 {
		t.Errorf("expected no dev focus updates (already correct), got %d", len(mgr.devFocusSet))
	}

	var unchanged int
	for _, a := range result.Actions {
		if a.ActionType == ActionDevFocusUnchanged {
			unchanged++
		}
	}
	if unchanged != 1 {
		t.Errorf("expected 1 dev_focus_unchanged action, got %d", unchanged)
	}
}

func TestSync_DryRun(t *testing.T) {
	mgr := &mockProjectManager{
		projects: map[string]*forge.Project{
			"sunbeam": {Name: "sunbeam", DevelopmentFocusLink: ""},
		},
		series: map[string][]forge.ProjectSeries{
			"sunbeam": {},
		},
	}

	svc := NewService(mgr, map[string]ProjectSyncConfig{
		"sunbeam": {Series: []string{"2025.1"}, DevelopmentFocus: "2025.1"},
	}, nil)
	result, err := svc.Sync(context.Background(), SyncOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// No actual API calls in dry-run.
	if len(mgr.createdSeries) != 0 {
		t.Errorf("expected no series created in dry-run, got %d", len(mgr.createdSeries))
	}
	if len(mgr.devFocusSet) != 0 {
		t.Errorf("expected no dev focus updates in dry-run, got %d", len(mgr.devFocusSet))
	}

	// Actions should still be planned.
	if len(result.Actions) == 0 {
		t.Error("expected at least one planned action in dry-run")
	}
}

func TestSync_ProjectFilter(t *testing.T) {
	mgr := &mockProjectManager{
		projects: map[string]*forge.Project{
			"sunbeam":        {Name: "sunbeam", DevelopmentFocusLink: ""},
			"snap-openstack": {Name: "snap-openstack", DevelopmentFocusLink: ""},
		},
		series: map[string][]forge.ProjectSeries{
			"sunbeam":        {},
			"snap-openstack": {},
		},
	}

	svc := NewService(mgr, map[string]ProjectSyncConfig{
		"sunbeam":        {Series: []string{"2025.1"}},
		"snap-openstack": {Series: []string{"2025.1"}},
	}, nil)
	result, err := svc.Sync(context.Background(), SyncOptions{Projects: []string{"sunbeam"}})
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// Only sunbeam should be processed.
	for _, a := range result.Actions {
		if a.Project == "snap-openstack" {
			t.Error("snap-openstack should not have been processed")
		}
	}
	if len(mgr.createdSeries) != 1 {
		t.Fatalf("expected 1 series created (sunbeam only), got %d", len(mgr.createdSeries))
	}
	if mgr.createdSeries[0].Project != "sunbeam" {
		t.Errorf("created series on %q, want sunbeam", mgr.createdSeries[0].Project)
	}
}

func TestSync_PerProjectOverrides(t *testing.T) {
	mgr := &mockProjectManager{
		projects: map[string]*forge.Project{
			"sunbeam":     {Name: "sunbeam", DevelopmentFocusLink: ""},
			"snap-consul": {Name: "snap-consul", DevelopmentFocusLink: ""},
		},
		series: map[string][]forge.ProjectSeries{
			"sunbeam":     {},
			"snap-consul": {},
		},
	}

	svc := NewService(mgr, map[string]ProjectSyncConfig{
		"sunbeam":     {Series: []string{"2024.1", "2025.1"}, DevelopmentFocus: "2025.1"},
		"snap-consul": {Series: []string{"1.19"}, DevelopmentFocus: "1.19"},
	}, nil)
	result, err := svc.Sync(context.Background(), SyncOptions{})
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// sunbeam should get 2024.1 + 2025.1, snap-consul should get 1.19
	if len(mgr.createdSeries) != 3 {
		t.Fatalf("expected 3 series created, got %d", len(mgr.createdSeries))
	}

	seriesByProject := make(map[string][]string)
	for _, cs := range mgr.createdSeries {
		seriesByProject[cs.Project] = append(seriesByProject[cs.Project], cs.Name)
	}
	if len(seriesByProject["sunbeam"]) != 2 {
		t.Errorf("expected 2 series for sunbeam, got %d", len(seriesByProject["sunbeam"]))
	}
	if len(seriesByProject["snap-consul"]) != 1 {
		t.Errorf("expected 1 series for snap-consul, got %d", len(seriesByProject["snap-consul"]))
	}
	if seriesByProject["snap-consul"][0] != "1.19" {
		t.Errorf("snap-consul series = %q, want 1.19", seriesByProject["snap-consul"][0])
	}

	// Both should have dev focus set
	devFocusActions := 0
	for _, a := range result.Actions {
		if a.ActionType == ActionSetDevFocus {
			devFocusActions++
		}
	}
	if devFocusActions != 2 {
		t.Errorf("expected 2 set_dev_focus actions, got %d", devFocusActions)
	}
}

func TestSync_MultipleProjects(t *testing.T) {
	mgr := &mockProjectManager{
		projects: map[string]*forge.Project{
			"sunbeam":        {Name: "sunbeam", DevelopmentFocusLink: ""},
			"snap-openstack": {Name: "snap-openstack", DevelopmentFocusLink: ""},
		},
		series: map[string][]forge.ProjectSeries{
			"sunbeam":        {},
			"snap-openstack": {},
		},
	}

	svc := NewService(mgr, map[string]ProjectSyncConfig{
		"sunbeam":        {Series: []string{"2025.1"}},
		"snap-openstack": {Series: []string{"2025.1"}},
	}, nil)
	result, err := svc.Sync(context.Background(), SyncOptions{})
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	if len(mgr.createdSeries) != 2 {
		t.Fatalf("expected 2 series created, got %d", len(mgr.createdSeries))
	}

	createActions := 0
	for _, a := range result.Actions {
		if a.ActionType == ActionCreateSeries {
			createActions++
		}
	}
	if createActions != 2 {
		t.Errorf("expected 2 create_series actions, got %d", createActions)
	}
}
