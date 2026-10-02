// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/canonical/sunbeam-watchtower/internal/config"
	"github.com/canonical/sunbeam-watchtower/pkg/client"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func TestConfigResolver_LocalOnly(t *testing.T) {
	local := &config.Config{
		Launchpad: config.LaunchpadConfig{DefaultOwner: "local-owner"},
	}
	resolver := NewConfigResolver(local, nil)

	got, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != local {
		t.Fatalf("Resolve() returned %v, want local config %v", got, local)
	}
}

func TestConfigResolver_RemoteFetchesAndCaches(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/config" {
			http.NotFound(w, r)
			return
		}
		callCount++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(dto.Config{
			Launchpad: dto.LaunchpadConfig{DefaultOwner: "remote-owner"},
		})
	}))
	defer srv.Close()

	c := client.NewClient(srv.URL)
	resolver := NewConfigResolver(nil, c)

	ctx := context.Background()
	got, err := resolver.Resolve(ctx)
	if err != nil {
		t.Fatalf("Resolve() first call error = %v", err)
	}
	if got.Launchpad.DefaultOwner != "remote-owner" {
		t.Fatalf("Resolve() DefaultOwner = %q, want %q", got.Launchpad.DefaultOwner, "remote-owner")
	}

	// Second call must return cached value without hitting the server again.
	got2, err := resolver.Resolve(ctx)
	if err != nil {
		t.Fatalf("Resolve() second call error = %v", err)
	}
	if got2 != got {
		t.Fatalf("Resolve() second call returned different pointer, caching broken")
	}
	if callCount != 1 {
		t.Fatalf("server called %d times, want 1 (caching broken)", callCount)
	}
}

func TestConfigResolver_NeitherSource(t *testing.T) {
	resolver := NewConfigResolver(nil, nil)

	_, err := resolver.Resolve(context.Background())
	if err == nil {
		t.Fatal("Resolve() expected error but got nil")
	}
}

func TestConfigResolver_RemotePreferred(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/config" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(dto.Config{
			Launchpad: dto.LaunchpadConfig{DefaultOwner: "remote-owner"},
		})
	}))
	defer srv.Close()

	local := &config.Config{
		Launchpad: config.LaunchpadConfig{DefaultOwner: "local-owner"},
	}
	c := client.NewClient(srv.URL)
	resolver := NewConfigResolver(local, c)

	got, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Launchpad.DefaultOwner != "remote-owner" {
		t.Fatalf("Resolve() DefaultOwner = %q, want %q (remote should be authoritative)", got.Launchpad.DefaultOwner, "remote-owner")
	}
}

func TestConfigResolver_LocalConfig(t *testing.T) {
	local := &config.Config{
		Launchpad: config.LaunchpadConfig{DefaultOwner: "local-owner"},
	}
	resolver := NewConfigResolver(local, nil)

	got := resolver.LocalConfig()
	if got != local {
		t.Fatalf("LocalConfig() returned %v, want local config %v", got, local)
	}
}

func TestConfigResolver_LocalConfig_NilWhenNotSet(t *testing.T) {
	resolver := NewConfigResolver(nil, nil)

	got := resolver.LocalConfig()
	if got != nil {
		t.Fatalf("LocalConfig() returned %v, want nil", got)
	}
}

func TestConfigResolver_RemoteServerUnreachable(t *testing.T) {
	c := client.NewClient("http://127.0.0.1:1") // unreachable port
	resolver := NewConfigResolver(nil, c)

	_, err := resolver.Resolve(context.Background())
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
	if !strings.Contains(err.Error(), "could not fetch configuration from server") {
		t.Fatalf("error should mention fetch failure, got: %v", err)
	}
}
