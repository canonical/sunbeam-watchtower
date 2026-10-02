// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	gh "github.com/canonical/sunbeam-watchtower/pkg/github/v1"
	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

const (
	// DefaultLaunchpadFlowTTL is the default lifetime of a pending auth flow.
	DefaultLaunchpadFlowTTL = 10 * time.Minute
)

var (
	// ErrLaunchpadAuthFlowNotFound indicates the requested pending auth flow was not found.
	ErrLaunchpadAuthFlowNotFound = errors.New("launchpad auth flow not found")
	// ErrLaunchpadAuthFlowExpired indicates the requested pending auth flow has expired.
	ErrLaunchpadAuthFlowExpired = errors.New("launchpad auth flow expired")
	// ErrLaunchpadEnvironmentCredentials indicates logout cannot clear env-provided credentials.
	ErrLaunchpadEnvironmentCredentials = errors.New(
		"launchpad credentials are provided by environment variables and cannot be removed",
	)
	// ErrGitHubAuthFlowNotFound indicates the requested pending auth flow was not found.
	ErrGitHubAuthFlowNotFound = errors.New("github auth flow not found")
	// ErrGitHubAuthFlowExpired indicates the requested pending auth flow has expired.
	ErrGitHubAuthFlowExpired = errors.New("github auth flow expired")
	// ErrGitHubEnvironmentCredentials indicates login/logout cannot alter env-provided credentials.
	ErrGitHubEnvironmentCredentials = errors.New(
		"github credentials are provided by environment variables and cannot be modified",
	)
	// ErrGitHubClientIDRequired indicates GitHub device flow cannot start without a client ID.
	ErrGitHubClientIDRequired = errors.New("github oauth client id is required")
	// ErrGitHubAccessDenied indicates the device flow was denied by the user.
	ErrGitHubAccessDenied = errors.New("github device flow access denied")
	// ErrGitHubKeyringNotImplemented indicates github.use_keyring is not yet supported.
	ErrGitHubKeyringNotImplemented = errors.New("github keyring storage is not implemented yet")
	// ErrSnapStoreEnvironmentCredentials indicates login/logout cannot alter env-provided credentials.
	ErrSnapStoreEnvironmentCredentials = errors.New(
		"snap store credentials are provided by environment variables and cannot be modified",
	)
	// ErrCharmhubEnvironmentCredentials indicates login/logout cannot alter env-provided credentials.
	ErrCharmhubEnvironmentCredentials = errors.New(
		"charmhub credentials are provided by environment variables and cannot be modified",
	)
)

// Service exposes application-surface authentication workflows.
type Service struct {
	launchpadStore port.LaunchpadCredentialStore
	launchpadFlows port.LaunchpadPendingAuthFlowStore
	launchpadAuth  port.LaunchpadAuthenticator

	githubStore      port.GitHubCredentialStore
	githubFlows      port.GitHubPendingAuthFlowStore
	githubAuth       port.GitHubAuthenticator
	githubMutableErr error

	snapStoreStore port.SnapStoreCredentialStore
	snapStoreAuth  port.SnapStoreAuthenticator
	charmhubStore  port.CharmhubCredentialStore
	charmhubAuth   port.CharmhubAuthenticator

	logger    *slog.Logger
	now       func() time.Time
	newFlowID func() (string, error)
	flowTTL   time.Duration
}

// NewService creates a new auth service with Launchpad support only.
func NewService(
	launchpadStore port.LaunchpadCredentialStore,
	launchpadFlows port.LaunchpadPendingAuthFlowStore,
	launchpadAuth port.LaunchpadAuthenticator,
	logger *slog.Logger,
) *Service {
	return NewServiceWithGitHub(
		launchpadStore,
		launchpadFlows,
		launchpadAuth,
		nil,
		nil,
		nil,
		nil,
		logger,
	)
}

// NewServiceWithGitHub creates a new auth service with both Launchpad and GitHub support.
func NewServiceWithGitHub(
	launchpadStore port.LaunchpadCredentialStore,
	launchpadFlows port.LaunchpadPendingAuthFlowStore,
	launchpadAuth port.LaunchpadAuthenticator,
	githubStore port.GitHubCredentialStore,
	githubFlows port.GitHubPendingAuthFlowStore,
	githubAuth port.GitHubAuthenticator,
	githubMutableErr error,
	logger *slog.Logger,
) *Service {
	return NewServiceWithStores(
		launchpadStore, launchpadFlows, launchpadAuth,
		githubStore, githubFlows, githubAuth, githubMutableErr,
		nil, nil,
		nil, nil,
		logger,
	)
}

// NewServiceWithStores creates a new auth service with Launchpad, GitHub, and store auth support.
func NewServiceWithStores(
	launchpadStore port.LaunchpadCredentialStore,
	launchpadFlows port.LaunchpadPendingAuthFlowStore,
	launchpadAuth port.LaunchpadAuthenticator,
	githubStore port.GitHubCredentialStore,
	githubFlows port.GitHubPendingAuthFlowStore,
	githubAuth port.GitHubAuthenticator,
	githubMutableErr error,
	snapStoreStore port.SnapStoreCredentialStore,
	snapStoreAuth port.SnapStoreAuthenticator,
	charmhubStore port.CharmhubCredentialStore,
	charmhubAuth port.CharmhubAuthenticator,
	logger *slog.Logger,
) *Service {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Service{
		launchpadStore:   launchpadStore,
		launchpadFlows:   launchpadFlows,
		launchpadAuth:    launchpadAuth,
		githubStore:      githubStore,
		githubFlows:      githubFlows,
		githubAuth:       githubAuth,
		githubMutableErr: githubMutableErr,
		snapStoreStore:   snapStoreStore,
		snapStoreAuth:    snapStoreAuth,
		charmhubStore:    charmhubStore,
		charmhubAuth:     charmhubAuth,
		logger:           logger,
		now:              time.Now,
		newFlowID:        randomFlowID,
		flowTTL:          DefaultLaunchpadFlowTTL,
	}
}

// Status returns the current authentication status.
func (s *Service) Status(ctx context.Context) (*dto.AuthStatus, error) {
	launchpadRecord, err := s.launchpadStore.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading launchpad credentials: %w", err)
	}
	var githubRecord *gh.CredentialRecord
	if s.githubStore != nil {
		githubRecord, err = s.githubStore.Load(ctx)
		if err != nil {
			return nil, fmt.Errorf("loading github credentials: %w", err)
		}
	}

	launchpadStatus, err := s.launchpadStatus(ctx, launchpadRecord)
	if err != nil {
		return nil, err
	}
	githubStatus, err := s.githubStatus(ctx, githubRecord)
	if err != nil {
		return nil, err
	}

	snapStoreStatus := s.snapStoreStatus(ctx)
	charmhubStatus := s.charmhubStatus(ctx)

	return &dto.AuthStatus{
		Launchpad: *launchpadStatus,
		GitHub:    *githubStatus,
		SnapStore: *snapStoreStatus,
		Charmhub:  *charmhubStatus,
	}, nil
}

// BeginLaunchpad starts a new Launchpad auth flow and stores its server-side secret state.
func (s *Service) BeginLaunchpad(ctx context.Context) (*dto.LaunchpadAuthBeginResult, error) {
	token, err := s.launchpadAuth.ObtainRequestToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("obtaining launchpad request token: %w", err)
	}

	flowID, err := s.newFlowID()
	if err != nil {
		return nil, fmt.Errorf("generating auth flow ID: %w", err)
	}

	now := s.now().UTC()
	flow := &lp.PendingAuthFlow{
		ID:                 flowID,
		RequestToken:       token.Token,
		RequestTokenSecret: token.TokenSecret,
		CreatedAt:          now,
		ExpiresAt:          now.Add(s.flowTTL),
	}
	if err := s.launchpadFlows.Put(ctx, flow); err != nil {
		return nil, fmt.Errorf("storing launchpad auth flow: %w", err)
	}

	return &dto.LaunchpadAuthBeginResult{
		FlowID:       flowID,
		AuthorizeURL: token.AuthorizeURL(),
		ExpiresAt:    flow.ExpiresAt,
	}, nil
}

// FinalizeLaunchpad completes a pending Launchpad auth flow and persists the resulting credentials.
func (s *Service) FinalizeLaunchpad(ctx context.Context, flowID string) (*dto.LaunchpadAuthFinalizeResult, error) {
	flow, err := s.launchpadFlows.Get(ctx, flowID)
	if err != nil {
		switch {
		case errors.Is(err, lp.ErrPendingAuthFlowNotFound):
			return nil, ErrLaunchpadAuthFlowNotFound
		case errors.Is(err, lp.ErrPendingAuthFlowExpired):
			return nil, ErrLaunchpadAuthFlowExpired
		default:
			return nil, fmt.Errorf("loading launchpad auth flow: %w", err)
		}
	}

	creds, err := s.launchpadAuth.ExchangeAccessToken(ctx, &lp.RequestToken{
		Token:       flow.RequestToken,
		TokenSecret: flow.RequestTokenSecret,
	})
	if err != nil {
		return nil, fmt.Errorf("exchanging launchpad access token: %w", err)
	}

	if err := s.launchpadFlows.Delete(ctx, flowID); err != nil {
		return nil, fmt.Errorf("deleting completed launchpad auth flow: %w", err)
	}

	record, err := s.launchpadStore.Save(ctx, creds)
	if err != nil {
		return nil, fmt.Errorf("saving launchpad credentials: %w", err)
	}

	launchpadStatus, err := s.launchpadStatus(ctx, record)
	if err != nil {
		return nil, err
	}

	return &dto.LaunchpadAuthFinalizeResult{Launchpad: *launchpadStatus}, nil
}

// LogoutLaunchpad clears persisted Launchpad credentials when they are file-backed.
func (s *Service) LogoutLaunchpad(ctx context.Context) (*dto.LaunchpadAuthLogoutResult, error) {
	record, err := s.launchpadStore.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading launchpad credentials: %w", err)
	}
	if record == nil {
		return &dto.LaunchpadAuthLogoutResult{}, nil
	}
	if record.Source == lp.CredentialSourceEnvironment {
		return nil, ErrLaunchpadEnvironmentCredentials
	}
	if err := s.launchpadStore.Clear(ctx); err != nil {
		return nil, fmt.Errorf("clearing launchpad credentials: %w", err)
	}

	return &dto.LaunchpadAuthLogoutResult{
		Cleared:         true,
		CredentialsPath: record.Path,
	}, nil
}

// BeginGitHub starts a new GitHub device flow and stores its server-side state.
func (s *Service) BeginGitHub(ctx context.Context) (*dto.GitHubAuthBeginResult, error) {
	if s.githubMutableErr != nil {
		return nil, s.githubMutableErr
	}
	if s.githubAuth == nil || s.githubAuth.ClientID() == "" {
		return nil, ErrGitHubClientIDRequired
	}
	record, err := s.githubStore.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading github credentials: %w", err)
	}
	if record != nil && record.Source == gh.CredentialSourceEnvironment {
		return nil, ErrGitHubEnvironmentCredentials
	}

	flow, err := s.githubAuth.BeginDeviceFlow(ctx)
	if err != nil {
		return nil, fmt.Errorf("starting github device flow: %w", err)
	}
	flowID, err := s.newFlowID()
	if err != nil {
		return nil, fmt.Errorf("generating auth flow ID: %w", err)
	}
	flow.ID = flowID
	if err := s.githubFlows.Put(ctx, flow); err != nil {
		return nil, fmt.Errorf("storing github auth flow: %w", err)
	}

	return &dto.GitHubAuthBeginResult{
		FlowID:          flow.ID,
		UserCode:        flow.UserCode,
		VerificationURI: flow.VerificationURI,
		IntervalSeconds: flow.IntervalSeconds,
		ExpiresAt:       flow.ExpiresAt,
	}, nil
}

// FinalizeGitHub completes a pending GitHub device flow and persists credentials.
func (s *Service) FinalizeGitHub(ctx context.Context, flowID string) (*dto.GitHubAuthFinalizeResult, error) {
	if s.githubMutableErr != nil {
		return nil, s.githubMutableErr
	}
	flow, err := s.githubFlows.Get(ctx, flowID)
	if err != nil {
		switch {
		case errors.Is(err, gh.ErrPendingAuthFlowNotFound):
			return nil, ErrGitHubAuthFlowNotFound
		case errors.Is(err, gh.ErrPendingAuthFlowExpired):
			return nil, ErrGitHubAuthFlowExpired
		default:
			return nil, fmt.Errorf("loading github auth flow: %w", err)
		}
	}

	creds, err := s.githubAuth.PollAccessToken(ctx, flow)
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return nil, err
		case errors.Is(err, gh.ErrAccessDenied):
			_ = s.githubFlows.Delete(ctx, flowID)
			return nil, ErrGitHubAccessDenied
		case errors.Is(err, gh.ErrExpiredToken), errors.Is(err, gh.ErrIncorrectDeviceCode):
			_ = s.githubFlows.Delete(ctx, flowID)
			return nil, ErrGitHubAuthFlowExpired
		default:
			return nil, fmt.Errorf("exchanging github access token: %w", err)
		}
	}

	if err := s.githubFlows.Delete(ctx, flowID); err != nil {
		return nil, fmt.Errorf("deleting completed github auth flow: %w", err)
	}

	record, err := s.githubStore.Save(ctx, creds)
	if err != nil {
		return nil, fmt.Errorf("saving github credentials: %w", err)
	}
	githubStatus, err := s.githubStatus(ctx, record)
	if err != nil {
		return nil, err
	}

	return &dto.GitHubAuthFinalizeResult{GitHub: *githubStatus}, nil
}

// LogoutGitHub clears persisted GitHub credentials when they are file-backed.
func (s *Service) LogoutGitHub(ctx context.Context) (*dto.GitHubAuthLogoutResult, error) {
	if s.githubMutableErr != nil {
		return nil, s.githubMutableErr
	}
	record, err := s.githubStore.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading github credentials: %w", err)
	}
	if record == nil {
		return &dto.GitHubAuthLogoutResult{}, nil
	}
	if record.Source == gh.CredentialSourceEnvironment {
		return nil, ErrGitHubEnvironmentCredentials
	}
	if err := s.githubStore.Clear(ctx); err != nil {
		return nil, fmt.Errorf("clearing github credentials: %w", err)
	}

	return &dto.GitHubAuthLogoutResult{
		Cleared:         true,
		CredentialsPath: record.Path,
	}, nil
}

func (s *Service) launchpadStatus(
	ctx context.Context,
	record *lp.CredentialRecord,
) (*dto.LaunchpadAuthStatus, error) {
	status := &dto.LaunchpadAuthStatus{}
	if record == nil || record.Credentials == nil {
		return status, nil
	}

	status.Source = record.Source
	if record.Path != "" {
		status.CredentialsPath = record.Path
	}

	identity, statusError := s.lookupLaunchpadIdentity(ctx, record)
	if statusError != "" {
		status.Error = statusError
		return status, nil
	}

	status.Authenticated = true
	status.Username = identity.Name
	status.DisplayName = identity.DisplayName
	return status, nil
}

func (s *Service) githubStatus(
	ctx context.Context,
	record *gh.CredentialRecord,
) (*dto.GitHubAuthStatus, error) {
	status := &dto.GitHubAuthStatus{}
	if record == nil || record.Credentials == nil {
		return status, nil
	}

	status.Source = record.Source
	if record.Path != "" {
		status.CredentialsPath = record.Path
	}

	if s.githubAuth == nil {
		status.Error = "github authenticator unavailable"
		return status, nil
	}

	identity, statusError := s.lookupGitHubIdentity(ctx, record)
	if statusError != "" {
		status.Error = statusError
		return status, nil
	}

	status.Authenticated = true
	status.Username = identity.Login
	if identity.Name != "" {
		status.DisplayName = identity.Name
	} else {
		status.DisplayName = identity.Login
	}
	return status, nil
}

// BeginSnapStore requests a root macaroon from the Snap Store.
// The client must discharge this macaroon locally using httpbakery.
func (s *Service) BeginSnapStore(ctx context.Context) (*dto.SnapStoreAuthBeginResult, error) {
	if s.snapStoreAuth == nil {
		return nil, fmt.Errorf("snap store authenticator not configured")
	}
	if s.snapStoreStore == nil {
		return nil, fmt.Errorf("snap store credential store not configured")
	}
	record, err := s.snapStoreStore.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading snap store credentials: %w", err)
	}
	if record != nil && record.Source == "environment" {
		return nil, ErrSnapStoreEnvironmentCredentials
	}

	flow, err := s.snapStoreAuth.BeginAuth(ctx)
	if err != nil {
		return nil, fmt.Errorf("starting snap store auth: %w", err)
	}

	return &dto.SnapStoreAuthBeginResult{
		RootMacaroon: flow.RootMacaroon,
	}, nil
}

// SaveSnapStoreCredential persists a discharged Snap Store credential.
func (s *Service) SaveSnapStoreCredential(ctx context.Context, macaroon string) (*dto.SnapStoreAuthSaveResult, error) {
	if s.snapStoreStore == nil {
		return nil, fmt.Errorf("snap store credential store not configured")
	}

	saved, err := s.snapStoreStore.Save(ctx, macaroon)
	if err != nil {
		return nil, fmt.Errorf("saving snap store credentials: %w", err)
	}

	return &dto.SnapStoreAuthSaveResult{
		SnapStore: dto.SnapStoreAuthStatus{
			Authenticated:   true,
			Source:          saved.Source,
			CredentialsPath: saved.Path,
		},
	}, nil
}

// LogoutSnapStore clears persisted Snap Store credentials.
func (s *Service) LogoutSnapStore(ctx context.Context) (*dto.SnapStoreAuthLogoutResult, error) {
	if s.snapStoreStore == nil {
		return &dto.SnapStoreAuthLogoutResult{}, nil
	}
	record, err := s.snapStoreStore.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading snap store credentials: %w", err)
	}
	if record == nil {
		return &dto.SnapStoreAuthLogoutResult{}, nil
	}
	if record.Source == "environment" {
		return nil, ErrSnapStoreEnvironmentCredentials
	}
	if err := s.snapStoreStore.Clear(ctx); err != nil {
		return nil, fmt.Errorf("clearing snap store credentials: %w", err)
	}
	return &dto.SnapStoreAuthLogoutResult{
		Cleared:         true,
		CredentialsPath: record.Path,
	}, nil
}

// BeginCharmhub requests a root macaroon from Charmhub.
// The client must discharge this macaroon locally using httpbakery.
func (s *Service) BeginCharmhub(ctx context.Context) (*dto.CharmhubAuthBeginResult, error) {
	if s.charmhubAuth == nil {
		return nil, fmt.Errorf("charmhub authenticator not configured")
	}
	if s.charmhubStore == nil {
		return nil, fmt.Errorf("charmhub credential store not configured")
	}
	record, err := s.charmhubStore.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading charmhub credentials: %w", err)
	}
	if record != nil && record.Source == "environment" {
		return nil, ErrCharmhubEnvironmentCredentials
	}

	flow, err := s.charmhubAuth.BeginAuth(ctx)
	if err != nil {
		return nil, fmt.Errorf("starting charmhub auth: %w", err)
	}

	return &dto.CharmhubAuthBeginResult{
		RootMacaroon: flow.RootMacaroon,
	}, nil
}

// SaveCharmhubCredential persists a Charmhub credential.
//
// The caller passes the client-side discharged macaroon bundle. Charmhub's
// publisher API rejects the discharged root directly ("Invalid macaroon"), so
// we exchange it server-side for the short-lived publisher token and persist
// that instead — the stored macaroon is what every downstream /v1/charm/...
// caller sends as `Authorization: Macaroon <token>`.
func (s *Service) SaveCharmhubCredential(ctx context.Context, macaroon string) (*dto.CharmhubAuthSaveResult, error) {
	if s.charmhubStore == nil {
		return nil, fmt.Errorf("charmhub credential store not configured")
	}

	// The caller passes the discharged bundle; we keep it alongside the
	// exchanged token so expired-token refreshes can be silent.
	stored := macaroon
	if s.charmhubAuth != nil {
		exchanged, err := s.charmhubAuth.ExchangeToken(ctx, macaroon)
		if err != nil {
			return nil, fmt.Errorf("exchanging charmhub macaroon: %w", err)
		}
		stored = exchanged
	}

	saved, err := s.charmhubStore.Save(ctx, macaroon, stored)
	if err != nil {
		return nil, fmt.Errorf("saving charmhub credentials: %w", err)
	}

	return &dto.CharmhubAuthSaveResult{
		Charmhub: dto.CharmhubAuthStatus{
			Authenticated:   true,
			Source:          saved.Source,
			CredentialsPath: saved.Path,
		},
	}, nil
}

// LogoutCharmhub clears persisted Charmhub credentials.
func (s *Service) LogoutCharmhub(ctx context.Context) (*dto.CharmhubAuthLogoutResult, error) {
	if s.charmhubStore == nil {
		return &dto.CharmhubAuthLogoutResult{}, nil
	}
	record, err := s.charmhubStore.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading charmhub credentials: %w", err)
	}
	if record == nil {
		return &dto.CharmhubAuthLogoutResult{}, nil
	}
	if record.Source == "environment" {
		return nil, ErrCharmhubEnvironmentCredentials
	}
	if err := s.charmhubStore.Clear(ctx); err != nil {
		return nil, fmt.Errorf("clearing charmhub credentials: %w", err)
	}
	return &dto.CharmhubAuthLogoutResult{
		Cleared:         true,
		CredentialsPath: record.Path,
	}, nil
}

func (s *Service) snapStoreStatus(_ context.Context) *dto.SnapStoreAuthStatus {
	status := &dto.SnapStoreAuthStatus{}
	if s.snapStoreStore == nil {
		return status
	}
	record, err := s.snapStoreStore.Load(context.Background())
	if err != nil || record == nil {
		return status
	}
	status.Authenticated = true
	status.Source = record.Source
	status.CredentialsPath = record.Path
	return status
}

func (s *Service) charmhubStatus(_ context.Context) *dto.CharmhubAuthStatus {
	status := &dto.CharmhubAuthStatus{}
	if s.charmhubStore == nil {
		return status
	}
	record, err := s.charmhubStore.Load(context.Background())
	if err != nil || record == nil {
		return status
	}
	status.Authenticated = true
	status.Source = record.Source
	status.CredentialsPath = record.Path
	return status
}

func randomFlowID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

func (s *Service) lookupLaunchpadIdentity(
	ctx context.Context,
	record *lp.CredentialRecord,
) (lp.Person, string) {
	identity, err := s.launchpadAuth.CurrentUser(ctx, record.Credentials)
	if err != nil {
		return lp.Person{}, err.Error()
	}
	return identity, ""
}

func (s *Service) lookupGitHubIdentity(
	ctx context.Context,
	record *gh.CredentialRecord,
) (gh.User, string) {
	identity, err := s.githubAuth.CurrentUser(ctx, record.Credentials)
	if err != nil {
		return gh.User{}, err.Error()
	}
	return identity, ""
}
