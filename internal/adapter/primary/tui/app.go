// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"context"
	"fmt"
	"io"

	runtimeadapter "github.com/canonical/sunbeam-watchtower/internal/adapter/primary/runtime"
	tea "github.com/charmbracelet/bubbletea"
)

// Version is set at build time via -ldflags.
var Version = "dev"

// Options controls the TUI process.
type Options struct {
	ConfigPath     string
	ServerAddr     string
	Verbose        bool
	NoColor        bool
	In             io.Reader
	Out            io.Writer
	ErrOut         io.Writer
	ExecutablePath string
}

// Run starts the TUI.
func Run(ctx context.Context, opts Options) error {
	logs := newLogBuffer(sessionLogLineLimit)
	session, err := runtimeadapter.NewSession(ctx, runtimeadapter.Options{
		ConfigPath:     opts.ConfigPath,
		ServerAddr:     opts.ServerAddr,
		Verbose:        opts.Verbose,
		LogWriter:      logs,
		ExecutablePath: opts.ExecutablePath,
		TargetPolicy:   runtimeadapter.TargetPolicyPreferExistingDaemon,
		AccessMode:     runtimeadapter.AccessModeFull,
	})
	if err != nil {
		return err
	}
	defer session.Close()

	model := newRootModelWithLogs(session, opts.NoColor, logs)
	programOpts := []tea.ProgramOption{tea.WithAltScreen()}
	if opts.In != nil {
		programOpts = append(programOpts, tea.WithInput(opts.In))
	}
	if opts.Out != nil {
		programOpts = append(programOpts, tea.WithOutput(opts.Out))
	}
	if _, err := tea.NewProgram(model, programOpts...).Run(); err != nil {
		return fmt.Errorf("run TUI: %w", err)
	}
	return nil
}
