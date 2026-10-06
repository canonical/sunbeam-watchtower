// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package dto

import "time"

// SRUMigrationTarget identifies a configured archive target, not its support status.
type SRUMigrationTarget struct {
	Relation       string                 `json:"relation,omitempty" yaml:"relation,omitempty"`
	Archive        string                 `json:"archive" yaml:"archive"`
	Series         string                 `json:"series" yaml:"series"`
	UbuntuBase     string                 `json:"ubuntu_base" yaml:"ubuntu_base"`
	ReleaseID      string                 `json:"release_id,omitempty" yaml:"release_id,omitempty"`
	ParentSeries   string                 `json:"parent_series,omitempty" yaml:"parent_series,omitempty"`
	ParentRequired bool                   `json:"parent_required,omitempty" yaml:"parent_required,omitempty"`
	Pockets        []SRUPocketObservation `json:"pockets" yaml:"pockets"`
}

// SRUPocketObservation distinguishes an empty pocket from an unavailable one.
type SRUPocketObservation struct {
	Pocket       string           `json:"pocket" yaml:"pocket"`
	Known        bool             `json:"known" yaml:"known"`
	Publications []SRUPublication `json:"publications" yaml:"publications"`
	Warning      string           `json:"warning,omitempty" yaml:"warning,omitempty"`
}

// SRUPublication contains independent package and fix evidence. An unavailable
// changes file does not invalidate the observed source version.
type SRUPublication struct {
	Version   string   `json:"version" yaml:"version"`
	URL       string   `json:"url" yaml:"url"`
	BugsKnown bool     `json:"bugs_known" yaml:"bugs_known"`
	BugIDs    []string `json:"bug_ids" yaml:"bug_ids"`
	Warning   string   `json:"warning,omitempty" yaml:"warning,omitempty"`
}

// SRUMigrationQuery identifies a desired fix by bug and source, not by comparing
// source versions across different releases.
type SRUMigrationQuery struct {
	BugID   string `json:"bug_id" yaml:"bug_id"`
	Package string `json:"package" yaml:"package"`
	Series  string `json:"series" yaml:"series"`
}

// SRUMigrationChain separates archive observations from inferred workflow steps.
type SRUMigrationChain struct {
	ObservedAt time.Time         `json:"observed_at" yaml:"observed_at"`
	Query      SRUMigrationQuery `json:"query" yaml:"query"`
	Scope      string            `json:"scope" yaml:"scope"`
	// Inventory retains every inspected target independently of fix association.
	Inventory []SRUMigrationTarget `json:"inventory" yaml:"inventory"`
	// Targets contains the selected/newer scope and observed lower backports.
	Targets          []SRUMigrationTarget     `json:"targets" yaml:"targets"`
	Transitions      []SRUMigrationTransition `json:"transitions" yaml:"transitions"`
	FirstOutstanding []string                 `json:"first_outstanding" yaml:"first_outstanding"`
	Warnings         []string                 `json:"warnings,omitempty" yaml:"warnings,omitempty"`
}

// SRUMigrationTransition is a policy inference, never proof of an archive block.
// Unknown steps express an evidence/applicability gap, not required backport work.
type SRUMigrationTransition struct {
	ID        string   `json:"id" yaml:"id"`
	Package   string   `json:"package" yaml:"package"`
	Archive   string   `json:"archive" yaml:"archive"`
	Series    string   `json:"series" yaml:"series"`
	Version   string   `json:"version,omitempty" yaml:"version,omitempty"`
	From      string   `json:"from" yaml:"from"`
	To        string   `json:"to" yaml:"to"`
	Kind      string   `json:"kind" yaml:"kind"`
	State     string   `json:"state" yaml:"state"`
	Reason    string   `json:"reason" yaml:"reason"`
	DependsOn []string `json:"depends_on" yaml:"depends_on"`
}
