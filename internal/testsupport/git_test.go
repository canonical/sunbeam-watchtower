// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package testsupport

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGitFixturePreservesHookRepository(t *testing.T) {
	hookRepo, fixtureRepo := t.TempDir(), t.TempDir()
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := GitCommand(args...)
		cmd.Dir = dir
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	run(hookRepo, "init")
	configPath := filepath.Join(hookRepo, ".git", "config")
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_DIR", filepath.Join(hookRepo, ".git"))
	t.Setenv("GIT_WORK_TREE", hookRepo)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(hookRepo, ".git", "index"))
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.bare")
	t.Setenv("GIT_CONFIG_VALUE_0", "true")
	run(fixtureRepo, "init")
	run(fixtureRepo, "config", "user.name", "Fixture")
	run(fixtureRepo, "config", "user.email", "fixture@example.invalid")
	if err := os.WriteFile(filepath.Join(fixtureRepo, "data"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	run(fixtureRepo, "add", "data")
	run(fixtureRepo, "-c", "commit.gpgsign=false", "commit", "-m", "fixture")
	run(fixtureRepo, "rev-parse", "--verify", "HEAD")
	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("fixture changed the hook repository configuration")
	}
	if _, err := os.Stat(filepath.Join(hookRepo, ".git", "index")); !os.IsNotExist(err) {
		t.Fatalf("fixture touched the hook repository index: %v", err)
	}
}
