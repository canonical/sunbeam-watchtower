package review

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"time"

	port "github.com/canonical/sunbeam-watchtower/internal/core/port"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

// ProjectForge pairs a Forge client with the project identifier it expects.
type ProjectForge struct {
	Forge     port.Forge
	ProjectID string // e.g. "owner/repo" for GitHub, project path for Gerrit/LP
}

// ListOptions controls filtering for List.
type ListOptions struct {
	Projects []string          // filter to these config project names (empty = all)
	Forges   []forge.ForgeType // filter to these forge types (empty = all)
	State    forge.MergeState
	Author   string
	Since    string // ISO 8601 date — filter MRs updated since this date
}

// ProjectResult holds merge requests from one project, or an error.
type ProjectResult struct {
	ProjectName   string               `json:"project_name" yaml:"project_name"`
	MergeRequests []forge.MergeRequest `json:"merge_requests" yaml:"merge_requests"`
	Err           error                `json:"-" yaml:"-"`
}

// Service aggregates reviews across multiple forges.
type Service struct {
	projects map[string]ProjectForge
	logger   *slog.Logger
}

// NewService creates a review service with the given project-to-forge mappings.
func NewService(projects map[string]ProjectForge, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Service{projects: projects, logger: logger}
}

// Get returns a single merge request by project name and ID.
func (s *Service) Get(ctx context.Context, project string, id string) (*forge.MergeRequest, error) {
	pf, ok := s.projects[project]
	if !ok {
		return nil, fmt.Errorf("unknown project %q", project)
	}

	s.logger.Debug("getting merge request", "project", project, "id", id)

	mr, err := pf.Forge.GetMergeRequest(ctx, pf.ProjectID, id)
	if err != nil {
		return nil, err
	}
	mr.Repo = project
	return mr, nil
}

// List returns merge requests across all configured projects, applying filters.
// Per-project errors are collected but do not stop aggregation (graceful degradation).
func (s *Service) List(ctx context.Context, opts ListOptions) ([]forge.MergeRequest, []ProjectResult, error) {
	projFilter := make(map[string]bool, len(opts.Projects))
	for _, p := range opts.Projects {
		projFilter[p] = true
	}
	forgeFilter := make(map[forge.ForgeType]bool, len(opts.Forges))
	for _, f := range opts.Forges {
		forgeFilter[f] = true
	}

	var sinceTime time.Time
	if opts.Since != "" {
		t, err := time.Parse(time.RFC3339, opts.Since)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid since value %q: %w", opts.Since, err)
		}
		sinceTime = t
	}

	s.logger.Debug("listing merge requests",
		"project_count", len(s.projects),
		"projects_filter", opts.Projects,
		"forges_filter", opts.Forges,
		"state", opts.State,
		"author", opts.Author,
		"since", opts.Since,
	)

	var results []ProjectResult
	var all []forge.MergeRequest

	for name, pf := range s.projects {
		if len(projFilter) > 0 && !projFilter[name] {
			s.logger.Debug("skipping project (filtered)", "project", name)
			continue
		}
		if len(forgeFilter) > 0 && !forgeFilter[pf.Forge.Type()] {
			s.logger.Debug("skipping project (filtered)", "project", name, "forge", pf.Forge.Type())
			continue
		}

		s.logger.Debug("querying project for reviews", "project", name)

		mrs, err := pf.Forge.ListMergeRequests(ctx, pf.ProjectID, forge.ListMergeRequestsOpts{
			State:  opts.State,
			Author: opts.Author,
		})

		result := ProjectResult{ProjectName: name}
		if err != nil {
			result.Err = fmt.Errorf("%s: %w", name, err)
			s.logger.Warn("project query failed", "project", name, "error", err)
		} else {
			for i := range mrs {
				mrs[i].Repo = name
			}
			if !sinceTime.IsZero() {
				filtered := mrs[:0]
				for _, mr := range mrs {
					if !mr.UpdatedAt.Before(sinceTime) {
						filtered = append(filtered, mr)
					}
				}
				mrs = filtered
			}
			result.MergeRequests = mrs
			all = append(all, mrs...)
			s.logger.Debug("project reviews fetched", "project", name, "count", len(mrs))
		}
		results = append(results, result)
	}

	// Sort by UpdatedAt descending.
	sort.Slice(all, func(i, j int) bool {
		return all[i].UpdatedAt.After(all[j].UpdatedAt)
	})

	s.logger.Debug("reviews aggregated", "total_count", len(all))

	return all, results, nil
}
