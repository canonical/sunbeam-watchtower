# Build Prepare Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rework the build trigger/cleanup pipeline to support branch-based local preparation with optional prepare-command execution, remote branch reuse, and branch cleanup via LP API.

**Architecture:** The `GitClient` port gets extended with branch/commit operations (go-git). `RepoManager` gets `ListBranches` and `DeleteGitRef` for cleanup via LP API. `LocalBuildPreparer` is reworked to create named temp branches, optionally run a prepare command and commit, push to LP, and restore local state. A `CommandRunner` interface handles shell execution on the frontend side. `Service.Cleanup` is fixed to use prefix-based discovery and extended to delete remote branches.

**Tech Stack:** Go, go-git/v5, Launchpad REST API, os/exec

**Spec:** `docs/superpowers/specs/2026-03-26-build-prepare-pipeline-design.md`

---

### Task 1: Extend GitClient Interface

**Files:**
- Modify: `internal/core/port/git.go`

- [ ] **Step 1: Add new methods to GitClient interface**

```go
// GitClient handles local git operations.
type GitClient interface {
	IsRepo(path string) bool
	HeadSHA(path string) (string, error)
	HasUncommittedChanges(path string) (bool, error)
	Push(path, remote, localRef, remoteRef string, force bool) error
	AddRemote(path, name, url string) error
	RemoveRemote(path, name string) error

	// Branch operations
	CreateBranch(path, branchName, startPoint string) error
	CheckoutBranch(path, branchName string) error
	CurrentBranch(path string) (string, error)
	DeleteLocalBranch(path, branchName string) error

	// Staging and committing
	AddAll(path string) error
	Commit(path, message string) error

	// Reset
	ResetHard(path, ref string) error
}
```

Note: `RemoteRefExists` was dropped from the spec — the remote branch check uses `RepoManager.GetGitRef` (LP API) instead. `DeleteRemoteBranch` was dropped — cleanup uses LP API `DeleteGitRef`. Added `DeleteLocalBranch` for cleaning up the local temp branch after push.

- [ ] **Step 2: Verify compilation fails**

Run: `go build ./internal/core/port/...`
Expected: PASS (interface change alone compiles)

Run: `go build ./internal/adapter/secondary/git/...`
Expected: FAIL — `Client` no longer satisfies `port.GitClient`

- [ ] **Step 3: Commit**

```bash
git add internal/core/port/git.go
git commit -m "port: extend GitClient interface with branch, commit, and reset operations"
```

---

### Task 2: Implement GitClient Extensions (go-git)

**Files:**
- Modify: `internal/adapter/secondary/git/client.go`
- Modify: `internal/adapter/secondary/git/client_test.go`

- [ ] **Step 1: Write tests for CreateBranch, CheckoutBranch, CurrentBranch, DeleteLocalBranch**

In `client_test.go`, add:

```go
func TestCreateBranchAndCheckout(t *testing.T) {
	c := adapter.NewClient(nil)
	dir := initTestRepo(t)

	if err := c.CreateBranch(dir, "feature-x", "HEAD"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if err := c.CheckoutBranch(dir, "feature-x"); err != nil {
		t.Fatalf("CheckoutBranch: %v", err)
	}
	branch, err := c.CurrentBranch(dir)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if branch != "feature-x" {
		t.Errorf("CurrentBranch = %q, want %q", branch, "feature-x")
	}
}

func TestDeleteLocalBranch(t *testing.T) {
	c := adapter.NewClient(nil)
	dir := initTestRepo(t)

	if err := c.CreateBranch(dir, "to-delete", "HEAD"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	// Must not be on the branch to delete it.
	if err := c.DeleteLocalBranch(dir, "to-delete"); err != nil {
		t.Fatalf("DeleteLocalBranch: %v", err)
	}
	// Verify branch is gone by attempting checkout.
	if err := c.CheckoutBranch(dir, "to-delete"); err == nil {
		t.Error("expected error checking out deleted branch")
	}
}

func TestCurrentBranch(t *testing.T) {
	c := adapter.NewClient(nil)
	dir := initTestRepo(t)

	branch, err := c.CurrentBranch(dir)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	// initTestRepo creates on master branch
	if branch != "master" && branch != "main" {
		t.Errorf("CurrentBranch = %q, want master or main", branch)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapter/secondary/git/... -run TestCreateBranch -v`
Expected: FAIL — methods not yet implemented

- [ ] **Step 3: Implement CreateBranch, CheckoutBranch, CurrentBranch, DeleteLocalBranch**

In `client.go`, add:

```go
func (c *Client) CreateBranch(path, branchName, startPoint string) error {
	c.logger.Debug("creating branch", "path", path, "branch", branchName, "startPoint", startPoint)
	repo, err := gogit.PlainOpen(path)
	if err != nil {
		return fmt.Errorf("open repo %s: %w", path, err)
	}

	var hash plumbing.Hash
	if startPoint == "HEAD" {
		head, err := repo.Head()
		if err != nil {
			return fmt.Errorf("resolve HEAD for %s: %w", path, err)
		}
		hash = head.Hash()
	} else {
		h, err := repo.ResolveRevision(plumbing.Revision(startPoint))
		if err != nil {
			return fmt.Errorf("resolve %s for %s: %w", startPoint, path, err)
		}
		hash = *h
	}

	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(branchName), hash)
	return repo.Storer.SetReference(ref)
}

func (c *Client) CheckoutBranch(path, branchName string) error {
	c.logger.Debug("checking out branch", "path", path, "branch", branchName)
	repo, err := gogit.PlainOpen(path)
	if err != nil {
		return fmt.Errorf("open repo %s: %w", path, err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("get worktree for %s: %w", path, err)
	}
	return wt.Checkout(&gogit.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branchName),
	})
}

func (c *Client) CurrentBranch(path string) (string, error) {
	c.logger.Debug("getting current branch", "path", path)
	repo, err := gogit.PlainOpen(path)
	if err != nil {
		return "", fmt.Errorf("open repo %s: %w", path, err)
	}
	head, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("get HEAD for %s: %w", path, err)
	}
	if !head.Name().IsBranch() {
		return "", fmt.Errorf("HEAD is not on a branch in %s", path)
	}
	return head.Name().Short(), nil
}

func (c *Client) DeleteLocalBranch(path, branchName string) error {
	c.logger.Debug("deleting local branch", "path", path, "branch", branchName)
	repo, err := gogit.PlainOpen(path)
	if err != nil {
		return fmt.Errorf("open repo %s: %w", path, err)
	}
	return repo.Storer.RemoveReference(plumbing.NewBranchReferenceName(branchName))
}
```

- [ ] **Step 4: Run branch tests to verify they pass**

Run: `go test ./internal/adapter/secondary/git/... -run "TestCreateBranch|TestDeleteLocal|TestCurrentBranch" -v`
Expected: PASS

- [ ] **Step 5: Write tests for AddAll, Commit, ResetHard**

```go
func TestAddAllAndCommit(t *testing.T) {
	c := adapter.NewClient(nil)
	dir := initTestRepo(t)

	// Create a new file.
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("hello"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := c.AddAll(dir); err != nil {
		t.Fatalf("AddAll: %v", err)
	}
	if err := c.Commit(dir, "add new file"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Verify repo is clean after commit.
	dirty, err := c.HasUncommittedChanges(dir)
	if err != nil {
		t.Fatalf("HasUncommittedChanges: %v", err)
	}
	if dirty {
		t.Error("expected clean repo after commit")
	}

	// Verify HEAD changed.
	sha, err := c.HeadSHA(dir)
	if err != nil {
		t.Fatalf("HeadSHA: %v", err)
	}
	if len(sha) != 40 {
		t.Errorf("expected 40-char SHA after commit, got %q", sha)
	}
}

func TestResetHard(t *testing.T) {
	c := adapter.NewClient(nil)
	dir := initTestRepo(t)

	originalSHA, _ := c.HeadSHA(dir)

	// Create and commit a new file.
	os.WriteFile(filepath.Join(dir, "extra.txt"), []byte("extra"), 0644)
	c.AddAll(dir)
	c.Commit(dir, "extra commit")

	newSHA, _ := c.HeadSHA(dir)
	if newSHA == originalSHA {
		t.Fatal("expected different SHA after second commit")
	}

	// Reset back.
	if err := c.ResetHard(dir, originalSHA); err != nil {
		t.Fatalf("ResetHard: %v", err)
	}

	afterReset, _ := c.HeadSHA(dir)
	if afterReset != originalSHA {
		t.Errorf("after reset SHA = %q, want %q", afterReset, originalSHA)
	}
}
```

- [ ] **Step 6: Run tests to verify they fail**

Run: `go test ./internal/adapter/secondary/git/... -run "TestAddAll|TestResetHard" -v`
Expected: FAIL

- [ ] **Step 7: Implement AddAll, Commit, ResetHard**

In `client.go`, add:

```go
func (c *Client) AddAll(path string) error {
	c.logger.Debug("staging all changes", "path", path)
	repo, err := gogit.PlainOpen(path)
	if err != nil {
		return fmt.Errorf("open repo %s: %w", path, err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("get worktree for %s: %w", path, err)
	}
	if err := wt.AddWithOptions(&gogit.AddOptions{All: true}); err != nil {
		return fmt.Errorf("add all for %s: %w", path, err)
	}
	return nil
}

func (c *Client) Commit(path, message string) error {
	c.logger.Debug("committing", "path", path, "message", message)
	repo, err := gogit.PlainOpen(path)
	if err != nil {
		return fmt.Errorf("open repo %s: %w", path, err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("get worktree for %s: %w", path, err)
	}
	_, err = wt.Commit(message, &gogit.CommitOptions{})
	if err != nil {
		return fmt.Errorf("commit for %s: %w", path, err)
	}
	return nil
}

func (c *Client) ResetHard(path, ref string) error {
	c.logger.Debug("resetting hard", "path", path, "ref", ref)
	repo, err := gogit.PlainOpen(path)
	if err != nil {
		return fmt.Errorf("open repo %s: %w", path, err)
	}

	h, err := repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return fmt.Errorf("resolve %s for %s: %w", ref, path, err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("get worktree for %s: %w", path, err)
	}
	return wt.Reset(&gogit.ResetOptions{
		Commit: *h,
		Mode:   gogit.HardReset,
	})
}
```

- [ ] **Step 8: Run all git client tests**

Run: `go test ./internal/adapter/secondary/git/... -v`
Expected: ALL PASS

- [ ] **Step 9: Commit**

```bash
git add internal/adapter/secondary/git/client.go internal/adapter/secondary/git/client_test.go
git commit -m "feat(git): implement branch, commit, and reset operations via go-git"
```

---

### Task 3: Extend RepoManager Interface and LP Adapter

**Files:**
- Modify: `internal/core/port/build.go`
- Modify: `pkg/launchpad/v1/git.go`
- Modify: `internal/adapter/secondary/launchpad/repo_manager.go`
- Modify: `internal/adapter/secondary/launchpad/repo_manager_test.go`
- Modify: `internal/core/service/build/service_test.go` (update mockRepoManager)

- [ ] **Step 1: Add ListBranches and DeleteGitRef to port.RepoManager**

In `internal/core/port/build.go`:

```go
// RepoManager handles temporary git repo/branch lifecycle on LP.
type RepoManager interface {
	GetCurrentUser(ctx context.Context) (string, error)
	GetDefaultRepo(ctx context.Context, projectName string) (repoSelfLink string, defaultBranch string, err error)
	GetOrCreateProject(ctx context.Context, owner string) (projectName string, err error)
	GetOrCreateRepo(ctx context.Context, owner, project, repoName string) (repoSelfLink, gitSSHURL string, err error)
	GetGitRef(ctx context.Context, repoSelfLink, refPath string) (refSelfLink string, err error)
	WaitForGitRef(ctx context.Context, repoSelfLink, refPath string, timeout time.Duration) (refSelfLink string, err error)
	ListBranches(ctx context.Context, repoSelfLink string) ([]BranchRef, error)
	DeleteGitRef(ctx context.Context, refSelfLink string) error
}

// BranchRef is a minimal representation of a git branch reference.
type BranchRef struct {
	Path     string
	SelfLink string
}
```

- [ ] **Step 2: Add DeleteGitRef to LP client**

In `pkg/launchpad/v1/git.go`, add:

```go
// DeleteGitRef deletes a git ref by its self_link.
func (c *Client) DeleteGitRef(ctx context.Context, refSelfLink string) error {
	return c.Delete(ctx, refSelfLink)
}
```

- [ ] **Step 3: Implement ListBranches and DeleteGitRef in RepoManager**

In `internal/adapter/secondary/launchpad/repo_manager.go`, add:

```go
func (m *RepoManager) ListBranches(ctx context.Context, repoSelfLink string) ([]port.BranchRef, error) {
	refs, err := m.client.GetGitBranches(ctx, repoSelfLink)
	if err != nil {
		return nil, fmt.Errorf("listing branches for repo: %w", err)
	}
	branches := make([]port.BranchRef, len(refs))
	for i, ref := range refs {
		selfLink := ref.SelfLink
		if selfLink == "" {
			selfLink = repoSelfLink + "/+ref/" + ref.Path
		}
		branches[i] = port.BranchRef{
			Path:     ref.Path,
			SelfLink: selfLink,
		}
	}
	return branches, nil
}

func (m *RepoManager) DeleteGitRef(ctx context.Context, refSelfLink string) error {
	m.logger.Info("deleting git ref", "ref", refSelfLink)
	return m.client.DeleteGitRef(ctx, refSelfLink)
}
```

- [ ] **Step 4: Write tests for ListBranches and DeleteGitRef**

In `repo_manager_test.go`, add:

```go
func TestRepoManagerListBranches(t *testing.T) {
	transport := repoRoundTripFunc(func(req *http.Request) *http.Response {
		if strings.Contains(req.URL.Path, "/branches") {
			return launchpadResponse(200, `{"entries":[{"path":"refs/heads/main","self_link":"https://api.lp/ref/main"},{"path":"refs/heads/tmp-abc","self_link":""}],"total_size":2}`)
		}
		return launchpadResponse(404, `{}`)
	})
	cleanup := withLaunchpadTransport(t, transport)
	defer cleanup()

	mgr := newRepoManagerForTest()
	branches, err := mgr.ListBranches(context.Background(), "https://api.lp/repo")
	if err != nil {
		t.Fatalf("ListBranches() error = %v", err)
	}
	if len(branches) != 2 {
		t.Fatalf("expected 2 branches, got %d", len(branches))
	}
	if branches[0].Path != "refs/heads/main" {
		t.Errorf("branches[0].Path = %q", branches[0].Path)
	}
	// Branch without self_link should get constructed one.
	if branches[1].SelfLink != "https://api.lp/repo/+ref/refs/heads/tmp-abc" {
		t.Errorf("branches[1].SelfLink = %q", branches[1].SelfLink)
	}
}

func TestRepoManagerDeleteGitRef(t *testing.T) {
	var deletedPath string
	transport := repoRoundTripFunc(func(req *http.Request) *http.Response {
		if req.Method == http.MethodDelete {
			deletedPath = req.URL.Path
			return launchpadResponse(200, ``)
		}
		return launchpadResponse(404, `{}`)
	})
	cleanup := withLaunchpadTransport(t, transport)
	defer cleanup()

	mgr := newRepoManagerForTest()
	err := mgr.DeleteGitRef(context.Background(), "https://api.lp/repo/+ref/refs/heads/tmp-abc")
	if err != nil {
		t.Fatalf("DeleteGitRef() error = %v", err)
	}
	if !strings.Contains(deletedPath, "tmp-abc") {
		t.Errorf("expected delete path to contain tmp-abc, got %q", deletedPath)
	}
}
```

- [ ] **Step 5: Update mockRepoManager in service_test.go**

In `internal/core/service/build/service_test.go`, add to `mockRepoManager`:

```go
// Add fields:
	branches []port.BranchRef
	deleteErr error

// Add methods:
func (m *mockRepoManager) ListBranches(_ context.Context, _ string) ([]port.BranchRef, error) {
	return m.branches, nil
}

func (m *mockRepoManager) DeleteGitRef(_ context.Context, _ string) error {
	return m.deleteErr
}
```

Add import for `port` package at top of file:
```go
"github.com/canonical/sunbeam-watchtower/internal/core/port"
```

- [ ] **Step 6: Update fakeRepoManager in build_prepare_test.go**

In `internal/adapter/primary/frontend/build_prepare_test.go`, add to `fakeRepoManager`:

```go
func (f *fakeRepoManager) ListBranches(context.Context, string) ([]port.BranchRef, error) {
	return nil, nil
}
func (f *fakeRepoManager) DeleteGitRef(context.Context, string) error {
	return nil
}
```

Add import: `"github.com/canonical/sunbeam-watchtower/internal/core/port"`

- [ ] **Step 7: Run all tests to verify everything compiles and passes**

Run: `go test ./internal/core/port/... ./internal/adapter/secondary/launchpad/... ./internal/core/service/build/... ./internal/adapter/primary/frontend/... -v`
Expected: ALL PASS

- [ ] **Step 8: Commit**

```bash
git add internal/core/port/build.go pkg/launchpad/v1/git.go internal/adapter/secondary/launchpad/repo_manager.go internal/adapter/secondary/launchpad/repo_manager_test.go internal/core/service/build/service_test.go internal/adapter/primary/frontend/build_prepare_test.go
git commit -m "feat: add ListBranches and DeleteGitRef to RepoManager for branch cleanup"
```

---

### Task 4: Add CommandRunner Interface and Implementation

**Files:**
- Create: `internal/adapter/primary/frontend/command_runner.go`
- Create: `internal/adapter/primary/frontend/command_runner_test.go`

- [ ] **Step 1: Write test for CommandRunner**

```go
// command_runner_test.go
package frontend

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestShellCommandRunner_Run(t *testing.T) {
	runner := &ShellCommandRunner{}
	dir := t.TempDir()

	err := runner.Run(context.Background(), dir, "echo hello > output.txt")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "output.txt"))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if got := string(data); got != "hello\n" {
		t.Errorf("output = %q, want %q", got, "hello\n")
	}
}

func TestShellCommandRunner_RunFailure(t *testing.T) {
	runner := &ShellCommandRunner{}
	dir := t.TempDir()

	err := runner.Run(context.Background(), dir, "false")
	if err == nil {
		t.Fatal("expected error for failing command")
	}
}

func TestShellCommandRunner_RunContextCancel(t *testing.T) {
	runner := &ShellCommandRunner{}
	dir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := runner.Run(ctx, dir, "sleep 60")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapter/primary/frontend/... -run TestShellCommand -v`
Expected: FAIL — type not defined

- [ ] **Step 3: Implement CommandRunner**

```go
// command_runner.go
package frontend

import (
	"context"
	"fmt"
	"os/exec"
)

// CommandRunner executes shell commands in a directory.
type CommandRunner interface {
	Run(ctx context.Context, dir string, command string) error
}

// ShellCommandRunner implements CommandRunner using os/exec with sh -c.
type ShellCommandRunner struct{}

func (r *ShellCommandRunner) Run(ctx context.Context, dir string, command string) error {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command %q in %s failed: %w\noutput: %s", command, dir, err, output)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/adapter/primary/frontend/... -run TestShellCommand -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/primary/frontend/command_runner.go internal/adapter/primary/frontend/command_runner_test.go
git commit -m "feat: add CommandRunner interface for shell command execution in build prepare"
```

---

### Task 5: Add PrepareCommand to ProjectBuilder and Wire Config

**Files:**
- Modify: `internal/core/service/build/project_builder.go`
- Modify: `internal/app/build_factories.go`

- [ ] **Step 1: Add PrepareCommand field to ProjectBuilder**

In `project_builder.go`:

```go
// ProjectBuilder groups a RecipeBuilder with its project-level metadata.
type ProjectBuilder struct {
	Builder             port.RecipeBuilder
	Owner               string
	Project             string // code project name (e.g. github repo name)
	LPProject           string // LP project for recipes (may differ from code project)
	Artifacts           []string
	Series              []string
	DevFocus            string
	OfficialCodehosting bool
	Strategy            ArtifactStrategy
	PrepareCommand      string // optional shell command to run before committing
}
```

- [ ] **Step 2: Wire PrepareCommand from config in build_factories.go**

In `buildRecipeBuildersFromConfig`, inside the loop, add `PrepareCommand` extraction alongside other build config fields:

Replace the current block:
```go
		var owner string
		var artifacts []string
		var lpProject string
		var officialCodehosting bool
		if proj.Build != nil {
			owner = proj.Build.Owner
			artifacts = proj.Build.Artifacts
			lpProject = proj.Build.LPProject
			officialCodehosting = proj.Build.OfficialCodehosting
		}
```

With:
```go
		var owner string
		var artifacts []string
		var lpProject string
		var officialCodehosting bool
		var prepareCommand string
		if proj.Build != nil {
			owner = proj.Build.Owner
			artifacts = proj.Build.Artifacts
			lpProject = proj.Build.LPProject
			officialCodehosting = proj.Build.OfficialCodehosting
			prepareCommand = proj.Build.PrepareCommand
		}
```

And add to the `ProjectBuilder` struct literal:
```go
		result[proj.Name] = build.ProjectBuilder{
			// ... existing fields ...
			PrepareCommand:      prepareCommand,
		}
```

- [ ] **Step 3: Verify compilation**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/core/service/build/project_builder.go internal/app/build_factories.go
git commit -m "feat: wire PrepareCommand from config to ProjectBuilder"
```

---

### Task 6: Rework LocalBuildPreparer.PrepareTrigger

**Files:**
- Modify: `internal/adapter/primary/frontend/build_prepare.go`
- Modify: `internal/adapter/primary/frontend/build_prepare_test.go`
- Modify: `internal/adapter/primary/frontend/builds_from_app.go`

- [ ] **Step 1: Update fakeGitClient to satisfy new interface**

In `build_prepare_test.go`, update `fakeGitClient`:

```go
type fakeGitClient struct {
	headSHA       string
	currentBranch string
	pushErr       error
}

func (f *fakeGitClient) IsRepo(string) bool                                  { return true }
func (f *fakeGitClient) HeadSHA(string) (string, error)                      { return f.headSHA, nil }
func (f *fakeGitClient) HasUncommittedChanges(string) (bool, error)          { return false, nil }
func (f *fakeGitClient) Push(string, string, string, string, bool) error     { return f.pushErr }
func (f *fakeGitClient) AddRemote(string, string, string) error              { return nil }
func (f *fakeGitClient) RemoveRemote(string, string) error                   { return nil }
func (f *fakeGitClient) CreateBranch(string, string, string) error           { return nil }
func (f *fakeGitClient) CheckoutBranch(string, string) error                 { return nil }
func (f *fakeGitClient) CurrentBranch(string) (string, error)                { return f.currentBranch, nil }
func (f *fakeGitClient) DeleteLocalBranch(string, string) error              { return nil }
func (f *fakeGitClient) AddAll(string) error                                 { return nil }
func (f *fakeGitClient) Commit(string, string) error                         { return nil }
func (f *fakeGitClient) ResetHard(string, string) error                      { return nil }
```

- [ ] **Step 2: Update fakeRepoManager to support ref-not-found for new flow**

```go
type fakeRepoManager struct {
	currentUser  string
	project      string
	repoSelfLink string
	gitSSHURL    string
	refSelfLink  string
	refErr       error // set to simulate ref-not-found
}

func (f *fakeRepoManager) GetGitRef(context.Context, string, string) (string, error) {
	if f.refErr != nil {
		return "", f.refErr
	}
	return f.refSelfLink, nil
}
```

Keep existing methods unchanged, add the two new ones from Task 3 Step 6.

- [ ] **Step 3: Update LocalBuildPreparer to accept CommandRunner**

In `build_prepare.go`:

```go
// LocalBuildPreparer handles frontend-side local preparation for split build workflows.
type LocalBuildPreparer struct {
	gitClient   port.GitClient
	repoManager port.RepoManager
	builders    map[string]build.ProjectBuilder
	cmdRunner   CommandRunner
}

// NewLocalBuildPreparer creates a reusable local build preparer.
func NewLocalBuildPreparer(
	gitClient port.GitClient,
	repoManager port.RepoManager,
	builders map[string]build.ProjectBuilder,
	cmdRunner CommandRunner,
) *LocalBuildPreparer {
	return &LocalBuildPreparer{
		gitClient:   gitClient,
		repoManager: repoManager,
		builders:    builders,
		cmdRunner:   cmdRunner,
	}
}
```

- [ ] **Step 4: Update builds_from_app.go to pass CommandRunner**

In `builds_from_app.go`:

```go
func NewLocalBuildPreparerFromApp(application *app.App) (*LocalBuildPreparer, error) {
	repoMgr, err := application.BuildRepoManager()
	if err != nil {
		return nil, fmt.Errorf("init repo manager: %w", err)
	}
	if repoMgr == nil {
		return nil, app.ErrLaunchpadAuthRequired
	}

	builders, err := application.BuildRecipeBuilders()
	if err != nil {
		return nil, fmt.Errorf("init recipe builders: %w", err)
	}

	return NewLocalBuildPreparer(application.GitClient(), repoMgr, builders, &ShellCommandRunner{}), nil
}
```

- [ ] **Step 5: Rewrite PrepareTrigger with branch-based flow**

Replace the entire `PrepareTrigger` method in `build_prepare.go`:

```go
// PrepareTrigger resolves local git and Launchpad state and returns a prepared build-trigger request.
func (p *LocalBuildPreparer) PrepareTrigger(
	ctx context.Context,
	req PreparedBuildTriggerRequest,
	localPath string,
) (PreparedBuildTriggerRequest, error) {
	if p.repoManager == nil {
		return req, app.ErrLaunchpadAuthRequired
	}
	pb, ok := p.builders[req.Project]
	if !ok {
		return req, fmt.Errorf("unknown project %q", req.Project)
	}

	// 1. Resolve HEAD SHA.
	sha, err := p.gitClient.HeadSHA(localPath)
	if err != nil {
		return req, fmt.Errorf("resolve HEAD SHA: %w", err)
	}
	shortSHA := sha
	if len(shortSHA) > 8 {
		shortSHA = shortSHA[:8]
	}

	// 2. Resolve LP owner.
	lpOwner := req.Owner
	if lpOwner == "" {
		lpOwner, err = p.repoManager.GetCurrentUser(ctx)
		if err != nil {
			return req, fmt.Errorf("get current LP user: %w", err)
		}
	}
	req.Owner = lpOwner

	// 3. Get or create personal LP project and repo.
	lpProject, err := p.repoManager.GetOrCreateProject(ctx, lpOwner)
	if err != nil {
		return req, fmt.Errorf("get/create LP project: %w", err)
	}
	repoSelfLink, gitSSHURL, err := p.repoManager.GetOrCreateRepo(ctx, lpOwner, lpProject, req.Project)
	if err != nil {
		return req, fmt.Errorf("get/create LP repo: %w", err)
	}

	// 4. Build branch name.
	branchName := "tmp-" + req.Prefix + "-" + shortSHA
	refPath := "refs/heads/" + branchName

	// 5. Check if branch already exists on LP (cache key).
	refLink, err := p.repoManager.GetGitRef(ctx, repoSelfLink, refPath)
	if err != nil {
		// Branch does not exist — prepare and push.
		if err := p.prepareAndPush(ctx, localPath, gitSSHURL, lpOwner, branchName, sha, pb.PrepareCommand); err != nil {
			return req, err
		}

		// Wait for LP to see the ref.
		refLink, err = p.repoManager.WaitForGitRef(ctx, repoSelfLink, refPath, 2*time.Minute)
		if err != nil {
			return req, fmt.Errorf("wait for git ref: %w", err)
		}
	}

	// 6. Discover artifacts from local clone.
	artifactNames := req.Artifacts
	if len(artifactNames) == 0 {
		artifactNames, err = pb.Strategy.DiscoverRecipes(localPath)
		if err != nil {
			return req, fmt.Errorf("discover artifacts: %w", err)
		}
	}

	// 7. Build PreparedBuildSource.
	tempNames := make([]string, 0, len(artifactNames))
	recipes := make(map[string]dto.PreparedBuildRecipe, len(artifactNames))
	for _, name := range artifactNames {
		tempName := pb.Strategy.TempRecipeName(name, sha, req.Prefix)
		tempNames = append(tempNames, tempName)
		recipes[tempName] = dto.PreparedBuildRecipe{
			SourceRef: refLink,
			BuildPath: pb.Strategy.BuildPath(name),
		}
	}
	req.Artifacts = tempNames
	req.Prepared = &dto.PreparedBuildSource{
		Backend:       dto.PreparedBuildBackendLaunchpad,
		TargetRef:     lpProject,
		RepositoryRef: repoSelfLink,
		Recipes:       recipes,
	}

	return req, nil
}

// prepareAndPush creates a temp branch, optionally runs the prepare command, and pushes to LP.
func (p *LocalBuildPreparer) prepareAndPush(
	ctx context.Context,
	localPath, gitSSHURL, lpOwner, branchName, sha, prepareCommand string,
) error {
	// Save current branch for later restore.
	originalBranch, err := p.gitClient.CurrentBranch(localPath)
	if err != nil {
		return fmt.Errorf("get current branch: %w", err)
	}

	// Create and checkout temp branch.
	if err := p.gitClient.CreateBranch(localPath, branchName, "HEAD"); err != nil {
		return fmt.Errorf("create branch %s: %w", branchName, err)
	}
	if err := p.gitClient.CheckoutBranch(localPath, branchName); err != nil {
		// Clean up the branch we just created.
		_ = p.gitClient.DeleteLocalBranch(localPath, branchName)
		return fmt.Errorf("checkout branch %s: %w", branchName, err)
	}

	// Ensure we restore local state on any exit path.
	defer func() {
		_ = p.gitClient.CheckoutBranch(localPath, originalBranch)
		_ = p.gitClient.DeleteLocalBranch(localPath, branchName)
	}()

	// Optionally run prepare command.
	if prepareCommand != "" {
		if p.cmdRunner == nil {
			return fmt.Errorf("prepare_command is set but no command runner is configured")
		}
		if err := p.cmdRunner.Run(ctx, localPath, prepareCommand); err != nil {
			return fmt.Errorf("prepare command: %w", err)
		}
		if err := p.gitClient.AddAll(localPath); err != nil {
			return fmt.Errorf("stage prepared changes: %w", err)
		}
		shortSHA := sha
		if len(shortSHA) > 8 {
			shortSHA = shortSHA[:8]
		}
		if err := p.gitClient.Commit(localPath, "watchtower: prepare "+shortSHA); err != nil {
			return fmt.Errorf("commit prepared changes: %w", err)
		}
	}

	// Push to LP.
	if err := pushToLaunchpad(p.gitClient, localPath, gitSSHURL, lpOwner, branchName); err != nil {
		return fmt.Errorf("push to LP: %w", err)
	}

	return nil
}
```

- [ ] **Step 6: Rewrite pushToLaunchpad to push a single named branch**

Replace the existing `pushToLaunchpad` function:

```go
func pushToLaunchpad(gitClient port.GitClient, localPath, gitSSHURL, lpOwner, branchName string) error {
	sshURL := strings.Replace(gitSSHURL, "git+ssh://", "ssh://", 1)
	if !strings.Contains(sshURL, "@") {
		sshURL = strings.Replace(sshURL, "ssh://", "ssh://"+lpOwner+"@", 1)
	}

	const remoteName = "watchtower-tmp"
	_ = gitClient.RemoveRemote(localPath, remoteName)
	if err := gitClient.AddRemote(localPath, remoteName, sshURL); err != nil {
		return fmt.Errorf("add remote: %w", err)
	}
	defer func() { _ = gitClient.RemoveRemote(localPath, remoteName) }()

	refSpec := "refs/heads/" + branchName + ":refs/heads/" + branchName
	if err := gitClient.Push(localPath, remoteName, "refs/heads/"+branchName, "refs/heads/"+branchName, true); err != nil {
		return fmt.Errorf("push branch %s: %w", branchName, err)
	}

	return nil
}
```

- [ ] **Step 7: Update tests for new PrepareTrigger flow**

Update `TestLocalBuildPreparerPrepareTrigger` in `build_prepare_test.go`:

```go
func TestLocalBuildPreparerPrepareTrigger(t *testing.T) {
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
			refErr:       fmt.Errorf("not found"), // simulate branch not existing
		},
		map[string]build.ProjectBuilder{
			"demo": {
				Project:  "demo",
				Strategy: &fakeStrategy{},
			},
		},
		nil, // no CommandRunner needed — no prepare_command
	)

	got, err := preparer.PrepareTrigger(context.Background(), PreparedBuildTriggerRequest{
		Project: "demo",
		Prefix:  "tmp-build",
	}, "/tmp/demo")
	if err != nil {
		t.Fatalf("PrepareTrigger() error = %v", err)
	}

	if got.Owner != "lp-user" || got.Prepared == nil || got.Prepared.TargetRef != "lp-project" || got.Prepared.RepositoryRef == "" {
		t.Fatalf("unexpected prepared trigger: %+v", got)
	}
	if len(got.Artifacts) != 1 || got.Artifacts[0] != "tmp-build-01234567-keystone" {
		t.Fatalf("Artifacts = %v", got.Artifacts)
	}
}

func TestLocalBuildPreparerPrepareTriggerSkipsWhenBranchExists(t *testing.T) {
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
			// refErr is nil — branch already exists
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
	}, "/tmp/demo")
	if err != nil {
		t.Fatalf("PrepareTrigger() error = %v", err)
	}

	// Should still produce valid output — just skipped the push.
	if got.Prepared == nil || len(got.Artifacts) != 1 {
		t.Fatalf("expected valid prepared output, got: %+v", got)
	}
}

func TestLocalBuildPreparerPrepareTriggerWithPrepareCommand(t *testing.T) {
	var ranCommand string
	runner := &fakeCommandRunner{runFn: func(_ context.Context, _ string, cmd string) error {
		ranCommand = cmd
		return nil
	}}

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

	_, err := preparer.PrepareTrigger(context.Background(), PreparedBuildTriggerRequest{
		Project: "demo",
		Prefix:  "tmp-build",
	}, "/tmp/demo")
	if err != nil {
		t.Fatalf("PrepareTrigger() error = %v", err)
	}
	if ranCommand != "./repository.py prepare" {
		t.Errorf("ran command = %q, want %q", ranCommand, "./repository.py prepare")
	}
}
```

Add the `fakeCommandRunner` helper:

```go
type fakeCommandRunner struct {
	runFn func(ctx context.Context, dir string, command string) error
}

func (f *fakeCommandRunner) Run(ctx context.Context, dir string, command string) error {
	if f.runFn != nil {
		return f.runFn(ctx, dir, command)
	}
	return nil
}
```

Add `"fmt"` to imports.

- [ ] **Step 8: Update fakeRepoManager to use refErr**

The `fakeRepoManager.GetGitRef` and `WaitForGitRef` methods need to check `refErr`:

```go
func (f *fakeRepoManager) GetGitRef(context.Context, string, string) (string, error) {
	if f.refErr != nil {
		return "", f.refErr
	}
	return f.refSelfLink, nil
}
func (f *fakeRepoManager) WaitForGitRef(context.Context, string, string, time.Duration) (string, error) {
	return f.refSelfLink, nil
}
```

Note: `WaitForGitRef` always succeeds (it's called after push, so the ref exists by then). Only `GetGitRef` checks `refErr` (used for the existence check).

- [ ] **Step 9: Run all frontend tests**

Run: `go test ./internal/adapter/primary/frontend/... -v`
Expected: ALL PASS

- [ ] **Step 10: Commit**

```bash
git add internal/adapter/primary/frontend/build_prepare.go internal/adapter/primary/frontend/build_prepare_test.go internal/adapter/primary/frontend/builds_from_app.go
git commit -m "feat: rework LocalBuildPreparer with branch-based prepare flow and CommandRunner"
```

---

### Task 7: Fix Service.Cleanup and Add Branch Deletion

**Files:**
- Modify: `internal/core/service/build/service.go`
- Modify: `internal/core/service/build/service_test.go`

- [ ] **Step 1: Write test for prefix-based cleanup**

In `service_test.go`, add:

```go
func TestCleanup_PrefixDiscovery(t *testing.T) {
	builder := &mockRecipeBuilder{
		ownerRecipes: []*dto.Recipe{
			{Name: "tmp-abc-keystone", SelfLink: "/recipe/tmp-abc-keystone", Project: "sunbeam"},
			{Name: "tmp-abc-nova", SelfLink: "/recipe/tmp-abc-nova", Project: "sunbeam"},
			{Name: "other-recipe", SelfLink: "/recipe/other", Project: "sunbeam"},
		},
		recipes: map[string]*dto.Recipe{
			"tmp-abc-keystone": {Name: "tmp-abc-keystone", SelfLink: "/recipe/tmp-abc-keystone"},
			"tmp-abc-nova":     {Name: "tmp-abc-nova", SelfLink: "/recipe/tmp-abc-nova"},
		},
	}
	repoMgr := &mockRepoManager{
		repoSelfLink: "/repo/sunbeam",
		gitSSHURL:    "ssh://git.lp/~user/proj/+git/sunbeam",
		project:      "user-sunbeam-remote-build",
		branches: []port.BranchRef{
			{Path: "refs/heads/tmp-abc-12345678", SelfLink: "/ref/tmp-abc"},
			{Path: "refs/heads/main", SelfLink: "/ref/main"},
		},
	}

	svc := NewService(
		map[string]ProjectBuilder{
			"sunbeam": {Builder: builder, Owner: "team", Project: "sunbeam", Artifacts: []string{"keystone"}, Strategy: &mockStrategy{}},
		},
		repoMgr, testLogger(),
	)

	result, err := svc.Cleanup(context.Background(), CleanupOpts{
		Owner:  "team",
		Prefix: "tmp-abc",
	})
	if err != nil {
		t.Fatalf("Cleanup() error: %v", err)
	}

	if len(result.DeletedRecipes) != 2 {
		t.Errorf("expected 2 deleted recipes, got %d: %v", len(result.DeletedRecipes), result.DeletedRecipes)
	}
	if len(result.DeletedBranches) != 1 {
		t.Errorf("expected 1 deleted branch, got %d: %v", len(result.DeletedBranches), result.DeletedBranches)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/service/build/... -run TestCleanup_Prefix -v`
Expected: FAIL — `CleanupResult` not defined

- [ ] **Step 3: Define CleanupResult and rework Cleanup**

In `service.go`, replace `CleanupOpts` and `Cleanup`:

```go
// CleanupOpts holds options for cleaning up temporary recipes.
type CleanupOpts struct {
	Projects  []string
	Owner     string
	Prefix    string
	DryRun    bool
	TargetRef string // LP project for branch cleanup resolution
}

// CleanupResult holds the result of a cleanup operation.
type CleanupResult struct {
	DeletedRecipes  []string
	DeletedBranches []string
}

// Cleanup removes temporary recipes matching the given prefix and their source branches.
func (s *Service) Cleanup(ctx context.Context, opts CleanupOpts) (*CleanupResult, error) {
	projFilter := make(map[string]bool, len(opts.Projects))
	for _, p := range opts.Projects {
		projFilter[p] = true
	}

	owner := opts.Owner
	result := &CleanupResult{}

	for name, pb := range s.projects {
		if len(projFilter) > 0 && !projFilter[name] {
			continue
		}

		projOwner := pb.Owner
		if owner != "" {
			projOwner = owner
		}
		if projOwner == "" {
			s.logger.Warn("skipping cleanup: owner required", "project", name)
			continue
		}

		// Discover recipes by prefix using ListRecipesByOwner.
		allRecipes, err := pb.Builder.ListRecipesByOwner(ctx, projOwner)
		if err != nil {
			s.logger.Warn("error listing recipes by owner", "project", name, "error", err)
			continue
		}

		targetRef := pb.RecipeProject()
		if opts.TargetRef != "" {
			targetRef = opts.TargetRef
		}

		for _, recipe := range allRecipes {
			if opts.Prefix != "" && !strings.HasPrefix(recipe.Name, opts.Prefix) {
				continue
			}
			if targetRef != "" && recipe.Project != "" && recipe.Project != targetRef {
				continue
			}

			if opts.DryRun {
				s.logger.Info("would delete recipe", "recipe", recipe.Name)
				result.DeletedRecipes = append(result.DeletedRecipes, recipe.Name)
				continue
			}

			if err := pb.Builder.DeleteRecipe(ctx, recipe.SelfLink); err != nil {
				s.logger.Warn("failed to delete recipe", "recipe", recipe.Name, "error", err)
				continue
			}
			result.DeletedRecipes = append(result.DeletedRecipes, recipe.Name)
		}
	}

	// Delete matching remote branches via LP API.
	if s.repoManager != nil && opts.Prefix != "" && owner != "" {
		if err := s.cleanupBranches(ctx, opts, result); err != nil {
			s.logger.Warn("branch cleanup failed", "error", err)
		}
	}

	return result, nil
}

func (s *Service) cleanupBranches(ctx context.Context, opts CleanupOpts, result *CleanupResult) error {
	targetRef := opts.TargetRef
	if targetRef == "" {
		// Resolve user's LP project for branch cleanup.
		var err error
		targetRef, err = s.repoManager.GetOrCreateProject(ctx, opts.Owner)
		if err != nil {
			return fmt.Errorf("resolve LP project for branch cleanup: %w", err)
		}
	}

	// We need the repo self_link. Try each project to find a matching repo.
	for _, pb := range s.projects {
		repoSelfLink, _, err := s.repoManager.GetOrCreateRepo(ctx, opts.Owner, targetRef, pb.Project)
		if err != nil {
			continue
		}

		branches, err := s.repoManager.ListBranches(ctx, repoSelfLink)
		if err != nil {
			s.logger.Warn("error listing branches", "error", err)
			continue
		}

		branchPrefix := "refs/heads/tmp-" + opts.Prefix
		for _, branch := range branches {
			if !strings.HasPrefix(branch.Path, branchPrefix) {
				continue
			}

			if opts.DryRun {
				s.logger.Info("would delete branch", "branch", branch.Path)
				result.DeletedBranches = append(result.DeletedBranches, branch.Path)
				continue
			}

			if err := s.repoManager.DeleteGitRef(ctx, branch.SelfLink); err != nil {
				s.logger.Warn("failed to delete branch", "branch", branch.Path, "error", err)
				continue
			}
			result.DeletedBranches = append(result.DeletedBranches, branch.Path)
		}
	}

	return nil
}
```

- [ ] **Step 4: Run cleanup test**

Run: `go test ./internal/core/service/build/... -run TestCleanup -v`
Expected: PASS

- [ ] **Step 5: Run all service tests**

Run: `go test ./internal/core/service/build/... -v`
Expected: ALL PASS

- [ ] **Step 6: Commit**

```bash
git add internal/core/service/build/service.go internal/core/service/build/service_test.go
git commit -m "fix: rework Cleanup to use prefix-based discovery and add branch deletion"
```

---

### Task 8: Update API, Client, and Frontend Workflow for CleanupResult

**Files:**
- Modify: `internal/adapter/primary/api/builds.go`
- Modify: `internal/adapter/primary/frontend/build_server_workflow.go`
- Modify: `internal/adapter/primary/frontend/build_workflow.go`
- Modify: `pkg/client/builds.go`

- [ ] **Step 1: Update BuildServerWorkflow.Cleanup return type**

In `build_server_workflow.go`, update the `Cleanup` method signature to return `*build.CleanupResult` instead of `[]string`. Find the current cleanup method and update it. The method should pass `TargetRef` from the cleanup opts.

- [ ] **Step 2: Update API cleanup handler**

In `builds.go`, update the cleanup output struct to include both recipes and branches:

```go
type BuildsCleanupOutput struct {
	Body struct {
		DeletedRecipes  []string `json:"deleted_recipes"`
		DeletedBranches []string `json:"deleted_branches"`
		// Keep backwards compat in JSON for now
		Deleted []string `json:"deleted"`
	}
}
```

- [ ] **Step 3: Update client BuildsCleanupResult**

In `pkg/client/builds.go`:

```go
type BuildsCleanupResult struct {
	DeletedRecipes  []string `json:"deleted_recipes"`
	DeletedBranches []string `json:"deleted_branches"`
	Deleted         []string `json:"deleted"` // backwards compat
}

func (c *Client) BuildsCleanup(ctx context.Context, opts BuildsCleanupOptions) (*BuildsCleanupResult, error) {
	var result BuildsCleanupResult
	err := c.post(ctx, "/api/v1/builds/cleanup", opts, &result)
	return &result, err
}
```

- [ ] **Step 4: Update BuildWorkflow.Cleanup return type**

In `build_workflow.go`, update `Cleanup` to return `*client.BuildsCleanupResult`:

```go
func (w *BuildWorkflow) Cleanup(ctx context.Context, req BuildCleanupRequest) (*client.BuildsCleanupResult, error) {
	if w.client == nil {
		return nil, errors.New("build workflow requires an API client")
	}
	return w.client.BuildsCleanup(ctx, client.BuildsCleanupOptions{
		Project: req.Project,
		Owner:   req.Owner,
		Prefix:  req.Prefix,
		DryRun:  req.DryRun,
	})
}
```

- [ ] **Step 5: Verify compilation**

Run: `go build ./...`
Expected: PASS (may need to fix callers of Cleanup — check CLI)

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/primary/api/builds.go internal/adapter/primary/frontend/build_server_workflow.go internal/adapter/primary/frontend/build_workflow.go pkg/client/builds.go
git commit -m "feat: propagate CleanupResult with deleted recipes and branches through API stack"
```

---

### Task 9: Update CLI Build Commands

**Files:**
- Modify: `internal/adapter/primary/cli/build.go`

- [ ] **Step 1: Remove --source flag from trigger, list, download commands**

In the trigger command, remove the `--source` flag binding. Change the local-preparation check from `req.Source == "local"` to checking `req.LocalPath != ""`.

In the list and download commands, similarly remove `--source` flag and derive the mode from whether `--prefix` / `--sha` are set.

- [ ] **Step 2: Update cleanup output to show deleted branches**

In the cleanup command's output rendering, show both deleted recipes and deleted branches:

```go
// After calling Cleanup, render:
result, err := buildsFrontend.Cleanup(ctx, req)
// ...
for _, name := range result.DeletedRecipes {
    fmt.Fprintf(cmd.OutOrStdout(), "deleted recipe: %s\n", name)
}
for _, branch := range result.DeletedBranches {
    fmt.Fprintf(cmd.OutOrStdout(), "deleted branch: %s\n", branch)
}
```

- [ ] **Step 3: Verify compilation and run CLI tests**

Run: `go build ./cmd/watchtower/... && go test ./internal/adapter/primary/cli/... -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/adapter/primary/cli/build.go
git commit -m "feat(cli): remove --source flag, show deleted branches in cleanup output"
```

---

### Task 10: Update watchtower.yaml Config

**Files:**
- Modify: `watchtower.yaml`

- [ ] **Step 1: Add build blocks to charm and snap projects**

Add `build:` blocks to `sunbeam-charms`:

```yaml
  - name: sunbeam-charms
    artifact_type: charm
    code:
      forge: gerrit
      host: https://review.opendev.org
      project: openstack/sunbeam-charms
    build:
      owner: canonical
      official_codehosting: true
      lp_project: sunbeam-charms
      prepare_command: "./repository.py prepare"
    bugs:
      # ... existing
```

Add `build:` blocks to snap projects as needed (e.g., `openstack`):

```yaml
  - name: openstack
    artifact_type: snap
    code:
      forge: github
      owner: canonical
      project: snap-openstack
    build:
      owner: canonical
      official_codehosting: true
      lp_project: snap-openstack
    bugs:
      # ... existing
```

- [ ] **Step 2: Commit**

```bash
git add watchtower.yaml
git commit -m "config: add build blocks to charm and snap projects"
```

---

### Task 11: Full Validation

- [ ] **Step 1: Run full test suite**

Run: `go test ./... -count=1`
Expected: ALL PASS

- [ ] **Step 2: Run linter**

Run: `golangci-lint run ./...`
Expected: PASS

- [ ] **Step 3: Run architecture check**

Run: `arch-go --color no`
Expected: PASS

- [ ] **Step 4: Run pre-commit**

Run: `pre-commit run --all-files`
Expected: PASS

- [ ] **Step 5: Update PLAN.md**

Add the build prepare pipeline work to the "Current State" section and remove/update any related gaps. Note that:
- build trigger now supports branch-based local preparation with optional prepare commands
- build cleanup now deletes both recipes and remote branches by prefix
- `--source` flag removed from CLI (local-path determines mode)

- [ ] **Step 6: Commit PLAN.md**

```bash
git add PLAN.md
git commit -m "docs: sync PLAN.md with build prepare pipeline implementation"
```

---

### Task 12: Integration Test — Build All Charms

- [ ] **Step 1: Run build trigger against sunbeam-charms**

Run:
```bash
go run ./cmd/watchtower build trigger sunbeam-charms \
  --local-path /home/guillaume.boutry@canonical.com/Documents/canonical/projects/openstack/sunbeam-charms \
  --prefix test-manual \
  --wait \
  --timeout 5h
```

This will:
1. Discover all charms in the monorepo via `CharmStrategy.DiscoverRecipes`
2. Create temp branch `tmp-test-manual-<sha>`
3. Run `./repository.py prepare` in the repo root
4. Commit the prepared tree
5. Push to LP
6. Create recipes for each discovered charm
7. Request builds and wait up to 5 hours

- [ ] **Step 2: Verify builds complete**

Check the output for build status. List builds:
```bash
go run ./cmd/watchtower build list --prefix test-manual --all
```

- [ ] **Step 3: Cleanup**

```bash
go run ./cmd/watchtower build cleanup --prefix test-manual --dry-run
go run ./cmd/watchtower build cleanup --prefix test-manual
```

Verify both recipes and branches are cleaned up.
