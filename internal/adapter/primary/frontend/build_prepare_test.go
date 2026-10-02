// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/app"
	"github.com/canonical/sunbeam-watchtower/internal/core/service/build"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

type fakeGitClient struct {
	headSHA       string
	currentBranch string

	// Call counts and last args for the new prepared-worktree flow.
	createWtCalls    int
	forceAddAllCalls int
	commitCalls      int
	cleanupCalls     int

	// tempWorktreeDir is returned by CreateDetachedWorktree when set;
	// otherwise a real os.MkdirTemp result is allocated on first call so
	// callers can stat/read it.
	tempWorktreeDir string

	// Captured args.
	lastCreateWtBranch string
	lastCreateWtSHA    string
	lastCommitPath     string
}

func (f *fakeGitClient) IsRepo(string) bool                         { return true }
func (f *fakeGitClient) HeadSHA(string) (string, error)             { return f.headSHA, nil }
func (f *fakeGitClient) HasUncommittedChanges(string) (bool, error) { return false, nil }
func (f *fakeGitClient) Push(string, string, string, string, bool) error {
	return nil
}
func (f *fakeGitClient) AddRemote(string, string, string) error    { return nil }
func (f *fakeGitClient) RemoveRemote(string, string) error         { return nil }
func (f *fakeGitClient) CreateBranch(string, string, string) error { return nil }
func (f *fakeGitClient) CheckoutBranch(string, string) error       { return nil }
func (f *fakeGitClient) CurrentBranch(string) (string, error) {
	if f.currentBranch != "" {
		return f.currentBranch, nil
	}
	return "master", nil
}
func (f *fakeGitClient) DeleteLocalBranch(string, string) error { return nil }
func (f *fakeGitClient) AddAll(string) error                    { return nil }
func (f *fakeGitClient) Commit(path, _ string) error {
	f.commitCalls++
	f.lastCommitPath = path
	return nil
}
func (f *fakeGitClient) ResetHard(string, string) error { return nil }
func (f *fakeGitClient) CreateDetachedWorktree(_ context.Context, _, branch, sha string) (string, func(), error) {
	f.createWtCalls++
	f.lastCreateWtBranch = branch
	f.lastCreateWtSHA = sha
	if f.tempWorktreeDir == "" {
		d, err := os.MkdirTemp("", "watchtower-test-wt-*")
		if err != nil {
			return "", func() {}, err
		}
		f.tempWorktreeDir = d
	}
	return f.tempWorktreeDir, func() { f.cleanupCalls++ }, nil
}
func (f *fakeGitClient) ForceAddAll(_ context.Context, _ string) error {
	f.forceAddAllCalls++
	return nil
}

type fakeRepoManager struct {
	currentUser  string
	project      string
	repoSelfLink string
	gitSSHURL    string
	refSelfLink  string
	refErr       error
}

func (f *fakeRepoManager) GetCurrentUser(context.Context) (string, error) { return f.currentUser, nil }
func (f *fakeRepoManager) GetDefaultRepo(context.Context, string) (string, string, error) {
	return "", "", errors.New("not used")
}
func (f *fakeRepoManager) GetOrCreateProject(context.Context, string) (string, error) {
	return f.project, nil
}
func (f *fakeRepoManager) GetOrCreateRepo(context.Context, string, string, string) (string, string, error) {
	return f.repoSelfLink, f.gitSSHURL, nil
}
func (f *fakeRepoManager) GetGitRef(context.Context, string, string) (string, error) {
	if f.refErr != nil {
		return "", f.refErr
	}
	return f.refSelfLink, nil
}
func (f *fakeRepoManager) WaitForGitRef(context.Context, string, string, time.Duration) (string, error) {
	return f.refSelfLink, nil
}

func (f *fakeRepoManager) ListBranches(context.Context, string) ([]dto.BranchRef, error) {
	return nil, nil
}

func (f *fakeRepoManager) DeleteGitRef(context.Context, string) error {
	return nil
}

type fakeStrategy struct{}

func (f *fakeStrategy) ArtifactType() dto.ArtifactType                          { return dto.ArtifactRock }
func (f *fakeStrategy) MetadataFileName() string                                { return "rockcraft.yaml" }
func (f *fakeStrategy) BuildPath(name string) string                            { return "rocks/" + name }
func (f *fakeStrategy) ParsePlatforms([]byte) ([]string, error)                 { return []string{"amd64"}, nil }
func (f *fakeStrategy) DiscoverRecipes(string) ([]build.DiscoveredRecipe, error) {
	return []build.DiscoveredRecipe{{Name: "keystone", RelPath: "rocks/keystone"}}, nil
}
func (f *fakeStrategy) OfficialRecipeName(name, series, devFocus string) string { return name }
func (f *fakeStrategy) BranchForSeries(series, devFocus, defaultBranch string) string {
	return defaultBranch
}
func (f *fakeStrategy) TempRecipeName(name, sha, prefix string) string {
	return prefix + "-" + sha[:8] + "-" + name
}

type fakeCommandRunner struct {
	runFn func(ctx context.Context, dir string, command string) error
}

func (f *fakeCommandRunner) Run(ctx context.Context, dir string, command string) error {
	if f.runFn != nil {
		return f.runFn(ctx, dir, command)
	}
	return nil
}

func TestLocalBuildPreparerPrepareTrigger(t *testing.T) {
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

	got, err := preparer.PrepareTrigger(context.Background(), PreparedBuildTriggerRequest{
		Project: "demo",
		Prefix:  "tmp-build",
	}, filepath.Join(t.TempDir(), "demo"))
	if err != nil {
		t.Fatalf("PrepareTrigger() error = %v", err)
	}

	if got.Owner != "lp-user" || got.Prepared == nil || got.Prepared.TargetRef != "lp-project" || got.Prepared.RepositoryRef == "" {
		t.Fatalf("unexpected prepared trigger: %+v", got)
	}
	if len(got.Artifacts) != 1 || got.Artifacts[0] != "tmp-build-01234567-keystone" {
		t.Fatalf("Artifacts = %v", got.Artifacts)
	}
	if got.Prepared.Recipes[got.Artifacts[0]].BuildPath != "rocks/keystone" {
		t.Fatalf("Recipes = %+v", got.Prepared.Recipes)
	}
	if got.Prepared.Recipes[got.Artifacts[0]].SourceRef == "" {
		t.Fatalf("Recipes = %+v", got.Prepared.Recipes)
	}
}

// nestedFakeStrategy reports a DiscoveredRecipe whose RelPath differs from the
// shallow BuildPath(name) — e.g. a monorepo charm nested at
// charms/storage/bar — so we can prove the PreparedBuildRecipe.BuildPath
// follows the discovered RelPath rather than the flat fallback.
type nestedFakeStrategy struct {
	fakeStrategy
}

func (n *nestedFakeStrategy) DiscoverRecipes(string) ([]build.DiscoveredRecipe, error) {
	return []build.DiscoveredRecipe{{Name: "bar", RelPath: "charms/storage/bar"}}, nil
}

func (n *nestedFakeStrategy) BuildPath(name string) string { return "charms/" + name }

func TestLocalBuildPreparerPrepareTriggerPreservesNestedRelPath(t *testing.T) {
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
				Strategy: &nestedFakeStrategy{},
			},
		},
		nil,
	)

	got, err := preparer.PrepareTrigger(context.Background(), PreparedBuildTriggerRequest{
		Project: "demo",
		Prefix:  "tmp-build",
	}, filepath.Join(t.TempDir(), "demo"))
	if err != nil {
		t.Fatalf("PrepareTrigger() error = %v", err)
	}

	if len(got.Artifacts) != 1 {
		t.Fatalf("Artifacts = %v, want one entry", got.Artifacts)
	}
	tempName := got.Artifacts[0]
	if got.Prepared == nil {
		t.Fatalf("Prepared is nil")
	}
	recipe := got.Prepared.Recipes[tempName]
	if recipe.BuildPath != "charms/storage/bar" {
		t.Fatalf("BuildPath = %q, want %q (discovered RelPath must win over flat BuildPath)",
			recipe.BuildPath, "charms/storage/bar")
	}
}

func TestLocalBuildPreparerPrepareTriggerExplicitArtifactsResolvesNestedRelPath(t *testing.T) {
	// Simulates `build trigger demo bar` on a monorepo where bar lives at
	// charms/storage/bar/charmcraft.yaml. The user passes the charm name
	// positionally; PrepareTrigger must still run discovery and use the
	// discovered RelPath instead of collapsing to the flat charms/bar.
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
				Strategy: &nestedFakeStrategy{},
			},
		},
		nil,
	)

	got, err := preparer.PrepareTrigger(context.Background(), PreparedBuildTriggerRequest{
		Project:   "demo",
		Prefix:    "tmp-build",
		Artifacts: []string{"bar"},
	}, filepath.Join(t.TempDir(), "demo"))
	if err != nil {
		t.Fatalf("PrepareTrigger() error = %v", err)
	}

	if len(got.Artifacts) != 1 {
		t.Fatalf("Artifacts = %v, want one entry", got.Artifacts)
	}
	recipe := got.Prepared.Recipes[got.Artifacts[0]]
	if recipe.BuildPath != "charms/storage/bar" {
		t.Fatalf("BuildPath = %q, want %q (explicit --artifacts must still go through discovery)",
			recipe.BuildPath, "charms/storage/bar")
	}
}

func TestLocalBuildPreparerPrepareTriggerRejectsUnknownArtifact(t *testing.T) {
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
				Strategy: &fakeStrategy{}, // only discovers "keystone"
			},
		},
		nil,
	)

	_, err := preparer.PrepareTrigger(context.Background(), PreparedBuildTriggerRequest{
		Project:   "demo",
		Prefix:    "tmp-build",
		Artifacts: []string{"ghost"},
	}, filepath.Join(t.TempDir(), "demo"))
	if err == nil {
		t.Fatal("PrepareTrigger() error = nil, want error for unknown artifact")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("error %q should mention the unknown artifact %q", err, "ghost")
	}
}

func TestLocalBuildPreparerPrepareTriggerSkipsWhenBranchExists(t *testing.T) {
	pushed := false
	preparer := NewLocalBuildPreparer(
		&fakeGitClient{
			headSHA:       "0123456789abcdef0123456789abcdef01234567",
			currentBranch: "main",
		},
		&fakeRepoManager{
			currentUser:  "lp-user",
			project:      "lp-project",
			repoSelfLink: "https://api.launchpad.net/devel/~lp-user/lp-project/+git/demo",
			gitSSHURL:    "git+ssh://git.launchpad.net/~lp-user/lp-project/+git/demo",
			refSelfLink:  "https://api.launchpad.net/devel/~lp-user/lp-project/+git/demo/+ref/refs/heads/tmp-tmp-build-01234567",
			// refErr is nil — branch already exists on LP.
		},
		map[string]build.ProjectBuilder{
			"demo": {
				Project:  "demo",
				Strategy: &fakeStrategy{},
			},
		},
		nil,
	)

	// Override push to detect if it's called.
	origGit := preparer.gitClient
	preparer.gitClient = &pushTrackingGitClient{fakeGitClient: origGit.(*fakeGitClient), pushed: &pushed}

	got, err := preparer.PrepareTrigger(context.Background(), PreparedBuildTriggerRequest{
		Project: "demo",
		Prefix:  "tmp-build",
	}, filepath.Join(t.TempDir(), "demo"))
	if err != nil {
		t.Fatalf("PrepareTrigger() error = %v", err)
	}
	if pushed {
		t.Fatal("expected no push when branch already exists")
	}
	if got.Prepared == nil || got.Prepared.TargetRef != "lp-project" {
		t.Fatalf("unexpected prepared trigger: %+v", got)
	}
	if got.Prepared.Recipes[got.Artifacts[0]].SourceRef == "" {
		t.Fatalf("SourceRef should be populated from existing ref")
	}
}

// pushTrackingGitClient wraps fakeGitClient and records whether Push was called.
type pushTrackingGitClient struct {
	*fakeGitClient
	pushed *bool
}

func (p *pushTrackingGitClient) Push(path, remote, localRef, remoteRef string, force bool) error {
	*p.pushed = true
	return nil
}

func TestLocalBuildPreparerPrepareTriggerWithPrepareCommand(t *testing.T) {
	var ranCommand, ranCwd string
	runner := &fakeCommandRunner{
		runFn: func(_ context.Context, dir string, command string) error {
			ranCommand = command
			ranCwd = dir
			return nil
		},
	}

	gitFake := &fakeGitClient{headSHA: "0123456789abcdef0123456789abcdef01234567", currentBranch: "main"}
	preparer := NewLocalBuildPreparer(
		gitFake,
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
				Project:        "demo",
				Strategy:       &fakeStrategy{},
				PrepareCommand: "./repository.py prepare",
			},
		},
		runner,
	)

	got, err := preparer.PrepareTrigger(context.Background(), PreparedBuildTriggerRequest{
		Project: "demo",
		Prefix:  "tmp-build",
	}, filepath.Join(t.TempDir(), "demo"))
	if err != nil {
		t.Fatalf("PrepareTrigger() error = %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(gitFake.tempWorktreeDir) })

	if ranCommand != "./repository.py prepare" {
		t.Fatalf("expected prepare command %q, got %q", "./repository.py prepare", ranCommand)
	}
	if ranCwd != gitFake.tempWorktreeDir {
		t.Fatalf("prepare command cwd = %q, want tempdir %q", ranCwd, gitFake.tempWorktreeDir)
	}
	if gitFake.createWtCalls != 1 {
		t.Fatalf("CreateDetachedWorktree called %d times, want 1", gitFake.createWtCalls)
	}
	if gitFake.forceAddAllCalls != 1 {
		t.Fatalf("ForceAddAll called %d times, want 1", gitFake.forceAddAllCalls)
	}
	if gitFake.commitCalls != 1 {
		t.Fatalf("Commit called %d times, want 1", gitFake.commitCalls)
	}
	if gitFake.lastCommitPath != gitFake.tempWorktreeDir {
		t.Fatalf("Commit called on %q, want temp worktree %q", gitFake.lastCommitPath, gitFake.tempWorktreeDir)
	}
	if gitFake.cleanupCalls != 1 {
		t.Fatalf("cleanup called %d times, want 1", gitFake.cleanupCalls)
	}
	if got.Prepared == nil || got.Prepared.TargetRef != "lp-project" {
		t.Fatalf("unexpected prepared trigger: %+v", got)
	}
}

func TestLocalBuildPreparerPrepareTriggerRequiresAuth(t *testing.T) {
	preparer := NewLocalBuildPreparer(&fakeGitClient{headSHA: "0123456789abcdef0123456789abcdef01234567"}, nil, nil, nil)

	_, err := preparer.PrepareTrigger(context.Background(), PreparedBuildTriggerRequest{
		Project: "demo",
		Prefix:  "tmp-build",
	}, filepath.Join(t.TempDir(), "demo"))
	if !errors.Is(err, app.ErrLaunchpadAuthRequired) {
		t.Fatalf("PrepareTrigger() error = %v, want %v", err, app.ErrLaunchpadAuthRequired)
	}
}

func TestLocalBuildPreparerPrepareListByPrefix(t *testing.T) {
	preparer := NewLocalBuildPreparer(
		nil,
		&fakeRepoManager{currentUser: "lp-user", project: "lp-project"},
		nil,
		nil,
	)

	got, err := preparer.PrepareListByPrefix(context.Background(), PreparedBuildListRequest{}, "tmp-build-01234567-")
	if err != nil {
		t.Fatalf("PrepareListByPrefix() error = %v", err)
	}
	if got.Owner != "lp-user" || got.TargetRef != "lp-project" || got.RecipePrefix != "tmp-build-01234567-" {
		t.Fatalf("unexpected list opts: %+v", got)
	}
}

func TestSnapProcessorsFromRepo(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "snap"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	yaml := []byte("name: foo\nplatforms:\n  amd64:\n  arm64:\n  s390x:\n")
	if err := os.WriteFile(filepath.Join(repo, "snap", "snapcraft.yaml"), yaml, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := snapProcessorsFromRepo(repo, build.DiscoveredRecipe{Name: "foo"}, &build.SnapStrategy{})
	if err != nil {
		t.Fatalf("snapProcessorsFromRepo() error = %v", err)
	}
	sort.Strings(got)
	want := []string{"amd64", "arm64", "s390x"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("processors = %v, want %v", got, want)
	}
}

func TestSnapProcessorsFromRepoMissing(t *testing.T) {
	repo := t.TempDir()
	got, err := snapProcessorsFromRepo(repo, build.DiscoveredRecipe{Name: "foo"}, &build.SnapStrategy{})
	if err != nil {
		t.Fatalf("snapProcessorsFromRepo() error = %v", err)
	}
	if got != nil {
		t.Fatalf("processors = %v, want nil", got)
	}
}

func TestLocalBuildPreparerPrepareTriggerPrepareSkipsPushWhenRefExists(t *testing.T) {
	// prepare_command is set, BUT the LP ref already exists. Prepare
	// still runs (so discovery has a prepared tree), push is skipped.
	var ranCommand string
	runner := &fakeCommandRunner{
		runFn: func(_ context.Context, _ string, command string) error {
			ranCommand = command
			return nil
		},
	}

	gitFake := &fakeGitClient{headSHA: "0123456789abcdef0123456789abcdef01234567", currentBranch: "main"}
	pushed := false
	tracked := &pushTrackingGitClient{fakeGitClient: gitFake, pushed: &pushed}

	preparer := NewLocalBuildPreparer(
		tracked,
		&fakeRepoManager{
			currentUser:  "lp-user",
			project:      "lp-project",
			repoSelfLink: "https://api.launchpad.net/devel/~lp-user/lp-project/+git/demo",
			gitSSHURL:    "git+ssh://git.launchpad.net/~lp-user/lp-project/+git/demo",
			refSelfLink:  "https://api.launchpad.net/devel/~lp-user/lp-project/+git/demo/+ref/refs/heads/tmp-tmp-build-01234567",
			// refErr nil → ref exists → push skipped.
		},
		map[string]build.ProjectBuilder{
			"demo": {
				Project:        "demo",
				Strategy:       &fakeStrategy{},
				PrepareCommand: "./prep.sh",
			},
		},
		runner,
	)

	_, err := preparer.PrepareTrigger(context.Background(), PreparedBuildTriggerRequest{
		Project: "demo",
		Prefix:  "tmp-build",
	}, filepath.Join(t.TempDir(), "demo"))
	if err != nil {
		t.Fatalf("PrepareTrigger: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(gitFake.tempWorktreeDir) })

	if ranCommand == "" {
		t.Fatal("prepare command should have run even when ref exists")
	}
	if gitFake.createWtCalls != 1 {
		t.Fatalf("CreateDetachedWorktree calls = %d, want 1", gitFake.createWtCalls)
	}
	if gitFake.forceAddAllCalls != 1 {
		t.Fatalf("ForceAddAll calls = %d, want 1", gitFake.forceAddAllCalls)
	}
	if gitFake.commitCalls != 1 {
		t.Fatalf("Commit calls = %d, want 1", gitFake.commitCalls)
	}
	if pushed {
		t.Fatal("Push should NOT have been called when LP ref exists")
	}
	if gitFake.cleanupCalls != 1 {
		t.Fatalf("cleanup calls = %d, want 1", gitFake.cleanupCalls)
	}
}

func TestLocalBuildPreparerPrepareTriggerNoPrepareCommandSkipsWorktree(t *testing.T) {
	// prepare_command unset: byte-for-byte today's flow. No worktree.
	gitFake := &fakeGitClient{headSHA: "0123456789abcdef0123456789abcdef01234567", currentBranch: "main"}

	preparer := NewLocalBuildPreparer(
		gitFake,
		&fakeRepoManager{
			currentUser:  "lp-user",
			project:      "lp-project",
			repoSelfLink: "https://api.launchpad.net/devel/~lp-user/lp-project/+git/demo",
			gitSSHURL:    "git+ssh://git.launchpad.net/~lp-user/lp-project/+git/demo",
			refSelfLink:  "https://api.launchpad.net/devel/~lp-user/lp-project/+git/demo/+ref/refs/heads/tmp-tmp-build-01234567",
			refErr:       fmt.Errorf("not found"),
		},
		map[string]build.ProjectBuilder{
			"demo": {Project: "demo", Strategy: &fakeStrategy{}}, // PrepareCommand intentionally empty
		},
		nil, // no cmdRunner — proves prepare path not entered
	)

	_, err := preparer.PrepareTrigger(context.Background(), PreparedBuildTriggerRequest{
		Project: "demo",
		Prefix:  "tmp-build",
	}, filepath.Join(t.TempDir(), "demo"))
	if err != nil {
		t.Fatalf("PrepareTrigger: %v", err)
	}

	if gitFake.createWtCalls != 0 {
		t.Fatalf("CreateDetachedWorktree should not be called without prepare_command; got %d", gitFake.createWtCalls)
	}
	if gitFake.forceAddAllCalls != 0 {
		t.Fatalf("ForceAddAll should not be called without prepare_command; got %d", gitFake.forceAddAllCalls)
	}
}

func TestLocalBuildPreparerPrepareTriggerCleanupOnPrepareFailure(t *testing.T) {
	// prepare_command fails — cleanup must still run.
	runner := &fakeCommandRunner{
		runFn: func(_ context.Context, _ string, _ string) error {
			return fmt.Errorf("boom")
		},
	}
	gitFake := &fakeGitClient{headSHA: "0123456789abcdef0123456789abcdef01234567", currentBranch: "main"}
	preparer := NewLocalBuildPreparer(
		gitFake,
		&fakeRepoManager{
			currentUser:  "lp-user",
			project:      "lp-project",
			repoSelfLink: "https://api.launchpad.net/devel/~lp-user/lp-project/+git/demo",
			gitSSHURL:    "git+ssh://git.launchpad.net/~lp-user/lp-project/+git/demo",
			refSelfLink:  "",
			refErr:       fmt.Errorf("not found"),
		},
		map[string]build.ProjectBuilder{
			"demo": {Project: "demo", Strategy: &fakeStrategy{}, PrepareCommand: "./prep.sh"},
		},
		runner,
	)

	_, err := preparer.PrepareTrigger(context.Background(), PreparedBuildTriggerRequest{
		Project: "demo",
		Prefix:  "tmp-build",
	}, filepath.Join(t.TempDir(), "demo"))
	if err == nil {
		t.Fatal("expected error from failing prepare command")
	}
	t.Cleanup(func() { os.RemoveAll(gitFake.tempWorktreeDir) })
	if gitFake.cleanupCalls != 1 {
		t.Fatalf("cleanup calls after failure = %d, want 1", gitFake.cleanupCalls)
	}
}
