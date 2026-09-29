package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/gboutry/sunbeam-watchtower/internal/adapter/primary/frontend"
	"github.com/spf13/cobra"
)

const (
	cacheTypeGit           = "git"
	cacheTypePackagesIndex = "packages-index"
	cacheTypePackageSets   = "packagesets"
	cacheTypeUpstreamRepos = "upstream-repos"
	cacheTypeBugs          = "bugs"
	cacheTypeExcuses       = "excuses"
	cacheTypeReleases      = "releases"
	cacheTypeReviews       = "reviews"
)

var allCacheTypes = []string{cacheTypeGit, cacheTypePackagesIndex, cacheTypePackageSets, cacheTypeUpstreamRepos, cacheTypeBugs, cacheTypeExcuses, cacheTypeReleases, cacheTypeReviews}

func validateCacheTypes(args []string) error {
	for _, arg := range args {
		if !slices.Contains(allCacheTypes, arg) {
			return fmt.Errorf("unknown cache type %q (valid: %s)", arg, strings.Join(allCacheTypes, ", "))
		}
	}
	return nil
}

func wantCacheType(args []string, typ string) bool {
	return len(args) == 0 || slices.Contains(args, typ)
}

func newCacheCmd(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Manage local caches (git repos, APT sources)",
	}

	cmd.AddCommand(newCacheSyncCmd(opts))
	cmd.AddCommand(newCacheClearCmd(opts))
	cmd.AddCommand(newCacheStatusCmd(opts))
	return cmd
}

func newCacheSyncCmd(opts *Options) *cobra.Command {
	var projects []string
	var distros, releases, backports []string
	var trackers []string
	var sets []string
	var since string

	cmd := withActionSelector(&cobra.Command{
		Use:   "sync [types...]",
		Short: "Sync caches (git repos and/or APT packages index)",
		Long: fmt.Sprintf("Sync one or more cache types. If no types are given, all are synced.\n\nValid types: %s",
			strings.Join(allCacheTypes, ", ")),
		ValidArgs: allCacheTypes,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateCacheTypes(args); err != nil {
				return err
			}
			workflow := opts.Frontend().Cache()

			progressOut := opts.Out
			if opts.Output == "json" || opts.Output == "yaml" {
				progressOut = opts.ErrOut
			}
			styler := newOutputStylerForOptions(opts, progressOut, opts.Output)
			errStyler := newOutputStylerForOptions(opts, opts.ErrOut, opts.Output)

			if wantCacheType(args, cacheTypeGit) {
				fmt.Fprintf(progressOut, "%s git caches...\n", styler.Action("syncing"))
				result, err := workflow.SyncGit(cmd.Context(), frontend.CacheSyncGitRequest{Projects: projects})
				if err != nil {
					return err
				}
				for _, w := range result.Warnings {
					if err := writeWarningLine(opts.ErrOut, errStyler, w); err != nil {
						return err
					}
				}
				fmt.Fprintf(progressOut, "git sync %s (%d repos synced).\n", styler.Action("done"), result.Synced)
			}

			if wantCacheType(args, cacheTypePackagesIndex) {
				fmt.Fprintf(progressOut, "%s packages index...\n", styler.Action("syncing"))
				if err := workflow.SyncPackagesIndex(cmd.Context(), frontend.CacheSyncPackagesIndexRequest{
					Distros:   distros,
					Releases:  releases,
					Backports: backports,
				}); err != nil {
					return err
				}
				fmt.Fprintf(progressOut, "packages index sync %s.\n", styler.Action("done"))
			}

			if wantCacheType(args, cacheTypePackageSets) {
				fmt.Fprintf(progressOut, "%s Launchpad packagesets...\n", styler.Action("syncing"))
				results, err := workflow.SyncPackageSets(cmd.Context(), frontend.CacheSyncPackageSetsRequest{Sets: sets})
				if err != nil {
					return err
				}
				for _, result := range results {
					fmt.Fprintf(progressOut, "%s: %d packages from %s\n", result.Name, result.PackageCount, result.Series)
				}
				fmt.Fprintf(progressOut, "packagesets sync %s (%d sets).\n", styler.Action("done"), len(results))
			}

			if wantCacheType(args, cacheTypeUpstreamRepos) {
				fmt.Fprintf(progressOut, "%s upstream repos...\n", styler.Action("syncing"))
				result, err := workflow.SyncUpstream(cmd.Context())
				if err != nil {
					return err
				}
				fmt.Fprintf(progressOut, "upstream repos sync: %s\n", styler.semantic(result.Status))
			}

			if wantCacheType(args, cacheTypeBugs) {
				fmt.Fprintf(progressOut, "%s bug caches...\n", styler.Action("syncing"))
				result, err := workflow.SyncBugs(cmd.Context(), frontend.CacheSyncBugsRequest{Projects: projects})
				if err != nil {
					return err
				}
				fmt.Fprintf(progressOut, "bug cache sync %s (%d tasks synced).\n", styler.Action("done"), result.Synced)
				if len(result.RebuiltProjects) > 0 {
					fmt.Fprintf(progressOut, "bug cache rebuilt for compatibility: %s\n", strings.Join(result.RebuiltProjects, ", "))
				}
			}

			if wantCacheType(args, cacheTypeExcuses) {
				fmt.Fprintf(progressOut, "%s excuses caches...\n", styler.Action("syncing"))
				result, err := workflow.SyncExcuses(cmd.Context(), frontend.CacheSyncExcusesRequest{Trackers: trackers})
				if err != nil {
					return err
				}
				fmt.Fprintf(progressOut, "excuses cache sync: %s\n", styler.semantic(result.Status))
			}

			if wantCacheType(args, cacheTypeReleases) {
				fmt.Fprintf(progressOut, "%s release caches...\n", styler.Action("syncing"))
				result, err := workflow.SyncReleases(cmd.Context())
				if err != nil {
					return err
				}
				for _, w := range result.Warnings {
					if err := writeWarningLine(opts.ErrOut, errStyler, w); err != nil {
						return err
					}
				}
				fmt.Fprintf(progressOut, "release cache sync: %s (discovered %d, synced %d, skipped %d)\n",
					styler.semantic(result.Status), result.Discovered, result.Synced, result.Skipped)
			}

			if wantCacheType(args, cacheTypeReviews) {
				fmt.Fprintf(progressOut, "%s review caches...\n", styler.Action("syncing"))
				result, err := workflow.SyncReviews(cmd.Context(), frontend.CacheSyncReviewsRequest{Projects: projects, Since: since})
				if err != nil {
					return err
				}
				for _, w := range result.Warnings {
					if err := writeWarningLine(opts.ErrOut, errStyler, w); err != nil {
						return err
					}
				}
				fmt.Fprintf(progressOut, "review cache sync %s (%d projects, %d summaries, %d details).\n",
					styler.Action("done"), result.ProjectsSynced, result.SummariesSynced, result.DetailsSynced)
			}

			return nil
		},
	}, "cache.sync")

	cmd.Flags().StringSliceVar(&projects, "project", nil, "sync only these projects (repeat flag; git/bugs/reviews only)")
	cmd.Flags().StringSliceVar(&distros, "distro", nil, "distros to update (packages-index only, default: all configured)")
	cmd.Flags().StringSliceVar(&releases, "release", nil, "distro release names to sync (packages-index only, default: all configured)")
	cmd.Flags().StringSliceVar(&backports, "backport", nil, "configured backport sources to sync (packages-index only, default: all)")
	cmd.Flags().StringSliceVar(&sets, "set", nil, "Launchpad packagesets to sync (packagesets only, default: all configured)")
	cmd.Flags().StringSliceVar(&trackers, "tracker", nil, "excuses trackers to sync (excuses only, default: all configured trackers)")
	cmd.Flags().StringVar(&since, "since", "", "sync full review detail for closed reviews updated since this time (reviews only)")

	return cmd
}

func newCacheClearCmd(opts *Options) *cobra.Command {
	var projects []string
	var trackers []string

	cmd := withActionID(&cobra.Command{
		Use:   "clear [types...]",
		Short: "Clear cached data (git repos and/or APT packages index)",
		Long: fmt.Sprintf("Clear one or more cache types. If no types are given, all are cleared.\n\nValid types: %s",
			strings.Join(allCacheTypes, ", ")),
		ValidArgs: allCacheTypes,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateCacheTypes(args); err != nil {
				return err
			}
			workflow := opts.Frontend().Cache()

			progressOut := opts.Out
			if opts.Output == "json" || opts.Output == "yaml" {
				progressOut = opts.ErrOut
			}
			styler := newOutputStylerForOptions(opts, progressOut, opts.Output)

			if wantCacheType(args, cacheTypeGit) {
				fmt.Fprintf(progressOut, "%s git cache...\n", styler.Action("clearing"))
				if err := workflow.Clear(cmd.Context(), frontend.CacheClearRequest{Type: "git", Projects: projects}); err != nil {
					return err
				}
				fmt.Fprintf(progressOut, "git cache %s.\n", styler.Action("cleared"))
			}

			if wantCacheType(args, cacheTypePackagesIndex) {
				fmt.Fprintf(progressOut, "%s packages index cache...\n", styler.Action("clearing"))
				if err := workflow.Clear(cmd.Context(), frontend.CacheClearRequest{Type: "packages-index"}); err != nil {
					return err
				}
				fmt.Fprintf(progressOut, "packages index cache %s.\n", styler.Action("cleared"))
			}

			if wantCacheType(args, cacheTypePackageSets) {
				fmt.Fprintf(progressOut, "%s packagesets cache...\n", styler.Action("clearing"))
				if err := workflow.Clear(cmd.Context(), frontend.CacheClearRequest{Type: cacheTypePackageSets}); err != nil {
					return err
				}
				fmt.Fprintf(progressOut, "packagesets cache %s.\n", styler.Action("cleared"))
			}

			if wantCacheType(args, cacheTypeUpstreamRepos) {
				fmt.Fprintf(progressOut, "%s upstream repos cache...\n", styler.Action("clearing"))
				if err := workflow.Clear(cmd.Context(), frontend.CacheClearRequest{Type: "upstream-repos"}); err != nil {
					return err
				}
				fmt.Fprintf(progressOut, "upstream repos cache %s.\n", styler.Action("cleared"))
			}

			if wantCacheType(args, cacheTypeBugs) {
				fmt.Fprintf(progressOut, "%s bug cache...\n", styler.Action("clearing"))
				if err := workflow.Clear(cmd.Context(), frontend.CacheClearRequest{Type: "bugs", Projects: projects}); err != nil {
					return err
				}
				fmt.Fprintf(progressOut, "bug cache %s.\n", styler.Action("cleared"))
			}

			if wantCacheType(args, cacheTypeExcuses) {
				fmt.Fprintf(progressOut, "%s excuses cache...\n", styler.Action("clearing"))
				if err := workflow.Clear(cmd.Context(), frontend.CacheClearRequest{Type: "excuses", Trackers: trackers}); err != nil {
					return err
				}
				fmt.Fprintf(progressOut, "excuses cache %s.\n", styler.Action("cleared"))
			}

			if wantCacheType(args, cacheTypeReleases) {
				fmt.Fprintf(progressOut, "%s release cache...\n", styler.Action("clearing"))
				if err := workflow.Clear(cmd.Context(), frontend.CacheClearRequest{Type: "releases"}); err != nil {
					return err
				}
				fmt.Fprintf(progressOut, "release cache %s.\n", styler.Action("cleared"))
			}

			if wantCacheType(args, cacheTypeReviews) {
				fmt.Fprintf(progressOut, "%s review cache...\n", styler.Action("clearing"))
				if err := workflow.Clear(cmd.Context(), frontend.CacheClearRequest{Type: "reviews", Projects: projects}); err != nil {
					return err
				}
				fmt.Fprintf(progressOut, "review cache %s.\n", styler.Action("cleared"))
			}

			return nil
		},
	}, frontend.ActionCacheClear)

	cmd.Flags().StringSliceVar(&projects, "project", nil, "clear only these projects (repeat flag; git/bugs/reviews only)")
	cmd.Flags().StringSliceVar(&trackers, "tracker", nil, "excuses trackers to clear (excuses only, default: all configured trackers)")
	return cmd
}

func newCacheStatusCmd(opts *Options) *cobra.Command {
	return withActionID(&cobra.Command{
		Use:   "status",
		Short: "Show size and freshness of all cached data",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Logger.Debug("listing cache status")

			result, err := opts.Frontend().Cache().Status(cmd.Context())
			if err != nil {
				return err
			}

			status := cacheFullStatus{}
			status.Git.Directory = result.Git.Directory
			for _, r := range result.Git.Repos {
				status.Git.Repos = append(status.Git.Repos, cacheEntry{Name: r.Name, Size: r.Size})
			}
			status.Packages.Directory = result.Packages.Directory
			status.Packages.Sources = result.Packages.Sources
			status.Packages.Error = result.Packages.Error
			status.PackageSets.Entries = append(status.PackageSets.Entries, result.PackageSets.Entries...)
			status.PackageSets.Error = result.PackageSets.Error
			status.Upstream.Directory = result.Upstream.Directory
			for _, r := range result.Upstream.Repos {
				status.Upstream.Repos = append(status.Upstream.Repos, cacheEntry{Name: r.Name, Size: r.Size})
			}

			status.Bugs.Entries = append(status.Bugs.Entries, result.Bugs.Entries...)
			status.Bugs.Directory = result.Bugs.Directory
			status.Bugs.Error = result.Bugs.Error
			status.Excuses.Entries = append(status.Excuses.Entries, result.Excuses.Entries...)
			status.Excuses.Directory = result.Excuses.Directory
			status.Excuses.Error = result.Excuses.Error
			status.Releases.Entries = append(status.Releases.Entries, result.Releases.Entries...)
			status.Releases.Directory = result.Releases.Directory
			status.Releases.Error = result.Releases.Error
			status.Reviews.Entries = append(status.Reviews.Entries, result.Reviews.Entries...)
			status.Reviews.Directory = result.Reviews.Directory
			status.Reviews.Error = result.Reviews.Error

			return renderCacheFullStatus(opts.Out, opts.Output, newOutputStylerForOptions(opts, opts.Out, opts.Output), &status)
		},
	}, frontend.ActionCacheStatus)
}
