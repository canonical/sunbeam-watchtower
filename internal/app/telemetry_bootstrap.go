// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	oteladapter "github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/otel"
	"github.com/canonical/sunbeam-watchtower/internal/config"
	bugsvc "github.com/canonical/sunbeam-watchtower/internal/core/service/bug"
	buildsvc "github.com/canonical/sunbeam-watchtower/internal/core/service/build"
	commitsvc "github.com/canonical/sunbeam-watchtower/internal/core/service/commit"
	pkgsvc "github.com/canonical/sunbeam-watchtower/internal/core/service/package"
	reviewsvc "github.com/canonical/sunbeam-watchtower/internal/core/service/review"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func (a *App) Telemetry(ctx context.Context) (*oteladapter.Telemetry, error) {
	a.telemetryOnce.Do(func() {
		if a.runtimeMode != RuntimeModePersistent {
			return
		}
		if !otelConfigured(a.GetConfig()) {
			return
		}
		a.telemetry, a.telemetryErr = oteladapter.New(ctx, a.GetConfig().OTel, a.Logger, newTelemetrySnapshotSource(a))
		if a.telemetryErr == nil && a.telemetry != nil {
			a.Logger = a.telemetry.Logger(a.Logger)
		}
	})
	return a.telemetry, a.telemetryErr
}

func otelConfigured(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	otel := cfg.OTel
	return (otel.Metrics.Self.Enabled && otel.Metrics.Self.ListenAddr != "") ||
		(otel.Metrics.Domain.Enabled && otel.Metrics.Domain.ListenAddr != "") ||
		otel.Traces.Enabled ||
		otel.Logs.Enabled
}

type telemetrySnapshotSource struct {
	app *App
}

func newTelemetrySnapshotSource(app *App) *oteladapter.SnapshotSource {
	source := &telemetrySnapshotSource{app: app}
	return &oteladapter.SnapshotSource{
		AuthSnapshot:      source.AuthSnapshot,
		OperationSnapshot: source.OperationSnapshot,
		ProjectSnapshot:   source.ProjectSnapshot,
		BuildSnapshot:     source.BuildSnapshot,
		ReleaseSnapshot:   source.ReleaseSnapshot,
		ReviewSnapshot:    source.ReviewSnapshot,
		CommitSnapshot:    source.CommitSnapshot,
		BugSnapshot:       source.BugSnapshot,
		PackageSnapshot:   source.PackageSnapshot,
		ExcusesSnapshot:   source.ExcusesSnapshot,
		CacheSnapshot:     source.CacheSnapshot,
	}
}

func (s *telemetrySnapshotSource) AuthSnapshot(ctx context.Context) (*oteladapter.AuthSnapshot, error) {
	providers := []oteladapter.AuthMetric{{Provider: "launchpad"}, {Provider: "github"}}
	record, err := s.app.LaunchpadCredentialStore().Load(ctx)
	if err != nil {
		return nil, err
	}
	providers[0].Authenticated = record != nil && record.Credentials != nil && record.Credentials.AccessToken != ""
	githubRecord, err := s.app.GitHubCredentialStore().Load(ctx)
	if err != nil {
		return nil, err
	}
	providers[1].Authenticated = githubRecord != nil && githubRecord.Credentials != nil && githubRecord.Credentials.AccessToken != ""
	return &oteladapter.AuthSnapshot{Providers: providers}, nil
}

func (s *telemetrySnapshotSource) OperationSnapshot(ctx context.Context) (*oteladapter.OperationSnapshot, error) {
	jobs, err := s.app.OperationStore().List(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	type key struct{ kind, state string }
	rows := map[key]oteladapter.OperationMetric{}
	for _, job := range jobs {
		k := key{kind: string(job.Kind), state: string(job.State)}
		row := rows[k]
		row.Kind = k.kind
		row.State = k.state
		row.Count++
		age := job.CreatedAt
		if !job.StartedAt.IsZero() {
			age = job.StartedAt
		}
		currentAge := now.Sub(age)
		if currentAge > row.OldestAge {
			row.OldestAge = currentAge
		}
		rows[k] = row
	}
	result := make([]oteladapter.OperationMetric, 0, len(rows))
	for _, row := range rows {
		result = append(result, row)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind == result[j].Kind {
			return result[i].State < result[j].State
		}
		return result[i].Kind < result[j].Kind
	})
	return &oteladapter.OperationSnapshot{Operations: result}, nil
}

func (s *telemetrySnapshotSource) ProjectSnapshot(ctx context.Context) (*oteladapter.ProjectSnapshot, error) {
	_ = ctx
	cfg := s.app.GetConfig()
	projects := make([]oteladapter.ProjectMetric, 0, len(cfg.Projects))
	for _, project := range cfg.Projects {
		cached := false
		cloneURL, err := project.Code.CloneURL()
		if err == nil {
			if path, pathErr := cachedRepoPath(cloneURL); pathErr == nil {
				if _, statErr := os.Stat(path); statErr == nil {
					cached = true
				}
			}
		}
		projects = append(projects, oteladapter.ProjectMetric{
			Project:      project.Name,
			Forge:        strings.ToLower(project.Code.Forge),
			ArtifactType: defaultMetricValue(project.ArtifactType),
			RepoCached:   cached,
		})
	}
	return &oteladapter.ProjectSnapshot{Projects: projects}, nil
}

func (s *telemetrySnapshotSource) BuildSnapshot(ctx context.Context) (*oteladapter.BuildSnapshot, error) {
	service, err := s.app.BuildService()
	if err != nil {
		return nil, err
	}
	builds, _, err := service.List(ctx, buildsvc.ListOpts{All: true})
	if err != nil {
		return nil, err
	}
	now := time.Now()
	type key struct{ project, artifactType, backend, state string }
	rows := map[key]oteladapter.BuildMetric{}
	for _, build := range builds {
		k := key{project: build.Project, artifactType: strings.ToLower(build.ArtifactType.String()), backend: "launchpad", state: build.State.String()}
		row := rows[k]
		row.Project = k.project
		row.ArtifactType = k.artifactType
		row.Backend = k.backend
		row.State = k.state
		row.Count++
		stamp := build.CreatedAt
		if !build.StartedAt.IsZero() {
			stamp = build.StartedAt
		}
		age := now.Sub(stamp)
		if age > row.OldestAge {
			row.OldestAge = age
		}
		rows[k] = row
	}
	result := make([]oteladapter.BuildMetric, 0, len(rows))
	for _, row := range rows {
		result = append(result, row)
	}
	return &oteladapter.BuildSnapshot{Builds: result}, nil
}

func (s *telemetrySnapshotSource) ReleaseSnapshot(ctx context.Context) (*oteladapter.ReleaseSnapshot, error) {
	cache, err := s.app.ReleaseCache()
	if err != nil {
		return nil, err
	}
	snapshots, err := cache.List(ctx)
	if err != nil {
		return nil, err
	}
	out := &oteladapter.ReleaseSnapshot{}
	for _, snapshot := range snapshots {
		for _, channel := range snapshot.Channels {
			for _, target := range channel.Targets {
				out.Targets = append(out.Targets, oteladapter.ReleaseTargetMetric{
					Project:      snapshot.Project,
					ArtifactType: strings.ToLower(snapshot.ArtifactType.String()),
					Artifact:     snapshot.Name,
					Track:        channel.Track,
					Risk:         string(channel.Risk),
					Branch:       channel.Branch,
					Architecture: defaultMetricValue(target.Architecture),
					Revision:     target.Revision,
					ReleasedAt:   target.ReleasedAt,
				})
			}
			for _, resource := range channel.Resources {
				out.Resources = append(out.Resources, oteladapter.ReleaseResourceMetric{
					Project:      snapshot.Project,
					ArtifactType: strings.ToLower(snapshot.ArtifactType.String()),
					Artifact:     snapshot.Name,
					Track:        channel.Track,
					Risk:         string(channel.Risk),
					Branch:       channel.Branch,
					Resource:     resource.Name,
					Revision:     resource.Revision,
				})
			}
		}
	}
	return out, nil
}

func (s *telemetrySnapshotSource) ReviewSnapshot(ctx context.Context) (*oteladapter.ReviewSnapshot, error) {
	clients, err := s.app.BuildReviewProjects()
	if err != nil {
		return nil, err
	}
	mrs, _, err := reviewsvc.NewService(clients, s.app.Logger).List(ctx, reviewsvc.ListOptions{})
	if err != nil {
		return nil, err
	}
	now := time.Now()
	type key struct{ project, forge, state string }
	rows := map[key]oteladapter.ReviewMetric{}
	for _, mr := range mrs {
		k := key{project: mr.Repo, forge: strings.ToLower(mr.Forge.String()), state: strings.ToLower(mr.State.String())}
		row := rows[k]
		row.Project = k.project
		row.Forge = k.forge
		row.State = k.state
		row.Count++
		age := now.Sub(mr.UpdatedAt)
		if age > row.OldestAge {
			row.OldestAge = age
		}
		rows[k] = row
	}
	result := make([]oteladapter.ReviewMetric, 0, len(rows))
	for _, row := range rows {
		result = append(result, row)
	}
	return &oteladapter.ReviewSnapshot{Reviews: result}, nil
}

func (s *telemetrySnapshotSource) CommitSnapshot(ctx context.Context) (*oteladapter.CommitSnapshot, error) {
	sources, err := s.app.BuildCommitSources()
	if err != nil {
		return nil, err
	}
	commits, _, err := commitsvc.NewService(sources, s.app.Logger).List(ctx, commitsvc.ListOptions{IncludeMRs: true})
	if err != nil {
		return nil, err
	}
	totals := map[[3]string]int{}
	for _, commit := range commits {
		state := "none"
		if commit.MergeRequest != nil {
			state = strings.ToLower(commit.MergeRequest.State.String())
		}
		bugRef := "false"
		if len(commit.BugRefs) > 0 {
			bugRef = "true"
		}
		key := [3]string{commit.Repo, state, bugRef}
		totals[key]++
	}
	result := make([]oteladapter.CommitMetric, 0, len(totals))
	for key, count := range totals {
		result = append(result, oteladapter.CommitMetric{Project: key[0], MergeRequestState: key[1], HasBugRef: key[2], Count: count})
	}
	return &oteladapter.CommitSnapshot{Commits: result}, nil
}

func (s *telemetrySnapshotSource) BugSnapshot(ctx context.Context) (*oteladapter.BugSnapshot, error) {
	trackers, projectMap, err := s.app.BuildBugTrackers()
	if err != nil {
		return nil, err
	}
	tasks, _, err := bugsvc.NewService(trackers, projectMap, s.app.Logger).List(ctx, bugsvc.ListOptions{})
	if err != nil {
		return nil, err
	}
	totals := map[[3]string]int{}
	for _, task := range tasks {
		assigned := "false"
		if task.Assignee != "" {
			assigned = "true"
		}
		key := [3]string{task.Project, strings.ToLower(task.Forge.String()), assigned}
		totals[key]++
	}
	result := make([]oteladapter.BugMetric, 0, len(totals))
	for key, count := range totals {
		result = append(result, oteladapter.BugMetric{Project: key[0], Forge: key[1], Assigned: key[2], Count: count})
	}
	return &oteladapter.BugSnapshot{Bugs: result}, nil
}

func (s *telemetrySnapshotSource) PackageSnapshot(ctx context.Context) (*oteladapter.PackageSnapshot, error) {
	cache, err := s.app.DistroCache()
	if err != nil {
		return nil, err
	}
	service := pkgsvc.NewService(cache, s.app.Logger)
	sources := s.app.BuildPackageSources(nil, nil, nil, nil)
	result := make([]oteladapter.PackageMetric, 0, len(sources))
	for _, source := range sources {
		packages, err := service.List(ctx, source.Name, dto.QueryOpts{})
		if err != nil {
			return nil, err
		}
		result = append(result, oteladapter.PackageMetric{Source: source.Name, Distro: "", Release: "", Component: "", Count: len(packages)})
	}
	return &oteladapter.PackageSnapshot{Packages: result}, nil
}

func (s *telemetrySnapshotSource) ExcusesSnapshot(ctx context.Context) (*oteladapter.ExcusesSnapshot, error) {
	cache, err := s.app.ExcusesCache()
	if err != nil {
		return nil, err
	}
	service := pkgsvc.NewExcusesService(cache, s.app.Logger)
	metrics := make([]oteladapter.ExcusesMetric, 0, len(s.app.ExcusesSources()))
	for _, source := range s.app.ExcusesSources() {
		entries, err := service.List(ctx, dto.ExcuseQueryOpts{Trackers: []string{source.Tracker}})
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, oteladapter.ExcusesMetric{Tracker: source.Tracker, Count: len(entries)})
	}
	return &oteladapter.ExcusesSnapshot{Trackers: metrics}, nil
}

func (s *telemetrySnapshotSource) CacheSnapshot(ctx context.Context) (*oteladapter.CacheSnapshot, error) {
	var caches []oteladapter.CacheMetric
	if cache, err := s.app.DistroCache(); err == nil {
		if statuses, statusErr := cache.Status(); statusErr == nil {
			for _, status := range statuses {
				caches = append(caches, oteladapter.CacheMetric{Kind: "packages", Scope: status.Name, Entries: status.EntryCount, LastUpdated: status.LastUpdated})
			}
		}
	}
	if cache, err := s.app.ExcusesCache(); err == nil {
		if statuses, statusErr := cache.Status(); statusErr == nil {
			for _, status := range statuses {
				caches = append(caches, oteladapter.CacheMetric{Kind: "excuses", Scope: status.Tracker, Entries: status.EntryCount, LastUpdated: status.LastUpdated})
			}
		}
	}
	if cache, err := s.app.ReleaseCache(); err == nil {
		if statuses, statusErr := cache.Status(ctx); statusErr == nil {
			for _, status := range statuses {
				caches = append(caches, oteladapter.CacheMetric{Kind: "releases", Scope: status.Project + "/" + status.Name, Entries: status.ChannelCount, LastUpdated: status.LastUpdated})
			}
		}
	}
	if store := s.app.OperationStore(); store != nil {
		if jobs, err := store.List(ctx); err == nil {
			caches = append(caches, oteladapter.CacheMetric{Kind: "operations", Scope: "all", Entries: len(jobs)})
		}
	}
	if cache, err := s.app.BugCache(); err == nil {
		if statuses, statusErr := cache.Status(ctx); statusErr == nil {
			for _, status := range statuses {
				caches = append(caches, oteladapter.CacheMetric{Kind: "bugs", Scope: status.Project, Entries: status.TaskCount, LastUpdated: status.LastSync})
			}
		}
	}
	return &oteladapter.CacheSnapshot{Caches: caches}, nil
}

func cachedRepoPath(cloneURL string) (string, error) {
	cacheDir, err := cacheSubdir("repos")
	if err != nil {
		return "", err
	}
	u, err := url.Parse(cloneURL)
	if err != nil {
		return "", fmt.Errorf("parsing clone URL %q: %w", cloneURL, err)
	}
	host := u.Host
	if idx := strings.LastIndex(host, ":"); idx > 0 {
		host = host[:idx]
	}
	path := strings.TrimPrefix(u.Path, "/")
	if !strings.HasSuffix(path, ".git") {
		path += ".git"
	}
	return filepath.Join(cacheDir, host, path), nil
}

func defaultMetricValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "none"
	}
	return strings.TrimSpace(value)
}
