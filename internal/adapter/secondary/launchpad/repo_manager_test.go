// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package launchpad

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

type repoRoundTripFunc func(*http.Request) (*http.Response, error)

func (f repoRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func withLaunchpadTransport(t *testing.T, transport http.RoundTripper) {
	t.Helper()
	orig := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() {
		http.DefaultTransport = orig
	})
}

func launchpadResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func newRepoManagerForTest() *RepoManager {
	client := lp.NewClient(&lp.Credentials{
		ConsumerKey:       "test-app",
		AccessToken:       "token",
		AccessTokenSecret: "secret",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return NewRepoManager(client, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestRepoManagerGetCurrentUser(t *testing.T) {
	withLaunchpadTransport(t, repoRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.Host != "api.launchpad.net" || req.URL.Path != "/devel/people/+me" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
		}
		return launchpadResponse(http.StatusOK, `{"name":"jdoe","display_name":"Jane Doe"}`), nil
	}))

	user, err := newRepoManagerForTest().GetCurrentUser(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentUser() error = %v", err)
	}
	if user != "jdoe" {
		t.Fatalf("GetCurrentUser() = %q, want jdoe", user)
	}
}

func TestRepoManagerGetDefaultRepo_DefaultBranchFallback(t *testing.T) {
	withLaunchpadTransport(t, repoRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.Host != "api.launchpad.net" || req.URL.Path != "/devel/+git" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
		}
		if got := req.URL.Query().Get("ws.op"); got != "getDefaultRepository" {
			t.Fatalf("ws.op = %q, want getDefaultRepository", got)
		}
		if got := req.URL.Query().Get("target"); got != "https://api.launchpad.net/devel/sunbeam" {
			t.Fatalf("target = %q", got)
		}
		return launchpadResponse(http.StatusOK, `{"self_link":"https://api.launchpad.net/devel/~team/sunbeam/+git/sunbeam","default_branch":""}`), nil
	}))

	repoSelfLink, defaultBranch, err := newRepoManagerForTest().GetDefaultRepo(context.Background(), "sunbeam")
	if err != nil {
		t.Fatalf("GetDefaultRepo() error = %v", err)
	}
	if repoSelfLink != "https://api.launchpad.net/devel/~team/sunbeam/+git/sunbeam" {
		t.Fatalf("repoSelfLink = %q", repoSelfLink)
	}
	if defaultBranch != "main" {
		t.Fatalf("defaultBranch = %q, want main", defaultBranch)
	}
}

func TestRepoManagerGetOrCreateProject_UsesExistingProject(t *testing.T) {
	withLaunchpadTransport(t, repoRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.Path != "/devel/owner-sunbeam-remote-build" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
		}
		return launchpadResponse(http.StatusOK, `{"name":"owner-sunbeam-remote-build"}`), nil
	}))

	project, err := newRepoManagerForTest().GetOrCreateProject(context.Background(), "owner")
	if err != nil {
		t.Fatalf("GetOrCreateProject() error = %v", err)
	}
	if project != "owner-sunbeam-remote-build" {
		t.Fatalf("project = %q", project)
	}
}

func TestRepoManagerGetOrCreateProject_CreatesMissingProject(t *testing.T) {
	var createCalled bool
	withLaunchpadTransport(t, repoRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/devel/owner-sunbeam-remote-build":
			if !createCalled {
				return launchpadResponse(http.StatusNotFound, "missing"), nil
			}
			return launchpadResponse(http.StatusOK, `{"name":"owner-sunbeam-remote-build"}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/devel/projects":
			createCalled = true
			if err := req.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			if got := req.FormValue("ws.op"); got != "new_project" {
				t.Fatalf("ws.op = %q, want new_project", got)
			}
			if got := req.FormValue("name"); got != "owner-sunbeam-remote-build" {
				t.Fatalf("name = %q", got)
			}
			return launchpadResponse(http.StatusCreated, ""), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	}))

	project, err := newRepoManagerForTest().GetOrCreateProject(context.Background(), "owner")
	if err != nil {
		t.Fatalf("GetOrCreateProject() error = %v", err)
	}
	if !createCalled {
		t.Fatal("expected project creation call")
	}
	if project != "owner-sunbeam-remote-build" {
		t.Fatalf("project = %q", project)
	}
}

func TestRepoManagerGetOrCreateRepo_CreatesMissingRepo(t *testing.T) {
	var createCalled bool
	withLaunchpadTransport(t, repoRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/devel/~owner/project/+git/repo":
			if !createCalled {
				return launchpadResponse(http.StatusNotFound, "missing"), nil
			}
			return launchpadResponse(http.StatusOK, `{"self_link":"https://api.launchpad.net/devel/~owner/project/+git/repo","git_ssh_url":"git+ssh://git.launchpad.net/~owner/project/+git/repo"}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/devel/+git":
			createCalled = true
			if err := req.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			if got := req.FormValue("ws.op"); got != "new" {
				t.Fatalf("ws.op = %q, want new", got)
			}
			if got := req.FormValue("owner"); got != "https://api.launchpad.net/devel/~owner" {
				t.Fatalf("owner = %q", got)
			}
			if got := req.FormValue("target"); got != "https://api.launchpad.net/devel/project" {
				t.Fatalf("target = %q", got)
			}
			if got := req.FormValue("name"); got != "repo" {
				t.Fatalf("name = %q", got)
			}
			return launchpadResponse(http.StatusCreated, ""), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	}))

	repoSelfLink, gitSSHURL, err := newRepoManagerForTest().GetOrCreateRepo(context.Background(), "owner", "project", "repo")
	if err != nil {
		t.Fatalf("GetOrCreateRepo() error = %v", err)
	}
	if !createCalled {
		t.Fatal("expected repo creation call")
	}
	if repoSelfLink != "https://api.launchpad.net/devel/~owner/project/+git/repo" {
		t.Fatalf("repoSelfLink = %q", repoSelfLink)
	}
	if gitSSHURL != "ssh://owner@git.launchpad.net/~owner/project/+git/repo" {
		t.Fatalf("gitSSHURL = %q", gitSSHURL)
	}
}

func TestRepoManagerGetOrCreateRepo_UsesExistingRepoAndNormalizesSSHURL(t *testing.T) {
	withLaunchpadTransport(t, repoRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.Path != "/devel/~owner/project/+git/repo" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
		}
		return launchpadResponse(http.StatusOK, `{"self_link":"https://api.launchpad.net/devel/~owner/project/+git/repo","git_ssh_url":"git+ssh://git.launchpad.net/~owner/project/+git/repo"}`), nil
	}))

	repoSelfLink, gitSSHURL, err := newRepoManagerForTest().GetOrCreateRepo(context.Background(), "owner", "project", "repo")
	if err != nil {
		t.Fatalf("GetOrCreateRepo() error = %v", err)
	}
	if repoSelfLink != "https://api.launchpad.net/devel/~owner/project/+git/repo" {
		t.Fatalf("repoSelfLink = %q", repoSelfLink)
	}
	if gitSSHURL != "ssh://owner@git.launchpad.net/~owner/project/+git/repo" {
		t.Fatalf("gitSSHURL = %q", gitSSHURL)
	}
}

func TestRepoManagerGetGitRef_ConstructsMissingSelfLink(t *testing.T) {
	withLaunchpadTransport(t, repoRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.Host != "api.launchpad.net" || req.URL.Path != "/devel/~owner/project/+git/repo" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
		}
		if got := req.URL.Query().Get("ws.op"); got != "getRefByPath" {
			t.Fatalf("ws.op = %q", got)
		}
		if got := req.URL.Query().Get("path"); got != "refs/heads/main" {
			t.Fatalf("path = %q", got)
		}
		return launchpadResponse(http.StatusOK, `{"path":"refs/heads/main","commit_sha1":"abc123"}`), nil
	}))

	refSelfLink, err := newRepoManagerForTest().GetGitRef(context.Background(), "https://api.launchpad.net/devel/~owner/project/+git/repo", "refs/heads/main")
	if err != nil {
		t.Fatalf("GetGitRef() error = %v", err)
	}
	want := "https://api.launchpad.net/devel/~owner/project/+git/repo/+ref/refs/heads/main"
	if refSelfLink != want {
		t.Fatalf("refSelfLink = %q, want %q", refSelfLink, want)
	}
}

func TestRepoManagerWaitForGitRef_ContextCancel(t *testing.T) {
	withLaunchpadTransport(t, repoRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return launchpadResponse(http.StatusNotFound, "missing"), nil
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := newRepoManagerForTest().WaitForGitRef(ctx, "https://api.launchpad.net/devel/~owner/project/+git/repo", "refs/heads/main", time.Minute)
	if err == nil {
		t.Fatal("WaitForGitRef() error = nil, want context error")
	}
	if err != context.Canceled {
		t.Fatalf("WaitForGitRef() error = %v, want %v", err, context.Canceled)
	}
}

func TestInjectSSHUser(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		lpUser string
		want   string
	}{
		{"normalizes git+ssh and adds user", "git+ssh://git.launchpad.net/~owner/project/+git/repo", "owner", "ssh://owner@git.launchpad.net/~owner/project/+git/repo"},
		{"keeps existing user", "ssh://alice@git.launchpad.net/~owner/project/+git/repo", "owner", "ssh://alice@git.launchpad.net/~owner/project/+git/repo"},
		{"returns invalid URL unchanged", "://bad-url", "owner", "://bad-url"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := injectSSHUser(tt.input, tt.lpUser); got != tt.want {
				t.Fatalf("injectSSHUser() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRepoManagerGetOrCreateRepo_CreateError(t *testing.T) {
	withLaunchpadTransport(t, repoRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.Method {
		case http.MethodGet:
			return launchpadResponse(http.StatusNotFound, "missing"), nil
		case http.MethodPost:
			return launchpadResponse(http.StatusBadRequest, "bad request"), nil
		default:
			return nil, fmt.Errorf("unexpected method: %s", req.Method)
		}
	}))

	_, _, err := newRepoManagerForTest().GetOrCreateRepo(context.Background(), "owner", "project", "repo")
	if err == nil {
		t.Fatal("GetOrCreateRepo() error = nil, want create error")
	}
}

func TestRepoManagerListBranches(t *testing.T) {
	withLaunchpadTransport(t, repoRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || !strings.HasSuffix(req.URL.Path, "/branches") {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
		}
		return launchpadResponse(http.StatusOK, `{
			"total_size": 2,
			"start": 0,
			"entries": [
				{"path":"refs/heads/main","self_link":"https://api.launchpad.net/devel/~owner/project/+git/repo/+ref/refs/heads/main"},
				{"path":"refs/heads/feature","self_link":""}
			]
		}`), nil
	}))

	branches, err := newRepoManagerForTest().ListBranches(context.Background(), "https://api.launchpad.net/devel/~owner/project/+git/repo")
	if err != nil {
		t.Fatalf("ListBranches() error = %v", err)
	}
	if len(branches) != 2 {
		t.Fatalf("expected 2 branches, got %d", len(branches))
	}
	if branches[0].Path != "refs/heads/main" || branches[0].SelfLink != "https://api.launchpad.net/devel/~owner/project/+git/repo/+ref/refs/heads/main" {
		t.Fatalf("branches[0] = %+v", branches[0])
	}
	// Second branch has no self_link, should be constructed.
	wantLink := "https://api.launchpad.net/devel/~owner/project/+git/repo/+ref/refs/heads/feature"
	if branches[1].SelfLink != wantLink {
		t.Fatalf("branches[1].SelfLink = %q, want %q", branches[1].SelfLink, wantLink)
	}
}

func TestRepoManagerDeleteGitRef(t *testing.T) {
	var deletedPath string
	withLaunchpadTransport(t, repoRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodDelete {
			t.Fatalf("unexpected method: %s", req.Method)
		}
		deletedPath = req.URL.Path
		return launchpadResponse(http.StatusOK, ""), nil
	}))

	refLink := "https://api.launchpad.net/devel/~owner/project/+git/repo/+ref/refs/heads/tmp-branch"
	err := newRepoManagerForTest().DeleteGitRef(context.Background(), refLink)
	if err != nil {
		t.Fatalf("DeleteGitRef() error = %v", err)
	}
	if deletedPath != "/devel/~owner/project/+git/repo/+ref/refs/heads/tmp-branch" {
		t.Fatalf("deleted path = %q", deletedPath)
	}
}
