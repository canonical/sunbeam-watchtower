// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"errors"

	frontend "github.com/gboutry/sunbeam-watchtower/internal/adapter/primary/frontend"
	runtimeadapter "github.com/gboutry/sunbeam-watchtower/internal/adapter/primary/runtime"
	"github.com/spf13/cobra"
)

const (
	actionIDAnnotationKey       = "watchtower.action_id"
	actionSelectorAnnotationKey = "watchtower.action_selector"
)

func withActionID(cmd *cobra.Command, id frontend.ActionID) *cobra.Command {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[actionIDAnnotationKey] = string(id)
	return cmd
}

func withActionSelector(cmd *cobra.Command, selector string) *cobra.Command {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[actionSelectorAnnotationKey] = selector
	return cmd
}

func commandActionID(cmd *cobra.Command, args []string) frontend.ActionID {
	if cmd == nil {
		return ""
	}
	if raw := cmd.Annotations[actionIDAnnotationKey]; raw != "" {
		return frontend.ActionID(raw)
	}
	switch cmd.Annotations[actionSelectorAnnotationKey] {
	case "build.cleanup":
		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			return frontend.ActionBuildCleanupDryRun
		}
		return frontend.ActionBuildCleanupApply
	case "project.sync":
		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			return frontend.ActionProjectSyncDryRun
		}
		return frontend.ActionProjectSyncApply
	case "bug.sync":
		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			return frontend.ActionBugSyncDryRun
		}
		return frontend.ActionBugSyncApply
	case "team.sync":
		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			return frontend.ActionTeamSyncDryRun
		}
		return frontend.ActionTeamSyncApply
	case "cache.sync":
		return cacheSyncActionID(args)
	default:
		return ""
	}
}

func cacheSyncActionID(args []string) frontend.ActionID {
	if len(args) != 1 {
		return frontend.ActionCacheSync
	}
	switch args[0] {
	case cacheTypeGit:
		return frontend.ActionCacheSyncGit
	case cacheTypePackagesIndex:
		return frontend.ActionCacheSyncPackages
	case cacheTypePackageSets:
		return frontend.ActionCacheSyncPackageSets
	case cacheTypeUpstreamRepos:
		return frontend.ActionCacheSyncUpstream
	case cacheTypeBugs:
		return frontend.ActionCacheSyncBugs
	case cacheTypeExcuses:
		return frontend.ActionCacheSyncExcuses
	case cacheTypeReleases:
		return frontend.ActionCacheSyncReleases
	case cacheTypeReviews:
		return frontend.ActionCacheSyncReviews
	case cacheTypeSRU:
		return frontend.ActionCacheSyncSRU
	default:
		return frontend.ActionCacheSync
	}
}

func resolveAndRecordCommandActionID(cmd *cobra.Command, args []string) frontend.ActionID {
	id := commandActionID(cmd, args)
	if id == "" {
		return ""
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[actionIDAnnotationKey] = string(id)
	return id
}

func preflightCommandAccess(opts *Options, cmd *cobra.Command, args []string) error {
	id := resolveAndRecordCommandActionID(cmd, args)
	if id == "" {
		return errors.New("command is missing an action classification")
	}
	return runtimeadapter.CheckActionAccess(opts.AccessMode, id, false)
}
