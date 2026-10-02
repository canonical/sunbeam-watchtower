// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package gitcache

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"

	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

var (
	testRepoOnce sync.Once
	testRepoPath string
	errTestRepo  error
)

// setupTestRepo creates a package-shared bare repo fixture with some commits.
func setupTestRepo(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary required")
	}

	testRepoOnce.Do(func() {
		dir, err := os.MkdirTemp("", "watchtower-gitcache-fixture-*")
		if err != nil {
			errTestRepo = err
			return
		}
		workDir := filepath.Join(dir, "work")
		bareDir := filepath.Join(dir, "bare.git")

		run := func(args ...string) error {
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Dir = workDir
			cmd.Env = append(os.Environ(),
				"GIT_AUTHOR_NAME=Test Author",
				"GIT_AUTHOR_EMAIL=test@example.com",
				"GIT_COMMITTER_NAME=Test Author",
				"GIT_COMMITTER_EMAIL=test@example.com",
			)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("command %v failed: %w\n%s", args, err, out)
			}
			return nil
		}

		if err := os.MkdirAll(workDir, 0o755); err != nil {
			errTestRepo = err
			return
		}

		if err := run("git", "init", "-b", "main"); err != nil {
			errTestRepo = err
			return
		}
		if err := run("git", "config", "user.email", "test@example.com"); err != nil {
			errTestRepo = err
			return
		}
		if err := run("git", "config", "user.name", "Test Author"); err != nil {
			errTestRepo = err
			return
		}
		if err := run("git", "config", "commit.gpgsign", "false"); err != nil {
			errTestRepo = err
			return
		}

		if err := os.WriteFile(filepath.Join(workDir, "file1.txt"), []byte("hello"), 0o644); err != nil {
			errTestRepo = err
			return
		}
		if err := run("git", "add", "file1.txt"); err != nil {
			errTestRepo = err
			return
		}
		if err := run("git", "commit", "-m", "initial commit\n\nLP: #12345"); err != nil {
			errTestRepo = err
			return
		}

		if err := os.WriteFile(filepath.Join(workDir, "file2.txt"), []byte("world"), 0o644); err != nil {
			errTestRepo = err
			return
		}
		if err := run("git", "add", "file2.txt"); err != nil {
			errTestRepo = err
			return
		}
		if err := run("git", "commit", "-m", "second commit\n\nCloses-Bug: #67890"); err != nil {
			errTestRepo = err
			return
		}

		cmd := exec.Command("git", "clone", "--bare", workDir, bareDir)
		out, err := cmd.CombinedOutput()
		if err != nil {
			errTestRepo = fmt.Errorf("bare clone failed: %w\n%s", err, out)
			return
		}

		testRepoPath = bareDir
	})
	if errTestRepo != nil {
		t.Fatalf("setupTestRepo() error: %v", errTestRepo)
	}
	return testRepoPath
}

func TestCache_EnsureRepo_CloneAndList(t *testing.T) {
	t.Parallel()
	bareRepo := setupTestRepo(t)
	cacheDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cache := NewCache(filepath.Join(cacheDir, "repos"), logger)

	ctx := context.Background()
	// Use file:// URL so repoPath can parse it.
	cloneURL := "file://" + bareRepo

	// EnsureRepo should clone.
	path, err := cache.EnsureRepo(ctx, cloneURL, nil)
	if err != nil {
		t.Fatalf("EnsureRepo() error: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cloned repo not found at %s", path)
	}

	// ListCommits should return 2 commits.
	commits, err := cache.ListCommits(ctx, cloneURL, forge.ListCommitsOpts{Branch: "main"})
	if err != nil {
		t.Fatalf("ListCommits() error: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(commits))
	}

	// Newest first (committer time order).
	if commits[0].Message == "" {
		t.Error("commit message should not be empty")
	}
	if commits[0].Author != "Test Author" {
		t.Errorf("author = %q, want %q", commits[0].Author, "Test Author")
	}

	// Check bug refs were extracted.
	allBugRefs := make(map[string]bool)
	for _, c := range commits {
		for _, ref := range c.BugRefs {
			allBugRefs[ref.ID] = true
		}
	}
	if !allBugRefs["12345"] {
		t.Error("expected bug ref 12345")
	}
	if !allBugRefs["67890"] {
		t.Error("expected bug ref 67890")
	}
}

func TestCache_FetchesRevisionTagAndListsTaggedHistory(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary required")
	}
	dir := t.TempDir()
	workDir := filepath.Join(dir, "work")
	originDir := filepath.Join(dir, "origin.git")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(cwd string, args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = cwd
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test Author", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test Author", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	run(workDir, "git", "init", "-b", "main")
	run(workDir, "git", "config", "user.email", "test@example.com")
	run(workDir, "git", "config", "user.name", "Test Author")
	if err := os.WriteFile(filepath.Join(workDir, "fix"), []byte("fixed"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(workDir, "git", "add", "fix")
	run(workDir, "git", "commit", "-m", "fix\n\nCloses-Bug: #12345")
	run(dir, "git", "clone", "--bare", workDir, originDir)

	cache := NewCache(filepath.Join(dir, "cache"), nil)
	cloneURL := "file://" + originDir
	if _, err := cache.EnsureRepo(context.Background(), cloneURL, nil); err != nil {
		t.Fatal(err)
	}

	// Add an annotated revision tag without advancing the branch.
	run(workDir, "git", "remote", "add", "origin", originDir)
	run(workDir, "git", "tag", "-a", "rev1101", "-m", "snap revision 1101")
	run(workDir, "git", "push", "origin", "refs/tags/rev1101")
	if err := cache.Fetch(context.Background(), cloneURL, nil); err != nil {
		t.Fatal(err)
	}

	commits, err := cache.ListCommits(context.Background(), cloneURL, forge.ListCommitsOpts{Revision: "refs/tags/rev1101^{}"})
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 1 || len(commits[0].BugRefs) != 1 || commits[0].BugRefs[0].ID != "12345" {
		t.Fatalf("tagged commits = %+v", commits)
	}
}

func TestCache_EnsureRepo_FetchExisting(t *testing.T) {
	t.Parallel()
	bareRepo := setupTestRepo(t)
	cacheDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cache := NewCache(filepath.Join(cacheDir, "repos"), logger)
	ctx := context.Background()
	cloneURL := "file://" + bareRepo

	// Clone first.
	_, err := cache.EnsureRepo(ctx, cloneURL, nil)
	if err != nil {
		t.Fatalf("first EnsureRepo() error: %v", err)
	}

	// EnsureRepo again should fetch (not fail).
	path, err := cache.EnsureRepo(ctx, cloneURL, nil)
	if err != nil {
		t.Fatalf("second EnsureRepo() error: %v", err)
	}
	if path == "" {
		t.Error("path should not be empty")
	}
}

func TestCache_EnsureRepo_FetchExistingUpdatesBareHEAD(t *testing.T) {
	t.Parallel()

	origin := createMutableRepo(t, map[string]string{
		"snap/snapcraft.yaml": "name: old-snap\n",
	})
	cacheDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cache := NewCache(filepath.Join(cacheDir, "repos"), logger)
	ctx := context.Background()
	cloneURL := "file://" + origin

	path, err := cache.EnsureRepo(ctx, cloneURL, nil)
	if err != nil {
		t.Fatalf("initial EnsureRepo() error: %v", err)
	}
	initial, err := ReadHEADFile(path, "snap/snapcraft.yaml")
	if err != nil {
		t.Fatalf("ReadHEADFile() initial error: %v", err)
	}
	if string(initial) != "name: old-snap\n" {
		t.Fatalf("initial HEAD file = %q, want old snap", initial)
	}

	commitFiles(t, origin, map[string]string{
		"snap/snapcraft.yaml": "name: new-snap\n",
	}, "update manifest")

	if _, err := cache.EnsureRepo(ctx, cloneURL, nil); err != nil {
		t.Fatalf("second EnsureRepo() error: %v", err)
	}
	updated, err := ReadHEADFile(path, "snap/snapcraft.yaml")
	if err != nil {
		t.Fatalf("ReadHEADFile() updated error: %v", err)
	}
	if string(updated) != "name: new-snap\n" {
		t.Fatalf("updated HEAD file = %q, want new snap", updated)
	}
}

func TestCache_EnsureRepo_FetchExistingReturnsFetchError(t *testing.T) {
	t.Parallel()

	origin := createMutableRepo(t, map[string]string{
		"README.md": "initial\n",
	})
	cacheDir := t.TempDir()
	cache := NewCache(filepath.Join(cacheDir, "repos"), nil)
	ctx := context.Background()
	cloneURL := "file://" + origin

	path, err := cache.EnsureRepo(ctx, cloneURL, nil)
	if err != nil {
		t.Fatalf("initial EnsureRepo() error: %v", err)
	}

	runGitDir(t, path, "config", "remote.origin.url", "file://"+filepath.Join(t.TempDir(), "missing"))

	if _, err := cache.EnsureRepo(ctx, cloneURL, nil); err == nil {
		t.Fatal("second EnsureRepo() error = nil, want fetch error")
	}
}

func TestCache_ListCommits_SinceFilter(t *testing.T) {
	t.Parallel()
	bareRepo := setupTestRepo(t)
	cacheDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cache := NewCache(filepath.Join(cacheDir, "repos"), logger)
	ctx := context.Background()
	cloneURL := "file://" + bareRepo

	if _, err := cache.EnsureRepo(ctx, cloneURL, nil); err != nil {
		t.Fatal(err)
	}

	// Since far in the future should return no commits.
	future := time.Now().Add(24 * time.Hour)
	commits, err := cache.ListCommits(ctx, cloneURL, forge.ListCommitsOpts{
		Branch: "main",
		Since:  &future,
	})
	if err != nil {
		t.Fatalf("ListCommits() error: %v", err)
	}
	if len(commits) != 0 {
		t.Errorf("expected 0 commits with future Since, got %d", len(commits))
	}
}

func createMutableRepo(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "origin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", dir, err)
	}
	runGitDir(t, dir, "init", "-b", "main")
	runGitDir(t, dir, "config", "user.email", "test@example.com")
	runGitDir(t, dir, "config", "user.name", "Test Author")
	runGitDir(t, dir, "config", "commit.gpgsign", "false")
	commitFiles(t, dir, files, "initial")
	return dir
}

func commitFiles(t *testing.T, repoDir string, files map[string]string, message string) {
	t.Helper()

	for name, content := range files {
		fullPath := filepath.Join(repoDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", fullPath, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", fullPath, err)
		}
		runGitDir(t, repoDir, "add", filepath.FromSlash(name))
	}
	runGitDir(t, repoDir, "commit", "-m", message)
}

func runGitDir(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test Author",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test Author",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func TestCache_Remove(t *testing.T) {
	t.Parallel()
	bareRepo := setupTestRepo(t)
	cacheDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cache := NewCache(filepath.Join(cacheDir, "repos"), logger)
	ctx := context.Background()
	cloneURL := "file://" + bareRepo

	path, err := cache.EnsureRepo(ctx, cloneURL, nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := cache.Remove(cloneURL); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("repo should have been removed")
	}
}

func TestCache_RemoveAll(t *testing.T) {
	t.Parallel()
	bareRepo := setupTestRepo(t)
	cacheDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cache := NewCache(filepath.Join(cacheDir, "repos"), logger)
	ctx := context.Background()
	cloneURL := "file://" + bareRepo

	if _, err := cache.EnsureRepo(ctx, cloneURL, nil); err != nil {
		t.Fatal(err)
	}

	if err := cache.RemoveAll(); err != nil {
		t.Fatalf("RemoveAll() error: %v", err)
	}

	if _, err := os.Stat(cache.CacheDir()); !os.IsNotExist(err) {
		t.Error("cache dir should have been removed")
	}
}

func TestCache_StoreMRMetadata_RoundTrip(t *testing.T) {
	t.Parallel()
	bareRepo := setupTestRepo(t)
	cacheDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cache := NewCache(filepath.Join(cacheDir, "repos"), logger)
	ctx := context.Background()
	cloneURL := "file://" + bareRepo

	if _, err := cache.EnsureRepo(ctx, cloneURL, nil); err != nil {
		t.Fatal(err)
	}

	// Store metadata.
	mrs := []dto.MRMetadata{
		{ID: "#42", State: forge.MergeStateOpen, URL: "https://github.com/org/repo/pull/42", HeadSHA: "abc123", GitRef: "refs/pull/42/head"},
		{ID: "#43", State: forge.MergeStateMerged, URL: "https://github.com/org/repo/pull/43", HeadSHA: "def456", GitRef: "refs/pull/43/head"},
	}
	if err := cache.StoreMRMetadata(cloneURL, mrs); err != nil {
		t.Fatalf("StoreMRMetadata() error: %v", err)
	}

	// Load and verify.
	loaded, err := cache.LoadMRMetadata(cloneURL)
	if err != nil {
		t.Fatalf("LoadMRMetadata() error: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 MR metadata entries, got %d", len(loaded))
	}
	if loaded[0].ID != "#42" {
		t.Errorf("first MR ID = %q, want %q", loaded[0].ID, "#42")
	}
	if loaded[0].State != forge.MergeStateOpen {
		t.Errorf("first MR state = %v, want Open", loaded[0].State)
	}
	if loaded[1].State != forge.MergeStateMerged {
		t.Errorf("second MR state = %v, want Merged", loaded[1].State)
	}
}

func TestCache_LoadMRMetadata_NoFile(t *testing.T) {
	t.Parallel()
	bareRepo := setupTestRepo(t)
	cacheDir := t.TempDir()

	cache := NewCache(filepath.Join(cacheDir, "repos"), nil)
	ctx := context.Background()
	cloneURL := "file://" + bareRepo

	if _, err := cache.EnsureRepo(ctx, cloneURL, nil); err != nil {
		t.Fatal(err)
	}

	// No metadata file should return nil without error.
	mrs, err := cache.LoadMRMetadata(cloneURL)
	if err != nil {
		t.Fatalf("LoadMRMetadata() error: %v", err)
	}
	if mrs != nil {
		t.Errorf("expected nil, got %v", mrs)
	}
}

func TestCache_ListMRCommits(t *testing.T) {
	t.Parallel()
	bareRepo := setupTestRepo(t)
	cacheDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cache := NewCache(filepath.Join(cacheDir, "repos"), logger)
	ctx := context.Background()
	cloneURL := "file://" + bareRepo

	if _, err := cache.EnsureRepo(ctx, cloneURL, nil); err != nil {
		t.Fatal(err)
	}

	// Get a real commit SHA from the repo to use in MR metadata.
	commits, err := cache.ListCommits(ctx, cloneURL, forge.ListCommitsOpts{Branch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) == 0 {
		t.Fatal("no commits in test repo")
	}
	headSHA := commits[0].SHA

	// Store MR metadata pointing to a real commit by SHA.
	mrs := []dto.MRMetadata{
		{ID: "#1", State: forge.MergeStateOpen, URL: "https://example.com/pr/1", HeadSHA: headSHA},
	}
	if err := cache.StoreMRMetadata(cloneURL, mrs); err != nil {
		t.Fatal(err)
	}

	// ListMRCommits should resolve the commit by SHA.
	mrCommits, err := cache.ListMRCommits(ctx, cloneURL)
	if err != nil {
		t.Fatalf("ListMRCommits() error: %v", err)
	}
	if len(mrCommits) != 1 {
		t.Fatalf("expected 1 MR commit, got %d", len(mrCommits))
	}
	if mrCommits[0].SHA != headSHA {
		t.Errorf("MR commit SHA = %q, want %q", mrCommits[0].SHA, headSHA)
	}
	if mrCommits[0].MergeRequest == nil {
		t.Fatal("MR commit should have MergeRequest annotation")
	}
	if mrCommits[0].MergeRequest.ID != "#1" {
		t.Errorf("MR ID = %q, want %q", mrCommits[0].MergeRequest.ID, "#1")
	}
	if mrCommits[0].MergeRequest.State != forge.MergeStateOpen {
		t.Errorf("MR state = %v, want Open", mrCommits[0].MergeRequest.State)
	}
}

func TestCache_repoPath(t *testing.T) {
	t.Parallel()
	base := filepath.Join(t.TempDir(), "cache", "repos")
	cache := NewCache(base, nil)

	tests := []struct {
		url     string
		want    string
		wantErr bool
	}{
		{
			url:  "https://github.com/org/repo.git",
			want: filepath.Join(base, "github.com/org/repo.git"),
		},
		{
			url:  "https://github.com/org/repo",
			want: filepath.Join(base, "github.com/org/repo.git"),
		},
		{
			url:  "https://review.opendev.org/openstack/nova",
			want: filepath.Join(base, "review.opendev.org/openstack/nova.git"),
		},
		{
			url:  "https://git.launchpad.net/sunbeam",
			want: filepath.Join(base, "git.launchpad.net/sunbeam.git"),
		},
		{
			url:     "not-a-url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got, err := cache.repoPath(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("repoPath() = %q, want %q", got, tt.want)
			}
		})
	}
}
