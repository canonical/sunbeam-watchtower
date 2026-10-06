// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package testsupport

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// ClearGitEnvironment isolates tests that invoke repository readers internally.
// Like testing.T.Setenv, this helper is only valid in non-parallel tests.
func ClearGitEnvironment(t testing.TB) {
	t.Helper()
	for _, entry := range os.Environ() {
		name, value, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "GIT_") {
			t.Setenv(name, value)
			if err := os.Unsetenv(name); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// GitCommand creates a Git fixture command isolated from the invoking Git hook.
// Hook repository paths, index paths, configuration overrides and identities
// must never redirect fixture operations into the developer's repository.
// Callers can append fixture-specific identities to the returned environment.
func GitCommand(args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	return cmd
}
