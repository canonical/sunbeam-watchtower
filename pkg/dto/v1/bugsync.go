// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package dto

// BugSyncActionType describes the kind of sync action.
type BugSyncActionType string

// SchemaName returns a unique name for Huma OpenAPI schema registration.
func (BugSyncActionType) SchemaName() string { return "BugSyncActionType" }

const (
	BugSyncActionStatusUpdate     BugSyncActionType = "status_update"
	BugSyncActionSeriesAssignment BugSyncActionType = "series_assignment"
	BugSyncActionAddProjectTask   BugSyncActionType = "add_project_task"
)

// BugSyncAction represents a single action taken (or planned) during sync.
type BugSyncAction struct {
	BugID      string            `json:"bug_id" yaml:"bug_id"`
	TaskTitle  string            `json:"task_title" yaml:"task_title"`
	OldStatus  string            `json:"old_status,omitempty" yaml:"old_status,omitempty"`
	NewStatus  string            `json:"new_status,omitempty" yaml:"new_status,omitempty"`
	SelfLink   string            `json:"self_link,omitempty" yaml:"self_link,omitempty"`
	URL        string            `json:"url,omitempty" yaml:"url,omitempty"`
	Series     string            `json:"series,omitempty" yaml:"series,omitempty"`
	Project    string            `json:"project,omitempty" yaml:"project,omitempty"`
	Reason     string            `json:"reason,omitempty" yaml:"reason,omitempty"`
	Channel    string            `json:"channel,omitempty" yaml:"channel,omitempty"`
	Revision   int               `json:"revision,omitempty" yaml:"revision,omitempty"`
	Tag        string            `json:"tag,omitempty" yaml:"tag,omitempty"`
	Commit     string            `json:"commit,omitempty" yaml:"commit,omitempty"`
	ActionType BugSyncActionType `json:"action_type" yaml:"action_type"`
}

// SchemaName returns a unique name for Huma OpenAPI schema registration.
func (BugSyncAction) SchemaName() string { return "BugSyncAction" }

// BugSyncResult holds the outcome of a sync operation.
type BugSyncResult struct {
	Actions []BugSyncAction `json:"actions" yaml:"actions"`
	Skipped int             `json:"skipped" yaml:"skipped"`
	Errors  []error         `json:"-" yaml:"-"`
}
