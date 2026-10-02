// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package port

import (
	"context"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

// RepoEnsurer materialises a local bare clone of a remote repository.
// Consumers that only need a repo path (e.g. to read a worktree snapshot)
// should depend on this narrow role interface rather than on GitRepoCache.
type RepoEnsurer interface {
	EnsureRepo(ctx context.Context, cloneURL string, opts *dto.SyncOptions) (path string, err error)
}

// GitRepoCache manages local bare git clones used for reading commit history.
type GitRepoCache interface {
	RepoEnsurer
	Fetch(ctx context.Context, cloneURL string, opts *dto.SyncOptions) error
	ListCommits(ctx context.Context, cloneURL string, opts forge.ListCommitsOpts) ([]forge.Commit, error)
	StoreMRMetadata(cloneURL string, mrs []dto.MRMetadata) error
	LoadMRMetadata(cloneURL string) ([]dto.MRMetadata, error)
	ListMRCommits(ctx context.Context, cloneURL string) ([]forge.Commit, error)
	ListBranches(ctx context.Context, cloneURL string) ([]string, error)
	Remove(cloneURL string) error
	RemoveAll() error
	CacheDir() string
}
