// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	"github.com/spf13/cobra"
)

func newSRUMigrationCmd(opts *Options) *cobra.Command {
	var query dto.SRUMigrationQuery
	command := withActionID(&cobra.Command{
		Use: "chain <source-package>", Short: "Inspect live SRU pockets and inferred migration dependencies",
		Long: "Inspect a bug's publications across configured UCA series and associated Ubuntu parents. Requires the upstream ordering cache; support and migration eligibility are not inferred.", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query.Package = args[0]
			id, err := sruBugID(query.BugID)
			if err != nil {
				return err
			}
			query.BugID = id
			result, err := opts.Frontend().SRU().Migration(cmd.Context(), query)
			if err != nil {
				return err
			}
			switch opts.Output {
			case "json":
				return renderJSON(opts.Out, result)
			case "yaml":
				return renderYAML(opts.Out, result)
			}
			return renderSRUMigration(opts.Out, result)
		},
	}, frontend.ActionSRUMigration)
	command.Flags().StringVar(&query.Series, "series", "", "selected configured UCA series")
	command.Flags().StringVar(&query.BugID, "bug-id", "", "desired fix's Launchpad bug ID or URL")
	_ = command.MarkFlagRequired("series")
	_ = command.MarkFlagRequired("bug-id")
	return command
}

func renderSRUMigration(w io.Writer, result *dto.SRUMigrationChain) error {
	if _, err := fmt.Fprintf(w, "%s · UCA/%s · bug #%s\nObserved: %s\nScope: %s\nDependencies are inferred policy; migration eligibility is not established.\n\n", sruDisplayText(result.Query.Package), sruDisplayText(result.Query.Series), sruDisplayText(result.Query.BugID), result.ObservedAt.Format("2006-01-02 15:04 UTC"), sruDisplayText(result.Scope)); err != nil {
		return err
	}
	rows := [][]string{}
	inventory := result.Inventory
	if inventory == nil {
		inventory = result.Targets
	}
	for _, target := range inventory {
		for _, pocket := range target.Pockets {
			if !pocket.Known {
				rows = append(rows, []string{target.Relation, target.Archive + "/" + target.Series, pocket.Pocket, "unknown", ""})
				continue
			}
			if len(pocket.Publications) == 0 {
				rows = append(rows, []string{target.Relation, target.Archive + "/" + target.Series, pocket.Pocket, "empty", ""})
			}
			for _, publication := range pocket.Publications {
				fixes := "not listed"
				if !publication.BugsKnown {
					fixes = "unknown"
				} else {
					for _, id := range publication.BugIDs {
						if id == result.Query.BugID {
							fixes = "yes"
						}
					}
				}
				rows = append(rows, []string{target.Relation, target.Archive + "/" + target.Series, pocket.Pocket, publication.Version, fixes})
			}
		}
	}
	for i := range rows {
		for j := range rows[i] {
			rows[i][j] = sruDisplayText(rows[i][j])
		}
	}
	if err := renderTableRows(w, []string{"RELATION", "TARGET", "POCKET", "VERSION / STATE", "FIX EVIDENCE"}, rows); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "\nInferred transitions (dependencies must come first):"); err != nil {
		return err
	}
	transitions := [][]string{}
	for _, step := range result.Transitions {
		transitions = append(transitions, []string{step.ID, step.Package, step.Version, step.From + " → " + step.To, step.State, strings.Join(step.DependsOn, ", ")})
	}
	for i := range transitions {
		for j := range transitions[i] {
			transitions[i][j] = sruDisplayText(transitions[i][j])
		}
	}
	if err := renderTableRows(w, []string{"ID", "SOURCE", "VERSION", "TRANSITION", "STATE", "DEPENDS ON"}, transitions); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "\nFirst outstanding: %s\n", sruDisplayText(strings.Join(result.FirstOutstanding, ", "))); err != nil {
		return err
	}
	for _, step := range result.Transitions {
		if _, err := fmt.Fprintf(w, "  %s: %s\n", sruDisplayText(step.ID), sruDisplayText(step.Reason)); err != nil {
			return err
		}
	}
	for _, warning := range result.Warnings {
		if _, err := fmt.Fprintf(w, "Warning: %s\n", sruDisplayText(warning)); err != nil {
			return err
		}
	}
	return nil
}
