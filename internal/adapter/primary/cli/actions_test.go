// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"slices"
	"testing"

	frontend "github.com/gboutry/sunbeam-watchtower/internal/adapter/primary/frontend"
	"github.com/spf13/cobra"
)

func TestLeafCommandsHaveActionClassification(t *testing.T) {
	root := NewRootCmd(&Options{
		Out:    &bytes.Buffer{},
		ErrOut: &bytes.Buffer{},
	})

	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.Name() == "help" || cmd.Name() == "completion" {
			return
		}
		children := cmd.Commands()
		runnableChildren := 0
		for _, child := range children {
			if child.Hidden {
				continue
			}
			runnableChildren++
			walk(child)
		}
		if runnableChildren > 0 || !cmd.Runnable() {
			return
		}
		if got := commandActionID(cmd, nil); got == "" {
			t.Fatalf("command %q is missing an action classification", cmd.CommandPath())
		}
	}

	walk(root)
}

func TestDynamicActionResolution(t *testing.T) {
	tests := []struct {
		name  string
		path  []string
		args  []string
		flags map[string]string
		want  frontend.ActionID
	}{
		{name: "build cleanup apply", path: []string{"build", "cleanup"}, want: frontend.ActionBuildCleanupApply},
		{name: "build cleanup dry run", path: []string{"build", "cleanup"}, flags: map[string]string{"dry-run": "true"}, want: frontend.ActionBuildCleanupDryRun},
		{name: "project sync apply", path: []string{"project", "sync"}, want: frontend.ActionProjectSyncApply},
		{name: "project sync dry run", path: []string{"project", "sync"}, flags: map[string]string{"dry-run": "true"}, want: frontend.ActionProjectSyncDryRun},
		{name: "bug sync apply", path: []string{"bug", "sync"}, want: frontend.ActionBugSyncApply},
		{name: "bug sync dry run", path: []string{"bug", "sync"}, flags: map[string]string{"dry-run": "true"}, want: frontend.ActionBugSyncDryRun},
		{name: "team sync apply", path: []string{"team", "sync"}, want: frontend.ActionTeamSyncApply},
		{name: "team sync dry run", path: []string{"team", "sync"}, flags: map[string]string{"dry-run": "true"}, want: frontend.ActionTeamSyncDryRun},
		{name: "cache sync all", path: []string{"cache", "sync"}, want: frontend.ActionCacheSync},
		{name: "cache sync packages", path: []string{"cache", "sync"}, args: []string{cacheTypePackagesIndex}, want: frontend.ActionCacheSyncPackages},
		{name: "cache sync packagesets", path: []string{"cache", "sync"}, args: []string{cacheTypePackageSets}, want: frontend.ActionCacheSyncPackageSets},
		{name: "cache sync reviews", path: []string{"cache", "sync"}, args: []string{cacheTypeReviews}, want: frontend.ActionCacheSyncReviews},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := NewRootCmd(&Options{
				Out:    &bytes.Buffer{},
				ErrOut: &bytes.Buffer{},
			})
			cmd := mustFindCommand(t, root, tt.path...)
			for name, value := range tt.flags {
				if err := cmd.Flags().Set(name, value); err != nil {
					t.Fatalf("Flags().Set(%q) error = %v", name, err)
				}
			}
			if got := commandActionID(cmd, tt.args); got != tt.want {
				t.Fatalf("commandActionID(%q) = %q, want %q", cmd.CommandPath(), got, tt.want)
			}
		})
	}
}

func TestHiddenServerActionsAreNotExported(t *testing.T) {
	for _, actionID := range []frontend.ActionID{
		frontend.ActionServeStart,
		frontend.ActionServerStart,
		frontend.ActionServerStop,
	} {
		if got := frontend.DescribeAction(actionID).ExportPolicy; got != frontend.ExportPolicyHidden {
			t.Fatalf("DescribeAction(%q).ExportPolicy = %q, want %q", actionID, got, frontend.ExportPolicyHidden)
		}
	}
}

func mustFindCommand(t *testing.T, root *cobra.Command, path ...string) *cobra.Command {
	t.Helper()

	cmd := root
	for _, name := range path {
		children := cmd.Commands()
		index := slices.IndexFunc(children, func(child *cobra.Command) bool {
			return child.Name() == name
		})
		if index < 0 {
			t.Fatalf("command %q not found under %q", name, cmd.CommandPath())
		}
		cmd = children[index]
	}
	return cmd
}
