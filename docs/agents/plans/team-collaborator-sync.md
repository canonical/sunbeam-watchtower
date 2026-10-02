# Team Collaborator Sync Implementation Plan

> **Superseded note:** The `internal/adapter/primary/frontend/team_discovery.go` primitive (`DiscoverTargets`) described in Task 4 and listed under File Structure has been removed. Manifest discovery is now provided by the canonical `internal/core/service/artifactdiscovery` service, which the team sync workflow consumes directly.
>
> **Final architecture (mono-repo fan-out):**
>
> - `internal/core/service/artifactdiscovery` is the single server-side discovery seam. It reads artifact manifests from a `gitcache`-backed bare repo (HEAD tree only, no worktree) for charm/snap/rock types and returns a deterministic sorted `[]DiscoveredArtifact{Name, RelPath, ArtifactType, Resources}` list. The concrete `gitcache` adapter is wired through `internal/app/artifactdiscovery_bootstrap.go` and reached from `TeamServerWorkflow` via `app.ArtifactDiscoveryService()`.
> - `TeamServerWorkflow.Sync` iterates configured projects, calls `app.GitCache().EnsureRepo` for each, invokes the discovery service, and emits one `dto.SyncTarget` per discovered artifact. Single-artifact repos still produce exactly one target (e.g. `openstack` → one snap name from `snap/snapcraft.yaml`); mono-repos fan out (e.g. `sunbeam-charms` → ~50 charms under `charms/*/charmcraft.yaml`). Discovery errors become per-project warnings on the result instead of aborting the whole sync.
> - `proj.team.skip_artifacts` (defined on `ProjectTeamConfig` in `internal/config/config.go`) is an optional per-project allow-list filter that drops individual artifacts from the fan-out. The field mirrors the existing `proj.release.skip_artifacts` pattern, is validated the same way (no empty strings, no duplicates), and is a no-op when the `team` block is absent. Example: `sunbeam-charms` configures `team.skip_artifacts: [sunbeam-libs]` to keep the internal-only library charm out of collaborator sync while still letting it ship via release tracking.
> - Test coverage: `internal/core/service/artifactdiscovery` has unit tests for single-charm root, charm and nested-charm mono-repos, snap root and mono-repo, rock mono-repo, missing manifests, and malformed YAML. `internal/adapter/primary/frontend/team_server_workflow_test.go` exercises the fan-out end-to-end for single-snap, monorepo-charms, project-filter, discovery-error, no-artifacts, skip-artifacts, and non-artifact-project cases against in-memory discoverer/cache/syncer seams.
> - The original `DiscoverTargets`-based design (Task 4 in this plan) is fully superseded. Task 4 is retained below only as historical context.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Sync Launchpad team members as collaborators on Snap Store and Charmhub artifacts, with dry-run/apply semantics and CLI/API/TUI frontends.

**Architecture:** Hexagonal architecture with ports in `internal/core/port/`, sync service in `internal/core/service/teamsync/`, store adapters in `internal/adapter/secondary/{snapstore,charmhub}/`, and frontend integration through the shared facade/workflow layer. Auth flows for both stores follow the existing LP/GitHub pattern.

**Tech Stack:** Go, bbolt (existing), Snap Store publisher API, Charmhub publisher API, Ubuntu SSO macaroons, cobra CLI, huma API, bubbletea TUI.

**Spec:** `docs/superpowers/specs/2026-03-19-team-collaborator-sync-design.md`

---

## File Structure

### New Files
- `pkg/dto/v1/collaborators.go` — TeamMember, StoreCollaborator, TeamSyncResult DTOs
- `internal/core/port/collaborators.go` — StoreCollaboratorManager and LaunchpadTeamProvider ports
- `internal/core/service/teamsync/service.go` — core sync service
- `internal/core/service/teamsync/service_test.go` — sync service tests
- `internal/adapter/primary/frontend/team_discovery.go` — client-side manifest discovery (local worktree access stays in frontend layer per AGENTS.md "local paths stay local")
- `internal/adapter/primary/frontend/team_discovery_test.go` — discovery tests
- `internal/adapter/secondary/snapstore/collaborator.go` — Snap Store collaborator adapter
- `internal/adapter/secondary/snapstore/collaborator_test.go` — adapter tests
- `internal/adapter/secondary/charmhub/collaborator.go` — Charmhub collaborator adapter
- `internal/adapter/secondary/charmhub/collaborator_test.go` — adapter tests
- `internal/adapter/secondary/snapstore/auth.go` — Snap Store credential store
- `internal/adapter/secondary/charmhub/auth.go` — Charmhub credential store
- `internal/adapter/primary/frontend/team_workflow.go` — team sync workflow (client + server)
- `internal/adapter/primary/frontend/team_workflow_test.go` — workflow tests
- `internal/adapter/primary/cli/team.go` — CLI subcommand
- `internal/adapter/primary/api/team.go` — API endpoints

### Modified Files
- `internal/config/config.go` — add CollaboratorsConfig to Config struct (line 410)
- `internal/config/config_test.go` — config validation tests
- `pkg/dto/v1/operation.go` — add OperationKindTeamSync (line 14)
- `pkg/dto/v1/build.go` — add MarshalJSON/UnmarshalJSON to ArtifactType for clean JSON serialization
- `internal/adapter/primary/frontend/action_catalog.go` — add team sync + store auth actions
- `internal/adapter/primary/frontend/action_catalog_test.go` — add TeamSync to dry-run/apply variant test
- `internal/adapter/primary/frontend/server_facade.go` — add Teams() accessor
- `internal/adapter/primary/frontend/client_facade.go` — add Teams() accessor
- `internal/adapter/primary/frontend/facade.go` — add StartTeamSync async method
- `internal/adapter/primary/cli/actions.go` — add team.sync action selector (line 57)
- `internal/adapter/primary/cli/actions_test.go` — add team.sync test cases
- `internal/adapter/primary/cli/runtime.go` — add team to commandNeedsPersistentServer (line 68)
- `internal/adapter/primary/cli/runtime_test.go` — add team sync --async test case
- `internal/adapter/primary/cli/root.go` — register team command group
- `internal/adapter/primary/runtime/runtime.go` — add `api.RegisterTeamAPI(srv.API(), application)` (line 429)
- `internal/adapter/primary/tui/model.go` — add team sync to sync overlay
- `internal/adapter/primary/frontend/auth_workflow.go` — add SnapStore/Charmhub auth methods
- `internal/core/service/auth/service.go` — add store auth flows
- `internal/core/port/auth.go` — add store credential store ports
- `internal/app/app.go` — wire team sync service
- `pkg/launchpad/v1/types.go` — add PreferredEmailAddressLink field to Person struct (line 34)
- `pkg/launchpad/v1/person.go` — add email resolution for team members
- `pkg/client/client.go` — add TeamSync and TeamSyncAsync methods
- `PLAN.md` — document new auth surface and sync operation
- `.coverage-policy.yaml` — add teamsync package threshold

---

### Task 1: DTOs and Operation Kind

**Files:**
- Create: `pkg/dto/v1/collaborators.go`
- Modify: `pkg/dto/v1/operation.go`

- [ ] **Step 1: Create collaborator DTOs**

Create `pkg/dto/v1/collaborators.go`:

```go
package dto

// TeamMember represents a member of a Launchpad team.
type TeamMember struct {
	Username string `json:"username" yaml:"username"`
	Email    string `json:"email,omitempty" yaml:"email,omitempty"` // empty if hidden
}

// StoreCollaborator represents a collaborator on a store artifact.
type StoreCollaborator struct {
	Username    string `json:"username" yaml:"username"`
	Email       string `json:"email" yaml:"email"`
	DisplayName string `json:"display_name,omitempty" yaml:"display_name,omitempty"`
	Status      string `json:"status" yaml:"status"` // "accepted", "pending", "expired"
}

// SyncTarget identifies one store artifact to check for collaborators.
type SyncTarget struct {
	Project      string       `json:"project" yaml:"project"`
	ArtifactType ArtifactType `json:"artifact_type" yaml:"artifact_type"`
	StoreName    string       `json:"store_name" yaml:"store_name"`
}

// TeamSyncRequest holds parameters for a team collaborator sync.
type TeamSyncRequest struct {
	Projects []string `json:"projects,omitempty" yaml:"projects,omitempty"`
	DryRun   bool     `json:"dry_run" yaml:"dry_run"`
}

// TeamSyncResult holds the outcome of a team collaborator sync.
type TeamSyncResult struct {
	Artifacts []ArtifactSyncResult `json:"artifacts" yaml:"artifacts"`
	Warnings  []string             `json:"warnings,omitempty" yaml:"warnings,omitempty"`
}

// ArtifactSyncResult holds the sync outcome for one store artifact.
type ArtifactSyncResult struct {
	Project      string       `json:"project" yaml:"project"`
	ArtifactType ArtifactType `json:"artifact_type" yaml:"artifact_type"`
	StoreName    string       `json:"store_name" yaml:"store_name"`
	Invited      []string     `json:"invited,omitempty" yaml:"invited,omitempty"`
	Extra        []string     `json:"extra,omitempty" yaml:"extra,omitempty"`
	Pending      []string     `json:"pending,omitempty" yaml:"pending,omitempty"`
	AlreadySync  bool         `json:"already_sync" yaml:"already_sync"`
	Error        string       `json:"error,omitempty" yaml:"error,omitempty"`
}
```

- [ ] **Step 2: Add OperationKindTeamSync**

In `pkg/dto/v1/operation.go`, add after `OperationKindProjectSync`:

```go
OperationKindTeamSync OperationKind = "team.sync"
```

- [ ] **Step 3: Verify build**

Run: `go build ./pkg/dto/...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add pkg/dto/v1/collaborators.go pkg/dto/v1/operation.go
git commit -m "dto: add team collaborator sync DTOs and operation kind"
```

---

### Task 2: Configuration

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write failing test for CollaboratorsConfig validation**

In `internal/config/config_test.go`, add:

```go
func TestValidate_CollaboratorsEmptyTeam(t *testing.T) {
	cfg := &Config{
		Collaborators: &CollaboratorsConfig{LaunchpadTeam: ""},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should error for empty launchpad_team")
	}
}

func TestValidate_CollaboratorsValid(t *testing.T) {
	cfg := &Config{
		Collaborators: &CollaboratorsConfig{LaunchpadTeam: "ubuntu-openstack"},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error: %v", err)
	}
}

func TestValidate_CollaboratorsNil(t *testing.T) {
	cfg := &Config{}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() should pass with nil collaborators: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/config/ -run TestValidate_Collaborators -v`
Expected: FAIL (CollaboratorsConfig type not defined)

- [ ] **Step 3: Add CollaboratorsConfig struct and wire into Config**

In `internal/config/config.go`, add the struct (near other config types, around line 240):

```go
// CollaboratorsConfig holds settings for team-to-store collaborator sync.
type CollaboratorsConfig struct {
	LaunchpadTeam string `mapstructure:"launchpad_team" yaml:"launchpad_team"`
}
```

In the `Config` struct (line ~420), add:

```go
Collaborators *CollaboratorsConfig `mapstructure:"collaborators" yaml:"collaborators,omitempty"`
```

In the `Validate()` method, add validation before the final return:

```go
if cfg.Collaborators != nil && cfg.Collaborators.LaunchpadTeam == "" {
	return fmt.Errorf("collaborators.launchpad_team must not be empty when collaborators block is present")
}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/config/ -run TestValidate_Collaborators -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "config: add collaborators block with launchpad_team"
```

---

### Task 3: Ports

**Files:**
- Create: `internal/core/port/collaborators.go`

- [ ] **Step 1: Create port interfaces**

Create `internal/core/port/collaborators.go`:

```go
package port

import (
	"context"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// StoreCollaboratorManager manages collaborators on a backing store artifact.
type StoreCollaboratorManager interface {
	ListCollaborators(ctx context.Context, storeName string) ([]dto.StoreCollaborator, error)
	InviteCollaborator(ctx context.Context, storeName string, email string) error
}

// LaunchpadTeamProvider fetches members of a Launchpad team.
type LaunchpadTeamProvider interface {
	GetTeamMembers(ctx context.Context, teamName string) ([]dto.TeamMember, error)
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./internal/core/port/...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/core/port/collaborators.go
git commit -m "port: add StoreCollaboratorManager and LaunchpadTeamProvider"
```

---

### Task 4: Manifest Discovery

Discovery is client-side (per AGENTS.md "local paths stay local"). The frontend layer scans local worktrees and produces prepared `[]dto.SyncTarget` for the server.

**Files:**
- Create: `internal/adapter/primary/frontend/team_discovery.go`
- Create: `internal/adapter/primary/frontend/team_discovery_test.go`

- [ ] **Step 1: Write failing tests for manifest discovery**

Create `internal/adapter/primary/frontend/team_discovery_test.go`:

```go
package frontend

import (
	"os"
	"path/filepath"
	"testing"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func TestDiscoverTargets_SnapAtRoot(t *testing.T) {
	dir := t.TempDir()
	snapDir := filepath.Join(dir, "snap")
	os.MkdirAll(snapDir, 0o755)
	os.WriteFile(filepath.Join(snapDir, "snapcraft.yaml"), []byte("name: my-snap\n"), 0o644)

	targets, err := DiscoverTargets(dir, dto.ArtifactSnap)
	if err != nil {
		t.Fatalf("DiscoverTargets() error = %v", err)
	}
	if len(targets) != 1 || targets[0] != "my-snap" {
		t.Fatalf("DiscoverTargets() = %v, want [my-snap]", targets)
	}
}

func TestDiscoverTargets_CharmMonorepo(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"keystone-k8s", "nova-k8s"} {
		d := filepath.Join(dir, "charms", name)
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "charmcraft.yaml"), []byte("name: "+name+"\n"), 0o644)
	}

	targets, err := DiscoverTargets(dir, dto.ArtifactCharm)
	if err != nil {
		t.Fatalf("DiscoverTargets() error = %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("DiscoverTargets() = %v, want 2 targets", targets)
	}
}

func TestDiscoverTargets_CharmAtRoot(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "charmcraft.yaml"), []byte("name: single-charm\n"), 0o644)

	targets, err := DiscoverTargets(dir, dto.ArtifactCharm)
	if err != nil {
		t.Fatalf("DiscoverTargets() error = %v", err)
	}
	if len(targets) != 1 || targets[0] != "single-charm" {
		t.Fatalf("DiscoverTargets() = %v, want [single-charm]", targets)
	}
}

func TestDiscoverTargets_SnapMonorepo(t *testing.T) {
	dir := t.TempDir()
	d := filepath.Join(dir, "snaps", "my-snap", "snap")
	os.MkdirAll(d, 0o755)
	os.WriteFile(filepath.Join(d, "snapcraft.yaml"), []byte("name: my-snap\n"), 0o644)

	targets, err := DiscoverTargets(dir, dto.ArtifactSnap)
	if err != nil {
		t.Fatalf("DiscoverTargets() error = %v", err)
	}
	if len(targets) != 1 || targets[0] != "my-snap" {
		t.Fatalf("DiscoverTargets() = %v, want [my-snap]", targets)
	}
}

func TestDiscoverTargets_NoManifests(t *testing.T) {
	dir := t.TempDir()

	targets, err := DiscoverTargets(dir, dto.ArtifactSnap)
	if err != nil {
		t.Fatalf("DiscoverTargets() error = %v", err)
	}
	if len(targets) != 0 {
		t.Fatalf("DiscoverTargets() = %v, want empty", targets)
	}
}

func TestDiscoverTargets_MalformedYAML(t *testing.T) {
	dir := t.TempDir()
	snapDir := filepath.Join(dir, "snap")
	os.MkdirAll(snapDir, 0o755)
	os.WriteFile(filepath.Join(snapDir, "snapcraft.yaml"), []byte("not:\n\tvalid yaml"), 0o644)

	targets, err := DiscoverTargets(dir, dto.ArtifactSnap)
	if err != nil {
		t.Fatalf("DiscoverTargets() error = %v", err)
	}
	if len(targets) != 0 {
		t.Fatalf("DiscoverTargets() = %v, want empty (malformed skipped)", targets)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/adapter/primary/frontend/ -run TestDiscoverTargets -v`
Expected: FAIL (DiscoverTargets not defined)

- [ ] **Step 3: Implement DiscoverTargets**

Create `internal/adapter/primary/frontend/team_discovery.go`:

```go
package frontend

import (
	"os"
	"path/filepath"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	"gopkg.in/yaml.v3"
)

type manifest struct {
	Name string `yaml:"name"`
}

// DiscoverTargets scans a worktree directory for artifact manifest files
// and returns the store names found.
func DiscoverTargets(worktree string, artifactType dto.ArtifactType) ([]string, error) {
	var patterns []string
	switch artifactType {
	case dto.ArtifactSnap:
		patterns = []string{
			filepath.Join(worktree, "snaps", "*", "snap", "snapcraft.yaml"),
			filepath.Join(worktree, "snaps", "*", "*", "snap", "snapcraft.yaml"),
			filepath.Join(worktree, "snap", "snapcraft.yaml"),
		}
	case dto.ArtifactCharm:
		patterns = []string{
			filepath.Join(worktree, "charms", "*", "charmcraft.yaml"),
			filepath.Join(worktree, "charms", "*", "*", "charmcraft.yaml"),
			filepath.Join(worktree, "charmcraft.yaml"),
		}
	default:
		return nil, nil
	}

	seen := make(map[string]bool)
	var names []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		for _, path := range matches {
			name, err := parseManifestName(path)
			if err != nil || name == "" || seen[name] {
				continue
			}
			seen[name] = true
			names = append(names, name)
		}
	}
	return names, nil
}

func parseManifestName(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var m manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return "", err
	}
	return m.Name, nil
}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/adapter/primary/frontend/ -run TestDiscoverTargets -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/primary/frontend/team_discovery.go internal/adapter/primary/frontend/team_discovery_test.go
git commit -m "frontend: add manifest discovery for snap and charm worktrees"
```

---

### Task 5: Sync Service Core Logic

**Files:**
- Create: `internal/core/service/teamsync/service.go`
- Create: `internal/core/service/teamsync/service_test.go`

- [ ] **Step 1: Write failing tests for sync service**

Create `internal/core/service/teamsync/service_test.go`:

```go
package teamsync

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

var _ port.StoreCollaboratorManager = (*fakeStoreManager)(nil)
var _ port.LaunchpadTeamProvider = (*fakeTeamProvider)(nil)

type fakeStoreManager struct {
	collaborators map[string][]dto.StoreCollaborator
	invited       map[string][]string
	listErr       error
	inviteErr     error
}

func (f *fakeStoreManager) ListCollaborators(_ context.Context, storeName string) ([]dto.StoreCollaborator, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.collaborators[storeName], nil
}

func (f *fakeStoreManager) InviteCollaborator(_ context.Context, storeName, email string) error {
	if f.inviteErr != nil {
		return f.inviteErr
	}
	if f.invited == nil {
		f.invited = make(map[string][]string)
	}
	f.invited[storeName] = append(f.invited[storeName], email)
	return nil
}

type fakeTeamProvider struct {
	members []dto.TeamMember
	err     error
}

func (f *fakeTeamProvider) GetTeamMembers(_ context.Context, _ string) ([]dto.TeamMember, error) {
	return f.members, f.err
}

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestSync_MissingMembersInvited(t *testing.T) {
	store := &fakeStoreManager{
		collaborators: map[string][]dto.StoreCollaborator{
			"my-snap": {{Email: "alice@example.com", Status: "accepted"}},
		},
	}
	team := &fakeTeamProvider{
		members: []dto.TeamMember{
			{Username: "alice", Email: "alice@example.com"},
			{Username: "bob", Email: "bob@example.com"},
		},
	}
	svc := NewService(team, map[dto.ArtifactType]port.StoreCollaboratorManager{
		dto.ArtifactSnap: store,
	}, testLogger())

	targets := []dto.SyncTarget{{Project: "p1", ArtifactType: dto.ArtifactSnap, StoreName: "my-snap"}}
	result, err := svc.Sync(context.Background(), "team", targets, false)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact result, got %d", len(result.Artifacts))
	}
	art := result.Artifacts[0]
	if len(art.Invited) != 1 || art.Invited[0] != "bob@example.com" {
		t.Fatalf("expected bob invited, got %v", art.Invited)
	}
	if len(store.invited["my-snap"]) != 1 {
		t.Fatalf("expected InviteCollaborator called once, got %v", store.invited)
	}
}

func TestSync_DryRunDoesNotInvite(t *testing.T) {
	store := &fakeStoreManager{
		collaborators: map[string][]dto.StoreCollaborator{},
	}
	team := &fakeTeamProvider{
		members: []dto.TeamMember{{Username: "alice", Email: "alice@example.com"}},
	}
	svc := NewService(team, map[dto.ArtifactType]port.StoreCollaboratorManager{
		dto.ArtifactSnap: store,
	}, testLogger())

	targets := []dto.SyncTarget{{Project: "p1", ArtifactType: dto.ArtifactSnap, StoreName: "my-snap"}}
	result, err := svc.Sync(context.Background(), "team", targets, true)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if len(result.Artifacts[0].Invited) != 1 {
		t.Fatalf("dry-run should report alice as would-be-invited")
	}
	if len(store.invited) != 0 {
		t.Fatal("dry-run should not call InviteCollaborator")
	}
}

func TestSync_ExtraCollaboratorsReported(t *testing.T) {
	store := &fakeStoreManager{
		collaborators: map[string][]dto.StoreCollaborator{
			"my-snap": {
				{Email: "alice@example.com", Status: "accepted"},
				{Email: "eve@example.com", Status: "accepted"},
			},
		},
	}
	team := &fakeTeamProvider{
		members: []dto.TeamMember{{Username: "alice", Email: "alice@example.com"}},
	}
	svc := NewService(team, map[dto.ArtifactType]port.StoreCollaboratorManager{
		dto.ArtifactSnap: store,
	}, testLogger())

	targets := []dto.SyncTarget{{Project: "p1", ArtifactType: dto.ArtifactSnap, StoreName: "my-snap"}}
	result, err := svc.Sync(context.Background(), "team", targets, false)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if len(result.Artifacts[0].Extra) != 1 || result.Artifacts[0].Extra[0] != "eve@example.com" {
		t.Fatalf("expected eve as extra, got %v", result.Artifacts[0].Extra)
	}
}

func TestSync_PendingNotReinvited(t *testing.T) {
	store := &fakeStoreManager{
		collaborators: map[string][]dto.StoreCollaborator{
			"my-snap": {{Email: "bob@example.com", Status: "pending"}},
		},
	}
	team := &fakeTeamProvider{
		members: []dto.TeamMember{{Username: "bob", Email: "bob@example.com"}},
	}
	svc := NewService(team, map[dto.ArtifactType]port.StoreCollaboratorManager{
		dto.ArtifactSnap: store,
	}, testLogger())

	targets := []dto.SyncTarget{{Project: "p1", ArtifactType: dto.ArtifactSnap, StoreName: "my-snap"}}
	result, err := svc.Sync(context.Background(), "team", targets, false)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	art := result.Artifacts[0]
	if len(art.Pending) != 1 || art.Pending[0] != "bob@example.com" {
		t.Fatalf("expected bob as pending, got %v", art.Pending)
	}
	if len(art.Invited) != 0 {
		t.Fatal("pending member should not be re-invited")
	}
}

func TestSync_AlreadySynced(t *testing.T) {
	store := &fakeStoreManager{
		collaborators: map[string][]dto.StoreCollaborator{
			"my-snap": {{Email: "alice@example.com", Status: "accepted"}},
		},
	}
	team := &fakeTeamProvider{
		members: []dto.TeamMember{{Username: "alice", Email: "alice@example.com"}},
	}
	svc := NewService(team, map[dto.ArtifactType]port.StoreCollaboratorManager{
		dto.ArtifactSnap: store,
	}, testLogger())

	targets := []dto.SyncTarget{{Project: "p1", ArtifactType: dto.ArtifactSnap, StoreName: "my-snap"}}
	result, err := svc.Sync(context.Background(), "team", targets, false)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if !result.Artifacts[0].AlreadySync {
		t.Fatal("expected AlreadySync = true")
	}
}

func TestSync_HiddenEmailWarning(t *testing.T) {
	store := &fakeStoreManager{collaborators: map[string][]dto.StoreCollaborator{}}
	team := &fakeTeamProvider{
		members: []dto.TeamMember{{Username: "hidden-user", Email: ""}},
	}
	svc := NewService(team, map[dto.ArtifactType]port.StoreCollaboratorManager{
		dto.ArtifactSnap: store,
	}, testLogger())

	targets := []dto.SyncTarget{{Project: "p1", ArtifactType: dto.ArtifactSnap, StoreName: "my-snap"}}
	result, err := svc.Sync(context.Background(), "team", targets, false)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected warning for hidden email member")
	}
}

func TestSync_StoreAPIErrorNonFatal(t *testing.T) {
	store := &fakeStoreManager{listErr: fmt.Errorf("store unavailable")}
	team := &fakeTeamProvider{
		members: []dto.TeamMember{{Username: "alice", Email: "alice@example.com"}},
	}
	svc := NewService(team, map[dto.ArtifactType]port.StoreCollaboratorManager{
		dto.ArtifactSnap: store,
	}, testLogger())

	targets := []dto.SyncTarget{{Project: "p1", ArtifactType: dto.ArtifactSnap, StoreName: "my-snap"}}
	result, err := svc.Sync(context.Background(), "team", targets, false)
	if err != nil {
		t.Fatalf("Sync() should not return error for per-artifact failures, got %v", err)
	}
	if result.Artifacts[0].Error == "" {
		t.Fatal("expected non-empty Error on artifact result")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/core/service/teamsync/ -run TestSync -v`
Expected: FAIL (NewService not defined)

- [ ] **Step 3: Implement sync service**

Create `internal/core/service/teamsync/service.go`:

```go
package teamsync

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// Service coordinates team collaborator synchronization.
type Service struct {
	teamProvider port.LaunchpadTeamProvider
	stores       map[dto.ArtifactType]port.StoreCollaboratorManager
	logger       *slog.Logger
}

// NewService creates a team sync service.
func NewService(
	teamProvider port.LaunchpadTeamProvider,
	stores map[dto.ArtifactType]port.StoreCollaboratorManager,
	logger *slog.Logger,
) *Service {
	return &Service{teamProvider: teamProvider, stores: stores, logger: logger}
}

// Sync compares LP team members against store collaborators for each target.
func (s *Service) Sync(ctx context.Context, teamName string, targets []dto.SyncTarget, dryRun bool) (*dto.TeamSyncResult, error) {
	members, err := s.teamProvider.GetTeamMembers(ctx, teamName)
	if err != nil {
		return nil, fmt.Errorf("fetching team %s members: %w", teamName, err)
	}

	result := &dto.TeamSyncResult{}

	// Separate members with and without emails.
	var memberEmails []string
	for _, m := range members {
		if m.Email == "" {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("member %q has hidden email — cannot match to store collaborators", m.Username))
			continue
		}
		memberEmails = append(memberEmails, m.Email)
	}
	teamSet := toSet(memberEmails)

	for _, target := range targets {
		art := s.syncTarget(ctx, target, teamSet, dryRun)
		result.Artifacts = append(result.Artifacts, art)
	}
	return result, nil
}

func (s *Service) syncTarget(ctx context.Context, target dto.SyncTarget, teamEmails map[string]bool, dryRun bool) dto.ArtifactSyncResult {
	art := dto.ArtifactSyncResult{
		Project:      target.Project,
		ArtifactType: target.ArtifactType,
		StoreName:    target.StoreName,
	}

	store, ok := s.stores[target.ArtifactType]
	if !ok {
		art.Error = fmt.Sprintf("no store manager for artifact type %s", target.ArtifactType)
		return art
	}

	collabs, err := store.ListCollaborators(ctx, target.StoreName)
	if err != nil {
		art.Error = fmt.Sprintf("listing collaborators for %s: %v", target.StoreName, err)
		return art
	}

	collabAccepted := make(map[string]bool)
	collabPending := make(map[string]bool)
	allCollabEmails := make(map[string]bool)
	for _, c := range collabs {
		email := strings.ToLower(c.Email)
		allCollabEmails[email] = true
		switch c.Status {
		case "pending":
			collabPending[email] = true
		default:
			collabAccepted[email] = true
		}
	}

	// Missing: in team, not in store.
	for email := range teamEmails {
		lower := strings.ToLower(email)
		if allCollabEmails[lower] {
			if collabPending[lower] {
				art.Pending = append(art.Pending, email)
			}
			continue
		}
		art.Invited = append(art.Invited, email)
		if !dryRun {
			if err := store.InviteCollaborator(ctx, target.StoreName, email); err != nil {
				s.logger.Warn("failed to invite collaborator", "store", target.StoreName, "email", email, "error", err)
			}
		}
	}

	// Extra: in store, not in team.
	for _, c := range collabs {
		lower := strings.ToLower(c.Email)
		if !teamEmails[lower] {
			art.Extra = append(art.Extra, c.Email)
		}
	}

	if len(art.Invited) == 0 && len(art.Extra) == 0 && len(art.Pending) == 0 {
		art.AlreadySync = true
	}

	return art
}

func toSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[strings.ToLower(s)] = true
	}
	return m
}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/core/service/teamsync/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/core/service/teamsync/service.go internal/core/service/teamsync/service_test.go
git commit -m "teamsync: implement core sync service with diff logic"
```

---

### Task 6: Action Catalog and Frontend Workflow

**Files:**
- Modify: `internal/adapter/primary/frontend/action_catalog.go`
- Modify: `internal/adapter/primary/cli/actions.go`
- Create: `internal/adapter/primary/frontend/team_workflow.go`
- Create: `internal/adapter/primary/frontend/team_workflow_test.go`

- [ ] **Step 1: Add action IDs**

In `internal/adapter/primary/frontend/action_catalog.go`, add after `ActionProjectSyncApply` (line ~96):

```go
ActionTeamSyncDryRun ActionID = "team.sync.dry_run"
ActionTeamSyncApply  ActionID = "team.sync.apply"
```

In the `actionCatalog` map (after `ActionProjectSyncApply` entry, line ~173):

```go
ActionTeamSyncDryRun: descriptor(ActionTeamSyncDryRun, "team", "team", MutabilityRead, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Preview team collaborator synchronization."),
ActionTeamSyncApply:  descriptor(ActionTeamSyncApply, "team", "team", MutabilityWrite, LocalEffectNone, RuntimeEmbeddedOK, ExportPolicyAllowed, "Synchronize team members as store collaborators."),
```

- [ ] **Step 2: Add action selector**

In `internal/adapter/primary/cli/actions.go`, add a new case in `commandActionID` (after the `bug.sync` case, line ~57):

```go
case "team.sync":
	if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
		return frontend.ActionTeamSyncDryRun
	}
	return frontend.ActionTeamSyncApply
```

- [ ] **Step 3: Add TeamSync to action catalog variant test**

In `internal/adapter/primary/frontend/action_catalog_test.go`, add `TeamSync` to `TestDryRunAndApplyVariantsDiffer` (around line 85) following the `BuildCleanup`/`ProjectSync`/`BugSync` pattern.

- [ ] **Step 4: Create team workflow**

Create `internal/adapter/primary/frontend/team_workflow.go`. Follow the `project_client_workflow.go` / `project_server_workflow.go` pattern. The exact content depends on the facade and client interfaces — this file bridges the sync service to the frontend layer.

Read `internal/adapter/primary/frontend/project_client_workflow.go` and `project_server_workflow.go` to replicate the pattern exactly, adapting for `TeamSyncRequest` / `TeamSyncResult` DTOs.

Note: API-facing structs must use `required:"false"` on `DryRun bool` and slice fields (per AGENTS.md Huma section).

- [ ] **Step 5: Wire into facades**

In `internal/adapter/primary/frontend/server_facade.go`, add a `teams` field and `Teams()` accessor method, following the existing `projects`/`bugs` pattern.

In `internal/adapter/primary/frontend/client_facade.go`, add the same `teams` field and `Teams()` accessor.

- [ ] **Step 6: Add async support to facade**

In `internal/adapter/primary/frontend/facade.go`, add a `StartTeamSync` method following the `StartProjectSync` pattern. This wraps the sync service call in an operation job with progress reporting.

- [ ] **Step 7: Create workflow test**

Create `internal/adapter/primary/frontend/team_workflow_test.go` with a fake operation store (reuse the `fakeOperationStore` pattern from `operation_workflow_test.go`). Test that `Sync()` returns results and `StartSync()` returns an operation job.

- [ ] **Step 8: Verify build and tests**

Run: `go build ./... && go test ./internal/adapter/primary/frontend/ -run TestTeam -v`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add internal/adapter/primary/frontend/action_catalog.go \
       internal/adapter/primary/frontend/action_catalog_test.go \
       internal/adapter/primary/cli/actions.go \
       internal/adapter/primary/frontend/team_workflow.go \
       internal/adapter/primary/frontend/team_workflow_test.go \
       internal/adapter/primary/frontend/server_facade.go \
       internal/adapter/primary/frontend/client_facade.go \
       internal/adapter/primary/frontend/facade.go
git commit -m "frontend: add team sync workflow with action catalog entries"
```

---

### Task 7: Store Collaborator Adapters

**Files:**
- Create: `internal/adapter/secondary/snapstore/collaborator.go`
- Create: `internal/adapter/secondary/snapstore/collaborator_test.go`
- Create: `internal/adapter/secondary/charmhub/collaborator.go`
- Create: `internal/adapter/secondary/charmhub/collaborator_test.go`

- [ ] **Step 1: Write Snap Store collaborator adapter tests**

Test against `httptest.Server` that returns fake collaborator JSON responses. Test `ListCollaborators` and `InviteCollaborator`.

- [ ] **Step 2: Implement Snap Store collaborator adapter**

Create `internal/adapter/secondary/snapstore/collaborator.go`:
- `CollaboratorManager` struct with `client *http.Client` and `baseURL string`
- `ListCollaborators` calls `GET /api/v2/snaps/{name}/collaborators`
- `InviteCollaborator` calls `POST /api/v2/snaps/{name}/collaborators` with email payload
- Both attach macaroon auth header
- Implement `port.StoreCollaboratorManager` interface

- [ ] **Step 3: Run Snap Store adapter tests**

Run: `go test ./internal/adapter/secondary/snapstore/ -run TestCollaborator -v`
Expected: PASS

- [ ] **Step 4: Write Charmhub collaborator adapter tests**

Same pattern as snap store but with Charmhub API endpoints.

- [ ] **Step 5: Implement Charmhub collaborator adapter**

Create `internal/adapter/secondary/charmhub/collaborator.go`:
- Same pattern as snap store adapter, but targeting `api.charmhub.io/v1/charm/{name}/collaborators`

- [ ] **Step 6: Run Charmhub adapter tests**

Run: `go test ./internal/adapter/secondary/charmhub/ -run TestCollaborator -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/adapter/secondary/snapstore/collaborator.go \
       internal/adapter/secondary/snapstore/collaborator_test.go \
       internal/adapter/secondary/charmhub/collaborator.go \
       internal/adapter/secondary/charmhub/collaborator_test.go
git commit -m "adapters: add Snap Store and Charmhub collaborator managers"
```

---

### Task 8: Store Authentication

**Files:**
- Create: `internal/adapter/secondary/snapstore/auth.go`
- Create: `internal/adapter/secondary/charmhub/auth.go`
- Modify: `internal/core/port/auth.go`
- Modify: `internal/core/service/auth/service.go`
- Modify: `internal/adapter/primary/frontend/auth_workflow.go`
- Modify: `internal/adapter/primary/frontend/action_catalog.go`
- Modify: `pkg/dto/v1/auth.go` (or wherever AuthStatus is defined)

- [ ] **Step 1: Add credential store ports**

In `internal/core/port/auth.go`, add:

```go
type SnapStoreCredentialStore interface {
	Load(ctx context.Context) (*SnapStoreCredentials, error)
	Save(ctx context.Context, creds *SnapStoreCredentials) error
	Clear(ctx context.Context) error
}

type CharmhubCredentialStore interface {
	Load(ctx context.Context) (*CharmhubCredentials, error)
	Save(ctx context.Context, creds *CharmhubCredentials) error
	Clear(ctx context.Context) error
}
```

Define the credential types (macaroon + discharge) in `pkg/dto/v1/auth.go` or a new DTO file.

- [ ] **Step 2: Add auth action IDs**

In `action_catalog.go`, add:

```go
ActionAuthSnapStoreBegin    ActionID = "auth.snapstore.begin"
ActionAuthSnapStoreFinalize ActionID = "auth.snapstore.finalize"
ActionAuthSnapStoreLogout   ActionID = "auth.snapstore.logout"
ActionAuthCharmhubBegin     ActionID = "auth.charmhub.begin"
ActionAuthCharmhubFinalize  ActionID = "auth.charmhub.finalize"
ActionAuthCharmhubLogout    ActionID = "auth.charmhub.logout"
```

Add corresponding entries to the `actionCatalog` map following the LP/GitHub pattern.

- [ ] **Step 3: Extend AuthStatus DTO**

Add `SnapStore` and `Charmhub` fields to `dto.AuthStatus`.

- [ ] **Step 4: Implement credential store adapters**

Create `internal/adapter/secondary/snapstore/auth.go` and `charmhub/auth.go` with keyring + file fallback + env var override. Follow the LP credential store pattern.

- [ ] **Step 5: Extend auth service**

Add snap store and charmhub auth methods to `internal/core/service/auth/service.go`. Follow the `BeginLaunchpad`/`FinalizeLaunchpad` pattern.

- [ ] **Step 6: Extend auth workflow**

Add `BeginSnapStore`, `FinalizeSnapStore`, `LogoutSnapStore` (and Charmhub equivalents) to `internal/adapter/primary/frontend/auth_workflow.go`.

- [ ] **Step 7: Verify build and tests**

Run: `go build ./... && go test ./internal/core/service/auth/ ./internal/adapter/primary/frontend/ -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/core/port/auth.go \
       internal/core/service/auth/service.go \
       internal/adapter/primary/frontend/auth_workflow.go \
       internal/adapter/primary/frontend/action_catalog.go \
       internal/adapter/secondary/snapstore/auth.go \
       internal/adapter/secondary/charmhub/auth.go \
       pkg/dto/v1/
git commit -m "auth: add Snap Store and Charmhub credential flows"
```

---

### Task 9: LP Team Provider Adapter

**Files:**
- Modify: `pkg/launchpad/v1/person.go`
- Create adapter file that wraps LP client into `LaunchpadTeamProvider` port

- [ ] **Step 1: Add PreferredEmailAddressLink to Person struct**

In `pkg/launchpad/v1/types.go`, add to the `Person` struct (after `HideEmailAddresses` at line 34):

```go
PreferredEmailAddressLink string `json:"preferred_email_address_link,omitempty"`
```

- [ ] **Step 2: Implement GetTeamMembersWithEmails**

In `pkg/launchpad/v1/person.go`, add a `GetTeamMembersWithEmails` method that:
1. Calls existing `GetTeamMembers`
2. Filters out sub-teams (`IsTeam == true`)
3. For each member with `PreferredEmailAddressLink != ""`, resolves the email via GET (returns `{"email": "user@example.com"}`)
4. Members with `HideEmailAddresses: true` or empty `PreferredEmailAddressLink` get empty email
5. Returns `[]dto.TeamMember`

- [ ] **Step 2: Create adapter**

Create a thin adapter (in `internal/adapter/secondary/launchpad/` or inline in the composition root) that implements `port.LaunchpadTeamProvider` by delegating to the LP client.

- [ ] **Step 3: Add tests**

Test with `httptest.Server` returning team member JSON with and without email links.

- [ ] **Step 4: Commit**

```bash
git add pkg/launchpad/v1/person.go internal/adapter/secondary/launchpad/
git commit -m "launchpad: add team member email resolution for collaborator sync"
```

---

### Task 10: CLI Integration

**Files:**
- Create: `internal/adapter/primary/cli/team.go`
- Modify: `internal/adapter/primary/cli/root.go`

- [ ] **Step 1: Create team CLI command**

Create `internal/adapter/primary/cli/team.go` following the `project.go` pattern:

```go
// watchtower team sync [--dry-run] [--async] [--project <name>...]
func newTeamCmd(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "team",
		Short: "Team management commands",
	}
	cmd.AddCommand(newTeamSyncCmd(opts))
	return cmd
}

func newTeamSyncCmd(opts *Options) *cobra.Command {
	cmd := withActionSelector(&cobra.Command{
		Use:   "sync",
		Short: "Sync LP team members as store collaborators",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get flags, call workflow.Sync() or workflow.StartSync()
			// based on --async flag. Print results.
		},
	}, "team.sync")
	cmd.Flags().Bool("dry-run", false, "Preview changes without applying")
	cmd.Flags().Bool("async", false, "Run as background operation")
	cmd.Flags().StringSlice("project", nil, "Filter by project name")
	return cmd
}
```

- [ ] **Step 2: Register in root**

In `internal/adapter/primary/cli/root.go`, add `newTeamCmd(opts)` to the root command's subcommands.

- [ ] **Step 3: Update commandNeedsPersistentServer**

In `internal/adapter/primary/cli/runtime.go`, add a case for the `team` parent command in `commandNeedsPersistentServer` (around line 68, following the `project` case pattern):

```go
case "team":
	asyncFlag, _ := cmd.Flags().GetBool("async")
	return asyncFlag
```

- [ ] **Step 4: Add test cases for actions and runtime**

In `internal/adapter/primary/cli/actions_test.go`, add test cases for `team.sync` dry-run and apply to `TestDynamicActionResolution`.

In `internal/adapter/primary/cli/runtime_test.go`, add test case for `team sync --async` requiring persistent server.

- [ ] **Step 5: Verify build and tests**

Run: `go build ./cmd/... && go test ./internal/adapter/primary/cli/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/primary/cli/team.go \
       internal/adapter/primary/cli/root.go \
       internal/adapter/primary/cli/runtime.go \
       internal/adapter/primary/cli/runtime_test.go \
       internal/adapter/primary/cli/actions_test.go
git commit -m "cli: add watchtower team sync command"
```

---

### Task 11: API Integration

**Files:**
- Create: `internal/adapter/primary/api/team.go`

- [ ] **Step 1: Create API endpoints**

Create `internal/adapter/primary/api/team.go` following `projects.go` pattern:

```go
// POST /api/v1/team/sync — synchronous
// POST /api/v1/team/sync/async — returns operation job
func RegisterTeamAPI(api huma.API, application *app.App) {
	facade := frontend.NewServerFacade(application)
	// Register both endpoints
}
```

- [ ] **Step 2: Add API handler tests**

Test in the existing `build_cache_test.go` pattern or create `team_test.go`.

- [ ] **Step 3: Register in API server**

In `internal/adapter/primary/runtime/runtime.go` (line 429, after `RegisterOperationsAPI`), add:

```go
api.RegisterTeamAPI(srv.API(), application)
```

- [ ] **Step 4: Add pkg/client transport methods**

In `pkg/client/client.go`, add `TeamSync` and `TeamSyncAsync` methods following the `ProjectsSync`/`ProjectsSyncAsync` pattern. These are needed by the client-side workflow.

- [ ] **Step 5: Verify build and tests**

Run: `go build ./... && go test ./internal/adapter/primary/api/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/primary/api/team.go \
       internal/adapter/primary/runtime/runtime.go \
       pkg/client/client.go
git commit -m "api: add team sync endpoints"
```

---

### Task 12: TUI Integration

**Files:**
- Modify: `internal/adapter/primary/tui/model.go`

- [ ] **Step 1: Add team sync to sync overlay**

In `model.go`:
1. Add `syncActionCollaborators` to the `syncActionTarget` enum
2. Add a team sync form (following `projectSyncForm` / `bugSyncForm` pattern)
3. Add handler for the new sync action that calls the workflow
4. Add result display in the sync modal summary

Follow the exact pattern used by `projectSyncFinishedMsg` / `bugSyncFinishedMsg`.

- [ ] **Step 2: Verify build**

Run: `go build ./internal/adapter/primary/tui/...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/adapter/primary/tui/model.go
git commit -m "tui: add team sync to sync overlay"
```

---

### Task 13: Composition Root Wiring

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Wire team sync service**

In `app.go`, add lazy initialization for the team sync service:
1. Create `teamSyncService` field with `sync.Once` guard
2. Wire `LaunchpadTeamProvider` adapter (using LP client)
3. Wire `StoreCollaboratorManager` map (snap store + charmhub adapters, using stored credentials)
4. Expose via `TeamSyncService() (*teamsync.Service, error)` method

- [ ] **Step 2: Wire store credential stores**

Add credential stores for snap store and charmhub to the app, following the LP credential store pattern.

- [ ] **Step 3: Verify full build and test suite**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/app/app.go
git commit -m "app: wire team sync service with store adapters"
```

---

### Task 14: Verification and Cleanup

- [ ] **Step 1: Run full test suite**

Run: `go test ./...`
Expected: All tests PASS

- [ ] **Step 2: Run linter**

Run: `golangci-lint run ./...`
Expected: No new issues from changed files

- [ ] **Step 3: Run architecture tests**

Run: `arch-go --color no`
Expected: 100% compliance

- [ ] **Step 4: Run coverage guard**

Run: `go run ./tools/coverageguard --config .coverage-policy.yaml $(git diff --name-only HEAD~14 -- '*.go')`
Expected: All packages PASS thresholds

- [ ] **Step 5: Update .coverage-policy.yaml**

Add coverage threshold entries for new packages:
- `internal/core/service/teamsync/` — 40% threshold (new package)
- Other new adapter packages as needed

- [ ] **Step 6: Update PLAN.md**

Update `PLAN.md` at project root to document:
- New authorization surface (Snap Store and Charmhub auth)
- New sync operation and action catalog entries
- New CLI command, API endpoints

- [ ] **Step 7: Commit**

```bash
git add PLAN.md .coverage-policy.yaml
git commit -m "docs: update PLAN.md with team collaborator sync"
```
