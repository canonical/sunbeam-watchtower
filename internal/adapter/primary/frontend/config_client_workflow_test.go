// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/canonical/sunbeam-watchtower/pkg/client"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func TestConfigClientWorkflowShow(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/config" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(dto.Config{
			Build: dto.BuildConfig{
				DefaultPrefix: "tmp-build",
			},
			TUI: dto.TUIConfig{
				DefaultPane: "packages",
			},
		})
	}))
	defer ts.Close()

	workflow := NewConfigClientWorkflow(NewClientTransport(client.NewClient(ts.URL)))
	got, err := workflow.Show(context.Background())
	if err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	if got.Build.DefaultPrefix != "tmp-build" {
		t.Fatalf("Show() = %+v, want tmp-build prefix", got)
	}
	if got.TUI.DefaultPane != "packages" {
		t.Fatalf("Show() TUI.DefaultPane = %q, want packages", got.TUI.DefaultPane)
	}
}
