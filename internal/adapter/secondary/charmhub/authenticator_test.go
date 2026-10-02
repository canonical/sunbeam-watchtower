// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package charmhub

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	sa "github.com/canonical/sunbeam-watchtower/pkg/storeauth/v1"
	"gopkg.in/macaroon.v2"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestAuthenticatorBeginAuthRequestsRootMacaroon(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req tokensRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if req.Description != "sunbeam-watchtower" {
			t.Fatalf("Description = %q, want sunbeam-watchtower", req.Description)
		}
		if len(req.Permissions) == 0 {
			t.Fatal("expected permissions")
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(tokensResponse{Macaroon: "fake-root-macaroon"})
	}))
	defer srv.Close()

	auth := NewAuthenticator(discardLogger(), srv.Client())
	auth.tokensEndpoint = srv.URL + "/v1/tokens"

	flow, err := auth.BeginAuth(context.Background())
	if err != nil {
		t.Fatalf("BeginAuth() error = %v", err)
	}
	if flow.RootMacaroon != "fake-root-macaroon" {
		t.Fatalf("RootMacaroon = %q, want fake-root-macaroon", flow.RootMacaroon)
	}
}

func TestAuthenticatorBeginAuthRejectsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("forbidden"))
	}))
	defer srv.Close()

	auth := NewAuthenticator(discardLogger(), srv.Client())
	auth.tokensEndpoint = srv.URL + "/v1/tokens"

	_, err := auth.BeginAuth(context.Background())
	if err == nil {
		t.Fatal("expected error on 403 response")
	}
}

func TestAuthenticatorExchangeTokenSendsMacaroonsHeader(t *testing.T) {
	// Build a two-macaroon slice and serialize it using the same helper the
	// discharge code path uses so the test exercises the real decode flow.
	root, err := macaroon.New([]byte("k1"), []byte("root-id"), "charmhub", macaroon.LatestVersion)
	if err != nil {
		t.Fatalf("macaroon.New(root) error = %v", err)
	}
	discharge, err := macaroon.New([]byte("k2"), []byte("discharge-id"), "idp", macaroon.LatestVersion)
	if err != nil {
		t.Fatalf("macaroon.New(discharge) error = %v", err)
	}
	bundle, err := sa.SerializeMacaroonSlice(macaroon.Slice{root, discharge})
	if err != nil {
		t.Fatalf("SerializeMacaroonSlice() error = %v", err)
	}

	var gotHeader string
	var gotMethod string
	var gotBody []byte
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotHeader = r.Header.Get("Macaroons")
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		_ = json.NewEncoder(w).Encode(tokensResponse{Macaroon: "exchanged-token"})
	}))
	defer srv.Close()

	auth := NewAuthenticator(discardLogger(), srv.Client())
	auth.exchangeEndpoint = srv.URL + "/v1/tokens/exchange"

	token, err := auth.ExchangeToken(context.Background(), bundle)
	if err != nil {
		t.Fatalf("ExchangeToken() error = %v", err)
	}
	if token != "exchanged-token" {
		t.Fatalf("ExchangeToken() = %q, want exchanged-token", token)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST", gotMethod)
	}
	if gotHeader == "" {
		t.Fatal("expected Macaroons header to be set")
	}
	if gotContentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", gotContentType)
	}
	// Charmhub's Python server rejects an empty body with "Expecting value:
	// line 1 column 1 (char 0)"; send an empty JSON object instead.
	if string(gotBody) != "{}" {
		t.Fatalf("body = %q, want %q", gotBody, "{}")
	}

	raw, err := base64.StdEncoding.DecodeString(gotHeader)
	if err != nil {
		t.Fatalf("Macaroons header is not base64: %v", err)
	}
	var decoded []json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Macaroons header is not a JSON array: %v (raw=%s)", err, raw)
	}
	if len(decoded) != 2 {
		t.Fatalf("Macaroons header carries %d macaroons, want 2", len(decoded))
	}
}

func TestAuthenticatorExchangeTokenRejectsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error-list":[{"code":"invalid-credentials","message":"bad"}]}`))
	}))
	defer srv.Close()

	m, err := macaroon.New([]byte("k"), []byte("id"), "loc", macaroon.LatestVersion)
	if err != nil {
		t.Fatalf("macaroon.New() error = %v", err)
	}
	bundle, err := sa.SerializeMacaroonSlice(macaroon.Slice{m})
	if err != nil {
		t.Fatalf("SerializeMacaroonSlice() error = %v", err)
	}

	auth := NewAuthenticator(discardLogger(), srv.Client())
	auth.exchangeEndpoint = srv.URL + "/v1/tokens/exchange"

	if _, err := auth.ExchangeToken(context.Background(), bundle); err == nil {
		t.Fatal("expected error on HTTP 401 response")
	}
}

func TestAuthenticatorExchangeTokenRejectsEmptyBundle(t *testing.T) {
	auth := NewAuthenticator(discardLogger(), nil)
	if _, err := auth.ExchangeToken(context.Background(), "   "); err == nil {
		t.Fatal("expected error on empty bundle")
	}
}

func TestAuthenticatorBeginAuthRejectsEmptyMacaroon(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(tokensResponse{Macaroon: ""})
	}))
	defer srv.Close()

	auth := NewAuthenticator(discardLogger(), srv.Client())
	auth.tokensEndpoint = srv.URL + "/v1/tokens"

	_, err := auth.BeginAuth(context.Background())
	if err == nil {
		t.Fatal("expected error on empty macaroon")
	}
}
