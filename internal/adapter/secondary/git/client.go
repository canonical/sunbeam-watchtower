// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package git

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

// Client implements port.GitClient using go-git.
type Client struct {
	logger *slog.Logger
}

var _ port.GitClient = (*Client)(nil)

// NewClient creates a new git Client. If logger is nil, a no-op logger is used.
func NewClient(logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Client{logger: logger}
}

// openRepo opens a repository with linked-worktree support enabled so
// operations run inside a worktree created by `git worktree add` reach
// the shared object store via the `.git` gitdir pointer file.
func openRepo(path string) (*gogit.Repository, error) {
	return gogit.PlainOpenWithOptions(path, &gogit.PlainOpenOptions{
		EnableDotGitCommonDir: true,
	})
}

func (c *Client) IsRepo(path string) bool {
	c.logger.Debug("checking if path is repo", "path", path)
	_, err := openRepo(path)
	return err == nil
}

func (c *Client) HeadSHA(path string) (string, error) {
	c.logger.Debug("reading HEAD SHA", "path", path)
	repo, err := openRepo(path)
	if err != nil {
		return "", fmt.Errorf("open repo %s: %w", path, err)
	}
	ref, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("get HEAD for %s: %w", path, err)
	}
	sha := ref.Hash().String()
	c.logger.Debug("HEAD SHA resolved", "path", path, "sha", sha)
	return sha, nil
}

func (c *Client) HasUncommittedChanges(path string) (bool, error) {
	c.logger.Debug("checking for uncommitted changes", "path", path)
	repo, err := openRepo(path)
	if err != nil {
		return false, fmt.Errorf("open repo %s: %w", path, err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return false, fmt.Errorf("get worktree for %s: %w", path, err)
	}
	status, err := wt.Status()
	if err != nil {
		return false, fmt.Errorf("get status for %s: %w", path, err)
	}
	return !status.IsClean(), nil
}

func (c *Client) Push(path, remote, localRef, remoteRef string, force bool) error {
	c.logger.Debug("pushing", "path", path, "remote", remote, "localRef", localRef, "remoteRef", remoteRef, "force", force)
	repo, err := openRepo(path)
	if err != nil {
		return fmt.Errorf("open repo %s: %w", path, err)
	}

	// go-git doesn't handle HEAD in refspecs reliably — resolve it to
	// the actual branch ref so the pack negotiation sends objects.
	if localRef == "HEAD" {
		head, err := repo.Head()
		if err != nil {
			return fmt.Errorf("resolve HEAD for %s: %w", path, err)
		}
		if head.Type() == plumbing.HashReference {
			return fmt.Errorf("HEAD is detached in %s; cannot push from a detached HEAD", path)
		}
		localRef = head.Name().String()
		c.logger.Debug("resolved HEAD", "path", path, "ref", localRef)
	}

	r, err := repo.Remote(remote)
	if err != nil {
		return fmt.Errorf("get remote %s for %s: %w", remote, path, err)
	}

	urls := r.Config().URLs
	if len(urls) == 0 {
		return fmt.Errorf("remote %s for %s has no configured URLs", remote, path)
	}
	remoteURL := urls[0]
	sshUser, err := sshUserFromURL(remoteURL)
	if err != nil {
		return fmt.Errorf("determine SSH user for remote URL %s: %w", remoteURL, err)
	}

	auth, err := sshAuth(sshUser)
	if err != nil {
		return fmt.Errorf("creating SSH auth: %w", err)
	}
	refspec := config.RefSpec(fmt.Sprintf("%s:%s", localRef, remoteRef))
	opts := &gogit.PushOptions{
		RemoteName: r.Config().Name,
		RefSpecs:   []config.RefSpec{refspec},
		Auth:       auth,
		Force:      force,
	}
	if err := r.Push(opts); err != nil && !errors.Is(err, gogit.NoErrAlreadyUpToDate) {
		return fmt.Errorf("push %s to %s for %s: %w", localRef, remoteRef, path, err)
	}
	return nil
}

// sshUserFromURL extracts the SSH user from a remote URL.
// Returns an error for HTTPS remotes (unsupported).
// Falls back to the effective unix user if no user is present.
func sshUserFromURL(remoteURL string) (string, error) {
	if strings.HasPrefix(remoteURL, "https://") || strings.HasPrefix(remoteURL, "http://") {
		return "", fmt.Errorf("HTTPS remotes are not supported for push; use an SSH remote")
	}

	// SCP-style: user@host:path
	if !strings.Contains(remoteURL, "://") {
		if at := strings.Index(remoteURL, "@"); at > 0 {
			return remoteURL[:at], nil
		}
		return effectiveUser()
	}

	// URL-style: ssh://user@host/path
	u, err := url.Parse(remoteURL)
	if err == nil && u.User != nil && u.User.Username() != "" {
		return u.User.Username(), nil
	}
	return effectiveUser()
}

func effectiveUser() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("determining current user: %w", err)
	}
	return u.Username, nil
}

// sshAuth returns SSH authentication, preferring the SSH agent and falling
// back to key files in ~/.ssh/ when the agent is unavailable.
func sshAuth(sshUser string) (transport.AuthMethod, error) {
	auth, err := gitssh.NewSSHAgentAuth(sshUser)
	if err == nil {
		return auth, nil
	}

	// WATCHTOWER_SSH_KEY points to a specific key file.
	if keyPath := os.Getenv("WATCHTOWER_SSH_KEY"); keyPath != "" {
		keys, keyErr := gitssh.NewPublicKeysFromFile(sshUser, keyPath, "")
		if keyErr != nil {
			return nil, fmt.Errorf("SSH agent unavailable (%w) and cannot load key %s: %w", err, keyPath, keyErr)
		}
		return keys, nil
	}

	sshDir := os.Getenv("WATCHTOWER_SSH_KEY_DIR")
	if sshDir == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return nil, fmt.Errorf("SSH agent unavailable (%w) and cannot determine home directory: %w", err, homeErr)
		}
		sshDir = filepath.Join(home, ".ssh")
	}

	// Try common key types in preference order.
	keyNames := []string{"id_ed25519", "id_ecdsa", "id_rsa"}
	for _, name := range keyNames {
		keyPath := filepath.Join(sshDir, name)
		if _, statErr := os.Stat(keyPath); statErr != nil {
			continue
		}
		keys, keyErr := gitssh.NewPublicKeysFromFile(sshUser, keyPath, "")
		if keyErr != nil {
			continue
		}
		return keys, nil
	}

	return nil, fmt.Errorf("SSH agent unavailable (%w) and no usable key found in %s", err, sshDir)
}

func (c *Client) AddRemote(path, name, url string) error {
	c.logger.Debug("adding remote", "path", path, "name", name, "url", url)
	repo, err := openRepo(path)
	if err != nil {
		return fmt.Errorf("open repo %s: %w", path, err)
	}
	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: name,
		URLs: []string{url},
		Fetch: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("+refs/heads/*:refs/remotes/%s/*", name)),
		},
	})
	if err != nil {
		return fmt.Errorf("add remote %s to %s: %w", name, path, err)
	}
	return nil
}

func (c *Client) RemoveRemote(path, name string) error {
	c.logger.Debug("removing remote", "path", path, "name", name)
	repo, err := openRepo(path)
	if err != nil {
		return fmt.Errorf("open repo %s: %w", path, err)
	}
	if err := repo.DeleteRemote(name); err != nil {
		return fmt.Errorf("remove remote %s from %s: %w", name, path, err)
	}
	return nil
}

func (c *Client) CreateBranch(path, branchName, startPoint string) error {
	c.logger.Debug("creating branch", "path", path, "branch", branchName, "startPoint", startPoint)
	repo, err := openRepo(path)
	if err != nil {
		return fmt.Errorf("open repo %s: %w", path, err)
	}
	var h plumbing.Hash
	if startPoint == "HEAD" {
		head, err := repo.Head()
		if err != nil {
			return fmt.Errorf("resolve HEAD for %s: %w", path, err)
		}
		h = head.Hash()
	} else {
		resolved, err := repo.ResolveRevision(plumbing.Revision(startPoint))
		if err != nil {
			return fmt.Errorf("resolve revision %q for %s: %w", startPoint, path, err)
		}
		h = *resolved
	}
	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(branchName), h)
	if err := repo.Storer.SetReference(ref); err != nil {
		return fmt.Errorf("create branch %s in %s: %w", branchName, path, err)
	}
	return nil
}

func (c *Client) CheckoutBranch(path, branchName string) error {
	c.logger.Debug("checking out branch", "path", path, "branch", branchName)
	repo, err := openRepo(path)
	if err != nil {
		return fmt.Errorf("open repo %s: %w", path, err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("get worktree for %s: %w", path, err)
	}
	if err := wt.Checkout(&gogit.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branchName),
	}); err != nil {
		return fmt.Errorf("checkout branch %s in %s: %w", branchName, path, err)
	}
	return nil
}

func (c *Client) CurrentBranch(path string) (string, error) {
	c.logger.Debug("getting current branch", "path", path)
	repo, err := openRepo(path)
	if err != nil {
		return "", fmt.Errorf("open repo %s: %w", path, err)
	}
	head, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("get HEAD for %s: %w", path, err)
	}
	if !head.Name().IsBranch() {
		return "", fmt.Errorf("HEAD is not a branch in %s", path)
	}
	return head.Name().Short(), nil
}

func (c *Client) DeleteLocalBranch(path, branchName string) error {
	c.logger.Debug("deleting local branch", "path", path, "branch", branchName)
	repo, err := openRepo(path)
	if err != nil {
		return fmt.Errorf("open repo %s: %w", path, err)
	}
	if err := repo.Storer.RemoveReference(plumbing.NewBranchReferenceName(branchName)); err != nil {
		return fmt.Errorf("delete branch %s from %s: %w", branchName, path, err)
	}
	return nil
}

func (c *Client) AddAll(path string) error {
	c.logger.Debug("adding all changes", "path", path)
	repo, err := openRepo(path)
	if err != nil {
		return fmt.Errorf("open repo %s: %w", path, err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("get worktree for %s: %w", path, err)
	}
	if err := wt.AddWithOptions(&gogit.AddOptions{All: true}); err != nil {
		return fmt.Errorf("add all in %s: %w", path, err)
	}
	return nil
}

func (c *Client) Commit(path, message string) error {
	c.logger.Debug("committing", "path", path)
	repo, err := openRepo(path)
	if err != nil {
		return fmt.Errorf("open repo %s: %w", path, err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("get worktree for %s: %w", path, err)
	}
	// Pass an explicit author signature so go-git does not try to read
	// /etc/gitconfig, which may be unreadable (e.g. in confined snaps or
	// GitHub Actions runners) and would otherwise fail the commit.
	sig := &object.Signature{
		Name:  "sunbeam-watchtower",
		Email: "watchtower@localhost",
		When:  time.Now(),
	}
	if _, err := wt.Commit(message, &gogit.CommitOptions{Author: sig, Committer: sig}); err != nil {
		return fmt.Errorf("commit in %s: %w", path, err)
	}
	return nil
}

func (c *Client) ResetHard(path, ref string) error {
	c.logger.Debug("resetting hard", "path", path, "ref", ref)
	repo, err := openRepo(path)
	if err != nil {
		return fmt.Errorf("open repo %s: %w", path, err)
	}
	h, err := repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return fmt.Errorf("resolve revision %q for %s: %w", ref, path, err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("get worktree for %s: %w", path, err)
	}
	if err := wt.Reset(&gogit.ResetOptions{Commit: *h, Mode: gogit.HardReset}); err != nil {
		return fmt.Errorf("reset hard to %s in %s: %w", ref, path, err)
	}
	return nil
}

// gitEnvPrefixes lists git plumbing environment variables that must be stripped
// before shelling out to git. When running inside a git hook (e.g. pre-commit),
// git sets these to point at the host repository. Inheriting them would
// redirect subprocess git commands away from the intended working directory.
var gitEnvPrefixes = []string{
	"GIT_DIR=",
	"GIT_INDEX_FILE=",
	"GIT_WORK_TREE=",
	"GIT_OBJECT_DIRECTORY=",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES=",
	"GIT_COMMON_DIR=",
}

// runGit executes `git` with fixed argv in the given working directory.
// No shell is involved; arguments are never interpreted.
// Git plumbing env vars are stripped so that invocations inside git hooks
// operate on the intended repository, not the hook's host repository.
func (c *Client) runGit(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
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
		c.logger.Debug("git command failed", "dir", dir, "args", args, "output", string(out), "err", err)
		return fmt.Errorf("git %s: %w (output: %s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	c.logger.Debug("git command ok", "dir", dir, "args", args)
	return nil
}

func (c *Client) CreateDetachedWorktree(ctx context.Context, repoPath, branch, sha string) (string, func(), error) {
	c.logger.Debug("creating detached worktree", "repoPath", repoPath, "branch", branch, "sha", sha)

	// Honour $TMPDIR for snap confinement.
	wtPath, err := os.MkdirTemp("", "watchtower-prepare-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("mkdirtemp: %w", err)
	}
	// MkdirTemp created the directory, but `git worktree add` requires
	// the target path not to exist. Remove it; git will recreate.
	if err := os.Remove(wtPath); err != nil {
		_ = os.RemoveAll(wtPath)
		return "", func() {}, fmt.Errorf("remove tmp slot: %w", err)
	}

	if err := c.runGit(ctx, repoPath, "worktree", "add", "-b", branch, wtPath, sha); err != nil {
		_ = os.RemoveAll(wtPath)
		return "", func() {}, fmt.Errorf("git worktree add: %w", err)
	}

	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := c.runGit(cctx, repoPath, "worktree", "remove", "--force", wtPath); err != nil {
				c.logger.Warn("worktree remove failed", "path", wtPath, "err", err)
			}
			if err := c.runGit(cctx, repoPath, "branch", "-D", branch); err != nil {
				c.logger.Debug("branch -D failed (may already be gone)", "branch", branch, "err", err)
			}
			if err := c.runGit(cctx, repoPath, "worktree", "prune", "--expire", "now"); err != nil {
				c.logger.Debug("worktree prune failed", "err", err)
			}
			if err := os.RemoveAll(wtPath); err != nil {
				c.logger.Warn("removeall tempdir failed", "path", wtPath, "err", err)
			}
		})
	}
	return wtPath, cleanup, nil
}

func (c *Client) ForceAddAll(ctx context.Context, worktreePath string) error {
	c.logger.Debug("force-staging all", "path", worktreePath)
	return c.runGit(ctx, worktreePath, "add", "-f", "-A")
}
