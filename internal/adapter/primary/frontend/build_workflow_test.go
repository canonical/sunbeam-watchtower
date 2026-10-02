// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canonical/sunbeam-watchtower/internal/core/service/build"
	"github.com/canonical/sunbeam-watchtower/pkg/client"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func TestBuildWorkflowTriggerAsync(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/builds/trigger/async" {
			t.Fatalf("path = %q, want async trigger path", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(dto.OperationJob{
			ID:    "op-build-1",
			Kind:  dto.OperationKindBuildTrigger,
			State: dto.OperationStateQueued,
		})
	}))
	defer ts.Close()

	workflow := NewBuildWorkflow(NewClientTransport(client.NewClient(ts.URL)), nil)
	got, err := workflow.Trigger(context.Background(), BuildTriggerRequest{
		Async:   true,
		Project: "demo",
	})
	if err != nil {
		t.Fatalf("Trigger() error = %v", err)
	}
	if got.Job == nil || got.Job.ID != "op-build-1" {
		t.Fatalf("Job = %+v, want op-build-1", got.Job)
	}
}

func TestBuildWorkflowTriggerLocalDownload(t *testing.T) {
	localPath := filepath.Join(t.TempDir(), "demo")
	artifactsDir := filepath.Join(t.TempDir(), "artifacts")
	var triggerBody client.BuildsTriggerOptions
	var downloadBody client.BuildsDownloadOptions

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/builds/trigger":
			if err := json.NewDecoder(r.Body).Decode(&triggerBody); err != nil {
				t.Fatalf("Decode(trigger) error = %v", err)
			}
			_ = json.NewEncoder(w).Encode(dto.BuildTriggerResult{
				Project: "demo",
				RecipeResults: []dto.BuildRecipeResult{{
					Name: "tmp-build-01234567-keystone",
					Builds: []dto.Build{{
						Project: "demo",
						Recipe:  "tmp-build-01234567-keystone",
						State:   dto.BuildSucceeded,
					}},
				}},
			})
		case "/api/v1/builds/download":
			if err := json.NewDecoder(r.Body).Decode(&downloadBody); err != nil {
				t.Fatalf("Decode(download) error = %v", err)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	preparer := NewLocalBuildPreparer(
		&fakeGitClient{headSHA: "0123456789abcdef0123456789abcdef01234567", currentBranch: "main"},
		&fakeRepoManager{
			currentUser:  "lp-user",
			project:      "lp-project",
			repoSelfLink: "https://api.launchpad.net/devel/~lp-user/lp-project/+git/demo",
			gitSSHURL:    "git+ssh://git.launchpad.net/~lp-user/lp-project/+git/demo",
			refSelfLink:  "https://api.launchpad.net/devel/~lp-user/lp-project/+git/demo/+ref/refs/heads/tmp-01234567",
			refErr:       fmt.Errorf("not found"),
		},
		map[string]build.ProjectBuilder{
			"demo": {
				Project:  "demo",
				Strategy: &fakeStrategy{},
			},
		},
		nil,
	)

	workflow := NewBuildWorkflow(NewClientTransport(client.NewClient(ts.URL)), preparer)
	got, err := workflow.Trigger(context.Background(), BuildTriggerRequest{
		Source:       "local",
		LocalPath:    localPath,
		Download:     true,
		ArtifactsDir: artifactsDir,
		Project:      "demo",
		Prefix:       "tmp-build",
		RetryCount:   2,
	})
	if err != nil {
		t.Fatalf("Trigger() error = %v", err)
	}
	if got.Result == nil {
		t.Fatal("Result = nil, want response")
	}
	if triggerBody.Prepared == nil || triggerBody.Prepared.TargetRef != "lp-project" {
		t.Fatalf("Prepared trigger body = %+v", triggerBody.Prepared)
	}
	if len(downloadBody.Artifacts) != 1 || !strings.HasPrefix(downloadBody.Artifacts[0], "tmp-build-01234567-") {
		t.Fatalf("download artifacts = %+v", downloadBody.Artifacts)
	}
	if downloadBody.ArtifactsDir != artifactsDir {
		t.Fatalf("ArtifactsDir = %q, want %q", downloadBody.ArtifactsDir, artifactsDir)
	}
	if downloadBody.RetryCount != 2 {
		t.Fatalf("download RetryCount = %d, want 2", downloadBody.RetryCount)
	}
}

func TestBuildWorkflowTriggerReportsWaitTimeoutAndDownloadFailure(t *testing.T) {
	downloadCalled := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/builds/trigger":
			_ = json.NewEncoder(w).Encode(dto.BuildTriggerResult{
				Project: "demo",
				RecipeResults: []dto.BuildRecipeResult{{
					Name: "keystone",
					Builds: []dto.Build{{
						Project:  "demo",
						Recipe:   "keystone",
						State:    dto.BuildSucceeded,
						SelfLink: "/build/amd64",
					}},
				}},
				WaitTimeout: &dto.BuildWaitTimeout{
					Timeout: "2h0m0s",
					Builds: []dto.BuildWaitTimeoutBuild{{
						Recipe:   "keystone",
						Arch:     "arm64",
						State:    "pending",
						URL:      "https://launchpad.test/build/arm64",
						SelfLink: "/build/arm64",
					}},
				},
			})
		case "/api/v1/builds/download":
			downloadCalled = true
			http.Error(w, "download failed", http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	workflow := NewBuildWorkflow(NewClientTransport(client.NewClient(ts.URL)), nil)
	response, err := workflow.Trigger(context.Background(), BuildTriggerRequest{
		Download: true,
		Project:  "demo",
		Wait:     true,
	})
	if err != nil {
		t.Fatalf("Trigger() error = %v", err)
	}
	if !downloadCalled {
		t.Fatal("download follow-up was not attempted")
	}
	if len(response.Errors) != 2 {
		t.Fatalf("response.Errors len = %d, want 2: %+v", len(response.Errors), response.Errors)
	}
	joined := errors.Join(response.Errors...).Error()
	for _, want := range []string{"timeout waiting for builds after 2h0m0s", "keystone", "arm64", "download: HTTP 500"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("joined error = %q, want to contain %q", joined, want)
		}
	}
}

func TestBuildWorkflowDownloadPassesRetryCount(t *testing.T) {
	var downloadBody client.BuildsDownloadOptions
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/builds/download" {
			t.Fatalf("path = %q, want download path", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&downloadBody); err != nil {
			t.Fatalf("Decode(download) error = %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	workflow := NewBuildWorkflow(NewClientTransport(client.NewClient(ts.URL)), nil)
	err := workflow.Download(context.Background(), BuildDownloadRequest{
		Project:    "demo",
		Artifacts:  []string{"keystone"},
		RetryCount: 3,
	})
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if downloadBody.RetryCount != 3 {
		t.Fatalf("download RetryCount = %d, want 3", downloadBody.RetryCount)
	}
}

func TestBuildWorkflowListLocal(t *testing.T) {
	var query string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{"builds": []dto.Build{}})
	}))
	defer ts.Close()

	preparer := NewLocalBuildPreparer(
		nil,
		&fakeRepoManager{currentUser: "lp-user", project: "lp-project"},
		nil,
		nil,
	)

	workflow := NewBuildWorkflow(NewClientTransport(client.NewClient(ts.URL)), preparer)
	_, err := workflow.List(context.Background(), BuildListRequest{
		Source:     "local",
		SHA:        "01234567",
		Prefix:     "tmp-build-",
		DefaultAll: true,
		Projects:   []string{"demo"},
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if !strings.Contains(query, "all=true") || !strings.Contains(query, "recipe_prefix=tmp-build-01234567-") || !strings.Contains(query, "target_ref=lp-project") {
		t.Fatalf("unexpected query: %q", query)
	}
}

func TestBuildWorkflowCleanup(t *testing.T) {
	var gotBody client.BuildsCleanupOptions

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/builds/cleanup" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		_ = json.NewEncoder(w).Encode(client.BuildsCleanupResult{
			DeletedRecipes:  []string{"tmp-build-keystone", "tmp-build-glance"},
			DeletedBranches: []string{"refs/heads/tmp-build-abc12345"},
		})
	}))
	defer ts.Close()

	workflow := NewBuildWorkflow(NewClientTransport(client.NewClient(ts.URL)), nil)
	got, err := workflow.Cleanup(context.Background(), BuildCleanupRequest{
		Project: "keystone",
		Owner:   "team-a",
		Prefix:  "tmp-build",
		DryRun:  true,
	})
	if err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if gotBody.Project != "keystone" || gotBody.Owner != "team-a" || gotBody.Prefix != "tmp-build" || !gotBody.DryRun {
		t.Fatalf("cleanup body = %+v, want keystone/team-a/tmp-build dry-run", gotBody)
	}
	if got == nil {
		t.Fatal("Cleanup() returned nil")
	}
	if len(got.DeletedRecipes) != 2 || got.DeletedRecipes[0] != "tmp-build-keystone" {
		t.Fatalf("Cleanup().DeletedRecipes = %+v, want deleted recipes", got.DeletedRecipes)
	}
	if len(got.DeletedBranches) != 1 || got.DeletedBranches[0] != "refs/heads/tmp-build-abc12345" {
		t.Fatalf("Cleanup().DeletedBranches = %+v, want deleted branches", got.DeletedBranches)
	}
}
