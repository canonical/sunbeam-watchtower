// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// mockRecipeBuilder implements port.RecipeBuilder for testing.
type mockRecipeBuilder struct {
	artifactType dto.ArtifactType
	recipes      map[string]*dto.Recipe       // name → recipe
	builds       map[string][]dto.Build       // recipe SelfLink → builds
	buildReqs    map[string]*dto.BuildRequest // recipe SelfLink → request
	fileURLs     map[string][]string          // build SelfLink → file URLs
	ownerRecipes []*dto.Recipe                // ListRecipesByOwner result
	createErr    error
	requestErr   error
	listErr      error
	retryErr     error
	ownerListErr error
	retried      []string // tracks retried build self links

	// processorsSet tracks SetProcessors calls (recipe SelfLink → processors).
	processorsSet map[string][]string

	// retryHook lets tests mutate builds in response to a retry call,
	// simulating LP transitioning the build record (e.g. Failed → Pending
	// → Succeeded across poll cycles).
	retryHook func(buildSelfLink string)
}

func (m *mockRecipeBuilder) ArtifactType() dto.ArtifactType { return m.artifactType }

func (m *mockRecipeBuilder) GetRecipe(_ context.Context, _, _, name string) (*dto.Recipe, error) {
	r, ok := m.recipes[name]
	if !ok {
		return nil, fmt.Errorf("recipe %q not found", name)
	}
	return r, nil
}

func (m *mockRecipeBuilder) CreateRecipe(_ context.Context, opts dto.CreateRecipeOpts) (*dto.Recipe, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	r := &dto.Recipe{
		Name:     opts.Name,
		SelfLink: "/~" + opts.Owner + "/" + opts.Project + "/+recipe/" + opts.Name,
	}
	if m.recipes == nil {
		m.recipes = make(map[string]*dto.Recipe)
	}
	m.recipes[opts.Name] = r
	return r, nil
}

func (m *mockRecipeBuilder) DeleteRecipe(_ context.Context, _ string) error { return nil }

func (m *mockRecipeBuilder) SetProcessors(_ context.Context, recipe *dto.Recipe, processors []string) error {
	if m.processorsSet == nil {
		m.processorsSet = make(map[string][]string)
	}
	if recipe != nil {
		m.processorsSet[recipe.SelfLink] = append([]string(nil), processors...)
	}
	return nil
}

func (m *mockRecipeBuilder) RequestBuilds(_ context.Context, recipe *dto.Recipe, _ dto.RequestBuildsOpts) (*dto.BuildRequest, error) {
	if m.requestErr != nil {
		return nil, m.requestErr
	}
	// Simulate LP creating build records asynchronously: if no builds
	// exist for this recipe yet, add a pending one so waitForBuildRecords
	// finds it.
	if m.builds == nil {
		m.builds = make(map[string][]dto.Build)
	}
	if len(m.builds[recipe.SelfLink]) == 0 {
		m.builds[recipe.SelfLink] = []dto.Build{
			{Recipe: recipe.Name, State: dto.BuildPending, SelfLink: recipe.SelfLink + "/+build/1"},
		}
	}
	if br, ok := m.buildReqs[recipe.SelfLink]; ok {
		return br, nil
	}
	return &dto.BuildRequest{SelfLink: recipe.SelfLink + "/+request/1"}, nil
}

func (m *mockRecipeBuilder) ListBuilds(_ context.Context, recipe *dto.Recipe) ([]dto.Build, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.builds[recipe.SelfLink], nil
}

func (m *mockRecipeBuilder) RetryBuild(_ context.Context, buildSelfLink string) error {
	m.retried = append(m.retried, buildSelfLink)
	if m.retryErr != nil {
		return m.retryErr
	}
	if m.retryHook != nil {
		m.retryHook(buildSelfLink)
	}
	return nil
}

func (m *mockRecipeBuilder) CancelBuild(_ context.Context, _ string) error { return nil }

func (m *mockRecipeBuilder) GetBuildFileURLs(_ context.Context, buildSelfLink string) ([]string, error) {
	return m.fileURLs[buildSelfLink], nil
}

func (m *mockRecipeBuilder) ListRecipesByOwner(_ context.Context, _ string) ([]*dto.Recipe, error) {
	if m.ownerListErr != nil {
		return nil, m.ownerListErr
	}
	return m.ownerRecipes, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func responseWithBody(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestDownloadFileRetriesTransientTransportError(t *testing.T) {
	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return nil, errors.New("connection reset by peer")
		}
		return responseWithBody(http.StatusOK, "artifact"), nil
	})}

	outputDir := t.TempDir()
	err := downloadFile(context.Background(), client, "https://launchpad.test/build/+files/pkg.charm", outputDir, "keystone", 2)
	if err != nil {
		t.Fatalf("downloadFile() error = %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	got, err := os.ReadFile(filepath.Join(outputDir, "keystone", "pkg.charm"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "artifact" {
		t.Fatalf("file content = %q, want artifact", got)
	}
}

func TestDownloadFilePreservesUmaskControlledPermissions(t *testing.T) {
	oldUmask := syscall.Umask(0o022)
	defer syscall.Umask(oldUmask)

	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return responseWithBody(http.StatusOK, "artifact"), nil
	})}

	outputDir := t.TempDir()
	err := downloadFile(context.Background(), client, "https://launchpad.test/build/+files/pkg.charm", outputDir, "keystone", 1)
	if err != nil {
		t.Fatalf("downloadFile() error = %v", err)
	}
	info, err := os.Stat(filepath.Join(outputDir, "keystone", "pkg.charm"))
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("file mode = %o, want 644", got)
	}
}

func TestDownloadFileRetriesTransientHTTPStatus(t *testing.T) {
	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return responseWithBody(http.StatusInternalServerError, "temporary"), nil
		}
		return responseWithBody(http.StatusOK, "artifact"), nil
	})}

	err := downloadFile(context.Background(), client, "https://launchpad.test/build/+files/pkg.charm", t.TempDir(), "keystone", 2)
	if err != nil {
		t.Fatalf("downloadFile() error = %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestDownloadFileDoesNotRetryPermanentHTTPStatus(t *testing.T) {
	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		return responseWithBody(http.StatusNotFound, "missing"), nil
	})}

	err := downloadFile(context.Background(), client, "https://launchpad.test/build/+files/pkg.charm", t.TempDir(), "keystone", 3)
	if err == nil {
		t.Fatal("downloadFile() error = nil, want status error")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestDownloadFileReturnsFinalErrorAfterRetryExhaustion(t *testing.T) {
	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		return nil, errors.New("connection reset by peer")
	})}

	err := downloadFile(context.Background(), client, "https://launchpad.test/build/+files/pkg.charm", t.TempDir(), "keystone", 2)
	if err == nil {
		t.Fatal("downloadFile() error = nil, want retry exhaustion error")
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if !strings.Contains(err.Error(), "connection reset by peer") {
		t.Fatalf("error = %v, want final transport error", err)
	}
}

func TestDownloadAttemptsRemainingFilesAfterFailure(t *testing.T) {
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{"keystone": recipe},
		builds: map[string][]dto.Build{
			"/recipe/keystone": {
				{Project: "sunbeam", Recipe: "keystone", State: dto.BuildSucceeded, Arch: "amd64", SelfLink: "/build/amd64", WebLink: "https://launchpad.test/build/amd64"},
			},
		},
	}

	attempts := map[string]int{}
	var attemptsMu sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptsMu.Lock()
		defer attemptsMu.Unlock()
		attempts[r.URL.Path]++
		switch r.URL.Path {
		case "/bad.charm":
			http.Error(w, "temporary", http.StatusInternalServerError)
		case "/good.charm":
			_, _ = w.Write([]byte("artifact"))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()
	builder.fileURLs = map[string][]string{
		"/build/amd64": {ts.URL + "/bad.charm", ts.URL + "/good.charm"},
	}

	svc := NewService(
		map[string]ProjectBuilder{
			"sunbeam": {Builder: builder, Owner: "team", Project: "sunbeam", Artifacts: []string{"keystone"}, Strategy: &mockStrategy{}},
		},
		nil, testLogger(),
	)

	outputDir := t.TempDir()
	err := svc.Download(context.Background(), DownloadOpts{
		Projects:      []string{"sunbeam"},
		ArtifactNames: []string{"keystone"},
		OutputDir:     outputDir,
		RetryCount:    2,
	})
	if err == nil {
		t.Fatal("Download() error = nil, want aggregate download error")
	}
	if attempts["/bad.charm"] != 2 {
		t.Fatalf("bad.charm attempts = %d, want 2", attempts["/bad.charm"])
	}
	if attempts["/good.charm"] != 1 {
		t.Fatalf("good.charm attempts = %d, want 1", attempts["/good.charm"])
	}
	if _, readErr := os.ReadFile(filepath.Join(outputDir, "keystone", "good.charm")); readErr != nil {
		t.Fatalf("good artifact was not downloaded: %v", readErr)
	}
	for _, want := range []string{"keystone", "amd64", "bad.charm", "https://launchpad.test/build/amd64"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Download() error = %q, want to contain %q", err.Error(), want)
		}
	}
}

func TestDownloadAggregatesMultipleFailedFiles(t *testing.T) {
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{"keystone": recipe},
		builds: map[string][]dto.Build{
			"/recipe/keystone": {
				{Project: "sunbeam", Recipe: "keystone", State: dto.BuildSucceeded, Arch: "amd64", SelfLink: "/build/amd64", WebLink: "https://launchpad.test/build/amd64"},
			},
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "temporary", http.StatusInternalServerError)
	}))
	defer ts.Close()
	builder.fileURLs = map[string][]string{
		"/build/amd64": {ts.URL + "/first.charm", ts.URL + "/second.charm"},
	}

	svc := NewService(
		map[string]ProjectBuilder{
			"sunbeam": {Builder: builder, Owner: "team", Project: "sunbeam", Artifacts: []string{"keystone"}, Strategy: &mockStrategy{}},
		},
		nil, testLogger(),
	)

	err := svc.Download(context.Background(), DownloadOpts{
		Projects:      []string{"sunbeam"},
		ArtifactNames: []string{"keystone"},
		OutputDir:     t.TempDir(),
		RetryCount:    1,
	})
	if err == nil {
		t.Fatal("Download() error = nil, want aggregate download error")
	}
	for _, want := range []string{"first.charm", "second.charm"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Download() error = %q, want to contain %q", err.Error(), want)
		}
	}
}

func TestDownloadRetryCountAppliesPerFile(t *testing.T) {
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{"keystone": recipe},
		builds: map[string][]dto.Build{
			"/recipe/keystone": {
				{Project: "sunbeam", Recipe: "keystone", State: dto.BuildSucceeded, Arch: "amd64", SelfLink: "/build/amd64", WebLink: "https://launchpad.test/build/amd64"},
			},
		},
	}

	attempts := map[string]int{}
	var attemptsMu sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptsMu.Lock()
		defer attemptsMu.Unlock()
		attempts[r.URL.Path]++
		if attempts[r.URL.Path] == 1 {
			http.Error(w, "temporary", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("artifact"))
	}))
	defer ts.Close()
	builder.fileURLs = map[string][]string{
		"/build/amd64": {ts.URL + "/first.charm", ts.URL + "/second.charm"},
	}

	svc := NewService(
		map[string]ProjectBuilder{
			"sunbeam": {Builder: builder, Owner: "team", Project: "sunbeam", Artifacts: []string{"keystone"}, Strategy: &mockStrategy{}},
		},
		nil, testLogger(),
	)

	err := svc.Download(context.Background(), DownloadOpts{
		Projects:      []string{"sunbeam"},
		ArtifactNames: []string{"keystone"},
		OutputDir:     t.TempDir(),
		RetryCount:    2,
	})
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	for _, path := range []string{"/first.charm", "/second.charm"} {
		if attempts[path] != 2 {
			t.Fatalf("%s attempts = %d, want 2", path, attempts[path])
		}
	}
}

// mockRepoManager implements port.RepoManager for testing.
type mockRepoManager struct {
	currentUser   string
	project       string
	repoSelfLink  string
	gitSSHURL     string
	refSelfLink   string
	defaultBranch string
	createErr     error
	repoErr       error
	refErr        error
	defaultErr    error
	branches      []dto.BranchRef
	deleteErr     error
	deleteMu      sync.Mutex
	deletedRefs   []string
}

func (m *mockRepoManager) GetCurrentUser(_ context.Context) (string, error) {
	if m.currentUser != "" {
		return m.currentUser, nil
	}
	return "test-user", nil
}

func (m *mockRepoManager) GetDefaultRepo(_ context.Context, projectName string) (string, string, error) {
	if m.defaultErr != nil {
		return "", "", m.defaultErr
	}
	link := m.repoSelfLink
	if link == "" {
		link = "https://api.launchpad.net/devel/~owner/" + projectName + "/+git/" + projectName
	}
	branch := m.defaultBranch
	if branch == "" {
		branch = "main"
	}
	return link, branch, nil
}

func (m *mockRepoManager) GetOrCreateProject(_ context.Context, _ string) (string, error) {
	if m.createErr != nil {
		return "", m.createErr
	}
	return m.project, nil
}

func (m *mockRepoManager) GetOrCreateRepo(_ context.Context, _, _, _ string) (string, string, error) {
	if m.repoErr != nil {
		return "", "", m.repoErr
	}
	return m.repoSelfLink, m.gitSSHURL, nil
}

func (m *mockRepoManager) GetGitRef(_ context.Context, _, _ string) (string, error) {
	if m.refErr != nil {
		return "", m.refErr
	}
	return m.refSelfLink, nil
}

func (m *mockRepoManager) WaitForGitRef(_ context.Context, _, _ string, _ time.Duration) (string, error) {
	if m.refErr != nil {
		return "", m.refErr
	}
	return m.refSelfLink, nil
}

func (m *mockRepoManager) ListBranches(_ context.Context, _ string) ([]dto.BranchRef, error) {
	return m.branches, nil
}

func (m *mockRepoManager) DeleteGitRef(_ context.Context, ref string) error {
	m.deleteMu.Lock()
	defer m.deleteMu.Unlock()
	m.deletedRefs = append(m.deletedRefs, ref)
	return m.deleteErr
}

// mockStrategy implements ArtifactStrategy for testing.
type mockStrategy struct {
	artifactType dto.ArtifactType
}

func (m *mockStrategy) ArtifactType() dto.ArtifactType            { return m.artifactType }
func (m *mockStrategy) MetadataFileName() string                  { return "rockcraft.yaml" }
func (m *mockStrategy) BuildPath(name string) string              { return "rocks/" + name }
func (m *mockStrategy) ParsePlatforms(_ []byte) ([]string, error) { return []string{"amd64"}, nil }
func (m *mockStrategy) TempRecipeName(name, sha, prefix string) string {
	return prefix + "-" + sha[:8] + "-" + name
}
func (m *mockStrategy) DiscoverRecipes(_ string) ([]DiscoveredRecipe, error) {
	return []DiscoveredRecipe{{Name: "discovered-recipe", RelPath: "rocks/discovered-recipe"}}, nil
}
func (m *mockStrategy) OfficialRecipeName(name, series, devFocus string) string {
	if series == devFocus {
		return name
	}
	return name + "-" + series
}
func (m *mockStrategy) BranchForSeries(series, devFocus, defaultBranch string) string {
	if series == devFocus {
		return defaultBranch
	}
	return "stable/" + series
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// ---------------------------------------------------------------------------
// Trigger tests
// ---------------------------------------------------------------------------

func TestTrigger_RemoteMode_RequestBuilds(t *testing.T) {
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{"keystone": recipe},
		builds:  map[string][]dto.Build{"/recipe/keystone": {}},
	}

	svc := NewService(
		map[string]ProjectBuilder{
			"sunbeam": {Builder: builder, Owner: "team", Project: "sunbeam", Artifacts: []string{"keystone"}, Strategy: &mockStrategy{}},
		},
		nil, testLogger(),
	)

	result, err := svc.Trigger(context.Background(), "sunbeam", nil, TriggerOpts{})
	if err != nil {
		t.Fatalf("Trigger() error: %v", err)
	}

	if len(result.RecipeResults) != 1 {
		t.Fatalf("expected 1 recipe result, got %d", len(result.RecipeResults))
	}
	rr := result.RecipeResults[0]
	if rr.Action != ActionRequestBuilds {
		t.Errorf("Action = %d, want ActionRequestBuilds (%d)", rr.Action, ActionRequestBuilds)
	}
	if rr.BuildRequest == nil {
		t.Error("expected BuildRequest to be set")
	}
}

func TestTrigger_RemoteMode_AllSucceeded(t *testing.T) {
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{"keystone": recipe},
		builds: map[string][]dto.Build{
			"/recipe/keystone": {
				{Recipe: "keystone", State: dto.BuildSucceeded, Arch: "amd64", SelfLink: "/build/1"},
				{Recipe: "keystone", State: dto.BuildSucceeded, Arch: "arm64", SelfLink: "/build/2"},
			},
		},
	}

	svc := NewService(
		map[string]ProjectBuilder{
			"sunbeam": {Builder: builder, Owner: "team", Project: "sunbeam", Artifacts: []string{"keystone"}, Strategy: &mockStrategy{}},
		},
		nil, testLogger(),
	)

	result, err := svc.Trigger(context.Background(), "sunbeam", nil, TriggerOpts{})
	if err != nil {
		t.Fatalf("Trigger() error: %v", err)
	}

	rr := result.RecipeResults[0]
	if rr.Action != ActionDownload {
		t.Errorf("Action = %d, want ActionDownload (%d)", rr.Action, ActionDownload)
	}
	if len(rr.Builds) != 2 {
		t.Errorf("expected 2 builds, got %d", len(rr.Builds))
	}
}

func TestTrigger_RemoteMode_RetryFailed(t *testing.T) {
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{"keystone": recipe},
		builds: map[string][]dto.Build{
			"/recipe/keystone": {
				{Recipe: "keystone", State: dto.BuildSucceeded, Arch: "amd64", SelfLink: "/build/1"},
				{Recipe: "keystone", State: dto.BuildFailed, Arch: "arm64", SelfLink: "/build/2", CanRetry: true},
			},
		},
	}

	svc := NewService(
		map[string]ProjectBuilder{
			"sunbeam": {Builder: builder, Owner: "team", Project: "sunbeam", Artifacts: []string{"keystone"}, Strategy: &mockStrategy{}},
		},
		nil, testLogger(),
	)

	result, err := svc.Trigger(context.Background(), "sunbeam", nil, TriggerOpts{})
	if err != nil {
		t.Fatalf("Trigger() error: %v", err)
	}

	rr := result.RecipeResults[0]
	// executeAction converts ActionRetryFailed → ActionMonitor after retrying
	if rr.Action != ActionMonitor {
		t.Errorf("Action = %d, want ActionMonitor (%d)", rr.Action, ActionMonitor)
	}
	if len(builder.retried) != 1 || builder.retried[0] != "/build/2" {
		t.Errorf("expected retry of /build/2, got %v", builder.retried)
	}
}

func TestTrigger_RemoteMode_MonitorActive(t *testing.T) {
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{"keystone": recipe},
		builds: map[string][]dto.Build{
			"/recipe/keystone": {
				{Recipe: "keystone", State: dto.BuildBuilding, Arch: "amd64", SelfLink: "/build/1"},
				{Recipe: "keystone", State: dto.BuildPending, Arch: "arm64", SelfLink: "/build/2"},
			},
		},
	}

	svc := NewService(
		map[string]ProjectBuilder{
			"sunbeam": {Builder: builder, Owner: "team", Project: "sunbeam", Artifacts: []string{"keystone"}, Strategy: &mockStrategy{}},
		},
		nil, testLogger(),
	)

	result, err := svc.Trigger(context.Background(), "sunbeam", nil, TriggerOpts{})
	if err != nil {
		t.Fatalf("Trigger() error: %v", err)
	}

	rr := result.RecipeResults[0]
	if rr.Action != ActionMonitor {
		t.Errorf("Action = %d, want ActionMonitor (%d)", rr.Action, ActionMonitor)
	}
	if len(rr.Builds) != 2 {
		t.Errorf("expected 2 builds, got %d", len(rr.Builds))
	}
}

func TestTrigger_RemoteMode_CreateRecipe(t *testing.T) {
	// Recipe doesn't exist → in remote mode, should error (create only in local mode).
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{},
	}

	svc := NewService(
		map[string]ProjectBuilder{
			"sunbeam": {Builder: builder, Owner: "team", Project: "sunbeam", Artifacts: []string{"keystone"}, Strategy: &mockStrategy{}},
		},
		nil, testLogger(),
	)

	result, err := svc.Trigger(context.Background(), "sunbeam", nil, TriggerOpts{})
	if err != nil {
		t.Fatalf("Trigger() error: %v", err)
	}

	rr := result.RecipeResults[0]
	if rr.Error == nil {
		t.Fatal("expected error for create recipe in remote mode")
	}
}

func TestTrigger_PreResolvedRefs_FullPipeline(t *testing.T) {
	// Simulates the CLI passing pre-resolved LP resources (as in local mode).
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{},
	}

	svc := NewService(
		map[string]ProjectBuilder{
			"sunbeam": {Builder: builder, Owner: "team", Project: "sunbeam", Artifacts: []string{"keystone"}, Strategy: &mockStrategy{}},
		},
		nil, testLogger(),
	)

	tempName := "tmp-abc12345-keystone"
	result, err := svc.Trigger(context.Background(), "sunbeam", []string{tempName}, TriggerOpts{
		Owner: "test-user",
		Prepared: &dto.PreparedBuildSource{
			Backend:       dto.PreparedBuildBackendLaunchpad,
			TargetRef:     "test-project",
			RepositoryRef: "/repo/sunbeam",
			Recipes: map[string]dto.PreparedBuildRecipe{
				tempName: {SourceRef: "/ref/abc12345", BuildPath: "rocks/keystone"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Trigger() error: %v", err)
	}

	if len(result.RecipeResults) != 1 {
		t.Fatalf("expected 1 recipe result, got %d", len(result.RecipeResults))
	}

	rr := result.RecipeResults[0]
	if rr.Error != nil {
		t.Errorf("unexpected error: %v", rr.Error)
	}
	if rr.BuildRequest == nil {
		t.Error("expected BuildRequest to be set after create+request")
	}

	if _, ok := builder.recipes[tempName]; !ok {
		t.Errorf("expected recipe %q to be created, got keys: %v", tempName, recipeKeys(builder.recipes))
	}
}

func TestTrigger_MultipleRecipes(t *testing.T) {
	rKeystone := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	rNova := &dto.Recipe{Name: "nova", SelfLink: "/recipe/nova"}
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{"keystone": rKeystone, "nova": rNova},
		builds: map[string][]dto.Build{
			"/recipe/keystone": {},
			"/recipe/nova":     {{Recipe: "nova", State: dto.BuildSucceeded, SelfLink: "/build/n1"}},
		},
	}

	svc := NewService(
		map[string]ProjectBuilder{
			"sunbeam": {Builder: builder, Owner: "team", Project: "sunbeam", Artifacts: []string{"keystone", "nova"}, Strategy: &mockStrategy{}},
		},
		nil, testLogger(),
	)

	result, err := svc.Trigger(context.Background(), "sunbeam", nil, TriggerOpts{})
	if err != nil {
		t.Fatalf("Trigger() error: %v", err)
	}

	if len(result.RecipeResults) != 2 {
		t.Fatalf("expected 2 recipe results, got %d", len(result.RecipeResults))
	}

	actions := map[string]RecipeAction{}
	for _, rr := range result.RecipeResults {
		actions[rr.Name] = rr.Action
	}
	if actions["keystone"] != ActionRequestBuilds {
		t.Errorf("keystone action = %d, want ActionRequestBuilds", actions["keystone"])
	}
	if actions["nova"] != ActionDownload {
		t.Errorf("nova action = %d, want ActionDownload", actions["nova"])
	}
}

func TestTrigger_UnknownProject(t *testing.T) {
	svc := NewService(map[string]ProjectBuilder{}, nil, testLogger())

	_, err := svc.Trigger(context.Background(), "nonexistent", nil, TriggerOpts{})
	if err == nil {
		t.Fatal("expected error for unknown project")
	}
}

// ---------------------------------------------------------------------------
// List tests
// ---------------------------------------------------------------------------

func TestList_ActiveOnly(t *testing.T) {
	now := time.Now()
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{"keystone": recipe},
		builds: map[string][]dto.Build{
			"/recipe/keystone": {
				{Recipe: "keystone", State: dto.BuildBuilding, SelfLink: "/b/1", CreatedAt: now},
				{Recipe: "keystone", State: dto.BuildSucceeded, SelfLink: "/b/2", CreatedAt: now.Add(-time.Hour)},
			},
		},
	}

	svc := NewService(
		map[string]ProjectBuilder{
			"sunbeam": {Builder: builder, Owner: "team", Project: "sunbeam", Artifacts: []string{"keystone"}, Strategy: &mockStrategy{}},
		},
		nil, testLogger(),
	)

	builds, _, err := svc.List(context.Background(), ListOpts{})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}

	// Default (All=false) should return only active builds.
	if len(builds) != 1 {
		t.Fatalf("expected 1 active build, got %d", len(builds))
	}
	if builds[0].State != dto.BuildBuilding {
		t.Errorf("State = %v, want BuildBuilding", builds[0].State)
	}
}

func TestList_AllBuilds(t *testing.T) {
	now := time.Now()
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{"keystone": recipe},
		builds: map[string][]dto.Build{
			"/recipe/keystone": {
				{Recipe: "keystone", State: dto.BuildBuilding, SelfLink: "/b/1", CreatedAt: now},
				{Recipe: "keystone", State: dto.BuildSucceeded, SelfLink: "/b/2", CreatedAt: now.Add(-time.Hour)},
				{Recipe: "keystone", State: dto.BuildFailed, SelfLink: "/b/3", CreatedAt: now.Add(-2 * time.Hour)},
			},
		},
	}

	svc := NewService(
		map[string]ProjectBuilder{
			"sunbeam": {Builder: builder, Owner: "team", Project: "sunbeam", Artifacts: []string{"keystone"}, Strategy: &mockStrategy{}},
		},
		nil, testLogger(),
	)

	builds, _, err := svc.List(context.Background(), ListOpts{All: true})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(builds) != 3 {
		t.Errorf("expected 3 builds, got %d", len(builds))
	}
}

func TestList_ProjectFilter(t *testing.T) {
	rA := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	rB := &dto.Recipe{Name: "nova", SelfLink: "/recipe/nova"}
	builderA := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{"keystone": rA},
		builds: map[string][]dto.Build{
			"/recipe/keystone": {{Recipe: "keystone", State: dto.BuildBuilding, SelfLink: "/b/a1"}},
		},
	}
	builderB := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{"nova": rB},
		builds: map[string][]dto.Build{
			"/recipe/nova": {{Recipe: "nova", State: dto.BuildBuilding, SelfLink: "/b/b1"}},
		},
	}

	svc := NewService(
		map[string]ProjectBuilder{
			"projA": {Builder: builderA, Owner: "team", Project: "projA", Artifacts: []string{"keystone"}, Strategy: &mockStrategy{}},
			"projB": {Builder: builderB, Owner: "team", Project: "projB", Artifacts: []string{"nova"}, Strategy: &mockStrategy{}},
		},
		nil, testLogger(),
	)

	builds, _, err := svc.List(context.Background(), ListOpts{Projects: []string{"projA"}})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}

	if len(builds) != 1 {
		t.Fatalf("expected 1 build from projA, got %d", len(builds))
	}
	if builds[0].Project != "projA" {
		t.Errorf("Project = %q, want projA", builds[0].Project)
	}
}

func TestList_GracefulDegradation(t *testing.T) {
	goodRecipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	goodBuilder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{"keystone": goodRecipe},
		builds: map[string][]dto.Build{
			"/recipe/keystone": {{Recipe: "keystone", State: dto.BuildBuilding, SelfLink: "/b/1"}},
		},
	}
	badBuilder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{}, // GetRecipe will fail
	}

	svc := NewService(
		map[string]ProjectBuilder{
			"good": {Builder: goodBuilder, Owner: "team", Project: "good", Artifacts: []string{"keystone"}, Strategy: &mockStrategy{}},
			"bad":  {Builder: badBuilder, Owner: "team", Project: "bad", Artifacts: []string{"missing"}, Strategy: &mockStrategy{}},
		},
		nil, testLogger(),
	)

	builds, results, err := svc.List(context.Background(), ListOpts{})
	if err != nil {
		t.Fatalf("List() should not return top-level error: %v", err)
	}

	if len(builds) != 1 {
		t.Errorf("expected 1 build from good project, got %d", len(builds))
	}

	// Both projects should have results (no top-level error for bad project).
	if len(results) != 2 {
		t.Errorf("expected 2 project results, got %d", len(results))
	}
}

func TestList_Sorting(t *testing.T) {
	now := time.Now()
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{"keystone": recipe},
		builds: map[string][]dto.Build{
			"/recipe/keystone": {
				{Recipe: "keystone", State: dto.BuildBuilding, SelfLink: "/b/old", CreatedAt: now.Add(-2 * time.Hour)},
				{Recipe: "keystone", State: dto.BuildBuilding, SelfLink: "/b/new", CreatedAt: now},
				{Recipe: "keystone", State: dto.BuildBuilding, SelfLink: "/b/mid", CreatedAt: now.Add(-1 * time.Hour)},
			},
		},
	}

	svc := NewService(
		map[string]ProjectBuilder{
			"sunbeam": {Builder: builder, Owner: "team", Project: "sunbeam", Artifacts: []string{"keystone"}, Strategy: &mockStrategy{}},
		},
		nil, testLogger(),
	)

	builds, _, err := svc.List(context.Background(), ListOpts{All: true})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}

	if len(builds) != 3 {
		t.Fatalf("expected 3 builds, got %d", len(builds))
	}

	// Should be sorted by CreatedAt descending.
	for i := 1; i < len(builds); i++ {
		if builds[i].CreatedAt.After(builds[i-1].CreatedAt) {
			t.Errorf("builds[%d].CreatedAt (%v) > builds[%d].CreatedAt (%v) — not sorted descending",
				i, builds[i].CreatedAt, i-1, builds[i-1].CreatedAt)
		}
	}
}

// ---------------------------------------------------------------------------
// Trigger: local mode resolves owner from Me()
// ---------------------------------------------------------------------------

func TestTrigger_PreResolved_OwnerOverride(t *testing.T) {
	// Project has NO owner configured; pre-resolved opts provide owner.
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{},
	}

	svc := NewService(
		map[string]ProjectBuilder{
			"sunbeam": {Builder: builder, Owner: "", Project: "sunbeam", Artifacts: []string{"keystone"}, Strategy: &mockStrategy{}},
		},
		nil, testLogger(),
	)

	tempName := "tmp-abc12345-keystone"
	result, err := svc.Trigger(context.Background(), "sunbeam", []string{tempName}, TriggerOpts{
		Owner: "test-user",
		Prepared: &dto.PreparedBuildSource{
			Backend:       dto.PreparedBuildBackendLaunchpad,
			TargetRef:     "test-project",
			RepositoryRef: "/repo/sunbeam",
			Recipes: map[string]dto.PreparedBuildRecipe{
				tempName: {SourceRef: "/ref/abc12345", BuildPath: "rocks/keystone"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Trigger() error: %v", err)
	}

	if len(result.RecipeResults) != 1 {
		t.Fatalf("expected 1 recipe result, got %d", len(result.RecipeResults))
	}
	rr := result.RecipeResults[0]
	if rr.Error != nil {
		t.Errorf("unexpected error: %v", rr.Error)
	}
}

// ---------------------------------------------------------------------------
// Trigger: remote mode with official codehosting expands series
// ---------------------------------------------------------------------------

func TestTrigger_RemoteMode_UsesOfficialRepo(t *testing.T) {
	// All recipes start as not-found so the service tries to create them.
	// The mock CreateRecipe fills them in.
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{},
	}
	repoMgr := &mockRepoManager{
		repoSelfLink:  "/repo/rocks",
		defaultBranch: "main",
		refSelfLink:   "/ref/heads/main",
	}

	svc := NewService(
		map[string]ProjectBuilder{
			"sunbeam": {
				Builder:             builder,
				Owner:               "team",
				Project:             "ubuntu-openstack-rocks",
				Artifacts:           []string{"nova-consolidated"},
				Series:              []string{"2024.1", "2025.1"},
				DevFocus:            "2025.1",
				OfficialCodehosting: true,
				Strategy:            &mockStrategy{},
			},
		},
		repoMgr, testLogger(),
	)

	result, err := svc.Trigger(context.Background(), "sunbeam", []string{"nova-consolidated"}, TriggerOpts{})
	if err != nil {
		t.Fatalf("Trigger() error: %v", err)
	}

	// Should expand into 2 recipes: nova-consolidated-2024.1 and nova-consolidated
	if len(result.RecipeResults) != 2 {
		t.Fatalf("expected 2 recipe results, got %d", len(result.RecipeResults))
	}

	names := map[string]bool{}
	for _, rr := range result.RecipeResults {
		names[rr.Name] = true
	}
	if !names["nova-consolidated"] {
		t.Error("expected recipe 'nova-consolidated' (dev focus)")
	}
	if !names["nova-consolidated-2024.1"] {
		t.Error("expected recipe 'nova-consolidated-2024.1' (non-dev series)")
	}
}

// ---------------------------------------------------------------------------
// Trigger: remote mode without official_codehosting creates recipe error
// ---------------------------------------------------------------------------

func TestTrigger_RemoteMode_FailsWithoutOfficialCodehosting(t *testing.T) {
	// Without OfficialCodehosting=true, remote mode has no git info.
	// Recipes that don't exist will fail to create (no repoSelfLink).
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{},
	}

	svc := NewService(
		map[string]ProjectBuilder{
			"sunbeam": {
				Builder:             builder,
				Owner:               "team",
				Project:             "sunbeam",
				Artifacts:           []string{"keystone"},
				OfficialCodehosting: false,
				Strategy:            &mockStrategy{},
			},
		},
		nil, testLogger(),
	)

	result, err := svc.Trigger(context.Background(), "sunbeam", nil, TriggerOpts{})
	if err != nil {
		t.Fatalf("Trigger() error: %v", err)
	}

	if len(result.RecipeResults) != 1 {
		t.Fatalf("expected 1 recipe result, got %d", len(result.RecipeResults))
	}
	rr := result.RecipeResults[0]
	if rr.Error == nil {
		t.Fatal("expected error when recipe not found and OfficialCodehosting is false")
	}
}

// ---------------------------------------------------------------------------
// ProjectBuilder tests
// ---------------------------------------------------------------------------

func TestProjectBuilder_RecipeProject(t *testing.T) {
	pb := ProjectBuilder{Project: "ubuntu-openstack-rocks"}
	if got := pb.RecipeProject(); got != "ubuntu-openstack-rocks" {
		t.Errorf("RecipeProject() = %q, want %q", got, "ubuntu-openstack-rocks")
	}

	pb.LPProject = "custom-lp-project"
	if got := pb.RecipeProject(); got != "custom-lp-project" {
		t.Errorf("RecipeProject() = %q, want %q", got, "custom-lp-project")
	}
}

// ---------------------------------------------------------------------------
// Cleanup tests
// ---------------------------------------------------------------------------

func TestCleanup_PrefixDiscovery(t *testing.T) {
	// Verify cleanup discovers recipes by prefix via ListRecipesByOwner
	// (not from pb.Artifacts), and also deletes matching branches.
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{},
		ownerRecipes: []*dto.Recipe{
			{Name: "tmp-build-abc12345-keystone", SelfLink: "/recipe/tmp-build-abc12345-keystone", Project: "test-project"},
			{Name: "tmp-build-abc12345-glance", SelfLink: "/recipe/tmp-build-abc12345-glance", Project: "test-project"},
			{Name: "official-keystone", SelfLink: "/recipe/official-keystone", Project: "test-project"},
		},
	}

	repoMgr := &mockRepoManager{
		project:      "test-project",
		repoSelfLink: "https://api.launchpad.net/devel/~team/test-project/+git/rocks",
		gitSSHURL:    "git+ssh://git.launchpad.net/~team/test-project/+git/rocks",
		branches: []dto.BranchRef{
			{Path: "refs/heads/tmp-tmp-build-abc12345", SelfLink: "/ref/tmp-tmp-build-abc12345"},
			{Path: "refs/heads/tmp-tmp-build-def99999", SelfLink: "/ref/tmp-tmp-build-def99999"},
			{Path: "refs/heads/main", SelfLink: "/ref/main"},
			{Path: "refs/heads/tmp-build-abc12345", SelfLink: "/ref/unprefixed"},
			{Path: "refs/heads/tmp-tmp-builder-abc12345", SelfLink: "/ref/neighbor"},
		},
	}

	svc := NewService(
		map[string]ProjectBuilder{
			"sunbeam": {
				Builder:   builder,
				Owner:     "team",
				Project:   "rocks",
				LPProject: "test-project",
				Artifacts: []string{"keystone", "glance"},
				Strategy:  &mockStrategy{},
			},
		},
		repoMgr, testLogger(),
	)

	result, err := svc.Cleanup(context.Background(), CleanupOpts{
		Owner:  "team",
		Prefix: "tmp-build",
	})
	if err != nil {
		t.Fatalf("Cleanup() error: %v", err)
	}

	// Should discover and delete 2 recipes matching prefix (not the official one).
	if len(result.DeletedRecipes) != 2 {
		t.Errorf("expected 2 deleted recipes, got %d: %v", len(result.DeletedRecipes), result.DeletedRecipes)
	}
	for _, name := range result.DeletedRecipes {
		if name != "tmp-build-abc12345-keystone" && name != "tmp-build-abc12345-glance" {
			t.Errorf("unexpected deleted recipe: %q", name)
		}
	}

	// Should delete only the generated temporary branches matching the prefix.
	if len(result.DeletedBranches) != 2 {
		t.Errorf("expected 2 deleted branches, got %d: %v", len(result.DeletedBranches), result.DeletedBranches)
	}
	for _, branch := range result.DeletedBranches {
		if branch != "refs/heads/tmp-tmp-build-abc12345" && branch != "refs/heads/tmp-tmp-build-def99999" {
			t.Errorf("unexpected deleted branch: %q", branch)
		}
	}
	if len(repoMgr.deletedRefs) != 2 {
		t.Fatalf("DeleteGitRef calls = %v, want two matching refs", repoMgr.deletedRefs)
	}
	for _, ref := range repoMgr.deletedRefs {
		if ref != "/ref/tmp-tmp-build-abc12345" && ref != "/ref/tmp-tmp-build-def99999" {
			t.Errorf("unexpected deleted ref: %q", ref)
		}
	}
}

func TestCleanup_DryRun(t *testing.T) {
	builder := &mockRecipeBuilder{
		ownerRecipes: []*dto.Recipe{
			{Name: "tmp-build-abc12345-keystone", SelfLink: "/recipe/tmp-build-abc12345-keystone", Project: "test-project"},
		},
	}

	repoMgr := &mockRepoManager{
		project:      "test-project",
		repoSelfLink: "https://api.launchpad.net/devel/~team/test-project/+git/rocks",
		gitSSHURL:    "git+ssh://git.launchpad.net/~team/test-project/+git/rocks",
		branches: []dto.BranchRef{
			{Path: "refs/heads/tmp-tmp-build-abc12345", SelfLink: "/ref/tmp-tmp-build-abc12345"},
		},
	}

	svc := NewService(
		map[string]ProjectBuilder{
			"sunbeam": {
				Builder:   builder,
				Owner:     "team",
				Project:   "rocks",
				LPProject: "test-project",
				Artifacts: []string{"keystone"},
				Strategy:  &mockStrategy{},
			},
		},
		repoMgr, testLogger(),
	)

	result, err := svc.Cleanup(context.Background(), CleanupOpts{
		Owner:  "team",
		Prefix: "tmp-build",
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("Cleanup() error: %v", err)
	}

	// Dry run should still report names but not actually delete.
	if len(result.DeletedRecipes) != 1 {
		t.Errorf("expected 1 deleted recipe in dry run, got %d", len(result.DeletedRecipes))
	}
	if len(result.DeletedBranches) != 1 {
		t.Errorf("expected 1 deleted branch in dry run, got %d", len(result.DeletedBranches))
	}
	if len(repoMgr.deletedRefs) != 0 {
		t.Fatalf("dry run called DeleteGitRef: %v", repoMgr.deletedRefs)
	}
}

// ---------------------------------------------------------------------------
// Retry loop tests (waitForBuilds)
// ---------------------------------------------------------------------------

// newRetryService builds a Service configured with tiny poll + post-retry
// delays so wait-loop tests complete in milliseconds.
func newRetryService(t *testing.T, builder *mockRecipeBuilder) *Service {
	t.Helper()
	svc := NewService(
		map[string]ProjectBuilder{
			"sunbeam": {Builder: builder, Owner: "team", Project: "sunbeam", Artifacts: []string{"keystone"}, Strategy: &mockStrategy{}},
		},
		nil, testLogger(),
	)
	svc.pollInterval = 10 * time.Millisecond
	svc.postRetryDelay = 2 * time.Millisecond
	return svc
}

// setState replaces builds[recipeSelfLink] with a copy that sets the given
// build SelfLink to the requested state. Preserves other builds untouched.
func setState(builder *mockRecipeBuilder, recipeSelfLink, buildSelfLink string, state dto.BuildState) {
	src := builder.builds[recipeSelfLink]
	out := make([]dto.Build, len(src))
	copy(out, src)
	for i := range out {
		if out[i].SelfLink == buildSelfLink {
			out[i].State = state
		}
	}
	builder.builds[recipeSelfLink] = out
}

// Case 1: retryCount=1 behaves like today — no retries.
func TestTrigger_Wait_RetryCount1_NoRetries(t *testing.T) {
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{"keystone": recipe},
		builds: map[string][]dto.Build{
			"/recipe/keystone": {
				{Recipe: "keystone", State: dto.BuildFailed, Arch: "amd64", SelfLink: "/build/1", CanRetry: true},
			},
		},
	}
	svc := newRetryService(t, builder)

	_, err := svc.Trigger(context.Background(), "sunbeam", nil, TriggerOpts{
		Wait:       true,
		Timeout:    5 * time.Second,
		RetryCount: 1,
	})
	if err != nil {
		t.Fatalf("Trigger() error: %v", err)
	}

	// The existing legacy ActionRetryFailed path still fires once (before
	// waitForBuilds). The new retry loop must NOT add further retries.
	if got := len(builder.retried); got != 1 {
		t.Fatalf("expected 1 retry (legacy path), got %d: %v", got, builder.retried)
	}
}

// Case 2: retryCount=3, build fails once then succeeds on retry.
func TestTrigger_Wait_RetryCount3_SucceedsOnFirstRetry(t *testing.T) {
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{"keystone": recipe},
		builds: map[string][]dto.Build{
			"/recipe/keystone": {
				{Recipe: "keystone", State: dto.BuildFailed, Arch: "amd64", SelfLink: "/build/1", CanRetry: true},
			},
		},
	}
	// First retry transitions the build to Succeeded.
	builder.retryHook = func(selfLink string) {
		setState(builder, "/recipe/keystone", selfLink, dto.BuildSucceeded)
	}
	svc := newRetryService(t, builder)

	result, err := svc.Trigger(context.Background(), "sunbeam", nil, TriggerOpts{
		Wait:       true,
		Timeout:    5 * time.Second,
		RetryCount: 3,
	})
	if err != nil {
		t.Fatalf("Trigger() error: %v", err)
	}

	if got := len(builder.retried); got != 1 {
		t.Fatalf("expected exactly 1 retry call, got %d: %v", got, builder.retried)
	}

	rr := result.RecipeResults[0]
	if len(rr.Builds) != 1 {
		t.Fatalf("expected 1 build in result, got %d", len(rr.Builds))
	}
	if rr.Builds[0].State != dto.BuildSucceeded {
		t.Errorf("final build state = %v, want BuildSucceeded", rr.Builds[0].State)
	}
}

// Case 3: retryCount=3, build fails on every attempt — exactly 2 retries issued.
func TestTrigger_Wait_RetryCount3_FailsAllAttempts(t *testing.T) {
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{"keystone": recipe},
		builds: map[string][]dto.Build{
			"/recipe/keystone": {
				{Recipe: "keystone", State: dto.BuildFailed, Arch: "amd64", SelfLink: "/build/1", CanRetry: true},
			},
		},
	}
	// retryHook leaves state as Failed — build keeps failing forever.
	builder.retryHook = func(string) {}
	svc := newRetryService(t, builder)

	result, err := svc.Trigger(context.Background(), "sunbeam", nil, TriggerOpts{
		Wait:       true,
		Timeout:    5 * time.Second,
		RetryCount: 3,
	})
	if err != nil {
		t.Fatalf("Trigger() error: %v", err)
	}

	// retryCount=3 means initial + 2 retries = 2 RetryBuild calls.
	if got := len(builder.retried); got != 2 {
		t.Fatalf("expected 2 retry calls, got %d: %v", got, builder.retried)
	}

	rr := result.RecipeResults[0]
	if len(rr.Builds) != 1 || rr.Builds[0].State != dto.BuildFailed {
		t.Errorf("expected final state BuildFailed, got %+v", rr.Builds)
	}
}

// Case 4: CanRetry=false — zero retries issued regardless of retryCount.
func TestTrigger_Wait_RetryCount3_NonRetryable(t *testing.T) {
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{"keystone": recipe},
		builds: map[string][]dto.Build{
			"/recipe/keystone": {
				{Recipe: "keystone", State: dto.BuildFailed, Arch: "amd64", SelfLink: "/build/1", CanRetry: false},
			},
		},
	}
	svc := newRetryService(t, builder)

	_, err := svc.Trigger(context.Background(), "sunbeam", nil, TriggerOpts{
		Wait:       true,
		Timeout:    5 * time.Second,
		RetryCount: 3,
	})
	if err != nil {
		t.Fatalf("Trigger() error: %v", err)
	}

	if got := len(builder.retried); got != 0 {
		t.Fatalf("expected 0 retries for non-retryable build, got %d: %v", got, builder.retried)
	}
}

// Case 5: RetryBuild itself errors on the first call — budget clamped to 0,
// no further retries, build returned in failed state.
func TestTrigger_Wait_RetryCount3_RetryBuildErrors(t *testing.T) {
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{"keystone": recipe},
		builds: map[string][]dto.Build{
			"/recipe/keystone": {
				{Recipe: "keystone", State: dto.BuildFailed, Arch: "amd64", SelfLink: "/build/1", CanRetry: true},
			},
		},
		retryErr: fmt.Errorf("LP retry API error"),
	}
	svc := newRetryService(t, builder)

	_, err := svc.Trigger(context.Background(), "sunbeam", nil, TriggerOpts{
		Wait:       true,
		Timeout:    5 * time.Second,
		RetryCount: 3,
	})
	if err != nil {
		t.Fatalf("Trigger() error: %v", err)
	}

	// Only 1 RetryBuild call; after error, budget clamped to 0 so no further retries.
	if got := len(builder.retried); got != 1 {
		t.Fatalf("expected exactly 1 retry attempt, got %d: %v", got, builder.retried)
	}
}

// Case 6: multi-arch recipe — per-arch retry budget independence.
// arch A (/build/A): fails once then succeeds (1 retry)
// arch B (/build/B): green from the start (0 retries)
// arch C (/build/C): fails every attempt (2 retries, budget exhausted)
func TestTrigger_Wait_RetryCount3_MultiArchIndependence(t *testing.T) {
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{"keystone": recipe},
		builds: map[string][]dto.Build{
			"/recipe/keystone": {
				{Recipe: "keystone", State: dto.BuildFailed, Arch: "amd64", SelfLink: "/build/A", CanRetry: true},
				{Recipe: "keystone", State: dto.BuildSucceeded, Arch: "arm64", SelfLink: "/build/B"},
				{Recipe: "keystone", State: dto.BuildFailed, Arch: "s390x", SelfLink: "/build/C", CanRetry: true},
			},
		},
	}
	// Only arch A transitions to Succeeded on its first retry.
	builder.retryHook = func(selfLink string) {
		if selfLink == "/build/A" {
			setState(builder, "/recipe/keystone", "/build/A", dto.BuildSucceeded)
		}
	}
	svc := newRetryService(t, builder)

	result, err := svc.Trigger(context.Background(), "sunbeam", nil, TriggerOpts{
		Wait:       true,
		Timeout:    5 * time.Second,
		RetryCount: 3,
	})
	if err != nil {
		t.Fatalf("Trigger() error: %v", err)
	}

	// Expect: A retried once, C retried twice (budget exhausted), B never.
	countA, countB, countC := 0, 0, 0
	for _, sl := range builder.retried {
		switch sl {
		case "/build/A":
			countA++
		case "/build/B":
			countB++
		case "/build/C":
			countC++
		}
	}
	if countA != 1 {
		t.Errorf("expected /build/A retried 1 time, got %d", countA)
	}
	if countB != 0 {
		t.Errorf("expected /build/B retried 0 times, got %d", countB)
	}
	if countC != 2 {
		t.Errorf("expected /build/C retried 2 times, got %d", countC)
	}

	// Final states
	byArch := map[string]dto.BuildState{}
	for _, b := range result.RecipeResults[0].Builds {
		byArch[b.Arch] = b.State
	}
	if byArch["amd64"] != dto.BuildSucceeded {
		t.Errorf("amd64 state = %v, want BuildSucceeded", byArch["amd64"])
	}
	if byArch["arm64"] != dto.BuildSucceeded {
		t.Errorf("arm64 state = %v, want BuildSucceeded", byArch["arm64"])
	}
	if byArch["s390x"] != dto.BuildFailed {
		t.Errorf("s390x state = %v, want BuildFailed", byArch["s390x"])
	}
}

// Case 7: pre-failed recipe with RetryCount>1 — executeAction must NOT eagerly
// call RetryBuild (defer to waitForBuilds), and waitForBuilds owns the retries.
func TestTrigger_Wait_RetryCount3_DefersFromActionRetryFailed(t *testing.T) {
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{"keystone": recipe},
		builds: map[string][]dto.Build{
			"/recipe/keystone": {
				{Recipe: "keystone", State: dto.BuildFailed, Arch: "amd64", SelfLink: "/build/1", CanRetry: true},
			},
		},
	}
	// Fail forever so we can count retry calls exactly.
	builder.retryHook = func(string) {}
	svc := newRetryService(t, builder)

	_, err := svc.Trigger(context.Background(), "sunbeam", nil, TriggerOpts{
		Wait:       true,
		Timeout:    5 * time.Second,
		RetryCount: 3,
	})
	if err != nil {
		t.Fatalf("Trigger() error: %v", err)
	}

	// With the unified retry path, exactly 2 retries (attempts 2 and 3) are
	// issued — NOT 3 (which would indicate the legacy eager retry + the new
	// loop both fired). This is the key correctness guarantee from the
	// "unify the retry path" fix.
	if got := len(builder.retried); got != 2 {
		t.Fatalf("expected exactly 2 retries (unified path), got %d: %v", got, builder.retried)
	}
}

// Case 8: legacy ActionRetryFailed preserved when RetryCount<=1.
func TestTrigger_Wait_RetryCount1_LegacyEagerRetry(t *testing.T) {
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{"keystone": recipe},
		builds: map[string][]dto.Build{
			"/recipe/keystone": {
				{Recipe: "keystone", State: dto.BuildFailed, Arch: "amd64", SelfLink: "/build/1", CanRetry: true},
			},
		},
	}
	builder.retryHook = func(string) {} // stays failed
	svc := newRetryService(t, builder)

	_, err := svc.Trigger(context.Background(), "sunbeam", nil, TriggerOpts{
		Wait:       true,
		Timeout:    5 * time.Second,
		RetryCount: 1, // legacy
	})
	if err != nil {
		t.Fatalf("Trigger() error: %v", err)
	}

	// Legacy path fires exactly once in executeAction; waitForBuilds does not add retries.
	if got := len(builder.retried); got != 1 {
		t.Fatalf("expected exactly 1 legacy retry, got %d: %v", got, builder.retried)
	}
}

// Case 9: context cancellation immediately after retry POST must return a
// snapshot that reflects the post-retry state, not the stale failure state.
func TestTrigger_Wait_RetryCount3_CancelAfterRetryReturnsFreshSnapshot(t *testing.T) {
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{"keystone": recipe},
		builds: map[string][]dto.Build{
			"/recipe/keystone": {
				{Recipe: "keystone", State: dto.BuildFailed, Arch: "amd64", SelfLink: "/build/1", CanRetry: true},
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Transition to Pending on retry AND cancel context so next iteration
	// exits. The post-retry refresh poll inside waitForBuilds should pick
	// up the Pending state before returning.
	builder.retryHook = func(selfLink string) {
		setState(builder, "/recipe/keystone", selfLink, dto.BuildPending)
		cancel()
	}
	svc := newRetryService(t, builder)

	result, _ := svc.Trigger(ctx, "sunbeam", nil, TriggerOpts{
		Wait:       true,
		Timeout:    5 * time.Second,
		RetryCount: 3,
	})

	// Service.Trigger swallows the wait error (logs Warn, returns success),
	// so we verify the captured snapshot instead of err.
	if len(result.RecipeResults) == 0 {
		t.Fatal("expected at least one recipe result")
	}
	rr := result.RecipeResults[0]
	if len(rr.Builds) != 1 {
		t.Fatalf("expected 1 build, got %d", len(rr.Builds))
	}
	// Key assertion: state is Pending (fresh snapshot), NOT Failed (stale).
	if rr.Builds[0].State != dto.BuildPending {
		t.Errorf("post-cancel build state = %v, want BuildPending (fresh post-retry snapshot)", rr.Builds[0].State)
	}
}

func TestTrigger_Wait_TimeoutIncludesActiveBuildDetails(t *testing.T) {
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{"keystone": recipe},
		builds: map[string][]dto.Build{
			"/recipe/keystone": {
				{Recipe: "keystone", State: dto.BuildPending, Arch: "arm64", SelfLink: "/build/arm64", WebLink: "https://launchpad.test/build/arm64"},
				{Recipe: "keystone", State: dto.BuildBuilding, Arch: "s390x", SelfLink: "/build/s390x", WebLink: "https://launchpad.test/build/s390x"},
				{Recipe: "keystone", State: dto.BuildSucceeded, Arch: "amd64", SelfLink: "/build/amd64", WebLink: "https://launchpad.test/build/amd64"},
			},
		},
	}
	svc := newRetryService(t, builder)

	result, err := svc.Trigger(context.Background(), "sunbeam", nil, TriggerOpts{
		Wait:       true,
		Timeout:    1 * time.Millisecond,
		RetryCount: 1,
	})
	if err != nil {
		t.Fatalf("Trigger() error: %v", err)
	}
	if result.WaitTimeout == nil {
		t.Fatal("WaitTimeout = nil, want timeout details")
	}
	if result.WaitTimeout.Timeout != "1ms" {
		t.Fatalf("WaitTimeout.Timeout = %q, want 1ms", result.WaitTimeout.Timeout)
	}
	if len(result.WaitTimeout.Builds) != 2 {
		t.Fatalf("WaitTimeout.Builds len = %d, want 2: %+v", len(result.WaitTimeout.Builds), result.WaitTimeout.Builds)
	}
	got := map[string]dto.BuildWaitTimeoutBuild{}
	for _, b := range result.WaitTimeout.Builds {
		got[b.Arch] = b
	}
	for arch, state := range map[string]string{"arm64": "pending", "s390x": "building"} {
		b, ok := got[arch]
		if !ok {
			t.Fatalf("missing timeout build for arch %s: %+v", arch, result.WaitTimeout.Builds)
		}
		if b.Recipe != "keystone" || b.State != state || b.URL == "" || b.SelfLink == "" {
			t.Fatalf("timeout build for %s = %+v, want recipe/state/url/self_link", arch, b)
		}
	}
	if _, ok := got["amd64"]; ok {
		t.Fatalf("succeeded build included in timeout details: %+v", result.WaitTimeout.Builds)
	}
}

// Case 10: defense-in-depth — RetryCount>1 without Wait is silently coerced to 1.
func TestTrigger_RetryCountWithoutWait_IgnoredSilently(t *testing.T) {
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{"keystone": recipe},
		builds: map[string][]dto.Build{
			"/recipe/keystone": {
				{Recipe: "keystone", State: dto.BuildFailed, Arch: "amd64", SelfLink: "/build/1", CanRetry: true},
			},
		},
	}
	svc := newRetryService(t, builder)

	// Wait=false: the wait loop does not run, so RetryCount>1 must coerce to
	// legacy behavior (one retry from ActionRetryFailed).
	_, err := svc.Trigger(context.Background(), "sunbeam", nil, TriggerOpts{
		Wait:       false,
		RetryCount: 3,
	})
	if err != nil {
		t.Fatalf("Trigger() error: %v", err)
	}

	// Legacy ActionRetryFailed still fires once; no additional retries from
	// waitForBuilds because it's never called.
	if got := len(builder.retried); got != 1 {
		t.Fatalf("expected 1 retry (legacy, coerced from RetryCount>1 + !Wait), got %d: %v", got, builder.retried)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func recipeKeys(m map[string]*dto.Recipe) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func TestDownloadConcurrentTransfers(t *testing.T) {
	for _, cancelTransfers := range []bool{false, true} {
		name := "complete"
		if cancelTransfers {
			name = "cancel"
		}
		t.Run(name, func(t *testing.T) {
			started := make(chan struct{}, 8)
			release := make(chan struct{})
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				started <- struct{}{}
				select {
				case <-release:
					_, _ = w.Write([]byte("artifact"))
				case <-r.Context().Done():
				}
			}))
			defer ts.Close()
			defer close(release)
			builder := &mockRecipeBuilder{
				recipes:  make(map[string]*dto.Recipe),
				builds:   make(map[string][]dto.Build),
				fileURLs: make(map[string][]string),
			}
			for _, recipe := range []string{"first", "second"} {
				builder.recipes[recipe] = &dto.Recipe{Name: recipe, SelfLink: recipe}
				builder.builds[recipe] = []dto.Build{{State: dto.BuildSucceeded, SelfLink: recipe}}
				for i := range 4 {
					builder.fileURLs[recipe] = append(builder.fileURLs[recipe], fmt.Sprintf("%s/%s-%d.rock", ts.URL, recipe, i))
				}
			}
			svc := NewService(map[string]ProjectBuilder{
				"sunbeam": {Builder: builder, Artifacts: []string{"first", "second"}},
			}, nil, testLogger())
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			outputDir := t.TempDir()
			result := make(chan error, 1)
			go func() {
				result <- svc.Download(ctx, DownloadOpts{OutputDir: outputDir, RetryCount: 2})
			}()
			for range 4 {
				select {
				case <-started:
				case <-ctx.Done():
					t.Fatal("four transfers did not start concurrently")
				}
			}
			select {
			case <-started:
				t.Fatal("more than four transfers started before a slot was freed")
			case <-time.After(100 * time.Millisecond):
			}
			if cancelTransfers {
				cancel()
			} else {
				// Let both waves finish, keeping the deferred close for cleanup.
				for range 8 {
					select {
					case release <- struct{}{}:
					case <-ctx.Done():
						t.Fatal("transfers did not finish")
					}
				}
			}
			select {
			case err := <-result:
				if cancelTransfers {
					var downloadErr *DownloadError
					if !errors.As(err, &downloadErr) || len(downloadErr.Failures) != 8 {
						t.Fatalf("Download() = %v, want eight cancellation failures", err)
					}
					for _, failure := range downloadErr.Failures {
						if !errors.Is(failure.Err, context.Canceled) {
							t.Fatalf("failure = %v, want context.Canceled", failure)
						}
					}
				} else {
					if err != nil {
						t.Fatalf("Download() = %v", err)
					}
					for _, recipe := range []string{"first", "second"} {
						for i := range 4 {
							data, err := os.ReadFile(filepath.Join(outputDir, recipe, fmt.Sprintf("%s-%d.rock", recipe, i)))
							if err != nil || string(data) != "artifact" {
								t.Fatalf("downloaded artifact = %q, %v", data, err)
							}
						}
					}
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Download() did not return")
			}
		})
	}
}

func TestDownloadPreservesSameDestinationOrder(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path))
	}))
	defer ts.Close()
	builder := &mockRecipeBuilder{
		recipes:  map[string]*dto.Recipe{"keystone": {Name: "keystone", SelfLink: "keystone"}},
		builds:   map[string][]dto.Build{"keystone": {{State: dto.BuildSucceeded, SelfLink: "build"}}},
		fileURLs: map[string][]string{"build": {ts.URL + "/first/shared.rock", ts.URL + "/second/shared.rock"}},
	}
	svc := NewService(map[string]ProjectBuilder{
		"sunbeam": {Builder: builder, Artifacts: []string{"keystone"}},
	}, nil, testLogger())
	outputDir := t.TempDir()
	if err := svc.Download(context.Background(), DownloadOpts{OutputDir: outputDir}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(outputDir, "keystone", "shared.rock"))
	if err != nil || string(data) != "/second/shared.rock" {
		t.Fatalf("final artifact = %q, %v, want second download", data, err)
	}
}
