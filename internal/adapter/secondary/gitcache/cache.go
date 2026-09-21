// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package gitcache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	dto "github.com/gboutry/sunbeam-watchtower/pkg/dto/v1"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	forge "github.com/gboutry/sunbeam-watchtower/pkg/forge/v1"
)

const mrMetadataFile = ".watchtower-mrs.json"

// Cache implements port.GitRepoCache using local bare git clones.
type Cache struct {
	baseDir string
	logger  *slog.Logger
}

// NewCache creates a new git cache rooted at baseDir. If logger is nil, a no-op logger is used.
func NewCache(baseDir string, logger *slog.Logger) *Cache {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Cache{baseDir: baseDir, logger: logger}
}

// CacheDir returns the base directory for cached repos.
func (c *Cache) CacheDir() string {
	return c.baseDir
}

// repoPath converts a clone URL to a local filesystem path under baseDir.
// e.g. "https://github.com/org/repo.git" → "<baseDir>/github.com/org/repo.git"
func (c *Cache) repoPath(cloneURL string) (string, error) {
	u, err := url.Parse(cloneURL)
	if err != nil {
		return "", fmt.Errorf("parsing clone URL %q: %w", cloneURL, err)
	}

	host := u.Host
	if host == "" {
		if u.Scheme == "file" {
			// file:// URLs use the path directly.
			host = "localhost"
		} else {
			return "", fmt.Errorf("clone URL %q has no host", cloneURL)
		}
	}

	// Strip port if present.
	if idx := strings.LastIndex(host, ":"); idx > 0 {
		host = host[:idx]
	}

	p := strings.TrimPrefix(u.Path, "/")
	if p == "" {
		return "", fmt.Errorf("clone URL %q has no path", cloneURL)
	}

	// Ensure path ends with .git.
	if !strings.HasSuffix(p, ".git") {
		p += ".git"
	}

	resolved := filepath.Join(c.baseDir, host, p)
	c.logger.Debug("resolved repo path", "cloneURL", cloneURL, "path", resolved)
	return resolved, nil
}

// EnsureRepo clones the repository if missing, or fetches if it already exists.
func (c *Cache) EnsureRepo(ctx context.Context, cloneURL string, opts *dto.SyncOptions) (string, error) {
	path, err := c.repoPath(cloneURL)
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(path); err == nil {
		// Repo exists, fetch.
		c.logger.Debug("fetching existing cached repo", "url", cloneURL, "path", path)
		if fetchErr := c.fetchRepo(ctx, path, opts); fetchErr != nil {
			return "", fetchErr
		}
		return path, nil
	}

	c.logger.Info("cloning repo into cache", "url", cloneURL, "path", path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("creating cache directory: %w", err)
	}

	_, err = git.PlainCloneContext(ctx, path, true, &git.CloneOptions{
		URL: cloneURL,
	})
	if err != nil {
		return "", fmt.Errorf("cloning %s: %w", cloneURL, err)
	}

	// Fetch the canonical branch and tag refspecs after cloning too. In
	// particular, default tag-following does not guarantee that a revision tag
	// added without a branch update is present in the cache.
	if fetchErr := c.fetchRepo(ctx, path, opts); fetchErr != nil {
		return "", fetchErr
	}

	return path, nil
}

// Fetch updates an existing cached repository from origin.
func (c *Cache) Fetch(ctx context.Context, cloneURL string, opts *dto.SyncOptions) error {
	path, err := c.repoPath(cloneURL)
	if err != nil {
		return err
	}
	return c.fetchRepo(ctx, path, opts)
}

func (c *Cache) fetchRepo(ctx context.Context, path string, opts *dto.SyncOptions) error {
	repo, err := git.PlainOpen(path)
	if err != nil {
		return fmt.Errorf("opening repo at %s: %w", path, err)
	}

	// Default fetch (branches).
	err = repo.FetchContext(ctx, &git.FetchOptions{})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("fetching: %w", err)
	}

	// Metadata discovery reads the bare cache's HEAD tree. Keep local branch
	// refs current as well as remote-tracking refs so HEAD follows upstream.
	err = repo.FetchContext(ctx, &git.FetchOptions{
		RefSpecs: []gitconfig.RefSpec{
			gitconfig.RefSpec("+refs/heads/*:refs/heads/*"),
		},
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("fetching branch refs: %w", err)
	}

	// Snap build revisions are represented by repository tags (for example,
	// rev1101). Fetch tags explicitly instead of relying on tag-following.
	err = repo.FetchContext(ctx, &git.FetchOptions{
		RefSpecs: []gitconfig.RefSpec{
			gitconfig.RefSpec("+refs/tags/*:refs/tags/*"),
		},
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("fetching tags: %w", err)
	}

	// Fetch extra refspecs if provided.
	if opts != nil && len(opts.ExtraRefSpecs) > 0 {
		refSpecs := make([]gitconfig.RefSpec, len(opts.ExtraRefSpecs))
		for i, rs := range opts.ExtraRefSpecs {
			refSpecs[i] = gitconfig.RefSpec(rs)
		}
		c.logger.Debug("fetching extra refspecs", "path", path, "refspecs", opts.ExtraRefSpecs)
		err = repo.FetchContext(ctx, &git.FetchOptions{
			RefSpecs: refSpecs,
		})
		if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
			return fmt.Errorf("fetching extra refspecs: %w", err)
		}
	}

	return nil
}

// ListCommits reads commit history from a cached repository.
func (c *Cache) ListCommits(ctx context.Context, cloneURL string, opts forge.ListCommitsOpts) ([]forge.Commit, error) {
	c.logger.Debug("listing commits from cache", "cloneURL", cloneURL, "branch", opts.Branch)
	path, err := c.repoPath(cloneURL)
	if err != nil {
		return nil, err
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		return nil, fmt.Errorf("opening repo at %s: %w", path, err)
	}

	// Resolve an explicit revision (normally refs/tags/revN for release
	// provenance) or fall back to the requested remote branch.
	var from plumbing.Hash
	if opts.Revision != "" {
		revision := plumbing.Revision(opts.Revision)
		resolved, resolveErr := repo.ResolveRevision(revision)
		if resolveErr != nil {
			return nil, fmt.Errorf("resolving revision %s: %w", opts.Revision, resolveErr)
		}
		from = *resolved
	}

	branch := opts.Branch
	if branch == "" {
		branch = "main"
	}
	if from.IsZero() {
		refName := plumbing.NewRemoteReferenceName("origin", branch)
		ref, refErr := repo.Reference(refName, true)
		if refErr != nil {
			// Try "master" as fallback if "main" was the default.
			if opts.Branch == "" {
				refName = plumbing.NewRemoteReferenceName("origin", "master")
				ref, refErr = repo.Reference(refName, true)
			}
			if refErr != nil {
				return nil, fmt.Errorf("resolving ref %s: %w", refName, refErr)
			}
		}
		from = ref.Hash()
	}

	logOpts := &git.LogOptions{
		From:  from,
		Order: git.LogOrderCommitterTime,
	}
	if opts.Since != nil {
		logOpts.Since = opts.Since
	}

	iter, err := repo.Log(logOpts)
	if err != nil {
		return nil, fmt.Errorf("reading log: %w", err)
	}
	defer iter.Close()

	var result []forge.Commit
	err = iter.ForEach(func(co *object.Commit) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		author := co.Author.Name
		if opts.Author != "" && author != opts.Author {
			return nil
		}

		result = append(result, forge.Commit{
			SHA:     co.Hash.String(),
			Message: co.Message,
			Author:  author,
			Date:    co.Author.When,
			BugRefs: forge.ExtractBugRefs(co.Message),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("iterating commits: %w", err)
	}

	c.logger.Debug("commits read from cache", "cloneURL", cloneURL, "count", len(result))
	return result, nil
}

// StoreMRMetadata writes merge request metadata as a sidecar JSON file.
func (c *Cache) StoreMRMetadata(cloneURL string, mrs []dto.MRMetadata) error {
	path, err := c.repoPath(cloneURL)
	if err != nil {
		return err
	}

	metaPath := filepath.Join(path, mrMetadataFile)
	c.logger.Debug("storing MR metadata", "cloneURL", cloneURL, "count", len(mrs), "path", metaPath)

	data, err := json.MarshalIndent(mrs, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling MR metadata: %w", err)
	}

	return os.WriteFile(metaPath, data, 0o644)
}

// LoadMRMetadata reads merge request metadata from the sidecar JSON file.
func (c *Cache) LoadMRMetadata(cloneURL string) ([]dto.MRMetadata, error) {
	path, err := c.repoPath(cloneURL)
	if err != nil {
		return nil, err
	}

	metaPath := filepath.Join(path, mrMetadataFile)
	data, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading MR metadata: %w", err)
	}

	var mrs []dto.MRMetadata
	if err := json.Unmarshal(data, &mrs); err != nil {
		return nil, fmt.Errorf("parsing MR metadata: %w", err)
	}

	c.logger.Debug("loaded MR metadata", "cloneURL", cloneURL, "count", len(mrs))
	return mrs, nil
}

// ListMRCommits reads the head commit for each cached merge request ref.
func (c *Cache) ListMRCommits(ctx context.Context, cloneURL string) ([]forge.Commit, error) {
	c.logger.Debug("listing MR commits", "cloneURL", cloneURL)

	mrs, err := c.LoadMRMetadata(cloneURL)
	if err != nil {
		return nil, err
	}
	if len(mrs) == 0 {
		return nil, nil
	}

	path, err := c.repoPath(cloneURL)
	if err != nil {
		return nil, err
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		return nil, fmt.Errorf("opening repo at %s: %w", path, err)
	}

	var result []forge.Commit
	for _, mr := range mrs {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if mr.GitRef == "" {
			// Try resolving by HeadSHA directly.
			if mr.HeadSHA == "" {
				continue
			}
			co, err := repo.CommitObject(plumbing.NewHash(mr.HeadSHA))
			if err != nil {
				c.logger.Debug("MR commit not found by SHA", "mr_id", mr.ID, "sha", mr.HeadSHA, "error", err)
				continue
			}
			result = append(result, commitFromObject(co, &mr))
			continue
		}

		ref, err := repo.Reference(plumbing.ReferenceName(mr.GitRef), true)
		if err != nil {
			c.logger.Debug("MR ref not found", "mr_id", mr.ID, "ref", mr.GitRef, "error", err)
			continue
		}

		co, err := repo.CommitObject(ref.Hash())
		if err != nil {
			c.logger.Debug("MR commit not found", "mr_id", mr.ID, "ref", mr.GitRef, "error", err)
			continue
		}

		result = append(result, commitFromObject(co, &mr))
	}

	c.logger.Debug("MR commits read", "cloneURL", cloneURL, "count", len(result))
	return result, nil
}

func commitFromObject(co *object.Commit, mr *dto.MRMetadata) forge.Commit {
	return forge.Commit{
		SHA:     co.Hash.String(),
		Message: co.Message,
		Author:  co.Author.Name,
		Date:    co.Author.When,
		BugRefs: forge.ExtractBugRefs(co.Message),
		MergeRequest: &forge.CommitMergeRequest{
			ID:    mr.ID,
			State: mr.State,
			URL:   mr.URL,
		},
	}
}

// ListBranches returns the branch names available in a cached repository.
func (c *Cache) ListBranches(_ context.Context, cloneURL string) ([]string, error) {
	c.logger.Debug("listing branches from cache", "cloneURL", cloneURL)
	path, err := c.repoPath(cloneURL)
	if err != nil {
		return nil, err
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		return nil, fmt.Errorf("opening repo at %s: %w", path, err)
	}

	refs, err := repo.References()
	if err != nil {
		return nil, fmt.Errorf("listing references: %w", err)
	}

	const prefix = "refs/remotes/origin/"
	var branches []string
	err = refs.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().String()
		if strings.HasPrefix(name, prefix) {
			branch := strings.TrimPrefix(name, prefix)
			if branch != "HEAD" {
				branches = append(branches, branch)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("iterating references: %w", err)
	}

	c.logger.Debug("branches found", "cloneURL", cloneURL, "count", len(branches))
	return branches, nil
}

// Remove deletes a single cached repository.
func (c *Cache) Remove(cloneURL string) error {
	c.logger.Debug("removing cached repo", "cloneURL", cloneURL)
	path, err := c.repoPath(cloneURL)
	if err != nil {
		return err
	}
	return os.RemoveAll(path)
}

// RemoveAll deletes all cached repositories.
func (c *Cache) RemoveAll() error {
	c.logger.Debug("removing all cached repos")
	return os.RemoveAll(c.baseDir)
}
