package cli

import (
	"fmt"

	"github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	"github.com/spf13/cobra"
)

func newReviewCmd(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "review",
		Short: "Manage merge requests and reviews",
	}

	cmd.AddCommand(newReviewListCmd(opts))
	cmd.AddCommand(newReviewShowCmd(opts))
	return cmd
}

func newReviewShowCmd(opts *Options) *cobra.Command {
	var project string

	cmd := withActionID(&cobra.Command{
		Use:   "show <id>",
		Short: "Show a merge request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Logger.Debug("review show command started", "project", project, "id", args[0])
			if project == "" {
				return fmt.Errorf("--project is required for review show")
			}

			mr, err := opts.Frontend().Reviews().Show(cmd.Context(), project, args[0])
			if err != nil {
				return err
			}

			return renderMergeRequestDetail(opts.Out, opts.Output, newOutputStylerForOptions(opts, opts.Out, opts.Output), mr)
		},
	}, frontend.ActionReviewShow)

	cmd.Flags().StringVar(&project, "project", "", "project name (required)")

	return cmd
}

func newReviewListCmd(opts *Options) *cobra.Command {
	var (
		projects []string
		forges   []string
		state    string
		author   string
		since    string
	)

	cmd := withActionID(&cobra.Command{
		Use:   "list",
		Short: "List merge requests across forges",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Logger.Debug("review list command started",
				"projects", projects,
				"forges", forges,
				"state", state,
				"author", author,
				"since", since,
			)

			result, err := opts.Frontend().Reviews().List(cmd.Context(), frontend.ReviewListRequest{
				Projects: projects,
				Forges:   forges,
				State:    state,
				Author:   author,
				Since:    since,
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

			return renderMergeRequests(opts.Out, opts.Output, newOutputStylerForOptions(opts, opts.Out, opts.Output), result.MergeRequests)
		},
	}, frontend.ActionReviewList)

	cmd.Flags().StringSliceVar(&projects, "project", nil, "filter by project name (repeatable)")
	cmd.Flags().StringSliceVar(&forges, "forge", nil, "filter by forge type: github, launchpad, gerrit (repeatable)")
	cmd.Flags().StringVar(&state, "state", "", "filter by state: open, merged, closed, wip, abandoned")
	cmd.Flags().StringVar(&author, "author", "", "filter by author")
	cmd.Flags().StringVar(&since, "since", "", "show only MRs updated since (e.g. 2d, 1w, 30m, 2025-01-01)")

	return cmd
}
