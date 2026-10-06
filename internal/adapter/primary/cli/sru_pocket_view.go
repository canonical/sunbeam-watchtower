// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"

	"github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	"github.com/spf13/cobra"
)

func newSRUPocketViewCmd(opts *Options) *cobra.Command {
	return withActionID(&cobra.Command{Use: "view <uca-series>", Short: "Show cached versions for every source package in UCA staging", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := opts.Frontend().SRU().PocketView(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		switch opts.Output {
		case "json":
			return renderJSON(opts.Out, result)
		case "yaml":
			return renderYAML(opts.Out, result)
		}
		return renderSRUPocketView(opts, result)
	}}, frontend.ActionSRUPocketView)
}

func renderSRUPocketView(opts *Options, result *dto.SRUPocketView) error {
	styler := newOutputStylerForOptions(opts, opts.Out, opts.Output)
	if _, err := fmt.Fprintf(opts.Out, "UCA/%s · Ubuntu %s · %d staging source packages\n", sruDisplayText(result.Series), sruDisplayText(result.UbuntuBase), len(result.Rows)); err != nil {
		return err
	}
	for _, status := range result.CacheStatus {
		if _, err := fmt.Fprintf(opts.Out, "Cache %s: %s\n", sruDisplayText(status.Name), status.LastUpdated.UTC().Format("2006-01-02 15:04 UTC")); err != nil {
			return err
		}
	}
	headers := []string{styler.Header("SOURCE"), styler.Header("Ubuntu " + sruDisplayText(result.ParentSeries)), "→", styler.Header("STAGING"), "→", styler.Header("PROPOSED"), "→", styler.Header("UPDATES")}
	rows := [][]string{}
	for _, row := range result.Rows {
		values := []string{sruDisplayText(row.Query.Package)}
		for i, cell := range row.Cells {
			if i > 0 {
				values = append(values, "→")
			}
			value, state := sruVersionCellText(styler, cell)
			values = append(values, value+" ["+state+"]")
		}
		rows = append(rows, values)
	}
	if err := renderTableRows(opts.Out, headers, rows); err != nil {
		return err
	}
	if err := renderSRUVersionsLegend(opts); err != nil {
		return err
	}
	warnings := map[string]bool{}
	for _, row := range result.Rows {
		for _, cell := range row.Cells {
			if cell.Warning != "" {
				warning := sruDisplayText(cell.Label + ": " + cell.Warning)
				if !warnings[warning] {
					warnings[warning] = true
					if _, err := fmt.Fprintln(opts.Out, warning); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}
