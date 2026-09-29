package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	frontend "github.com/gboutry/sunbeam-watchtower/internal/adapter/primary/frontend"
	dto "github.com/gboutry/sunbeam-watchtower/pkg/dto/v1"
	forge "github.com/gboutry/sunbeam-watchtower/pkg/forge/v1"
	"gopkg.in/yaml.v3"
)

// renderMergeRequests writes merge requests in the requested format.
func renderMergeRequests(w io.Writer, format string, styler *outputStyler, mrs []forge.MergeRequest) error {
	switch format {
	case "json":
		return renderJSON(w, mrs)
	case "yaml":
		return renderYAML(w, mrs)
	default:
		return renderTable(w, styler, mrs)
	}
}

func renderJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func renderYAML(w io.Writer, v any) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// renderMergeRequestDetail writes a single merge request in the requested format.
func renderMergeRequestDetail(w io.Writer, format string, styler *outputStyler, mr *forge.MergeRequest) error {
	switch format {
	case "json":
		return renderJSON(w, mr)
	case "yaml":
		return renderYAML(w, mr)
	default:
		return renderMergeRequestDetailTable(w, styler, mr)
	}
}

func renderMergeRequestDetailTable(w io.Writer, styler *outputStyler, mr *forge.MergeRequest) error {
	if err := writeKeyValue(w, styler, "Project", mr.Repo); err != nil {
		return err
	}
	if err := writeKeyValue(w, styler, "Forge", mr.Forge.String()); err != nil {
		return err
	}
	if err := writeKeyValue(w, styler, "ID", mr.ID); err != nil {
		return err
	}
	if err := writeKeyValue(w, styler, "Title", mr.Title); err != nil {
		return err
	}
	if err := writeKeyValue(w, styler, "Author", mr.Author); err != nil {
		return err
	}
	if err := writeKeyValue(w, styler, "State", mr.State.String()); err != nil {
		return err
	}
	if err := writeKeyValue(w, styler, "Review", mr.ReviewState.String()); err != nil {
		return err
	}
	if err := writeKeyValue(w, styler, "Source", mr.SourceBranch); err != nil {
		return err
	}
	if err := writeKeyValue(w, styler, "Target", mr.TargetBranch); err != nil {
		return err
	}
	if err := writeKeyValue(w, styler, "URL", mr.URL); err != nil {
		return err
	}
	if !mr.CreatedAt.IsZero() {
		if err := writeKeyValue(w, styler, "Created", mr.CreatedAt.Format("2006-01-02 15:04")); err != nil {
			return err
		}
	}
	if !mr.UpdatedAt.IsZero() {
		if err := writeKeyValue(w, styler, "Updated", mr.UpdatedAt.Format("2006-01-02 15:04")); err != nil {
			return err
		}
	}
	if len(mr.Checks) > 0 {
		fmt.Fprintln(w)
		if err := writeSectionTitle(w, styler, "Checks:"); err != nil {
			return err
		}
		for _, c := range mr.Checks {
			fmt.Fprintf(w, "  %s  %s  %s\n", styler.semantic(c.State.String()), c.Name, styler.DetailValue("URL", c.URL))
		}
	}
	if mr.Description != "" {
		fmt.Fprintln(w)
		if err := writeSectionTitle(w, styler, "Description:"); err != nil {
			return err
		}
		fmt.Fprintln(w, mr.Description)
	}
	if len(mr.Comments) > 0 {
		fmt.Fprintln(w)
		if err := writeSectionTitle(w, styler, "Comments:"); err != nil {
			return err
		}
		for _, comment := range mr.Comments {
			anchor := comment.File
			if comment.Line > 0 {
				anchor = fmt.Sprintf("%s:%d", emptyDash(comment.File), comment.Line)
			}
			if anchor == "" {
				anchor = "-"
			}
			fmt.Fprintf(w, "  [%s] %s  %s  %s\n", comment.Kind, emptyDash(comment.Author), emptyDash(anchor), formatTimestamp(comment.CreatedAt))
			if comment.Body != "" {
				fmt.Fprintf(w, "    %s\n", strings.ReplaceAll(comment.Body, "\n", "\n    "))
			}
		}
	}
	if len(mr.Files) > 0 {
		fmt.Fprintln(w)
		if err := writeSectionTitle(w, styler, "Files:"); err != nil {
			return err
		}
		headers := []string{"PATH", "STATUS", "+", "-", "PATCH"}
		rows := make([][]string, 0, len(mr.Files))
		for _, file := range mr.Files {
			patch := file.Patch
			if patch == "" {
				switch {
				case file.Binary:
					patch = "(binary)"
				case file.Truncated:
					patch = "(truncated)"
				default:
					patch = "-"
				}
			} else if len(patch) > 50 {
				patch = patch[:47] + "..."
			}
			rows = append(rows, []string{file.Path, emptyDash(file.Status), fmt.Sprintf("%d", file.Additions), fmt.Sprintf("%d", file.Deletions), patch})
		}
		if err := renderStyledTable(w, styler, headers, rows); err != nil {
			return err
		}
	}
	if mr.DiffText != "" {
		fmt.Fprintln(w)
		if err := writeSectionTitle(w, styler, "Diff:"); err != nil {
			return err
		}
		fmt.Fprintln(w, mr.DiffText)
	}
	return nil
}

// renderBugDetail writes a single bug in the requested format.
func renderBugDetail(w io.Writer, format string, styler *outputStyler, b *forge.Bug) error {
	switch format {
	case "json":
		return renderJSON(w, b)
	case "yaml":
		return renderYAML(w, b)
	default:
		return renderBugDetailTable(w, styler, b)
	}
}

func renderBugDetailTable(w io.Writer, styler *outputStyler, b *forge.Bug) error {
	if err := writeKeyValue(w, styler, "Bug", "#"+b.ID); err != nil {
		return err
	}
	if err := writeKeyValue(w, styler, "Title", b.Title); err != nil {
		return err
	}
	if err := writeKeyValue(w, styler, "Owner", b.Owner); err != nil {
		return err
	}
	if err := writeKeyValue(w, styler, "URL", b.URL); err != nil {
		return err
	}
	if len(b.Tags) > 0 {
		if err := writeKeyValue(w, styler, "Tags", strings.Join(b.Tags, ", ")); err != nil {
			return err
		}
	}
	if b.InformationType != "" {
		if err := writeKeyValue(w, styler, "Information type", b.InformationType); err != nil {
			return err
		}
	}
	if b.Private {
		if err := writeKeyValue(w, styler, "Private", "true"); err != nil {
			return err
		}
	}
	if b.SecurityRelated {
		if err := writeKeyValue(w, styler, "Security related", "true"); err != nil {
			return err
		}
	}
	if b.Provenance != nil {
		if err := writeKeyValue(w, styler, "Data source", b.Provenance.Source); err != nil {
			return err
		}
		if !b.Provenance.SyncedAt.IsZero() {
			if err := writeKeyValue(w, styler, "Cache synced", b.Provenance.SyncedAt.Format(time.RFC3339)); err != nil {
				return err
			}
		}
		if !b.Provenance.VerifiedAt.IsZero() {
			if err := writeKeyValue(w, styler, "Visibility verified", b.Provenance.VerifiedAt.Format(time.RFC3339)); err != nil {
				return err
			}
		}
	}
	if !b.CreatedAt.IsZero() {
		if err := writeKeyValue(w, styler, "Created", b.CreatedAt.Format("2006-01-02 15:04")); err != nil {
			return err
		}
	}
	if !b.UpdatedAt.IsZero() {
		if err := writeKeyValue(w, styler, "Updated", b.UpdatedAt.Format("2006-01-02 15:04")); err != nil {
			return err
		}
	}
	if b.Description != "" {
		fmt.Fprintln(w)
		if err := writeSectionTitle(w, styler, "Description:"); err != nil {
			return err
		}
		fmt.Fprintln(w, b.Description)
	}
	if len(b.Tasks) > 0 {
		fmt.Fprintln(w)
		if err := writeSectionTitle(w, styler, "Tasks:"); err != nil {
			return err
		}
		headers := []string{"TARGET", "STATUS", "IMPORTANCE", "ASSIGNEE", "URL"}
		rows := make([][]string, 0, len(b.Tasks))
		for _, t := range b.Tasks {
			rows = append(rows, []string{t.Title, t.Status, t.Importance, t.Assignee, t.URL})
		}
		if err := renderStyledTable(w, styler, headers, rows); err != nil {
			return err
		}
	}
	if len(b.Comments) > 0 {
		fmt.Fprintln(w)
		if err := writeSectionTitle(w, styler, "Comments:"); err != nil {
			return err
		}
		for _, comment := range b.Comments {
			fmt.Fprintf(w, "  %s  %s\n", emptyDash(comment.Author), formatTimestamp(comment.CreatedAt))
			if comment.Subject != "" {
				fmt.Fprintf(w, "    %s\n", comment.Subject)
			}
			if comment.Body != "" {
				fmt.Fprintf(w, "    %s\n", strings.ReplaceAll(comment.Body, "\n", "\n    "))
			}
			if comment.URL != "" {
				fmt.Fprintf(w, "    %s\n", comment.URL)
			}
		}
	}
	if len(b.Links) > 0 {
		fmt.Fprintln(w)
		if err := writeSectionTitle(w, styler, "Associated links:"); err != nil {
			return err
		}
		for _, link := range b.Links {
			fmt.Fprintf(w, "  %-24s %s\n", link.Relation, link.URL)
		}
	}
	return nil
}

func renderBugSearch(
	w io.Writer,
	format string,
	styler *outputStyler,
	result *dto.BugSearchResponse,
	showRelated bool,
) error {
	switch format {
	case "json":
		return renderJSON(w, result)
	case "yaml":
		return renderYAML(w, result)
	}

	fmt.Fprintf(w, "Outcome: %s\n", styler.semantic(string(result.Outcome)))
	fmt.Fprintf(
		w,
		"Scope: mode=%s fields=%s fuzzy=%t closed=%s merge=%t sort=%s documents=%d\n",
		result.Applied.Mode,
		strings.Join(result.Applied.Fields, ","),
		result.Applied.Fuzzy,
		result.Applied.Closed,
		result.Applied.Merge,
		result.Applied.Sort,
		result.Applied.DocumentCount,
	)
	for _, source := range result.Provenance {
		synced := "not synced"
		if !source.SyncedAt.IsZero() {
			synced = source.SyncedAt.Format(time.RFC3339)
		}
		verified := ""
		if source.Verified {
			verified = " (visibility verified)"
		}
		fmt.Fprintf(w, "Source: %s/%s %s at %s%s\n", source.Forge, emptyDash(source.Project), source.Source, synced, verified)
	}

	if len(result.Results) == 0 {
		fmt.Fprintln(w, "\nNo direct or partial matching bug was found.")
	} else {
		fmt.Fprintln(w)
		if err := writeSectionTitle(w, styler, "Matches:"); err != nil {
			return err
		}
		if err := renderBugSearchGroup(w, styler, result.Results); err != nil {
			return err
		}
	}
	if len(result.Related) > 0 && (showRelated || len(result.Results) == 0) {
		fmt.Fprintln(w)
		if err := writeSectionTitle(w, styler, "Related results:"); err != nil {
			return err
		}
		if err := renderBugSearchGroup(w, styler, result.Related); err != nil {
			return err
		}
	}
	if result.Truncated {
		fmt.Fprintf(
			w,
			"\nResults truncated: %d primary and %d related candidates matched the applied scope.\n",
			result.TotalResults,
			result.TotalRelated,
		)
	}
	return nil
}

func renderBugSearchGroup(w io.Writer, styler *outputStyler, results []dto.BugSearchResult) error {
	for i, candidate := range results {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(
			w,
			"[%s score=%.3f] %s\n",
			styler.semantic(string(candidate.Classification)),
			candidate.Score,
			candidate.Reference,
		)
		fmt.Fprintf(w, "Title: %s\n", candidate.Title)
		fmt.Fprintf(w, "Projects: %s\n", strings.Join(candidate.Projects, ", "))
		fmt.Fprintf(w, "Status: %s\n", strings.Join(candidate.Status, ", "))
		fmt.Fprintf(w, "Importance: %s\n", strings.Join(candidate.Importance, ", "))
		fmt.Fprintf(w, "URL: %s\n", candidate.URL)
		fmt.Fprintf(
			w,
			"Coverage: %.0f%% (%d/%d clauses)\n",
			candidate.Coverage.Ratio*100,
			candidate.Coverage.MatchedClauses,
			candidate.Coverage.TotalClauses,
		)
		for _, evidence := range candidate.Evidence {
			before, after := "", ""
			if evidence.TruncatedBefore {
				before = "…"
			}
			if evidence.TruncatedAfter {
				after = "…"
			}
			if evidence.MatchTruncated {
				after += " [match truncated]"
			}
			source := ""
			if evidence.SourceReference != "" {
				source = " " + evidence.SourceReference
			}
			var matches []string
			for _, match := range evidence.Matches {
				matches = append(matches, fmt.Sprintf(
					"%s/%s=%q",
					match.QueryConcept,
					match.MatchType,
					match.MatchedText,
				))
			}
			fmt.Fprintf(w, "Why: %s%s [%s] -> %s%s%s\n",
				evidence.Field, source, strings.Join(matches, ", "),
				before, evidence.Excerpt, after)
		}
		if candidate.EvidenceTruncated {
			fmt.Fprintf(
				w,
				"Evidence: showing %d of %d grouped excerpts\n",
				len(candidate.Evidence),
				candidate.EvidenceTotal,
			)
		}
	}
	return nil
}

// renderBugTasks writes bug tasks in the requested format.
func renderBugTasks(w io.Writer, format string, styler *outputStyler, tasks []forge.BugTask) error {
	switch format {
	case "json":
		return renderJSON(w, tasks)
	case "yaml":
		return renderYAML(w, tasks)
	default:
		return renderBugTable(w, styler, tasks)
	}
}

func renderBugTable(w io.Writer, styler *outputStyler, tasks []forge.BugTask) error {
	if len(tasks) == 0 {
		fmt.Fprintln(w, "No bug tasks found.")
		return nil
	}

	headers := []string{"PROJECT", "ID", "STATUS", "IMPORTANCE", "ASSIGNEE", "TITLE", "URL"}
	rows := make([][]string, 0, len(tasks))
	for _, t := range tasks {
		title := cleanBugListTitle(t.Title, t.BugID, t.Project)
		if len(title) > 60 {
			title = title[:57] + "..."
		}
		title = strings.ReplaceAll(title, "\t", " ")

		rows = append(rows, []string{t.Project, t.BugID, t.Status, t.Importance, t.Assignee, title, t.URL})
	}
	return renderStyledTable(w, styler, headers, rows)
}

func cleanBugListTitle(title, bugID, project string) string {
	title = stripBugListTitlePrefix(title, bugID, project)
	title = strings.TrimSpace(title)
	if len(title) >= 2 && strings.HasPrefix(title, "\"") && strings.HasSuffix(title, "\"") {
		title = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(title, "\""), "\""))
	}
	return title
}

func stripBugListTitlePrefix(title, bugID, project string) string {
	title = strings.TrimSpace(title)
	if title == "" || bugID == "" {
		return title
	}

	genericPrefix := fmt.Sprintf("Bug #%s in ", bugID)
	if strings.HasPrefix(title, genericPrefix) {
		if idx := strings.Index(title[len(genericPrefix):], ": "); idx >= 0 {
			return strings.TrimSpace(title[len(genericPrefix)+idx+2:])
		}
	}

	prefixes := []string{
		fmt.Sprintf("Bug #%s in %s: ", bugID, project),
		fmt.Sprintf("Bug #%s in %s: ", bugID, humanizeBugProject(project)),
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(title, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(title, prefix))
		}
	}
	return title
}

func humanizeBugProject(project string) string {
	project = strings.TrimSpace(project)
	if project == "" {
		return project
	}
	parts := strings.FieldsFunc(project, func(r rune) bool {
		return r == '-' || r == '_' || r == '/'
	})
	if len(parts) == 0 {
		return project
	}
	for i, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(strings.ToLower(part))
		runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
		parts[i] = string(runes)
	}
	return strings.Join(parts, " ")
}

func renderTable(w io.Writer, styler *outputStyler, mrs []forge.MergeRequest) error {
	if len(mrs) == 0 {
		fmt.Fprintln(w, "No merge requests found.")
		return nil
	}

	headers := []string{"PROJECT", "FORGE", "ID", "STATE", "AUTHOR", "TITLE", "URL"}
	rows := make([][]string, 0, len(mrs))
	for _, mr := range mrs {
		title := mr.Title
		if len(title) > 60 {
			title = title[:57] + "..."
		}
		title = strings.ReplaceAll(title, "\t", " ")

		rows = append(rows, []string{mr.Repo, mr.Forge.String(), mr.ID, mr.State.String(), mr.Author, title, mr.URL})
	}
	return renderStyledTable(w, styler, headers, rows)
}

// renderBuilds writes builds in the requested format.
func renderBuilds(w io.Writer, format string, styler *outputStyler, builds []dto.Build) error {
	switch format {
	case "json":
		return renderJSON(w, builds)
	case "yaml":
		return renderYAML(w, builds)
	default:
		return renderBuildsTable(w, styler, builds)
	}
}

func renderBuildsTable(w io.Writer, styler *outputStyler, builds []dto.Build) error {
	if len(builds) == 0 {
		fmt.Fprintln(w, "No builds found.")
		return nil
	}

	headers := []string{"PROJECT", "RECIPE", "ARCH", "STATE", "CREATED", "URL"}
	rows := make([][]string, 0, len(builds))
	for _, b := range builds {
		created := ""
		if !b.CreatedAt.IsZero() {
			created = b.CreatedAt.Format("2006-01-02 15:04")
		}
		rows = append(rows, []string{b.Project, b.Recipe, b.Arch, b.State.String(), created, b.WebLink})
	}
	return renderStyledTable(w, styler, headers, rows)
}

// renderBuildRequests writes build request results in the requested format.
func renderBuildRequests(w io.Writer, format string, styler *outputStyler, results []dto.BuildRequest) error {
	switch format {
	case "json":
		return renderJSON(w, results)
	case "yaml":
		return renderYAML(w, results)
	default:
		return renderBuildRequestsTable(w, styler, results)
	}
}

func renderBuildRequestsTable(w io.Writer, styler *outputStyler, results []dto.BuildRequest) error {
	if len(results) == 0 {
		fmt.Fprintln(w, "No build requests found.")
		return nil
	}

	headers := []string{"STATUS", "ERROR", "URL"}
	rows := make([][]string, 0, len(results))
	for _, r := range results {
		rows = append(rows, []string{r.Status, r.ErrorMessage, r.WebLink})
	}
	return renderStyledTable(w, styler, headers, rows)
}

// renderOperationJobs writes operations in the requested format.
func renderOperationJobs(w io.Writer, format string, styler *outputStyler, jobs []dto.OperationJob) error {
	switch format {
	case "json":
		return renderJSON(w, jobs)
	case "yaml":
		return renderYAML(w, jobs)
	default:
		return renderOperationJobsTable(w, styler, jobs)
	}
}

func renderOperationJobsTable(w io.Writer, styler *outputStyler, jobs []dto.OperationJob) error {
	if len(jobs) == 0 {
		fmt.Fprintln(w, "No operations found.")
		return nil
	}

	headers := []string{"ID", "KIND", "STATE", "CREATED", "SUMMARY"}
	rows := make([][]string, 0, len(jobs))
	for _, job := range jobs {
		rows = append(rows, []string{job.ID, string(job.Kind), string(job.State), formatTimestamp(job.CreatedAt), job.Summary})
	}
	return renderStyledTable(w, styler, headers, rows)
}

// renderOperationJob writes a single operation in the requested format.
func renderOperationJob(w io.Writer, format string, styler *outputStyler, job *dto.OperationJob) error {
	switch format {
	case "json":
		return renderJSON(w, job)
	case "yaml":
		return renderYAML(w, job)
	default:
		return renderOperationJobTable(w, styler, job)
	}
}

func renderOperationJobTable(w io.Writer, styler *outputStyler, job *dto.OperationJob) error {
	if job == nil {
		fmt.Fprintln(w, "No operation found.")
		return nil
	}

	if err := writeKeyValue(w, styler, "ID", job.ID); err != nil {
		return err
	}
	if err := writeKeyValue(w, styler, "Kind", string(job.Kind)); err != nil {
		return err
	}
	if err := writeKeyValue(w, styler, "State", string(job.State)); err != nil {
		return err
	}
	if err := writeKeyValue(w, styler, "Created", formatTimestamp(job.CreatedAt)); err != nil {
		return err
	}
	if !job.StartedAt.IsZero() {
		if err := writeKeyValue(w, styler, "Started", formatTimestamp(job.StartedAt)); err != nil {
			return err
		}
	}
	if !job.FinishedAt.IsZero() {
		if err := writeKeyValue(w, styler, "Finished", formatTimestamp(job.FinishedAt)); err != nil {
			return err
		}
	}
	if err := writeKeyValue(w, styler, "Cancellable", fmt.Sprintf("%t", job.Cancellable)); err != nil {
		return err
	}
	if job.Summary != "" {
		if err := writeKeyValue(w, styler, "Summary", job.Summary); err != nil {
			return err
		}
	}
	if job.Error != "" {
		if err := writeKeyValue(w, styler, "Error", job.Error); err != nil {
			return err
		}
	}
	if job.Progress != nil {
		if err := writeKeyValue(w, styler, "Progress", formatOperationProgress(job.Progress)); err != nil {
			return err
		}
	}
	if len(job.Attributes) > 0 {
		if err := writeSectionTitle(w, styler, "Attributes:"); err != nil {
			return err
		}
		for key, value := range job.Attributes {
			fmt.Fprintf(w, "  %s=%s\n", styler.Key(key), value)
		}
	}
	return nil
}

// renderOperationEvents writes operation events in the requested format.
func renderOperationEvents(w io.Writer, format string, styler *outputStyler, events []dto.OperationEvent) error {
	switch format {
	case "json":
		return renderJSON(w, events)
	case "yaml":
		return renderYAML(w, events)
	default:
		return renderOperationEventsTable(w, styler, events)
	}
}

func renderOperationEventsTable(w io.Writer, styler *outputStyler, events []dto.OperationEvent) error {
	if len(events) == 0 {
		fmt.Fprintln(w, "No operation events found.")
		return nil
	}

	headers := []string{"TIME", "TYPE", "MESSAGE", "ERROR", "PROGRESS"}
	rows := make([][]string, 0, len(events))
	for _, event := range events {
		rows = append(rows, []string{
			formatTimestamp(event.Time),
			event.Type,
			event.Message,
			event.Error,
			formatOperationProgress(event.Progress),
		})
	}
	return renderStyledTable(w, styler, headers, rows)
}

// renderCommits writes commits in the requested format.
func renderCommits(w io.Writer, format string, styler *outputStyler, commits []forge.Commit) error {
	switch format {
	case "json":
		return renderJSON(w, commits)
	case "yaml":
		return renderYAML(w, commits)
	default:
		return renderCommitTable(w, styler, commits)
	}
}

func renderCommitTable(w io.Writer, styler *outputStyler, commits []forge.Commit) error {
	if len(commits) == 0 {
		fmt.Fprintln(w, "No commits found.")
		return nil
	}

	headers := []string{"PROJECT", "FORGE", "SHA", "AUTHOR", "DATE", "STATUS", "LINK", "MESSAGE"}
	rows := make([][]string, 0, len(commits))
	for _, c := range commits {
		msg := c.Message
		if idx := strings.Index(msg, "\n"); idx != -1 {
			msg = msg[:idx]
		}
		if len(msg) > 60 {
			msg = msg[:57] + "..."
		}
		msg = strings.ReplaceAll(msg, "\t", " ")

		date := ""
		if !c.Date.IsZero() {
			date = c.Date.Format("2006-01-02 15:04")
		}
		sha := c.SHA
		if len(sha) > 10 {
			sha = sha[:10]
		}

		status := ""
		link := ""
		if c.MergeRequest != nil {
			status = c.MergeRequest.State.String()
			link = c.MergeRequest.URL
		}

		rows = append(rows, []string{c.Repo, c.Forge.String(), sha, c.Author, date, status, link, msg})
	}
	return renderStyledTable(w, styler, headers, rows)
}

// renderBugSyncResult writes bug sync results in the requested format.
func renderBugSyncResult(w io.Writer, format string, styler *outputStyler, result *dto.BugSyncResult, dryRun bool) error {
	switch format {
	case "json":
		return renderJSON(w, result)
	case "yaml":
		return renderYAML(w, result)
	default:
		return renderBugSyncTable(w, styler, result, dryRun)
	}
}

func renderBugSyncTable(w io.Writer, styler *outputStyler, result *dto.BugSyncResult, dryRun bool) error {
	if len(result.Actions) == 0 {
		fmt.Fprintln(w, "No bugs to sync.")
		return nil
	}
	prefix := ""
	if dryRun {
		prefix = styler.Dim("would") + " "
	}
	for _, a := range result.Actions {
		switch a.ActionType {
		case dto.BugSyncActionStatusUpdate:
			fmt.Fprintf(w, "%s%s Bug #%s task %q %s -> %s\n", prefix, styler.Action("update:"), a.BugID, a.TaskTitle, styler.semantic(a.OldStatus), styler.semantic(a.NewStatus))
		case dto.BugSyncActionSeriesAssignment:
			fmt.Fprintf(w, "%s%s Bug #%s to series %q on project %q\n", prefix, styler.Action("assign:"), a.BugID, a.Series, a.Project)
		case dto.BugSyncActionAddProjectTask:
			fmt.Fprintf(w, "%s%s Bug #%s task on project %q\n", prefix, styler.Action("add:"), a.BugID, a.Project)
		}
		if a.Reason != "" {
			fmt.Fprintf(w, "  %s %s\n", styler.Dim("reason:"), a.Reason)
		}
	}
	return nil
}

// renderProjectSyncResult writes project sync results in the requested format.
func renderProjectSyncResult(w io.Writer, format string, styler *outputStyler, result *dto.ProjectSyncResult, dryRun bool) error {
	switch format {
	case "json":
		return renderJSON(w, result)
	case "yaml":
		return renderYAML(w, result)
	default:
		return renderProjectSyncTable(w, styler, result, dryRun)
	}
}

func renderProjectSyncTable(w io.Writer, styler *outputStyler, result *dto.ProjectSyncResult, dryRun bool) error {
	if len(result.Actions) == 0 {
		fmt.Fprintln(w, "No changes needed.")
		return nil
	}
	prefix := ""
	if dryRun {
		prefix = styler.Dim("would") + " "
	}
	for _, a := range result.Actions {
		switch a.ActionType {
		case dto.ProjectSyncActionCreateSeries:
			fmt.Fprintf(w, "%s%s series %q on project %q\n", prefix, styler.Action("create:"), a.Series, a.Project)
		case dto.ProjectSyncActionSetDevFocus:
			fmt.Fprintf(w, "%s%s development focus to %q on project %q\n", prefix, styler.Action("set:"), a.Series, a.Project)
		case dto.ProjectSyncActionDevFocusUnchanged:
			fmt.Fprintf(w, "%s development focus already %q on project %q\n", styler.Action("unchanged:"), a.Series, a.Project)
		}
	}
	return nil
}

// renderTeamSyncResult writes team sync results in the requested format.
func renderTeamSyncResult(w io.Writer, format string, styler *outputStyler, result *dto.TeamSyncResult, dryRun bool) error {
	switch format {
	case "json":
		return renderJSON(w, result)
	case "yaml":
		return renderYAML(w, result)
	default:
		return renderTeamSyncTable(w, styler, result, dryRun)
	}
}

func renderTeamSyncTable(w io.Writer, styler *outputStyler, result *dto.TeamSyncResult, dryRun bool) error {
	if len(result.Artifacts) == 0 {
		fmt.Fprintln(w, "No artifacts to sync.")
		return nil
	}
	prefix := ""
	if dryRun {
		prefix = styler.Dim("would") + " "
	}
	for _, a := range result.Artifacts {
		label := fmt.Sprintf("%s/%s (%s)", a.Project, a.StoreName, a.ArtifactType.String())
		if a.Error != "" {
			fmt.Fprintf(w, "%s %s: %s\n", styler.Action("error:"), label, a.Error)
			continue
		}
		if a.Unsupported {
			dest := a.UnsupportedURL
			if dest == "" {
				dest = "the store's web UI"
			}
			fmt.Fprintf(w, "%s %s: manage collaborators at %s\n", styler.Action("unsupported:"), label, dest)
			continue
		}
		if a.AuthExpired {
			hint := a.AuthHint
			if hint == "" {
				hint = "re-authenticate with the store"
			}
			fmt.Fprintf(w, "%s %s: authentication expired — %s\n", styler.Action("auth:"), label, hint)
			continue
		}
		if a.AlreadySync {
			fmt.Fprintf(w, "%s %s\n", styler.Action("in-sync:"), label)
			continue
		}
		for _, u := range a.Invited {
			fmt.Fprintf(w, "%s%s %s -> %s\n", prefix, styler.Action("invite:"), label, u)
		}
		for _, u := range a.Extra {
			fmt.Fprintf(w, "%s %s extra collaborator: %s\n", styler.Action("warn:"), label, u)
		}
		for _, u := range a.Pending {
			fmt.Fprintf(w, "%s %s pending invite: %s\n", styler.Action("pending:"), label, u)
		}
	}
	return nil
}

// renderStringList writes a list of strings in the requested format.
func renderStringList(w io.Writer, format string, styler *outputStyler, items []string) error {
	switch format {
	case "json":
		return renderJSON(w, items)
	case "yaml":
		return renderYAML(w, items)
	default:
		for _, item := range items {
			fmt.Fprintln(w, styler.Value("NAME", item))
		}
		return nil
	}
}

// renderReleaseList writes release list rows in the requested format.
func renderReleaseList(w io.Writer, format string, styler *outputStyler, releases []dto.ReleaseListEntry) error {
	switch format {
	case "json":
		return renderJSON(w, releases)
	case "yaml":
		return renderYAML(w, releases)
	default:
		if len(releases) == 0 {
			fmt.Fprintln(w, "No releases found.")
			return nil
		}
		headers := []string{"PROJECT", "TYPE", "NAME", "TRACK", "RISK", "BRANCH", "TARGETS", "RESOURCES", "RELEASED"}
		rows := make([][]string, 0, len(releases))
		for _, release := range releases {
			rows = append(rows, []string{
				release.Project,
				release.ArtifactType.String(),
				release.Name,
				release.Track,
				string(release.Risk),
				emptyDash(release.Branch),
				formatReleaseTargets(release.Targets),
				formatReleaseResources(release.Resources),
				formatTimestamp(release.ReleasedAt),
			})
		}
		return renderStyledTable(w, styler, headers, rows)
	}
}

// renderReleaseShow writes one release matrix in the requested format.
func renderReleaseShow(w io.Writer, format string, styler *outputStyler, release *dto.ReleaseShowResult) error {
	switch format {
	case "json":
		return renderJSON(w, release)
	case "yaml":
		return renderYAML(w, release)
	default:
		if err := writeKeyValue(w, styler, "Project", release.Project); err != nil {
			return err
		}
		if err := writeKeyValue(w, styler, "Type", release.ArtifactType.String()); err != nil {
			return err
		}
		if err := writeKeyValue(w, styler, "Name", release.Name); err != nil {
			return err
		}
		if err := writeKeyValue(w, styler, "Updated", formatTimestamp(release.UpdatedAt)); err != nil {
			return err
		}
		if len(release.Tracks) > 0 {
			if err := writeKeyValue(w, styler, "Tracks", strings.Join(release.Tracks, ", ")); err != nil {
				return err
			}
		}
		fmt.Fprintln(w)
		headers := []string{"TRACK", "RISK", "BRANCH", "TARGETS", "RESOURCES"}
		rows := make([][]string, 0, len(release.Channels))
		for _, channel := range release.Channels {
			rows = append(rows, []string{
				channel.Track,
				string(channel.Risk),
				emptyDash(channel.Branch),
				formatReleaseTargets(channel.Targets),
				formatReleaseResources(channel.Resources),
			})
		}
		return renderStyledTable(w, styler, headers, rows)
	}
}

func formatReleaseTargets(targets []dto.ReleaseTargetSnapshot) string {
	return frontend.FormatReleaseTargets(targets)
}

func formatReleaseResources(resources []dto.ReleaseResourceSnapshot) string {
	if len(resources) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(resources))
	for _, resource := range resources {
		if resource.Revision > 0 {
			parts = append(parts, fmt.Sprintf("%s:r%d", resource.Name, resource.Revision))
		} else {
			parts = append(parts, resource.Name)
		}
	}
	return strings.Join(parts, ", ")
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

type cacheEntry struct {
	Name string `json:"name" yaml:"name"`
	Size string `json:"size" yaml:"size"`
}

type cacheFullStatus struct {
	Git struct {
		Directory string       `json:"directory" yaml:"directory"`
		Repos     []cacheEntry `json:"repos" yaml:"repos"`
	} `json:"git" yaml:"git"`
	Packages struct {
		Directory string            `json:"directory,omitempty" yaml:"directory,omitempty"`
		Error     string            `json:"error,omitempty" yaml:"error,omitempty"`
		Sources   []dto.CacheStatus `json:"sources" yaml:"sources"`
	} `json:"packages" yaml:"packages"`
	PackageSets struct {
		Entries []dto.PackageSetCacheStatus `json:"entries" yaml:"entries"`
		Error   string                      `json:"error,omitempty" yaml:"error,omitempty"`
	} `json:"packagesets" yaml:"packagesets"`
	Upstream struct {
		Directory string       `json:"directory" yaml:"directory"`
		Repos     []cacheEntry `json:"repos" yaml:"repos"`
	} `json:"upstream" yaml:"upstream"`
	Bugs struct {
		Directory string               `json:"directory" yaml:"directory"`
		Entries   []dto.BugCacheStatus `json:"entries" yaml:"entries"`
		Error     string               `json:"error,omitempty" yaml:"error,omitempty"`
	} `json:"bugs" yaml:"bugs"`
	Excuses struct {
		Directory string                   `json:"directory" yaml:"directory"`
		Entries   []dto.ExcusesCacheStatus `json:"entries" yaml:"entries"`
		Error     string                   `json:"error,omitempty" yaml:"error,omitempty"`
	} `json:"excuses" yaml:"excuses"`
	Releases struct {
		Directory string                   `json:"directory" yaml:"directory"`
		Entries   []dto.ReleaseCacheStatus `json:"entries" yaml:"entries"`
		Error     string                   `json:"error,omitempty" yaml:"error,omitempty"`
	} `json:"releases" yaml:"releases"`
	Reviews struct {
		Directory string                  `json:"directory" yaml:"directory"`
		Entries   []dto.ReviewCacheStatus `json:"entries" yaml:"entries"`
		Error     string                  `json:"error,omitempty" yaml:"error,omitempty"`
	} `json:"reviews" yaml:"reviews"`
	SRU struct {
		Status dto.SRUCacheStatus `json:"status" yaml:"status"`
		Error  string             `json:"error,omitempty" yaml:"error,omitempty"`
	} `json:"sru" yaml:"sru"`
}

func renderCacheFullStatus(w io.Writer, format string, styler *outputStyler, status *cacheFullStatus) error {
	switch format {
	case "json":
		return renderJSON(w, status)
	case "yaml":
		return renderYAML(w, status)
	default:
		return renderCacheFullStatusTable(w, styler, status)
	}
}

func renderCacheFullStatusTable(w io.Writer, styler *outputStyler, status *cacheFullStatus) error {
	if err := writeSectionTitle(w, styler, "=== Git Repos ==="); err != nil {
		return err
	}
	if len(status.Git.Repos) == 0 {
		fmt.Fprintln(w, " ", styler.Placeholder("(none)"))
	} else {
		if err := writeKeyValue(w, styler, "directory", status.Git.Directory); err != nil {
			return err
		}
		for _, r := range status.Git.Repos {
			fmt.Fprintf(w, "  %s  (%s)\n", styler.Value("NAME", r.Name), styler.Dim(r.Size))
		}
	}

	fmt.Fprintln(w)
	if err := writeSectionTitle(w, styler, "=== Packages Index ==="); err != nil {
		return err
	}
	if status.Packages.Error != "" {
		fmt.Fprintf(w, "  %s\n", styler.Error("(unavailable: "+status.Packages.Error+")"))
	} else if len(status.Packages.Sources) == 0 {
		fmt.Fprintln(w, " ", styler.Placeholder("(none)"))
	} else {
		if err := writeKeyValue(w, styler, "directory", status.Packages.Directory); err != nil {
			return err
		}
		if err := renderCacheStatusTable(w, styler, status.Packages.Sources); err != nil {
			return err
		}
	}

	fmt.Fprintln(w)
	if err := writeSectionTitle(w, styler, "=== Launchpad Packagesets ==="); err != nil {
		return err
	}
	if status.PackageSets.Error != "" {
		fmt.Fprintf(w, "  %s\n", styler.Error("(unavailable: "+status.PackageSets.Error+")"))
	} else if len(status.PackageSets.Entries) == 0 {
		fmt.Fprintln(w, " ", styler.Placeholder("(none configured)"))
	} else {
		for _, entry := range status.PackageSets.Entries {
			if entry.SyncedAt.IsZero() {
				fmt.Fprintf(w, "  %s  %s\n", styler.Value("NAME", entry.Name), styler.Placeholder("(not synced)"))
				continue
			}
			fmt.Fprintf(w, "  %s  %d packages from %s (synced %s)\n",
				styler.Value("NAME", entry.Name), entry.PackageCount, entry.Series, entry.SyncedAt.Format("2006-01-02 15:04 UTC"))
		}
	}

	fmt.Fprintln(w)
	if err := writeSectionTitle(w, styler, "=== Upstream Repos ==="); err != nil {
		return err
	}
	if len(status.Upstream.Repos) == 0 {
		fmt.Fprintln(w, " ", styler.Placeholder("(none)"))
	} else {
		if err := writeKeyValue(w, styler, "directory", status.Upstream.Directory); err != nil {
			return err
		}
		for _, r := range status.Upstream.Repos {
			fmt.Fprintf(w, "  %s  (%s)\n", styler.Value("NAME", r.Name), styler.Dim(r.Size))
		}
	}

	fmt.Fprintln(w)
	if err := writeSectionTitle(w, styler, "=== Bugs ==="); err != nil {
		return err
	}
	if status.Bugs.Error != "" {
		fmt.Fprintf(w, "  %s\n", styler.Error("(unavailable: "+status.Bugs.Error+")"))
	} else if len(status.Bugs.Entries) == 0 {
		fmt.Fprintln(w, " ", styler.Placeholder("(none)"))
	} else {
		if err := writeKeyValue(w, styler, "directory", status.Bugs.Directory); err != nil {
			return err
		}
		for _, e := range status.Bugs.Entries {
			syncStr := "never"
			if !e.LastSync.IsZero() {
				syncStr = e.LastSync.Format("2006-01-02 15:04:05")
			}
			compatibility := fmt.Sprintf("schema: %d", e.SchemaVersion)
			if e.NeedsRefresh {
				compatibility += ", refresh required"
			}
			fmt.Fprintf(
				w,
				"  %s/%s: %d bugs, %d tasks (synced: %s; %s)\n",
				e.ForgeType,
				e.Project,
				e.BugCount,
				e.TaskCount,
				syncStr,
				compatibility,
			)
		}
	}

	fmt.Fprintln(w)
	if err := writeSectionTitle(w, styler, "=== Excuses ==="); err != nil {
		return err
	}
	if status.Excuses.Error != "" {
		fmt.Fprintf(w, "  %s\n", styler.Error("(unavailable: "+status.Excuses.Error+")"))
	} else if len(status.Excuses.Entries) == 0 {
		fmt.Fprintln(w, " ", styler.Placeholder("(none)"))
	} else {
		if err := writeKeyValue(w, styler, "directory", status.Excuses.Directory); err != nil {
			return err
		}
		headers := []string{"TRACKER", "ENTRIES", "LAST UPDATED", "SIZE"}
		rows := make([][]string, 0, len(status.Excuses.Entries))
		for _, entry := range status.Excuses.Entries {
			lastUpdated := "never"
			if !entry.LastUpdated.IsZero() {
				lastUpdated = entry.LastUpdated.Format("2006-01-02 15:04:05")
			}
			rows = append(rows, []string{entry.Tracker, fmt.Sprintf("%d", entry.EntryCount), lastUpdated, formatSize(entry.DiskSize)})
		}
		if err := renderStyledTable(w, styler, headers, rows); err != nil {
			return err
		}
	}

	fmt.Fprintln(w)
	if err := writeSectionTitle(w, styler, "=== Releases ==="); err != nil {
		return err
	}
	if status.Releases.Error != "" {
		fmt.Fprintf(w, "  %s\n", styler.Error("(unavailable: "+status.Releases.Error+")"))
	} else if len(status.Releases.Entries) == 0 {
		fmt.Fprintln(w, " ", styler.Placeholder("(none)"))
	} else {
		if err := writeKeyValue(w, styler, "directory", status.Releases.Directory); err != nil {
			return err
		}
		headers := []string{"PROJECT", "TYPE", "NAME", "TRACKS", "CHANNELS", "LAST UPDATED"}
		rows := make([][]string, 0, len(status.Releases.Entries))
		for _, entry := range status.Releases.Entries {
			lastUpdated := "never"
			if !entry.LastUpdated.IsZero() {
				lastUpdated = entry.LastUpdated.Format("2006-01-02 15:04:05")
			}
			rows = append(rows, []string{
				entry.Project,
				entry.ArtifactType.String(),
				entry.Name,
				fmt.Sprintf("%d", entry.TrackCount),
				fmt.Sprintf("%d", entry.ChannelCount),
				lastUpdated,
			})
		}
		if err := renderStyledTable(w, styler, headers, rows); err != nil {
			return err
		}
	}

	fmt.Fprintln(w)
	if err := writeSectionTitle(w, styler, "=== Reviews ==="); err != nil {
		return err
	}
	if status.Reviews.Error != "" {
		fmt.Fprintf(w, "  %s\n", styler.Error("(unavailable: "+status.Reviews.Error+")"))
	} else if len(status.Reviews.Entries) == 0 {
		fmt.Fprintln(w, " ", styler.Placeholder("(none)"))
	} else {
		if err := writeKeyValue(w, styler, "directory", status.Reviews.Directory); err != nil {
			return err
		}
		headers := []string{"PROJECT", "FORGE", "SUMMARIES", "DETAILS", "LAST SYNC"}
		rows := make([][]string, 0, len(status.Reviews.Entries))
		for _, entry := range status.Reviews.Entries {
			lastSync := "never"
			if !entry.LastSync.IsZero() {
				lastSync = entry.LastSync.Format("2006-01-02 15:04:05")
			}
			rows = append(rows, []string{
				entry.Project,
				entry.ForgeType,
				fmt.Sprintf("%d", entry.SummaryCount),
				fmt.Sprintf("%d", entry.DetailCount),
				lastSync,
			})
		}
		if err := renderStyledTable(w, styler, headers, rows); err != nil {
			return err
		}
	}

	fmt.Fprintln(w)
	if err := writeSectionTitle(w, styler, "=== OpenStack SRUs ==="); err != nil {
		return err
	}
	if status.SRU.Error != "" {
		fmt.Fprintf(w, "  %s\n", styler.Error("(unavailable: "+status.SRU.Error+")"))
	} else if status.SRU.Status.SyncedAt.IsZero() {
		fmt.Fprintln(w, " ", styler.Placeholder("(not synced)"))
	} else {
		fmt.Fprintf(w, "  %d targets (synced %s)\n", status.SRU.Status.Targets,
			status.SRU.Status.SyncedAt.Format("2006-01-02 15:04 UTC"))
	}

	return nil
}

func formatSize(bytes int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func formatTimestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

func formatOperationProgress(progress *dto.OperationProgress) string {
	if progress == nil {
		return ""
	}
	parts := []string{}
	if progress.Phase != "" {
		parts = append(parts, progress.Phase)
	}
	if progress.Message != "" {
		parts = append(parts, progress.Message)
	}
	if progress.Indeterminate {
		parts = append(parts, "indeterminate")
	} else if progress.Total > 0 {
		parts = append(parts, fmt.Sprintf("%d/%d", progress.Current, progress.Total))
	}
	return strings.Join(parts, " | ")
}
