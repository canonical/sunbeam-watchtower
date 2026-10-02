// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package dto

import (
	"time"

	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

// BugCacheTask preserves the tracker project bucket that owns a cached task.
type BugCacheTask struct {
	TrackerProject string        `json:"tracker_project" yaml:"tracker_project"`
	Task           forge.BugTask `json:"task" yaml:"task"`
}

// BugCacheDocument is one complete cached bug and its scoped tasks.
type BugCacheDocument struct {
	Bug   *forge.Bug     `json:"bug" yaml:"bug"`
	Tasks []BugCacheTask `json:"tasks" yaml:"tasks"`
}

// BugCacheSnapshot is an atomic read view used by explainable search.
type BugCacheSnapshot struct {
	Documents []BugCacheDocument `json:"documents" yaml:"documents"`
	Status    []BugCacheStatus   `json:"status" yaml:"status"`
}

// BugSearchMode selects the query evaluator.
type BugSearchMode string

const (
	BugSearchModeText  BugSearchMode = "text"
	BugSearchModeRegex BugSearchMode = "regex"
)

// BugSearchClassification describes evidence coverage, not causal identity.
type BugSearchClassification string

const (
	BugSearchDirect  BugSearchClassification = "direct"
	BugSearchPartial BugSearchClassification = "partial"
	BugSearchRelated BugSearchClassification = "related"
)

// BugSearchOutcome makes successful empty and related-only responses explicit.
type BugSearchOutcome string

const (
	BugSearchOutcomeMatches     BugSearchOutcome = "matches"
	BugSearchOutcomeRelatedOnly BugSearchOutcome = "related_only"
	BugSearchOutcomeNoResults   BugSearchOutcome = "no_results"
)

// BugSearchRequest is the shared search contract used by every frontend.
type BugSearchRequest struct {
	Query          string   `json:"query,omitempty" yaml:"query,omitempty"`
	Mode           string   `json:"mode,omitempty" yaml:"mode,omitempty"`
	Fields         []string `json:"fields,omitempty" yaml:"fields,omitempty" required:"false"`
	Phrases        []string `json:"phrases,omitempty" yaml:"phrases,omitempty" required:"false"`
	RequiredTerms  []string `json:"required_terms,omitempty" yaml:"required_terms,omitempty" required:"false"`
	ExcludedTerms  []string `json:"excluded_terms,omitempty" yaml:"excluded_terms,omitempty" required:"false"`
	Fuzzy          *bool    `json:"fuzzy,omitempty" yaml:"fuzzy,omitempty"`
	CaseSensitive  bool     `json:"case_sensitive,omitempty" yaml:"case_sensitive,omitempty" required:"false"`
	Projects       []string `json:"projects,omitempty" yaml:"projects,omitempty" required:"false"`
	Trackers       []string `json:"trackers,omitempty" yaml:"trackers,omitempty" required:"false"`
	Status         []string `json:"status,omitempty" yaml:"status,omitempty" required:"false"`
	Importance     []string `json:"importance,omitempty" yaml:"importance,omitempty" required:"false"`
	Assignee       string   `json:"assignee,omitempty" yaml:"assignee,omitempty"`
	Tags           []string `json:"tags,omitempty" yaml:"tags,omitempty" required:"false"`
	Closed         string   `json:"closed,omitempty" yaml:"closed,omitempty"`
	CreatedAfter   string   `json:"created_after,omitempty" yaml:"created_after,omitempty"`
	CreatedBefore  string   `json:"created_before,omitempty" yaml:"created_before,omitempty"`
	ModifiedAfter  string   `json:"modified_after,omitempty" yaml:"modified_after,omitempty"`
	ModifiedBefore string   `json:"modified_before,omitempty" yaml:"modified_before,omitempty"`
	Merge          *bool    `json:"merge,omitempty" yaml:"merge,omitempty"`
	Sort           string   `json:"sort,omitempty" yaml:"sort,omitempty"`
	Limit          int      `json:"limit,omitempty" yaml:"limit,omitempty"`
	RelatedLimit   *int     `json:"related_limit,omitempty" yaml:"related_limit,omitempty"`
	EvidenceLimit  int      `json:"evidence_limit,omitempty" yaml:"evidence_limit,omitempty"`
}

// BugSearchApplied records resolved defaults and the searched corpus.
type BugSearchApplied struct {
	Mode          string   `json:"mode" yaml:"mode"`
	Fields        []string `json:"fields" yaml:"fields"`
	Fuzzy         bool     `json:"fuzzy" yaml:"fuzzy"`
	Closed        string   `json:"closed" yaml:"closed"`
	Merge         bool     `json:"merge" yaml:"merge"`
	Sort          string   `json:"sort" yaml:"sort"`
	Limit         int      `json:"limit" yaml:"limit"`
	RelatedLimit  int      `json:"related_limit" yaml:"related_limit"`
	EvidenceLimit int      `json:"evidence_limit" yaml:"evidence_limit"`
	DocumentCount int      `json:"document_count" yaml:"document_count"`
}

// BugSearchSource reports cache age and any live verification.
type BugSearchSource struct {
	Forge    string    `json:"forge" yaml:"forge"`
	Project  string    `json:"project" yaml:"project"`
	Source   string    `json:"source" yaml:"source"`
	SyncedAt time.Time `json:"synced_at,omitempty" yaml:"synced_at,omitempty"`
	Verified bool      `json:"verified,omitempty" yaml:"verified,omitempty"`
}

// BugEvidenceMatch describes one query concept inside an evidence excerpt.
type BugEvidenceMatch struct {
	QueryConcept string `json:"query_concept" yaml:"query_concept"`
	MatchedText  string `json:"matched_text" yaml:"matched_text"`
	MatchType    string `json:"match_type" yaml:"match_type"`
}

// BugMatchEvidence groups overlapping matches into one bounded excerpt.
type BugMatchEvidence struct {
	Field           string             `json:"field" yaml:"field"`
	SourceReference string             `json:"source_reference,omitempty" yaml:"source_reference,omitempty"`
	Excerpt         string             `json:"excerpt" yaml:"excerpt"`
	Matches         []BugEvidenceMatch `json:"matches" yaml:"matches"`
	TruncatedBefore bool               `json:"truncated_before,omitempty" yaml:"truncated_before,omitempty"`
	TruncatedAfter  bool               `json:"truncated_after,omitempty" yaml:"truncated_after,omitempty"`
	MatchTruncated  bool               `json:"match_truncated,omitempty" yaml:"match_truncated,omitempty"`
}

// BugSearchCoverage explains the evidence-coverage classification.
type BugSearchCoverage struct {
	MatchedConcepts   []string `json:"matched_concepts" yaml:"matched_concepts"`
	UnmatchedConcepts []string `json:"unmatched_concepts" yaml:"unmatched_concepts"`
	MatchedWeight     float64  `json:"matched_weight" yaml:"matched_weight"`
	TotalWeight       float64  `json:"total_weight" yaml:"total_weight"`
	Ratio             float64  `json:"ratio" yaml:"ratio"`
	MatchedClauses    int      `json:"matched_clauses" yaml:"matched_clauses"`
	TotalClauses      int      `json:"total_clauses" yaml:"total_clauses"`
}

// BugSearchResult is one bug- or task-scoped ranked candidate.
type BugSearchResult struct {
	Reference         string                  `json:"reference" yaml:"reference"`
	Forge             forge.ForgeType         `json:"forge" yaml:"forge"`
	ID                string                  `json:"id" yaml:"id"`
	Title             string                  `json:"title" yaml:"title"`
	URL               string                  `json:"url" yaml:"url"`
	Projects          []string                `json:"projects" yaml:"projects"`
	Status            []string                `json:"status" yaml:"status"`
	Importance        []string                `json:"importance" yaml:"importance"`
	Tags              []string                `json:"tags,omitempty" yaml:"tags,omitempty"`
	CreatedAt         time.Time               `json:"created_at" yaml:"created_at"`
	UpdatedAt         time.Time               `json:"updated_at" yaml:"updated_at"`
	Classification    BugSearchClassification `json:"classification" yaml:"classification"`
	Score             float64                 `json:"score" yaml:"score"`
	Tasks             []forge.BugTask         `json:"tasks" yaml:"tasks"`
	MatchedTasks      []string                `json:"matched_tasks,omitempty" yaml:"matched_tasks,omitempty"`
	Evidence          []BugMatchEvidence      `json:"evidence" yaml:"evidence"`
	EvidenceTotal     int                     `json:"evidence_total" yaml:"evidence_total"`
	EvidenceTruncated bool                    `json:"evidence_truncated" yaml:"evidence_truncated"`
	Coverage          BugSearchCoverage       `json:"coverage" yaml:"coverage"`
	Private           bool                    `json:"private,omitempty" yaml:"private,omitempty"`
	SecurityRelated   bool                    `json:"security_related,omitempty" yaml:"security_related,omitempty"`
	InformationType   string                  `json:"information_type,omitempty" yaml:"information_type,omitempty"`
	Provenance        []BugSearchSource       `json:"provenance" yaml:"provenance"`
}

// BugSearchResponse is the stable API/CLI/TUI result envelope.
type BugSearchResponse struct {
	Outcome        BugSearchOutcome  `json:"outcome" yaml:"outcome"`
	Results        []BugSearchResult `json:"results" yaml:"results"`
	Related        []BugSearchResult `json:"related" yaml:"related"`
	TotalResults   int               `json:"total_results" yaml:"total_results"`
	TotalRelated   int               `json:"total_related" yaml:"total_related"`
	Truncated      bool              `json:"truncated" yaml:"truncated"`
	Applied        BugSearchApplied  `json:"applied" yaml:"applied"`
	Provenance     []BugSearchSource `json:"provenance" yaml:"provenance"`
	Warnings       []string          `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	ScoringVersion string            `json:"scoring_version" yaml:"scoring_version"`
}
