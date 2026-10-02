// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	"github.com/spf13/cobra"
)

func newTeamCmd(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "team",
		Short: "Team management commands",
	}
	cmd.AddCommand(newTeamSyncCmd(opts))
	return cmd
}

func newTeamSyncCmd(opts *Options) *cobra.Command {
	var (
		projects []string
		dryRun   bool
		async    bool
	)

	cmd := withActionSelector(&cobra.Command{
		Use:   "sync",
		Short: "Sync LP team members as store collaborators",
		Long:  "Compare Launchpad team membership against Snap Store and Charmhub collaborator lists, reporting discrepancies (dry-run) or sending invites (apply).",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Logger.Debug("team sync command started", "dry_run", dryRun)
			workflow := opts.Frontend().Teams()
			styler := newOutputStylerForOptions(opts, opts.Out, opts.Output)
			errStyler := newOutputStylerForOptions(opts, opts.ErrOut, opts.Output)

			req := dto.TeamSyncRequest{
				Projects: projects,
				DryRun:   dryRun,
			}

			if async {
				job, err := workflow.StartSync(cmd.Context(), req)
				if err != nil {
					return err
				}
				return renderOperationJob(opts.Out, opts.Output, styler, job)
			}

			result, err := workflow.Sync(cmd.Context(), req)
			if err != nil {
				return err
			}

			for _, w := range result.Warnings {
				if err := writeWarningLine(opts.ErrOut, errStyler, w); err != nil {
					return err
				}
			}

			syncResult := &dto.TeamSyncResult{
				Artifacts: result.Artifacts,
				Warnings:  result.Warnings,
			}
			return renderTeamSyncResult(opts.Out, opts.Output, styler, syncResult, dryRun)
		},
	}, "team.sync")

	cmd.Flags().StringSliceVar(&projects, "project", nil, "filter to specific LP project names")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be done without making changes")
	cmd.Flags().BoolVar(&async, "async", false, "queue the team sync as a long-running operation")

	return cmd
}
