# Minimal Client Config Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow the CLI client to operate against a remote/daemon server without requiring the full configuration file locally.

**Architecture:** Two-phase config loading with a `ConfigResolver` that fetches server config on demand. `NewSession()` is refactored to skip `App` creation for remote/daemon targets. Auth middleware gates TCP access with bearer tokens while Unix sockets remain trusted. New `server_address`, `server_token`, and `auth_token` config fields wire the discovery and auth chains.

**Tech Stack:** Go, chi router, Huma API framework, Viper config, `crypto/rand` for token generation.

---

## File Structure

### New files

| File | Responsibility |
|------|---------------|
| `internal/adapter/primary/runtime/config_resolver.go` | `ConfigResolver` struct: lazy config resolution from local or remote server |
| `internal/adapter/primary/runtime/config_resolver_test.go` | Tests for `ConfigResolver` |
| `internal/adapter/primary/api/auth_middleware.go` | Bearer token auth middleware for TCP connections |
| `internal/adapter/primary/api/auth_middleware_test.go` | Tests for auth middleware |
| `internal/adapter/primary/frontend/config_dto_convert.go` | `DTOToConfig()` reverse conversion |
| `internal/adapter/primary/frontend/config_dto_convert_test.go` | Tests for DTO→Config conversion |

### Modified files

| File | Changes |
|------|---------|
| `internal/config/config.go` | Add `ServerAddress`, `ServerToken`, `AuthToken` fields to `Config` struct |
| `pkg/dto/v1/config.go` | Add matching DTO fields |
| `pkg/client/client.go` | Add `NewClientWithToken()` constructor; inject `Authorization` header in `do()` |
| `pkg/client/client_test.go` | Test token injection |
| `internal/adapter/primary/runtime/runtime.go` | Refactor `NewSession()`, `Session` struct, `useRemoteTarget()`, `useDaemonTarget()` |
| `internal/adapter/primary/runtime/runtime_test.go` | Update session tests for new flow |
| `internal/adapter/primary/cli/root.go` | Use `session.GetConfig(ctx)` instead of `session.Config` |
| `internal/adapter/primary/cli/serve.go` | Wire auth middleware and token generation |
| `internal/adapter/primary/api/server.go` | Add `TransportKind` context key, dual-listener support |
| `internal/adapter/primary/frontend/config_server_workflow.go` | Export existing `ConfigToDTO` (already exported) |

---

### Task 1: Add client config fields to config struct

**Files:**
- Modify: `internal/config/config.go:410-422`
- Modify: `pkg/dto/v1/config.go:314-325`
- Test: `internal/config/config_test.go` (existing)

- [ ] **Step 1: Write a test that loads config with new fields**

In `internal/config/config_test.go`, add:

```go
func TestLoad_ClientFields(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `
server_address: "http://remote:8472"
server_token: "secret-token"
auth_token: "server-auth-token"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ServerAddress != "http://remote:8472" {
		t.Fatalf("ServerAddress = %q, want %q", cfg.ServerAddress, "http://remote:8472")
	}
	if cfg.ServerToken != "secret-token" {
		t.Fatalf("ServerToken = %q, want %q", cfg.ServerToken, "secret-token")
	}
	if cfg.AuthToken != "server-auth-token" {
		t.Fatalf("AuthToken = %q, want %q", cfg.AuthToken, "server-auth-token")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/config/ -run TestLoad_ClientFields -v`
Expected: FAIL — `cfg.ServerAddress` undefined

- [ ] **Step 3: Add the fields to the Config struct**

In `internal/config/config.go`, add three fields to the `Config` struct after `Collaborators`:

```go
type Config struct {
	Launchpad     LaunchpadConfig           `mapstructure:"launchpad" yaml:"launchpad"`
	GitHub        GitHubConfig              `mapstructure:"github" yaml:"github"`
	Gerrit        GerritConfig              `mapstructure:"gerrit" yaml:"gerrit"`
	BugGroups     map[string]BugGroupConfig `mapstructure:"bug_groups" yaml:"bug_groups,omitempty"`
	Projects      []ProjectConfig           `mapstructure:"projects" yaml:"projects"`
	Build         BuildConfig               `mapstructure:"build" yaml:"build"`
	Releases      ReleasesConfig            `mapstructure:"releases" yaml:"releases,omitempty"`
	Packages      PackagesConfig            `mapstructure:"packages" yaml:"packages,omitempty"`
	TUI           TUIConfig                 `mapstructure:"tui" yaml:"tui,omitempty"`
	OTel          OTelConfig                `mapstructure:"otel" yaml:"otel,omitempty"`
	Collaborators *CollaboratorsConfig      `mapstructure:"collaborators" yaml:"collaborators,omitempty"`

	// Client-side fields (not needed by the server).
	ServerAddress string `mapstructure:"server_address" yaml:"server_address,omitempty"`
	ServerToken   string `mapstructure:"server_token" yaml:"server_token,omitempty"`

	// Server-side auth field.
	AuthToken string `mapstructure:"auth_token" yaml:"auth_token,omitempty"`
}
```

- [ ] **Step 4: Add matching DTO fields**

In `pkg/dto/v1/config.go`, add to the `Config` struct:

```go
	ServerAddress string `json:"server_address,omitempty" yaml:"server_address,omitempty"`
	ServerToken   string `json:"server_token,omitempty" yaml:"server_token,omitempty"`
	AuthToken     string `json:"auth_token,omitempty" yaml:"auth_token,omitempty"`
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/config/ -run TestLoad_ClientFields -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go pkg/dto/v1/config.go internal/config/config_test.go
git commit -m "feat(config): add server_address, server_token, auth_token fields

New client-side fields for remote server discovery and authentication.
server_address slots into the target resolution chain between env var
and socket discovery. server_token is the client's bearer token.
auth_token is the server-side token for gating TCP access."
```

---

### Task 2: Add bearer token support to HTTP client

**Files:**
- Modify: `pkg/client/client.go:19-51`
- Create: `pkg/client/client_test.go`

- [ ] **Step 1: Write failing test for token injection**

Create `pkg/client/client_test.go`:

```go
package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClientWithToken_InjectsAuthorizationHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	c := NewClientWithToken(srv.URL, "test-token-123")

	err := c.get(context.Background(), "/api/v1/health", nil, &healthResponse{})
	if err != nil {
		t.Fatalf("get() error = %v", err)
	}
	if gotAuth != "Bearer test-token-123" {
		t.Fatalf("Authorization header = %q, want %q", gotAuth, "Bearer test-token-123")
	}
}

func TestNewClient_NoAuthorizationHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)

	err := c.get(context.Background(), "/api/v1/health", nil, &healthResponse{})
	if err != nil {
		t.Fatalf("get() error = %v", err)
	}
	if gotAuth != "" {
		t.Fatalf("Authorization header = %q, want empty", gotAuth)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./pkg/client/ -run TestNewClientWithToken -v`
Expected: FAIL — `NewClientWithToken` undefined

- [ ] **Step 3: Add token field and constructor**

In `pkg/client/client.go`, add a `token` field to `Client` and a new constructor:

```go
// Client is a typed HTTP client for the Sunbeam Watchtower API server.
type Client struct {
	baseURL string
	http    *http.Client
	token   string
}
```

Add `NewClientWithToken` after `NewClient`:

```go
// NewClientWithToken creates a new Client that attaches a bearer token to every
// request. Use this when connecting to a network-exposed server that requires
// authentication.
func NewClientWithToken(addr, token string) *Client {
	c := NewClient(addr)
	c.token = token
	return c
}
```

In the `do` method, inject the header before executing the request:

```go
func (c *Client) do(req *http.Request, result interface{}) error {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/client/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/client/client.go pkg/client/client_test.go
git commit -m "feat(client): add NewClientWithToken for bearer auth

Adds a token field to Client and a NewClientWithToken constructor that
injects Authorization: Bearer <token> on every request via the do()
method."
```

---

### Task 3: Add auth middleware for TCP connections

**Files:**
- Create: `internal/adapter/primary/api/auth_middleware.go`
- Create: `internal/adapter/primary/api/auth_middleware_test.go`
- Modify: `internal/adapter/primary/api/server.go:47-56`

- [ ] **Step 1: Write failing test for middleware**

Create `internal/adapter/primary/api/auth_middleware_test.go`:

```go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBearerAuthMiddleware_AllowsUnixSocket(t *testing.T) {
	handler := BearerAuthMiddleware("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req = req.WithContext(context.WithValue(req.Context(), TransportKindKey, TransportUnix))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for unix socket, got %d", rec.Code)
	}
}

func TestBearerAuthMiddleware_RejectsTCPWithoutToken(t *testing.T) {
	handler := BearerAuthMiddleware("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	req = req.WithContext(context.WithValue(req.Context(), TransportKindKey, TransportTCP))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for TCP without token, got %d", rec.Code)
	}

	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Error != "authentication required" {
		t.Fatalf("error = %q, want %q", body.Error, "authentication required")
	}
}

func TestBearerAuthMiddleware_AllowsTCPWithValidToken(t *testing.T) {
	handler := BearerAuthMiddleware("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	req = req.WithContext(context.WithValue(req.Context(), TransportKindKey, TransportTCP))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for TCP with valid token, got %d", rec.Code)
	}
}

func TestBearerAuthMiddleware_RejectsTCPWithWrongToken(t *testing.T) {
	handler := BearerAuthMiddleware("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	req = req.WithContext(context.WithValue(req.Context(), TransportKindKey, TransportTCP))
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for TCP with wrong token, got %d", rec.Code)
	}
}

func TestBearerAuthMiddleware_HealthExempt(t *testing.T) {
	handler := BearerAuthMiddleware("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req = req.WithContext(context.WithValue(req.Context(), TransportKindKey, TransportTCP))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for health endpoint without token, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/adapter/primary/api/ -run TestBearerAuthMiddleware -v`
Expected: FAIL — `BearerAuthMiddleware` undefined

- [ ] **Step 3: Implement auth middleware**

Create `internal/adapter/primary/api/auth_middleware.go`:

```go
// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
)

// TransportKind identifies the listener that accepted a connection.
type TransportKind string

const (
	TransportUnix TransportKind = "unix"
	TransportTCP  TransportKind = "tcp"
)

// contextKey is an unexported type for context keys in this package.
type contextKey string

// TransportKindKey is the context key for the transport kind.
const TransportKindKey contextKey = "transport_kind"

// BearerAuthMiddleware returns chi-compatible middleware that enforces bearer
// token authentication on TCP connections. Unix socket connections and the
// health endpoint are exempt.
func BearerAuthMiddleware(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			transport, _ := r.Context().Value(TransportKindKey).(TransportKind)

			// Unix socket connections are trusted.
			if transport == TransportUnix {
				next.ServeHTTP(w, r)
				return
			}

			// Health endpoint is always exempt for liveness probes.
			if r.URL.Path == "/api/v1/health" {
				next.ServeHTTP(w, r)
				return
			}

			// Extract bearer token from Authorization header.
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				writeUnauthorized(w)
				return
			}
			provided := strings.TrimPrefix(auth, "Bearer ")

			if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
				writeUnauthorized(w)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"error": "authentication required"})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/adapter/primary/api/ -run TestBearerAuthMiddleware -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/primary/api/auth_middleware.go internal/adapter/primary/api/auth_middleware_test.go
git commit -m "feat(api): add bearer token auth middleware for TCP connections

Unix socket connections pass through (OS-level trust). TCP connections
require Authorization: Bearer <token>. Health endpoint is exempt for
liveness probes. Uses constant-time comparison."
```

---

### Task 4: Wire transport-kind context tagging in the server

**Files:**
- Modify: `internal/adapter/primary/api/server.go:47-56,107-149`

- [ ] **Step 1: Write a test that verifies transport context tagging**

Add to `internal/adapter/primary/api/server_test.go`:

```go
func TestServer_TCPSetsTransportContext(t *testing.T) {
	var gotTransport TransportKind
	srv := NewServer(discardLogger(), ServerOptions{
		ListenAddr: "127.0.0.1:0",
		Middleware: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotTransport, _ = r.Context().Value(TransportKindKey).(TransportKind)
				next.ServeHTTP(w, r)
			})
		},
	})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown(context.Background())

	resp, err := http.Get("http://" + srv.Addr() + "/api/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if gotTransport != TransportTCP {
		t.Fatalf("transport = %q, want %q", gotTransport, TransportTCP)
	}
}

func TestServer_UnixSetsTransportContext(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "test.sock")

	var gotTransport TransportKind
	srv := NewServer(discardLogger(), ServerOptions{
		UnixSocket: sock,
		Middleware: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotTransport, _ = r.Context().Value(TransportKindKey).(TransportKind)
				next.ServeHTTP(w, r)
			})
		},
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
	resp.Body.Close()

	if gotTransport != TransportUnix {
		t.Fatalf("transport = %q, want %q", gotTransport, TransportUnix)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/adapter/primary/api/ -run TestServer_TCPSetsTransportContext -v`
Expected: FAIL — `gotTransport` is empty

- [ ] **Step 3: Add transport-kind context tagging to the server**

In `internal/adapter/primary/api/server.go`, add a `transportKind` field to `Server` and inject it into request context. Modify `Start()` to record the transport kind, and add a context-injecting middleware that runs before all others:

```go
// Server is the HTTP server for Sunbeam Watchtower.
type Server struct {
	router        chi.Router
	api           huma.API
	logger        *slog.Logger
	listener      net.Listener
	httpSrv       *http.Server
	opts          ServerOptions
	transportKind TransportKind
}
```

In `NewServer`, inject a transport-kind middleware as the very first middleware (before user-provided middleware):

```go
func NewServer(logger *slog.Logger, opts ServerOptions) *Server {
	router := chi.NewMux()

	// Determine transport kind from options. Default to TCP.
	transportKind := TransportTCP
	if opts.UnixSocket != "" {
		transportKind = TransportUnix
	}

	// Inject transport kind into request context as the first middleware.
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), TransportKindKey, transportKind)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})

	if opts.Middleware != nil {
		router.Use(opts.Middleware)
	}
	// ... rest unchanged
```

Also add `"context"` to the imports.

Store `transportKind` on the server struct:

```go
	s := &Server{
		router:        router,
		api:           api,
		logger:        logger,
		opts:          opts,
		transportKind: transportKind,
	}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/adapter/primary/api/ -run TestServer_.*TransportContext -v`
Expected: PASS

- [ ] **Step 5: Run all existing server tests to verify no regressions**

Run: `go test ./internal/adapter/primary/api/ -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/primary/api/server.go internal/adapter/primary/api/server_test.go
git commit -m "feat(api): tag request context with transport kind

Each request gets TransportKindKey in its context, set to TransportUnix
or TransportTCP based on the listener. This enables the auth middleware
to distinguish trusted local connections from network connections."
```

---

### Task 5: Wire auth middleware into serve command

**Files:**
- Modify: `internal/adapter/primary/cli/serve.go`
- Modify: `internal/adapter/primary/runtime/runtime.go:405-432`

- [ ] **Step 1: Write a test for token generation and file writing**

Add to `internal/adapter/primary/api/auth_middleware_test.go`:

```go
func TestGenerateToken(t *testing.T) {
	token, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	if len(token) != 64 { // 32 bytes hex-encoded
		t.Fatalf("token length = %d, want 64", len(token))
	}

	token2, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	if token == token2 {
		t.Fatal("two generated tokens should not be equal")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/adapter/primary/api/ -run TestGenerateToken -v`
Expected: FAIL — `GenerateToken` undefined

- [ ] **Step 3: Add GenerateToken to auth_middleware.go**

Append to `internal/adapter/primary/api/auth_middleware.go`:

```go
import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
)

// GenerateToken creates a cryptographically random 32-byte hex-encoded token.
func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
```

(Update the existing import block to include `"crypto/rand"` and `"encoding/hex"`.)

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/adapter/primary/api/ -run TestGenerateToken -v`
Expected: PASS

- [ ] **Step 5: Add AuthToken field to ServerOptions**

In `internal/adapter/primary/api/server.go`, add to `ServerOptions`:

```go
type ServerOptions struct {
	ListenAddr string
	UnixSocket string
	Middleware func(http.Handler) http.Handler
	// AuthToken, if non-empty, enables bearer auth middleware on TCP connections.
	AuthToken string
}
```

In `NewServer`, wire the auth middleware when a token is configured:

```go
	// If auth token is configured and this is a TCP server, add auth middleware.
	if opts.AuthToken != "" && opts.UnixSocket == "" {
		router.Use(BearerAuthMiddleware(opts.AuthToken))
	}

	if opts.Middleware != nil {
		router.Use(opts.Middleware)
	}
```

- [ ] **Step 6: Wire token resolution in serve command**

In `internal/adapter/primary/cli/serve.go`, resolve the auth token from config or generate one, and pass it to `ServerOptions`. Modify the `RunE` function:

After building `serverOpts` (around line 35), add:

```go
			// Resolve auth token for TCP listeners.
			if serverOpts.UnixSocket == "" {
				authToken := opts.Application().GetConfig().AuthToken
				if authToken == "" {
					generated, err := api.GenerateToken()
					if err != nil {
						return fmt.Errorf("generating auth token: %w", err)
					}
					authToken = generated

					// Write to well-known path for local clients.
					tokenDir, err := os.UserHomeDir()
					if err == nil {
						tokenPath := filepath.Join(tokenDir, ".config", "sunbeam-watchtower", "server.token")
						if err := os.MkdirAll(filepath.Dir(tokenPath), 0o755); err == nil {
							if err := os.WriteFile(tokenPath, []byte(authToken), 0o600); err != nil {
								opts.Logger.Warn("failed to write server token file", "error", err)
							} else {
								opts.Logger.Info("auth token written", "path", tokenPath)
							}
						}
					}
				}
				serverOpts.AuthToken = authToken
				opts.Logger.Info("TCP authentication enabled")
			}
```

Add `"path/filepath"` to the import block.

- [ ] **Step 7: Run existing serve-related tests**

Run: `go test ./internal/adapter/primary/cli/ -v -count=1`
Expected: All PASS

- [ ] **Step 8: Commit**

```bash
git add internal/adapter/primary/api/auth_middleware.go internal/adapter/primary/api/auth_middleware_test.go internal/adapter/primary/api/server.go internal/adapter/primary/cli/serve.go
git commit -m "feat(serve): wire bearer auth on TCP listeners

When the server listens on TCP, it either uses auth_token from config
or generates a random token and writes it to ~/.config/sunbeam-watchtower/server.token.
The BearerAuthMiddleware rejects unauthenticated TCP requests with 401."
```

---

### Task 6: Build DTOToConfig reverse conversion

**Files:**
- Create: `internal/adapter/primary/frontend/config_dto_convert.go`
- Create: `internal/adapter/primary/frontend/config_dto_convert_test.go`

- [ ] **Step 1: Write a round-trip test**

Create `internal/adapter/primary/frontend/config_dto_convert_test.go`:

```go
package frontend

import (
	"testing"

	"github.com/canonical/sunbeam-watchtower/internal/config"
)

func TestDTOToConfig_RoundTrip(t *testing.T) {
	original := &config.Config{
		Launchpad: config.LaunchpadConfig{
			DefaultOwner:     "test-owner",
			Series:           []string{"noble", "oracular"},
			DevelopmentFocus: "oracular",
		},
		GitHub: config.GitHubConfig{
			ClientID: "gh-client-id",
		},
		Projects: []config.ProjectConfig{
			{
				Name:         "sunbeam",
				ArtifactType: "charm",
				Code: config.CodeConfig{
					Forge: "github",
					Owner: "canonical",
				},
			},
		},
		Build: config.BuildConfig{
			DefaultPrefix:  "test-prefix",
			TimeoutMinutes: 45,
			ArtifactsDir:   "artifacts",
		},
	}

	dto := ConfigToDTO(original)
	roundTripped := DTOToConfig(dto)

	if roundTripped.Launchpad.DefaultOwner != original.Launchpad.DefaultOwner {
		t.Fatalf("DefaultOwner = %q, want %q", roundTripped.Launchpad.DefaultOwner, original.Launchpad.DefaultOwner)
	}
	if roundTripped.Launchpad.DevelopmentFocus != original.Launchpad.DevelopmentFocus {
		t.Fatalf("DevelopmentFocus = %q, want %q", roundTripped.Launchpad.DevelopmentFocus, original.Launchpad.DevelopmentFocus)
	}
	if len(roundTripped.Projects) != 1 {
		t.Fatalf("len(Projects) = %d, want 1", len(roundTripped.Projects))
	}
	if roundTripped.Projects[0].Name != "sunbeam" {
		t.Fatalf("Projects[0].Name = %q, want %q", roundTripped.Projects[0].Name, "sunbeam")
	}
	if roundTripped.Build.TimeoutMinutes != 45 {
		t.Fatalf("Build.TimeoutMinutes = %d, want 45", roundTripped.Build.TimeoutMinutes)
	}
}

func TestDTOToConfig_Nil(t *testing.T) {
	if got := DTOToConfig(nil); got != nil {
		t.Fatalf("DTOToConfig(nil) = %v, want nil", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/adapter/primary/frontend/ -run TestDTOToConfig -v`
Expected: FAIL — `DTOToConfig` undefined

- [ ] **Step 3: Implement DTOToConfig**

Create `internal/adapter/primary/frontend/config_dto_convert.go`:

```go
// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"github.com/canonical/sunbeam-watchtower/internal/config"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// DTOToConfig converts a public DTO config to the internal config type.
// This is the reverse of ConfigToDTO and is used by the ConfigResolver to
// convert server responses into the internal representation.
func DTOToConfig(d *dto.Config) *config.Config {
	if d == nil {
		return nil
	}

	out := &config.Config{
		Launchpad: config.LaunchpadConfig{
			DefaultOwner:     d.Launchpad.DefaultOwner,
			UseKeyring:       d.Launchpad.UseKeyring,
			Series:           append([]string(nil), d.Launchpad.Series...),
			DevelopmentFocus: d.Launchpad.DevelopmentFocus,
		},
		GitHub: config.GitHubConfig{
			UseKeyring: d.GitHub.UseKeyring,
			ClientID:   d.GitHub.ClientID,
		},
		Build: config.BuildConfig{
			DefaultPrefix:  d.Build.DefaultPrefix,
			TimeoutMinutes: d.Build.TimeoutMinutes,
			ArtifactsDir:   d.Build.ArtifactsDir,
		},
		Releases: config.ReleasesConfig{
			DefaultTargetProfile: d.Releases.DefaultTargetProfile,
			TargetProfiles:       make(map[string]config.ReleaseTargetProfileConfig, len(d.Releases.TargetProfiles)),
		},
		TUI: config.TUIConfig{
			DefaultPane: d.TUI.DefaultPane,
		},
		BugGroups:     make(map[string]config.BugGroupConfig, len(d.BugGroups)),
		ServerAddress: d.ServerAddress,
		ServerToken:   d.ServerToken,
		AuthToken:     d.AuthToken,
	}

	// Gerrit hosts
	out.Gerrit.Hosts = make([]config.GerritHost, len(d.Gerrit.Hosts))
	for i, host := range d.Gerrit.Hosts {
		out.Gerrit.Hosts[i] = config.GerritHost{URL: host.URL}
	}

	// Projects
	out.Projects = make([]config.ProjectConfig, len(d.Projects))
	for i, p := range d.Projects {
		outProject := config.ProjectConfig{
			Name:             p.Name,
			ArtifactType:     p.ArtifactType,
			Series:           append([]string(nil), p.Series...),
			DevelopmentFocus: p.DevelopmentFocus,
			Code: config.CodeConfig{
				Forge:   p.Code.Forge,
				Owner:   p.Code.Owner,
				Host:    p.Code.Host,
				Project: p.Code.Project,
				GitURL:  p.Code.GitURL,
			},
		}

		if p.Build != nil {
			outProject.Build = &config.ProjectBuildConfig{
				Owner:          p.Build.Owner,
				Artifacts:      append([]string(nil), p.Build.Artifacts...),
				PrepareCommand: p.Build.PrepareCommand,
			}
		}

		outProject.Bugs = make([]config.BugTrackerConfig, len(p.Bugs))
		for j, bug := range p.Bugs {
			outProject.Bugs[j] = config.BugTrackerConfig{
				Forge:   bug.Forge,
				Owner:   bug.Owner,
				Host:    bug.Host,
				Project: bug.Project,
				Group:   bug.Group,
			}
		}

		if p.Release != nil {
			outProject.Release = &config.ProjectReleaseConfig{
				Tracks:        append([]string(nil), p.Release.Tracks...),
				TrackMap:       make(map[string]string, len(p.Release.TrackMap)),
				SkipArtifacts: append([]string(nil), p.Release.SkipArtifacts...),
				TargetProfile: p.Release.TargetProfile,
			}
			if p.Release.TargetProfileOverrides != nil {
				outProject.Release.TargetProfileOverrides = profileDTOToConfig(p.Release.TargetProfileOverrides)
			}
			for series, track := range p.Release.TrackMap {
				outProject.Release.TrackMap[series] = track
			}
			outProject.Release.Branches = make([]config.ProjectReleaseBranchConfig, len(p.Release.Branches))
			for j, branch := range p.Release.Branches {
				outProject.Release.Branches[j] = config.ProjectReleaseBranchConfig{
					Series: branch.Series,
					Track:  branch.Track,
					Branch: branch.Branch,
					Risks:  append([]string(nil), branch.Risks...),
				}
			}
		}

		out.Projects[i] = outProject
	}

	// Release target profiles
	for name, profile := range d.Releases.TargetProfiles {
		out.Releases.TargetProfiles[name] = *profileDTOToConfig(&profile)
	}

	// Bug groups
	for name, group := range d.BugGroups {
		out.BugGroups[name] = config.BugGroupConfig{
			CommonProject: group.CommonProject,
		}
	}

	// Packages
	if len(d.Packages.Distros) > 0 {
		out.Packages.Distros = make(map[string]config.DistroConfig, len(d.Packages.Distros))
		for name, distro := range d.Packages.Distros {
			outDistro := config.DistroConfig{
				Mirror:     distro.Mirror,
				Components: append([]string(nil), distro.Components...),
				Releases:   make(map[string]config.ReleaseConfig, len(distro.Releases)),
			}
			for releaseName, release := range distro.Releases {
				outRelease := config.ReleaseConfig{
					Suites:    append([]string(nil), release.Suites...),
					Backports: make(map[string]config.BackportConfig, len(release.Backports)),
				}
				for backportName, backport := range release.Backports {
					outBackport := config.BackportConfig{
						ParentRelease: backport.ParentRelease,
						Sources:       make([]config.DistroSourceConfig, len(backport.Sources)),
					}
					for k, source := range backport.Sources {
						outBackport.Sources[k] = config.DistroSourceConfig{
							Mirror:     source.Mirror,
							Suites:     append([]string(nil), source.Suites...),
							Components: append([]string(nil), source.Components...),
						}
					}
					outRelease.Backports[backportName] = outBackport
				}
				outDistro.Releases[releaseName] = outRelease
			}
			out.Packages.Distros[name] = outDistro
		}
	}

	if len(d.Packages.Sets) > 0 {
		out.Packages.Sets = make(map[string][]string, len(d.Packages.Sets))
		for setName, packages := range d.Packages.Sets {
			out.Packages.Sets[setName] = append([]string(nil), packages...)
		}
	}

	if d.Packages.Upstream != nil {
		out.Packages.Upstream = &config.UpstreamConfig{
			Provider:         d.Packages.Upstream.Provider,
			ReleasesRepo:     d.Packages.Upstream.ReleasesRepo,
			RequirementsRepo: d.Packages.Upstream.RequirementsRepo,
		}
	}

	// OTel (preserve for completeness but client rarely needs it)
	out.OTel = config.OTelConfig{
		ServiceName:        d.OTel.ServiceName,
		ServiceNamespace:   d.OTel.ServiceNamespace,
		ResourceAttributes: copyStringMap(d.OTel.ResourceAttributes),
	}

	return out
}

func profileDTOToConfig(profile *dto.ReleaseTargetProfileConfig) *config.ReleaseTargetProfileConfig {
	if profile == nil {
		return nil
	}
	out := &config.ReleaseTargetProfileConfig{
		Include: make([]config.ReleaseTargetMatcherConfig, len(profile.Include)),
		Exclude: make([]config.ReleaseTargetMatcherConfig, len(profile.Exclude)),
	}
	for i, matcher := range profile.Include {
		out.Include[i] = matcherDTOToConfig(matcher)
	}
	for i, matcher := range profile.Exclude {
		out.Exclude[i] = matcherDTOToConfig(matcher)
	}
	return out
}

func matcherDTOToConfig(matcher dto.ReleaseTargetMatcherConfig) config.ReleaseTargetMatcherConfig {
	return config.ReleaseTargetMatcherConfig{
		BaseNames:      append([]string(nil), matcher.BaseNames...),
		BaseChannels:   append([]string(nil), matcher.BaseChannels...),
		MinBaseChannel: matcher.MinBaseChannel,
		Architectures:  append([]string(nil), matcher.Architectures...),
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/adapter/primary/frontend/ -run TestDTOToConfig -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/primary/frontend/config_dto_convert.go internal/adapter/primary/frontend/config_dto_convert_test.go
git commit -m "feat(frontend): add DTOToConfig reverse conversion

Mirrors the existing ConfigToDTO function. Used by the ConfigResolver
to convert server API responses into the internal config type."
```

---

### Task 7: Build the ConfigResolver

**Files:**
- Create: `internal/adapter/primary/runtime/config_resolver.go`
- Create: `internal/adapter/primary/runtime/config_resolver_test.go`

- [ ] **Step 1: Write tests for the ConfigResolver**

Create `internal/adapter/primary/runtime/config_resolver_test.go`:

```go
package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/canonical/sunbeam-watchtower/internal/config"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	"github.com/canonical/sunbeam-watchtower/pkg/client"
)

func TestConfigResolver_LocalOnly(t *testing.T) {
	local := &config.Config{
		Launchpad: config.LaunchpadConfig{DefaultOwner: "local-owner"},
	}
	resolver := NewConfigResolver(local, nil)

	cfg, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if cfg.Launchpad.DefaultOwner != "local-owner" {
		t.Fatalf("DefaultOwner = %q, want %q", cfg.Launchpad.DefaultOwner, "local-owner")
	}
}

func TestConfigResolver_RemoteFetchesAndCaches(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(dto.Config{
			Launchpad: dto.LaunchpadConfig{DefaultOwner: "remote-owner"},
		})
	}))
	defer srv.Close()

	c := client.NewClient(srv.URL)
	resolver := NewConfigResolver(nil, c)

	cfg, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if cfg.Launchpad.DefaultOwner != "remote-owner" {
		t.Fatalf("DefaultOwner = %q, want %q", cfg.Launchpad.DefaultOwner, "remote-owner")
	}

	// Second call should use cache.
	cfg2, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() second call error = %v", err)
	}
	if cfg2.Launchpad.DefaultOwner != "remote-owner" {
		t.Fatalf("second call DefaultOwner = %q", cfg2.Launchpad.DefaultOwner)
	}
	if calls != 1 {
		t.Fatalf("server called %d times, want 1 (cached)", calls)
	}
}

func TestConfigResolver_NeitherSource(t *testing.T) {
	resolver := NewConfigResolver(nil, nil)

	_, err := resolver.Resolve(context.Background())
	if err == nil {
		t.Fatal("Resolve() with no sources should return error")
	}
}

func TestConfigResolver_RemotePreferred(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(dto.Config{
			Launchpad: dto.LaunchpadConfig{DefaultOwner: "remote-owner"},
		})
	}))
	defer srv.Close()

	local := &config.Config{
		Launchpad: config.LaunchpadConfig{DefaultOwner: "local-owner"},
	}
	c := client.NewClient(srv.URL)
	resolver := NewConfigResolver(local, c)

	cfg, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	// When client is set, remote is authoritative.
	if cfg.Launchpad.DefaultOwner != "remote-owner" {
		t.Fatalf("DefaultOwner = %q, want %q (remote preferred)", cfg.Launchpad.DefaultOwner, "remote-owner")
	}
}

func TestConfigResolver_LocalConfig(t *testing.T) {
	local := &config.Config{
		TUI:           config.TUIConfig{DefaultPane: "builds"},
		ServerAddress: "http://remote:8472",
		ServerToken:   "my-token",
	}
	resolver := NewConfigResolver(local, nil)

	cfg := resolver.LocalConfig()
	if cfg.TUI.DefaultPane != "builds" {
		t.Fatalf("TUI.DefaultPane = %q, want %q", cfg.TUI.DefaultPane, "builds")
	}
	if cfg.ServerAddress != "http://remote:8472" {
		t.Fatalf("ServerAddress = %q", cfg.ServerAddress)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/adapter/primary/runtime/ -run TestConfigResolver -v`
Expected: FAIL — `NewConfigResolver` undefined

- [ ] **Step 3: Implement ConfigResolver**

Create `internal/adapter/primary/runtime/config_resolver.go`:

```go
// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"errors"
	"sync"

	frontend "github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	"github.com/canonical/sunbeam-watchtower/internal/config"
	"github.com/canonical/sunbeam-watchtower/pkg/client"
)

// ConfigResolver resolves configuration from a local file, a remote server,
// or both. When a remote client is available, the server's config is authoritative.
// Results are cached for the lifetime of the resolver (one CLI session).
type ConfigResolver struct {
	local  *config.Config
	client *client.Client
	cached *config.Config
	mu     sync.Mutex
}

// NewConfigResolver creates a resolver. Either local or client (or both) may be
// nil, but Resolve() will fail if neither source can provide config.
func NewConfigResolver(local *config.Config, client *client.Client) *ConfigResolver {
	return &ConfigResolver{
		local:  local,
		client: client,
	}
}

// Resolve returns the effective configuration. If a remote client is set, the
// server's config is fetched (once) and returned. Otherwise, the local config
// is returned.
func (r *ConfigResolver) Resolve(ctx context.Context) (*config.Config, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cached != nil {
		return r.cached, nil
	}

	if r.client != nil {
		dtoConfig, err := r.client.ConfigShow(ctx)
		if err != nil {
			return nil, err
		}
		r.cached = frontend.DTOToConfig(dtoConfig)
		return r.cached, nil
	}

	if r.local != nil {
		return r.local, nil
	}

	return nil, errors.New("no configuration source available: provide a config file or connect to a server (--server)")
}

// LocalConfig returns the locally-loaded config for client-side fields (TUI
// preferences, server_token, server_address). Returns nil if no local config
// was loaded.
func (r *ConfigResolver) LocalConfig() *config.Config {
	return r.local
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/adapter/primary/runtime/ -run TestConfigResolver -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/primary/runtime/config_resolver.go internal/adapter/primary/runtime/config_resolver_test.go
git commit -m "feat(runtime): add ConfigResolver for on-demand config fetching

Resolves config from a remote server (preferred when client is set) or
local file. Results are cached per session. LocalConfig() provides
direct access to client-side fields like TUI prefs and server_token."
```

---

### Task 8: Refactor NewSession to skip App for remote/daemon targets

**Files:**
- Modify: `internal/adapter/primary/runtime/runtime.go:447-543,636-677`
- Modify: `internal/adapter/primary/runtime/runtime_test.go`

- [ ] **Step 1: Write test for remote session without config**

Add to `internal/adapter/primary/runtime/runtime_test.go`:

```go
func TestNewSession_RemoteTargetSkipsAppCreation(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	session, err := NewSession(context.Background(), Options{
		ServerAddr:   "http://127.0.0.1:9999",
		LogWriter:    &bytes.Buffer{},
		TargetPolicy: TargetPolicyPreferExistingDaemon,
	})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	defer session.Close()

	if session.App != nil {
		t.Fatal("expected App to be nil for remote target")
	}
	if session.Frontend == nil {
		t.Fatal("expected Frontend to be non-nil")
	}
	if session.Target().Kind != TargetKindRemote {
		t.Fatalf("Target().Kind = %q, want %q", session.Target().Kind, TargetKindRemote)
	}
}

func TestNewSession_RemoteTargetWithToken(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("WATCHTOWER_TOKEN", "my-secret-token")

	session, err := NewSession(context.Background(), Options{
		ServerAddr:   "http://127.0.0.1:9999",
		LogWriter:    &bytes.Buffer{},
		TargetPolicy: TargetPolicyPreferExistingDaemon,
	})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	defer session.Close()

	if session.Target().Kind != TargetKindRemote {
		t.Fatalf("Target().Kind = %q, want %q", session.Target().Kind, TargetKindRemote)
	}
}
```

- [ ] **Step 2: Run the tests to verify the new assertions fail**

Run: `go test ./internal/adapter/primary/runtime/ -run TestNewSession_RemoteTarget -v`
Expected: The `App != nil` assertion fails because the current code always creates an App.

- [ ] **Step 3: Update Session struct and Options**

In `internal/adapter/primary/runtime/runtime.go`, update the `Session` struct to include `ConfigResolver`:

```go
// Session owns the local app plus current API target for a frontend.
type Session struct {
	Config         *ConfigResolver
	Logger         *slog.Logger
	App            *app.App
	Client         *client.Client
	Frontend       *frontend.ClientFacade

	accessMode  AccessMode
	target      TargetInfo
	manager     *LocalServerManager
	embeddedSrv *api.Server
	opts        Options
}

// GetConfig resolves the effective configuration from the best available source.
func (s *Session) GetConfig(ctx context.Context) (*config.Config, error) {
	if s == nil || s.Config == nil {
		return nil, errors.New("no session or config resolver available")
	}
	return s.Config.Resolve(ctx)
}
```

Add `"errors"` to the import block if not already present.

- [ ] **Step 4: Refactor NewSession**

Replace the body of `NewSession()`:

```go
func NewSession(ctx context.Context, opts Options) (*Session, error) {
	ApplyEnvDefaults(&opts)
	if opts.AccessMode == "" {
		opts.AccessMode = AccessModeFull
	}
	logger := opts.Logger
	if logger == nil {
		logger = NewLogger(opts.Verbose, opts.LogWriter)
	}

	// Tolerant config load — missing file is okay.
	cfg, _ := config.Load(opts.ConfigPath)

	// Resolve server address from config if not provided via flag/env.
	if opts.ServerAddr == "" && cfg != nil && cfg.ServerAddress != "" {
		opts.ServerAddr = cfg.ServerAddress
	}

	// Resolve token from config or env.
	token := os.Getenv("WATCHTOWER_TOKEN")
	if token == "" && cfg != nil {
		token = cfg.ServerToken
	}

	session := &Session{
		Logger:     logger,
		accessMode: opts.AccessMode,
		opts:       opts,
	}

	// Fast path: explicit remote target — skip App and LocalServerManager.
	if opts.ServerAddr != "" {
		if token != "" {
			session.Client = client.NewClientWithToken(opts.ServerAddr, token)
		} else {
			session.Client = client.NewClient(opts.ServerAddr)
		}
		session.Config = NewConfigResolver(cfg, session.Client)
		session.target = TargetInfo{
			Kind:        TargetKindRemote,
			Address:     opts.ServerAddr,
			Remote:      true,
			Description: "remote server",
		}
		session.Frontend = frontend.NewClientFacade(frontend.NewClientTransport(session.Client), nil)
		return session, nil
	}

	// Local path: need manager to discover/start daemon.
	manager, err := NewLocalServerManager(Options{
		ConfigPath:     opts.ConfigPath,
		ServerAddr:     opts.ServerAddr,
		Verbose:        opts.Verbose,
		Logger:         logger,
		LogWriter:      opts.LogWriter,
		ExecutablePath: opts.ExecutablePath,
	})
	if err != nil {
		return nil, err
	}
	session.manager = manager

	status, err := manager.Status(ctx)
	if err != nil {
		return nil, err
	}

	switch opts.TargetPolicy {
	case TargetPolicyPreferEmbedded:
		// Embedded mode needs a full local config and App.
		if cfg == nil {
			return nil, errors.New("embedded mode requires a configuration file")
		}
		application := app.NewAppWithOptions(cfg, logger, app.Options{RuntimeMode: app.RuntimeModeEphemeral, ConfigPath: opts.ConfigPath})
		session.App = application
		session.Config = NewConfigResolver(cfg, nil)
		if err := session.startEmbeddedTarget(); err != nil {
			_ = application.Close()
			return nil, err
		}

	case TargetPolicyPreferExistingDaemon:
		if status.Running {
			session.useDaemonTarget(status, token)
			session.Config = NewConfigResolver(cfg, session.Client)
			return session, nil
		}
		// Fall back to embedded.
		if cfg == nil {
			return nil, errors.New("no running daemon found and embedded mode requires a configuration file")
		}
		application := app.NewAppWithOptions(cfg, logger, app.Options{RuntimeMode: app.RuntimeModeEphemeral, ConfigPath: opts.ConfigPath})
		session.App = application
		session.Config = NewConfigResolver(cfg, nil)
		if err := session.startEmbeddedTarget(); err != nil {
			_ = application.Close()
			return nil, err
		}

	case TargetPolicyRequirePersistent:
		if status.Running {
			session.useDaemonTarget(status, token)
			session.Config = NewConfigResolver(cfg, session.Client)
			return session, nil
		}
		if cfg == nil {
			return nil, errors.New("cannot auto-start daemon without full configuration — provide a complete config file or connect to an existing server (--server)")
		}
		application := app.NewAppWithOptions(cfg, logger, app.Options{RuntimeMode: app.RuntimeModeEphemeral, ConfigPath: opts.ConfigPath})
		session.App = application
		status, _, err = manager.EnsureRunning(ctx)
		if err != nil {
			_ = application.Close()
			return nil, err
		}
		session.useDaemonTarget(status, token)
		session.Config = NewConfigResolver(cfg, session.Client)

	default:
		return nil, fmt.Errorf("runtime session requires a target policy")
	}

	return session, nil
}
```

- [ ] **Step 5: Update useRemoteTarget and useDaemonTarget**

Remove `useRemoteTarget` (logic moved into NewSession). Update `useDaemonTarget` to accept a token:

```go
func (s *Session) useDaemonTarget(status LocalServerStatus, token string) {
	if token != "" {
		s.Client = client.NewClientWithToken(status.Address, token)
	} else {
		s.Client = client.NewClient(status.Address)
	}
	s.target = TargetInfo{
		Kind:        TargetKindDaemon,
		Address:     status.Address,
		LogFile:     status.LogFile,
		ConfigPath:  status.ConfigPath,
		StartedAt:   status.StartedAt,
		PID:         status.PID,
		CanUpgrade:  false,
		Description: "local persistent daemon",
	}
	s.Frontend = frontend.NewClientFacade(frontend.NewClientTransport(s.Client), s.App)
}
```

Update `startEmbeddedTarget` to wire the Config resolver:

```go
func (s *Session) startEmbeddedTarget() error {
	srv := NewConfiguredServer(s.Logger, s.App, api.ServerOptions{ListenAddr: "127.0.0.1:0"})
	if err := srv.Start(); err != nil {
		return err
	}
	s.embeddedSrv = srv
	s.Client = client.NewClient("http://" + srv.Addr())
	s.target = TargetInfo{
		Kind:        TargetKindEmbedded,
		Address:     "http://" + srv.Addr(),
		CanUpgrade:  true,
		Description: "embedded session server",
	}
	s.Frontend = frontend.NewClientFacade(frontend.NewClientTransport(s.Client), s.App)
	return nil
}
```

- [ ] **Step 6: Run all session tests**

Run: `go test ./internal/adapter/primary/runtime/ -v`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add internal/adapter/primary/runtime/runtime.go internal/adapter/primary/runtime/runtime_test.go
git commit -m "feat(runtime): refactor NewSession for minimal client config

Remote and daemon targets skip App creation and use ConfigResolver for
on-demand config. Token from WATCHTOWER_TOKEN or server_token config
field is injected into the client. Embedded mode still requires a
local config file."
```

---

### Task 9: Update CLI root to use Session.GetConfig

**Files:**
- Modify: `internal/adapter/primary/cli/root.go:110-144`

- [ ] **Step 1: Update opts.config assignment**

In `internal/adapter/primary/cli/root.go`, change line 125 from:

```go
opts.config = session.Config
```

to:

```go
// Config will be resolved on demand via session.GetConfig(ctx).
// For commands that need config eagerly (serve), it's loaded separately.
opts.Session = session
opts.ServerAddr = session.Target().Address
return nil
```

Wait — looking at the current code more carefully, `opts.config` is used in the `commandNeedsConfig` path (line 130-136) and the `commandNeedsApp` path (line 138-143). The session path (line 110-128) sets both `opts.config` and `opts.Session`. We need `opts.config` to remain available for the session path.

The cleanest change: resolve config lazily when needed rather than eagerly. Replace:

```go
				opts.Session = session
				opts.config = session.Config
				opts.ServerAddr = session.Target().Address
				return nil
```

with:

```go
				opts.Session = session
				opts.ServerAddr = session.Target().Address
				return nil
```

Then ensure any code that reads `opts.config` goes through the session instead. Check what reads `opts.config` — if it's only used in the `commandNeedsConfig` branch (which is for `serve`), the session path doesn't need it at all.

- [ ] **Step 2: Verify what uses opts.config**

Search the CLI package for `opts.config` usage. The key question is: does any session-using command read `opts.config` directly? If yes, those need to call `session.GetConfig(ctx)` instead.

Grep for `opts\.config` and `opts\.Config()` in `internal/adapter/primary/cli/`.

- [ ] **Step 3: Update all opts.config readers in session-using commands**

For each command that accesses `opts.config` and also uses a session, change it to call `opts.Session.GetConfig(cmd.Context())` instead.

If `opts.config` is only used in the `commandNeedsConfig` / `commandNeedsApp` branch (for `serve`), then simply removing the assignment from the session branch is sufficient.

- [ ] **Step 4: Run CLI tests**

Run: `go test ./internal/adapter/primary/cli/ -v -count=1`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/primary/cli/root.go
git commit -m "refactor(cli): remove eager config assignment from session path

Session-using commands resolve config on demand via
session.GetConfig(ctx). The opts.config field is only populated in the
serve/config-only path where local config is always required."
```

---

### Task 10: Update TUI session.Config references

**Files:**
- Modify: `internal/adapter/primary/tui/views_extra.go`
- Modify: `internal/adapter/primary/tui/model.go`

- [ ] **Step 1: Audit all session.Config accesses in TUI**

The TUI accesses `session.Config` directly in multiple places for things like project lists, package sets, and release profiles. Since TUI always has a session, these need to call `session.GetConfig(ctx)` instead.

However, the TUI runs interactively and many of these accesses happen in Bubble Tea `Update()` or view helpers where a `context.Context` isn't readily available. The pragmatic approach: resolve config once at TUI startup and cache it in the TUI model.

- [ ] **Step 2: Add a config field to the TUI model**

In the TUI model initialization (wherever the session is first available), call:

```go
cfg, err := session.GetConfig(context.Background())
if err != nil {
    // handle error — TUI cannot start without config
}
```

Store `cfg` in the model struct and replace all `session.Config.X` with `model.cfg.X`.

- [ ] **Step 3: Replace all session.Config references**

Replace each `session.Config.Projects` / `session.Config.Packages.Sets` / etc. with the model's cached config. This is mechanical — each reference is the same pattern.

- [ ] **Step 4: Run TUI tests**

Run: `go test ./internal/adapter/primary/tui/ -v -count=1`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/primary/tui/views_extra.go internal/adapter/primary/tui/model.go
git commit -m "refactor(tui): resolve config via session.GetConfig at startup

Replace direct session.Config field access with a config resolved once
at TUI initialization. This supports the ConfigResolver flow where
config may come from a remote server."
```

---

### Task 11: Wire server_address into target discovery chain

**Files:**
- Modify: `internal/adapter/primary/runtime/runtime.go` (ApplyEnvDefaults)

- [ ] **Step 1: Write a test for server_address resolution**

Add to `internal/adapter/primary/runtime/runtime_test.go`:

```go
func TestNewSession_ServerAddressFromConfig(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `server_address: "http://config-server:8472"`
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	session, err := NewSession(context.Background(), Options{
		ConfigPath:   cfgPath,
		LogWriter:    &bytes.Buffer{},
		TargetPolicy: TargetPolicyPreferExistingDaemon,
	})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	defer session.Close()

	if session.Target().Kind != TargetKindRemote {
		t.Fatalf("Target().Kind = %q, want %q", session.Target().Kind, TargetKindRemote)
	}
	if session.Target().Address != "http://config-server:8472" {
		t.Fatalf("Target().Address = %q, want %q", session.Target().Address, "http://config-server:8472")
	}
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./internal/adapter/primary/runtime/ -run TestNewSession_ServerAddressFromConfig -v`
Expected: PASS — this should already work because Task 8 added the `server_address` resolution in NewSession.

If it fails, the resolution logic in the refactored NewSession needs adjustment.

- [ ] **Step 3: Commit (if changes were needed)**

```bash
git add internal/adapter/primary/runtime/runtime_test.go
git commit -m "test(runtime): verify server_address config field in target discovery"
```

---

### Task 12: Add actionable error messages

**Files:**
- Modify: `internal/adapter/primary/runtime/config_resolver.go`
- Modify: `internal/adapter/primary/runtime/runtime.go`

- [ ] **Step 1: Write tests for specific error messages**

Add to `internal/adapter/primary/runtime/config_resolver_test.go`:

```go
func TestConfigResolver_RemoteServerUnreachable(t *testing.T) {
	c := client.NewClient("http://127.0.0.1:1") // unreachable port
	resolver := NewConfigResolver(nil, c)

	_, err := resolver.Resolve(context.Background())
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
	// Should contain the address for user diagnosis.
	if !strings.Contains(err.Error(), "127.0.0.1:1") {
		t.Fatalf("error should mention server address, got: %v", err)
	}
}
```

Add `"strings"` to imports.

- [ ] **Step 2: Run the test**

Run: `go test ./internal/adapter/primary/runtime/ -run TestConfigResolver_RemoteServerUnreachable -v`
Expected: PASS (the client error already includes the address). If the error message is not descriptive enough, wrap it.

- [ ] **Step 3: Enhance error wrapping in ConfigResolver if needed**

In `config_resolver.go`, wrap the remote fetch error:

```go
	if r.client != nil {
		dtoConfig, err := r.client.ConfigShow(ctx)
		if err != nil {
			return nil, fmt.Errorf("could not fetch configuration from server: %w", err)
		}
```

Add `"fmt"` to imports.

- [ ] **Step 4: Verify error messages in NewSession for auto-start failure**

The error message for `RequirePersistent` with no config was already added in Task 8:
```
"cannot auto-start daemon without full configuration — provide a complete config file or connect to an existing server (--server)"
```

- [ ] **Step 5: Run all runtime tests**

Run: `go test ./internal/adapter/primary/runtime/ -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/adapter/primary/runtime/config_resolver.go internal/adapter/primary/runtime/config_resolver_test.go
git commit -m "feat(runtime): add actionable error messages for config resolution

Wrap remote fetch errors with context. Error messages guide users
to provide config files or connect to servers as appropriate."
```

---

### Task 13: Handle 401 errors on the client side

**Files:**
- Modify: `pkg/client/client.go`
- Test: `pkg/client/client_test.go`

- [ ] **Step 1: Write a test for 401 error handling**

Add to `pkg/client/client_test.go`:

```go
func TestClient_401ProducesAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"authentication required"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	err := c.get(context.Background(), "/api/v1/config", nil, &struct{}{})
	if err == nil {
		t.Fatal("expected error for 401")
	}

	var authErr *AuthRequiredError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *AuthRequiredError, got %T: %v", err, err)
	}
	if authErr.ServerAddr != srv.URL {
		t.Fatalf("ServerAddr = %q, want %q", authErr.ServerAddr, srv.URL)
	}
}
```

Add `"errors"` to imports.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./pkg/client/ -run TestClient_401 -v`
Expected: FAIL — `AuthRequiredError` undefined

- [ ] **Step 3: Add AuthRequiredError type and detection**

In `pkg/client/client.go`, add:

```go
// AuthRequiredError is returned when the server responds with 401.
type AuthRequiredError struct {
	ServerAddr string
}

func (e *AuthRequiredError) Error() string {
	return fmt.Sprintf("server at %s requires authentication — set WATCHTOWER_TOKEN or add server_token to your config", e.ServerAddr)
}
```

In the `do` method, detect 401 specifically:

```go
func (c *Client) do(req *http.Request, result interface{}) error {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return &AuthRequiredError{ServerAddr: c.baseURL}
	}

	if resp.StatusCode >= 400 {
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./pkg/client/ -run TestClient_401 -v`
Expected: PASS

- [ ] **Step 5: Run all client tests**

Run: `go test ./pkg/client/ -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/client/client.go pkg/client/client_test.go
git commit -m "feat(client): return AuthRequiredError on 401 responses

Provides an actionable error message guiding users to set
WATCHTOWER_TOKEN or add server_token to their config file."
```

---

### Task 14: Update ConfigToDTO to include new fields

**Files:**
- Modify: `internal/adapter/primary/frontend/config_server_workflow.go`

- [ ] **Step 1: Write a test for new fields in ConfigToDTO**

Add to an existing test file or `config_dto_convert_test.go`:

```go
func TestConfigToDTO_IncludesClientFields(t *testing.T) {
	cfg := &config.Config{
		ServerAddress: "http://remote:8472",
		ServerToken:   "token",
		AuthToken:     "auth",
	}
	dto := ConfigToDTO(cfg)
	if dto.ServerAddress != "http://remote:8472" {
		t.Fatalf("ServerAddress = %q", dto.ServerAddress)
	}
	if dto.ServerToken != "token" {
		t.Fatalf("ServerToken = %q", dto.ServerToken)
	}
	if dto.AuthToken != "auth" {
		t.Fatalf("AuthToken = %q", dto.AuthToken)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/adapter/primary/frontend/ -run TestConfigToDTO_IncludesClientFields -v`
Expected: FAIL — fields not set in DTO output

- [ ] **Step 3: Update ConfigToDTO**

In `config_server_workflow.go`, in the `ConfigToDTO` function, add the new fields to the `out` struct initialization:

```go
	out := &dto.Config{
		// ... existing fields ...
		ServerAddress: cfg.ServerAddress,
		ServerToken:   cfg.ServerToken,
		AuthToken:     cfg.AuthToken,
	}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/adapter/primary/frontend/ -run TestConfigToDTO_IncludesClientFields -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/primary/frontend/config_server_workflow.go internal/adapter/primary/frontend/config_dto_convert_test.go
git commit -m "feat(frontend): include client/auth fields in ConfigToDTO

Maps server_address, server_token, and auth_token through the
DTO conversion so the config API endpoint exposes all fields."
```

---

### Task 15: Sync PLAN.md

**Files:**
- Modify: `PLAN.md`

- [ ] **Step 1: Update PLAN.md**

Add the minimal client config feature status to the appropriate section in `PLAN.md`, following the existing format. Mark it as implemented and note:
- Two-phase config loading
- ConfigResolver for remote config fetching
- Bearer token auth on TCP
- server_address / server_token / auth_token config fields
- Graceful degradation in embedded mode

- [ ] **Step 2: Commit**

```bash
git add PLAN.md
git commit -m "docs: sync PLAN.md with minimal client config implementation"
```
