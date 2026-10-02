package api

import (
	"errors"
	"fmt"
	"strings"

	frontend "github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

func isFrontendValidationError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, frontend.ErrInvalidBugSyncSince) {
		return true
	}
	msg := err.Error()
	return strings.HasPrefix(msg, "invalid forge ") || strings.HasPrefix(msg, "invalid state ")
}

func parseAPIMergeState(s string) (forge.MergeState, error) {
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

func parseAPIForgeType(s string) (forge.ForgeType, error) {
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
