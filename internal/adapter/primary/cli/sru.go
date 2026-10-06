// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newSRUCmd(opts *Options) *cobra.Command {
	cmd := &cobra.Command{Use: "sru", Short: "Monitor OpenStack SRUs"}
	var filter dto.SRUFilter
	var listOptions sruListOptions
	list := withActionID(&cobra.Command{
		Use: "list", Short: "List SRUs needing attention, grouped by bug and package",
		RunE: func(cmd *cobra.Command, _ []string) error {
			filter.NeedsAttention = !listOptions.all
			result, err := opts.Frontend().SRU().List(cmd.Context(), filter)
			if err != nil {
				return err
			}
			return renderSRU(opts, result, false, listOptions)
		},
	}, frontend.ActionSRUList)
	list.Flags().StringVar(&filter.Package, "package", "", "filter by source package")
	list.Flags().StringVar(&filter.Set, "set", "", "filter by configured package set")
	list.Flags().StringVar(&filter.Archive, "archive", "", "filter by ubuntu or uca")
	list.Flags().StringVar(&filter.Series, "series", "", "filter by target series")
	list.Flags().StringVar(&filter.Stage, "stage", "", "filter by archive stage")
	list.Flags().StringVar(&filter.TaskStatus, "status", "", "filter by Launchpad task status")
	list.Flags().StringVar(&filter.Verification, "verification", "", "filter by verification state")
	list.Flags().StringVar(&filter.BugID, "bug-id", "", "filter by Launchpad bug ID")
	list.Flags().BoolVar(&listOptions.all, "all", false, "include targets that do not need attention")
	list.Flags().BoolVar(&listOptions.targets, "targets", false, "show one row per target")
	list.Flags().BoolVar(&listOptions.diagnostics, "diagnostics", false, "show detailed warnings after the table")
	cmd.AddCommand(list)
	cmd.AddCommand(newSRUMigrationCmd(opts))
	cmd.AddCommand(newSRUVersionsCmd(opts))
	cmd.AddCommand(withActionID(&cobra.Command{
		Use: "show <bug-id-or-url>", Short: "Show all monitored targets for one bug", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			id, err := sruBugID(args[0])
			if err != nil {
				return err
			}
			result, err := opts.Frontend().SRU().Show(command.Context(), id)
			if err != nil {
				return err
			}
			if len(result.Rows) == 0 {
				return fmt.Errorf("bug %s has no monitored SRU targets", id)
			}
			return renderSRU(opts, result, true, sruListOptions{})
		},
	}, frontend.ActionSRUShow))
	return cmd
}

func sruBugID(raw string) (string, error) {
	if strings.HasPrefix(raw, "https://") {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Hostname() != "bugs.launchpad.net" || parsed.Port() != "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return "", fmt.Errorf("expected a bugs.launchpad.net URL or numeric bug ID")
		}
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(parts) < 2 || (parts[len(parts)-2] != "+bug" && parts[len(parts)-2] != "bugs") {
			return "", fmt.Errorf("expected a Launchpad bug URL")
		}
		raw = parts[len(parts)-1]
	}
	id, err := strconv.Atoi(raw)
	if err != nil || id < 1 {
		return "", fmt.Errorf("invalid Launchpad bug ID %q", raw)
	}
	return strconv.Itoa(id), nil
}

type sruListOptions struct {
	all         bool
	targets     bool
	diagnostics bool
}

func renderSRU(opts *Options, snapshot *dto.SRUSnapshot, detail bool, listOptions sruListOptions) error {
	switch opts.Output {
	case "json":
		return renderJSON(opts.Out, snapshot)
	case "yaml":
		return renderYAML(opts.Out, snapshot)
	}
	styler := newOutputStylerForOptions(opts, opts.Out, opts.Output)
	if detail {
		return renderSRUDetailTable(opts.Out, styler, snapshot)
	}
	if listOptions.targets {
		return renderSRUTargetTableWidth(opts.Out, styler, snapshot, sruOutputWidth(opts.Out), listOptions)
	}
	return renderSRUListTableWidth(opts.Out, styler, snapshot, sruOutputWidth(opts.Out), listOptions)
}

func sruOutputWidth(w io.Writer) int {
	const fallbackWidth = 120
	file, ok := w.(*os.File)
	if !ok {
		return fallbackWidth
	}
	fd := file.Fd()
	maxInt := ^uint(0) >> 1
	if fd > uintptr(maxInt) {
		return fallbackWidth
	}
	//nolint:gosec // fd is checked against the maximum int above.
	width, _, err := term.GetSize(int(fd))
	if err != nil || width < 60 {
		return fallbackWidth
	}
	return min(width, 120)
}

type sruGroup struct {
	bugID       string
	bugURL      string
	packageName string
	title       string
	rows        []dto.SRURow
}

func groupSRURows(rows []dto.SRURow) []sruGroup {
	type key struct{ bugID, packageName string }
	positions := make(map[key]int)
	groups := make([]sruGroup, 0)
	for _, row := range rows {
		id := key{row.BugID, row.Package}
		position, exists := positions[id]
		if !exists {
			position = len(groups)
			positions[id] = position
			groups = append(groups, sruGroup{bugID: row.BugID, bugURL: row.BugURL,
				packageName: row.Package, title: row.Title})
		}
		groups[position].rows = append(groups[position].rows, row)
	}
	return groups
}

func sruProgressSummary(rows []dto.SRURow) string {
	type progress struct {
		label    string
		count    int
		priority int
	}
	counts := make(map[string]int)
	priorities := make(map[string]int)
	for _, row := range rows {
		label := row.Stage
		if label == "" {
			label = "not observed"
		}
		priority := 4
		switch row.Verification {
		case "failed":
			label += "/failed"
			priority = 0
		case "needed":
			label += "/needed"
			priority = 1
		case "done":
			label += "/done"
		}
		if row.Verification != "failed" && row.Verification != "needed" {
			switch row.Stage {
			case "unapproved", "staging":
				priority = 2
			case "proposed":
				priority = 3
			case "not observed":
				priority = 5
			case "updates":
				priority = 6
			}
		}
		counts[label]++
		priorities[label] = priority
	}
	items := make([]progress, 0, len(counts))
	for label, count := range counts {
		items = append(items, progress{label: label, count: count, priority: priorities[label]})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].priority != items[j].priority {
			return items[i].priority < items[j].priority
		}
		return items[i].label < items[j].label
	})
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if item.count == 1 {
			parts = append(parts, item.label)
		} else {
			parts = append(parts, fmt.Sprintf("%d×%s", item.count, item.label))
		}
	}
	return strings.Join(parts, ", ")
}

func renderSRUListTableWidth(w io.Writer, styler *outputStyler, snapshot *dto.SRUSnapshot, width int, options sruListOptions) error {
	groups := groupSRURows(snapshot.Rows)
	counts := fmt.Sprintf("%d %s / %d %s", len(groups), sruPlural("SRU", len(groups)),
		len(snapshot.Rows), sruPlural("target", len(snapshot.Rows)))
	if _, err := fmt.Fprintf(w, "%s  %s\n", styler.Section(sruListHeading(options.all)), counts); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%s %s\n\n", styler.Key("Synced:"),
		snapshot.SyncedAt.Format("2006-01-02 15:04 UTC")); err != nil {
		return err
	}
	if len(groups) == 0 {
		_, err := fmt.Fprintln(w, styler.Placeholder("No matching SRUs."))
		return err
	}
	packageWidth, progressWidth := 20, 24
	if width < 100 {
		packageWidth, progressWidth = 15, 19
	}
	if width < 80 {
		packageWidth, progressWidth = 11, 15
	}
	const bugWidth, targetsWidth, separators = 8, 7, 8
	titleWidth := max(1, width-bugWidth-packageWidth-targetsWidth-progressWidth-separators)
	headers := []string{styler.Header("BUG"), styler.Header("PACKAGE"), styler.Header("TARGETS"),
		styler.Header("PROGRESS"), styler.Header("TITLE")}
	rows := make([][]string, 0, len(groups))
	for _, group := range groups {
		progress := sruShorten(sruProgressSummary(group.rows), progressWidth)
		rows = append(rows, []string{
			sruLink(styler, styler.Value("BUG", "#"+group.bugID), group.bugURL),
			styler.Value("PACKAGE", sruShorten(group.packageName, packageWidth)),
			strconv.Itoa(len(group.rows)), sruGroupProgressStyle(styler, group.rows, progress),
			sruShorten(group.title, titleWidth),
		})
	}
	if err := renderTableRows(w, headers, rows); err != nil {
		return err
	}
	return renderSRUDiagnostics(w, styler, snapshot, options.diagnostics)
}

func sruListHeading(all bool) string {
	if all {
		return "OpenStack SRUs · all"
	}
	return "OpenStack SRUs · attention"
}

func sruPlural(singular string, count int) string {
	if count == 1 {
		return singular
	}
	return singular + "s"
}

func sruGroupProgressStyle(styler *outputStyler, rows []dto.SRURow, summary string) string {
	for _, row := range rows {
		if row.Verification == "failed" {
			return styler.apply(styler.failure, summary)
		}
	}
	for _, row := range rows {
		if row.Verification == "needed" {
			return styler.Warning(summary)
		}
	}
	return styler.Value("STAGE", summary)
}

func renderSRUTargetTableWidth(w io.Writer, styler *outputStyler, snapshot *dto.SRUSnapshot, width int, options sruListOptions) error {
	if _, err := fmt.Fprintf(w, "%s  %d %s\n", styler.Section(sruListHeading(options.all)),
		len(snapshot.Rows), sruPlural("target", len(snapshot.Rows))); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%s %s\n\n", styler.Key("Synced:"),
		snapshot.SyncedAt.Format("2006-01-02 15:04 UTC")); err != nil {
		return err
	}
	if len(snapshot.Rows) == 0 {
		_, err := fmt.Fprintln(w, styler.Placeholder("No matching SRU targets."))
		return err
	}
	packageWidth, targetWidth := 15, 16
	showTask := width >= 80
	fixedWidth := 8 + 13 + 6 + 8 // bug, stage, verify, separators
	if showTask {
		fixedWidth += 9 + 2 // task and separator
	} else {
		packageWidth = min(15, max(10, (width-fixedWidth)/2))
		targetWidth = min(16, width-fixedWidth-packageWidth)
	}
	if width >= 100 {
		packageWidth, targetWidth = 20, 20
	}
	titleWidth := width - fixedWidth - packageWidth - targetWidth - 2
	headers := []string{"BUG", "PACKAGE", "TARGET"}
	if showTask {
		headers = append(headers, "TASK")
	}
	headers = append(headers, "STAGE", "VERIFY")
	showTitle := titleWidth >= 25
	if showTitle {
		headers = append(headers, "TITLE")
	}
	styledHeaders := make([]string, len(headers))
	for i, header := range headers {
		styledHeaders[i] = styler.Header(header)
	}
	rows := make([][]string, 0, len(snapshot.Rows))
	for _, row := range snapshot.Rows {
		stage := sruStageStyle(styler, row.Stage)
		if options.diagnostics && row.Warning != "" {
			stage += styler.Warning("!")
		}
		cells := []string{
			sruLink(styler, styler.Value("BUG", "#"+row.BugID), row.BugURL),
			styler.Value("PACKAGE", sruShorten(row.Package, packageWidth)),
			styler.Value("TARGET", sruShorten(row.Archive+"/"+row.Series, targetWidth)),
		}
		if showTask {
			cells = append(cells, sruStatusStyle(styler, sruShortStatus(row.TaskStatus)))
		}
		cells = append(cells, stage, sruVerificationStyle(styler, row.Verification))
		if showTitle {
			cells = append(cells, sruShorten(row.Title, titleWidth))
		}
		rows = append(rows, cells)
	}
	if err := renderTableRows(w, styledHeaders, rows); err != nil {
		return err
	}
	if !showTitle {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if err := writeSectionTitle(w, styler, "Bugs"); err != nil {
			return err
		}
		seen := make(map[string]bool)
		titles := make([][]string, 0, len(snapshot.Rows))
		for _, row := range snapshot.Rows {
			if seen[row.BugID] {
				continue
			}
			seen[row.BugID] = true
			titles = append(titles, []string{
				sruLink(styler, styler.Value("BUG", "#"+row.BugID), row.BugURL),
				sruShorten(row.Title, max(12, width-12)),
			})
		}
		if err := renderTableRows(w, []string{styler.Header("BUG"), styler.Header("TITLE")}, titles); err != nil {
			return err
		}
	}
	return renderSRUDiagnostics(w, styler, snapshot, options.diagnostics)
}

func renderSRUDiagnostics(w io.Writer, styler *outputStyler, snapshot *dto.SRUSnapshot, enabled bool) error {
	if !enabled {
		return nil
	}
	var hasWarnings bool
	for _, row := range snapshot.Rows {
		if row.Warning != "" {
			hasWarnings = true
			break
		}
	}
	if !hasWarnings {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if err := writeSectionTitle(w, styler, "Warnings"); err != nil {
		return err
	}
	for _, row := range snapshot.Rows {
		if row.Warning == "" {
			continue
		}
		if _, err := fmt.Fprintf(w, "  %s %s/%s: %s\n", styler.Value("BUG", "#"+sruDisplayText(row.BugID)),
			sruDisplayText(row.Archive), sruDisplayText(row.Series), styler.Warning(sruDisplayText(row.Warning))); err != nil {
			return err
		}
	}
	return nil
}

func renderSRUDetailTable(w io.Writer, styler *outputStyler, snapshot *dto.SRUSnapshot) error {
	if len(snapshot.Rows) == 0 {
		_, err := fmt.Fprintln(w, styler.Placeholder("No matching SRU targets."))
		return err
	}
	first := snapshot.Rows[0]
	if err := writeSectionTitle(w, styler, "Bug #"+first.BugID+": "+sruDisplayText(first.Title)); err != nil {
		return err
	}
	if err := writeKeyValue(w, styler, "Synced", snapshot.SyncedAt.Format("2006-01-02 15:04 UTC")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%s %s\n", styler.Key("Bug URL:"),
		sruLink(styler, styler.Value("URL", sruDisplayText(first.BugURL)), first.BugURL)); err != nil {
		return err
	}
	for _, row := range snapshot.Rows {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if err := writeSectionTitle(w, styler, row.Package+"  "+row.Archive+"/"+row.Series); err != nil {
			return err
		}
		for _, item := range [][2]string{
			{"Package sets", strings.Join(row.PackageSets, ", ")},
			{"Task", row.TaskStatus}, {"Stage", row.Stage}, {"Version", row.Version},
			{"Verification", row.Verification}, {"Tag", row.VerificationTag},
			{"Parent", row.ParentSeries},
		} {
			value := item[1]
			switch item[0] {
			case "Task":
				value = sruStatusStyle(styler, value)
			case "Stage":
				value = sruStageStyle(styler, value)
			case "Verification":
				value = sruVerificationStyle(styler, value)
			}
			if err := writeKeyValue(w, styler, item[0], value); err != nil {
				return err
			}
		}
		if row.ParentRequired {
			if err := writeKeyValue(w, styler, "Parent ready", fmt.Sprint(row.ParentReady)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "%s %s\n", styler.Key("Task URL:"),
			sruLink(styler, styler.Value("URL", sruDisplayText(row.TaskURL)), row.TaskURL)); err != nil {
			return err
		}
		if row.Warning != "" {
			if err := writeWarningLine(w, styler, row.Warning); err != nil {
				return err
			}
		}
		if len(row.Evidence) == 0 {
			continue
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if err := writeSectionTitle(w, styler, "Archive evidence"); err != nil {
			return err
		}
		headers := []string{styler.Header("STAGE"), styler.Header("VERSION"), styler.Header("PUBLICATION")}
		evidenceRows := make([][]string, 0, len(row.Evidence))
		for _, evidence := range row.Evidence {
			evidenceRows = append(evidenceRows, []string{
				sruStageStyle(styler, evidence.Stage), evidence.Version,
				sruLink(styler, styler.Value("URL", sruDisplayText(evidence.URL)), evidence.URL),
			})
		}
		if err := renderTableRows(w, headers, evidenceRows); err != nil {
			return err
		}
	}
	return nil
}

func sruStatusStyle(styler *outputStyler, status string) string {
	switch status {
	case "Fix Released", "Released":
		return styler.apply(styler.success, status)
	case "Invalid", "Won't Fix", "Wont Fix":
		return styler.apply(styler.failure, status)
	case "New", "Confirmed", "Triaged", "In Progress", "Fix Committed", "Committed", "Progress":
		return styler.apply(styler.pending, status)
	default:
		return styler.Value("STATUS", status)
	}
}

func sruShortStatus(status string) string {
	switch status {
	case "Fix Released":
		return "Released"
	case "Fix Committed":
		return "Committed"
	case "In Progress":
		return "Progress"
	case "Won't Fix":
		return "Wont Fix"
	default:
		return sruShorten(status, 9)
	}
}

func sruShorten(value string, width int) string {
	value = sruDisplayText(value)
	if lipgloss.Width(value) <= width {
		return value
	}
	if width < 2 {
		return "…"
	}
	var shortened strings.Builder
	used := 0
	for _, char := range value {
		charWidth := lipgloss.Width(string(char))
		if used+charWidth > width-1 {
			break
		}
		shortened.WriteRune(char)
		used += charWidth
	}
	return shortened.String() + "…"
}

func sruStageStyle(styler *outputStyler, stage string) string {
	switch stage {
	case "updates":
		return styler.apply(styler.success, stage)
	case "proposed":
		return styler.apply(styler.pending, stage)
	case "unapproved", "staging":
		return styler.Warning(stage)
	case "not observed":
		return styler.Placeholder(stage)
	default:
		return stage
	}
}

func sruVerificationStyle(styler *outputStyler, verification string) string {
	switch verification {
	case "done":
		return styler.apply(styler.success, verification)
	case "needed":
		return styler.Warning(verification)
	case "failed":
		return styler.apply(styler.failure, verification)
	case "none":
		return styler.Placeholder(verification)
	default:
		return verification
	}
}

func sruDisplayText(value string) string {
	return strings.Map(func(char rune) rune {
		if unicode.IsControl(char) {
			return ' '
		}
		return char
	}, value)
}

func sruLink(styler *outputStyler, text, rawURL string) string {
	if strings.ContainsFunc(rawURL, unicode.IsControl) {
		return text
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" {
		return text
	}
	switch parsed.Hostname() {
	case "bugs.launchpad.net", "api.launchpad.net", "launchpad.net":
		return styler.Hyperlink(text, rawURL)
	default:
		return text
	}
}
