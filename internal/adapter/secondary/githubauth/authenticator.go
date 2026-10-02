// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package githubauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/go-github/v68/github"

	gh "github.com/canonical/sunbeam-watchtower/pkg/github/v1"
)

const (
	deviceCodeURL  = "https://github.com/login/device/code"
	accessTokenURL = "https://github.com/login/oauth/access_token"
)

type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type accessTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`

	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	ErrorURI         string `json:"error_uri"`
}

// Authenticator bridges the core auth service to the GitHub device-flow API.
type Authenticator struct {
	clientID   string
	logger     *slog.Logger
	httpClient *http.Client
}

// NewAuthenticator creates a GitHub device-flow authenticator adapter.
func NewAuthenticator(clientID string, logger *slog.Logger, httpClient *http.Client) *Authenticator {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Authenticator{clientID: clientID, logger: logger, httpClient: httpClient}
}

// ClientID returns the configured GitHub OAuth app client ID.
func (a *Authenticator) ClientID() string {
	return a.clientID
}

// BeginDeviceFlow starts the GitHub device flow.
func (a *Authenticator) BeginDeviceFlow(ctx context.Context) (*gh.PendingAuthFlow, error) {
	values := url.Values{}
	values.Set("client_id", a.clientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deviceCodeURL, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating device flow request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var payload deviceCodeResponse
	if err := a.doJSON(req, &payload); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	return &gh.PendingAuthFlow{
		DeviceCode:      payload.DeviceCode,
		UserCode:        payload.UserCode,
		VerificationURI: payload.VerificationURI,
		IntervalSeconds: payload.Interval,
		CreatedAt:       now,
		ExpiresAt:       now.Add(time.Duration(payload.ExpiresIn) * time.Second),
	}, nil
}

// PollAccessToken waits for the GitHub device flow to complete.
func (a *Authenticator) PollAccessToken(ctx context.Context, flow *gh.PendingAuthFlow) (*gh.Credentials, error) {
	interval := flow.IntervalSeconds
	if interval <= 0 {
		interval = 5
	}
	firstAttempt := true

	for {
		if !firstAttempt {
			timer := time.NewTimer(time.Duration(interval) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
		firstAttempt = false

		resp, err := a.exchangeDeviceCode(ctx, flow.DeviceCode)
		if err != nil {
			switch {
			case errors.Is(err, gh.ErrAuthorizationPending):
				continue
			case errors.Is(err, gh.ErrSlowDown):
				interval += 5
				continue
			default:
				return nil, err
			}
		}
		return resp, nil
	}
}

// CurrentUser returns the authenticated GitHub identity for the given credentials.
func (a *Authenticator) CurrentUser(ctx context.Context, creds *gh.Credentials) (gh.User, error) {
	client := github.NewClient(a.httpClient).WithAuthToken(creds.AccessToken)
	user, _, err := client.Users.Get(ctx, "")
	if err != nil {
		return gh.User{}, err
	}
	return gh.User{Login: user.GetLogin(), Name: user.GetName()}, nil
}

func (a *Authenticator) exchangeDeviceCode(ctx context.Context, deviceCode string) (*gh.Credentials, error) {
	values := url.Values{}
	values.Set("client_id", a.clientID)
	values.Set("device_code", deviceCode)
	values.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, accessTokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating access token request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var payload accessTokenResponse
	if err := a.doJSON(req, &payload); err != nil {
		return nil, err
	}

	switch payload.Error {
	case "":
		return &gh.Credentials{
			AccessToken: payload.AccessToken,
			TokenType:   payload.TokenType,
			Scope:       payload.Scope,
		}, nil
	case "authorization_pending":
		return nil, gh.ErrAuthorizationPending
	case "slow_down":
		return nil, gh.ErrSlowDown
	case "access_denied":
		return nil, gh.ErrAccessDenied
	case "expired_token":
		return nil, gh.ErrExpiredToken
	case "incorrect_device_code":
		return nil, gh.ErrIncorrectDeviceCode
	default:
		if payload.ErrorDescription != "" {
			return nil, fmt.Errorf("%s: %s", payload.Error, payload.ErrorDescription)
		}
		return nil, fmt.Errorf("github token exchange failed: %s", payload.Error)
	}
}

func (a *Authenticator) doJSON(req *http.Request, result any) error {
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing GitHub auth request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("GitHub auth request failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("decoding GitHub auth response: %w", err)
	}
	return nil
}
