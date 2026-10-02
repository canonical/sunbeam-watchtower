// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/app"
	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	"github.com/canonical/sunbeam-watchtower/internal/core/service/build"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// PreparedBuildTriggerRequest holds build trigger fields after frontend-side preparation.
type PreparedBuildTriggerRequest struct {
	Project   string
	Artifacts []string
	Wait      bool
	Timeout   time.Duration
	Owner     string
	Prefix    string
	Prepared  *dto.PreparedBuildSource
}

// PreparedBuildListRequest holds build list fields after frontend-side preparation.
type PreparedBuildListRequest struct {
	Projects     []string
	All          bool
	State        string
	Owner        string
	TargetRef    string
	RecipePrefix string
}

// PreparedBuildDownloadRequest holds build download fields after frontend-side preparation.
type PreparedBuildDownloadRequest struct {
	Project      string
	Artifacts    []string
	ArtifactsDir string
	Owner        string
	TargetRef    string
	RecipePrefix string
}

// LocalBuildPreparer handles frontend-side local preparation for split build workflows.
type LocalBuildPreparer struct {
	gitClient   port.GitClient
	repoManager port.RepoManager
	builders    map[string]build.ProjectBuilder
	cmdRunner   port.CommandRunner
}

// NewLocalBuildPreparer creates a reusable local build preparer.
func NewLocalBuildPreparer(
	gitClient port.GitClient,
	repoManager port.RepoManager,
	builders map[string]build.ProjectBuilder,
	cmdRunner port.CommandRunner,
) *LocalBuildPreparer {
	return &LocalBuildPreparer{
		gitClient:   gitClient,
		repoManager: repoManager,
		builders:    builders,
		cmdRunner:   cmdRunner,
	}
}

// PrepareTrigger resolves local git and Launchpad state and returns a prepared build-trigger request.
func (p *LocalBuildPreparer) PrepareTrigger(
	ctx context.Context,
	req PreparedBuildTriggerRequest,
	localPath string,
) (PreparedBuildTriggerRequest, error) {
	if p.repoManager == nil {
		return req, app.ErrLaunchpadAuthRequired
	}
	pb, ok := p.builders[req.Project]
	if !ok {
		return req, fmt.Errorf("unknown project %q", req.Project)
	}

	// 1. Resolve HEAD SHA from local clone.
	sha, err := p.gitClient.HeadSHA(localPath)
	if err != nil {
		return req, fmt.Errorf("resolve HEAD SHA: %w", err)
	}
	shortSHA := sha
	if len(shortSHA) > 8 {
		shortSHA = shortSHA[:8]
	}

	// 2. Resolve LP owner.
	lpOwner := req.Owner
	if lpOwner == "" {
		lpOwner, err = p.repoManager.GetCurrentUser(ctx)
		if err != nil {
			return req, fmt.Errorf("get current LP user: %w", err)
		}
	}
	req.Owner = lpOwner

	// 3. Get or create personal LP project and repo.
	lpProject, err := p.repoManager.GetOrCreateProject(ctx, lpOwner)
	if err != nil {
		return req, fmt.Errorf("get/create LP project: %w", err)
	}

	repoSelfLink, gitSSHURL, err := p.repoManager.GetOrCreateRepo(ctx, lpOwner, lpProject, req.Project)
	if err != nil {
		return req, fmt.Errorf("get/create LP repo: %w", err)
	}

	// 4. Build branch name.
	branchName := "tmp-" + req.Prefix + "-" + shortSHA
	refPath := "refs/heads/" + branchName

	// 5. Check if branch already exists on LP.
	_, refCheckErr := p.repoManager.GetGitRef(ctx, repoSelfLink, refPath)
	skipPush := refCheckErr == nil

	// 6. Prepare and optionally push. discoverPath is the path discovery
	// should read — localPath for the no-prepare case, the temp worktree
	// for the prepare case (so LP and discovery see the same source).
	// cleanupWorktree is always registered via defer so it fires even when
	// prepareAndPush returns an error after creating the worktree.
	var cleanupWorktree func()
	defer func() {
		if cleanupWorktree != nil {
			cleanupWorktree()
		}
	}()
	discoverPath, cleanupWorktree, err := p.prepareAndPush(ctx, localPath, gitSSHURL, repoSelfLink, lpOwner, branchName, sha, pb.PrepareCommand, skipPush)
	if err != nil {
		return req, fmt.Errorf("prepare and push: %w", err)
	}

	// 7. Always wait for the ref to be usable on LP.
	// LP is slow to process git pushes — even if GetGitRef returned a
	// constructed self_link, recipe creation can fail until the ref is
	// fully indexed. WaitForGitRef polls until LP confirms the ref.
	refLink, err := p.repoManager.WaitForGitRef(ctx, repoSelfLink, refPath, 10*time.Minute)
	if err != nil {
		return req, fmt.Errorf("wait for git ref: %w", err)
	}

	// 8. Discover artifacts. When prepare_command is set, discovery reads
	// the prepared worktree so LP and discovery see the same source.
	// Discovery always runs in local mode so nested monorepo layouts
	// (e.g. charms/storage/foo) reach Launchpad with the correct build
	// path. When the caller supplied an explicit artifact list we filter
	// the discovered recipes by that list and surface an error for any
	// name that isn't present locally.
	discovered, err := pb.Strategy.DiscoverRecipes(discoverPath)
	if err != nil {
		return req, fmt.Errorf("discover artifacts: %w", err)
	}

	recipes := discovered
	if len(req.Artifacts) > 0 {
		byName := make(map[string]build.DiscoveredRecipe, len(discovered))
		for _, r := range discovered {
			byName[r.Name] = r
		}
		recipes = make([]build.DiscoveredRecipe, 0, len(req.Artifacts))
		var missing []string
		for _, name := range req.Artifacts {
			r, ok := byName[name]
			if !ok {
				missing = append(missing, name)
				continue
			}
			recipes = append(recipes, r)
		}
		if len(missing) > 0 {
			return req, fmt.Errorf("artifact(s) not found in local repo %q: %s",
				localPath, strings.Join(missing, ", "))
		}
	}

	// Filter out skipped artifacts.
	if len(pb.SkipArtifacts) > 0 {
		skip := make(map[string]bool, len(pb.SkipArtifacts))
		for _, s := range pb.SkipArtifacts {
			skip[s] = true
		}
		filtered := recipes[:0]
		for _, r := range recipes {
			if !skip[r.Name] {
				filtered = append(filtered, r)
			}
		}
		recipes = filtered
	}

	tempNames := make([]string, 0, len(recipes))
	buildPaths := make(map[string]string, len(recipes))
	processors := make(map[string][]string, len(recipes))
	for _, r := range recipes {
		tempName := pb.Strategy.TempRecipeName(r.Name, sha, req.Prefix)
		tempNames = append(tempNames, tempName)
		// r.RelPath is empty for single-artifact repos (metadata at root),
		// which is the correct build_path value for Launchpad in that case.
		buildPaths[tempName] = r.RelPath
		// LP snap.requestBuilds takes no architectures arg — the snap's
		// processors field drives dispatch. Auto-detect from snapcraft.yaml.
		if pb.Strategy.ArtifactType() == dto.ArtifactSnap {
			procs, err := snapProcessorsFromRepo(discoverPath, r, pb.Strategy)
			if err != nil {
				return req, fmt.Errorf("parse snap platforms for %q: %w", r.Name, err)
			}
			if len(procs) > 0 {
				processors[tempName] = procs
			}
		}
	}
	req.Artifacts = tempNames

	// 9. Build PreparedBuildSource with one entry per artifact.
	req.Prepared = &dto.PreparedBuildSource{
		Backend:       dto.PreparedBuildBackendLaunchpad,
		TargetRef:     lpProject,
		RepositoryRef: repoSelfLink,
		Recipes:       make(map[string]dto.PreparedBuildRecipe, len(tempNames)),
	}
	for _, name := range tempNames {
		req.Prepared.Recipes[name] = dto.PreparedBuildRecipe{
			SourceRef:  refLink,
			BuildPath:  buildPaths[name],
			Processors: processors[name],
		}
	}

	return req, nil
}

// snapProcessorsFromRepo reads the snap metadata file from the local repo and
// returns the parsed platforms. Looks at snap/snapcraft.yaml first, then at
// the repo root (matching SnapStrategy.DiscoverRecipes). Returns nil when no
// metadata file is found, so the caller can fall back to LP defaults.
func snapProcessorsFromRepo(repoPath string, r build.DiscoveredRecipe, strategy build.ArtifactStrategy) ([]string, error) {
	metaName := strategy.MetadataFileName()
	candidates := []string{
		filepath.Join(repoPath, r.RelPath, "snap", metaName),
		filepath.Join(repoPath, r.RelPath, metaName),
	}
	for _, candidate := range candidates {
		content, err := os.ReadFile(candidate)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		return strategy.ParsePlatforms(content)
	}
	return nil, nil
}

// prepareAndPush isolates the optional prepare command in a temporary
// linked worktree, stages + commits its outputs (bypassing .gitignore),
// and pushes from there. When prepareCommand is empty, it preserves the
// prior branch-dance flow on the live repo via pushFromLocalPath.
//
// Returns the path that subsequent discovery should read from (localPath
// or the temp worktree) plus a cleanup closure the caller must defer.
// cleanup is a no-op when prepareCommand is empty.
//
// When skipPush is true (LP ref already exists), the push is skipped but
// the worktree is still materialised and prepare still runs — so
// discovery has a prepared tree.
func (p *LocalBuildPreparer) prepareAndPush(
	ctx context.Context,
	localPath, gitSSHURL, repoSelfLink, lpOwner, branchName, sha, prepareCommand string,
	skipPush bool,
) (discoverPath string, cleanup func(), err error) {
	cleanup = func() {}

	if prepareCommand == "" {
		// Non-prepare path: byte-for-byte the old behaviour.
		if !skipPush {
			if err := pushFromLocalPath(ctx, p.gitClient, p.repoManager, localPath, gitSSHURL, repoSelfLink, lpOwner, branchName, sha); err != nil {
				return "", cleanup, fmt.Errorf("push from local path: %w", err)
			}
		}
		return localPath, cleanup, nil
	}

	if p.cmdRunner == nil {
		return "", cleanup, fmt.Errorf("prepare command configured but no command runner available")
	}

	wtPath, wtCleanup, err := p.gitClient.CreateDetachedWorktree(ctx, localPath, branchName, sha)
	if err != nil {
		return "", cleanup, fmt.Errorf("create detached worktree: %w", err)
	}
	cleanup = wtCleanup

	if err := p.cmdRunner.Run(ctx, wtPath, prepareCommand); err != nil {
		return "", cleanup, fmt.Errorf("run prepare command: %w", err)
	}
	if err := p.gitClient.ForceAddAll(ctx, wtPath); err != nil {
		return "", cleanup, fmt.Errorf("force-stage prepared changes: %w", err)
	}
	if err := p.gitClient.Commit(wtPath, "watchtower: prepare build"); err != nil {
		return "", cleanup, fmt.Errorf("commit prepared changes: %w", err)
	}

	if !skipPush {
		needsMain := true
		if _, err := p.repoManager.GetGitRef(ctx, repoSelfLink, "refs/heads/main"); err == nil {
			needsMain = false
		}
		if err := pushToLaunchpad(p.gitClient, wtPath, gitSSHURL, lpOwner, branchName, needsMain); err != nil {
			return "", cleanup, fmt.Errorf("push to LP: %w", err)
		}
	}

	return wtPath, cleanup, nil
}

// pushFromLocalPath replicates the pre-worktree branch-dance for the
// no-prepare-command path: create the temp branch on localPath, push via
// the same pushToLaunchpad helper, then restore origBranch and delete
// the temp branch. Preserves prior behaviour exactly.
func pushFromLocalPath(
	ctx context.Context,
	gitClient port.GitClient,
	repoManager port.RepoManager,
	localPath, gitSSHURL, repoSelfLink, lpOwner, branchName, sha string,
) error {
	origBranch, err := gitClient.CurrentBranch(localPath)
	if err != nil {
		return fmt.Errorf("get current branch: %w", err)
	}
	if err := gitClient.CreateBranch(localPath, branchName, sha); err != nil {
		return fmt.Errorf("create branch %s: %w", branchName, err)
	}
	if err := gitClient.CheckoutBranch(localPath, branchName); err != nil {
		return fmt.Errorf("checkout branch %s: %w", branchName, err)
	}
	defer func() {
		_ = gitClient.CheckoutBranch(localPath, origBranch)
		_ = gitClient.DeleteLocalBranch(localPath, branchName)
	}()
	needsMain := true
	if _, err := repoManager.GetGitRef(ctx, repoSelfLink, "refs/heads/main"); err == nil {
		needsMain = false
	}
	return pushToLaunchpad(gitClient, localPath, gitSSHURL, lpOwner, branchName, needsMain)
}

// PrepareListByPrefix resolves Launchpad owner/project state for local-build discovery by prefix.
func (p *LocalBuildPreparer) PrepareListByPrefix(
	ctx context.Context,
	req PreparedBuildListRequest,
	prefix string,
) (PreparedBuildListRequest, error) {
	if p.repoManager == nil {
		return req, app.ErrLaunchpadAuthRequired
	}

	lpOwner := req.Owner
	var err error
	if lpOwner == "" {
		lpOwner, err = p.repoManager.GetCurrentUser(ctx)
		if err != nil {
			return req, fmt.Errorf("get current LP user: %w", err)
		}
		req.Owner = lpOwner
	}

	lpProject, err := p.repoManager.GetOrCreateProject(ctx, lpOwner)
	if err != nil {
		return req, fmt.Errorf("get LP project: %w", err)
	}
	req.TargetRef = lpProject
	req.RecipePrefix = prefix

	return req, nil
}

// PrepareDownloadByPrefix resolves Launchpad owner/project state for local-build downloads.
func (p *LocalBuildPreparer) PrepareDownloadByPrefix(
	ctx context.Context,
	req PreparedBuildDownloadRequest,
	prefix string,
) (PreparedBuildDownloadRequest, error) {
	if p.repoManager == nil {
		return req, app.ErrLaunchpadAuthRequired
	}

	lpOwner := req.Owner
	var err error
	if lpOwner == "" {
		lpOwner, err = p.repoManager.GetCurrentUser(ctx)
		if err != nil {
			return req, fmt.Errorf("get current LP user: %w", err)
		}
	}
	req.Owner = lpOwner

	lpProject, err := p.repoManager.GetOrCreateProject(ctx, lpOwner)
	if err != nil {
		return req, fmt.Errorf("get LP project: %w", err)
	}
	req.TargetRef = lpProject
	req.RecipePrefix = prefix

	return req, nil
}

func pushToLaunchpad(gitClient port.GitClient, localPath, gitSSHURL, lpOwner, branchName string, pushMain bool) error {
	sshURL := strings.Replace(gitSSHURL, "git+ssh://", "ssh://", 1)
	if !strings.Contains(sshURL, "@") {
		sshURL = strings.Replace(sshURL, "ssh://", "ssh://"+lpOwner+"@", 1)
	}

	const remoteName = "watchtower-tmp"
	_ = gitClient.RemoveRemote(localPath, remoteName)
	if err := gitClient.AddRemote(localPath, remoteName, sshURL); err != nil {
		return fmt.Errorf("add remote: %w", err)
	}
	defer func() { _ = gitClient.RemoveRemote(localPath, remoteName) }()

	// LP requires a main branch to exist before it processes other refs.
	// Only push main when the repo has no main branch yet, to avoid
	// conflicting with concurrent CI runs.
	if pushMain {
		if err := gitClient.Push(localPath, remoteName, "refs/heads/"+branchName, "refs/heads/main", true); err != nil {
			return fmt.Errorf("push main: %w", err)
		}
	}
	if err := gitClient.Push(localPath, remoteName, "refs/heads/"+branchName, "refs/heads/"+branchName, true); err != nil {
		return fmt.Errorf("push branch %s: %w", branchName, err)
	}

	return nil
}
