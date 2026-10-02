// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

// LaunchpadForge implements Forge for Launchpad repositories.
type LaunchpadForge struct {
	client *lp.Client
}

// NewLaunchpadForge creates a new Launchpad forge client.
func NewLaunchpadForge(client *lp.Client) *LaunchpadForge {
	return &LaunchpadForge{client: client}
}

func (l *LaunchpadForge) Type() ForgeType {
	return ForgeLaunchpad
}

func (l *LaunchpadForge) ListMergeRequests(ctx context.Context, repo string, opts ListMergeRequestsOpts) ([]MergeRequest, error) {
	statuses := lpMergeStatuses(opts.State)

	mps, err := l.client.GetProjectMergeProposals(ctx, repo, statuses...)
	if err != nil {
		return nil, fmt.Errorf("listing LP merge proposals for %s: %w", repo, err)
	}

	var result []MergeRequest
	for _, mp := range mps {
		mr := lpMPToMergeRequest(repo, &mp)
		if opts.Author != "" && mr.Author != opts.Author {
			continue
		}
		result = append(result, mr)
	}

	return result, nil
}

func (l *LaunchpadForge) GetMergeRequest(ctx context.Context, repo string, id string) (*MergeRequest, error) {
	// id is the self_link for LP merge proposals.
	mp, err := l.client.GetMergeProposal(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("getting LP merge proposal %s: %w", id, err)
	}

	mr := lpMPToMergeRequest(repo, &mp)
	if comments, err := l.client.GetMergeProposalComments(ctx, mp.AllCommentsCollectionLink); err == nil {
		for _, comment := range comments {
			body := strings.TrimSpace(comment.Content)
			if body == "" {
				continue
			}
			mr.Comments = append(mr.Comments, ReviewComment{
				Kind:      ReviewCommentGeneral,
				Author:    lpExtractName(comment.AuthorLink),
				Body:      body,
				URL:       comment.WebLink,
				CreatedAt: lpTime(comment.DateCreated),
				UpdatedAt: lpTime(comment.DateLastEdited),
			})
		}
	}
	if diff, err := l.client.GetPreviewDiff(ctx, mp.PreviewDiffLink); err == nil {
		if diffText, err := l.client.GetPreviewDiffText(ctx, diff.DiffTextLink); err == nil {
			mr.DiffText = diffText
		}
	}
	sort.Slice(mr.Comments, func(i, j int) bool {
		return mr.Comments[i].CreatedAt.Before(mr.Comments[j].CreatedAt)
	})
	return &mr, nil
}

func (l *LaunchpadForge) ListCommits(ctx context.Context, repo string, opts ListCommitsOpts) ([]Commit, error) {
	// Launchpad doesn't have a direct commit listing API for projects.
	// Commits are accessed through git repositories.
	// Return empty for now — this would require knowing the specific git repo.
	return nil, fmt.Errorf("ListCommits not supported for Launchpad projects directly; use git repository refs")
}

func lpMPToMergeRequest(repo string, mp *lp.MergeProposal) MergeRequest {
	mr := MergeRequest{
		Forge:       ForgeLaunchpad,
		Repo:        repo,
		ID:          mp.SelfLink,
		Title:       lpMPTitle(mp),
		Description: mp.Description,
		URL:         mp.WebLink,
	}

	// Extract author from registrant_link: ".../~username" -> "username"
	mr.Author = lpExtractName(mp.RegistrantLink)

	// Git merge proposals.
	if mp.SourceGitPath != "" {
		mr.SourceBranch = mp.SourceGitPath
	}
	if mp.TargetGitPath != "" {
		mr.TargetBranch = mp.TargetGitPath
	}

	// Bazaar merge proposals.
	if mp.SourceBranchLink != "" && mr.SourceBranch == "" {
		mr.SourceBranch = mp.SourceBranchLink
	}
	if mp.TargetBranchLink != "" && mr.TargetBranch == "" {
		mr.TargetBranch = mp.TargetBranchLink
	}

	if mp.DateCreated != nil {
		mr.CreatedAt = mp.DateCreated.Time
	}
	if mp.DateReviewRequested != nil {
		mr.UpdatedAt = mp.DateReviewRequested.Time
	}
	if mp.DateMerged != nil {
		mr.UpdatedAt = mp.DateMerged.Time
	}

	mr.State = lpMergeState(mp.QueueStatus)
	mr.ReviewState = lpReviewState(mp.QueueStatus)

	return mr
}

// lpMPTitle returns a title for a merge proposal.
// LP merge proposals don't have a dedicated title field, so we derive one.
func lpMPTitle(mp *lp.MergeProposal) string {
	if mp.CommitMessage != "" {
		// Use first line of commit message.
		for i, c := range mp.CommitMessage {
			if c == '\n' {
				return mp.CommitMessage[:i]
			}
		}
		return mp.CommitMessage
	}
	if mp.Description != "" {
		for i, c := range mp.Description {
			if c == '\n' {
				return mp.Description[:i]
			}
		}
		if len(mp.Description) > 80 {
			return mp.Description[:80] + "..."
		}
		return mp.Description
	}
	// Fall back to source branch name.
	if mp.SourceGitPath != "" {
		return mp.SourceGitPath
	}
	return "(untitled)"
}

// lpExtractName extracts the username from a LP person link.
// "https://api.launchpad.net/devel/~username" -> "username"
func lpExtractName(link string) string {
	if link == "" {
		return ""
	}
	// Find last /~ segment.
	for i := len(link) - 1; i >= 0; i-- {
		if link[i] == '~' {
			return link[i+1:]
		}
	}
	// Fall back: take everything after the last /.
	for i := len(link) - 1; i >= 0; i-- {
		if link[i] == '/' {
			return link[i+1:]
		}
	}
	return link
}

func lpTime(value *lp.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return value.Time
}

// lpMergeState converts a LP queue_status to our MergeState.
func lpMergeState(status string) MergeState {
	switch status {
	case "Work in progress":
		return MergeStateWIP
	case "Needs review":
		return MergeStateOpen
	case "Approved", "Queued":
		return MergeStateOpen
	case "Merged":
		return MergeStateMerged
	case "Rejected":
		return MergeStateClosed
	case "Superseded":
		return MergeStateClosed
	case "Code failed to merge":
		return MergeStateOpen
	default:
		return MergeStateOpen
	}
}

// lpReviewState converts a LP queue_status to our ReviewState.
func lpReviewState(status string) ReviewState {
	switch status {
	case "Approved", "Queued", "Merged":
		return ReviewStateApproved
	case "Rejected":
		return ReviewStateRejected
	case "Needs review", "Code failed to merge":
		return ReviewStatePending
	case "Work in progress":
		return ReviewStatePending
	default:
		return ReviewStatePending
	}
}

// lpMergeStatuses converts our MergeState to LP status strings.
func lpMergeStatuses(state MergeState) []string {
	switch state {
	case MergeStateOpen:
		return []string{"Needs review", "Approved", "Code failed to merge"}
	case MergeStateWIP:
		return []string{"Work in progress"}
	case MergeStateMerged:
		return []string{"Merged"}
	case MergeStateClosed:
		return []string{"Rejected", "Superseded"}
	default:
		return nil // all
	}
}
