// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

// ErrNoBugTrackerConfigured is returned when no bug tracker is configured.
var ErrNoBugTrackerConfigured = errors.New("no bug tracker configured")

// ErrInvalidBugSyncSince is returned when one bug sync request uses an invalid RFC 3339 timestamp.
var ErrInvalidBugSyncSince = errors.New("invalid since value: expected RFC 3339 timestamp")

// ErrInvalidBugID is returned when bug sync receives a non-numeric LP bug ID.
var ErrInvalidBugID = errors.New("invalid Launchpad bug ID")

func normalizeBugIDs(values []string) ([]string, error) {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimPrefix(strings.TrimSpace(value), "#")
		id, err := strconv.Atoi(value)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("%w %q", ErrInvalidBugID, value)
		}
		set[strconv.Itoa(id)] = true
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out, nil
}

func parseMergeState(s string) (forge.MergeState, error) {
	switch strings.ToLower(s) {
	case "open":
		return forge.MergeStateOpen, nil
	case "merged":
		return forge.MergeStateMerged, nil
	case "closed":
		return forge.MergeStateClosed, nil
	case "wip":
		return forge.MergeStateWIP, nil
	case "abandoned":
		return forge.MergeStateAbandoned, nil
	default:
		return 0, fmt.Errorf("invalid state %q (valid: open, merged, closed, wip, abandoned)", s)
	}
}

func parseForgeType(s string) (forge.ForgeType, error) {
	switch strings.ToLower(s) {
	case "github":
		return forge.ForgeGitHub, nil
	case "launchpad":
		return forge.ForgeLaunchpad, nil
	case "gerrit":
		return forge.ForgeGerrit, nil
	default:
		return 0, fmt.Errorf("invalid forge %q (valid: github, launchpad, gerrit)", s)
	}
}
