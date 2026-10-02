// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package commit

import (
	"context"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

// Ensure implementations satisfy CommitSource.
var (
	_ port.CommitSource = (*ForgeCommitSource)(nil)
	_ port.CommitSource = (*CachedGitSource)(nil)
)

// ForgeCommitSource wraps an existing ForgeClient for backward compatibility.
type ForgeCommitSource struct {
	Forge     ForgeClient
	ProjectID string
}

// ListCommits delegates to the underlying Forge.
func (f *ForgeCommitSource) ListCommits(ctx context.Context, opts forge.ListCommitsOpts) ([]forge.Commit, error) {
	return f.Forge.ListCommits(ctx, f.ProjectID, opts)
}

// ListMRCommits is not supported via the forge API path.
func (f *ForgeCommitSource) ListMRCommits(_ context.Context) ([]forge.Commit, error) {
	return nil, nil
}

// ListBranches is not supported via the forge API path.
func (f *ForgeCommitSource) ListBranches(_ context.Context) ([]string, error) {
	return nil, nil
}

// ForgeType returns the forge used by the underlying source.
func (f *ForgeCommitSource) ForgeType() forge.ForgeType {
	return f.Forge.Type()
}

// CachedGitSource reads commits from a local git cache.
type CachedGitSource struct {
	Cache     port.GitRepoCache
	CloneURL  string
	Type      forge.ForgeType
	CommitURL func(string) string
}

// ListCommits reads commits from the local git cache and populates commit URLs.
func (c *CachedGitSource) ListCommits(ctx context.Context, opts forge.ListCommitsOpts) ([]forge.Commit, error) {
	commits, err := c.Cache.ListCommits(ctx, c.CloneURL, opts)
	if err != nil {
		return nil, err
	}

	for i := range commits {
		commits[i].Forge = c.Type
		if c.CommitURL != nil {
			commits[i].URL = c.CommitURL(commits[i].SHA)
		}
	}

	return commits, nil
}

// ListMRCommits reads merge request head commits from the cache.
func (c *CachedGitSource) ListMRCommits(ctx context.Context) ([]forge.Commit, error) {
	commits, err := c.Cache.ListMRCommits(ctx, c.CloneURL)
	if err != nil {
		return nil, err
	}

	for i := range commits {
		commits[i].Forge = c.Type
	}

	return commits, nil
}

// ListBranches returns branch names from the local git cache.
func (c *CachedGitSource) ListBranches(ctx context.Context) ([]string, error) {
	return c.Cache.ListBranches(ctx, c.CloneURL)
}

// ForgeType returns the forge used by the cached source.
func (c *CachedGitSource) ForgeType() forge.ForgeType {
	return c.Type
}
