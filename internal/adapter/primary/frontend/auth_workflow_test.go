// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	authsvc "github.com/canonical/sunbeam-watchtower/internal/core/service/auth"
	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

func TestAuthWorkflowStatus(t *testing.T) {
	workflow := NewAuthWorkflowFromService(authsvc.NewService(
		&fakeCredentialStore{},
		&fakeFlowStore{},
		&fakeLaunchpadAuthenticator{},
		discardFrontendLogger(),
	))

	status, err := workflow.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Launchpad.Authenticated {
		t.Fatalf("Status() = %+v, want unauthenticated launchpad status", status)
	}
}

func TestAuthWorkflowBeginFinalizeAndLogout(t *testing.T) {
	store := &fakeCredentialStore{}
	flows := &fakeFlowStore{}
	authenticator := &fakeLaunchpadAuthenticator{
		requestToken: &lp.RequestToken{
			Token:       "request-token",
			TokenSecret: "request-secret",
		},
		accessCredentials: &lp.Credentials{
			ConsumerKey:       lp.ConsumerKey(),
			AccessToken:       "access-token",
			AccessTokenSecret: "access-secret",
		},
		currentUser: lp.Person{Name: "jdoe", DisplayName: "Jane Doe"},
	}
	service := authsvc.NewService(store, flows, authenticator, discardFrontendLogger())

	workflow := NewAuthWorkflowFromService(service)
	begin, err := workflow.BeginLaunchpad(context.Background())
	if err != nil {
		t.Fatalf("BeginLaunchpad() error = %v", err)
	}
	if begin.FlowID == "" {
		t.Fatal("BeginLaunchpad() returned empty flow ID")
	}

	finalized, err := workflow.FinalizeLaunchpad(context.Background(), begin.FlowID)
	if err != nil {
		t.Fatalf("FinalizeLaunchpad() error = %v", err)
	}
	if !finalized.Launchpad.Authenticated || finalized.Launchpad.Username != "jdoe" {
		t.Fatalf("FinalizeLaunchpad() = %+v, want authenticated jdoe", finalized)
	}

	logout, err := workflow.LogoutLaunchpad(context.Background())
	if err != nil {
		t.Fatalf("LogoutLaunchpad() error = %v", err)
	}
	if !logout.Cleared {
		t.Fatalf("LogoutLaunchpad() = %+v, want cleared result", logout)
	}
}

type fakeCredentialStore struct {
	record   *lp.CredentialRecord
	savePath string
}

func (f *fakeCredentialStore) Load(context.Context) (*lp.CredentialRecord, error) {
	return f.record, nil
}

func (f *fakeCredentialStore) Save(_ context.Context, creds *lp.Credentials) (*lp.CredentialRecord, error) {
	f.record = &lp.CredentialRecord{
		Credentials: creds,
		Source:      lp.CredentialSourceFile,
		Path:        f.savePath,
	}
	return f.record, nil
}

func (f *fakeCredentialStore) Clear(context.Context) error {
	if f.record == nil {
		return nil
	}
	f.record = nil
	return nil
}

type fakeFlowStore struct {
	flows map[string]*lp.PendingAuthFlow
}

func (f *fakeFlowStore) Put(_ context.Context, flow *lp.PendingAuthFlow) error {
	if f.flows == nil {
		f.flows = make(map[string]*lp.PendingAuthFlow)
	}
	cloned := *flow
	f.flows[flow.ID] = &cloned
	return nil
}

func (f *fakeFlowStore) Get(_ context.Context, id string) (*lp.PendingAuthFlow, error) {
	flow, ok := f.flows[id]
	if !ok {
		return nil, lp.ErrPendingAuthFlowNotFound
	}
	if !flow.ExpiresAt.IsZero() && flow.ExpiresAt.Before(time.Now().UTC()) {
		return nil, lp.ErrPendingAuthFlowExpired
	}
	cloned := *flow
	return &cloned, nil
}

func (f *fakeFlowStore) Delete(_ context.Context, id string) error {
	delete(f.flows, id)
	return nil
}

type fakeLaunchpadAuthenticator struct {
	requestToken      *lp.RequestToken
	accessCredentials *lp.Credentials
	currentUser       lp.Person
	userErr           error
}

func (f *fakeLaunchpadAuthenticator) ConsumerKey() string {
	return lp.ConsumerKey()
}

func (f *fakeLaunchpadAuthenticator) ObtainRequestToken(context.Context) (*lp.RequestToken, error) {
	return f.requestToken, nil
}

func (f *fakeLaunchpadAuthenticator) ExchangeAccessToken(context.Context, *lp.RequestToken) (*lp.Credentials, error) {
	return f.accessCredentials, nil
}

func (f *fakeLaunchpadAuthenticator) CurrentUser(context.Context, *lp.Credentials) (lp.Person, error) {
	if f.userErr != nil {
		return lp.Person{}, f.userErr
	}
	return f.currentUser, nil
}

var _ port.LaunchpadCredentialStore = (*fakeCredentialStore)(nil)
var _ port.LaunchpadPendingAuthFlowStore = (*fakeFlowStore)(nil)
var _ port.LaunchpadAuthenticator = (*fakeLaunchpadAuthenticator)(nil)

func discardFrontendLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
