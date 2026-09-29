// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import "sort"

// ActionID identifies one canonical user-invoked frontend action.
type ActionID string

// Mutability classifies whether one action mutates durable or remote state.
type Mutability string

// LocalEffect describes whether one action touches local machine state.
type LocalEffect string

// RuntimeRequirement describes whether an action requires a persistent server target.
type RuntimeRequirement string

// ExportPolicy controls whether future frontends such as MCP should expose an action.
type ExportPolicy string

const (
	MutabilityRead  Mutability = "read"
	MutabilityWrite Mutability = "write"
)

const (
	LocalEffectNone  LocalEffect = "none"
	LocalEffectRead  LocalEffect = "local_read"
	LocalEffectWrite LocalEffect = "local_write"
)

const (
	RuntimeEmbeddedOK         RuntimeRequirement = "embedded_ok"
	RuntimePersistentRequired RuntimeRequirement = "persistent_required"
)

const (
	ExportPolicyAllowed ExportPolicy = "allowed"
	ExportPolicyHidden  ExportPolicy = "hidden"
)

const (
	ActionAuthStatus            ActionID = "auth.status"
	ActionAuthLaunchpadBegin    ActionID = "auth.launchpad.begin"
	ActionAuthLaunchpadFinalize ActionID = "auth.launchpad.finalize"
	ActionAuthLaunchpadLogout   ActionID = "auth.launchpad.logout"
	ActionAuthGitHubBegin       ActionID = "auth.github.begin"
	ActionAuthGitHubFinalize    ActionID = "auth.github.finalize"
	ActionAuthGitHubLogout      ActionID = "auth.github.logout"
	ActionAuthSnapStoreBegin    ActionID = "auth.snapstore.begin"
	ActionAuthSnapStoreSave     ActionID = "auth.snapstore.save"
	ActionAuthSnapStoreLogout   ActionID = "auth.snapstore.logout"
	ActionAuthCharmhubBegin     ActionID = "auth.charmhub.begin"
	ActionAuthCharmhubSave      ActionID = "auth.charmhub.save"
	ActionAuthCharmhubLogout    ActionID = "auth.charmhub.logout"
	ActionBuildTrigger          ActionID = "build.trigger"
	ActionBuildList             ActionID = "build.list"
	ActionBuildDownload         ActionID = "build.download"
	ActionBuildRetry            ActionID = "build.retry"
	ActionBuildCancel           ActionID = "build.cancel"
	ActionBuildCleanupDryRun    ActionID = "build.cleanup.dry_run"
	ActionBuildCleanupApply     ActionID = "build.cleanup.apply"
	ActionBugShow               ActionID = "bug.show"
	ActionBugList               ActionID = "bug.list"
	ActionBugSearch             ActionID = "bug.search"
	ActionBugSyncDryRun         ActionID = "bug.sync.dry_run"
	ActionBugSyncApply          ActionID = "bug.sync.apply"
	ActionSRUList               ActionID = "sru.list"
	ActionSRUShow               ActionID = "sru.show"
	ActionSRUOpen               ActionID = "sru.open"
	ActionCacheStatus           ActionID = "cache.status"
	ActionCacheSync             ActionID = "cache.sync"
	ActionCacheSyncGit          ActionID = "cache.sync.git"
	ActionCacheSyncPackages     ActionID = "cache.sync.packages"
	ActionCacheSyncPackageSets  ActionID = "cache.sync.packagesets"
	ActionCacheSyncUpstream     ActionID = "cache.sync.upstream"
	ActionCacheSyncBugs         ActionID = "cache.sync.bugs"
	ActionCacheSyncExcuses      ActionID = "cache.sync.excuses"
	ActionCacheSyncReleases     ActionID = "cache.sync.releases"
	ActionCacheSyncReviews      ActionID = "cache.sync.reviews"
	ActionCacheSyncSRU          ActionID = "cache.sync.sru"
	ActionCacheClear            ActionID = "cache.clear"
	ActionCommitLog             ActionID = "commit.log"
	ActionCommitTrack           ActionID = "commit.track"
	ActionConfigShow            ActionID = "config.show"
	ActionConfigReload          ActionID = "config.reload"
	ActionDashboardRefresh      ActionID = "dashboard.refresh"
	ActionBuildsRefresh         ActionID = "builds.refresh"
	ActionReleasesRefresh       ActionID = "releases.refresh"
	ActionReleaseDetailRefresh  ActionID = "releases.detail.refresh"
	ActionOperationsRefresh     ActionID = "operations.refresh"
	ActionAuthRefresh           ActionID = "auth.refresh"
	ActionCacheRefresh          ActionID = "cache.refresh"
	ActionLogsRefresh           ActionID = "logs.refresh"
	ActionOperationList         ActionID = "operation.list"
	ActionOperationShow         ActionID = "operation.show"
	ActionOperationEvents       ActionID = "operation.events"
	ActionOperationWait         ActionID = "operation.wait"
	ActionOperationCancel       ActionID = "operation.cancel"
	ActionPackagesDiff          ActionID = "packages.diff"
	ActionPackagesShowVersion   ActionID = "packages.show_version"
	ActionPackagesShowDetail    ActionID = "packages.show_detail"
	ActionPackagesList          ActionID = "packages.list"
	ActionPackagesDsc           ActionID = "packages.dsc"
	ActionPackagesRdepends      ActionID = "packages.rdepends"
	ActionPackagesExcusesList   ActionID = "packages.excuses.list"
	ActionPackagesExcusesShow   ActionID = "packages.excuses.show"
	ActionProjectSyncDryRun     ActionID = "project.sync.dry_run"
	ActionProjectSyncApply      ActionID = "project.sync.apply"
	ActionTeamSyncDryRun        ActionID = "team.sync.dry_run"
	ActionTeamSyncApply         ActionID = "team.sync.apply"
	ActionReleaseList           ActionID = "releases.list"
	ActionReleaseShow           ActionID = "releases.show"
	ActionReviewList            ActionID = "review.list"
	ActionReviewShow            ActionID = "review.show"
	ActionServeStart            ActionID = "serve.start"
	ActionServerStart           ActionID = "server.start"
	ActionServerStatus          ActionID = "server.status"
	ActionServerStop            ActionID = "server.stop"
	ActionServerSwitchTarget    ActionID = "server.switch_target"
	ActionVersionShow           ActionID = "version.show"
)

// ActionDescriptor documents the access and export characteristics of one action.
type ActionDescriptor struct {
	ID                 ActionID
	Domain             string
	ResourceKind       string
	Mutability         Mutability
	LocalEffect        LocalEffect
	RuntimeRequirement RuntimeRequirement
	ExportPolicy       ExportPolicy
	Summary            string
}

var actionCatalog = map[ActionID]ActionDescriptor{
	ActionAuthStatus:            descriptor(ActionAuthStatus, "auth", "auth", MutabilityRead, LocalEffectNone, RuntimePersistentRequired, ExportPolicyAllowed, "Show authentication status."),
	ActionAuthLaunchpadBegin:    descriptor(ActionAuthLaunchpadBegin, "auth", "auth", MutabilityWrite, LocalEffectNone, RuntimePersistentRequired, ExportPolicyAllowed, "Begin Launchpad authentication."),
	ActionAuthLaunchpadFinalize: descriptor(ActionAuthLaunchpadFinalize, "auth", "auth", MutabilityWrite, LocalEffectNone, RuntimePersistentRequired, ExportPolicyAllowed, "Finalize Launchpad authentication."),
	ActionAuthLaunchpadLogout:   descriptor(ActionAuthLaunchpadLogout, "auth", "auth", MutabilityWrite, LocalEffectNone, RuntimePersistentRequired, ExportPolicyAllowed, "Clear persisted Launchpad credentials."),
	ActionAuthGitHubBegin:       descriptor(ActionAuthGitHubBegin, "auth", "auth", MutabilityWrite, LocalEffectNone, RuntimePersistentRequired, ExportPolicyAllowed, "Begin GitHub authentication."),
	ActionAuthGitHubFinalize:    descriptor(ActionAuthGitHubFinalize, "auth", "auth", MutabilityWrite, LocalEffectNone, RuntimePersistentRequired, ExportPolicyAllowed, "Finalize GitHub authentication."),
	ActionAuthGitHubLogout:      descriptor(ActionAuthGitHubLogout, "auth", "auth", MutabilityWrite, LocalEffectNone, RuntimePersistentRequired, ExportPolicyAllowed, "Clear persisted GitHub credentials."),
	ActionAuthSnapStoreBegin:    descriptor(ActionAuthSnapStoreBegin, "auth", "auth", MutabilityWrite, LocalEffectNone, RuntimePersistentRequired, ExportPolicyAllowed, "Begin Snap Store authentication."),
	ActionAuthSnapStoreSave:     descriptor(ActionAuthSnapStoreSave, "auth", "auth", MutabilityWrite, LocalEffectNone, RuntimePersistentRequired, ExportPolicyAllowed, "Save discharged Snap Store credential."),
	ActionAuthSnapStoreLogout:   descriptor(ActionAuthSnapStoreLogout, "auth", "auth", MutabilityWrite, LocalEffectNone, RuntimePersistentRequired, ExportPolicyAllowed, "Clear persisted Snap Store credentials."),
	ActionAuthCharmhubBegin:     descriptor(ActionAuthCharmhubBegin, "auth", "auth", MutabilityWrite, LocalEffectNone, RuntimePersistentRequired, ExportPolicyAllowed, "Begin Charmhub authentication."),
	ActionAuthCharmhubSave:      descriptor(ActionAuthCharmhubSave, "auth", "auth", MutabilityWrite, LocalEffectNone, RuntimePersistentRequired, ExportPolicyAllowed, "Save discharged Charmhub credential."),
	ActionAuthCharmhubLogout:    descriptor(ActionAuthCharmhubLogout, "auth", "auth", MutabilityWrite, LocalEffectNone, RuntimePersistentRequired, ExportPolicyAllowed, "Clear persisted Charmhub credentials."),
	ActionBuildTrigger:          descriptor(ActionBuildTrigger, "build", "build", MutabilityWrite, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Trigger builds for a project."),
	ActionBuildList:             descriptor(ActionBuildList, "build", "build", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "List builds."),
	ActionBuildDownload:         descriptor(ActionBuildDownload, "build", "build", MutabilityRead, LocalEffectWrite, RuntimeEmbeddedOK, ExportPolicyAllowed, "Download build artifacts to the local machine."),
	ActionBuildRetry:            descriptor(ActionBuildRetry, "build", "build", MutabilityWrite, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Retry a failed build."),
	ActionBuildCancel:           descriptor(ActionBuildCancel, "build", "build", MutabilityWrite, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Cancel an active build."),
	ActionBuildCleanupDryRun:    descriptor(ActionBuildCleanupDryRun, "build", "build", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Preview temporary build recipe cleanup."),
	ActionBuildCleanupApply:     descriptor(ActionBuildCleanupApply, "build", "build", MutabilityWrite, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Delete temporary build recipes."),
	ActionBugShow:               descriptor(ActionBugShow, "bug", "bug", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Show one bug and its tasks."),
	ActionBugList:               descriptor(ActionBugList, "bug", "bug", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "List bug tasks."),
	ActionBugSearch:             descriptor(ActionBugSearch, "bug", "bug", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Search bugs with match explanations."),
	ActionBugSyncDryRun:         descriptor(ActionBugSyncDryRun, "bug", "bug", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Preview bug synchronization."),
	ActionBugSyncApply:          descriptor(ActionBugSyncApply, "bug", "bug", MutabilityWrite, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Synchronize bug state from cached commits."),
	ActionSRUList:               descriptor(ActionSRUList, "sru", "sru", MutabilityRead, LocalEffectRead, RuntimeEmbeddedOK, ExportPolicyAllowed, "List monitored OpenStack SRUs."),
	ActionSRUShow:               descriptor(ActionSRUShow, "sru", "sru", MutabilityRead, LocalEffectRead, RuntimeEmbeddedOK, ExportPolicyAllowed, "Show one monitored SRU."),
	ActionSRUOpen:               descriptor(ActionSRUOpen, "sru", "sru", MutabilityRead, LocalEffectWrite, RuntimeEmbeddedOK, ExportPolicyHidden, "Open a monitored SRU in Launchpad."),
	ActionCacheStatus:           descriptor(ActionCacheStatus, "cache", "cache", MutabilityRead, LocalEffectRead, RuntimeEmbeddedOK, ExportPolicyAllowed, "Show cache status."),
	ActionCacheSync:             descriptor(ActionCacheSync, "cache", "cache", MutabilityWrite, LocalEffectRead, RuntimeEmbeddedOK, ExportPolicyAllowed, "Synchronize multiple cache types."),
	ActionCacheSyncGit:          descriptor(ActionCacheSyncGit, "cache", "cache", MutabilityWrite, LocalEffectRead, RuntimeEmbeddedOK, ExportPolicyAllowed, "Synchronize git caches."),
	ActionCacheSyncPackages:     descriptor(ActionCacheSyncPackages, "cache", "cache", MutabilityWrite, LocalEffectRead, RuntimeEmbeddedOK, ExportPolicyAllowed, "Synchronize package index caches."),
	ActionCacheSyncPackageSets:  descriptor(ActionCacheSyncPackageSets, "cache", "cache", MutabilityWrite, LocalEffectRead, RuntimeEmbeddedOK, ExportPolicyAllowed, "Synchronize Launchpad packageset caches."),
	ActionCacheSyncUpstream:     descriptor(ActionCacheSyncUpstream, "cache", "cache", MutabilityWrite, LocalEffectRead, RuntimeEmbeddedOK, ExportPolicyAllowed, "Synchronize upstream caches."),
	ActionCacheSyncBugs:         descriptor(ActionCacheSyncBugs, "cache", "cache", MutabilityWrite, LocalEffectRead, RuntimeEmbeddedOK, ExportPolicyAllowed, "Synchronize bug caches."),
	ActionCacheSyncExcuses:      descriptor(ActionCacheSyncExcuses, "cache", "cache", MutabilityWrite, LocalEffectRead, RuntimeEmbeddedOK, ExportPolicyAllowed, "Synchronize excuses caches."),
	ActionCacheSyncReleases:     descriptor(ActionCacheSyncReleases, "cache", "cache", MutabilityWrite, LocalEffectRead, RuntimeEmbeddedOK, ExportPolicyAllowed, "Synchronize release caches."),
	ActionCacheSyncReviews:      descriptor(ActionCacheSyncReviews, "cache", "cache", MutabilityWrite, LocalEffectRead, RuntimeEmbeddedOK, ExportPolicyAllowed, "Synchronize review caches."),
	ActionCacheSyncSRU:          descriptor(ActionCacheSyncSRU, "cache", "cache", MutabilityWrite, LocalEffectWrite, RuntimeEmbeddedOK, ExportPolicyAllowed, "Synchronize the local SRU snapshot from Launchpad."),
	ActionCacheClear:            descriptor(ActionCacheClear, "cache", "cache", MutabilityWrite, LocalEffectWrite, RuntimeEmbeddedOK, ExportPolicyAllowed, "Clear cached data."),
	ActionCommitLog:             descriptor(ActionCommitLog, "commit", "commit", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "List commits."),
	ActionCommitTrack:           descriptor(ActionCommitTrack, "commit", "commit", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Track commits related to a bug."),
	ActionConfigShow:            descriptor(ActionConfigShow, "system", "system", MutabilityRead, LocalEffectRead, RuntimeEmbeddedOK, ExportPolicyAllowed, "Show current configuration."),
	ActionConfigReload:          descriptor(ActionConfigReload, "system", "system", MutabilityWrite, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Reload configuration from file."),
	ActionDashboardRefresh:      descriptor(ActionDashboardRefresh, "system", "system", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Refresh the TUI dashboard."),
	ActionBuildsRefresh:         descriptor(ActionBuildsRefresh, "build", "build", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Refresh the TUI builds view."),
	ActionReleasesRefresh:       descriptor(ActionReleasesRefresh, "release", "release", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Refresh the TUI releases view."),
	ActionReleaseDetailRefresh:  descriptor(ActionReleaseDetailRefresh, "release", "release", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Refresh the TUI release detail view."),
	ActionOperationsRefresh:     descriptor(ActionOperationsRefresh, "operation", "operation", MutabilityRead, LocalEffectNone, RuntimePersistentRequired, ExportPolicyAllowed, "Refresh the TUI operations view."),
	ActionAuthRefresh:           descriptor(ActionAuthRefresh, "auth", "auth", MutabilityRead, LocalEffectNone, RuntimePersistentRequired, ExportPolicyAllowed, "Refresh the TUI auth view."),
	ActionCacheRefresh:          descriptor(ActionCacheRefresh, "cache", "cache", MutabilityRead, LocalEffectRead, RuntimeEmbeddedOK, ExportPolicyAllowed, "Refresh the TUI cache view."),
	ActionLogsRefresh:           descriptor(ActionLogsRefresh, "system", "system", MutabilityRead, LocalEffectRead, RuntimeEmbeddedOK, ExportPolicyHidden, "Refresh the TUI logs view."),
	ActionOperationList:         descriptor(ActionOperationList, "operation", "operation", MutabilityRead, LocalEffectNone, RuntimePersistentRequired, ExportPolicyAllowed, "List operations."),
	ActionOperationShow:         descriptor(ActionOperationShow, "operation", "operation", MutabilityRead, LocalEffectNone, RuntimePersistentRequired, ExportPolicyAllowed, "Show one operation."),
	ActionOperationEvents:       descriptor(ActionOperationEvents, "operation", "operation", MutabilityRead, LocalEffectNone, RuntimePersistentRequired, ExportPolicyAllowed, "Show operation events."),
	ActionOperationWait:         descriptor(ActionOperationWait, "operation", "operation", MutabilityRead, LocalEffectNone, RuntimePersistentRequired, ExportPolicyAllowed, "Wait for one operation to finish."),
	ActionOperationCancel:       descriptor(ActionOperationCancel, "operation", "operation", MutabilityWrite, LocalEffectNone, RuntimePersistentRequired, ExportPolicyAllowed, "Request cancellation for one operation."),
	ActionPackagesDiff:          descriptor(ActionPackagesDiff, "package", "package", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Compare package versions across distros."),
	ActionPackagesShowVersion:   descriptor(ActionPackagesShowVersion, "package", "package", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Show versions for one package."),
	ActionPackagesShowDetail:    descriptor(ActionPackagesShowDetail, "package", "package", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Show detailed metadata for one package."),
	ActionPackagesList:          descriptor(ActionPackagesList, "package", "package", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "List packages from configured sources."),
	ActionPackagesDsc:           descriptor(ActionPackagesDsc, "package", "package", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Resolve DSC files for packages."),
	ActionPackagesRdepends:      descriptor(ActionPackagesRdepends, "package", "package", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Show reverse dependencies."),
	ActionPackagesExcusesList:   descriptor(ActionPackagesExcusesList, "package", "package", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "List migration excuses."),
	ActionPackagesExcusesShow:   descriptor(ActionPackagesExcusesShow, "package", "package", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Show one migration excuse."),
	ActionProjectSyncDryRun:     descriptor(ActionProjectSyncDryRun, "project", "project", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Preview project synchronization."),
	ActionProjectSyncApply:      descriptor(ActionProjectSyncApply, "project", "project", MutabilityWrite, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Synchronize Launchpad project metadata."),
	ActionTeamSyncDryRun:        descriptor(ActionTeamSyncDryRun, "team", "team", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Preview team collaborator synchronization."),
	ActionTeamSyncApply:         descriptor(ActionTeamSyncApply, "team", "team", MutabilityWrite, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Synchronize team members as store collaborators."),
	ActionReleaseList:           descriptor(ActionReleaseList, "release", "release", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "List releases."),
	ActionReleaseShow:           descriptor(ActionReleaseShow, "release", "release", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Show one release."),
	ActionReviewList:            descriptor(ActionReviewList, "review", "review", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "List reviews."),
	ActionReviewShow:            descriptor(ActionReviewShow, "review", "review", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Show one review."),
	ActionServeStart:            descriptor(ActionServeStart, "server", "server", MutabilityWrite, LocalEffectWrite, RuntimeEmbeddedOK, ExportPolicyHidden, "Start the HTTP API server."),
	ActionServerStart:           descriptor(ActionServerStart, "server", "server", MutabilityWrite, LocalEffectWrite, RuntimeEmbeddedOK, ExportPolicyHidden, "Start the local persistent server."),
	ActionServerStatus:          descriptor(ActionServerStatus, "server", "server", MutabilityRead, LocalEffectRead, RuntimeEmbeddedOK, ExportPolicyAllowed, "Show local server status."),
	ActionServerStop:            descriptor(ActionServerStop, "server", "server", MutabilityWrite, LocalEffectWrite, RuntimeEmbeddedOK, ExportPolicyHidden, "Stop the local persistent server."),
	ActionServerSwitchTarget:    descriptor(ActionServerSwitchTarget, "server", "server", MutabilityWrite, LocalEffectWrite, RuntimeEmbeddedOK, ExportPolicyHidden, "Switch the TUI session to the local persistent server."),
	ActionVersionShow:           descriptor(ActionVersionShow, "system", "system", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Show the Watchtower version."),
}

func descriptor(
	id ActionID,
	domain string,
	resourceKind string,
	mutability Mutability,
	localEffect LocalEffect,
	runtimeRequirement RuntimeRequirement,
	exportPolicy ExportPolicy,
	summary string,
) ActionDescriptor {
	return ActionDescriptor{
		ID:                 id,
		Domain:             domain,
		ResourceKind:       resourceKind,
		Mutability:         mutability,
		LocalEffect:        localEffect,
		RuntimeRequirement: runtimeRequirement,
		ExportPolicy:       exportPolicy,
		Summary:            summary,
	}
}

// DescribeAction returns the catalog descriptor for one action ID.
func DescribeAction(id ActionID) ActionDescriptor {
	if desc, ok := actionCatalog[id]; ok {
		return desc
	}
	return ActionDescriptor{
		ID:                 id,
		Domain:             "system",
		ResourceKind:       "system",
		Mutability:         MutabilityWrite,
		LocalEffect:        LocalEffectNone,
		RuntimeRequirement: RuntimeEmbeddedOK,
		ExportPolicy:       ExportPolicyHidden,
		Summary:            "Unknown action.",
	}
}

// AllActions returns the full action catalog in stable order.
func AllActions() []ActionDescriptor {
	actions := make([]ActionDescriptor, 0, len(actionCatalog))
	for _, desc := range actionCatalog {
		actions = append(actions, desc)
	}
	sort.Slice(actions, func(i, j int) bool {
		return actions[i].ID < actions[j].ID
	})
	return actions
}

// ReadOnlyExportedActions returns the actions eligible for default MCP exposure.
func ReadOnlyExportedActions() []ActionDescriptor {
	exported := make([]ActionDescriptor, 0)
	for _, desc := range AllActions() {
		if desc.ExportPolicy != ExportPolicyAllowed {
			continue
		}
		if desc.Mutability != MutabilityRead {
			continue
		}
		exported = append(exported, desc)
	}
	return exported
}

// IsAllowedInReadOnlyMode reports whether one action can run without an explicit override.
func IsAllowedInReadOnlyMode(id ActionID, override bool) bool {
	desc := DescribeAction(id)
	if desc.Mutability == MutabilityRead {
		return true
	}
	return override
}
