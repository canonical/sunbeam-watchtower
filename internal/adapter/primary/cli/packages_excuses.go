package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	"github.com/spf13/cobra"
)

func newPackagesExcusesCmd(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "excuses",
		Short: "Inspect package migration excuses",
	}
	cmd.AddCommand(
		newPackagesExcusesListCmd(opts),
		newPackagesExcusesShowCmd(opts),
	)
	return cmd
}

func newPackagesExcusesListCmd(opts *Options) *cobra.Command {
	var trackers []string
	var name, component, team, blockedBy, set, blockedBySet string
	var ftbfs, autopkgtest, bugged, reverse bool
	var minAge, maxAge, limit int

	cmd := withActionID(&cobra.Command{
		Use:   "list",
		Short: "List package migration excuses",
		RunE: func(cmd *cobra.Command, args []string) error {
			results, err := opts.Frontend().Packages().ExcusesList(cmd.Context(), frontend.PackagesExcusesListRequest{
				Trackers:     trackers,
				Name:         name,
				Component:    component,
				Team:         team,
				FTBFS:        ftbfs,
				Autopkgtest:  autopkgtest,
				BlockedBy:    blockedBy,
				Bugged:       bugged,
				MinAge:       minAge,
				MaxAge:       maxAge,
				Limit:        limit,
				Reverse:      reverse,
				Set:          set,
				BlockedBySet: blockedBySet,
			})
			if err != nil {
				return err
			}
			return renderPackageExcuseSummaries(opts.Out, opts.Output, newOutputStylerForOptions(opts, opts.Out, opts.Output), results)
		},
	}, frontend.ActionPackagesExcusesList)

	cmd.Flags().StringSliceVar(&trackers, "tracker", nil, "excuses trackers to query (default: configured default tracker)")
	cmd.Flags().StringVar(&name, "name", "", "case-insensitive regex to filter source package names")
	cmd.Flags().StringVar(&component, "component", "", "archive component to filter on")
	cmd.Flags().StringVar(&team, "team", "", "team to filter on")
	cmd.Flags().BoolVar(&ftbfs, "ftbfs", false, "show only FTBFS excuses")
	cmd.Flags().BoolVar(&autopkgtest, "autopkgtest", false, "show only autopkgtest-related excuses")
	cmd.Flags().StringVar(&blockedBy, "blocked-by", "", "show only excuses blocked by the given package")
	cmd.Flags().BoolVar(&bugged, "bugged", false, "show only excuses with an attached bug reference")
	cmd.Flags().IntVar(&minAge, "min-age", 0, "only include excuses at least this many days old")
	cmd.Flags().IntVar(&maxAge, "max-age", 0, "only include excuses no older than this many days")
	cmd.Flags().IntVar(&limit, "limit", 0, "limit the number of results")
	cmd.Flags().BoolVar(&reverse, "reverse", false, "show older excuses first")
	cmd.Flags().StringVar(&set, "set", "", "only show excuses for packages in the named config set")
	cmd.Flags().StringVar(&blockedBySet, "blocked-by-set", "", "only show excuses blocked by packages in the named config set")
	return cmd
}

func newPackagesExcusesShowCmd(opts *Options) *cobra.Command {
	var tracker, version string

	cmd := withActionID(&cobra.Command{
		Use:   "show <package>",
		Short: "Show a detailed package migration excuse",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := opts.Frontend().Packages().ExcusesShow(cmd.Context(), frontend.PackagesExcusesShowRequest{
				Package: args[0],
				Tracker: tracker,
				Version: version,
			})
			if err != nil {
				return err
			}
			return renderPackageExcuse(opts.Out, opts.Output, newOutputStylerForOptions(opts, opts.Out, opts.Output), result)
		},
	}, frontend.ActionPackagesExcusesShow)

	cmd.Flags().StringVar(&tracker, "tracker", "", "excuses tracker to query (default: configured default tracker)")
	cmd.Flags().StringVar(&version, "version", "", "exact Debian version string")
	return cmd
}

func renderPackageExcuseSummaries(w io.Writer, format string, styler *outputStyler, results []dto.PackageExcuseSummary) error {
	switch format {
	case "json":
		return renderJSON(w, results)
	case "yaml":
		return renderYAML(w, results)
	default:
		return renderPackageExcuseSummariesTable(w, styler, results)
	}
}

func renderPackageExcuseSummariesTable(w io.Writer, styler *outputStyler, results []dto.PackageExcuseSummary) error {
	if len(results) == 0 {
		fmt.Fprintln(w, "No package excuses found.")
		return nil
	}
	headers := []string{"DAYS", "TRACKER", "PACKAGE", "VERSION", "COMPONENT", "TEAM", "PRIMARY_REASON", "BLOCKED_BY", "BUG"}
	rows := make([][]string, 0, len(results))
	for _, result := range results {
		blockedBy := strings.Join(result.BlockedBy, ", ")
		if len(blockedBy) > 32 {
			blockedBy = blockedBy[:29] + "..."
		}
		rows = append(rows, []string{
			fmt.Sprintf("%d", result.AgeDays),
			result.Tracker,
			result.Package,
			result.Version,
			result.Component,
			result.Team,
			result.PrimaryReason,
			blockedBy,
			result.Bug,
		})
	}
	return renderStyledTable(w, styler, headers, rows)
}

func renderPackageExcuse(w io.Writer, format string, styler *outputStyler, excuse *dto.PackageExcuse) error {
	switch format {
	case "json":
		return renderJSON(w, excuse)
	case "yaml":
		return renderYAML(w, excuse)
	default:
		return renderPackageExcuseTable(w, styler, excuse)
	}
}

func renderPackageExcuseTable(w io.Writer, styler *outputStyler, excuse *dto.PackageExcuse) error {
	if err := writeKeyValue(w, styler, "Tracker", excuse.Tracker); err != nil {
		return err
	}
	if err := writeKeyValue(w, styler, "Package", excuse.Package); err != nil {
		return err
	}
	if err := writeKeyValue(w, styler, "Item", excuse.ItemName); err != nil {
		return err
	}
	if err := writeKeyValue(w, styler, "Version", excuse.Version); err != nil {
		return err
	}
	if excuse.OldVersion != "" {
		if err := writeKeyValue(w, styler, "Old Version", excuse.OldVersion); err != nil {
			return err
		}
	}
	if excuse.Component != "" {
		if err := writeKeyValue(w, styler, "Component", excuse.Component); err != nil {
			return err
		}
	}
	if err := writeKeyValue(w, styler, "Candidate", fmt.Sprintf("%t", excuse.Candidate)); err != nil {
		return err
	}
	if excuse.Verdict != "" {
		if err := writeKeyValue(w, styler, "Verdict", excuse.Verdict); err != nil {
			return err
		}
	}
	if err := writeKeyValue(w, styler, "Age (days)", fmt.Sprintf("%d", excuse.AgeDays)); err != nil {
		return err
	}
	if err := writeKeyValue(w, styler, "FTBFS", fmt.Sprintf("%t", excuse.FTBFS)); err != nil {
		return err
	}
	if excuse.Team != "" {
		if err := writeKeyValue(w, styler, "Team", excuse.Team); err != nil {
			return err
		}
	}
	if excuse.Maintainer != "" {
		if err := writeKeyValue(w, styler, "Maintainer", excuse.Maintainer); err != nil {
			return err
		}
	}
	if excuse.Bug != "" {
		if err := writeKeyValue(w, styler, "Bug", excuse.Bug); err != nil {
			return err
		}
	}
	if excuse.PrimaryReason != "" {
		if err := writeKeyValue(w, styler, "Primary", excuse.PrimaryReason); err != nil {
			return err
		}
	}
	if len(excuse.BlockedBy) > 0 {
		if err := writeKeyValue(w, styler, "Blocked By", strings.Join(excuse.BlockedBy, ", ")); err != nil {
			return err
		}
	}
	if excuse.BlocksCount > 0 {
		if err := writeKeyValue(w, styler, "Blocks", fmt.Sprintf("%d", excuse.BlocksCount)); err != nil {
			return err
		}
	}

	if len(excuse.Reasons) > 0 {
		fmt.Fprintln(w)
		if err := writeSectionTitle(w, styler, "Reasons:"); err != nil {
			return err
		}
		for _, reason := range excuse.Reasons {
			fmt.Fprintf(w, "  - %s", styler.Value("PRIMARY_REASON", reason.Code))
			if reason.Message != "" && reason.Message != reason.Code {
				fmt.Fprintf(w, ": %s", reason.Message)
			}
			fmt.Fprintln(w)
		}
	}

	if len(excuse.Dependencies) > 0 {
		fmt.Fprintln(w)
		if err := writeSectionTitle(w, styler, "Dependencies:"); err != nil {
			return err
		}
		for _, dep := range excuse.Dependencies {
			fmt.Fprintf(w, "  - %s: %s\n", styler.Value("KIND", dep.Kind), dep.Package)
		}
	}

	if len(excuse.ReverseDependencies) > 0 {
		fmt.Fprintln(w)
		if err := writeKeyValue(w, styler, "Reverse Dependencies", strings.Join(excuse.ReverseDependencies, ", ")); err != nil {
			return err
		}
	}

	if len(excuse.BuildFailures) > 0 {
		fmt.Fprintln(w)
		if err := writeSectionTitle(w, styler, "Build Failures:"); err != nil {
			return err
		}
		for _, failure := range excuse.BuildFailures {
			fmt.Fprintf(w, "  - %s", styler.Warning(failure.Kind))
			if failure.Architecture != "" {
				fmt.Fprintf(w, " on %s", failure.Architecture)
			}
			if failure.Message != "" {
				fmt.Fprintf(w, ": %s", failure.Message)
			}
			fmt.Fprintln(w)
		}
	}

	if len(excuse.Autopkgtests) > 0 {
		fmt.Fprintln(w)
		if err := writeSectionTitle(w, styler, "Autopkgtests:"); err != nil {
			return err
		}
		if err := renderAutopkgtestSection(w, styler, excuse.Autopkgtests); err != nil {
			return err
		}
	}

	if len(excuse.Messages) > 0 {
		fmt.Fprintln(w)
		if err := writeSectionTitle(w, styler, "Messages:"); err != nil {
			return err
		}
		for _, message := range excuse.Messages {
			fmt.Fprintf(w, "  - %s\n", message)
		}
	}
	return nil
}

// renderAutopkgtestSection renders autopkgtest results grouped by triggering
// package, with colored statuses and OSC 8 hyperlinks.
func renderAutopkgtestSection(w io.Writer, styler *outputStyler, tests []dto.ExcuseAutopkgtest) error {
	// Group by Package, preserving insertion order.
	var pkgOrder []string
	pkgMap := map[string][]dto.ExcuseAutopkgtest{}
	for _, test := range tests {
		pkg := test.Package
		if _, seen := pkgMap[pkg]; !seen {
			pkgOrder = append(pkgOrder, pkg)
		}
		pkgMap[pkg] = append(pkgMap[pkg], test)
	}

	for _, pkg := range pkgOrder {
		entries := pkgMap[pkg]

		// Fallback entries (no architecture) are plain-text messages; render
		// the stripped message directly instead of the arch=status format.
		if len(entries) > 0 && entries[0].Architecture == "" {
			for _, e := range entries {
				if _, err := fmt.Fprintf(w, "  - %s\n", styler.Dim(e.Message)); err != nil {
					return err
				}
			}
			continue
		}

		parts := make([]string, 0, len(entries))
		for _, e := range entries {
			arch := e.Architecture
			if arch == "" {
				arch = "-"
			}
			status := styler.semantic(e.Status)
			pair := arch + "=" + status
			parts = append(parts, styler.Hyperlink(pair, e.URL))
		}
		label := pkg
		if label == "" {
			label = "(unknown)"
		}
		if _, err := fmt.Fprintf(w, "  %s: %s\n", styler.apply(styler.project, label), strings.Join(parts, "  ")); err != nil {
			return err
		}
	}
	return nil
}
