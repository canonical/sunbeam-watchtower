// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/app"
	"github.com/canonical/sunbeam-watchtower/internal/config"
	"github.com/canonical/sunbeam-watchtower/internal/testsupport"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// startTestServer creates and starts a server on an ephemeral port, returning
// the server and its base URL. The caller must call srv.Shutdown.
func startTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	srv := NewServer(discardLogger(), ServerOptions{ListenAddr: "127.0.0.1:0"})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	return srv, "http://" + srv.Addr()
}

func newEphemeralTestApp(t *testing.T, cfg *config.Config) *app.App {
	t.Helper()
	testsupport.ClearForgeCredentials(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	application := app.NewAppWithOptions(cfg, discardLogger(), app.Options{
		RuntimeMode: app.RuntimeModeEphemeral,
	})
	t.Cleanup(func() {
		if err := application.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	return application
}

func TestNewServer(t *testing.T) {
	srv := NewServer(discardLogger(), ServerOptions{ListenAddr: "127.0.0.1:0"})
	if srv.API() == nil {
		t.Fatal("expected API() to be non-nil")
	}
	if srv.Addr() != "" {
		t.Fatalf("expected empty Addr before Start, got %q", srv.Addr())
	}
}

func TestServerReadHeaderTimeout(t *testing.T) {
	srv := NewServer(discardLogger(), ServerOptions{ListenAddr: "127.0.0.1:0"})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown(context.Background())

	if srv.httpSrv == nil {
		t.Fatal("expected http server to be initialized")
	}
	if srv.httpSrv.ReadHeaderTimeout != defaultReadHeaderTimeout {
		t.Fatalf("expected ReadHeaderTimeout=%s, got %s", defaultReadHeaderTimeout, srv.httpSrv.ReadHeaderTimeout)
	}
}

func TestHealth(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	resp, err := http.Get(base + "/api/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" {
		t.Fatalf("expected status=ok, got %q", body.Status)
	}
}

func TestConfigEndpoint(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := app.NewApp(&config.Config{
		Launchpad: config.LaunchpadConfig{DefaultOwner: "test-owner"},
	}, discardLogger())
	RegisterConfigAPI(srv.API(), application)

	resp, err := http.Get(base + "/api/v1/config")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var cfg dto.Config
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Launchpad.DefaultOwner != "test-owner" {
		t.Fatalf("expected default_owner=test-owner, got %q", cfg.Launchpad.DefaultOwner)
	}
}

func TestConfigEndpoint_NilConfig(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := app.NewApp(nil, discardLogger())
	RegisterConfigAPI(srv.API(), application)

	resp, err := http.Get(base + "/api/v1/config")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}

func TestOpenAPISpec(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	resp, err := http.Get(base + "/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)

	if !strings.Contains(body, "/api/v1/health") {
		t.Fatal("expected openapi.json to contain /api/v1/health path")
	}
	if !strings.Contains(body, "Sunbeam Watchtower API") {
		t.Fatal("expected openapi.json to contain API title")
	}
}

func TestDocsEndpoint(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	resp, err := http.Get(base + "/docs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bodyBytes), "<") {
		t.Fatal("expected HTML response from /docs")
	}
}

func TestServerShutdown(t *testing.T) {
	srv, _ := startTestServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}
}

func TestServerShutdown_NotStarted(t *testing.T) {
	srv := NewServer(discardLogger(), ServerOptions{ListenAddr: "127.0.0.1:0"})
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown on non-started server returned error: %v", err)
	}
}

func TestUnixSocket(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "test.sock")

	srv := NewServer(discardLogger(), ServerOptions{UnixSocket: sock})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}

	// Verify socket file exists.
	if _, err := os.Stat(sock); err != nil {
		t.Fatalf("expected socket file at %s: %v", sock, err)
	}

	// Make a request over the unix socket.
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", sock)
			},
		},
	}
	resp, err := client.Get("http://unix/api/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Shutdown and verify socket is cleaned up.
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}
	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Fatal("expected socket file to be removed after Shutdown")
	}
}

func TestServer_TCPSetsTransportContext(t *testing.T) {
	var gotTransport TransportKind
	capturingMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotTransport, _ = r.Context().Value(TransportKindKey).(TransportKind)
			next.ServeHTTP(w, r)
		})
	}

	srv := NewServer(discardLogger(), ServerOptions{
		ListenAddr: "127.0.0.1:0",
		Middleware: capturingMiddleware,
	})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown(context.Background())

	resp, err := http.Get("http://" + srv.Addr() + "/api/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if gotTransport != TransportTCP {
		t.Fatalf("expected TransportTCP, got %q", gotTransport)
	}
}

func TestServer_UnixSetsTransportContext(t *testing.T) {
	var gotTransport TransportKind
	capturingMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotTransport, _ = r.Context().Value(TransportKindKey).(TransportKind)
			next.ServeHTTP(w, r)
		})
	}

	dir := t.TempDir()
	sock := filepath.Join(dir, "transport_test.sock")

	srv := NewServer(discardLogger(), ServerOptions{
		UnixSocket: sock,
		Middleware: capturingMiddleware,
	})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown(context.Background())

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", sock)
			},
		},
	}
	resp, err := client.Get("http://unix/api/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if gotTransport != TransportUnix {
		t.Fatalf("expected TransportUnix, got %q", gotTransport)
	}
}

func TestParseAPIMergeState(t *testing.T) {
	tests := []struct {
		input   string
		want    forge.MergeState
		wantErr bool
	}{
		{"open", forge.MergeStateOpen, false},
		{"Open", forge.MergeStateOpen, false},
		{"OPEN", forge.MergeStateOpen, false},
		{"merged", forge.MergeStateMerged, false},
		{"closed", forge.MergeStateClosed, false},
		{"wip", forge.MergeStateWIP, false},
		{"abandoned", forge.MergeStateAbandoned, false},
		{"invalid", 0, true},
		{"", 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parseAPIMergeState(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseAPIMergeState(%q): err=%v, wantErr=%v", tc.input, err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("parseAPIMergeState(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseAPIForgeType(t *testing.T) {
	tests := []struct {
		input   string
		want    forge.ForgeType
		wantErr bool
	}{
		{"github", forge.ForgeGitHub, false},
		{"GitHub", forge.ForgeGitHub, false},
		{"GITHUB", forge.ForgeGitHub, false},
		{"launchpad", forge.ForgeLaunchpad, false},
		{"gerrit", forge.ForgeGerrit, false},
		{"invalid", 0, true},
		{"", 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parseAPIForgeType(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseAPIForgeType(%q): err=%v, wantErr=%v", tc.input, err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("parseAPIForgeType(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}
