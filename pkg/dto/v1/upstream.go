// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package dto

// UpstreamSeries is upstream lifecycle metadata, not a distro support promise.
type UpstreamSeries struct {
	Name      string `json:"name" yaml:"name"`
	ReleaseID string `json:"release_id" yaml:"release_id"`
	Status    string `json:"status" yaml:"status"`
}

// DeliverableType classifies upstream project deliverables.
type DeliverableType int

const (
	DeliverableService DeliverableType = iota
	DeliverableLibrary
	DeliverableClient
	DeliverableOther
)

func (d DeliverableType) String() string {
	switch d {
	case DeliverableService:
		return "service"
	case DeliverableLibrary:
		return "library"
	case DeliverableClient:
		return "client"
	default:
		return "other"
	}
}

// Deliverable represents an upstream project with its latest version.
type Deliverable struct {
	Name    string          `json:"name" yaml:"name"`
	Type    DeliverableType `json:"type" yaml:"type"`
	Version string          `json:"version" yaml:"version"`
	Team    string          `json:"team,omitempty" yaml:"team,omitempty"`
}
