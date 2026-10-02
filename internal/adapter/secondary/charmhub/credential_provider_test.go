// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package charmhub

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	sa "github.com/canonical/sunbeam-watchtower/pkg/storeauth/v1"
)

type fakeStore struct {
	record   *dto.StoreCredentialRecord
	savePath string
	loadErr  error
	saveErr  error
	saveCall struct {
		bundle    string
		macaroon  string
		callCount int
	}
}

func (s *fakeStore) Load(context.Context) (*dto.StoreCredentialRecord, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	if s.record == nil {
		return nil, nil
	}
	cp := *s.record
	return &cp, nil
}

func (s *fakeStore) Save(_ context.Context, bundle, macaroon string) (*dto.StoreCredentialRecord, error) {
	s.saveCall.bundle = bundle
	s.saveCall.macaroon = macaroon
	s.saveCall.callCount++
	if s.saveErr != nil {
		return nil, s.saveErr
	}
	s.record = &dto.StoreCredentialRecord{
		Macaroon:         macaroon,
		DischargedBundle: bundle,
		Source:           "file",
		Path:             s.savePath,
	}
	return s.record, nil
}

func (s *fakeStore) Clear(context.Context) error { s.record = nil; return nil }

type fakeAuth struct {
	exchangeToken string
	exchangeErr   error
	lastBundle    string
	calls         int
}

func (a *fakeAuth) BeginAuth(context.Context) (*sa.PendingAuthFlow, error) {
	return &sa.PendingAuthFlow{}, nil
}

func (a *fakeAuth) ExchangeToken(_ context.Context, bundle string) (string, error) {
	a.lastBundle = bundle
	a.calls++
	if a.exchangeErr != nil {
		return "", a.exchangeErr
	}
	return a.exchangeToken, nil
}

func TestCredentialProvider_TokenReturnsStoredMacaroon(t *testing.T) {
	store := &fakeStore{record: &dto.StoreCredentialRecord{
		Macaroon:         "short-lived",
		DischargedBundle: "long-lived",
	}}
	p := NewCredentialProvider(store, &fakeAuth{})

	tok, err := p.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if tok != "short-lived" {
		t.Fatalf("Token() = %q, want short-lived", tok)
	}
}

func TestCredentialProvider_TokenReturnsEmptyWhenAbsent(t *testing.T) {
	p := NewCredentialProvider(&fakeStore{}, &fakeAuth{})
	tok, err := p.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if tok != "" {
		t.Fatalf("Token() = %q, want empty", tok)
	}
}

func TestCredentialProvider_RefreshExchangesAndPersists(t *testing.T) {
	store := &fakeStore{record: &dto.StoreCredentialRecord{
		Macaroon:         "stale",
		DischargedBundle: "long-bundle",
	}}
	auth := &fakeAuth{exchangeToken: "fresh"}
	p := NewCredentialProvider(store, auth)

	if err := p.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if auth.lastBundle != "long-bundle" {
		t.Errorf("ExchangeToken called with %q, want long-bundle", auth.lastBundle)
	}
	if store.saveCall.macaroon != "fresh" || store.saveCall.bundle != "long-bundle" {
		t.Errorf("Save args = %+v, want fresh/long-bundle", store.saveCall)
	}

	tok, _ := p.Token(context.Background())
	if tok != "fresh" {
		t.Errorf("Token() after refresh = %q, want fresh", tok)
	}
}

func TestCredentialProvider_RefreshWithNoBundleReturnsReloginRequired(t *testing.T) {
	store := &fakeStore{record: &dto.StoreCredentialRecord{Macaroon: "legacy-token"}}
	p := NewCredentialProvider(store, &fakeAuth{})

	err := p.Refresh(context.Background())
	if !errors.Is(err, port.ErrCharmhubReloginRequired) {
		t.Fatalf("Refresh() error = %v, want ErrCharmhubReloginRequired", err)
	}
}

func TestCredentialProvider_RefreshWithNoRecordReturnsReloginRequired(t *testing.T) {
	p := NewCredentialProvider(&fakeStore{}, &fakeAuth{})

	err := p.Refresh(context.Background())
	if !errors.Is(err, port.ErrCharmhubReloginRequired) {
		t.Fatalf("Refresh() error = %v, want ErrCharmhubReloginRequired", err)
	}
}

func TestCredentialProvider_RefreshAuthExpiredFoldsIntoReloginRequired(t *testing.T) {
	// Simulates the exchange endpoint rejecting an exhausted discharge —
	// the provider converts the generic auth-expired signal into the
	// actionable re-login sentinel.
	store := &fakeStore{record: &dto.StoreCredentialRecord{
		Macaroon:         "stale",
		DischargedBundle: "discharge-exhausted",
	}}
	auth := &fakeAuth{exchangeErr: fmt.Errorf("exchange failed: %w", port.ErrStoreAuthExpired)}
	p := NewCredentialProvider(store, auth)

	err := p.Refresh(context.Background())
	if !errors.Is(err, port.ErrCharmhubReloginRequired) {
		t.Fatalf("Refresh() error = %v, want ErrCharmhubReloginRequired", err)
	}
	if store.saveCall.callCount != 0 {
		t.Errorf("Save called %d times on refresh failure, want 0", store.saveCall.callCount)
	}
}

func TestCredentialProvider_RefreshWithOtherExchangeErrorPropagates(t *testing.T) {
	store := &fakeStore{record: &dto.StoreCredentialRecord{
		Macaroon:         "stale",
		DischargedBundle: "bundle",
	}}
	boom := errors.New("network unreachable")
	auth := &fakeAuth{exchangeErr: boom}
	p := NewCredentialProvider(store, auth)

	err := p.Refresh(context.Background())
	if err == nil || errors.Is(err, port.ErrCharmhubReloginRequired) {
		t.Fatalf("Refresh() error = %v, want non-relogin wrap of transport failure", err)
	}
	if !errors.Is(err, boom) {
		t.Errorf("Refresh() error chain missing transport cause: %v", err)
	}
}
