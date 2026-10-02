// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package port

import (
	"context"

	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

// CommitSource can list commits for a single project.
type CommitSource interface {
	ForgeType() forge.ForgeType
	ListCommits(ctx context.Context, opts forge.ListCommitsOpts) ([]forge.Commit, error)
	ListMRCommits(ctx context.Context) ([]forge.Commit, error)
	ListBranches(ctx context.Context) ([]string, error)
}
