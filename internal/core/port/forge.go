// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package port

import (
	"context"

	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

// Forge is the unified interface for interacting with code forges.
type Forge interface {
	Type() forge.ForgeType
	ListMergeRequests(ctx context.Context, repo string, opts forge.ListMergeRequestsOpts) ([]forge.MergeRequest, error)
	GetMergeRequest(ctx context.Context, repo string, id string) (*forge.MergeRequest, error)
	ListCommits(ctx context.Context, repo string, opts forge.ListCommitsOpts) ([]forge.Commit, error)
}
