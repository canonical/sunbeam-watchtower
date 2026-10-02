package bug

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"

	port "github.com/canonical/sunbeam-watchtower/internal/core/port"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

// ErrBugNotFound marks an identifier that no configured tracker could resolve.
var ErrBugNotFound = errors.New("bug not found")

// ProjectBugTracker pairs a BugTracker with the project identifier it expects.
type ProjectBugTracker struct {
	Tracker   port.BugTracker
	ProjectID string // e.g. LP project name
}

// ProjectBinding associates one tracker query with one configured Watchtower project.
type ProjectBinding struct {
	ProjectName   string
	Group         string
	CommonProject string
}

// ListOptions controls filtering for List.
type ListOptions struct {
	Projects   []string // filter to these config project names (empty = all)
	Status     []string
	Importance []string
	Assignee   string
	Tags       []string
	Since      string // ISO 8601 date — filter tasks created/modified since this date
	Merge      bool
	Limit      int
}

// ProjectResult holds bug tasks from one query, or an error.
type ProjectResult struct {
	ProjectName string
	BugTasks    []forge.BugTask
	Err         error
}

// Service aggregates bug tasks across multiple bug trackers.
type Service struct {
	// keyed by dedup key "forge:projectID" to avoid querying the same tracker project twice
	trackers map[string]ProjectBugTracker
	// maps dedup key → configured project bindings for that tracker project
	bindings map[string][]ProjectBinding
	logger   *slog.Logger
}

// NewService creates a bug service.
// trackers maps dedup key to the tracker, bindings maps dedup key to configured project bindings.
func NewService(trackers map[string]ProjectBugTracker, bindings map[string][]ProjectBinding, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Service{trackers: trackers, bindings: bindings, logger: logger}
}

// Get fetches a single bug by ID. It tries each configured tracker until one succeeds.
func (s *Service) Get(ctx context.Context, id string) (*forge.Bug, error) {
	s.logger.Debug("getting bug", "id", id)

	// Try each unique tracker (deduplicated).
	seen := make(map[port.BugTracker]bool)
	for _, pt := range s.trackers {
		if seen[pt.Tracker] {
			continue
		}
		seen[pt.Tracker] = true

		bug, err := pt.Tracker.GetBug(ctx, id)
		if err == nil {
			bug.Tasks = s.expandTaskBindings(bug.Forge, bug.Tasks)
			return bug, nil
		}
	}
	if len(seen) == 0 {
		return nil, fmt.Errorf("no bug trackers configured")
	}
	return nil, fmt.Errorf("%w: %s", ErrBugNotFound, id)
}

func (s *Service) expandTaskBindings(forgeType forge.ForgeType, tasks []forge.BugTask) []forge.BugTask {
	expanded := make([]forge.BugTask, 0, len(tasks))
	for _, task := range tasks {
		key := strings.ToLower(forgeType.String()) + ":" + task.Project
		bindings := s.bindings[key]
		if len(bindings) == 0 {
			expanded = append(expanded, task)
			continue
		}
		for _, binding := range bindings {
			scopedTask := task
			scopedTask.Project = binding.ProjectName
			expanded = append(expanded, scopedTask)
		}
	}
	return expanded
}

// List returns bug tasks across all configured trackers, applying filters.
// Per-tracker errors are collected but do not stop aggregation (graceful degradation).
func (s *Service) List(ctx context.Context, opts ListOptions) ([]forge.BugTask, []ProjectResult, error) {
	projFilter := make(map[string]bool, len(opts.Projects))
	for _, p := range opts.Projects {
		projFilter[p] = true
	}

	s.logger.Debug("listing bugs",
		"tracker_count", len(s.trackers),
		"projects_filter", opts.Projects,
		"status_filter", opts.Status,
		"importance_filter", opts.Importance,
		"assignee", opts.Assignee,
		"tags", opts.Tags,
	)

	var results []ProjectResult
	var all []forge.BugTask
	merged := make(map[string]mergedBugTask)

	for key, pt := range s.trackers {
		projectBindings := s.bindings[key]

		// If project filter is set, check if any associated project matches.
		if len(projFilter) > 0 {
			var matched bool
			for _, binding := range projectBindings {
				if projFilter[binding.ProjectName] {
					matched = true
					break
				}
			}
			if !matched {
				s.logger.Debug("skipping tracker (filtered)", "tracker", key)
				continue
			}
		}

		s.logger.Debug("querying tracker", "tracker", key, "projects", projectBindings)

		tasks, err := pt.Tracker.ListBugTasks(ctx, pt.ProjectID, forge.ListBugTasksOpts{
			Status:        opts.Status,
			Importance:    opts.Importance,
			Assignee:      opts.Assignee,
			Tags:          opts.Tags,
			CreatedSince:  opts.Since,
			ModifiedSince: opts.Since,
		})

		if err != nil {
			s.logger.Warn("tracker query failed", "tracker", key, "error", err)
			for _, binding := range projectBindings {
				results = append(results, ProjectResult{
					ProjectName: binding.ProjectName,
					Err:         fmt.Errorf("%s: %w", binding.ProjectName, err),
				})
			}
			continue
		}

		// Expand results to all associated watchtower projects.
		for _, binding := range projectBindings {
			if len(projFilter) > 0 && !projFilter[binding.ProjectName] {
				continue
			}
			expanded := make([]forge.BugTask, len(tasks))
			for i, t := range tasks {
				t.Project = binding.ProjectName
				expanded[i] = t
				if opts.Merge {
					groupKey, displayProject := binding.mergeGroupKey(pt.ProjectID)
					mergeKey := fmt.Sprintf("%s:%s:%s", groupKey, t.Forge.String(), t.BugID)
					merged[mergeKey] = mergeTask(merged[mergeKey], mergeCandidate{
						task:           t,
						displayProject: displayProject,
						trackerProject: pt.ProjectID,
						commonProject:  binding.commonProjectOrTracker(pt.ProjectID),
					})
				}
			}
			results = append(results, ProjectResult{
				ProjectName: binding.ProjectName,
				BugTasks:    expanded,
			})
			all = append(all, expanded...)
			s.logger.Debug("tracker bugs fetched", "tracker", key, "project", binding.ProjectName, "count", len(expanded))
		}
	}

	if opts.Merge {
		all = all[:0]
		for _, item := range merged {
			all = append(all, item.task)
		}
	}

	// Sort by UpdatedAt descending.
	sort.Slice(all, func(i, j int) bool {
		return all[i].UpdatedAt.After(all[j].UpdatedAt)
	})

	if opts.Limit > 0 && len(all) > opts.Limit {
		all = all[:opts.Limit]
	}

	s.logger.Debug("bugs aggregated", "total_count", len(all))

	return all, results, nil
}

type mergeCandidate struct {
	task           forge.BugTask
	displayProject string
	trackerProject string
	commonProject  string
}

type mergedBugTask struct {
	task forge.BugTask
}

func mergeTask(existing mergedBugTask, candidate mergeCandidate) mergedBugTask {
	candidate.task.Project = candidate.displayProject
	if existing.task.BugID == "" {
		return mergedBugTask{task: candidate.task}
	}

	existingPreferred := existing.task.TargetName == candidate.commonProject
	candidatePreferred := candidate.trackerProject == candidate.commonProject || candidate.task.TargetName == candidate.commonProject

	if candidatePreferred && !existingPreferred {
		return mergedBugTask{task: candidate.task}
	}
	if candidatePreferred == existingPreferred && candidate.task.UpdatedAt.After(existing.task.UpdatedAt) {
		return mergedBugTask{task: candidate.task}
	}
	return existing
}

func (b ProjectBinding) mergeGroupKey(trackerProject string) (string, string) {
	if b.Group != "" {
		return "group:" + b.Group, b.commonProjectOrTracker(trackerProject)
	}
	return "tracker:" + trackerProject, trackerProject
}

func (b ProjectBinding) commonProjectOrTracker(trackerProject string) string {
	if b.CommonProject != "" {
		return b.CommonProject
	}
	return trackerProject
}
