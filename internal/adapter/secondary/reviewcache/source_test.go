// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package reviewcache

import (
	"context"
	"errors"
	"testing"
	"time"

	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

type mockForge struct {
	mrs       []forge.MergeRequest
	details   map[string]forge.MergeRequest
	detailErr map[string]error
}

func (m *mockForge) Type() forge.ForgeType { return forge.ForgeGitHub }

func (m *mockForge) ListMergeRequests(_ context.Context, _ string, _ forge.ListMergeRequestsOpts) ([]forge.MergeRequest, error) {
	return append([]forge.MergeRequest(nil), m.mrs...), nil
}

func (m *mockForge) GetMergeRequest(_ context.Context, _ string, id string) (*forge.MergeRequest, error) {
	if err := m.detailErr[id]; err != nil {
		return nil, err
	}
	detail, ok := m.details[id]
	if !ok {
		return nil, errors.New("missing detail")
	}
	return &detail, nil
}

func (m *mockForge) ListCommits(_ context.Context, _ string, _ forge.ListCommitsOpts) ([]forge.Commit, error) {
	return nil, nil
}

func TestCachedForge_SyncAndServeFromCache(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatalf("NewCache() error = %v", err)
	}
	defer cache.Close()

	now := time.Now().UTC()
	live := &mockForge{
		mrs: []forge.MergeRequest{
			{Forge: forge.ForgeGitHub, Repo: "snap-openstack", ID: "#1", Title: "Open review", State: forge.MergeStateOpen, UpdatedAt: now},
			{Forge: forge.ForgeGitHub, Repo: "snap-openstack", ID: "#2", Title: "Old merged review", State: forge.MergeStateMerged, UpdatedAt: now.Add(-60 * 24 * time.Hour)},
		},
		details: map[string]forge.MergeRequest{
			"#1": {Forge: forge.ForgeGitHub, Repo: "snap-openstack", ID: "#1", Title: "Open review", State: forge.MergeStateOpen, UpdatedAt: now, Comments: []forge.ReviewComment{{Kind: forge.ReviewCommentGeneral, Body: "cached"}}},
		},
	}
	cached := NewCachedForge(live, cache, "snap-openstack", nil)

	result, err := cached.Sync(context.Background(), "canonical/snap-openstack", nil)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if result.Summaries != 2 {
		t.Fatalf("Summaries = %d, want 2", result.Summaries)
	}

	listed, err := cached.ListMergeRequests(context.Background(), "canonical/snap-openstack", forge.ListMergeRequestsOpts{})
	if err != nil {
		t.Fatalf("ListMergeRequests() error = %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("len(ListMergeRequests()) = %d, want 2", len(listed))
	}

	detail, err := cached.GetMergeRequest(context.Background(), "canonical/snap-openstack", "#1")
	if err != nil {
		t.Fatalf("GetMergeRequest(#1) error = %v", err)
	}
	if len(detail.Comments) != 1 {
		t.Fatalf("cached detail comments = %+v, want full detail", detail.Comments)
	}

	if _, err := cached.GetMergeRequest(context.Background(), "canonical/snap-openstack", "#2"); !errors.Is(err, ErrDetailNotCached) {
		t.Fatalf("GetMergeRequest(#2) error = %v, want ErrDetailNotCached", err)
	}
}

func TestCachedForge_ListFailsBeforeSync(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatalf("NewCache() error = %v", err)
	}
	defer cache.Close()

	cached := NewCachedForge(&mockForge{}, cache, "snap-openstack", nil)
	if _, err := cached.ListMergeRequests(context.Background(), "canonical/snap-openstack", forge.ListMergeRequestsOpts{}); !errors.Is(err, ErrNotSynced) {
		t.Fatalf("ListMergeRequests() error = %v, want ErrNotSynced", err)
	}
}
