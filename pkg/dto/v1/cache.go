// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package dto

import (
	"time"

	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

// BugCacheSchemaVersion is the current offline-search cache contract.
const BugCacheSchemaVersion = 1

// SyncOptions controls additional behavior when syncing a cached repository.
type SyncOptions struct {
	ExtraRefSpecs []string
}

// MRMetadata stores merge request information as a sidecar to the cached repo.
type MRMetadata struct {
	ID      string           `json:"id"`
	State   forge.MergeState `json:"state"`
	URL     string           `json:"url"`
	HeadSHA string           `json:"head_sha"`
	GitRef  string           `json:"git_ref"`
}

// SourceEntry represents a single APT Sources index to download.
type SourceEntry struct {
	Mirror    string `json:"mirror" yaml:"mirror"`
	Suite     string `json:"suite" yaml:"suite"`
	Component string `json:"component" yaml:"component"`
}

// QueryOpts controls filtering when querying the cache.
type QueryOpts struct {
	Packages   []string `json:"packages,omitempty" yaml:"packages,omitempty"`
	Suites     []string `json:"suites,omitempty" yaml:"suites,omitempty"`
	Components []string `json:"components,omitempty" yaml:"components,omitempty"`
}

// BugCacheStatus reports per-project bug cache statistics.
type BugCacheStatus struct {
	ForgeType     string    `json:"forge_type" yaml:"forge_type"`
	Project       string    `json:"project" yaml:"project"`
	BugCount      int       `json:"bug_count" yaml:"bug_count"`
	TaskCount     int       `json:"task_count" yaml:"task_count"`
	LastSync      time.Time `json:"last_sync" yaml:"last_sync"`
	SchemaVersion int       `json:"schema_version" yaml:"schema_version"`
	NeedsRefresh  bool      `json:"needs_refresh" yaml:"needs_refresh"`
}

// ReviewCacheStatus reports per-project review cache statistics.
type ReviewCacheStatus struct {
	ForgeType    string    `json:"forge_type" yaml:"forge_type"`
	Project      string    `json:"project" yaml:"project"`
	SummaryCount int       `json:"summary_count" yaml:"summary_count"`
	DetailCount  int       `json:"detail_count" yaml:"detail_count"`
	LastSync     time.Time `json:"last_sync" yaml:"last_sync"`
}

// CacheStatus reports metadata about a cached source group.
type CacheStatus struct {
	Name        string    `json:"name" yaml:"name"`
	EntryCount  int       `json:"entry_count" yaml:"entry_count"`
	LastUpdated time.Time `json:"last_updated" yaml:"last_updated"`
	DiskSize    int64     `json:"disk_size" yaml:"disk_size"`
}
