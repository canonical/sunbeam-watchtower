// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	"github.com/canonical/sunbeam-watchtower/internal/testsupport"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	gh "github.com/canonical/sunbeam-watchtower/pkg/github/v1"
	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
	sa "github.com/canonical/sunbeam-watchtower/pkg/storeauth/v1"
)

// TestMain unsets every forge credential env var for the whole test binary
// so the developer's shell environment cannot influence outcomes — even
// though the tests below use fake stores that never read env vars today.
func TestMain(m *testing.M) {
	for _, name := range testsupport.ForgeCredentialEnvVars {
		_ = os.Unsetenv(name)
	}
	os.Exit(m.Run())
}

type fakeCredentialStore struct {
	record   *lp.CredentialRecord
	savePath string
	loadErr  error
	saveErr  error
	clearErr error
}

func (s *fakeCredentialStore) Load(context.Context) (*lp.CredentialRecord, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	if s.record == nil {
		return nil, nil
	}
	recordCopy := *s.record
	return &recordCopy, nil
}

func (s *fakeCredentialStore) Save(_ context.Context, creds *lp.Credentials) (*lp.CredentialRecord, error) {
	if s.saveErr != nil {
		return nil, s.saveErr
	}
	s.record = &lp.CredentialRecord{
		Credentials: creds,
		Source:      lp.CredentialSourceFile,
		Path:        s.savePath,
	}
	return s.Load(context.Background())
}

func (s *fakeCredentialStore) Clear(context.Context) error {
	if s.clearErr != nil {
		return s.clearErr
	}
	s.record = nil
	return nil
}

type fakeFlowStore struct {
	flows     map[string]lp.PendingAuthFlow
	putErr    error
	getErr    error
	deleteErr error
}

type fakeGitHubCredentialStore struct {
	record   *gh.CredentialRecord
	savePath string
	loadErr  error
	saveErr  error
	clearErr error
}

func (s *fakeGitHubCredentialStore) Load(context.Context) (*gh.CredentialRecord, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	if s.record == nil {
		return nil, nil
	}
	recordCopy := *s.record
	return &recordCopy, nil
}

func (s *fakeGitHubCredentialStore) Save(_ context.Context, creds *gh.Credentials) (*gh.CredentialRecord, error) {
	if s.saveErr != nil {
		return nil, s.saveErr
	}
	s.record = &gh.CredentialRecord{
		Credentials: creds,
		Source:      gh.CredentialSourceFile,
		Path:        s.savePath,
	}
	return s.Load(context.Background())
}

func (s *fakeGitHubCredentialStore) Clear(context.Context) error {
	if s.clearErr != nil {
		return s.clearErr
	}
	s.record = nil
	return nil
}

type fakeGitHubFlowStore struct {
	flows     map[string]gh.PendingAuthFlow
	putErr    error
	getErr    error
	deleteErr error
}

func (s *fakeGitHubFlowStore) Put(_ context.Context, flow *gh.PendingAuthFlow) error {
	if s.flows == nil {
		s.flows = make(map[string]gh.PendingAuthFlow)
	}
	s.flows[flow.ID] = *flow
	return s.putErr
}

func (s *fakeGitHubFlowStore) Get(_ context.Context, id string) (*gh.PendingAuthFlow, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	flow, ok := s.flows[id]
	if !ok {
		return nil, gh.ErrPendingAuthFlowNotFound
	}
	flowCopy := flow
	return &flowCopy, nil
}

func (s *fakeGitHubFlowStore) Delete(_ context.Context, id string) error {
	delete(s.flows, id)
	return s.deleteErr
}

func (s *fakeFlowStore) Put(_ context.Context, flow *lp.PendingAuthFlow) error {
	if s.flows == nil {
		s.flows = make(map[string]lp.PendingAuthFlow)
	}
	s.flows[flow.ID] = *flow
	return s.putErr
}

func (s *fakeFlowStore) Get(_ context.Context, id string) (*lp.PendingAuthFlow, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	flow, ok := s.flows[id]
	if !ok {
		return nil, lp.ErrPendingAuthFlowNotFound
	}
	flowCopy := flow
	return &flowCopy, nil
}

func (s *fakeFlowStore) Delete(_ context.Context, id string) error {
	delete(s.flows, id)
	return s.deleteErr
}

type fakeLaunchpadAuthenticator struct {
	requestToken *lp.RequestToken
	creds        *lp.Credentials
	identity     lp.Person
	userErr      error
	lastToken    *lp.RequestToken
}

func (a *fakeLaunchpadAuthenticator) ConsumerKey() string { return "sunbeam-watchtower" }

func (a *fakeLaunchpadAuthenticator) ObtainRequestToken(context.Context) (*lp.RequestToken, error) {
	return a.requestToken, nil
}

func (a *fakeLaunchpadAuthenticator) ExchangeAccessToken(_ context.Context, token *lp.RequestToken) (*lp.Credentials, error) {
	a.lastToken = token
	return a.creds, nil
}

func (a *fakeLaunchpadAuthenticator) CurrentUser(context.Context, *lp.Credentials) (lp.Person, error) {
	if a.userErr != nil {
		return lp.Person{}, a.userErr
	}
	return a.identity, nil
}

type fakeGitHubAuthenticator struct {
	beginFlow *gh.PendingAuthFlow
	creds     *gh.Credentials
	identity  gh.User
	userErr   error
	pollErr   error
	beginErr  error
}

func (a *fakeGitHubAuthenticator) ClientID() string { return "client-id" }

func (a *fakeGitHubAuthenticator) BeginDeviceFlow(context.Context) (*gh.PendingAuthFlow, error) {
	if a.beginErr != nil {
		return nil, a.beginErr
	}
	return a.beginFlow, nil
}

func (a *fakeGitHubAuthenticator) PollAccessToken(context.Context, *gh.PendingAuthFlow) (*gh.Credentials, error) {
	if a.pollErr != nil {
		return nil, a.pollErr
	}
	return a.creds, nil
}

func (a *fakeGitHubAuthenticator) CurrentUser(context.Context, *gh.Credentials) (gh.User, error) {
	if a.userErr != nil {
		return gh.User{}, a.userErr
	}
	return a.identity, nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestBeginLaunchpadStoresServerSideSecretState(t *testing.T) {
	flows := &fakeFlowStore{}
	svc := NewService(
		&fakeCredentialStore{},
		flows,
		&fakeLaunchpadAuthenticator{
			requestToken: &lp.RequestToken{
				Token:       "request-token",
				TokenSecret: "request-secret",
			},
		},
		testLogger(),
	)
	svc.now = func() time.Time { return time.Date(2026, 3, 6, 20, 0, 0, 0, time.UTC) }
	svc.newFlowID = func() (string, error) { return "flow-123", nil }

	result, err := svc.BeginLaunchpad(context.Background())
	if err != nil {
		t.Fatalf("BeginLaunchpad() error = %v", err)
	}

	if result.FlowID != "flow-123" {
		t.Fatalf("FlowID = %q, want flow-123", result.FlowID)
	}
	if result.AuthorizeURL != "https://launchpad.net/+authorize-token?oauth_token=request-token" {
		t.Fatalf("AuthorizeURL = %q", result.AuthorizeURL)
	}
	if _, ok := flows.flows["flow-123"]; !ok {
		t.Fatal("pending flow not stored")
	}
	if got := flows.flows["flow-123"].RequestTokenSecret; got != "request-secret" {
		t.Fatalf("stored request token secret = %q, want request-secret", got)
	}
}

func TestFinalizeLaunchpadSavesCredentialsAndDeletesFlow(t *testing.T) {
	credsPath := filepath.Join(t.TempDir(), "credentials.json")
	store := &fakeCredentialStore{savePath: credsPath}
	flows := &fakeFlowStore{
		flows: map[string]lp.PendingAuthFlow{
			"flow-123": {
				ID:                 "flow-123",
				RequestToken:       "request-token",
				RequestTokenSecret: "request-secret",
			},
		},
	}
	authenticator := &fakeLaunchpadAuthenticator{
		creds: &lp.Credentials{
			ConsumerKey:       "sunbeam-watchtower",
			AccessToken:       "access-token",
			AccessTokenSecret: "access-secret",
		},
		identity: lp.Person{
			Name:        "gboutry",
			DisplayName: "Guillaume Boutry",
		},
	}
	svc := NewService(store, flows, authenticator, testLogger())

	result, err := svc.FinalizeLaunchpad(context.Background(), "flow-123")
	if err != nil {
		t.Fatalf("FinalizeLaunchpad() error = %v", err)
	}

	if authenticator.lastToken.TokenSecret != "request-secret" {
		t.Fatalf("exchange used secret %q, want request-secret", authenticator.lastToken.TokenSecret)
	}
	if _, ok := flows.flows["flow-123"]; ok {
		t.Fatal("flow was not deleted after finalize")
	}
	if !result.Launchpad.Authenticated {
		t.Fatal("expected authenticated status after finalize")
	}
	if result.Launchpad.CredentialsPath != credsPath {
		t.Fatalf("credentials path = %q", result.Launchpad.CredentialsPath)
	}
}

func TestStatusReportsInvalidStoredCredentialsWithoutFailing(t *testing.T) {
	svc := NewService(
		&fakeCredentialStore{
			record: &lp.CredentialRecord{
				Credentials: &lp.Credentials{AccessToken: "token", AccessTokenSecret: "secret"},
				Source:      lp.CredentialSourceFile,
				Path:        filepath.Join(t.TempDir(), "credentials.json"),
			},
		},
		&fakeFlowStore{},
		&fakeLaunchpadAuthenticator{userErr: errors.New("invalid credentials")},
		testLogger(),
	)

	status, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Launchpad.Authenticated {
		t.Fatal("expected unauthenticated status when verification fails")
	}
	if status.Launchpad.Error == "" {
		t.Fatal("expected verification error in auth status")
	}
}

func TestLogoutLaunchpadRejectsEnvironmentCredentials(t *testing.T) {
	svc := NewService(
		&fakeCredentialStore{
			record: &lp.CredentialRecord{
				Credentials: &lp.Credentials{AccessToken: "token", AccessTokenSecret: "secret"},
				Source:      lp.CredentialSourceEnvironment,
			},
		},
		&fakeFlowStore{},
		&fakeLaunchpadAuthenticator{},
		testLogger(),
	)

	_, err := svc.LogoutLaunchpad(context.Background())
	if !errors.Is(err, ErrLaunchpadEnvironmentCredentials) {
		t.Fatalf("LogoutLaunchpad() error = %v, want %v", err, ErrLaunchpadEnvironmentCredentials)
	}
}

func TestBeginLaunchpadReturnsStoreError(t *testing.T) {
	svc := NewService(
		&fakeCredentialStore{},
		&fakeFlowStore{putErr: errors.New("write failed")},
		&fakeLaunchpadAuthenticator{
			requestToken: &lp.RequestToken{Token: "request-token", TokenSecret: "request-secret"},
		},
		nil,
	)
	svc.newFlowID = func() (string, error) { return "flow-123", nil }

	_, err := svc.BeginLaunchpad(context.Background())
	if err == nil || !strings.Contains(err.Error(), "storing launchpad auth flow") {
		t.Fatalf("BeginLaunchpad() error = %v, want storing launchpad auth flow", err)
	}
}

func TestFinalizeLaunchpadMapsMissingAndExpiredFlows(t *testing.T) {
	t.Run("missing flow", func(t *testing.T) {
		svc := NewService(&fakeCredentialStore{}, &fakeFlowStore{}, &fakeLaunchpadAuthenticator{}, testLogger())

		_, err := svc.FinalizeLaunchpad(context.Background(), "missing")
		if !errors.Is(err, ErrLaunchpadAuthFlowNotFound) {
			t.Fatalf("FinalizeLaunchpad() error = %v, want %v", err, ErrLaunchpadAuthFlowNotFound)
		}
	})

	t.Run("expired flow", func(t *testing.T) {
		svc := NewService(
			&fakeCredentialStore{},
			&fakeFlowStore{getErr: lp.ErrPendingAuthFlowExpired},
			&fakeLaunchpadAuthenticator{},
			testLogger(),
		)

		_, err := svc.FinalizeLaunchpad(context.Background(), "expired")
		if !errors.Is(err, ErrLaunchpadAuthFlowExpired) {
			t.Fatalf("FinalizeLaunchpad() error = %v, want %v", err, ErrLaunchpadAuthFlowExpired)
		}
	})
}

func TestFinalizeLaunchpadReturnsDeleteError(t *testing.T) {
	svc := NewService(
		&fakeCredentialStore{},
		&fakeFlowStore{
			flows: map[string]lp.PendingAuthFlow{
				"flow-123": {
					ID:                 "flow-123",
					RequestToken:       "request-token",
					RequestTokenSecret: "request-secret",
				},
			},
			deleteErr: errors.New("delete failed"),
		},
		&fakeLaunchpadAuthenticator{
			creds: &lp.Credentials{
				ConsumerKey:       "sunbeam-watchtower",
				AccessToken:       "access-token",
				AccessTokenSecret: "access-secret",
			},
		},
		testLogger(),
	)

	_, err := svc.FinalizeLaunchpad(context.Background(), "flow-123")
	if err == nil || !strings.Contains(err.Error(), "deleting completed launchpad auth flow") {
		t.Fatalf("FinalizeLaunchpad() error = %v, want deleting completed launchpad auth flow", err)
	}
}

func TestLogoutLaunchpadClearsPersistedCredentials(t *testing.T) {
	credsPath := filepath.Join(t.TempDir(), "credentials.json")
	store := &fakeCredentialStore{
		record: &lp.CredentialRecord{
			Credentials: &lp.Credentials{AccessToken: "token", AccessTokenSecret: "secret"},
			Source:      lp.CredentialSourceFile,
			Path:        credsPath,
		},
	}
	svc := NewService(store, &fakeFlowStore{}, &fakeLaunchpadAuthenticator{}, testLogger())

	result, err := svc.LogoutLaunchpad(context.Background())
	if err != nil {
		t.Fatalf("LogoutLaunchpad() error = %v", err)
	}
	if !result.Cleared || result.CredentialsPath != credsPath {
		t.Fatalf("LogoutLaunchpad() = %+v, want cleared persisted credentials", result)
	}
	if store.record != nil {
		t.Fatal("store.record should be cleared")
	}
}

func TestRandomFlowIDReturnsHexToken(t *testing.T) {
	id, err := randomFlowID()
	if err != nil {
		t.Fatalf("randomFlowID() error = %v", err)
	}
	if len(id) != 32 {
		t.Fatalf("len(randomFlowID()) = %d, want 32", len(id))
	}
	if _, err := hex.DecodeString(id); err != nil {
		t.Fatalf("randomFlowID() = %q, want hex: %v", id, err)
	}
}

func TestBeginGitHubStoresPendingFlow(t *testing.T) {
	flows := &fakeGitHubFlowStore{}
	svc := NewServiceWithGitHub(
		&fakeCredentialStore{},
		&fakeFlowStore{},
		&fakeLaunchpadAuthenticator{},
		&fakeGitHubCredentialStore{},
		flows,
		&fakeGitHubAuthenticator{
			beginFlow: &gh.PendingAuthFlow{
				DeviceCode:      "device",
				UserCode:        "ABCD-EFGH",
				VerificationURI: "https://github.com/login/device",
				IntervalSeconds: 5,
				ExpiresAt:       time.Now().Add(time.Minute),
			},
		},
		nil,
		testLogger(),
	)
	svc.newFlowID = func() (string, error) { return "flow-123", nil }

	result, err := svc.BeginGitHub(context.Background())
	if err != nil {
		t.Fatalf("BeginGitHub() error = %v", err)
	}
	if result.FlowID != "flow-123" || result.UserCode != "ABCD-EFGH" {
		t.Fatalf("BeginGitHub() = %+v", result)
	}
	if _, ok := flows.flows["flow-123"]; !ok {
		t.Fatal("pending flow not stored")
	}
}

func TestFinalizeGitHubSavesCredentials(t *testing.T) {
	store := &fakeGitHubCredentialStore{savePath: filepath.Join(t.TempDir(), "github-creds")}
	flows := &fakeGitHubFlowStore{
		flows: map[string]gh.PendingAuthFlow{
			"flow-123": {ID: "flow-123", DeviceCode: "device", UserCode: "ABCD-EFGH"},
		},
	}
	svc := NewServiceWithGitHub(
		&fakeCredentialStore{},
		&fakeFlowStore{},
		&fakeLaunchpadAuthenticator{},
		store,
		flows,
		&fakeGitHubAuthenticator{
			creds:    &gh.Credentials{AccessToken: "token"},
			identity: gh.User{Login: "jdoe", Name: "Jane Doe"},
		},
		nil,
		testLogger(),
	)

	result, err := svc.FinalizeGitHub(context.Background(), "flow-123")
	if err != nil {
		t.Fatalf("FinalizeGitHub() error = %v", err)
	}
	if !result.GitHub.Authenticated || result.GitHub.Username != "jdoe" {
		t.Fatalf("FinalizeGitHub() = %+v", result)
	}
	if _, ok := flows.flows["flow-123"]; ok {
		t.Fatal("flow was not deleted after finalize")
	}
}

func TestStatusReportsGitHubAuthentication(t *testing.T) {
	svc := NewServiceWithGitHub(
		&fakeCredentialStore{},
		&fakeFlowStore{},
		&fakeLaunchpadAuthenticator{},
		&fakeGitHubCredentialStore{
			record: &gh.CredentialRecord{
				Credentials: &gh.Credentials{AccessToken: "token"},
				Source:      gh.CredentialSourceFile,
				Path:        filepath.Join(t.TempDir(), "github-creds.json"),
			},
		},
		&fakeGitHubFlowStore{},
		&fakeGitHubAuthenticator{identity: gh.User{Login: "jdoe", Name: "Jane Doe"}},
		nil,
		testLogger(),
	)

	status, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.GitHub.Authenticated || status.GitHub.Username != "jdoe" || status.GitHub.DisplayName != "Jane Doe" {
		t.Fatalf("Status().GitHub = %+v", status.GitHub)
	}
}

func TestStatusReportsGitHubVerificationErrorWithoutFailing(t *testing.T) {
	svc := NewServiceWithGitHub(
		&fakeCredentialStore{},
		&fakeFlowStore{},
		&fakeLaunchpadAuthenticator{},
		&fakeGitHubCredentialStore{
			record: &gh.CredentialRecord{
				Credentials: &gh.Credentials{AccessToken: "token"},
				Source:      gh.CredentialSourceFile,
				Path:        filepath.Join(t.TempDir(), "github-creds.json"),
			},
		},
		&fakeGitHubFlowStore{},
		&fakeGitHubAuthenticator{userErr: errors.New("invalid github token")},
		nil,
		testLogger(),
	)

	status, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.GitHub.Authenticated {
		t.Fatal("expected unauthenticated GitHub status when verification fails")
	}
	if status.GitHub.Error == "" {
		t.Fatal("expected GitHub verification error in auth status")
	}
}

func TestStatusReportsGitHubAuthenticatorUnavailable(t *testing.T) {
	svc := NewServiceWithGitHub(
		&fakeCredentialStore{},
		&fakeFlowStore{},
		&fakeLaunchpadAuthenticator{},
		&fakeGitHubCredentialStore{
			record: &gh.CredentialRecord{
				Credentials: &gh.Credentials{AccessToken: "token"},
				Source:      gh.CredentialSourceFile,
			},
		},
		&fakeGitHubFlowStore{},
		nil,
		nil,
		testLogger(),
	)

	status, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.GitHub.Authenticated {
		t.Fatal("expected unauthenticated GitHub status without authenticator")
	}
	if status.GitHub.Error == "" {
		t.Fatal("expected unavailable authenticator error")
	}
}

func TestBeginGitHubRejectsEnvironmentCredentials(t *testing.T) {
	svc := NewServiceWithGitHub(
		&fakeCredentialStore{},
		&fakeFlowStore{},
		&fakeLaunchpadAuthenticator{},
		&fakeGitHubCredentialStore{
			record: &gh.CredentialRecord{
				Credentials: &gh.Credentials{AccessToken: "token"},
				Source:      gh.CredentialSourceEnvironment,
			},
		},
		&fakeGitHubFlowStore{},
		&fakeGitHubAuthenticator{beginFlow: &gh.PendingAuthFlow{}},
		nil,
		testLogger(),
	)

	_, err := svc.BeginGitHub(context.Background())
	if !errors.Is(err, ErrGitHubEnvironmentCredentials) {
		t.Fatalf("BeginGitHub() error = %v, want %v", err, ErrGitHubEnvironmentCredentials)
	}
}

func TestBeginGitHubRejectsMutableError(t *testing.T) {
	svc := NewServiceWithGitHub(
		&fakeCredentialStore{},
		&fakeFlowStore{},
		&fakeLaunchpadAuthenticator{},
		&fakeGitHubCredentialStore{},
		&fakeGitHubFlowStore{},
		&fakeGitHubAuthenticator{beginFlow: &gh.PendingAuthFlow{}},
		ErrGitHubKeyringNotImplemented,
		testLogger(),
	)

	_, err := svc.BeginGitHub(context.Background())
	if !errors.Is(err, ErrGitHubKeyringNotImplemented) {
		t.Fatalf("BeginGitHub() error = %v, want %v", err, ErrGitHubKeyringNotImplemented)
	}
}

func TestBeginGitHubRequiresClientID(t *testing.T) {
	svc := NewServiceWithGitHub(
		&fakeCredentialStore{},
		&fakeFlowStore{},
		&fakeLaunchpadAuthenticator{},
		&fakeGitHubCredentialStore{},
		&fakeGitHubFlowStore{},
		nil,
		nil,
		testLogger(),
	)

	_, err := svc.BeginGitHub(context.Background())
	if !errors.Is(err, ErrGitHubClientIDRequired) {
		t.Fatalf("BeginGitHub() error = %v, want %v", err, ErrGitHubClientIDRequired)
	}
}

func TestFinalizeGitHubMapsErrors(t *testing.T) {
	tests := []struct {
		name    string
		pollErr error
		wantErr error
	}{
		{name: "access denied", pollErr: gh.ErrAccessDenied, wantErr: ErrGitHubAccessDenied},
		{name: "expired token", pollErr: gh.ErrExpiredToken, wantErr: ErrGitHubAuthFlowExpired},
		{name: "incorrect device code", pollErr: gh.ErrIncorrectDeviceCode, wantErr: ErrGitHubAuthFlowExpired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flows := &fakeGitHubFlowStore{
				flows: map[string]gh.PendingAuthFlow{
					"flow-123": {ID: "flow-123", DeviceCode: "device"},
				},
			}
			svc := NewServiceWithGitHub(
				&fakeCredentialStore{},
				&fakeFlowStore{},
				&fakeLaunchpadAuthenticator{},
				&fakeGitHubCredentialStore{},
				flows,
				&fakeGitHubAuthenticator{pollErr: tt.pollErr},
				nil,
				testLogger(),
			)

			_, err := svc.FinalizeGitHub(context.Background(), "flow-123")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("FinalizeGitHub() error = %v, want %v", err, tt.wantErr)
			}
			if _, ok := flows.flows["flow-123"]; ok {
				t.Fatal("flow should be deleted after terminal GitHub finalize error")
			}
		})
	}
}

func TestFinalizeGitHubMapsMissingAndExpiredFlows(t *testing.T) {
	t.Run("missing flow", func(t *testing.T) {
		svc := NewServiceWithGitHub(
			&fakeCredentialStore{},
			&fakeFlowStore{},
			&fakeLaunchpadAuthenticator{},
			&fakeGitHubCredentialStore{},
			&fakeGitHubFlowStore{},
			&fakeGitHubAuthenticator{},
			nil,
			testLogger(),
		)

		_, err := svc.FinalizeGitHub(context.Background(), "missing")
		if !errors.Is(err, ErrGitHubAuthFlowNotFound) {
			t.Fatalf("FinalizeGitHub() error = %v, want %v", err, ErrGitHubAuthFlowNotFound)
		}
	})

	t.Run("expired flow", func(t *testing.T) {
		svc := NewServiceWithGitHub(
			&fakeCredentialStore{},
			&fakeFlowStore{},
			&fakeLaunchpadAuthenticator{},
			&fakeGitHubCredentialStore{},
			&fakeGitHubFlowStore{getErr: gh.ErrPendingAuthFlowExpired},
			&fakeGitHubAuthenticator{},
			nil,
			testLogger(),
		)

		_, err := svc.FinalizeGitHub(context.Background(), "expired")
		if !errors.Is(err, ErrGitHubAuthFlowExpired) {
			t.Fatalf("FinalizeGitHub() error = %v, want %v", err, ErrGitHubAuthFlowExpired)
		}
	})
}

func TestLogoutGitHubPaths(t *testing.T) {
	t.Run("rejects environment credentials", func(t *testing.T) {
		svc := NewServiceWithGitHub(
			&fakeCredentialStore{},
			&fakeFlowStore{},
			&fakeLaunchpadAuthenticator{},
			&fakeGitHubCredentialStore{
				record: &gh.CredentialRecord{
					Credentials: &gh.Credentials{AccessToken: "token"},
					Source:      gh.CredentialSourceEnvironment,
				},
			},
			&fakeGitHubFlowStore{},
			&fakeGitHubAuthenticator{},
			nil,
			testLogger(),
		)

		_, err := svc.LogoutGitHub(context.Background())
		if !errors.Is(err, ErrGitHubEnvironmentCredentials) {
			t.Fatalf("LogoutGitHub() error = %v, want %v", err, ErrGitHubEnvironmentCredentials)
		}
	})

	t.Run("clears persisted credentials", func(t *testing.T) {
		credsPath := filepath.Join(t.TempDir(), "github-creds.json")
		store := &fakeGitHubCredentialStore{
			record: &gh.CredentialRecord{
				Credentials: &gh.Credentials{AccessToken: "token"},
				Source:      gh.CredentialSourceFile,
				Path:        credsPath,
			},
		}
		svc := NewServiceWithGitHub(
			&fakeCredentialStore{},
			&fakeFlowStore{},
			&fakeLaunchpadAuthenticator{},
			store,
			&fakeGitHubFlowStore{},
			&fakeGitHubAuthenticator{},
			nil,
			testLogger(),
		)

		result, err := svc.LogoutGitHub(context.Background())
		if err != nil {
			t.Fatalf("LogoutGitHub() error = %v", err)
		}
		if !result.Cleared || result.CredentialsPath != credsPath {
			t.Fatalf("LogoutGitHub() = %+v", result)
		}
		if store.record != nil {
			t.Fatal("expected GitHub credentials to be cleared")
		}
	})
}

func TestFinalizeGitHubReturnsDeleteError(t *testing.T) {
	svc := NewServiceWithGitHub(
		&fakeCredentialStore{},
		&fakeFlowStore{},
		&fakeLaunchpadAuthenticator{},
		&fakeGitHubCredentialStore{},
		&fakeGitHubFlowStore{
			flows: map[string]gh.PendingAuthFlow{
				"flow-123": {ID: "flow-123", DeviceCode: "device"},
			},
			deleteErr: errors.New("delete failed"),
		},
		&fakeGitHubAuthenticator{
			creds:    &gh.Credentials{AccessToken: "token"},
			identity: gh.User{Login: "jdoe"},
		},
		nil,
		testLogger(),
	)

	_, err := svc.FinalizeGitHub(context.Background(), "flow-123")
	if err == nil || !strings.Contains(err.Error(), "deleting completed github auth flow") {
		t.Fatalf("FinalizeGitHub() error = %v, want deleting completed github auth flow", err)
	}
}

// fakeStoreCredentialStore is a test double for SnapStoreCredentialStore.
type fakeStoreCredentialStore struct {
	record   *dto.StoreCredentialRecord
	savePath string
	loadErr  error
	saveErr  error
	clearErr error
}

func (s *fakeStoreCredentialStore) Load(context.Context) (*dto.StoreCredentialRecord, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	if s.record == nil {
		return nil, nil
	}
	recordCopy := *s.record
	return &recordCopy, nil
}

func (s *fakeStoreCredentialStore) Save(_ context.Context, macaroon string) (*dto.StoreCredentialRecord, error) {
	if s.saveErr != nil {
		return nil, s.saveErr
	}
	s.record = &dto.StoreCredentialRecord{
		Macaroon: macaroon,
		Source:   "file",
		Path:     s.savePath,
	}
	return s.Load(context.Background())
}

func (s *fakeStoreCredentialStore) Clear(context.Context) error {
	if s.clearErr != nil {
		return s.clearErr
	}
	s.record = nil
	return nil
}

// fakeCharmhubCredentialStore is a test double for CharmhubCredentialStore.
// Save captures both the discharged bundle and the exchanged token so tests
// can assert on the refresh-readable layout.
type fakeCharmhubCredentialStore struct {
	record   *dto.StoreCredentialRecord
	savePath string
	loadErr  error
	saveErr  error
	clearErr error
}

func (s *fakeCharmhubCredentialStore) Load(context.Context) (*dto.StoreCredentialRecord, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	if s.record == nil {
		return nil, nil
	}
	recordCopy := *s.record
	return &recordCopy, nil
}

func (s *fakeCharmhubCredentialStore) Save(_ context.Context, dischargedBundle, exchangedMacaroon string) (*dto.StoreCredentialRecord, error) {
	if s.saveErr != nil {
		return nil, s.saveErr
	}
	s.record = &dto.StoreCredentialRecord{
		Macaroon:         exchangedMacaroon,
		DischargedBundle: dischargedBundle,
		Source:           "file",
		Path:             s.savePath,
	}
	return s.Load(context.Background())
}

func (s *fakeCharmhubCredentialStore) Clear(context.Context) error {
	if s.clearErr != nil {
		return s.clearErr
	}
	s.record = nil
	return nil
}

type fakeStoreAuthenticator struct {
	flow            *sa.PendingAuthFlow
	beginErr        error
	exchangeToken   string
	exchangeErr     error
	exchangeCalls   int
	lastExchangeArg string
}

func (a *fakeStoreAuthenticator) BeginAuth(context.Context) (*sa.PendingAuthFlow, error) {
	if a.beginErr != nil {
		return nil, a.beginErr
	}
	return a.flow, nil
}

// ExchangeToken satisfies port.CharmhubAuthenticator. It is harmless for the
// snap-store code path because snap wires this type through the bare
// StoreAuthenticator alias.
func (a *fakeStoreAuthenticator) ExchangeToken(_ context.Context, bundle string) (string, error) {
	a.exchangeCalls++
	a.lastExchangeArg = bundle
	if a.exchangeErr != nil {
		return "", a.exchangeErr
	}
	if a.exchangeToken == "" {
		return bundle, nil
	}
	return a.exchangeToken, nil
}

func newServiceWithStores(snapStore *fakeStoreCredentialStore, charmhubStore *fakeCharmhubCredentialStore) *Service {
	var ss port.SnapStoreCredentialStore
	if snapStore != nil {
		ss = snapStore
	}
	var cs port.CharmhubCredentialStore
	if charmhubStore != nil {
		cs = charmhubStore
	}
	return NewServiceWithStores(
		&fakeCredentialStore{},
		&fakeFlowStore{},
		&fakeLaunchpadAuthenticator{},
		nil, nil, nil, nil,
		ss, nil,
		cs, nil,
		testLogger(),
	)
}

func newServiceWithStoreAuth(
	snapStore *fakeStoreCredentialStore,
	snapAuth *fakeStoreAuthenticator,
	charmhubStore *fakeCharmhubCredentialStore,
	charmhubAuth *fakeStoreAuthenticator,
) *Service {
	var ss port.SnapStoreCredentialStore
	if snapStore != nil {
		ss = snapStore
	}
	var cs port.CharmhubCredentialStore
	if charmhubStore != nil {
		cs = charmhubStore
	}
	var ssAuth port.SnapStoreAuthenticator
	if snapAuth != nil {
		ssAuth = snapAuth
	}
	var csAuth port.CharmhubAuthenticator
	if charmhubAuth != nil {
		csAuth = charmhubAuth
	}
	return NewServiceWithStores(
		&fakeCredentialStore{},
		&fakeFlowStore{},
		&fakeLaunchpadAuthenticator{},
		nil, nil, nil, nil,
		ss, ssAuth,
		cs, csAuth,
		testLogger(),
	)
}

func TestBeginSnapStoreReturnsRootMacaroon(t *testing.T) {
	store := &fakeStoreCredentialStore{}
	auth := &fakeStoreAuthenticator{
		flow: &sa.PendingAuthFlow{
			RootMacaroon: "root-mac",
		},
	}
	svc := newServiceWithStoreAuth(store, auth, nil, nil)

	result, err := svc.BeginSnapStore(context.Background())
	if err != nil {
		t.Fatalf("BeginSnapStore() error = %v", err)
	}
	if result.RootMacaroon != "root-mac" {
		t.Fatalf("BeginSnapStore() RootMacaroon = %q, want root-mac", result.RootMacaroon)
	}
}

func TestSaveSnapStoreCredentialSavesCredentials(t *testing.T) {
	store := &fakeStoreCredentialStore{}
	svc := newServiceWithStoreAuth(store, &fakeStoreAuthenticator{
		flow: &sa.PendingAuthFlow{RootMacaroon: "root-mac"},
	}, nil, nil)

	result, err := svc.SaveSnapStoreCredential(context.Background(), "bound-credential")
	if err != nil {
		t.Fatalf("SaveSnapStoreCredential() error = %v", err)
	}
	if !result.SnapStore.Authenticated {
		t.Fatal("expected authenticated status after save")
	}
	if store.record == nil || store.record.Macaroon != "bound-credential" {
		t.Fatalf("expected credential to be saved, got %+v", store.record)
	}
}

func TestBeginSnapStoreRejectsEnvironmentCredentials(t *testing.T) {
	store := &fakeStoreCredentialStore{
		record: &dto.StoreCredentialRecord{Macaroon: "env-mac", Source: "environment"},
	}
	auth := &fakeStoreAuthenticator{
		flow: &sa.PendingAuthFlow{RootMacaroon: "root-mac"},
	}
	svc := newServiceWithStoreAuth(store, auth, nil, nil)

	_, err := svc.BeginSnapStore(context.Background())
	if !errors.Is(err, ErrSnapStoreEnvironmentCredentials) {
		t.Fatalf("BeginSnapStore() error = %v, want %v", err, ErrSnapStoreEnvironmentCredentials)
	}
}

func TestBeginSnapStoreRejectsNilAuthenticator(t *testing.T) {
	svc := newServiceWithStores(nil, nil)

	_, err := svc.BeginSnapStore(context.Background())
	if err == nil {
		t.Fatal("expected error when authenticator is nil")
	}
}

func TestBeginSnapStoreRejectsNilStore(t *testing.T) {
	auth := &fakeStoreAuthenticator{
		flow: &sa.PendingAuthFlow{RootMacaroon: "root-mac"},
	}
	svc := NewServiceWithStores(
		&fakeCredentialStore{},
		&fakeFlowStore{},
		&fakeLaunchpadAuthenticator{},
		nil, nil, nil, nil,
		nil, auth,
		nil, nil,
		testLogger(),
	)

	_, err := svc.BeginSnapStore(context.Background())
	if err == nil {
		t.Fatal("expected error when credential store is nil")
	}
}

func TestSaveSnapStoreCredentialRejectsNilStore(t *testing.T) {
	svc := newServiceWithStores(nil, nil)

	_, err := svc.SaveSnapStoreCredential(context.Background(), "credential")
	if err == nil {
		t.Fatal("expected error when credential store is nil")
	}
}

func TestLogoutSnapStoreClearsCredentials(t *testing.T) {
	store := &fakeStoreCredentialStore{
		record: &dto.StoreCredentialRecord{Macaroon: "mac", Source: "file", Path: filepath.Join(t.TempDir(), "store-creds.json")},
	}
	svc := newServiceWithStores(store, nil)

	result, err := svc.LogoutSnapStore(context.Background())
	if err != nil {
		t.Fatalf("LogoutSnapStore() error = %v", err)
	}
	if !result.Cleared {
		t.Fatal("expected cleared result")
	}
	if store.record != nil {
		t.Fatal("expected store record to be cleared")
	}
}

func TestLogoutSnapStoreRejectsEnvironmentCredentials(t *testing.T) {
	store := &fakeStoreCredentialStore{
		record: &dto.StoreCredentialRecord{Macaroon: "mac", Source: "environment"},
	}
	svc := newServiceWithStores(store, nil)

	_, err := svc.LogoutSnapStore(context.Background())
	if !errors.Is(err, ErrSnapStoreEnvironmentCredentials) {
		t.Fatalf("LogoutSnapStore() error = %v, want %v", err, ErrSnapStoreEnvironmentCredentials)
	}
}

func TestLogoutSnapStoreNoCredentials(t *testing.T) {
	store := &fakeStoreCredentialStore{}
	svc := newServiceWithStores(store, nil)

	result, err := svc.LogoutSnapStore(context.Background())
	if err != nil {
		t.Fatalf("LogoutSnapStore() error = %v", err)
	}
	if result.Cleared {
		t.Fatal("expected not cleared when no credentials exist")
	}
}

func TestBeginCharmhubReturnsRootMacaroon(t *testing.T) {
	store := &fakeCharmhubCredentialStore{}
	auth := &fakeStoreAuthenticator{
		flow: &sa.PendingAuthFlow{
			RootMacaroon: "root-mac",
		},
	}
	svc := newServiceWithStoreAuth(nil, nil, store, auth)

	result, err := svc.BeginCharmhub(context.Background())
	if err != nil {
		t.Fatalf("BeginCharmhub() error = %v", err)
	}
	if result.RootMacaroon != "root-mac" {
		t.Fatalf("BeginCharmhub() RootMacaroon = %q, want root-mac", result.RootMacaroon)
	}
}

func TestSaveCharmhubCredentialExchangesAndSavesToken(t *testing.T) {
	store := &fakeCharmhubCredentialStore{}
	auth := &fakeStoreAuthenticator{
		flow:          &sa.PendingAuthFlow{RootMacaroon: "root-mac"},
		exchangeToken: "exchanged-publisher-token",
	}
	svc := newServiceWithStoreAuth(nil, nil, store, auth)

	result, err := svc.SaveCharmhubCredential(context.Background(), "bound-credential")
	if err != nil {
		t.Fatalf("SaveCharmhubCredential() error = %v", err)
	}
	if !result.Charmhub.Authenticated {
		t.Fatal("expected authenticated status after save")
	}
	if auth.exchangeCalls != 1 {
		t.Fatalf("expected 1 exchange call, got %d", auth.exchangeCalls)
	}
	if auth.lastExchangeArg != "bound-credential" {
		t.Fatalf("ExchangeToken called with %q, want bound-credential", auth.lastExchangeArg)
	}
	if store.record == nil || store.record.Macaroon != "exchanged-publisher-token" {
		t.Fatalf("expected exchanged token to be saved, got %+v", store.record)
	}
	if store.record.DischargedBundle != "bound-credential" {
		t.Fatalf("expected discharged bundle to be persisted, got %q", store.record.DischargedBundle)
	}
}

func TestSaveCharmhubCredentialPropagatesExchangeError(t *testing.T) {
	store := &fakeCharmhubCredentialStore{}
	auth := &fakeStoreAuthenticator{
		flow:        &sa.PendingAuthFlow{RootMacaroon: "root-mac"},
		exchangeErr: errors.New("boom"),
	}
	svc := newServiceWithStoreAuth(nil, nil, store, auth)

	_, err := svc.SaveCharmhubCredential(context.Background(), "bound-credential")
	if err == nil {
		t.Fatal("expected error when exchange fails")
	}
	if store.record != nil {
		t.Fatalf("expected store to be untouched on exchange failure, got %+v", store.record)
	}
}

func TestBeginCharmhubRejectsEnvironmentCredentials(t *testing.T) {
	store := &fakeCharmhubCredentialStore{
		record: &dto.StoreCredentialRecord{Macaroon: "env-mac", Source: "environment"},
	}
	auth := &fakeStoreAuthenticator{
		flow: &sa.PendingAuthFlow{RootMacaroon: "root-mac"},
	}
	svc := newServiceWithStoreAuth(nil, nil, store, auth)

	_, err := svc.BeginCharmhub(context.Background())
	if !errors.Is(err, ErrCharmhubEnvironmentCredentials) {
		t.Fatalf("BeginCharmhub() error = %v, want %v", err, ErrCharmhubEnvironmentCredentials)
	}
}

func TestBeginCharmhubRejectsNilAuthenticator(t *testing.T) {
	svc := newServiceWithStores(nil, nil)

	_, err := svc.BeginCharmhub(context.Background())
	if err == nil {
		t.Fatal("expected error when authenticator is nil")
	}
}

func TestBeginCharmhubRejectsNilStore(t *testing.T) {
	auth := &fakeStoreAuthenticator{
		flow: &sa.PendingAuthFlow{RootMacaroon: "root-mac"},
	}
	svc := NewServiceWithStores(
		&fakeCredentialStore{},
		&fakeFlowStore{},
		&fakeLaunchpadAuthenticator{},
		nil, nil, nil, nil,
		nil, nil,
		nil, auth,
		testLogger(),
	)

	_, err := svc.BeginCharmhub(context.Background())
	if err == nil {
		t.Fatal("expected error when credential store is nil")
	}
}

func TestSaveCharmhubCredentialRejectsNilStore(t *testing.T) {
	svc := newServiceWithStores(nil, nil)

	_, err := svc.SaveCharmhubCredential(context.Background(), "credential")
	if err == nil {
		t.Fatal("expected error when credential store is nil")
	}
}

func TestLogoutCharmhubClearsCredentials(t *testing.T) {
	store := &fakeCharmhubCredentialStore{
		record: &dto.StoreCredentialRecord{Macaroon: "mac", Source: "file", Path: filepath.Join(t.TempDir(), "charmhub-creds.json")},
	}
	svc := newServiceWithStores(nil, store)

	result, err := svc.LogoutCharmhub(context.Background())
	if err != nil {
		t.Fatalf("LogoutCharmhub() error = %v", err)
	}
	if !result.Cleared {
		t.Fatal("expected cleared result")
	}
	if store.record != nil {
		t.Fatal("expected store record to be cleared")
	}
}

func TestLogoutCharmhubRejectsEnvironmentCredentials(t *testing.T) {
	store := &fakeCharmhubCredentialStore{
		record: &dto.StoreCredentialRecord{Macaroon: "mac", Source: "environment"},
	}
	svc := newServiceWithStores(nil, store)

	_, err := svc.LogoutCharmhub(context.Background())
	if !errors.Is(err, ErrCharmhubEnvironmentCredentials) {
		t.Fatalf("LogoutCharmhub() error = %v, want %v", err, ErrCharmhubEnvironmentCredentials)
	}
}

func TestLogoutCharmhubNoCredentials(t *testing.T) {
	store := &fakeCharmhubCredentialStore{}
	svc := newServiceWithStores(nil, store)

	result, err := svc.LogoutCharmhub(context.Background())
	if err != nil {
		t.Fatalf("LogoutCharmhub() error = %v", err)
	}
	if result.Cleared {
		t.Fatal("expected not cleared when no credentials exist")
	}
}

func TestStatusReportsStoreAuthentication(t *testing.T) {
	snapStore := &fakeStoreCredentialStore{
		record: &dto.StoreCredentialRecord{Macaroon: "snap-mac", Source: "file", Path: filepath.Join(t.TempDir(), "snap.json")},
	}
	charmhubStore := &fakeCharmhubCredentialStore{
		record: &dto.StoreCredentialRecord{Macaroon: "charm-mac", Source: "environment"},
	}
	svc := newServiceWithStores(snapStore, charmhubStore)

	status, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.SnapStore.Authenticated || status.SnapStore.Source != "file" {
		t.Fatalf("Status().SnapStore = %+v", status.SnapStore)
	}
	if !status.Charmhub.Authenticated || status.Charmhub.Source != "environment" {
		t.Fatalf("Status().Charmhub = %+v", status.Charmhub)
	}
}

func TestStatusReportsUnauthenticatedStoresWhenNil(t *testing.T) {
	svc := newServiceWithStores(nil, nil)

	status, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.SnapStore.Authenticated {
		t.Fatal("expected unauthenticated Snap Store when store is nil")
	}
	if status.Charmhub.Authenticated {
		t.Fatal("expected unauthenticated Charmhub when store is nil")
	}
}
