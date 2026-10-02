// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/app"
	"github.com/canonical/sunbeam-watchtower/internal/config"
	"github.com/canonical/sunbeam-watchtower/internal/core/service/bugsearch"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

func TestBugServerSearchDoesNotCallLaunchpad(t *testing.T) {
	var upstreamCalls atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		http.Error(w, "unexpected upstream call", http.StatusInternalServerError)
	}))
	defer proxy.Close()
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	application := app.NewApp(&config.Config{
		Projects: []config.ProjectConfig{{
			Name: "openstack",
			Bugs: []config.BugTrackerConfig{{
				Forge: "launchpad", Project: "cinder",
			}},
		}},
	}, nil)
	cache, err := application.BugCache()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	if err := cache.ReplaceProject(
		context.Background(),
		forge.ForgeLaunchpad,
		"cinder",
		[]*forge.Bug{
			{
				Forge: forge.ForgeLaunchpad, ID: "2161602",
				Title:           "MicroCeph monitor address reordering restarts cinder-volume",
				VisibilityKnown: true,
			},
			{
				Forge: forge.ForgeLaunchpad, ID: "999",
				Title: "Private cinder-volume report", Private: true,
				VisibilityKnown: true,
			},
		},
		[]forge.BugTask{
			{
				Forge: forge.ForgeLaunchpad, BugID: "2161602",
				TargetName: "cinder", VisibilityKnown: true,
			},
			{
				Forge: forge.ForgeLaunchpad, BugID: "999",
				TargetName: "cinder", Private: true, VisibilityKnown: true,
			},
		},
		now,
		dto.BugCacheSchemaVersion,
	); err != nil {
		t.Fatal(err)
	}

	workflow := NewBugServerWorkflow(application)
	for _, query := range []string{"cinder-volume", "launchpad:2161602"} {
		result, err := workflow.Search(context.Background(), dto.BugSearchRequest{Query: query})
		if err != nil {
			t.Fatalf("Search(%q): %v", query, err)
		}
		if len(result.Results) != 1 || result.Results[0].ID != "2161602" {
			t.Fatalf("Search(%q) = %+v", query, result)
		}
	}
	privateResult, err := workflow.Search(
		context.Background(),
		dto.BugSearchRequest{Query: "launchpad:999"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if privateResult.Outcome != dto.BugSearchOutcomeNoResults ||
		len(privateResult.Warnings) != 1 {
		t.Fatalf("private search result = %+v", privateResult)
	}
	if calls := upstreamCalls.Load(); calls != 0 {
		t.Fatalf("Launchpad calls = %d, want 0", calls)
	}
}

func TestBugServerSearchRejectsLegacyCache(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	application := app.NewApp(&config.Config{}, nil)
	cache, err := application.BugCache()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	if err := cache.StoreBugs(context.Background(), []*forge.Bug{{
		Forge: forge.ForgeLaunchpad, ID: "1", Title: "Legacy cached bug",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := cache.StoreBugTasks(
		context.Background(),
		forge.ForgeLaunchpad,
		"cinder",
		[]forge.BugTask{{Forge: forge.ForgeLaunchpad, BugID: "1", TargetName: "cinder"}},
	); err != nil {
		t.Fatal(err)
	}

	_, err = NewBugServerWorkflow(application).Search(
		context.Background(),
		dto.BugSearchRequest{Query: "legacy"},
	)
	if !errors.Is(err, bugsearch.ErrCacheIncompatible) {
		t.Fatalf("Search error = %v, want ErrCacheIncompatible", err)
	}
}
