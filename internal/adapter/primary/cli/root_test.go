// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	runtimeadapter "github.com/canonical/sunbeam-watchtower/internal/adapter/primary/runtime"
	"github.com/canonical/sunbeam-watchtower/internal/app"
	"github.com/canonical/sunbeam-watchtower/internal/config"
)

func TestOptionsFrontendUsesSessionFacade(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	session, err := runtimeadapter.NewSession(context.Background(), runtimeadapter.Options{
		LogWriter:    &bytes.Buffer{},
		TargetPolicy: runtimeadapter.TargetPolicyPreferEmbedded,
	})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	defer session.Close()

	opts := &Options{Session: session}
	if opts.Frontend() != session.Frontend {
		t.Fatal("Frontend() should return the session facade")
	}
	if opts.Application() != session.App {
		t.Fatal("Application() should return the session app")
	}
}

func TestOptionsApplicationFallsBackToStandaloneApp(t *testing.T) {
	opts := &Options{}
	opts.App = app.NewApp(&config.Config{}, discardTestLogger())

	if opts.Application() != opts.App {
		t.Fatal("Application() should return the standalone app when no session is active")
	}
}

func TestNewRootCmd_HelpGroupsCommands(t *testing.T) {
	var out bytes.Buffer
	opts := &Options{
		Out:    &out,
		ErrOut: &out,
	}

	cmd := NewRootCmd(opts)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"Workflows",
		"Meta Commands",
		"review",
		"build",
		"releases",
		"auth",
		"cache",
		"serve",
		"server",
		"version",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("help output missing %q:\n%s", want, help)
		}
	}
}

func discardTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
}
