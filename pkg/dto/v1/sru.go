// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package dto

import "time"

// SRURow describes one bug, source package, and affected archive target.
// Archive evidence is kept separate from Launchpad task status.
type SRURow struct {
	BugID           string               `json:"bug_id" yaml:"bug_id"`
	Title           string               `json:"title" yaml:"title"`
	BugURL          string               `json:"bug_url" yaml:"bug_url"`
	Package         string               `json:"package" yaml:"package"`
	PackageSets     []string             `json:"package_sets" yaml:"package_sets"`
	Archive         string               `json:"archive" yaml:"archive"`
	Series          string               `json:"series" yaml:"series"`
	UbuntuBase      string               `json:"ubuntu_base,omitempty" yaml:"ubuntu_base,omitempty"`
	ParentSeries    string               `json:"parent_series,omitempty" yaml:"parent_series,omitempty"`
	TaskStatus      string               `json:"task_status" yaml:"task_status"`
	TaskURL         string               `json:"task_url" yaml:"task_url"`
	Verification    string               `json:"verification" yaml:"verification"`
	VerificationTag string               `json:"verification_tag,omitempty" yaml:"verification_tag,omitempty"`
	Stage           string               `json:"stage" yaml:"stage"`
	Version         string               `json:"version,omitempty" yaml:"version,omitempty"`
	EvidenceURL     string               `json:"evidence_url,omitempty" yaml:"evidence_url,omitempty"`
	Evidence        []SRUArchiveEvidence `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	ParentRequired  bool                 `json:"parent_required,omitempty" yaml:"parent_required,omitempty"`
	ParentReady     bool                 `json:"parent_ready,omitempty" yaml:"parent_ready,omitempty"`
	Warning         string               `json:"warning,omitempty" yaml:"warning,omitempty"`
	UpdatedAt       time.Time            `json:"updated_at" yaml:"updated_at"`
}

// SRUArchiveEvidence retains every observed pocket for the bug and version.
type SRUArchiveEvidence struct {
	Stage   string `json:"stage" yaml:"stage"`
	Version string `json:"version" yaml:"version"`
	URL     string `json:"url" yaml:"url"`
}

// SRUSnapshot is an atomic, locally cached monitoring result.
type SRUSnapshot struct {
	SyncedAt time.Time `json:"synced_at" yaml:"synced_at"`
	Rows     []SRURow  `json:"rows" yaml:"rows"`
	Warnings []string  `json:"warnings,omitempty" yaml:"warnings,omitempty"`
}

// SRUCacheStatus summarizes the persisted monitoring snapshot.
type SRUCacheStatus struct {
	SyncedAt time.Time `json:"synced_at,omitempty" yaml:"synced_at,omitempty"`
	Targets  int       `json:"targets" yaml:"targets"`
}

// SRUFilter selects rows without changing the stored snapshot.
type SRUFilter struct {
	Package        string `json:"package,omitempty" yaml:"package,omitempty"`
	Set            string `json:"set,omitempty" yaml:"set,omitempty"`
	Archive        string `json:"archive,omitempty" yaml:"archive,omitempty"`
	Series         string `json:"series,omitempty" yaml:"series,omitempty"`
	Stage          string `json:"stage,omitempty" yaml:"stage,omitempty"`
	TaskStatus     string `json:"task_status,omitempty" yaml:"task_status,omitempty"`
	Verification   string `json:"verification,omitempty" yaml:"verification,omitempty"`
	BugID          string `json:"bug_id,omitempty" yaml:"bug_id,omitempty"`
	NeedsAttention bool   `json:"needs_attention,omitempty" yaml:"needs_attention,omitempty"`
}
