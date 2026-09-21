// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"fmt"
	"strconv"
	"time"

	lp "github.com/gboutry/sunbeam-watchtower/pkg/launchpad/v1"
)

// LaunchpadBugTracker implements BugTracker for Launchpad projects.
type LaunchpadBugTracker struct {
	client *lp.Client
}

// NewLaunchpadBugTracker creates a new Launchpad bug tracker.
func NewLaunchpadBugTracker(client *lp.Client) *LaunchpadBugTracker {
	return &LaunchpadBugTracker{client: client}
}

func (l *LaunchpadBugTracker) Type() ForgeType {
	return ForgeLaunchpad
}

func (l *LaunchpadBugTracker) GetBug(ctx context.Context, id string) (*Bug, error) {
	bugID, err := strconv.Atoi(id)
	if err != nil {
		return nil, fmt.Errorf("invalid LP bug ID %q: %w", id, err)
	}

	lpBug, err := l.client.GetBug(ctx, bugID)
	if err != nil {
		return nil, err
	}

	lpTasks, err := l.client.GetBugTasks(ctx, bugID)
	if err != nil {
		return nil, fmt.Errorf("fetching tasks for bug %d: %w", bugID, err)
	}

	lpMessages, err := l.client.GetBugMessages(ctx, lpBug.MessagesCollectionLink)
	if err != nil {
		return nil, fmt.Errorf("fetching comments for bug %d: %w", bugID, err)
	}

	b := &Bug{
		Forge:           ForgeLaunchpad,
		ID:              id,
		Title:           lpBug.Title,
		Description:     lpBug.Description,
		Owner:           lpExtractName(lpBug.OwnerLink),
		Tags:            lpBug.Tags,
		URL:             lpBug.WebLink,
		Private:         lpBug.Private,
		SecurityRelated: lpBug.SecurityRelated,
		InformationType: lpBug.InformationType,
		VisibilityKnown: true,
	}
	b.Links = appendBugLink(b.Links, "duplicate_of", lpBug.DuplicateOfLink)
	b.Links = appendBugLink(b.Links, "attachments", lpBug.AttachmentsCollectionLink)
	b.Links = appendBugLink(b.Links, "linked_branches", lpBug.LinkedBranchesCollectionLink)
	b.Links = appendBugLink(b.Links, "linked_merge_proposals", lpBug.LinkedMergeProposalsCollectionLink)
	b.Links = appendBugLink(b.Links, "cves", lpBug.CVEsCollectionLink)
	b.Links = appendBugLink(b.Links, "vulnerabilities", lpBug.VulnerabilitiesCollectionLink)
	if lpBug.DateCreated != nil {
		b.CreatedAt = lpBug.DateCreated.Time
	}
	if lpBug.DateLastUpdated != nil {
		b.UpdatedAt = lpBug.DateLastUpdated.Time
	}

	for _, t := range lpTasks {
		b.Tasks = append(b.Tasks, lpBugTaskToBugTask(&t))
	}
	for _, message := range lpMessages {
		if !message.Visible {
			continue
		}
		comment := BugComment{
			Author:  lpExtractName(message.OwnerLink),
			Subject: message.Subject,
			Body:    message.Content,
			URL:     message.WebLink,
		}
		if message.DateCreated != nil {
			comment.CreatedAt = message.DateCreated.Time
			comment.UpdatedAt = message.DateCreated.Time
		}
		if message.DateLastEdited != nil {
			comment.UpdatedAt = message.DateLastEdited.Time
		}
		b.Comments = append(b.Comments, comment)
	}

	return b, nil
}

func appendBugLink(links []BugLink, relation, rawURL string) []BugLink {
	if rawURL == "" {
		return links
	}
	return append(links, BugLink{Relation: relation, URL: rawURL})
}

func (l *LaunchpadBugTracker) ListBugTasks(ctx context.Context, project string, opts ListBugTasksOpts) ([]BugTask, error) {
	lpOpts := lp.BugTaskSearchOpts{
		Status:         opts.Status,
		Importance:     opts.Importance,
		Tags:           opts.Tags,
		CreatedSince:   opts.CreatedSince,
		ModifiedSince:  opts.ModifiedSince,
		OmitDuplicates: true,
	}
	if opts.Assignee != "" {
		lpOpts.Assignee = "https://api.launchpad.net/devel/~" + opts.Assignee
	}

	tasks, err := l.searchBugTasks(ctx, project, lpOpts)
	if err != nil {
		return nil, fmt.Errorf("searching LP bug tasks for %s: %w", project, err)
	}

	result := make([]BugTask, 0, len(tasks))
	for _, t := range tasks {
		result = append(result, lpBugTaskToBugTask(&t))
	}
	return result, nil
}

func (l *LaunchpadBugTracker) searchBugTasks(ctx context.Context, project string, opts lp.BugTaskSearchOpts) ([]lp.BugTask, error) {
	if opts.CreatedSince == "" || opts.ModifiedSince == "" {
		return l.client.SearchBugTasks(ctx, project, opts)
	}

	createdOnly := opts
	createdOnly.ModifiedSince = ""
	createdTasks, err := l.client.SearchBugTasks(ctx, project, createdOnly)
	if err != nil {
		return nil, err
	}

	modifiedOnly := opts
	modifiedOnly.CreatedSince = ""
	modifiedTasks, err := l.client.SearchBugTasks(ctx, project, modifiedOnly)
	if err != nil {
		return nil, err
	}

	merged := make(map[string]lp.BugTask, len(createdTasks)+len(modifiedTasks))
	for _, task := range createdTasks {
		merged[task.SelfLink] = task
	}
	for _, task := range modifiedTasks {
		merged[task.SelfLink] = task
	}

	result := make([]lp.BugTask, 0, len(merged))
	for _, task := range merged {
		result = append(result, task)
	}
	return result, nil
}

func (l *LaunchpadBugTracker) UpdateBugTaskStatus(ctx context.Context, taskSelfLink, status string) error {
	return l.client.UpdateBugTaskStatus(ctx, taskSelfLink, status)
}

func (l *LaunchpadBugTracker) AddBugTask(ctx context.Context, bugID int, seriesSelfLink string) error {
	return l.client.AddBugTask(ctx, bugID, seriesSelfLink)
}

func (l *LaunchpadBugTracker) GetProjectSeries(ctx context.Context, projectName string) ([]ProjectSeries, error) {
	col, err := l.client.GetProjectSeries(ctx, projectName)
	if err != nil {
		return nil, fmt.Errorf("fetching series for %s: %w", projectName, err)
	}
	result := make([]ProjectSeries, 0, len(col.Entries))
	for _, s := range col.Entries {
		result = append(result, ProjectSeries{
			Name:     s.Name,
			SelfLink: s.SelfLink,
			Active:   s.Active,
		})
	}
	return result, nil
}

func (l *LaunchpadBugTracker) GetProject(ctx context.Context, projectName string) (*Project, error) {
	p, err := l.client.GetProject(ctx, projectName)
	if err != nil {
		return nil, err
	}
	return &Project{
		Name:                 p.Name,
		SelfLink:             p.SelfLink,
		DevelopmentFocusLink: p.DevelopmentFocusLink,
	}, nil
}

func lpBugTaskToBugTask(t *lp.BugTask) BugTask {
	bt := BugTask{
		Forge:      ForgeLaunchpad,
		BugID:      lpExtractBugID(t.BugLink),
		Title:      t.Title,
		Status:     t.Status,
		Importance: t.Importance,
		Assignee:   lpExtractName(t.AssigneeLink),
		URL:        t.WebLink,
		SelfLink:   t.SelfLink,
		TargetName: t.BugTargetName,
		TargetLink: t.TargetLink,
	}
	if t.DateCreated != nil {
		bt.CreatedAt = t.DateCreated.Time
	}
	bt.UpdatedAt = lpBugTaskUpdatedAt(t)
	return bt
}

func lpBugTaskUpdatedAt(t *lp.BugTask) time.Time {
	if t == nil {
		return time.Time{}
	}
	candidates := []*lp.Time{
		t.DateCreated,
		t.DateAssigned,
		t.DateClosed,
		t.DateConfirmed,
		t.DateFixCommitted,
		t.DateFixReleased,
		t.DateInProgress,
		t.DateIncomplete,
		t.DateTriaged,
		t.DateLeftNew,
		t.DateLeftClosed,
	}
	var latest time.Time
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		if candidate.After(latest) {
			latest = candidate.Time
		}
	}
	return latest
}

// lpExtractBugID extracts the bug ID from a bug link.
// "https://api.launchpad.net/devel/bugs/12345" -> "12345"
func lpExtractBugID(link string) string {
	if link == "" {
		return ""
	}
	for i := len(link) - 1; i >= 0; i-- {
		if link[i] == '/' {
			id := link[i+1:]
			if _, err := strconv.Atoi(id); err == nil {
				return id
			}
			return id
		}
	}
	return link
}
