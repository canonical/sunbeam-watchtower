// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// defaultConcurrency is the max number of parallel LP operations or artifact transfers.
const defaultConcurrency = 4

// RecipeAction is the action determined for a recipe after assessment.
type RecipeAction = dto.BuildRecipeAction

const (
	ActionCreateRecipe  RecipeAction = dto.BuildActionCreateRecipe
	ActionRequestBuilds RecipeAction = dto.BuildActionRequestBuilds
	ActionRetryFailed   RecipeAction = dto.BuildActionRetryFailed
	ActionMonitor       RecipeAction = dto.BuildActionMonitor
	ActionDownload      RecipeAction = dto.BuildActionDownload
	ActionNoop          RecipeAction = dto.BuildActionNoop
)

// RecipeStatus holds the assessed state of a single recipe.
type RecipeStatus struct {
	Name   string
	Action RecipeAction
	Recipe *dto.Recipe
	Builds []dto.Build
	Error  error
}

// TriggerOpts holds options for triggering builds.
type TriggerOpts struct {
	Wait       bool
	Timeout    time.Duration
	Owner      string // override project owner
	Prefix     string // temp recipe name prefix
	RetryCount int    // max attempts per build during Wait; <=1 means no retry

	TargetRef string // override backend target reference for recipe operations
	Prepared  *dto.PreparedBuildSource

	Channels      map[string]string
	Architectures []string
	// Snap-specific
	ArchiveLink string
	Pocket      string
}

// TriggerResult holds the result of a trigger operation.
type TriggerResult = dto.BuildTriggerResult

// RecipeResult holds the result of a single recipe action.
type RecipeResult = dto.BuildRecipeResult

// ListOpts holds options for listing builds.
type ListOpts struct {
	Projects     []string
	All          bool     // show all builds, not just active
	State        string   // filter by state
	Owner        string   // override project owner
	TargetRef    string   // override backend target reference for recipe lookup
	RecipeNames  []string // explicit recipe names (overrides project config)
	RecipePrefix string   // filter recipes by name prefix (used with ListRecipesByOwner)
}

// ProjectResult holds builds from one project, or an error.
type ProjectResult struct {
	ProjectName string
	Builds      []dto.Build
	Err         error
}

// CleanupOpts holds options for cleaning up temporary recipes.
type CleanupOpts struct {
	Projects  []string
	Owner     string
	Prefix    string
	DryRun    bool
	TargetRef string // LP project for branch cleanup resolution
}

// CleanupResult holds the result of a cleanup operation.
type CleanupResult struct {
	DeletedRecipes  []string
	DeletedBranches []string
}

// Service orchestrates builds across projects.
type Service struct {
	projects    map[string]ProjectBuilder // keyed by watchtower project name
	repoManager port.RepoManager
	logger      *slog.Logger

	// Timing controls for waitForBuilds. Zero values fall back to production
	// defaults via waitPollInterval / waitPostRetryDelay. Tests may override
	// these to short durations to avoid slow runs.
	pollInterval   time.Duration
	postRetryDelay time.Duration
	downloadClient *http.Client
}

const (
	defaultWaitPollInterval   = 60 * time.Second
	defaultWaitPostRetryDelay = 2 * time.Second
)

// NewService creates a build service with the given project-to-builder mappings.
func NewService(projects map[string]ProjectBuilder, repoManager port.RepoManager, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Service{
		projects:    projects,
		repoManager: repoManager,
		logger:      logger,
	}
}

// waitPollInterval returns the polling interval for waitForBuilds.
func (s *Service) waitPollInterval() time.Duration {
	if s.pollInterval > 0 {
		return s.pollInterval
	}
	return defaultWaitPollInterval
}

// waitPostRetryDelay returns the short settle delay issued after retries, so
// LP has a chance to transition build state before we re-poll.
func (s *Service) waitPostRetryDelay() time.Duration {
	if s.postRetryDelay > 0 {
		return s.postRetryDelay
	}
	return defaultWaitPostRetryDelay
}

func (s *Service) artifactDownloadClient() *http.Client {
	if s.downloadClient != nil {
		return s.downloadClient
	}
	return http.DefaultClient
}

// Trigger orchestrates the build pipeline for a project. It is re-entrant:
// calling it multiple times for the same recipes picks up where it left off.
//
// When opts.Prepared is provided (e.g. by CLI/TUI local preparation), the
// service uses those pre-resolved Launchpad references directly. Otherwise, it
// resolves repo and ref information from Launchpad (remote/official mode).
func (s *Service) Trigger(ctx context.Context, projectName string, artifactNames []string, opts TriggerOpts) (*TriggerResult, error) {
	pb, ok := s.projects[projectName]
	if !ok {
		return nil, fmt.Errorf("unknown project %q", projectName)
	}

	// Defense-in-depth: retry > 1 only has meaning when the service waits for
	// builds. API and CLI layers already reject this combo; log and proceed
	// as if RetryCount were unset so the legacy path is exercised.
	if opts.RetryCount > 1 && !opts.Wait {
		s.logger.Debug("ignoring RetryCount>1 because Wait=false", "retry_count", opts.RetryCount)
		opts.RetryCount = 1
	}

	owner := opts.Owner
	if owner == "" {
		owner = pb.Owner
	}
	if owner == "" {
		return nil, fmt.Errorf("no owner configured for project %q (set build.owner in config or use --owner)", projectName)
	}
	pb.Owner = owner

	// Merge project-level channels (defaults) with trigger opts (overrides).
	if len(pb.Channels) > 0 && len(opts.Channels) == 0 {
		opts.Channels = pb.Channels
	} else if len(pb.Channels) > 0 {
		merged := make(map[string]string, len(pb.Channels)+len(opts.Channels))
		for k, v := range pb.Channels {
			merged[k] = v
		}
		for k, v := range opts.Channels {
			merged[k] = v // opts override project defaults
		}
		opts.Channels = merged
	}

	prepared := opts.Prepared.Normalize()

	targetRef := opts.TargetRef
	if prepared != nil && prepared.TargetRef != "" {
		targetRef = prepared.TargetRef
	}
	if targetRef != "" {
		pb.LPProject = targetRef
	}

	recipes := artifactNames
	if len(recipes) == 0 {
		recipes = pb.Artifacts
	}
	if len(recipes) == 0 {
		return nil, fmt.Errorf("no artifacts specified for project %q", projectName)
	}

	// Filter out skipped artifacts.
	if len(pb.SkipArtifacts) > 0 {
		skip := make(map[string]bool, len(pb.SkipArtifacts))
		for _, s := range pb.SkipArtifacts {
			skip[s] = true
		}
		filtered := recipes[:0]
		for _, name := range recipes {
			if !skip[name] {
				filtered = append(filtered, name)
			}
		}
		recipes = filtered
	}

	// Resolve LP repo and ref information.
	repoSelfLink := ""
	var gitRefLinks map[string]string
	var buildPaths map[string]string
	var processorsByRecipe map[string][]string
	if prepared != nil {
		repoSelfLink = prepared.RepositoryRef
		if len(prepared.Recipes) > 0 {
			gitRefLinks = make(map[string]string, len(prepared.Recipes))
			buildPaths = make(map[string]string, len(prepared.Recipes))
			processorsByRecipe = make(map[string][]string, len(prepared.Recipes))
			for recipeName, recipe := range prepared.Recipes {
				gitRefLinks[recipeName] = recipe.SourceRef
				buildPaths[recipeName] = recipe.BuildPath
				if len(recipe.Processors) > 0 {
					processorsByRecipe[recipeName] = recipe.Processors
				}
			}
		}
	}
	if gitRefLinks == nil {
		gitRefLinks = make(map[string]string)
	}
	if buildPaths == nil {
		buildPaths = make(map[string]string)
	}
	if processorsByRecipe == nil {
		processorsByRecipe = make(map[string][]string)
	}

	// If caller didn't provide pre-resolved values and project uses official
	// codehosting, resolve repo and refs from Launchpad.
	if repoSelfLink == "" && pb.OfficialCodehosting {
		if s.repoManager == nil {
			return nil, fmt.Errorf("official codehosting requires a RepoManager")
		}

		lpProject := pb.RecipeProject()
		repoLink, defaultBranch, err := s.repoManager.GetDefaultRepo(ctx, lpProject)
		if err != nil {
			return nil, fmt.Errorf("get default repo for %q: %w", lpProject, err)
		}
		repoSelfLink = repoLink

		// If series are configured, expand artifacts into series-based recipes.
		if len(pb.Series) > 0 && len(pb.DevFocus) > 0 {
			var expandedRecipes []string
			for _, artifactName := range recipes {
				for _, series := range pb.Series {
					recipeName := pb.Strategy.OfficialRecipeName(artifactName, series, pb.DevFocus)
					branch := pb.Strategy.BranchForSeries(series, pb.DevFocus, defaultBranch)
					refPath := "refs/heads/" + branch

					refLink, err := s.repoManager.GetGitRef(ctx, repoSelfLink, refPath)
					if err != nil {
						s.logger.Warn("branch not found, skipping", "branch", branch, "series", series, "error", err)
						continue
					}
					gitRefLinks[recipeName] = refLink
					buildPaths[recipeName] = pb.Strategy.BuildPath(artifactName)
					expandedRecipes = append(expandedRecipes, recipeName)
				}
			}
			recipes = expandedRecipes
		} else {
			// No series: use default branch for all recipes.
			refPath := "refs/heads/" + defaultBranch
			refLink, err := s.repoManager.GetGitRef(ctx, repoSelfLink, refPath)
			if err != nil {
				return nil, fmt.Errorf("get git ref %q: %w", refPath, err)
			}
			for _, name := range recipes {
				gitRefLinks[name] = refLink
				buildPaths[name] = pb.Strategy.BuildPath(name)
			}
		}
	}

	result := &TriggerResult{Project: projectName}

	// Assess and execute recipe actions concurrently.
	type triggerJob struct {
		index int
		name  string
	}
	type triggerResult struct {
		index  int
		result RecipeResult
	}

	workerCount := min(defaultConcurrency, len(recipes))
	jobs := make(chan triggerJob)
	results := make(chan triggerResult, len(recipes))

	var wg sync.WaitGroup
	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				if ctx.Err() != nil {
					return
				}
				status := s.assessRecipe(ctx, pb, job.name)
				refLink := gitRefLinks[job.name]
				bp := buildPaths[job.name]
				procs := processorsByRecipe[job.name]
				s.logger.Debug("dispatching recipe action",
					"recipe", job.name, "action", status.Action,
					"repoSelfLink", repoSelfLink, "gitRefLink", refLink, "buildPath", bp)
				rr := s.executeAction(ctx, pb, status, opts, repoSelfLink, refLink, bp, procs)
				results <- triggerResult{index: job.index, result: rr}
			}
		}()
	}

	for i, name := range recipes {
		if ctx.Err() != nil {
			break
		}
		jobs <- triggerJob{index: i, name: name}
	}
	close(jobs)
	wg.Wait()
	close(results)

	// Reassemble results in original order.
	ordered := make([]RecipeResult, len(recipes))
	for tr := range results {
		ordered[tr.index] = tr.result
	}

	var recipePtrs []*dto.Recipe
	for _, rr := range ordered {
		if rr.Name == "" {
			continue // skipped due to context cancellation
		}
		result.RecipeResults = append(result.RecipeResults, rr)
		if rr.Error == nil && rr.Recipe != nil {
			recipePtrs = append(recipePtrs, rr.Recipe)
		}
	}

	if opts.Wait && len(recipePtrs) > 0 {
		timeout := opts.Timeout
		if timeout == 0 {
			timeout = 30 * time.Minute
		}
		builds, err := s.waitForBuilds(ctx, pb, recipePtrs, timeout, opts.RetryCount)
		if err != nil {
			s.logger.Warn("wait for builds completed with error", "error", err)
			var timeoutErr *BuildWaitTimeoutError
			if errors.As(err, &timeoutErr) {
				result.WaitTimeout = buildWaitTimeoutDTO(timeoutErr)
			}
		}
		// Replace results with final build states from the wait loop.
		for i := range result.RecipeResults {
			rr := &result.RecipeResults[i]
			var recipeBuilds []dto.Build
			for _, b := range builds {
				if b.Recipe == rr.Name {
					recipeBuilds = append(recipeBuilds, b)
				}
			}
			if len(recipeBuilds) > 0 {
				rr.Builds = recipeBuilds
			}
		}
	}

	return result, nil
}

func (s *Service) assessRecipe(ctx context.Context, pb ProjectBuilder, recipeName string) RecipeStatus {
	recipe, err := pb.Builder.GetRecipe(ctx, pb.Owner, pb.RecipeProject(), recipeName)
	if err != nil {
		return RecipeStatus{Name: recipeName, Action: ActionCreateRecipe}
	}

	builds, err := pb.Builder.ListBuilds(ctx, recipe)
	if err != nil {
		return RecipeStatus{Name: recipeName, Action: ActionRequestBuilds, Recipe: recipe}
	}

	if len(builds) == 0 {
		return RecipeStatus{Name: recipeName, Action: ActionRequestBuilds, Recipe: recipe}
	}

	allSucceeded := true
	hasActive := false
	hasFailed := false
	for _, b := range builds {
		if !b.State.IsTerminal() {
			hasActive = true
			allSucceeded = false
		} else if b.State.IsFailure() {
			hasFailed = true
			allSucceeded = false
		} else if b.State != dto.BuildSucceeded {
			allSucceeded = false
		}
	}

	if allSucceeded {
		return RecipeStatus{Name: recipeName, Action: ActionDownload, Recipe: recipe, Builds: builds}
	}
	if hasActive {
		return RecipeStatus{Name: recipeName, Action: ActionMonitor, Recipe: recipe, Builds: builds}
	}
	if hasFailed {
		return RecipeStatus{Name: recipeName, Action: ActionRetryFailed, Recipe: recipe, Builds: builds}
	}
	return RecipeStatus{Name: recipeName, Action: ActionRequestBuilds, Recipe: recipe, Builds: builds}
}

func (s *Service) executeAction(ctx context.Context, pb ProjectBuilder, status RecipeStatus, opts TriggerOpts, repoSelfLink, gitRefLink, buildPath string, processors []string) RecipeResult {
	result := RecipeResult{Name: status.Name, Action: status.Action, Recipe: status.Recipe}

	setErr := func(err error) {
		result.Error = err
		result.ErrorMessage = err.Error()
		s.logger.Error("recipe action failed", "recipe", status.Name, "action", status.Action, "error", err)
	}

	switch status.Action {
	case ActionCreateRecipe:
		if repoSelfLink == "" || gitRefLink == "" {
			setErr(fmt.Errorf("recipe %q not found (create requires git repo info; use local mode or enable official_codehosting)", status.Name))
			return result
		}
		bp := buildPath
		s.logger.Info("creating recipe", "recipe", status.Name, "owner", pb.Owner, "project", pb.RecipeProject(), "buildPath", bp)
		createOpts := dto.CreateRecipeOpts{
			Name:        status.Name,
			Owner:       pb.Owner,
			Project:     pb.RecipeProject(),
			GitRepoLink: repoSelfLink,
			GitRefLink:  gitRefLink,
			BuildPath:   bp,
			Channels:    opts.Channels,
			Processors:  processors,
		}
		// LP has a propagation delay between its git indexer and recipe
		// service. Retry on "No such object" errors for the git_ref.
		var recipe *dto.Recipe
		var err error
		for attempt := range 5 {
			recipe, err = pb.Builder.CreateRecipe(ctx, createOpts)
			if err == nil || !strings.Contains(err.Error(), "No such object") {
				break
			}
			s.logger.Warn("LP ref not yet propagated, retrying", "recipe", status.Name, "attempt", attempt+1, "error", err)
			select {
			case <-ctx.Done():
				setErr(ctx.Err())
				return result
			case <-time.After(30 * time.Second):
			}
		}
		if err != nil {
			setErr(fmt.Errorf("create recipe %q: %w", status.Name, err))
			return result
		}
		result.Recipe = recipe
		br, err := pb.Builder.RequestBuilds(ctx, recipe, buildOpts(opts))
		result.BuildRequest = br
		if err != nil {
			setErr(fmt.Errorf("request builds for %q: %w", status.Name, err))
			return result
		}
		// LP processes requestBuilds asynchronously — builds may not
		// appear immediately. Poll until at least one build exists.
		result.Builds = s.waitForBuildRecords(ctx, pb, recipe)

	case ActionRequestBuilds:
		// Sync recipe-level processors (snap-only — no-op for rock/charm)
		// before requesting builds, so a snapcraft.yaml platforms change
		// takes effect on the next trigger.
		if len(processors) > 0 {
			if err := pb.Builder.SetProcessors(ctx, status.Recipe, processors); err != nil {
				setErr(fmt.Errorf("sync processors for %q: %w", status.Name, err))
				return result
			}
		}
		br, err := pb.Builder.RequestBuilds(ctx, status.Recipe, buildOpts(opts))
		result.BuildRequest = br
		if err != nil {
			setErr(fmt.Errorf("request builds for %q: %w", status.Name, err))
			return result
		}
		result.Builds = s.waitForBuildRecords(ctx, pb, status.Recipe)

	case ActionRetryFailed:
		// When the caller requested a retry budget (RetryCount > 1), defer
		// all retry decisions to waitForBuilds so there is a single retry
		// owner. waitForBuilds retries each eligible build per-arch as soon as
		// it observes it, without waiting for sibling recipes or
		// architectures. Otherwise preserve the legacy one-shot retry
		// behavior for callers that don't set RetryCount.
		if opts.RetryCount > 1 {
			result.Action = ActionMonitor
			result.Builds = status.Builds
			break
		}
		for _, b := range status.Builds {
			if b.State.IsFailure() && b.CanRetry {
				if err := pb.Builder.RetryBuild(ctx, b.SelfLink); err != nil {
					s.logger.Warn("failed to retry build", "build", b.SelfLink, "error", err)
				}
			}
		}
		result.Action = ActionMonitor
		result.Builds = status.Builds

	case ActionMonitor:
		result.Builds = status.Builds

	case ActionDownload:
		result.Builds = status.Builds

	case ActionNoop:
		// nothing to do
	}

	return result
}

// waitForBuildRecords polls ListBuilds until at least one build record
// appears, or gives up after 2 minutes. LP creates build records
// asynchronously after requestBuilds returns.
func (s *Service) waitForBuildRecords(ctx context.Context, pb ProjectBuilder, recipe *dto.Recipe) []dto.Build {
	wait := 2 * time.Second
	maxWait := 15 * time.Second
	deadline := time.Now().Add(2 * time.Minute)

	for {
		builds, err := pb.Builder.ListBuilds(ctx, recipe)
		if err == nil && len(builds) > 0 {
			return builds
		}

		if time.Now().After(deadline) {
			s.logger.Warn("timed out waiting for build records", "recipe", recipe.Name)
			return nil
		}

		s.logger.Debug("waiting for build records to appear", "recipe", recipe.Name, "retry_in", wait)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(wait):
		}
		wait *= 2
		if wait > maxWait {
			wait = maxWait
		}
	}
}

func buildOpts(opts TriggerOpts) dto.RequestBuildsOpts {
	return dto.RequestBuildsOpts{
		Channels:      opts.Channels,
		Architectures: opts.Architectures,
		ArchiveLink:   opts.ArchiveLink,
		Pocket:        opts.Pocket,
	}
}

// waitForBuilds polls Launchpad for the given recipes' builds until every
// observed build is terminal and no accepted retry is still awaiting a
// transition. Failed-but-retryable builds are retried during the same polling
// pass that observes them, up to retryCount-1 times per b.SelfLink, so retries
// overlap sibling recipes and architectures that are still building.
// retryCount <= 1 disables retries (legacy behavior).
func (s *Service) waitForBuilds(
	ctx context.Context,
	pb ProjectBuilder,
	recipes []*dto.Recipe,
	timeout time.Duration,
	retryCount int,
) ([]dto.Build, error) {
	deadline := time.Now().Add(timeout)
	pollInterval := s.waitPollInterval()

	observer := &waitBuildObserver{
		initBudget: max(retryCount-1, 0),
		trackers:   make(map[string]*buildRetryTracker),
		lastGood:   make(map[string]dto.Build),
	}

	// listRecipe reads one recipe and feeds successful observations into the
	// lifecycle tracker. With schedule=false it is a read-only refresh: it may
	// acknowledge an outstanding retry but never issues a new retry POST.
	// complete is false for a listing error or an empty build list; both mean
	// the pass saw an incomplete view and must not count as finished.
	listRecipe := func(recipe *dto.Recipe, schedule bool) (complete, allTerminal, retried bool) {
		complete, allTerminal = true, true
		builds, err := pb.Builder.ListBuilds(ctx, recipe)
		if err != nil {
			s.logger.Warn("error listing builds", "recipe", recipe.Name, "error", err)
			return false, false, false
		}
		if len(builds) == 0 {
			s.logger.Warn("recipe returned no builds while waiting", "recipe", recipe.Name)
			return false, false, false
		}
		for _, b := range builds {
			observer.observe(b)
			if !b.State.IsTerminal() {
				allTerminal = false
			}
		}
		if schedule {
			for _, b := range builds {
				if s.scheduleRetry(ctx, pb, b, observer, retryCount, deadline) {
					retried = true
				}
			}
		}
		return complete, allTerminal, retried
	}

	// refresh takes a read-only snapshot across all recipes, acknowledging any
	// retry that has visibly transitioned. It deliberately attempts reads even
	// after cancellation so the freshest obtainable state is returned; failed
	// reads retain the last successful observation per self-link.
	refresh := func() {
		for _, recipe := range recipes {
			listRecipe(recipe, false)
		}
	}

	poll := func() (complete, allTerminal, retried bool) {
		complete, allTerminal = true, true
		for _, recipe := range recipes {
			if ctx.Err() != nil {
				return false, false, retried
			}
			c, t, r := listRecipe(recipe, true)
			if !c {
				complete = false
			}
			if !t {
				allTerminal = false
			}
			if r {
				retried = true
			}
		}
		return complete, allTerminal, retried
	}

	for {
		if err := ctx.Err(); err != nil {
			refresh()
			return observer.snapshot(), err
		}

		complete, allTerminal, retried := poll()

		if retried {
			// Give LP a brief moment to transition the retried builds, then
			// take a read-only refresh so cancellation/timeout paths return the
			// freshest obtainable state without issuing further retries.
			select {
			case <-ctx.Done():
				refresh()
				return observer.snapshot(), ctx.Err()
			case <-time.After(s.waitPostRetryDelay()):
			}
			refresh()
		}

		if complete && allTerminal && !retried && !observer.anyAwaiting() {
			return observer.snapshot(), nil
		}

		if !time.Now().Before(deadline) {
			return observer.snapshot(), &BuildWaitTimeoutError{
				Timeout: timeout,
				Builds:  observer.outstanding(),
			}
		}

		select {
		case <-ctx.Done():
			refresh()
			return observer.snapshot(), ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

// buildRetryTracker is the request-local retry lifecycle for one build
// self-link during a wait.
type buildRetryTracker struct {
	remaining int       // retries still available; 0 means the budget is spent
	awaiting  bool      // a retry POST succeeded and its transition is unacknowledged
	baseline  dto.Build // observation saved before the accepted retry POST
}

// waitBuildObserver accumulates retry lifecycle state and the latest successful
// observation for each build self-link.
type waitBuildObserver struct {
	initBudget int
	trackers   map[string]*buildRetryTracker
	lastGood   map[string]dto.Build
	order      []string // self-links in first-seen order, for deterministic snapshots
}

// tracker returns the lifecycle state for a self-link, seeding a fresh budget on
// first observation.
func (o *waitBuildObserver) tracker(selfLink string) *buildRetryTracker {
	t, ok := o.trackers[selfLink]
	if !ok {
		t = &buildRetryTracker{remaining: o.initBudget}
		o.trackers[selfLink] = t
	}
	return t
}

// observe records a successful observation of b, advancing the retry lifecycle
// and updating the best-effort snapshot. Builds without a self-link are
// ignored so unrelated malformed records cannot share a retry budget.
func (o *waitBuildObserver) observe(b dto.Build) {
	if b.SelfLink == "" {
		return
	}
	if _, seen := o.lastGood[b.SelfLink]; !seen {
		o.order = append(o.order, b.SelfLink)
	}
	o.lastGood[b.SelfLink] = b

	t := o.tracker(b.SelfLink)
	if !t.awaiting {
		return
	}
	switch {
	case b.State.IsActive():
		// A visible active state acknowledges the retry.
		t.awaiting = false
	case b.State == dto.BuildSucceeded, b.State == dto.BuildSuperseded:
		// A terminal non-failure also completes the outstanding retry.
		t.awaiting = false
	case b.State.IsFailure():
		// Without an intervening active observation, a failure whose attempt
		// timestamps advanced past the pre-retry snapshot is a new completed
		// attempt (e.g. a retry that ran and failed entirely between polls).
		if retryAttemptAdvanced(b, t.baseline) {
			t.awaiting = false
		}
	}
}

// anyAwaiting reports whether any accepted retry is still awaiting a visible
// transition.
func (o *waitBuildObserver) anyAwaiting() bool {
	for _, t := range o.trackers {
		if t.awaiting {
			return true
		}
	}
	return false
}

// snapshot returns the latest successful observation per build, in first-seen
// order.
func (o *waitBuildObserver) snapshot() []dto.Build {
	out := make([]dto.Build, 0, len(o.order))
	for _, link := range o.order {
		if b, ok := o.lastGood[link]; ok {
			out = append(out, b)
		}
	}
	return out
}

// outstanding returns the builds still unresolved at timeout: active builds
// plus builds with an accepted retry that never visibly transitioned. A stale
// terminal snapshot from an accepted retry is reported with its observed state
// rather than dropped.
func (o *waitBuildObserver) outstanding() []dto.Build {
	out := make([]dto.Build, 0, len(o.order))
	for _, link := range o.order {
		b, ok := o.lastGood[link]
		if !ok {
			continue
		}
		t := o.trackers[link]
		if b.State.IsActive() || (t != nil && t.awaiting) {
			out = append(out, b)
		}
	}
	return out
}

// scheduleRetry issues a retry POST for one observed build when it is an
// eligible failure with remaining budget. It returns whether a POST succeeded.
func (s *Service) scheduleRetry(
	ctx context.Context,
	pb ProjectBuilder,
	b dto.Build,
	observer *waitBuildObserver,
	retryCount int,
	deadline time.Time,
) bool {
	if b.SelfLink == "" || !b.State.IsFailure() || !b.CanRetry {
		return false
	}
	t := observer.tracker(b.SelfLink)
	if t.remaining <= 0 || t.awaiting {
		return false
	}
	// Check cancellation and expiry before starting a mutation.
	if ctx.Err() != nil || !time.Now().Before(deadline) {
		return false
	}

	attemptIndex := retryCount - t.remaining + 1
	s.logger.Info("retrying failed build",
		"recipe", b.Recipe,
		"build", b.SelfLink,
		"arch", b.Arch,
		"attempt", attemptIndex,
		"max_attempts", retryCount,
	)
	if err := pb.Builder.RetryBuild(ctx, b.SelfLink); err != nil {
		s.logger.Warn("retry call failed; giving up on this build",
			"build", b.SelfLink, "error", err)
		t.remaining = 0
		return false
	}
	t.remaining--
	t.awaiting = true
	t.baseline = b
	return true
}

// retryAttemptAdvanced reports whether a failed observation is distinguishable
// from the pre-retry baseline because a nonzero attempt timestamp moved
// forward. A timestamp becoming zero is not evidence of a new attempt.
func retryAttemptAdvanced(observed, baseline dto.Build) bool {
	if !observed.StartedAt.IsZero() && observed.StartedAt.After(baseline.StartedAt) {
		return true
	}
	if !observed.BuiltAt.IsZero() && observed.BuiltAt.After(baseline.BuiltAt) {
		return true
	}
	return false
}

// BuildWaitTimeoutError reports the active build snapshot captured when a
// wait deadline expires.
type BuildWaitTimeoutError struct {
	Timeout time.Duration
	Builds  []dto.Build
}

func (e *BuildWaitTimeoutError) Error() string {
	if e == nil {
		return ""
	}
	return buildWaitTimeoutMessage(buildWaitTimeoutDTO(e))
}

func buildWaitTimeoutDTO(err *BuildWaitTimeoutError) *dto.BuildWaitTimeout {
	out := &dto.BuildWaitTimeout{Timeout: err.Timeout.String()}
	for _, b := range err.Builds {
		out.Builds = append(out.Builds, dto.BuildWaitTimeoutBuild{
			Project:  b.Project,
			Recipe:   b.Recipe,
			Arch:     b.Arch,
			State:    b.State.String(),
			URL:      b.WebLink,
			SelfLink: b.SelfLink,
		})
	}
	return out
}

func buildWaitTimeoutMessage(timeout *dto.BuildWaitTimeout) string {
	if timeout == nil {
		return ""
	}
	if len(timeout.Builds) == 0 {
		return fmt.Sprintf("timeout waiting for builds after %s", timeout.Timeout)
	}
	details := make([]string, 0, len(timeout.Builds))
	for _, b := range timeout.Builds {
		details = append(details, fmt.Sprintf("recipe=%s arch=%s state=%s url=%s", b.Recipe, b.Arch, b.State, b.URL))
	}
	return fmt.Sprintf("timeout waiting for builds after %s: %s", timeout.Timeout, strings.Join(details, "; "))
}

// List returns builds across configured projects, applying filters.
// Per-project errors are collected but do not stop aggregation (graceful degradation).
//
// When opts.Owner is set, it overrides the project's configured owner.
// When opts.RecipeNames is set, it overrides the project's configured recipe list.
func (s *Service) List(ctx context.Context, opts ListOpts) ([]dto.Build, []ProjectResult, error) {
	projFilter := make(map[string]bool, len(opts.Projects))
	for _, p := range opts.Projects {
		projFilter[p] = true
	}

	var results []ProjectResult
	var all []dto.Build

	for name, pb := range s.projects {
		if len(projFilter) > 0 && !projFilter[name] {
			continue
		}

		result := ProjectResult{ProjectName: name}
		var projBuilds []dto.Build

		owner := pb.Owner
		if opts.Owner != "" {
			owner = opts.Owner
		}

		targetRef := pb.RecipeProject()
		if opts.TargetRef != "" {
			targetRef = opts.TargetRef
		}

		recipeNames := pb.Artifacts
		if len(opts.RecipeNames) > 0 {
			recipeNames = opts.RecipeNames
		}

		// When a prefix is given without explicit recipe names, discover
		// recipes from LP and filter by prefix + LP project.
		if opts.RecipePrefix != "" && len(opts.RecipeNames) == 0 {
			if owner == "" {
				s.logger.Warn("skipping prefix discovery: owner required", "project", name)
				continue
			}
			allRecipes, err := pb.Builder.ListRecipesByOwner(ctx, owner)
			if err != nil {
				s.logger.Warn("error listing recipes by owner", "project", name, "error", err)
				result.Err = err
				results = append(results, result)
				continue
			}
			recipeNames = nil
			for _, r := range allRecipes {
				if !strings.HasPrefix(r.Name, opts.RecipePrefix) {
					continue
				}
				if targetRef != "" && r.Project != targetRef {
					continue
				}
				recipeNames = append(recipeNames, r.Name)
			}
		}

		for _, recipeName := range recipeNames {
			recipe, err := pb.Builder.GetRecipe(ctx, owner, targetRef, recipeName)
			if err != nil {
				s.logger.Warn("error getting recipe", "project", name, "recipe", recipeName, "error", err)
				continue
			}

			builds, err := pb.Builder.ListBuilds(ctx, recipe)
			if err != nil {
				s.logger.Warn("error listing builds", "project", name, "recipe", recipeName, "error", err)
				continue
			}

			for i := range builds {
				builds[i].Project = name
			}
			projBuilds = append(projBuilds, builds...)
		}

		// Apply state filter.
		if opts.State != "" {
			filtered := projBuilds[:0]
			for _, b := range projBuilds {
				if strings.EqualFold(b.State.String(), opts.State) {
					filtered = append(filtered, b)
				}
			}
			projBuilds = filtered
		}

		// If not showing all, only return active builds.
		if !opts.All {
			filtered := projBuilds[:0]
			for _, b := range projBuilds {
				if b.State.IsActive() {
					filtered = append(filtered, b)
				}
			}
			projBuilds = filtered
		}

		result.Builds = projBuilds
		results = append(results, result)
		all = append(all, projBuilds...)
	}

	// Sort by CreatedAt descending.
	sort.Slice(all, func(i, j int) bool {
		return all[i].CreatedAt.After(all[j].CreatedAt)
	})

	return all, results, nil
}

// DownloadOpts holds options for downloading build artifacts.
type DownloadOpts struct {
	Projects      []string // project name filter
	ArtifactNames []string // explicit artifact names (maps to recipe names)
	RecipePrefix  string   // discover recipes by prefix
	Owner         string   // override LP owner
	TargetRef     string   // override backend target reference
	OutputDir     string   // output directory
	RetryCount    int      // max attempts per artifact file; <=1 means no retry
}

// Download retrieves build artifacts for succeeded builds of the given recipes.
func (s *Service) Download(ctx context.Context, opts DownloadOpts) error {
	projFilter := make(map[string]bool, len(opts.Projects))
	for _, p := range opts.Projects {
		projFilter[p] = true
	}
	var downloads []*DownloadFileError
	var wg sync.WaitGroup
	defer wg.Wait() // Discovery errors must also wait for started transfers.
	slots := make(chan struct{}, defaultConcurrency)
	// Keep writes to the same destination in discovery order.
	destinations := make(map[string]chan struct{})
	client := s.artifactDownloadClient()

	for name, pb := range s.projects {
		if len(projFilter) > 0 && !projFilter[name] {
			continue
		}

		owner := pb.Owner
		if opts.Owner != "" {
			owner = opts.Owner
		}

		targetRef := pb.RecipeProject()
		if opts.TargetRef != "" {
			targetRef = opts.TargetRef
		}

		recipeNames := opts.ArtifactNames
		if len(recipeNames) == 0 && opts.RecipePrefix == "" {
			recipeNames = pb.Artifacts
		}

		// Prefix-based discovery.
		if opts.RecipePrefix != "" && len(opts.ArtifactNames) == 0 {
			if owner == "" {
				s.logger.Warn("skipping prefix discovery: owner required", "project", name)
				continue
			}
			allRecipes, err := pb.Builder.ListRecipesByOwner(ctx, owner)
			if err != nil {
				return fmt.Errorf("listing recipes by owner for %q: %w", name, err)
			}
			recipeNames = nil
			for _, r := range allRecipes {
				if !strings.HasPrefix(r.Name, opts.RecipePrefix) {
					continue
				}
				if targetRef != "" && r.Project != targetRef {
					continue
				}
				recipeNames = append(recipeNames, r.Name)
			}
		}

		for _, recipeName := range recipeNames {
			recipe, err := pb.Builder.GetRecipe(ctx, owner, targetRef, recipeName)
			if err != nil {
				return fmt.Errorf("recipe %q: %w", recipeName, err)
			}
			builds, err := pb.Builder.ListBuilds(ctx, recipe)
			if err != nil {
				return fmt.Errorf("listing builds for %q: %w", recipeName, err)
			}

			for _, b := range builds {
				if b.State != dto.BuildSucceeded {
					continue
				}
				urls, err := pb.Builder.GetBuildFileURLs(ctx, b.SelfLink)
				if err != nil {
					s.logger.Warn("failed to get file URLs", "build", b.SelfLink, "error", err)
					continue
				}
				for _, u := range urls {
					dest := filepath.Join(opts.OutputDir, recipeName, path.Base(u))
					if previous := destinations[dest]; previous != nil {
						<-previous
					}
					done := make(chan struct{})
					destinations[dest] = done
					file := &DownloadFileError{
						Project:       name,
						Recipe:        recipeName,
						Arch:          b.Arch,
						BuildURL:      b.WebLink,
						BuildSelfLink: b.SelfLink,
						FileURL:       u,
					}
					downloads = append(downloads, file)
					slots <- struct{}{}
					wg.Add(1)
					go func() {
						defer wg.Done()
						defer func() { <-slots; close(done) }()
						file.Err = downloadFile(ctx, client, file.FileURL, opts.OutputDir, file.Recipe, opts.RetryCount)
					}()
				}
			}
		}
	}
	wg.Wait()
	var failures []DownloadFileError
	for _, file := range downloads {
		if file.Err != nil {
			failures = append(failures, *file)
		}
	}
	if len(failures) > 0 {
		return &DownloadError{Failures: failures}
	}
	return nil
}

// DownloadFileError describes one artifact file that could not be fetched.
type DownloadFileError struct {
	Project       string
	Recipe        string
	Arch          string
	BuildURL      string
	BuildSelfLink string
	FileURL       string
	Err           error
}

func (f DownloadFileError) Error() string {
	return fmt.Sprintf(
		"project=%s recipe=%s arch=%s build_url=%s build_self_link=%s file_url=%s: %v",
		f.Project,
		f.Recipe,
		f.Arch,
		f.BuildURL,
		f.BuildSelfLink,
		f.FileURL,
		f.Err,
	)
}

func (f DownloadFileError) Unwrap() error {
	return f.Err
}

// DownloadError aggregates artifact file download failures after all eligible
// downloads have been attempted.
type DownloadError struct {
	Failures []DownloadFileError
}

func (e *DownloadError) Error() string {
	if e == nil || len(e.Failures) == 0 {
		return ""
	}
	parts := make([]string, 0, len(e.Failures))
	for _, failure := range e.Failures {
		parts = append(parts, failure.Error())
	}
	return fmt.Sprintf("download failed for %d artifact file(s): %s", len(e.Failures), strings.Join(parts, "; "))
}

// downloadFile downloads a file from fileURL into outputDir/artifactName/,
// with path traversal protection and retries for transient transfer failures.
func downloadFile(ctx context.Context, client *http.Client, fileURL, outputDir, artifactName string, attempts int) error {
	if attempts < 1 {
		attempts = 1
	}
	if client == nil {
		client = http.DefaultClient
	}

	// Extract filename from URL (last path segment).
	filename := path.Base(fileURL)
	if filename == "" || filename == "." || filename == "/" {
		return fmt.Errorf("cannot determine filename from URL %q", fileURL)
	}
	if strings.Contains(filename, "..") {
		return fmt.Errorf("invalid filename %q: contains path traversal", filename)
	}

	destDir := filepath.Join(outputDir, artifactName)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	destPath := filepath.Join(destDir, filename)
	// Verify resolved path is within outputDir.
	absOutput, err := filepath.Abs(outputDir)
	if err != nil {
		return fmt.Errorf("resolving output dir: %w", err)
	}
	absDest, err := filepath.Abs(destPath)
	if err != nil {
		return fmt.Errorf("resolving dest path: %w", err)
	}
	if !strings.HasPrefix(absDest, absOutput+string(filepath.Separator)) {
		return fmt.Errorf("path traversal detected: %q is outside %q", absDest, absOutput)
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := downloadFileOnce(ctx, client, fileURL, destDir, destPath)
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt == attempts || !isTransientDownloadError(err) {
			break
		}
	}

	return lastErr
}

type downloadStatusError struct {
	url        string
	statusCode int
}

func (e *downloadStatusError) Error() string {
	return fmt.Sprintf("HTTP GET %q: status %d", e.url, e.statusCode)
}

type transientDownloadError struct {
	err error
}

func (e *transientDownloadError) Error() string {
	return e.err.Error()
}

func (e *transientDownloadError) Unwrap() error {
	return e.err
}

func downloadFileOnce(ctx context.Context, client *http.Client, fileURL, destDir, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return fmt.Errorf("creating HTTP GET %q: %w", fileURL, err)
	}

	resp, err := client.Do(req) //nolint:gosec // URL comes from LP API
	if err != nil {
		return &transientDownloadError{err: fmt.Errorf("HTTP GET %q: %w", fileURL, err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &downloadStatusError{url: fileURL, statusCode: resp.StatusCode}
	}

	tmp, err := createDownloadTempFile(destDir, filepath.Base(destPath))
	if err != nil {
		return fmt.Errorf("creating temp file for %q: %w", destPath, err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		_ = tmp.Close()
		return &transientDownloadError{err: fmt.Errorf("writing file %q: %w", destPath, err)}
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing file %q: %w", destPath, err)
	}
	if err := os.Rename(tmpPath, destPath); err != nil { //nolint:gosec // destPath is validated to stay under outputDir before download.
		return fmt.Errorf("renaming file %q: %w", destPath, err)
	}

	return nil
}

func createDownloadTempFile(destDir, filename string) (*os.File, error) {
	prefix := "." + filename + "."
	for i := range 100 {
		tmpPath := filepath.Join(destDir, fmt.Sprintf("%s%d-%d.tmp", prefix, time.Now().UnixNano(), i))
		f, err := os.OpenFile(tmpPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o666) //nolint:gosec // artifacts intentionally use normal umask-controlled permissions.
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("could not create unique temp file in %q", destDir)
}

func isTransientDownloadError(err error) bool {
	var statusErr *downloadStatusError
	if errors.As(err, &statusErr) {
		return statusErr.statusCode == http.StatusTooManyRequests || statusErr.statusCode >= http.StatusInternalServerError
	}
	var transferErr *transientDownloadError
	return errors.As(err, &transferErr)
}

// Cleanup removes temporary recipes matching the given prefix and cleans up
// temporary branches. It uses ListRecipesByOwner to discover recipes by prefix
// rather than iterating the configured artifact list.
func (s *Service) Cleanup(ctx context.Context, opts CleanupOpts) (*CleanupResult, error) {
	projFilter := make(map[string]bool, len(opts.Projects))
	for _, p := range opts.Projects {
		projFilter[p] = true
	}

	owner := opts.Owner
	result := &CleanupResult{}

	for name, pb := range s.projects {
		if len(projFilter) > 0 && !projFilter[name] {
			continue
		}

		projOwner := pb.Owner
		if owner != "" {
			projOwner = owner
		}

		if projOwner == "" {
			s.logger.Warn("skipping cleanup: owner required", "project", name)
			continue
		}

		targetRef := pb.RecipeProject()
		if opts.TargetRef != "" {
			targetRef = opts.TargetRef
		}

		// Discover recipes by owner and filter by prefix.
		allRecipes, err := pb.Builder.ListRecipesByOwner(ctx, projOwner)
		if err != nil {
			s.logger.Warn("error listing recipes by owner", "project", name, "error", err)
			continue
		}

		// Filter matching recipes.
		var toDelete []*dto.Recipe
		for _, recipe := range allRecipes {
			if opts.Prefix != "" && !strings.HasPrefix(recipe.Name, opts.Prefix) {
				continue
			}
			// When cleaning up by prefix, skip the project filter —
			// temp recipes live under the user's personal LP project,
			// not the configured recipe project.
			if opts.Prefix == "" && targetRef != "" && recipe.Project != "" && recipe.Project != targetRef {
				continue
			}
			toDelete = append(toDelete, recipe)
		}

		if opts.DryRun {
			for _, recipe := range toDelete {
				s.logger.Info("would delete recipe", "recipe", recipe.Name)
				result.DeletedRecipes = append(result.DeletedRecipes, recipe.Name)
			}
		} else {
			deleted := s.deleteRecipesConcurrent(ctx, pb.Builder, toDelete)
			result.DeletedRecipes = append(result.DeletedRecipes, deleted...)
		}
	}

	// Clean up temporary branches if repoManager is available and prefix is set.
	if s.repoManager != nil && opts.Prefix != "" {
		branchResult, err := s.cleanupBranches(ctx, opts)
		if err != nil {
			s.logger.Warn("branch cleanup failed", "error", err)
		} else {
			result.DeletedBranches = append(result.DeletedBranches, branchResult...)
		}
	}

	return result, nil
}

// cleanupBranches removes temporary branches matching the prefix from LP repos.
// Branches live in the user's personal LP project (same as the trigger flow),
// keyed by the watchtower project name — not the code forge project path.
func (s *Service) cleanupBranches(ctx context.Context, opts CleanupOpts) ([]string, error) {
	projFilter := make(map[string]bool, len(opts.Projects))
	for _, p := range opts.Projects {
		projFilter[p] = true
	}

	owner := opts.Owner
	if owner == "" {
		return nil, nil
	}

	// Resolve the user's personal LP build project (same as trigger uses).
	lpProject, err := s.repoManager.GetOrCreateProject(ctx, owner)
	if err != nil {
		return nil, fmt.Errorf("resolve LP project for branch cleanup: %w", err)
	}

	// PrepareTrigger names branches tmp-<recipe-prefix>-<short-sha>.
	// Include the separator so neighboring recipe prefixes do not match.
	branchPrefix := "refs/heads/tmp-" + opts.Prefix + "-"

	// Check each watchtower project's repo for matching branches.
	var deleted []string
	seen := make(map[string]bool) // track repos we've already cleaned
	for name := range s.projects {
		if len(projFilter) > 0 && !projFilter[name] {
			continue
		}

		// The repo name is the watchtower project name (e.g. "sunbeam-charms"),
		// not the code project path (e.g. "openstack/sunbeam-charms").
		repoSelfLink, _, err := s.repoManager.GetOrCreateRepo(ctx, owner, lpProject, name)
		if err != nil {
			s.logger.Warn("could not resolve repo for branch cleanup", "project", name, "error", err)
			continue
		}
		if seen[repoSelfLink] {
			continue
		}
		seen[repoSelfLink] = true

		branches, err := s.repoManager.ListBranches(ctx, repoSelfLink)
		if err != nil {
			s.logger.Warn("could not list branches for cleanup", "project", name, "error", err)
			continue
		}

		// Filter matching branches.
		var toDelete []dto.BranchRef
		for _, branch := range branches {
			if !strings.HasPrefix(branch.Path, branchPrefix) {
				continue
			}
			toDelete = append(toDelete, branch)
		}

		if opts.DryRun {
			for _, branch := range toDelete {
				s.logger.Info("would delete branch", "branch", branch.Path)
				deleted = append(deleted, branch.Path)
			}
		} else {
			deleted = append(deleted, s.deleteBranchesConcurrent(ctx, toDelete)...)
		}
	}

	return deleted, nil
}

// deleteRecipesConcurrent deletes recipes using a bounded worker pool.
func (s *Service) deleteRecipesConcurrent(ctx context.Context, builder port.RecipeBuilder, recipes []*dto.Recipe) []string {
	if len(recipes) == 0 {
		return nil
	}

	type deleteResult struct {
		name string
		ok   bool
	}

	workerCount := min(defaultConcurrency, len(recipes))
	jobs := make(chan *dto.Recipe)
	results := make(chan deleteResult, len(recipes))

	var wg sync.WaitGroup
	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for recipe := range jobs {
				if ctx.Err() != nil {
					return
				}
				if err := builder.DeleteRecipe(ctx, recipe.SelfLink); err != nil {
					s.logger.Warn("failed to delete recipe", "recipe", recipe.Name, "error", err)
					continue
				}
				results <- deleteResult{name: recipe.Name, ok: true}
			}
		}()
	}

	for _, recipe := range recipes {
		if ctx.Err() != nil {
			break
		}
		jobs <- recipe
	}
	close(jobs)
	wg.Wait()
	close(results)

	var deleted []string
	for r := range results {
		if r.ok {
			deleted = append(deleted, r.name)
		}
	}
	return deleted
}

// deleteBranchesConcurrent deletes git ref branches using a bounded worker pool.
func (s *Service) deleteBranchesConcurrent(ctx context.Context, branches []dto.BranchRef) []string {
	if len(branches) == 0 {
		return nil
	}

	type deleteResult struct {
		path string
		ok   bool
	}

	workerCount := min(defaultConcurrency, len(branches))
	jobs := make(chan dto.BranchRef)
	results := make(chan deleteResult, len(branches))

	var wg sync.WaitGroup
	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for branch := range jobs {
				if ctx.Err() != nil {
					return
				}
				if err := s.repoManager.DeleteGitRef(ctx, branch.SelfLink); err != nil {
					s.logger.Warn("failed to delete branch", "branch", branch.Path, "error", err)
					continue
				}
				results <- deleteResult{path: branch.Path, ok: true}
			}
		}()
	}

	for _, branch := range branches {
		if ctx.Err() != nil {
			break
		}
		jobs <- branch
	}
	close(jobs)
	wg.Wait()
	close(results)

	var deleted []string
	for r := range results {
		if r.ok {
			deleted = append(deleted, r.path)
		}
	}
	return deleted
}
