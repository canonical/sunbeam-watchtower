// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package git_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	adapter "github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/git"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// requireGit skips the test if the git binary is not on PATH.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary required")
	}
}

// initTestRepo creates a temporary git repo with one commit.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}
	// Create a file and commit it.
	f, err := os.Create(filepath.Join(dir, "README.md"))
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	f.Close()
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatalf("add: %v", err)
	}
	_, err = wt.Commit("initial commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@test.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	return dir
}

func TestIsRepo(t *testing.T) {
	c := adapter.NewClient(nil)
	dir := initTestRepo(t)
	if !c.IsRepo(dir) {
		t.Error("expected IsRepo to return true for a git repo")
	}
	nonGit := t.TempDir()
	if c.IsRepo(nonGit) {
		t.Error("expected IsRepo to return false for a non-git directory")
	}
}

func TestHeadSHA(t *testing.T) {
	c := adapter.NewClient(nil)
	dir := initTestRepo(t)
	sha, err := c.HeadSHA(dir)
	if err != nil {
		t.Fatalf("HeadSHA: %v", err)
	}
	if len(sha) != 40 {
		t.Errorf("expected 40-char SHA, got %q (len %d)", sha, len(sha))
	}
}

func TestHasUncommittedChanges(t *testing.T) {
	c := adapter.NewClient(nil)
	dir := initTestRepo(t)

	// Clean repo should have no uncommitted changes.
	dirty, err := c.HasUncommittedChanges(dir)
	if err != nil {
		t.Fatalf("HasUncommittedChanges: %v", err)
	}
	if dirty {
		t.Error("expected no uncommitted changes in clean repo")
	}

	// Create an untracked file.
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("hello"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	dirty, err = c.HasUncommittedChanges(dir)
	if err != nil {
		t.Fatalf("HasUncommittedChanges: %v", err)
	}
	if !dirty {
		t.Error("expected uncommitted changes after adding a file")
	}
}

func TestAddAndRemoveRemote(t *testing.T) {
	c := adapter.NewClient(nil)
	dir := initTestRepo(t)

	if err := c.AddRemote(dir, "upstream", "https://example.com/repo.git"); err != nil {
		t.Fatalf("AddRemote: %v", err)
	}

	// Verify remote exists by opening repo.
	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := repo.Remote("upstream"); err != nil {
		t.Fatalf("expected remote upstream to exist: %v", err)
	}

	if err := c.RemoveRemote(dir, "upstream"); err != nil {
		t.Fatalf("RemoveRemote: %v", err)
	}
	if _, err := repo.Remote("upstream"); err == nil {
		t.Error("expected remote upstream to be removed")
	}
}

func TestPush_DetachedHEAD(t *testing.T) {
	c := adapter.NewClient(nil)
	dir := initTestRepo(t)

	// Detach HEAD by checking out the commit hash directly.
	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	if err := wt.Checkout(&gogit.CheckoutOptions{Hash: head.Hash()}); err != nil {
		t.Fatalf("checkout: %v", err)
	}

	err = c.Push(dir, "origin", "HEAD", "refs/heads/main", false)
	if err == nil {
		t.Fatal("expected error for detached HEAD push")
	}
	if got := err.Error(); !strings.Contains(got, "detached") {
		t.Fatalf("expected detached HEAD error, got: %v", err)
	}
}

func TestPush_NonexistentRemote(t *testing.T) {
	c := adapter.NewClient(nil)
	dir := initTestRepo(t)

	err := c.Push(dir, "nonexistent", "refs/heads/master", "refs/heads/main", false)
	if err == nil {
		t.Fatal("expected error for nonexistent remote")
	}
}

func TestCreateBranchAndCheckout(t *testing.T) {
	c := adapter.NewClient(nil)
	dir := initTestRepo(t)

	if err := c.CreateBranch(dir, "feature", "HEAD"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if err := c.CheckoutBranch(dir, "feature"); err != nil {
		t.Fatalf("CheckoutBranch: %v", err)
	}
	branch, err := c.CurrentBranch(dir)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if branch != "feature" {
		t.Errorf("CurrentBranch = %q, want feature", branch)
	}
}

func TestDeleteLocalBranch(t *testing.T) {
	c := adapter.NewClient(nil)
	dir := initTestRepo(t)

	if err := c.CreateBranch(dir, "to-delete", "HEAD"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if err := c.DeleteLocalBranch(dir, "to-delete"); err != nil {
		t.Fatalf("DeleteLocalBranch: %v", err)
	}
	// Checkout to deleted branch should fail.
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
	if branch != "master" {
		t.Errorf("CurrentBranch = %q, want master", branch)
	}
}

func TestAddAllAndCommit(t *testing.T) {
	c := adapter.NewClient(nil)
	dir := initTestRepo(t)

	origSHA, err := c.HeadSHA(dir)
	if err != nil {
		t.Fatalf("HeadSHA: %v", err)
	}

	// Write a new file.
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("content"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if err := c.AddAll(dir); err != nil {
		t.Fatalf("AddAll: %v", err)
	}
	if err := c.Commit(dir, "add new file"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Repo should be clean now.
	dirty, err := c.HasUncommittedChanges(dir)
	if err != nil {
		t.Fatalf("HasUncommittedChanges: %v", err)
	}
	if dirty {
		t.Error("expected clean repo after commit")
	}

	// SHA should have changed.
	newSHA, err := c.HeadSHA(dir)
	if err != nil {
		t.Fatalf("HeadSHA: %v", err)
	}
	if newSHA == origSHA {
		t.Error("expected SHA to change after commit")
	}
}

func TestResetHard(t *testing.T) {
	c := adapter.NewClient(nil)
	dir := initTestRepo(t)

	origSHA, err := c.HeadSHA(dir)
	if err != nil {
		t.Fatalf("HeadSHA: %v", err)
	}

	// Create a second commit.
	if err := os.WriteFile(filepath.Join(dir, "extra.txt"), []byte("data"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := c.AddAll(dir); err != nil {
		t.Fatalf("AddAll: %v", err)
	}
	if err := c.Commit(dir, "second commit"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	newSHA, err := c.HeadSHA(dir)
	if err != nil {
		t.Fatalf("HeadSHA: %v", err)
	}
	if newSHA == origSHA {
		t.Fatal("expected different SHA after second commit")
	}

	// Reset to original commit.
	if err := c.ResetHard(dir, origSHA); err != nil {
		t.Fatalf("ResetHard: %v", err)
	}

	resetSHA, err := c.HeadSHA(dir)
	if err != nil {
		t.Fatalf("HeadSHA: %v", err)
	}
	if resetSHA != origSHA {
		t.Errorf("expected SHA %s after reset, got %s", origSHA, resetSHA)
	}
}

func TestClient_Commit_LinkedWorktree(t *testing.T) {
	t.Parallel()
	requireGit(t)
	// Arrange: create a source repo with one commit.
	srcDir := t.TempDir()
	runGit(t, srcDir, "init", "-q", "-b", "main")
	runGit(t, srcDir, "config", "user.email", "test@example.com")
	runGit(t, srcDir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(srcDir, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, srcDir, "add", "README.md")
	runGit(t, srcDir, "commit", "-q", "-m", "init")

	sha := strings.TrimSpace(runGit(t, srcDir, "rev-parse", "HEAD"))

	// Create a linked worktree via the real git CLI.
	wtDir := filepath.Join(t.TempDir(), "wt")
	runGit(t, srcDir, "worktree", "add", "-b", "tmp-branch", wtDir, sha)

	// Add a new file inside the linked worktree.
	if err := os.WriteFile(filepath.Join(wtDir, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, wtDir, "add", "new.txt")

	// Act: commit via our adapter.
	c := adapter.NewClient(nil)
	if err := c.Commit(wtDir, "test commit in linked worktree"); err != nil {
		t.Fatalf("Commit in linked worktree: %v", err)
	}

	// Assert: HEAD advanced.
	newSHA := strings.TrimSpace(runGit(t, wtDir, "rev-parse", "HEAD"))
	if newSHA == sha {
		t.Fatalf("HEAD did not advance: still %s", sha)
	}
}

// runGit is a test helper that invokes the git CLI and fails the test on error.
// It strips git-specific environment variables (GIT_DIR, GIT_INDEX_FILE, etc.)
// so that tests run correctly even when invoked from inside a git hook, where
// git sets these variables to point at the host repository.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	// Filter out git plumbing env vars that would redirect git operations
	// away from the temp repo created for this test.
	gitEnvPrefixes := []string{
		"GIT_DIR=",
		"GIT_INDEX_FILE=",
		"GIT_WORK_TREE=",
		"GIT_OBJECT_DIRECTORY=",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES=",
		"GIT_COMMON_DIR=",
	}
	for _, e := range os.Environ() {
		skip := false
		for _, prefix := range gitEnvPrefixes {
			if strings.HasPrefix(e, prefix) {
				skip = true
				break
			}
		}
		if !skip {
			cmd.Env = append(cmd.Env, e)
		}
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, string(out))
	}
	return string(out)
}

func TestClient_CreateDetachedWorktree_RoundTrip(t *testing.T) {
	t.Parallel()
	requireGit(t)
	srcDir := t.TempDir()
	runGit(t, srcDir, "init", "-q", "-b", "main")
	runGit(t, srcDir, "config", "user.email", "test@example.com")
	runGit(t, srcDir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(srcDir, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, srcDir, "add", "README.md")
	runGit(t, srcDir, "commit", "-q", "-m", "init")
	sha := strings.TrimSpace(runGit(t, srcDir, "rev-parse", "HEAD"))

	c := adapter.NewClient(nil)
	wtPath, cleanup, err := c.CreateDetachedWorktree(context.Background(), srcDir, "tmp-test-branch", sha)
	if err != nil {
		t.Fatalf("CreateDetachedWorktree: %v", err)
	}

	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("worktree dir not created: %v", err)
	}
	gotBranch := strings.TrimSpace(runGit(t, wtPath, "branch", "--show-current"))
	if gotBranch != "tmp-test-branch" {
		t.Fatalf("branch = %q, want tmp-test-branch", gotBranch)
	}

	listOut := runGit(t, srcDir, "worktree", "list")
	if !strings.Contains(listOut, wtPath) {
		t.Fatalf("worktree not listed in parent: %s", listOut)
	}

	cleanup()

	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Fatalf("worktree dir still present after cleanup: err=%v", err)
	}
	listOut = runGit(t, srcDir, "worktree", "list")
	if strings.Contains(listOut, wtPath) {
		t.Fatalf("worktree still listed after cleanup: %s", listOut)
	}
	branches := runGit(t, srcDir, "branch", "--list")
	if strings.Contains(branches, "tmp-test-branch") {
		t.Fatalf("branch still present after cleanup: %s", branches)
	}

	// Double-cleanup is safe.
	cleanup()
}

func TestClient_CreateDetachedWorktree_FixedArgv(t *testing.T) {
	t.Parallel()
	requireGit(t)
	// Proof that we don't route through `sh -c`: a branch name with
	// shell metacharacters must reach git as literal argv. git itself
	// rejects it as an invalid ref, which is the signal we want.
	srcDir := t.TempDir()
	runGit(t, srcDir, "init", "-q", "-b", "main")
	runGit(t, srcDir, "config", "user.email", "test@example.com")
	runGit(t, srcDir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(srcDir, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, srcDir, "add", "README.md")
	runGit(t, srcDir, "commit", "-q", "-m", "init")
	sha := strings.TrimSpace(runGit(t, srcDir, "rev-parse", "HEAD"))

	c := adapter.NewClient(nil)
	_, _, err := c.CreateDetachedWorktree(context.Background(), srcDir, ";rm -rf /", sha)
	if err == nil {
		t.Fatalf("expected git to reject malformed branch name")
	}
	if _, statErr := os.Stat(srcDir); statErr != nil {
		t.Fatalf("source dir destroyed: %v", statErr)
	}
}

func TestClient_ForceAddAll_StagesIgnoredFiles(t *testing.T) {
	t.Parallel()
	requireGit(t)
	srcDir := t.TempDir()
	runGit(t, srcDir, "init", "-q", "-b", "main")
	runGit(t, srcDir, "config", "user.email", "test@example.com")
	runGit(t, srcDir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(srcDir, ".gitignore"), []byte("build/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, srcDir, "add", ".gitignore")
	runGit(t, srcDir, "commit", "-q", "-m", "init")

	if err := os.MkdirAll(filepath.Join(srcDir, "build"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "build", "artifact.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Baseline sanity: plain `git add -A` does NOT stage the ignored file.
	runGit(t, srcDir, "add", "-A")
	staged := strings.TrimSpace(runGit(t, srcDir, "diff", "--cached", "--name-only"))
	if strings.Contains(staged, "build/artifact.txt") {
		t.Fatalf("plain add staged ignored file — test setup wrong: %s", staged)
	}
	runGit(t, srcDir, "reset", "-q")

	c := adapter.NewClient(nil)
	if err := c.ForceAddAll(context.Background(), srcDir); err != nil {
		t.Fatalf("ForceAddAll: %v", err)
	}

	staged = strings.TrimSpace(runGit(t, srcDir, "diff", "--cached", "--name-only"))
	if !strings.Contains(staged, "build/artifact.txt") {
		t.Fatalf("ignored file not staged: %s", staged)
	}
}
