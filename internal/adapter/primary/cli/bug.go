package cli

import (
	"strings"

	"github.com/gboutry/sunbeam-watchtower/internal/adapter/primary/frontend"
	dto "github.com/gboutry/sunbeam-watchtower/pkg/dto/v1"
	"github.com/spf13/cobra"
)

func newBugCmd(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bug",
		Short: "Manage bugs across trackers",
	}

	cmd.AddCommand(newBugListCmd(opts))
	cmd.AddCommand(newBugSearchCmd(opts))
	cmd.AddCommand(newBugShowCmd(opts))
	cmd.AddCommand(newBugSyncCmd(opts))
	return cmd
}

func newBugSearchCmd(opts *Options) *cobra.Command {
	var (
		mode           string
		fields         []string
		phrases        []string
		required       []string
		excluded       []string
		fuzzy          bool
		caseSensitive  bool
		projects       []string
		trackers       []string
		status         []string
		importance     []string
		assignee       string
		tags           []string
		closed         string
		createdAfter   string
		createdBefore  string
		modifiedAfter  string
		modifiedBefore string
		noMerge        bool
		sortBy         string
		limit          int
		relatedLimit   int
		evidenceLimit  int
		showRelated    bool
	)

	cmd := withActionID(&cobra.Command{
		Use:   "search [query words...]",
		Short: "Search cached bug text with match explanations",
		RunE: func(cmd *cobra.Command, args []string) error {
			req := dto.BugSearchRequest{
				Query:          strings.Join(args, " "),
				Mode:           mode,
				Fields:         fields,
				Phrases:        phrases,
				RequiredTerms:  required,
				ExcludedTerms:  excluded,
				CaseSensitive:  caseSensitive,
				Projects:       projects,
				Trackers:       trackers,
				Status:         status,
				Importance:     importance,
				Assignee:       assignee,
				Tags:           tags,
				Closed:         closed,
				CreatedAfter:   createdAfter,
				CreatedBefore:  createdBefore,
				ModifiedAfter:  modifiedAfter,
				ModifiedBefore: modifiedBefore,
				Sort:           sortBy,
				Limit:          limit,
				RelatedLimit:   &relatedLimit,
				EvidenceLimit:  evidenceLimit,
			}
			merge := !noMerge
			req.Merge = &merge
			if cmd.Flags().Changed("fuzzy") {
				req.Fuzzy = &fuzzy
			}

			result, err := opts.Frontend().Bugs().Search(cmd.Context(), req)
			if err != nil {
				return err
			}
			errStyler := newOutputStylerForOptions(opts, opts.ErrOut, opts.Output)
			for _, warning := range result.Warnings {
				if err := writeWarningLine(opts.ErrOut, errStyler, warning); err != nil {
					return err
				}
			}
			return renderBugSearch(
				opts.Out,
				opts.Output,
				newOutputStylerForOptions(opts, opts.Out, opts.Output),
				result,
				showRelated,
			)
		},
	}, frontend.ActionBugSearch)

	cmd.Flags().StringVar(&mode, "mode", "text", "search mode: text or regex")
	cmd.Flags().StringSliceVar(&fields, "field", nil, "search field (repeatable)")
	cmd.Flags().StringSliceVar(&phrases, "phrase", nil, "required exact phrase (repeatable)")
	cmd.Flags().StringSliceVar(&required, "require", nil, "required term (repeatable)")
	cmd.Flags().StringSliceVar(&excluded, "exclude", nil, "excluded lexical term (repeatable)")
	cmd.Flags().BoolVar(&fuzzy, "fuzzy", true, "enable stemming, aliases, and spelling tolerance")
	cmd.Flags().BoolVar(&caseSensitive, "case-sensitive", false, "make regex matching case-sensitive")
	cmd.Flags().StringSliceVar(&projects, "project", nil, "filter by Watchtower project (repeatable)")
	cmd.Flags().StringSliceVar(&trackers, "tracker", nil, "filter by tracker type (repeatable)")
	cmd.Flags().StringSliceVar(&status, "status", nil, "filter by task status (repeatable)")
	cmd.Flags().StringSliceVar(&importance, "importance", nil, "filter by task importance (repeatable)")
	cmd.Flags().StringVar(&assignee, "assignee", "", "filter by assignee username")
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "require tag (repeatable)")
	cmd.Flags().StringVar(&closed, "closed", "include", "closed tasks: include, exclude, or only")
	cmd.Flags().StringVar(&createdAfter, "created-after", "", "filter tasks created after a date, timestamp, or duration")
	cmd.Flags().StringVar(&createdBefore, "created-before", "", "filter tasks created before a date, timestamp, or duration")
	cmd.Flags().StringVar(&modifiedAfter, "modified-after", "", "filter tasks modified after a date, timestamp, or duration")
	cmd.Flags().StringVar(&modifiedBefore, "modified-before", "", "filter tasks modified before a date, timestamp, or duration")
	cmd.Flags().BoolVar(&noMerge, "no-merge", false, "return one result per matching task")
	cmd.Flags().StringVar(&sortBy, "sort", "relevance", "sort by relevance, modified, created, importance, or status")
	cmd.Flags().IntVar(&limit, "limit", 10, "maximum primary results (1-100)")
	cmd.Flags().IntVar(&relatedLimit, "related-limit", 5, "maximum related results (0-50)")
	cmd.Flags().IntVar(&evidenceLimit, "evidence-limit", 5, "maximum evidence excerpts per result (1-20)")
	cmd.Flags().BoolVar(&showRelated, "show-related", false, "show related results even when primary matches exist")
	return cmd
}

func newBugShowCmd(opts *Options) *cobra.Command {
	cmd := withActionID(&cobra.Command{
		Use:   "show <id>",
		Short: "Show a bug and its tasks",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := opts.Frontend().Bugs().Show(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return renderBugDetail(opts.Out, opts.Output, newOutputStylerForOptions(opts, opts.Out, opts.Output), result)
		},
	}, frontend.ActionBugShow)

	return cmd
}

func newBugListCmd(opts *Options) *cobra.Command {
	var (
		projects   []string
		status     []string
		importance []string
		assignee   string
		tags       []string
		since      string
		merge      bool
		limit      int
	)

	cmd := withActionID(&cobra.Command{
		Use:   "list",
		Short: "List bug tasks across bug trackers",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := opts.Frontend().Bugs().List(cmd.Context(), frontend.BugListRequest{
				Projects:   projects,
				Status:     status,
				Importance: importance,
				Assignee:   assignee,
				Tags:       tags,
				Since:      since,
				Merge:      merge,
				Limit:      limit,
			})
			if err != nil {
				return err
			}
			errStyler := newOutputStylerForOptions(opts, opts.ErrOut, opts.Output)
			for _, w := range result.Warnings {
				if err := writeWarningLine(opts.ErrOut, errStyler, w); err != nil {
					return err
				}
			}
			return renderBugTasks(opts.Out, opts.Output, newOutputStylerForOptions(opts, opts.Out, opts.Output), result.Tasks)
		},
	}, frontend.ActionBugList)

	cmd.Flags().StringSliceVar(&projects, "project", nil, "filter by project name (repeatable)")
	cmd.Flags().StringSliceVar(&status, "status", nil, "filter by status: New, Confirmed, Triaged, In Progress, etc. (repeatable; omit to include all statuses)")
	cmd.Flags().StringSliceVar(&importance, "importance", nil, "filter by importance: Critical, High, Medium, Low, etc. (repeatable)")
	cmd.Flags().StringVar(&assignee, "assignee", "", "filter by assignee username")
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "filter by tag (repeatable)")
	cmd.Flags().StringVar(&since, "since", "", "show only bugs created/modified since (e.g. 2d, 1w, 30m, 2025-01-01)")
	cmd.Flags().BoolVar(&merge, "merge", false, "collapse grouped duplicate bug rows")
	cmd.Flags().IntVar(&limit, "limit", 0, "limit the number of bug rows")

	return cmd
}

func newBugSyncCmd(opts *Options) *cobra.Command {
	var (
		projects []string
		dryRun   bool
		since    string
	)

	cmd := withActionSelector(&cobra.Command{
		Use:   "sync [bug-id...]",
		Short: "Update LP bug statuses from cached commits",
		Long:  "Scans cached commits for LP bug references, ensures mapped project tasks, and updates statuses to Fix Committed or Fix Released when stable snap revision tags prove publication. With bug IDs and --project, the projects explicitly declare the affected scope.",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := opts.Frontend().Bugs().Sync(cmd.Context(), frontend.BugSyncRequest{
				Projects: projects,
				BugIDs:   args,
				DryRun:   dryRun,
				Since:    since,
			})
			if err != nil {
				return err
			}
			errStyler := newOutputStylerForOptions(opts, opts.ErrOut, opts.Output)
			for _, e := range result.Warnings {
				if err := writeWarningLine(opts.ErrOut, errStyler, e); err != nil {
					return err
				}
			}
			return renderBugSyncResult(opts.Out, opts.Output, newOutputStylerForOptions(opts, opts.Out, opts.Output), result.Result, dryRun)
		},
	}, "bug.sync")

	cmd.Flags().StringSliceVar(&projects, "project", nil, "filter by project name (repeatable)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would change without updating")
	cmd.Flags().StringVar(&since, "since", "", "only consider bugs created/modified since (e.g. 2d, 1w, 30m, 2025-01-01)")

	return cmd
}
