// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

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

// TestSync_MissingMembersInvited: alice is collaborator, bob is in team but not collaborator → bob should be invited (apply mode).
func TestSync_MissingMembersInvited(t *testing.T) {
	team := &fakeTeamProvider{
		members: []dto.TeamMember{
			{Username: "alice", Email: "alice@example.com"},
			{Username: "bob", Email: "bob@example.com"},
		},
	}
	store := &fakeStoreManager{
		collaborators: map[string][]dto.StoreCollaborator{
			"my-snap": {
				{Username: "alice", Email: "alice@example.com", Status: "accepted"},
			},
		},
	}
	stores := map[dto.ArtifactType]port.StoreCollaboratorManager{
		dto.ArtifactSnap: store,
	}
	targets := []dto.SyncTarget{
		{Project: "my-project", ArtifactType: dto.ArtifactSnap, StoreName: "my-snap"},
	}

	svc := NewService(team, stores, testLogger())
	result, err := svc.Sync(context.Background(), "myteam", targets, false)
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	if len(result.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact result, got %d", len(result.Artifacts))
	}
	art := result.Artifacts[0]

	if len(art.Invited) != 1 || art.Invited[0] != "bob@example.com" {
		t.Errorf("Invited = %v, want [bob@example.com]", art.Invited)
	}

	// Verify InviteCollaborator was actually called in apply mode.
	if len(store.invited["my-snap"]) != 1 || store.invited["my-snap"][0] != "bob@example.com" {
		t.Errorf("InviteCollaborator called with %v, want [bob@example.com]", store.invited["my-snap"])
	}
}

// TestSync_DryRunDoesNotInvite: alice in team but not collaborator → reports would-be-invited but InviteCollaborator NOT called.
func TestSync_DryRunDoesNotInvite(t *testing.T) {
	team := &fakeTeamProvider{
		members: []dto.TeamMember{
			{Username: "alice", Email: "alice@example.com"},
		},
	}
	store := &fakeStoreManager{
		collaborators: map[string][]dto.StoreCollaborator{},
	}
	stores := map[dto.ArtifactType]port.StoreCollaboratorManager{
		dto.ArtifactSnap: store,
	}
	targets := []dto.SyncTarget{
		{Project: "my-project", ArtifactType: dto.ArtifactSnap, StoreName: "my-snap"},
	}

	svc := NewService(team, stores, testLogger())
	result, err := svc.Sync(context.Background(), "myteam", targets, true /* dryRun */)
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	if len(result.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact result, got %d", len(result.Artifacts))
	}
	art := result.Artifacts[0]

	if len(art.Invited) != 1 || art.Invited[0] != "alice@example.com" {
		t.Errorf("Invited = %v, want [alice@example.com]", art.Invited)
	}

	// Verify InviteCollaborator was NOT called in dry-run mode.
	if len(store.invited) != 0 {
		t.Errorf("InviteCollaborator should not be called in dry-run mode, got %v", store.invited)
	}
}

// TestSync_ExtraCollaboratorsReported: eve is collaborator but not in team → reported as extra.
func TestSync_ExtraCollaboratorsReported(t *testing.T) {
	team := &fakeTeamProvider{
		members: []dto.TeamMember{
			{Username: "alice", Email: "alice@example.com"},
		},
	}
	store := &fakeStoreManager{
		collaborators: map[string][]dto.StoreCollaborator{
			"my-snap": {
				{Username: "alice", Email: "alice@example.com", Status: "accepted"},
				{Username: "eve", Email: "eve@example.com", Status: "accepted"},
			},
		},
	}
	stores := map[dto.ArtifactType]port.StoreCollaboratorManager{
		dto.ArtifactSnap: store,
	}
	targets := []dto.SyncTarget{
		{Project: "my-project", ArtifactType: dto.ArtifactSnap, StoreName: "my-snap"},
	}

	svc := NewService(team, stores, testLogger())
	result, err := svc.Sync(context.Background(), "myteam", targets, false)
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	if len(result.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact result, got %d", len(result.Artifacts))
	}
	art := result.Artifacts[0]

	if len(art.Extra) != 1 || art.Extra[0] != "eve@example.com" {
		t.Errorf("Extra = %v, want [eve@example.com]", art.Extra)
	}
	if len(art.Invited) != 0 {
		t.Errorf("Invited = %v, want []", art.Invited)
	}
}

func TestSync_ExtraCollaboratorWithoutEmailUsesAvailableIdentity(t *testing.T) {
	team := &fakeTeamProvider{
		members: []dto.TeamMember{
			{Username: "alice", Email: "alice@example.com"},
		},
	}
	store := &fakeStoreManager{
		collaborators: map[string][]dto.StoreCollaborator{
			"my-charm": {
				{Username: "alice", Email: "alice@example.com", Status: "accepted"},
				{Username: "eve", DisplayName: "Eve Smith", Status: "accepted"},
			},
		},
	}
	stores := map[dto.ArtifactType]port.StoreCollaboratorManager{
		dto.ArtifactCharm: store,
	}
	targets := []dto.SyncTarget{
		{Project: "my-project", ArtifactType: dto.ArtifactCharm, StoreName: "my-charm"},
	}

	svc := NewService(team, stores, testLogger())
	result, err := svc.Sync(context.Background(), "myteam", targets, true)
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	if len(result.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact result, got %d", len(result.Artifacts))
	}
	art := result.Artifacts[0]

	if len(art.Extra) != 1 || art.Extra[0] != "eve" {
		t.Errorf("Extra = %v, want [eve]", art.Extra)
	}
}

// TestSync_PendingNotReinvited: bob has pending invite → reported as pending, NOT re-invited.
func TestSync_PendingNotReinvited(t *testing.T) {
	team := &fakeTeamProvider{
		members: []dto.TeamMember{
			{Username: "bob", Email: "bob@example.com"},
		},
	}
	store := &fakeStoreManager{
		collaborators: map[string][]dto.StoreCollaborator{
			"my-snap": {
				{Username: "bob", Email: "bob@example.com", Status: "pending"},
			},
		},
	}
	stores := map[dto.ArtifactType]port.StoreCollaboratorManager{
		dto.ArtifactSnap: store,
	}
	targets := []dto.SyncTarget{
		{Project: "my-project", ArtifactType: dto.ArtifactSnap, StoreName: "my-snap"},
	}

	svc := NewService(team, stores, testLogger())
	result, err := svc.Sync(context.Background(), "myteam", targets, false)
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	if len(result.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact result, got %d", len(result.Artifacts))
	}
	art := result.Artifacts[0]

	if len(art.Pending) != 1 || art.Pending[0] != "bob@example.com" {
		t.Errorf("Pending = %v, want [bob@example.com]", art.Pending)
	}
	if len(art.Invited) != 0 {
		t.Errorf("Invited = %v, want [] (pending should not be re-invited)", art.Invited)
	}
	if len(store.invited) != 0 {
		t.Errorf("InviteCollaborator should not be called for pending user, got %v", store.invited)
	}
}

// TestSync_AlreadySynced: alice in both team and store → AlreadySync = true.
func TestSync_AlreadySynced(t *testing.T) {
	team := &fakeTeamProvider{
		members: []dto.TeamMember{
			{Username: "alice", Email: "alice@example.com"},
		},
	}
	store := &fakeStoreManager{
		collaborators: map[string][]dto.StoreCollaborator{
			"my-snap": {
				{Username: "alice", Email: "alice@example.com", Status: "accepted"},
			},
		},
	}
	stores := map[dto.ArtifactType]port.StoreCollaboratorManager{
		dto.ArtifactSnap: store,
	}
	targets := []dto.SyncTarget{
		{Project: "my-project", ArtifactType: dto.ArtifactSnap, StoreName: "my-snap"},
	}

	svc := NewService(team, stores, testLogger())
	result, err := svc.Sync(context.Background(), "myteam", targets, false)
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	if len(result.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact result, got %d", len(result.Artifacts))
	}
	art := result.Artifacts[0]

	if !art.AlreadySync {
		t.Errorf("AlreadySync = false, want true (team and store are in sync)")
	}
	if len(art.Invited) != 0 || len(art.Extra) != 0 || len(art.Pending) != 0 {
		t.Errorf("expected no diffs, got Invited=%v Extra=%v Pending=%v", art.Invited, art.Extra, art.Pending)
	}
}

// TestSync_HiddenEmailWarning: member with empty email → warning emitted.
func TestSync_HiddenEmailWarning(t *testing.T) {
	team := &fakeTeamProvider{
		members: []dto.TeamMember{
			{Username: "ghost", Email: ""}, // hidden email
		},
	}
	store := &fakeStoreManager{
		collaborators: map[string][]dto.StoreCollaborator{},
	}
	stores := map[dto.ArtifactType]port.StoreCollaboratorManager{
		dto.ArtifactSnap: store,
	}
	targets := []dto.SyncTarget{
		{Project: "my-project", ArtifactType: dto.ArtifactSnap, StoreName: "my-snap"},
	}

	svc := NewService(team, stores, testLogger())
	result, err := svc.Sync(context.Background(), "myteam", targets, false)
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	if len(result.Warnings) == 0 {
		t.Error("expected at least one warning for member with hidden email")
	}

	found := false
	for _, w := range result.Warnings {
		if contains(w, "ghost") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("warning should mention username 'ghost', got: %v", result.Warnings)
	}
}

// TestSync_StoreAPIErrorNonFatal: ListCollaborators returns error → artifact gets Error field set, no fatal error returned.
func TestSync_StoreAPIErrorNonFatal(t *testing.T) {
	team := &fakeTeamProvider{
		members: []dto.TeamMember{
			{Username: "alice", Email: "alice@example.com"},
		},
	}
	store := &fakeStoreManager{
		listErr: fmt.Errorf("store API unavailable"),
	}
	stores := map[dto.ArtifactType]port.StoreCollaboratorManager{
		dto.ArtifactSnap: store,
	}
	targets := []dto.SyncTarget{
		{Project: "my-project", ArtifactType: dto.ArtifactSnap, StoreName: "my-snap"},
	}

	svc := NewService(team, stores, testLogger())
	result, err := svc.Sync(context.Background(), "myteam", targets, false)
	if err != nil {
		t.Fatalf("Sync() returned fatal error, want nil: %v", err)
	}

	if len(result.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact result, got %d", len(result.Artifacts))
	}
	art := result.Artifacts[0]

	if art.Error == "" {
		t.Error("expected artifact Error to be set when store API fails")
	}
}

// TestSync_UnsupportedMarksArtifactAndDoesNotFailOthers: a store that returns
// ErrCollaboratorsUnsupported should surface as a non-fatal per-artifact
// status with the dashboard URL attached, and sibling artifacts on other
// store types should still sync normally.
func TestSync_UnsupportedMarksArtifactAndDoesNotFailOthers(t *testing.T) {
	team := &fakeTeamProvider{
		members: []dto.TeamMember{
			{Username: "alice", Email: "alice@example.com"},
		},
	}
	snapStore := &fakeStoreManager{
		listErr: fmt.Errorf("wrapping: %w", port.ErrCollaboratorsUnsupported),
	}
	charmStore := &fakeStoreManager{
		collaborators: map[string][]dto.StoreCollaborator{
			"my-charm": {
				{Username: "alice", Email: "alice@example.com", Status: "accepted"},
			},
		},
	}
	stores := map[dto.ArtifactType]port.StoreCollaboratorManager{
		dto.ArtifactSnap:  snapStore,
		dto.ArtifactCharm: charmStore,
	}
	targets := []dto.SyncTarget{
		{Project: "snap-proj", ArtifactType: dto.ArtifactSnap, StoreName: "my-snap"},
		{Project: "charm-proj", ArtifactType: dto.ArtifactCharm, StoreName: "my-charm"},
	}

	svc := NewService(team, stores, testLogger())
	result, err := svc.Sync(context.Background(), "myteam", targets, false)
	if err != nil {
		t.Fatalf("Sync() returned fatal error: %v", err)
	}

	if len(result.Artifacts) != 2 {
		t.Fatalf("expected 2 artifact results, got %d", len(result.Artifacts))
	}

	snap := result.Artifacts[0]
	if !snap.Unsupported {
		t.Errorf("snap artifact Unsupported = false, want true")
	}
	if snap.Error != "" {
		t.Errorf("snap artifact Error = %q, want empty (unsupported should not set Error)", snap.Error)
	}
	wantURL := "https://dashboard.snapcraft.io/snaps/my-snap/collaboration/"
	if snap.UnsupportedURL != wantURL {
		t.Errorf("snap artifact UnsupportedURL = %q, want %q", snap.UnsupportedURL, wantURL)
	}
	if len(snap.Invited) != 0 {
		t.Errorf("snap artifact Invited = %v, want []", snap.Invited)
	}
	// Invite must not be attempted when listing reports unsupported.
	if len(snapStore.invited) != 0 {
		t.Errorf("InviteCollaborator should not be called on unsupported store, got %v", snapStore.invited)
	}

	charm := result.Artifacts[1]
	if charm.Unsupported {
		t.Errorf("charm artifact Unsupported = true, want false")
	}
	if !charm.AlreadySync {
		t.Errorf("charm artifact AlreadySync = false, want true")
	}
}

// TestSync_UnsupportedURLEmptyForNonSnapArtifact: non-snap artifacts that
// report unsupported get Unsupported=true but no dashboard URL (we only know
// one today).
func TestSync_UnsupportedURLEmptyForNonSnapArtifact(t *testing.T) {
	team := &fakeTeamProvider{
		members: []dto.TeamMember{{Username: "alice", Email: "alice@example.com"}},
	}
	store := &fakeStoreManager{listErr: port.ErrCollaboratorsUnsupported}
	stores := map[dto.ArtifactType]port.StoreCollaboratorManager{
		dto.ArtifactCharm: store,
	}
	targets := []dto.SyncTarget{
		{Project: "charm-proj", ArtifactType: dto.ArtifactCharm, StoreName: "my-charm"},
	}

	svc := NewService(team, stores, testLogger())
	result, err := svc.Sync(context.Background(), "myteam", targets, false)
	if err != nil {
		t.Fatalf("Sync() returned fatal error: %v", err)
	}
	art := result.Artifacts[0]
	if !art.Unsupported {
		t.Errorf("Unsupported = false, want true")
	}
	if art.UnsupportedURL != "" {
		t.Errorf("UnsupportedURL = %q, want empty string for non-snap artifact", art.UnsupportedURL)
	}
}

// TestSync_AuthExpiredMarksArtifactWithHint: when a store reports
// ErrStoreAuthExpired (e.g. Charmhub returning an expired macaroon), the
// artifact must be flagged with AuthExpired + a login hint without
// aborting the sync, and sibling artifacts on other stores must still run.
func TestSync_AuthExpiredMarksArtifactWithHint(t *testing.T) {
	team := &fakeTeamProvider{
		members: []dto.TeamMember{
			{Username: "alice", Email: "alice@example.com"},
		},
	}
	charmStore := &fakeStoreManager{
		listErr: fmt.Errorf("listing collaborators: HTTP 400: macaroon-needs-refresh: macaroon has expired: %w", port.ErrStoreAuthExpired),
	}
	snapStore := &fakeStoreManager{
		collaborators: map[string][]dto.StoreCollaborator{
			"my-snap": {
				{Username: "alice", Email: "alice@example.com", Status: "accepted"},
			},
		},
	}
	stores := map[dto.ArtifactType]port.StoreCollaboratorManager{
		dto.ArtifactCharm: charmStore,
		dto.ArtifactSnap:  snapStore,
	}
	targets := []dto.SyncTarget{
		{Project: "charm-proj", ArtifactType: dto.ArtifactCharm, StoreName: "rabbitmq-k8s"},
		{Project: "snap-proj", ArtifactType: dto.ArtifactSnap, StoreName: "my-snap"},
	}

	svc := NewService(team, stores, testLogger())
	result, err := svc.Sync(context.Background(), "myteam", targets, false)
	if err != nil {
		t.Fatalf("Sync() returned fatal error: %v", err)
	}

	if len(result.Artifacts) != 2 {
		t.Fatalf("expected 2 artifact results, got %d", len(result.Artifacts))
	}

	charm := result.Artifacts[0]
	if !charm.AuthExpired {
		t.Errorf("charm AuthExpired = false, want true")
	}
	if charm.Error != "" {
		t.Errorf("charm Error = %q, want empty (auth-expired should not set Error)", charm.Error)
	}
	if !contains(charm.AuthHint, "watchtower auth charmhub login") {
		t.Errorf("charm AuthHint = %q, want mention of \"watchtower auth charmhub login\"", charm.AuthHint)
	}
	if len(charmStore.invited) != 0 {
		t.Errorf("InviteCollaborator should not be called when auth expired, got %v", charmStore.invited)
	}

	snap := result.Artifacts[1]
	if snap.AuthExpired {
		t.Errorf("snap AuthExpired = true, want false")
	}
	if !snap.AlreadySync {
		t.Errorf("snap AlreadySync = false, want true (sibling must still sync)")
	}
}

// contains is a helper to check if a string contains a substring.
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
