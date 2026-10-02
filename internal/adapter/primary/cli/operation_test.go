package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/canonical/sunbeam-watchtower/pkg/client"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func TestBuildTriggerCmd_AsyncRendersOperation(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(dto.OperationJob{
			ID:        "op-build-1",
			Kind:      dto.OperationKindBuildTrigger,
			State:     dto.OperationStateQueued,
			CreatedAt: time.Date(2026, 3, 7, 9, 0, 0, 0, time.UTC),
		})
	}))
	defer ts.Close()

	var out bytes.Buffer
	opts := &Options{
		Out:    &out,
		ErrOut: &bytes.Buffer{},
		Output: "table",
		Client: client.NewClient(ts.URL),
		Logger: slog.Default(),
	}

	cmd := newBuildCmd(opts)
	cmd.SetArgs([]string{"trigger", "demo", "--async"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if gotPath != "/api/v1/builds/trigger/async" {
		t.Fatalf("path = %q, want async trigger endpoint", gotPath)
	}
	output := out.String()
	if !strings.Contains(output, "op-build-1") || !strings.Contains(output, "build.trigger") || !strings.Contains(output, "queued") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestProjectSyncCmd_AsyncRendersOperation(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(dto.OperationJob{
			ID:        "op-project-1",
			Kind:      dto.OperationKindProjectSync,
			State:     dto.OperationStateRunning,
			CreatedAt: time.Date(2026, 3, 7, 9, 0, 0, 0, time.UTC),
		})
	}))
	defer ts.Close()

	var out bytes.Buffer
	opts := &Options{
		Out:    &out,
		ErrOut: &bytes.Buffer{},
		Output: "table",
		Client: client.NewClient(ts.URL),
		Logger: slog.Default(),
	}

	cmd := newProjectCmd(opts)
	cmd.SetArgs([]string{"sync", "--async", "--dry-run"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if gotPath != "/api/v1/projects/sync/async" {
		t.Fatalf("path = %q, want async project sync endpoint", gotPath)
	}
	output := out.String()
	if !strings.Contains(output, "op-project-1") || !strings.Contains(output, "project.sync") || !strings.Contains(output, "running") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestOperationListCmd_RendersOperations(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/operations" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jobs": []dto.OperationJob{{
				ID:        "op-1",
				Kind:      dto.OperationKindProjectSync,
				State:     dto.OperationStateSucceeded,
				CreatedAt: time.Date(2026, 3, 7, 9, 0, 0, 0, time.UTC),
				Summary:   "done",
			}},
		})
	}))
	defer ts.Close()

	var out bytes.Buffer
	opts := &Options{
		Out:    &out,
		ErrOut: &bytes.Buffer{},
		Output: "table",
		Client: client.NewClient(ts.URL),
		Logger: slog.Default(),
	}

	cmd := newOperationCmd(opts)
	cmd.SetArgs([]string{"list"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "op-1") || !strings.Contains(output, "project.sync") || !strings.Contains(output, "done") {
		t.Fatalf("unexpected output: %q", output)
	}
}
