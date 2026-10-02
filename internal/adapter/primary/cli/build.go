package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	"github.com/spf13/cobra"
)

func newBuildCmd(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Manage Launchpad builds",
	}
	cmd.AddCommand(
		newBuildTriggerCmd(opts),
		newBuildListCmd(opts),
		newBuildDownloadCmd(opts),
		newBuildCleanupCmd(opts),
	)
	return cmd
}

func newBuildTriggerCmd(opts *Options) *cobra.Command {
	var owner, prefix, localPath, artifactsDir string
	var wait, download, async bool
	var timeout time.Duration
	var retryCount int

	cmd := withActionID(&cobra.Command{
		Use:   "trigger <project> [artifacts...]",
		Short: "Trigger builds for a project",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectName := args[0]
			artifactNames := args[1:]
			styler := newOutputStylerForOptions(opts, opts.Out, opts.Output)

			// --download implies --wait.
			if download {
				wait = true
			}
			if async && wait {
				return fmt.Errorf("--async cannot be combined with --wait or --download")
			}
			if retryCount < 1 {
				return fmt.Errorf("--retry must be >= 1")
			}
			if retryCount > 1 && !wait {
				return fmt.Errorf("--retry requires --wait (or --download, which implies --wait)")
			}
			if retryCount > 1 && async {
				return fmt.Errorf("--retry cannot be combined with --async")
			}

			// Determine source mode: local when --local-path is explicitly set.
			source := ""
			if cmd.Flags().Changed("local-path") {
				source = "local"
			}

			buildsFrontend := opts.Frontend().Builds()
			if source == "local" {
				if err := opts.Frontend().LocalBuildPreparationError(); err != nil {
					return err
				}
			}

			response, err := buildsFrontend.Trigger(cmd.Context(), frontend.BuildTriggerRequest{
				Source:       source,
				LocalPath:    localPath,
				Async:        async,
				Download:     download,
				ArtifactsDir: artifactsDir,
				Project:      projectName,
				Artifacts:    artifactNames,
				Wait:         wait,
				Timeout:      timeout,
				Owner:        owner,
				Prefix:       prefix,
				RetryCount:   retryCount,
			})
			if err != nil {
				return err
			}

			if async {
				return renderOperationJob(opts.Out, opts.Output, styler, response.Job)
			}

			if len(response.Requests) > 0 {
				if err := renderBuildRequests(opts.Out, opts.Output, styler, response.Requests); err != nil {
					return err
				}
			}
			if len(response.Builds) > 0 {
				if err := renderBuilds(opts.Out, opts.Output, styler, response.Builds); err != nil {
					return err
				}
			}

			return errors.Join(response.Errors...)
		},
	}, frontend.ActionBuildTrigger)

	cmd.Flags().BoolVar(&wait, "wait", false, "wait for builds to complete")
	cmd.Flags().BoolVar(&download, "download", false, "download artifacts after builds succeed (implies --wait)")
	cmd.Flags().BoolVar(&async, "async", false, "queue the build trigger as a long-running operation")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Hour, "max wait time")
	cmd.Flags().StringVar(&owner, "owner", "", "override LP owner")
	cmd.Flags().StringVar(&prefix, "prefix", "tmp-build", "temp recipe name prefix (local mode)")
	cmd.Flags().StringVar(&localPath, "local-path", "", "path to local git repo (enables local mode)")
	cmd.Flags().StringVar(&artifactsDir, "artifacts-dir", "", "output directory for downloaded artifacts (default from config)")
	cmd.Flags().IntVar(&retryCount, "retry", 1, "max attempts per individual build when --wait is set, and per artifact download when --download is set (1 = no retry)")

	return cmd
}

func newBuildListCmd(opts *Options) *cobra.Command {
	var projects []string
	var all bool
	var state, sha, prefix, owner string

	cmd := withActionID(&cobra.Command{
		Use:   "list [projects...]",
		Short: "List builds across projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Positional args and --project flag are merged.
			allProjects := append(projects, args...)

			// Determine source mode: local when --prefix or --sha are set.
			source := ""
			if cmd.Flags().Changed("prefix") || cmd.Flags().Changed("sha") {
				source = "local"
			}

			buildsFrontend := opts.Frontend().Builds()
			if source == "local" {
				if err := opts.Frontend().LocalBuildPreparationError(); err != nil {
					return err
				}
			}
			builds, err := buildsFrontend.List(cmd.Context(), frontend.BuildListRequest{
				Source:     source,
				SHA:        sha,
				Prefix:     prefix,
				DefaultAll: !cmd.Flags().Changed("all"),
				Projects:   allProjects,
				All:        all,
				State:      state,
				Owner:      owner,
			})
			if err != nil {
				return err
			}

			return renderBuilds(opts.Out, opts.Output, newOutputStylerForOptions(opts, opts.Out, opts.Output), builds)
		},
	}, frontend.ActionBuildList)

	cmd.Flags().StringSliceVar(&projects, "project", nil, "filter by project name")
	cmd.Flags().BoolVar(&all, "all", false, "show all builds (not just active)")
	cmd.Flags().StringVar(&state, "state", "", "filter by state")
	cmd.Flags().StringVar(&sha, "sha", "", "git commit SHA for local build lookup")
	cmd.Flags().StringVar(&prefix, "prefix", "tmp-build", "temp recipe name prefix (local mode)")
	cmd.Flags().StringVar(&owner, "owner", "", "override LP owner")

	return cmd
}

func newBuildDownloadCmd(opts *Options) *cobra.Command {
	var artifactsDir, sha, prefix, owner string
	var retryCount int

	cmd := withActionID(&cobra.Command{
		Use:   "download <project> [artifacts...]",
		Short: "Download build artifacts",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectName := args[0]
			artifactNames := args[1:]
			if retryCount < 1 {
				return fmt.Errorf("--retry must be >= 1")
			}

			// Determine source mode: local when --prefix or --sha are set.
			source := ""
			if cmd.Flags().Changed("prefix") || cmd.Flags().Changed("sha") {
				source = "local"
			}

			buildsFrontend := opts.Frontend().Builds()
			if source == "local" {
				if err := opts.Frontend().LocalBuildPreparationError(); err != nil {
					return err
				}
			}
			return buildsFrontend.Download(cmd.Context(), frontend.BuildDownloadRequest{
				Source:       source,
				SHA:          sha,
				Prefix:       prefix,
				Project:      projectName,
				Artifacts:    artifactNames,
				ArtifactsDir: artifactsDir,
				Owner:        owner,
				RetryCount:   retryCount,
			})
		},
	}, frontend.ActionBuildDownload)

	cmd.Flags().StringVar(&artifactsDir, "artifacts-dir", "", "output directory (default from config)")
	cmd.Flags().IntVar(&retryCount, "retry", 1, "max download attempts per artifact file (1 = no retry)")
	cmd.Flags().StringVar(&sha, "sha", "", "git commit SHA (narrows prefix in local mode)")
	cmd.Flags().StringVar(&prefix, "prefix", "tmp-build-", "temp recipe name prefix (local mode)")
	cmd.Flags().StringVar(&owner, "owner", "", "override LP owner")

	return cmd
}

func newBuildCleanupCmd(opts *Options) *cobra.Command {
	var project, owner, prefix string
	var dryRun bool

	cmd := withActionSelector(&cobra.Command{
		Use:   "cleanup",
		Short: "Delete temporary build recipes",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := opts.Frontend().Builds().Cleanup(cmd.Context(), frontend.BuildCleanupRequest{
				Project: project,
				Owner:   owner,
				Prefix:  prefix,
				DryRun:  dryRun,
			})
			if err != nil {
				return err
			}

			styler := newOutputStylerForOptions(opts, opts.Out, opts.Output)
			if len(result.DeletedRecipes) > 0 {
				fmt.Fprintln(opts.Out, "Deleted Recipes:")
				if err := renderStringList(opts.Out, opts.Output, styler, result.DeletedRecipes); err != nil {
					return err
				}
			}
			if len(result.DeletedBranches) > 0 {
				fmt.Fprintln(opts.Out, "Deleted Branches:")
				if err := renderStringList(opts.Out, opts.Output, styler, result.DeletedBranches); err != nil {
					return err
				}
			}
			if len(result.DeletedRecipes) == 0 && len(result.DeletedBranches) == 0 {
				fmt.Fprintln(opts.Out, "No resources to clean up.")
			}
			return nil
		},
	}, "build.cleanup")

	cmd.Flags().StringVar(&project, "project", "", "filter to specific project")
	cmd.Flags().StringVar(&owner, "owner", "", "LP owner")
	cmd.Flags().StringVar(&prefix, "prefix", "tmp-build", "recipe name prefix to match")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be deleted")

	return cmd
}
