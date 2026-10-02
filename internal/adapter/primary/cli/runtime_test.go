package cli

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/adapter/primary/api"
	runtimeadapter "github.com/canonical/sunbeam-watchtower/internal/adapter/primary/runtime"
	"github.com/spf13/cobra"
)

func TestCommandNeedsPersistentServer(t *testing.T) {
	t.Run("auth commands do not require persistent server", func(t *testing.T) {
		auth := &cobra.Command{Use: "auth"}
		cmd := &cobra.Command{Use: "status"}
		cmd.SetArgs(nil)
		auth.AddCommand(cmd)

		if commandNeedsPersistentServer(cmd) {
			t.Fatal("auth status should not require persistent server")
		}
	})

	t.Run("build trigger only requires persistent server when async", func(t *testing.T) {
		build := &cobra.Command{Use: "build"}
		cmd := &cobra.Command{Use: "trigger"}
		cmd.Flags().Bool("async", false, "")
		build.AddCommand(cmd)

		if commandNeedsPersistentServer(cmd) {
			t.Fatal("sync build trigger should not require persistent server")
		}
		if err := cmd.Flags().Set("async", "true"); err != nil {
			t.Fatalf("Set(async) error = %v", err)
		}
		if !commandNeedsPersistentServer(cmd) {
			t.Fatal("async build trigger should require persistent server")
		}
	})

	t.Run("project sync only requires persistent server when async", func(t *testing.T) {
		project := &cobra.Command{Use: "project"}
		cmd := &cobra.Command{Use: "sync"}
		cmd.Flags().Bool("async", false, "")
		project.AddCommand(cmd)

		if commandNeedsPersistentServer(cmd) {
			t.Fatal("sync project command should not require persistent server")
		}
		if err := cmd.Flags().Set("async", "true"); err != nil {
			t.Fatalf("Set(async) error = %v", err)
		}
		if !commandNeedsPersistentServer(cmd) {
			t.Fatal("async project sync should require persistent server")
		}
	})

	t.Run("team sync only requires persistent server when async", func(t *testing.T) {
		team := &cobra.Command{Use: "team"}
		cmd := &cobra.Command{Use: "sync"}
		cmd.Flags().Bool("async", false, "")
		team.AddCommand(cmd)

		if commandNeedsPersistentServer(cmd) {
			t.Fatal("sync team command should not require persistent server")
		}
		if err := cmd.Flags().Set("async", "true"); err != nil {
			t.Fatalf("Set(async) error = %v", err)
		}
		if !commandNeedsPersistentServer(cmd) {
			t.Fatal("async team sync should require persistent server")
		}
	})
}

func TestTargetPolicyForCommand(t *testing.T) {
	auth := &cobra.Command{Use: "auth"}
	authStatus := &cobra.Command{Use: "status"}
	auth.AddCommand(authStatus)

	review := &cobra.Command{Use: "review"}
	reviewList := &cobra.Command{Use: "list"}
	review.AddCommand(reviewList)

	if got := targetPolicyForCommand(authStatus); got != runtimeadapter.TargetPolicyPreferExistingDaemon {
		t.Fatalf("auth policy = %v, want %v", got, runtimeadapter.TargetPolicyPreferExistingDaemon)
	}
	if got := targetPolicyForCommand(reviewList); got != runtimeadapter.TargetPolicyPreferExistingDaemon {
		t.Fatalf("review policy = %v, want %v", got, runtimeadapter.TargetPolicyPreferExistingDaemon)
	}
}

func TestLocalServerManagerStatusRunning(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	manager, err := newLocalServerManager(&Options{Logger: logger})
	if err != nil {
		t.Fatalf("newLocalServerManager() error = %v", err)
	}
	if err := os.MkdirAll(manager.Paths().Dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	srv := api.NewServer(logger, api.ServerOptions{UnixSocket: manager.Paths().Socket})
	if err := srv.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer srv.Shutdown(context.Background())

	if err := os.WriteFile(manager.Paths().PIDFile, []byte("1234"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	startedAt := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	if err := runtimeadapter.WriteLocalServerMetadata(manager.Paths().Metadata, runtimeadapter.LocalServerMetadata{
		PID:       1234,
		Address:   "unix://" + manager.Paths().Socket,
		StartedAt: startedAt,
		LogFile:   manager.Paths().LogFile,
	}); err != nil {
		t.Fatalf("writeLocalServerMetadata() error = %v", err)
	}

	status, err := manager.Status(context.Background())
	if err != nil {
		t.Fatalf("status() error = %v", err)
	}
	if !status.Running {
		t.Fatal("status.Running = false, want true")
	}
	if status.PID != 1234 {
		t.Fatalf("status.PID = %d, want 1234", status.PID)
	}
	if status.Address != "unix://"+manager.Paths().Socket {
		t.Fatalf("status.Address = %q, want %q", status.Address, "unix://"+manager.Paths().Socket)
	}
	if !status.StartedAt.Equal(startedAt) {
		t.Fatalf("status.StartedAt = %s, want %s", status.StartedAt, startedAt)
	}
	if !status.MetadataPresent {
		t.Fatal("status.MetadataPresent = false, want true")
	}
}

func TestLocalServerManagerStatusDetectsStaleFiles(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	manager, err := newLocalServerManager(&Options{Logger: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))})
	if err != nil {
		t.Fatalf("newLocalServerManager() error = %v", err)
	}
	if err := os.MkdirAll(manager.Paths().Dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(manager.Paths().Socket, []byte("stale"), 0o600); err != nil {
		t.Fatalf("WriteFile(socket) error = %v", err)
	}
	if err := os.WriteFile(manager.Paths().PIDFile, []byte("9999"), 0o600); err != nil {
		t.Fatalf("WriteFile(pid) error = %v", err)
	}
	if err := runtimeadapter.WriteLocalServerMetadata(manager.Paths().Metadata, runtimeadapter.LocalServerMetadata{
		PID:     9999,
		Address: manager.Paths().SocketURI,
		LogFile: manager.Paths().LogFile,
	}); err != nil {
		t.Fatalf("writeLocalServerMetadata() error = %v", err)
	}

	status, err := manager.Status(context.Background())
	if err != nil {
		t.Fatalf("status() error = %v", err)
	}
	if status.Running {
		t.Fatal("status.Running = true, want false")
	}
	if !status.StaleSocket || !status.StalePIDFile {
		t.Fatalf("status = %+v, want stale socket and pid markers", status)
	}
	if !status.MetadataPresent {
		t.Fatal("status.MetadataPresent = false, want true")
	}
}

func TestLocalServerManagerStopCleansStaleFiles(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	manager, err := newLocalServerManager(&Options{Logger: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))})
	if err != nil {
		t.Fatalf("newLocalServerManager() error = %v", err)
	}
	if err := os.MkdirAll(manager.Paths().Dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	for _, path := range []string{manager.Paths().Socket, manager.Paths().PIDFile, manager.Paths().Metadata} {
		if err := os.WriteFile(path, []byte("stale"), 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}

	stopped, err := manager.Stop(context.Background())
	if err != nil {
		t.Fatalf("stop() error = %v", err)
	}
	if stopped {
		t.Fatal("stop() = true, want false for stale-only cleanup")
	}
	for _, path := range []string{manager.Paths().Socket, manager.Paths().PIDFile, manager.Paths().Metadata} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, stat error = %v", path, err)
		}
	}
}

func TestServerLifecycleCommandsDoNotRequireConfig(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	var out bytes.Buffer
	opts := &Options{Out: &out, ErrOut: &bytes.Buffer{}}
	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"server", "status", "--config", filepath.Join(t.TempDir(), "missing.yaml")})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "Local server not running.") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestServerStatusCmd_ReportsStaleFiles(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	manager, err := newLocalServerManager(&Options{Logger: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))})
	if err != nil {
		t.Fatalf("newLocalServerManager() error = %v", err)
	}
	if err := os.MkdirAll(manager.Paths().Dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(manager.Paths().Socket, []byte("stale"), 0o600); err != nil {
		t.Fatalf("WriteFile(socket) error = %v", err)
	}
	if err := os.WriteFile(manager.Paths().PIDFile, []byte("9999"), 0o600); err != nil {
		t.Fatalf("WriteFile(pid) error = %v", err)
	}

	var out bytes.Buffer
	opts := &Options{Out: &out, ErrOut: &bytes.Buffer{}}
	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"server", "status"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "Stale socket file detected") || !strings.Contains(output, "Stale pid file detected") {
		t.Fatalf("unexpected output: %q", output)
	}
}
