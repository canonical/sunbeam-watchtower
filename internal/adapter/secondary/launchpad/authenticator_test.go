// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package launchpad

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func withDefaultTransport(t *testing.T, transport http.RoundTripper) {
	t.Helper()
	orig := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() {
		http.DefaultTransport = orig
	})
}

func textResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNewAuthenticator_DefaultConsumerKey(t *testing.T) {
	auth := NewAuthenticator("", nil)
	if got := auth.ConsumerKey(); got != lp.ConsumerKey() {
		t.Fatalf("ConsumerKey() = %q, want %q", got, lp.ConsumerKey())
	}
}

func TestAuthenticatorObtainRequestToken(t *testing.T) {
	withDefaultTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", req.Method)
		}
		if req.URL.Host != "launchpad.net" || req.URL.Path != "/+request-token" {
			t.Fatalf("unexpected URL: %s", req.URL.String())
		}
		if err := req.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		if got := req.FormValue("oauth_consumer_key"); got != "test-app" {
			t.Fatalf("oauth_consumer_key = %q, want test-app", got)
		}
		return textResponse(http.StatusOK, "oauth_token=req-token&oauth_token_secret=req-secret"), nil
	}))

	token, err := NewAuthenticator("test-app", discardLogger()).ObtainRequestToken(context.Background())
	if err != nil {
		t.Fatalf("ObtainRequestToken() error = %v", err)
	}
	if token.Token != "req-token" || token.TokenSecret != "req-secret" {
		t.Fatalf("unexpected request token: %+v", token)
	}
}

func TestAuthenticatorExchangeAccessToken(t *testing.T) {
	withDefaultTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", req.Method)
		}
		if req.URL.Host != "launchpad.net" || req.URL.Path != "/+access-token" {
			t.Fatalf("unexpected URL: %s", req.URL.String())
		}
		if err := req.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		if got := req.FormValue("oauth_consumer_key"); got != "test-app" {
			t.Fatalf("oauth_consumer_key = %q, want test-app", got)
		}
		if got := req.FormValue("oauth_signature"); got != "&req-secret" {
			t.Fatalf("oauth_signature = %q, want &req-secret", got)
		}
		return textResponse(http.StatusOK, "oauth_token=access-token&oauth_token_secret=access-secret"), nil
	}))

	creds, err := NewAuthenticator("test-app", discardLogger()).ExchangeAccessToken(context.Background(), &lp.RequestToken{
		Token:       "req-token",
		TokenSecret: "req-secret",
	})
	if err != nil {
		t.Fatalf("ExchangeAccessToken() error = %v", err)
	}
	if creds.AccessToken != "access-token" || creds.AccessTokenSecret != "access-secret" {
		t.Fatalf("unexpected credentials: %+v", creds)
	}
}

func TestAuthenticatorCurrentUser(t *testing.T) {
	withDefaultTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", req.Method)
		}
		if req.URL.Host != "api.launchpad.net" || req.URL.Path != "/devel/people/+me" {
			t.Fatalf("unexpected URL: %s", req.URL.String())
		}
		authHeader := req.Header.Get("Authorization")
		if !strings.Contains(authHeader, `oauth_token="access-token"`) {
			t.Fatalf("Authorization header missing oauth token: %s", authHeader)
		}
		return textResponse(http.StatusOK, `{"name":"jdoe","display_name":"Jane Doe"}`), nil
	}))

	person, err := NewAuthenticator("test-app", discardLogger()).CurrentUser(context.Background(), &lp.Credentials{
		ConsumerKey:       "test-app",
		AccessToken:       "access-token",
		AccessTokenSecret: "access-secret",
	})
	if err != nil {
		t.Fatalf("CurrentUser() error = %v", err)
	}
	if person.Name != "jdoe" || person.DisplayName != "Jane Doe" {
		t.Fatalf("unexpected person: %+v", person)
	}
}

func TestAuthenticatorCurrentUser_HTTPError(t *testing.T) {
	withDefaultTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return textResponse(http.StatusUnauthorized, "unauthorized"), nil
	}))

	_, err := NewAuthenticator("test-app", discardLogger()).CurrentUser(context.Background(), &lp.Credentials{
		ConsumerKey:       "test-app",
		AccessToken:       "access-token",
		AccessTokenSecret: "access-secret",
	})
	if err == nil {
		t.Fatal("CurrentUser() error = nil, want HTTP error")
	}
}
