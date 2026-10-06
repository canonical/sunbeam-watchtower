// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"

	"github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	"github.com/spf13/cobra"
)

func newSRUVersionsCmd(opts *Options) *cobra.Command {
	var query dto.SRUVersionsQuery
	command := withActionID(&cobra.Command{Use: "versions <source-package>", Short: "Show Ubuntu parent → staging → proposed → updates versions", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		query.Package = args[0]
		if query.Series == "" {
			result, err := opts.Frontend().SRU().AllVersions(cmd.Context(), query.Package)
			if err != nil {
				return err
			}
			switch opts.Output {
			case "json":
				return renderJSON(opts.Out, result)
			case "yaml":
				return renderYAML(opts.Out, result)
			}
			for i := range result.Rows {
				if err := renderSRUVersionRow(opts, &result.Rows[i]); err != nil {
					return err
				}
				if _, err := fmt.Fprintln(opts.Out); err != nil {
					return err
				}
			}
			return renderSRUVersionsLegend(opts)
		}
		result, err := opts.Frontend().SRU().Versions(cmd.Context(), query)
		if err != nil {
			return err
		}
		switch opts.Output {
		case "json":
			return renderJSON(opts.Out, result)
		case "yaml":
			return renderYAML(opts.Out, result)
		}
		return renderSRUVersions(opts, result)
	}}, frontend.ActionSRUVersions)
	command.Flags().StringVar(&query.Series, "series", "", "filter to one configured UCA series (default: all configured series)")
	return command
}

func renderSRUVersions(opts *Options, result *dto.SRUVersions) error {
	if err := renderSRUVersionRow(opts, result); err != nil {
		return err
	}
	return renderSRUVersionsLegend(opts)
}

func renderSRUVersionsLegend(opts *Options) error {
	_, err := fmt.Fprintln(opts.Out, "Green/current = latest observed version within each series; yellow/behind = older. Cloud rebuilds count between UCA pockets; ~cloudN is ignored only against Ubuntu. No support or migration eligibility is inferred.")
	return err
}

func renderSRUVersionRow(opts *Options, result *dto.SRUVersions) error {
	styler := newOutputStylerForOptions(opts, opts.Out, opts.Output)
	if _, err := fmt.Fprintf(opts.Out, "%s · UCA/%s (Ubuntu %s)\nObserved: %s\n\n", sruDisplayText(result.Query.Package), sruDisplayText(result.Query.Series), sruDisplayText(result.UbuntuBase), result.ObservedAt.Format("2006-01-02 15:04 UTC")); err != nil {
		return err
	}
	headers, values, states := []string{}, []string{}, []string{}
	for i, cell := range result.Cells {
		if i > 0 {
			headers = append(headers, "→")
			values = append(values, "→")
			states = append(states, "")
		}
		headers = append(headers, styler.Header(sruDisplayText(cell.Label)))
		value, state := sruVersionCellText(styler, cell)
		values = append(values, value)
		states = append(states, state)
	}
	if err := renderTableRows(opts.Out, headers, [][]string{values, states}); err != nil {
		return err
	}
	for _, cell := range result.Cells {
		if cell.Warning != "" {
			if _, err := fmt.Fprintf(opts.Out, "%s: %s\n", sruDisplayText(cell.Label), sruDisplayText(cell.Warning)); err != nil {
				return err
			}
		}
	}
	return nil
}

func sruVersionCellText(styler *outputStyler, cell dto.SRUVersionCell) (string, string) {
	value := sruDisplayText(cell.Version)
	if value == "" {
		value = "—"
	}
	state := sruDisplayText(cell.State)
	switch cell.State {
	case "current":
		return styler.apply(styler.success, value), styler.apply(styler.success, state)
	case "behind":
		return styler.Warning(value), styler.Warning(state)
	default:
		return styler.Dim(value), styler.Dim(state)
	}
}
