// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	frontend "github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	"github.com/spf13/cobra"
)

func newProjectCmd(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage LP project metadata",
	}
	cmd.AddCommand(newProjectSyncCmd(opts))
	return cmd
}

func newProjectSyncCmd(opts *Options) *cobra.Command {
	var (
		projects []string
		dryRun   bool
		async    bool
	)

	cmd := withActionSelector(&cobra.Command{
		Use:   "sync",
		Short: "Ensure LP projects have declared series and development focus",
		Long:  "Iterates over all unique LP projects from bug tracker config entries, ensures each declared series exists (creating if missing), and sets the development focus to the configured series.",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Logger.Debug("project sync command started", "dry_run", dryRun)
			workflow := opts.Frontend().Projects()
			styler := newOutputStylerForOptions(opts, opts.Out, opts.Output)
			errStyler := newOutputStylerForOptions(opts, opts.ErrOut, opts.Output)
			request := frontend.ProjectSyncRequest{
				Projects: projects,
				DryRun:   dryRun,
			}

			if async {
				job, err := workflow.StartSync(cmd.Context(), request)
				if err != nil {
					return err
				}
				return renderOperationJob(opts.Out, opts.Output, styler, job)
			}

			result, err := workflow.Sync(cmd.Context(), request)
			if err != nil {
				return err
			}

			for _, e := range result.Errors {
				if err := writeErrorLine(opts.ErrOut, errStyler, e); err != nil {
					return err
				}
			}

			syncResult := &dto.ProjectSyncResult{
				Actions: result.Actions,
			}
			return renderProjectSyncResult(opts.Out, opts.Output, styler, syncResult, dryRun)
		},
	}, "project.sync")

	cmd.Flags().StringSliceVar(&projects, "project", nil, "filter to specific LP project names")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be done without making changes")
	cmd.Flags().BoolVar(&async, "async", false, "queue the project sync as a long-running operation")

	return cmd
}
