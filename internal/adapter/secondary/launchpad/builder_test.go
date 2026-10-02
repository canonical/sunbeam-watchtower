// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package launchpad

import (
	"context"
	"net/http"
	"testing"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

func newBuilderClientForTest() *lp.Client {
	return lp.NewClient(&lp.Credentials{
		ConsumerKey:       "test-app",
		AccessToken:       "token",
		AccessTokenSecret: "secret",
	}, nil)
}

func TestSnapBuilderWorkflow(t *testing.T) {
	builder := NewSnapBuilder(newBuilderClientForTest(), "https://api.launchpad.net/devel/ubuntu/+archive/primary", "Proposed")

	withLaunchpadTransport(t, repoRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/devel/+snaps":
			if err := req.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			if got := req.FormValue("ws.op"); got != "new" {
				t.Fatalf("ws.op = %q, want new", got)
			}
			if got := req.FormValue("owner"); got != "https://api.launchpad.net/devel/~owner" {
				t.Fatalf("owner = %q", got)
			}
			if got := req.FormValue("git_ref"); got != "https://api.launchpad.net/devel/~owner/project/+git/repo/+ref/refs/heads/main" {
				t.Fatalf("git_ref = %q", got)
			}
			return launchpadResponse(http.StatusCreated, ""), nil
		case req.Method == http.MethodGet && req.URL.Path == "/devel/~owner/+snap/my-snap":
			return launchpadResponse(http.StatusOK, `{"name":"my-snap","self_link":"https://api.launchpad.net/devel/~owner/+snap/my-snap","web_link":"https://launchpad.net/~owner/+snap/my-snap","owner_link":"https://api.launchpad.net/devel/~owner","git_path":"snap/","auto_build":true}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/devel/~owner/+snap/my-snap":
			if err := req.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			if got := req.FormValue("ws.op"); got != "requestBuilds" {
				t.Fatalf("ws.op = %q, want requestBuilds", got)
			}
			if got := req.FormValue("archive"); got != "https://api.launchpad.net/devel/ubuntu/+archive/primary" {
				t.Fatalf("archive = %q", got)
			}
			if got := req.FormValue("pocket"); got != "Proposed" {
				t.Fatalf("pocket = %q", got)
			}
			if got := req.FormValue("channels"); got != `{"edge":"latest/edge"}` {
				t.Fatalf("channels = %q", got)
			}
			return launchpadResponse(http.StatusOK, `{"self_link":"https://api.launchpad.net/devel/~owner/+snap/my-snap/+build-request/1","web_link":"https://launchpad.net/~owner/+snap/my-snap/+build-request/1","status":"Currently building","builds_collection_link":"https://api.launchpad.net/devel/~owner/+snap/my-snap/+build-request/1/builds"}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/devel/~owner/+snap/my-snap/builds":
			return launchpadResponse(http.StatusOK, `{"total_size":1,"entries":[{"self_link":"https://api.launchpad.net/devel/~owner/+snap/my-snap/+build/1","web_link":"https://launchpad.net/~owner/+snap/my-snap/+build/1","title":"my-snap amd64 build","buildstate":"Successfully built","arch_tag":"amd64","build_log_url":"https://launchpad.net/build.log","can_be_retried":true,"can_be_cancelled":false}]}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/devel/~owner/+snap/my-snap/+build/1":
			if got := req.URL.Query().Get("ws.op"); got != "getFileUrls" {
				t.Fatalf("ws.op = %q, want getFileUrls", got)
			}
			return launchpadResponse(http.StatusOK, `["https://launchpad.net/files/my-snap.snap"]`), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	}))

	recipe, err := builder.CreateRecipe(context.Background(), dto.CreateRecipeOpts{
		Name:       "my-snap",
		Owner:      "owner",
		GitRefLink: "https://api.launchpad.net/devel/~owner/project/+git/repo/+ref/refs/heads/main",
	})
	if err != nil {
		t.Fatalf("CreateRecipe() error = %v", err)
	}
	if recipe.Owner != "~owner" || recipe.Project != "" || recipe.ArtifactType != dto.ArtifactSnap {
		t.Fatalf("unexpected recipe: %+v", recipe)
	}

	buildRequest, err := builder.RequestBuilds(context.Background(), recipe, dto.RequestBuildsOpts{
		Channels: map[string]string{"edge": "latest/edge"},
	})
	if err != nil {
		t.Fatalf("RequestBuilds() error = %v", err)
	}
	if buildRequest == nil || buildRequest.SelfLink == "" {
		t.Fatalf("unexpected build request: %+v", buildRequest)
	}

	builds, err := builder.ListBuilds(context.Background(), recipe)
	if err != nil {
		t.Fatalf("ListBuilds() error = %v", err)
	}
	if len(builds) != 1 || builds[0].State != dto.BuildSucceeded || !builds[0].CanRetry {
		t.Fatalf("unexpected builds: %+v", builds)
	}

	files, err := builder.GetBuildFileURLs(context.Background(), builds[0].SelfLink)
	if err != nil {
		t.Fatalf("GetBuildFileURLs() error = %v", err)
	}
	if len(files) != 1 || files[0] != "https://launchpad.net/files/my-snap.snap" {
		t.Fatalf("unexpected file URLs: %+v", files)
	}
}

func TestCharmBuilderListBuildsRetriesTransientLaunchpadRead(t *testing.T) {
	builder := NewCharmBuilder(newBuilderClientForTest())
	attempts := 0

	withLaunchpadTransport(t, repoRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.Path != "/devel/~owner/project/+charm/my-charm/builds" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
		}
		attempts++
		if attempts == 1 {
			return launchpadResponse(http.StatusServiceUnavailable, "temporary"), nil
		}
		return launchpadResponse(http.StatusOK, `{"total_size":1,"entries":[{"self_link":"https://api.launchpad.net/devel/~owner/project/+charm/my-charm/+build/1","web_link":"https://launchpad.net/~owner/project/+charm/my-charm/+build/1","title":"my-charm amd64 build","buildstate":"Successfully built","arch_tag":"amd64","can_be_retried":true,"can_be_cancelled":false}]}`), nil
	}))

	builds, err := builder.ListBuilds(context.Background(), &dto.Recipe{
		Name:     "my-charm",
		SelfLink: "https://api.launchpad.net/devel/~owner/project/+charm/my-charm",
	})
	if err != nil {
		t.Fatalf("ListBuilds() error = %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if len(builds) != 1 || builds[0].State != dto.BuildSucceeded {
		t.Fatalf("builds = %+v, want one succeeded build", builds)
	}
}

func TestSnapBuilderProcessors(t *testing.T) {
	builder := NewSnapBuilder(newBuilderClientForTest(), "", "")

	withLaunchpadTransport(t, repoRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/devel/+snaps":
			if err := req.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			if got := req.FormValue("processors"); got != `["https://api.launchpad.net/devel/+processors/amd64","https://api.launchpad.net/devel/+processors/arm64","https://api.launchpad.net/devel/+processors/s390x"]` {
				t.Fatalf("processors = %q", got)
			}
			return launchpadResponse(http.StatusCreated, ""), nil
		case req.Method == http.MethodGet && req.URL.Path == "/devel/~owner/+snap/multi-arch":
			return launchpadResponse(http.StatusOK, `{"name":"multi-arch","self_link":"https://api.launchpad.net/devel/~owner/+snap/multi-arch","owner_link":"https://api.launchpad.net/devel/~owner"}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/devel/~owner/+snap/multi-arch":
			if err := req.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			if got := req.FormValue("ws.op"); got != "setProcessors" {
				t.Fatalf("ws.op = %q, want setProcessors", got)
			}
			if got := req.FormValue("processors"); got != `["https://api.launchpad.net/devel/+processors/amd64","https://api.launchpad.net/devel/+processors/arm64"]` {
				t.Fatalf("processors = %q", got)
			}
			return launchpadResponse(http.StatusOK, ""), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	}))

	recipe, err := builder.CreateRecipe(context.Background(), dto.CreateRecipeOpts{
		Name:       "multi-arch",
		Owner:      "owner",
		GitRefLink: "https://api.launchpad.net/devel/~owner/project/+git/repo/+ref/refs/heads/main",
		Processors: []string{"amd64", "arm64", "s390x"},
	})
	if err != nil {
		t.Fatalf("CreateRecipe() error = %v", err)
	}

	if err := builder.SetProcessors(context.Background(), recipe, []string{"amd64", "arm64"}); err != nil {
		t.Fatalf("SetProcessors() error = %v", err)
	}
}

func TestCharmBuilderWorkflow(t *testing.T) {
	builder := NewCharmBuilder(newBuilderClientForTest())

	withLaunchpadTransport(t, repoRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/devel/~owner/project/+charm/my-charm":
			return launchpadResponse(http.StatusOK, `{"name":"my-charm","self_link":"https://api.launchpad.net/devel/~owner/project/+charm/my-charm","web_link":"https://launchpad.net/~owner/project/+charm/my-charm","owner_link":"https://api.launchpad.net/devel/~owner","project_link":"https://api.launchpad.net/devel/project","git_path":"charm/","build_path":"charm/","auto_build":true}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/devel/+charm-recipes":
			if err := req.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			if got := req.FormValue("project"); got != "https://api.launchpad.net/devel/project" {
				t.Fatalf("project = %q", got)
			}
			if got := req.FormValue("build_path"); got != "charm/" {
				t.Fatalf("build_path = %q", got)
			}
			return launchpadResponse(http.StatusCreated, ""), nil
		case req.Method == http.MethodPost && req.URL.Path == "/devel/~owner/project/+charm/my-charm":
			if err := req.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			if got := req.FormValue("architectures"); got != "amd64,arm64" {
				t.Fatalf("architectures = %q", got)
			}
			if got := req.FormValue("channels"); got != `{"latest":"edge"}` {
				t.Fatalf("channels = %q", got)
			}
			return launchpadResponse(http.StatusOK, `{"self_link":"https://api.launchpad.net/devel/~owner/project/+charm/my-charm/+build-request/1","web_link":"https://launchpad.net/~owner/project/+charm/my-charm/+build-request/1","status":"Queued"}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/devel/~owner/project/+charm/my-charm/builds":
			return launchpadResponse(http.StatusOK, `{"total_size":1,"entries":[{"self_link":"https://api.launchpad.net/devel/~owner/project/+charm/my-charm/+build/1","web_link":"https://launchpad.net/~owner/project/+charm/my-charm/+build/1","title":"my-charm arm64 build","buildstate":"Building","arch_tag":"arm64","can_be_retried":false,"can_be_cancelled":true}]}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/devel/~owner/project/+charm/my-charm/+build/1":
			if err := req.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			if got := req.FormValue("ws.op"); got != "retry" && got != "cancel" {
				t.Fatalf("ws.op = %q, want retry/cancel", got)
			}
			return launchpadResponse(http.StatusOK, `{"status":"ok"}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	}))

	got, err := builder.GetRecipe(context.Background(), "owner", "project", "my-charm")
	if err != nil {
		t.Fatalf("GetRecipe() error = %v", err)
	}
	if got.Owner != "~owner" || got.Project != "project" || got.BuildPath != "charm/" {
		t.Fatalf("unexpected recipe: %+v", got)
	}

	recipe, err := builder.CreateRecipe(context.Background(), dto.CreateRecipeOpts{
		Name:       "my-charm",
		Owner:      "owner",
		Project:    "project",
		GitRefLink: "https://api.launchpad.net/devel/~owner/project/+git/repo/+ref/refs/heads/main",
		BuildPath:  "charm/",
	})
	if err != nil {
		t.Fatalf("CreateRecipe() error = %v", err)
	}

	buildRequest, err := builder.RequestBuilds(context.Background(), recipe, dto.RequestBuildsOpts{
		Channels:      map[string]string{"latest": "edge"},
		Architectures: []string{"amd64", "arm64"},
	})
	if err != nil {
		t.Fatalf("RequestBuilds() error = %v", err)
	}
	if buildRequest == nil || buildRequest.SelfLink == "" {
		t.Fatalf("unexpected build request: %+v", buildRequest)
	}

	builds, err := builder.ListBuilds(context.Background(), recipe)
	if err != nil {
		t.Fatalf("ListBuilds() error = %v", err)
	}
	if len(builds) != 1 || builds[0].State != dto.BuildBuilding || !builds[0].CanCancel {
		t.Fatalf("unexpected builds: %+v", builds)
	}

	if err := builder.RetryBuild(context.Background(), builds[0].SelfLink); err != nil {
		t.Fatalf("RetryBuild() error = %v", err)
	}
	if err := builder.CancelBuild(context.Background(), builds[0].SelfLink); err != nil {
		t.Fatalf("CancelBuild() error = %v", err)
	}
}

func TestRockBuilderWorkflow(t *testing.T) {
	builder := NewRockBuilder(newBuilderClientForTest())

	withLaunchpadTransport(t, repoRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/devel/+rock-recipes":
			if err := req.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			if got := req.FormValue("project"); got != "https://api.launchpad.net/devel/project" {
				t.Fatalf("project = %q", got)
			}
			if got := req.FormValue("build_path"); got != "rock/" {
				t.Fatalf("build_path = %q", got)
			}
			return launchpadResponse(http.StatusCreated, ""), nil
		case req.Method == http.MethodGet && req.URL.Path == "/devel/~owner/project/+rock/my-rock":
			return launchpadResponse(http.StatusOK, `{"name":"my-rock","self_link":"https://api.launchpad.net/devel/~owner/project/+rock/my-rock","web_link":"https://launchpad.net/~owner/project/+rock/my-rock","owner_link":"https://api.launchpad.net/devel/~owner","project_link":"https://api.launchpad.net/devel/project","git_path":"rock/","build_path":"rock/"}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/devel/+rock-recipes":
			if got := req.URL.Query().Get("ws.op"); got != "findByOwner" {
				t.Fatalf("ws.op = %q, want findByOwner", got)
			}
			return launchpadResponse(http.StatusOK, `{"total_size":1,"entries":[{"name":"my-rock","self_link":"https://api.launchpad.net/devel/~owner/project/+rock/my-rock","web_link":"https://launchpad.net/~owner/project/+rock/my-rock","owner_link":"https://api.launchpad.net/devel/~owner","project_link":"https://api.launchpad.net/devel/project","git_path":"rock/","build_path":"rock/"}]}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/devel/~owner/project/+rock/my-rock":
			if err := req.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			if got := req.FormValue("architectures"); got != "s390x" {
				t.Fatalf("architectures = %q", got)
			}
			return launchpadResponse(http.StatusOK, `{"self_link":"https://api.launchpad.net/devel/~owner/project/+rock/my-rock/+build-request/1","web_link":"https://launchpad.net/~owner/project/+rock/my-rock/+build-request/1","status":"Processing"}`), nil
		case req.Method == http.MethodGet && req.URL.Path == "/devel/~owner/project/+rock/my-rock/builds":
			return launchpadResponse(http.StatusOK, `{"total_size":1,"entries":[{"self_link":"https://api.launchpad.net/devel/~owner/project/+rock/my-rock/+build/1","web_link":"https://launchpad.net/~owner/project/+rock/my-rock/+build/1","title":"my-rock s390x build","buildstate":"Needs building","arch_tag":"s390x","can_be_retried":false,"can_be_cancelled":true}]}`), nil
		case req.Method == http.MethodDelete && req.URL.Path == "/devel/~owner/project/+rock/my-rock":
			return launchpadResponse(http.StatusNoContent, ""), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	}))

	recipe, err := builder.CreateRecipe(context.Background(), dto.CreateRecipeOpts{
		Name:       "my-rock",
		Owner:      "owner",
		Project:    "project",
		GitRefLink: "https://api.launchpad.net/devel/~owner/project/+git/repo/+ref/refs/heads/main",
		BuildPath:  "rock/",
	})
	if err != nil {
		t.Fatalf("CreateRecipe() error = %v", err)
	}
	if recipe.ArtifactType != dto.ArtifactRock || recipe.Project != "project" {
		t.Fatalf("unexpected recipe: %+v", recipe)
	}

	recipes, err := builder.ListRecipesByOwner(context.Background(), "owner")
	if err != nil {
		t.Fatalf("ListRecipesByOwner() error = %v", err)
	}
	if len(recipes) != 1 || recipes[0].Name != "my-rock" {
		t.Fatalf("unexpected recipes: %+v", recipes)
	}

	buildRequest, err := builder.RequestBuilds(context.Background(), recipe, dto.RequestBuildsOpts{
		Architectures: []string{"s390x"},
	})
	if err != nil {
		t.Fatalf("RequestBuilds() error = %v", err)
	}
	if buildRequest == nil || buildRequest.SelfLink == "" {
		t.Fatalf("unexpected build request: %+v", buildRequest)
	}

	builds, err := builder.ListBuilds(context.Background(), recipe)
	if err != nil {
		t.Fatalf("ListBuilds() error = %v", err)
	}
	if len(builds) != 1 || builds[0].State != dto.BuildPending {
		t.Fatalf("unexpected builds: %+v", builds)
	}

	if err := builder.DeleteRecipe(context.Background(), recipe.SelfLink); err != nil {
		t.Fatalf("DeleteRecipe() error = %v", err)
	}
}

func TestBuilderHelpers(t *testing.T) {
	if got := extractNameFromLink("https://api.launchpad.net/devel/~owner"); got != "~owner" {
		t.Fatalf("extractNameFromLink() = %q", got)
	}
	if got := parseBuildState("Cancelled"); got != dto.BuildCancelled {
		t.Fatalf("parseBuildState() = %q", got)
	}
	if got := parseBuildState("unknown"); got != dto.BuildPending {
		t.Fatalf("parseBuildState() unknown = %q", got)
	}
}
