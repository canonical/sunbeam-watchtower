// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	distro "github.com/canonical/sunbeam-watchtower/pkg/distro/v1"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	"github.com/spf13/cobra"
)

func newPackagesCmd(opts *Options) *cobra.Command {
	cmd := withActionID(&cobra.Command{
		Use:   "packages",
		Short: "Compare source package versions across distros",
	}, frontend.ActionPackagesDsc)
	cmd.AddCommand(
		newPackagesDiffCmd(opts),
		newPackagesShowCmd(opts),
		newPackagesShowDetailCmd(opts),
		newPackagesExcusesCmd(opts),
		newPackagesListCmd(opts),
		newPackagesDscCmd(opts),
		newPackagesRdependsCmd(opts),
	)
	return cmd
}

func newPackagesDiffCmd(opts *Options) *cobra.Command {
	var distros, releases, backports []string
	var suites, components []string
	var merge bool
	var upstreamRelease string
	var behindUpstream bool
	var onlyIn string
	var constraints string

	cmd := withActionID(&cobra.Command{
		Use:   "diff <set>",
		Short: "Compare package versions across distros",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			setName := args[0]
			ctx := cmd.Context()
			workflow := opts.Frontend().Packages()

			response, err := workflow.Diff(ctx, frontend.PackagesDiffRequest{
				Set:             setName,
				Distros:         distros,
				Releases:        releases,
				Suites:          suites,
				Backports:       backports,
				Merge:           merge,
				UpstreamRelease: upstreamRelease,
				BehindUpstream:  behindUpstream,
				OnlyIn:          onlyIn,
				Constraints:     constraints,
			})
			if err != nil {
				return err
			}

			return renderDiffResults(opts.Out, opts.Output, newOutputStylerForOptions(opts, opts.Out, opts.Output), response.Results, response.Sources, response.Merge, response.HasUpstream)
		},
	}, frontend.ActionPackagesDiff)

	cmd.Flags().StringSliceVar(&distros, "distro", nil, "distros to query (default: all configured)")
	cmd.Flags().StringSliceVar(&releases, "release", nil, "distro releases to query (default: all configured)")
	cmd.Flags().StringSliceVar(&backports, "backport", []string{"none"}, "backports to include (default: none; pass names to include)")
	cmd.Flags().StringSliceVar(&suites, "suite", nil, "filter by suite type within release (release, updates, proposed)")
	cmd.Flags().StringSliceVar(&components, "component", nil, "filter by component")
	cmd.Flags().BoolVar(&merge, "merge", false, "merge suites per source into a single column showing highest version")
	cmd.Flags().StringVar(&upstreamRelease, "upstream-release", "", "upstream release to compare against (e.g. 2025.1)")
	cmd.Flags().BoolVar(&behindUpstream, "behind-upstream", false, "show only packages where distro version is behind upstream")
	cmd.Flags().StringVar(&onlyIn, "only-in", "", "show only packages present in the named source")
	cmd.Flags().StringVar(&constraints, "constraints", "", "include upper-constraints packages for the given release")

	return cmd
}

func newPackagesShowCmd(opts *Options) *cobra.Command {
	var distros, releases, backports []string
	var merge bool
	var upstreamRelease string

	cmd := withActionID(&cobra.Command{
		Use:   "show-version <package>",
		Short: "Show all versions of a package across distros",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkgName := args[0]
			ctx := cmd.Context()
			workflow := opts.Frontend().Packages()

			response, err := workflow.ShowVersion(ctx, frontend.PackagesShowVersionRequest{
				Package:         pkgName,
				Distros:         distros,
				Releases:        releases,
				Backports:       backports,
				Merge:           merge,
				UpstreamRelease: upstreamRelease,
			})
			if err != nil {
				return err
			}

			return renderDiffResults(opts.Out, opts.Output, newOutputStylerForOptions(opts, opts.Out, opts.Output), []dto.PackageDiffResult{response.Result}, response.Sources, response.Merge, response.HasUpstream)
		},
	}, frontend.ActionPackagesShowVersion)

	cmd.Flags().StringSliceVar(&distros, "distro", nil, "distros to query (default: all configured)")
	cmd.Flags().StringSliceVar(&releases, "release", nil, "distro releases to query (default: all configured)")
	cmd.Flags().StringSliceVar(&backports, "backport", []string{"none"}, "backports to include (default: none; pass names to include)")
	cmd.Flags().BoolVar(&merge, "merge", false, "merge suites per source into a single column showing highest version")
	cmd.Flags().StringVar(&upstreamRelease, "upstream-release", "", "upstream release to compare against (e.g. 2025.1)")

	return cmd
}

func newPackagesShowDetailCmd(opts *Options) *cobra.Command {
	var distros, releases, suites, backports []string

	cmd := withActionID(&cobra.Command{
		Use:   "show <package> [version]",
		Short: "Show full APT metadata for a package",
		Long: `Show all fields from the APT Sources index for a specific package.

If a Debian version string is given as a second argument, returns the exact
match. Otherwise, returns the highest version found across the configured
(or filtered) sources.`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkgName := args[0]
			var version string
			if len(args) > 1 {
				version = args[1]
			}

			workflow := opts.Frontend().Packages()
			result, err := workflow.Detail(cmd.Context(), frontend.PackagesDetailRequest{
				Package:   pkgName,
				Version:   version,
				Distros:   distros,
				Releases:  releases,
				Suites:    suites,
				Backports: backports,
			})
			if err != nil {
				return err
			}

			return renderPackageInfo(opts.Out, opts.Output, newOutputStylerForOptions(opts, opts.Out, opts.Output), result)
		},
	}, frontend.ActionPackagesShowDetail)

	cmd.Flags().StringSliceVar(&distros, "distro", nil, "distros to query (default: all configured)")
	cmd.Flags().StringSliceVar(&releases, "release", nil, "distro releases to query (default: all configured)")
	cmd.Flags().StringSliceVar(&suites, "suite", nil, "suite types to query (default: all)")
	cmd.Flags().StringSliceVar(&backports, "backport", []string{"none"}, "backports to include (default: none)")

	return cmd
}

func newPackagesListCmd(opts *Options) *cobra.Command {
	var distros, releases, backports []string
	var suites, components []string

	cmd := withActionID(&cobra.Command{
		Use:   "list",
		Short: "List packages in a distro",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			workflow := opts.Frontend().Packages()

			result, err := workflow.List(ctx, frontend.PackagesListRequest{
				Distros:    distros,
				Releases:   releases,
				Suites:     suites,
				Components: components,
				Backports:  backports,
			})
			if err != nil {
				return err
			}

			return renderSourcePackages(opts.Out, opts.Output, newOutputStylerForOptions(opts, opts.Out, opts.Output), result)
		},
	}, frontend.ActionPackagesList)

	cmd.Flags().StringSliceVar(&distros, "distro", nil, "distros to query (default: all configured)")
	cmd.Flags().StringSliceVar(&releases, "release", nil, "distro releases to query (default: all configured)")
	cmd.Flags().StringSliceVar(&backports, "backport", []string{"none"}, "backports to include (default: none; pass names to include)")
	cmd.Flags().StringSliceVar(&suites, "suite", nil, "filter by suite type within release (release, updates, proposed)")
	cmd.Flags().StringSliceVar(&components, "component", nil, "filter by component")

	return cmd
}

// renderDiffResults writes diff results in the requested format.
func renderDiffResults(w io.Writer, format string, styler *outputStyler, results []dto.PackageDiffResult, sources []dto.PackageSource, merge bool, hasUpstream bool) error {
	switch format {
	case "json":
		return renderJSON(w, results)
	case "yaml":
		return renderYAML(w, results)
	default:
		if merge {
			return renderMergedDiffTable(w, styler, results, sources, hasUpstream)
		}
		return renderDiffTable(w, styler, results, sources, hasUpstream)
	}
}

// renderDiffTable renders a table with dynamic columns based on queried sources.
func renderDiffTable(w io.Writer, styler *outputStyler, results []dto.PackageDiffResult, sources []dto.PackageSource, hasUpstream bool) error {
	if len(results) == 0 {
		fmt.Fprintln(w, "No results found.")
		return nil
	}

	type column struct {
		source string
		suite  string
		header string
	}

	var columns []column
	for _, src := range sources {
		suites := make(map[string]bool)
		for _, r := range results {
			for _, p := range r.Versions[src.Name] {
				suites[p.Suite] = true
			}
		}
		if len(suites) == 0 {
			columns = append(columns, column{
				source: src.Name,
				suite:  "",
				header: strings.ToUpper(src.Name),
			})
			continue
		}
		for suite := range suites {
			columns = append(columns, column{
				source: src.Name,
				suite:  suite,
				header: strings.ToUpper(src.Name) + ":" + suite,
			})
		}
	}

	headers := []string{"PACKAGE"}
	for _, col := range columns {
		headers = append(headers, col.header)
	}
	if hasUpstream {
		headers = append(headers, "UPSTREAM")
	}
	rows := make([][]string, 0, len(results))

	for _, r := range results {
		row := []string{r.Package}
		for _, col := range columns {
			version := "-"
			for _, p := range r.Versions[col.source] {
				if col.suite == "" || p.Suite == col.suite {
					version = p.Version
					break
				}
			}
			row = append(row, version)
		}
		if hasUpstream {
			up := r.Upstream
			if up == "" {
				up = "—"
			}
			row = append(row, up)
		}
		rows = append(rows, row)
	}

	return renderStyledTable(w, styler, headers, rows)
}

// suiteMarker returns a short origin marker for a suite name.
func suiteMarker(suite string) string {
	s := strings.ToLower(suite)
	switch {
	case strings.Contains(s, "updates"):
		return "U"
	case strings.Contains(s, "proposed"):
		return "P"
	case strings.Contains(s, "security"):
		return "S"
	case strings.Contains(s, "staging"):
		return "S"
	case strings.Contains(s, "unstable"):
		return "U"
	case strings.Contains(s, "experimental"):
		return "E"
	default:
		return "R"
	}
}

// renderMergedDiffTable renders a table with one column per source, showing the
// highest version with origin markers indicating which suites carry it.
func renderMergedDiffTable(w io.Writer, styler *outputStyler, results []dto.PackageDiffResult, sources []dto.PackageSource, hasUpstream bool) error {
	if len(results) == 0 {
		fmt.Fprintln(w, "No results found.")
		return nil
	}

	headers := []string{"PACKAGE"}
	for _, src := range sources {
		headers = append(headers, strings.ToUpper(src.Name))
	}
	if hasUpstream {
		headers = append(headers, "UPSTREAM")
	}
	rows := make([][]string, 0, len(results))

	for _, r := range results {
		row := []string{r.Package}
		for _, src := range sources {
			versions := r.Versions[src.Name]
			if len(versions) == 0 {
				row = append(row, "—")
				continue
			}

			highest := distro.PickHighest(versions)
			var markers []string
			for _, p := range versions {
				if distro.CompareVersions(p.Version, highest.Version) == 0 {
					markers = append(markers, suiteMarker(p.Suite))
				}
			}
			sort.Strings(markers)
			// Deduplicate sorted markers.
			deduped := markers[:0]
			for i, m := range markers {
				if i == 0 || m != markers[i-1] {
					deduped = append(deduped, m)
				}
			}
			row = append(row, highest.Version+" ("+strings.Join(deduped, ",")+")")
		}
		if hasUpstream {
			up := r.Upstream
			if up == "" {
				up = "—"
			}
			row = append(row, up)
		}
		rows = append(rows, row)
	}

	return renderStyledTable(w, styler, headers, rows)
}

// renderSourcePackages writes source packages in the requested format.
func renderSourcePackages(w io.Writer, format string, styler *outputStyler, pkgs []distro.SourcePackage) error {
	switch format {
	case "json":
		return renderJSON(w, pkgs)
	case "yaml":
		return renderYAML(w, pkgs)
	default:
		return renderSourcePackagesTable(w, styler, pkgs)
	}
}

func renderSourcePackagesTable(w io.Writer, styler *outputStyler, pkgs []distro.SourcePackage) error {
	if len(pkgs) == 0 {
		fmt.Fprintln(w, "No packages found.")
		return nil
	}

	headers := []string{"PACKAGE", "VERSION", "SUITE", "COMPONENT"}
	rows := make([][]string, 0, len(pkgs))
	for _, p := range pkgs {
		rows = append(rows, []string{p.Package, p.Version, p.Suite, p.Component})
	}
	return renderStyledTable(w, styler, headers, rows)
}

func renderCacheStatusTable(w io.Writer, styler *outputStyler, statuses []dto.CacheStatus) error {
	if len(statuses) == 0 {
		fmt.Fprintln(w, "No cached sources found.")
		return nil
	}

	headers := []string{"SOURCE", "PACKAGES", "LAST UPDATED", "SIZE"}
	rows := make([][]string, 0, len(statuses))
	for _, s := range statuses {
		updated := "-"
		if !s.LastUpdated.IsZero() {
			updated = s.LastUpdated.Format("2006-01-02 15:04")
		}
		rows = append(rows, []string{s.Name, fmt.Sprintf("%d", s.EntryCount), updated, formatBytes(s.DiskSize)})
	}
	return renderStyledTable(w, styler, headers, rows)
}

// formatBytes formats a byte count into a human-readable string.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func newPackagesDscCmd(opts *Options) *cobra.Command {
	var distros, releases, backports []string

	cmd := withActionID(&cobra.Command{
		Use:   "dsc <pkg> <version> [<pkg> <version> ...]",
		Short: "Look up .dsc file URLs for source package/version pairs",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 || len(args)%2 != 0 {
				return fmt.Errorf("arguments must be <package> <version> pairs")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			workflow := opts.Frontend().Packages()

			var packages []string
			for i := 0; i < len(args); i += 2 {
				packages = append(packages, args[i]+"="+args[i+1])
			}

			results, err := workflow.Dsc(ctx, frontend.PackagesDscRequest{
				Packages:  packages,
				Distros:   distros,
				Releases:  releases,
				Backports: backports,
			})
			if err != nil {
				return err
			}

			return renderDscResults(opts.Out, opts.Output, newOutputStylerForOptions(opts, opts.Out, opts.Output), results)
		},
	}, frontend.ActionPackagesRdepends)

	cmd.Flags().StringSliceVar(&distros, "distro", nil, "distros to query (default: all configured)")
	cmd.Flags().StringSliceVar(&releases, "release", nil, "distro releases to query (default: all configured)")
	cmd.Flags().StringSliceVar(&backports, "backport", []string{"none"}, "backports to include (default: none; pass names to include)")

	return cmd
}

// renderDscResults writes dsc lookup results in the requested format.
func renderDscResults(w io.Writer, format string, styler *outputStyler, results []dto.PackageDscResult) error {
	switch format {
	case "json":
		return renderJSON(w, results)
	case "yaml":
		return renderYAML(w, results)
	default:
		return renderDscTable(w, styler, results)
	}
}

func renderDscTable(w io.Writer, styler *outputStyler, results []dto.PackageDscResult) error {
	if len(results) == 0 {
		fmt.Fprintln(w, "No results.")
		return nil
	}

	headers := []string{"PACKAGE", "VERSION", "DSC URL"}
	rows := make([][]string, 0)
	for _, r := range results {
		if len(r.URLs) == 0 {
			rows = append(rows, []string{r.Package, r.Version, "(not found)"})
			continue
		}
		for i, u := range r.URLs {
			if i == 0 {
				rows = append(rows, []string{r.Package, r.Version, u})
			} else {
				rows = append(rows, []string{"", "", u})
			}
		}
	}
	return renderStyledTable(w, styler, headers, rows)
}

func newPackagesRdependsCmd(opts *Options) *cobra.Command {
	var distros, releases, backports []string
	var suites []string

	cmd := withActionID(&cobra.Command{
		Use:   "rdepends <package>",
		Short: "Find source packages that build-depend on a given package",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkgName := args[0]
			ctx := cmd.Context()
			workflow := opts.Frontend().Packages()

			results, err := workflow.Rdepends(ctx, frontend.PackagesRdependsRequest{
				Package:   pkgName,
				Distros:   distros,
				Releases:  releases,
				Suites:    suites,
				Backports: backports,
			})
			if err != nil {
				return err
			}

			if len(results) == 0 {
				styler := newOutputStylerForOptions(opts, opts.Out, opts.Output)
				fmt.Fprintf(opts.Out, "%s %q was not found as a source package; it may be a binary package name.\n", styler.Warning("Warning:"), pkgName)
				fmt.Fprintf(opts.Out, "%s rdepends searches Build-Depends which reference source package names.\n", styler.Dim("Hint:"))
			}

			return renderSourcePackageDetails(opts.Out, opts.Output, newOutputStylerForOptions(opts, opts.Out, opts.Output), results)
		},
	}, frontend.ActionPackagesRdepends)

	cmd.Flags().StringSliceVar(&distros, "distro", nil, "distros to query (default: all configured)")
	cmd.Flags().StringSliceVar(&releases, "release", nil, "distro releases to query (default: all configured)")
	cmd.Flags().StringSliceVar(&backports, "backport", []string{"none"}, "backports to include (default: none; pass names to include)")
	cmd.Flags().StringSliceVar(&suites, "suite", nil, "filter by suite type within release (release, updates, proposed)")

	return cmd
}

// renderSourcePackageDetails writes SourcePackageDetail entries in the requested format.
func renderSourcePackageDetails(w io.Writer, format string, styler *outputStyler, pkgs []distro.SourcePackageDetail) error {
	switch format {
	case "json":
		return renderJSON(w, pkgs)
	case "yaml":
		return renderYAML(w, pkgs)
	default:
		return renderSourcePackageDetailsTable(w, styler, pkgs)
	}
}

func renderSourcePackageDetailsTable(w io.Writer, styler *outputStyler, pkgs []distro.SourcePackageDetail) error {
	if len(pkgs) == 0 {
		fmt.Fprintln(w, "No reverse dependencies found.")
		return nil
	}

	headers := []string{"PACKAGE", "VERSION", "SUITE", "COMPONENT"}
	rows := make([][]string, 0, len(pkgs))
	for _, p := range pkgs {
		rows = append(rows, []string{p.Package, p.Version, p.Suite, p.Component})
	}
	return renderStyledTable(w, styler, headers, rows)
}

func renderPackageInfo(w io.Writer, format string, styler *outputStyler, info *distro.SourcePackageInfo) error {
	switch format {
	case "json":
		return renderJSON(w, info)
	case "yaml":
		return renderYAML(w, info)
	default:
		return renderPackageInfoTable(w, styler, info)
	}
}

func renderPackageInfoTable(w io.Writer, styler *outputStyler, info *distro.SourcePackageInfo) error {
	if err := writeKeyValue(w, styler, "Source", info.Package); err != nil {
		return err
	}
	if err := writeKeyValue(w, styler, "Version", info.Version); err != nil {
		return err
	}
	if err := writeKeyValue(w, styler, "Suite", info.Suite); err != nil {
		return err
	}
	if err := writeKeyValue(w, styler, "Component", info.Component); err != nil {
		return err
	}
	fmt.Fprintln(w, styler.Dim("---"))
	for _, f := range info.Fields {
		if f.Key == "Package" || f.Key == "Version" {
			continue
		}
		value := f.Value
		if strings.Contains(value, "\n") {
			lines := strings.Split(value, "\n")
			if err := writeKeyValue(w, styler, f.Key, lines[0]); err != nil {
				return err
			}
			for _, line := range lines[1:] {
				fmt.Fprintf(w, "%s %s\n", strings.Repeat(" ", detailLabelWidth+1), line)
			}
		} else {
			if err := writeKeyValue(w, styler, f.Key, value); err != nil {
				return err
			}
		}
	}
	return nil
}
