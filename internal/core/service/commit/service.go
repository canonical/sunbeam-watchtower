// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package commit

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sort"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

// ProjectForge pairs a Forge client with the project identifier it expects.
// Deprecated: Use ProjectSource instead.
type ProjectForge struct {
	Forge     ForgeClient
	ProjectID string // e.g. "owner/repo" for GitHub, project path for Gerrit/LP
}

// ForgeClient is the minimal interface needed from a forge for commit listing.
type ForgeClient interface {
	Type() forge.ForgeType
	ListCommits(ctx context.Context, repo string, opts forge.ListCommitsOpts) ([]forge.Commit, error)
}

// ListOptions controls filtering for List.
type ListOptions struct {
	Projects   []string          // filter to these config project names (empty = all)
	Forges     []forge.ForgeType // filter to these forge types (empty = all)
	Branch     string            // branch to list commits from (empty = default)
	Author     string            // filter by author
	BugID      string            // filter to commits referencing this bug ID
	IncludeMRs bool              // include commits from merge request refs
}

// ProjectResult holds commits from one project, or an error.
type ProjectResult struct {
	ProjectName string         `json:"project_name" yaml:"project_name"`
	Commits     []forge.Commit `json:"commits" yaml:"commits"`
	Err         error          `json:"-" yaml:"-"`
}

// Service aggregates commits across multiple forges.
type Service struct {
	projects map[string]port.CommitSource
	logger   *slog.Logger
}

// NewService creates a commit service from commit source mappings.
func NewService(projects map[string]port.CommitSource, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Service{projects: projects, logger: logger}
}

// NewServiceFromForges creates a commit service from legacy ProjectForge mappings.
func NewServiceFromForges(projects map[string]ProjectForge, logger *slog.Logger) *Service {
	sources := make(map[string]port.CommitSource, len(projects))
	for name, pf := range projects {
		sources[name] = &ForgeCommitSource{
			Forge:     pf.Forge,
			ProjectID: pf.ProjectID,
		}
	}
	return NewService(sources, logger)
}

// List returns commits across all configured projects, applying filters.
// Per-project errors are collected but do not stop aggregation (graceful degradation).
func (s *Service) List(ctx context.Context, opts ListOptions) ([]forge.Commit, []ProjectResult, error) {
	projFilter := make(map[string]bool, len(opts.Projects))
	for _, p := range opts.Projects {
		projFilter[p] = true
	}
	forgeFilter := make(map[forge.ForgeType]bool, len(opts.Forges))
	for _, f := range opts.Forges {
		forgeFilter[f] = true
	}

	s.logger.Debug("listing commits",
		"project_count", len(s.projects),
		"projects_filter", opts.Projects,
		"forges_filter", opts.Forges,
		"branch", opts.Branch,
		"author", opts.Author,
		"bug_id", opts.BugID,
	)

	var results []ProjectResult
	var all []forge.Commit

	for name, ps := range s.projects {
		if len(projFilter) > 0 && !projFilter[name] {
			s.logger.Debug("skipping project (filtered)", "project", name)
			continue
		}
		forgeType := ps.ForgeType()
		if len(forgeFilter) > 0 && !forgeFilter[forgeType] {
			s.logger.Debug("skipping project (filtered)", "project", name, "forge", forgeType)
			continue
		}

		s.logger.Debug("querying project", "project", name, "forge", forgeType)

		commits, err := ps.ListCommits(ctx, forge.ListCommitsOpts{
			Branch: opts.Branch,
			Author: opts.Author,
		})

		result := ProjectResult{ProjectName: name}
		if err != nil {
			result.Err = fmt.Errorf("%s: %w", name, err)
			s.logger.Warn("project query failed", "project", name, "error", err)
		} else {
			for i := range commits {
				commits[i].Repo = name
				// Commits from the branch are inherently merged.
				if commits[i].MergeRequest == nil {
					commits[i].MergeRequest = &forge.CommitMergeRequest{
						State: forge.MergeStateMerged,
					}
				}
			}

			// Include MR commits if requested, with deduplication.
			if opts.IncludeMRs {
				mrCommits, mrErr := ps.ListMRCommits(ctx)
				if mrErr != nil {
					s.logger.Warn("MR commit query failed", "project", name, "error", mrErr)
				} else if len(mrCommits) > 0 {
					// Build index of branch commits by SHA for annotation.
					branchIdx := make(map[string]int, len(commits))
					for i, c := range commits {
						branchIdx[c.SHA] = i
					}
					for i := range mrCommits {
						if idx, found := branchIdx[mrCommits[i].SHA]; found {
							// Commit is on the branch — annotate as Merged with MR link.
							if mrCommits[i].MergeRequest != nil {
								commits[idx].MergeRequest = &forge.CommitMergeRequest{
									ID:    mrCommits[i].MergeRequest.ID,
									State: forge.MergeStateMerged,
									URL:   mrCommits[i].MergeRequest.URL,
								}
							}
							continue
						}
						mrCommits[i].Repo = name
						commits = append(commits, mrCommits[i])
					}
					s.logger.Debug("MR commits included", "project", name, "mr_commit_count", len(mrCommits))
				}
			}

			if opts.BugID != "" {
				commits = filterByBugRef(commits, opts.BugID)
			}

			result.Commits = commits
			all = append(all, commits...)
			s.logger.Debug("project commits fetched", "project", name, "commit_count", len(commits))
		}
		results = append(results, result)
	}

	// Sort by Date descending (newest first).
	sort.Slice(all, func(i, j int) bool {
		return all[i].Date.After(all[j].Date)
	})

	s.logger.Debug("commits aggregated", "total_count", len(all))

	return all, results, nil
}

// filterByBugRef returns only commits whose BugRefs contain the given bug ID.
func filterByBugRef(commits []forge.Commit, bugID string) []forge.Commit {
	var filtered []forge.Commit
	for _, c := range commits {
		for _, ref := range c.BugRefs {
			if ref.ID == bugID {
				filtered = append(filtered, c)
				break
			}
		}
	}
	return filtered
}
