// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package launchpad

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

type projectRoundTripFunc func(*http.Request) (*http.Response, error)

func (f projectRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func withProjectTransport(t *testing.T, transport http.RoundTripper) {
	t.Helper()
	orig := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() {
		http.DefaultTransport = orig
	})
}

func projectResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func newProjectManagerForTest() *ProjectManager {
	client := lp.NewClient(&lp.Credentials{
		ConsumerKey:       "test-app",
		AccessToken:       "token",
		AccessTokenSecret: "secret",
	}, nil)
	return NewProjectManager(client)
}

func TestProjectManagerGetProject(t *testing.T) {
	withProjectTransport(t, projectRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.Host != "api.launchpad.net" || req.URL.Path != "/devel/sunbeam" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
		}
		return projectResponse(http.StatusOK, `{"name":"sunbeam","self_link":"https://api.launchpad.net/devel/sunbeam","development_focus_link":"https://api.launchpad.net/devel/sunbeam/2025.1"}`), nil
	}))

	project, err := newProjectManagerForTest().GetProject(context.Background(), "sunbeam")
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if project.Name != "sunbeam" || project.SelfLink == "" || project.DevelopmentFocusLink == "" {
		t.Fatalf("unexpected project: %+v", project)
	}
}

func TestProjectManagerGetProjectSeries(t *testing.T) {
	withProjectTransport(t, projectRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.Host != "api.launchpad.net" || req.URL.Path != "/devel/sunbeam/series" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
		}
		return projectResponse(http.StatusOK, `{"total_size":2,"entries":[{"name":"2024.1","self_link":"https://api.launchpad.net/devel/sunbeam/2024.1","active":true},{"name":"2025.1","self_link":"https://api.launchpad.net/devel/sunbeam/2025.1","active":false}]}`), nil
	}))

	series, err := newProjectManagerForTest().GetProjectSeries(context.Background(), "sunbeam")
	if err != nil {
		t.Fatalf("GetProjectSeries() error = %v", err)
	}
	if len(series) != 2 {
		t.Fatalf("len(series) = %d, want 2", len(series))
	}
	if series[0].Name != "2024.1" || !series[0].Active {
		t.Fatalf("unexpected first series: %+v", series[0])
	}
}

func TestProjectManagerCreateSeries(t *testing.T) {
	withProjectTransport(t, projectRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodPost && req.URL.Host == "api.launchpad.net" && req.URL.Path == "/devel/sunbeam":
			if err := req.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			if got := req.FormValue("ws.op"); got != "newSeries" {
				t.Fatalf("ws.op = %q, want newSeries", got)
			}
			if got := req.FormValue("name"); got != "2025.1" {
				t.Fatalf("name = %q, want 2025.1", got)
			}
			return projectResponse(http.StatusCreated, ""), nil
		case req.Method == http.MethodGet && req.URL.Path == "/devel/sunbeam/2025.1":
			return projectResponse(http.StatusOK, `{"name":"2025.1","self_link":"https://api.launchpad.net/devel/sunbeam/2025.1","active":true}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	}))

	series, err := newProjectManagerForTest().CreateSeries(context.Background(), "sunbeam", "2025.1", "2025.1 series")
	if err != nil {
		t.Fatalf("CreateSeries() error = %v", err)
	}
	if series.Name != "2025.1" || series.SelfLink == "" {
		t.Fatalf("unexpected series: %+v", series)
	}
}

func TestProjectManagerSetDevelopmentFocus(t *testing.T) {
	withProjectTransport(t, projectRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPatch || req.URL.Host != "api.launchpad.net" || req.URL.Path != "/devel/sunbeam" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
		}
		var payload map[string]string
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if payload["development_focus_link"] != "https://api.launchpad.net/devel/sunbeam/2025.1" {
			t.Fatalf("unexpected payload: %+v", payload)
		}
		return projectResponse(http.StatusOK, `{"status":"ok"}`), nil
	}))

	if err := newProjectManagerForTest().SetDevelopmentFocus(context.Background(), "sunbeam", "https://api.launchpad.net/devel/sunbeam/2025.1"); err != nil {
		t.Fatalf("SetDevelopmentFocus() error = %v", err)
	}
}

func TestProjectManagerGetProjectSeries_ErrorWrap(t *testing.T) {
	withProjectTransport(t, projectRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return projectResponse(http.StatusInternalServerError, "boom"), nil
	}))

	_, err := newProjectManagerForTest().GetProjectSeries(context.Background(), "sunbeam")
	if err == nil {
		t.Fatal("GetProjectSeries() error = nil, want wrapped error")
	}
	if !strings.Contains(err.Error(), "fetching series for sunbeam") {
		t.Fatalf("unexpected error: %v", err)
	}
}
