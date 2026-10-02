// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	"github.com/spf13/cobra"
)

func newReleasesCmd(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "releases",
		Short: "Track published snaps and charms across tracks and risks",
	}
	cmd.AddCommand(newReleasesListCmd(opts), newReleasesShowCmd(opts))
	return cmd
}

func newReleasesListCmd(opts *Options) *cobra.Command {
	var names, projects, tracks, branches, risks []string
	var artifactType, targetProfile string
	var allTargets bool

	cmd := withActionID(&cobra.Command{
		Use:   "list [names...]",
		Short: "List cached published snap and charm releases",
		RunE: func(cmd *cobra.Command, args []string) error {
			allNames := append(names, args...)
			results, err := opts.Frontend().Releases().List(cmd.Context(), frontend.ReleasesListRequest{
				Names:         allNames,
				Projects:      projects,
				ArtifactType:  artifactType,
				Tracks:        tracks,
				Branches:      branches,
				Risks:         risks,
				TargetProfile: targetProfile,
				AllTargets:    allTargets,
			})
			if err != nil {
				return err
			}
			return renderReleaseList(opts.Out, opts.Output, newOutputStylerForOptions(opts, opts.Out, opts.Output), results)
		},
	}, frontend.ActionReleaseList)

	cmd.Flags().StringSliceVar(&names, "name", nil, "filter by published artifact name")
	cmd.Flags().StringSliceVar(&projects, "project", nil, "filter by watchtower project")
	cmd.Flags().StringVar(&artifactType, "type", "", "filter by artifact type (snap|charm)")
	cmd.Flags().StringSliceVar(&tracks, "track", nil, "filter by track")
	cmd.Flags().StringSliceVar(&branches, "branch", nil, "filter by branch")
	cmd.Flags().StringSliceVar(&risks, "risk", nil, "filter by risk (edge, beta, candidate, stable)")
	cmd.Flags().StringVar(&targetProfile, "target-profile", "", "local target visibility profile for release targets")
	cmd.Flags().BoolVar(&allTargets, "all-targets", false, "bypass local target visibility filtering")

	return cmd
}

func newReleasesShowCmd(opts *Options) *cobra.Command {
	var artifactType, track, branch, targetProfile string
	var allTargets bool

	cmd := withActionID(&cobra.Command{
		Use:   "show <name>",
		Short: "Show the full cached release matrix for one published artifact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := opts.Frontend().Releases().Show(cmd.Context(), frontend.ReleasesShowRequest{
				Name:          args[0],
				ArtifactType:  artifactType,
				Track:         track,
				Branch:        branch,
				TargetProfile: targetProfile,
				AllTargets:    allTargets,
			})
			if err != nil {
				return err
			}
			return renderReleaseShow(opts.Out, opts.Output, newOutputStylerForOptions(opts, opts.Out, opts.Output), result)
		},
	}, frontend.ActionReleaseShow)

	cmd.Flags().StringVar(&artifactType, "type", "", "artifact type to disambiguate duplicate names (snap|charm)")
	cmd.Flags().StringVar(&track, "track", "", "optional track filter")
	cmd.Flags().StringVar(&branch, "branch", "", "optional branch filter")
	cmd.Flags().StringVar(&targetProfile, "target-profile", "", "local target visibility profile for release targets")
	cmd.Flags().BoolVar(&allTargets, "all-targets", false, "bypass local target visibility filtering")
	return cmd
}
