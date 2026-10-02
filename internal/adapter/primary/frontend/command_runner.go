// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
)

// ShellCommandRunner implements port.CommandRunner using os/exec with sh -c.
type ShellCommandRunner struct{}

var _ port.CommandRunner = (*ShellCommandRunner)(nil)

func (r *ShellCommandRunner) Run(ctx context.Context, dir string, command string) error {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command %q in %s failed: %w\noutput: %s", command, dir, err, output)
	}
	return nil
}
