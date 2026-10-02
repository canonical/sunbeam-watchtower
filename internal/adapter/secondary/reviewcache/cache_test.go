// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package reviewcache

import (
	"context"
	"testing"
	"time"

	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

func TestCache_StoreAndReadRoundTrip(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatalf("NewCache() error = %v", err)
	}
	defer cache.Close()

	summary := forge.MergeRequest{
		Forge:     forge.ForgeGitHub,
		Repo:      "snap-openstack",
		ID:        "#42",
		Title:     "Refactor auth flow",
		State:     forge.MergeStateOpen,
		UpdatedAt: time.Now().UTC(),
	}
	detail := summary
	detail.Description = "Long description"
	detail.Comments = []forge.ReviewComment{{Kind: forge.ReviewCommentGeneral, Author: "alice", Body: "looks good"}}
	detail.Files = []forge.ReviewFile{{Path: "README.md", Status: "modified", Additions: 2, Deletions: 1}}
	detail.DiffText = "diff --git a/README.md b/README.md"

	ctx := context.Background()
	if err := cache.StoreSummaries(ctx, forge.ForgeGitHub, "snap-openstack", []forge.MergeRequest{summary}); err != nil {
		t.Fatalf("StoreSummaries() error = %v", err)
	}
	if err := cache.StoreDetail(ctx, forge.ForgeGitHub, "snap-openstack", detail); err != nil {
		t.Fatalf("StoreDetail() error = %v", err)
	}
	if err := cache.SetLastSync(ctx, forge.ForgeGitHub, "snap-openstack", summary.UpdatedAt); err != nil {
		t.Fatalf("SetLastSync() error = %v", err)
	}

	summaries, err := cache.List(ctx, forge.ForgeGitHub, "snap-openstack")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("len(List()) = %d, want 1", len(summaries))
	}
	if summaries[0].Description != "" || len(summaries[0].Comments) != 0 || len(summaries[0].Files) != 0 || summaries[0].DiffText != "" {
		t.Fatalf("summary should be stripped of heavy detail: %+v", summaries[0])
	}

	got, err := cache.GetDetail(ctx, forge.ForgeGitHub, "snap-openstack", "#42")
	if err != nil {
		t.Fatalf("GetDetail() error = %v", err)
	}
	if got.Description != "Long description" || len(got.Comments) != 1 || len(got.Files) != 1 || got.DiffText == "" {
		t.Fatalf("GetDetail() = %+v, want full detail", got)
	}

	statuses, err := cache.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if len(statuses) != 1 || statuses[0].SummaryCount != 1 || statuses[0].DetailCount != 1 {
		t.Fatalf("Status() = %+v, want one summary and one detail", statuses)
	}
}

func TestCache_PruneDetailsBeforeKeepsOpenAndDropsOldClosed(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatalf("NewCache() error = %v", err)
	}
	defer cache.Close()

	now := time.Now().UTC()
	ctx := context.Background()
	oldClosed := forge.MergeRequest{
		Forge:     forge.ForgeGitHub,
		Repo:      "snap-openstack",
		ID:        "#1",
		State:     forge.MergeStateMerged,
		UpdatedAt: now.Add(-90 * 24 * time.Hour),
	}
	oldOpen := forge.MergeRequest{
		Forge:     forge.ForgeGitHub,
		Repo:      "snap-openstack",
		ID:        "#2",
		State:     forge.MergeStateOpen,
		UpdatedAt: now.Add(-90 * 24 * time.Hour),
	}
	if err := cache.StoreDetail(ctx, forge.ForgeGitHub, "snap-openstack", oldClosed); err != nil {
		t.Fatalf("StoreDetail(oldClosed) error = %v", err)
	}
	if err := cache.StoreDetail(ctx, forge.ForgeGitHub, "snap-openstack", oldOpen); err != nil {
		t.Fatalf("StoreDetail(oldOpen) error = %v", err)
	}

	if err := cache.PruneDetailsBefore(ctx, now.Add(-30*24*time.Hour)); err != nil {
		t.Fatalf("PruneDetailsBefore() error = %v", err)
	}

	if _, err := cache.GetDetail(ctx, forge.ForgeGitHub, "snap-openstack", "#1"); err == nil {
		t.Fatal("old merged review detail should have been pruned")
	}
	if _, err := cache.GetDetail(ctx, forge.ForgeGitHub, "snap-openstack", "#2"); err != nil {
		t.Fatalf("old open review detail should be kept, got error %v", err)
	}
}
