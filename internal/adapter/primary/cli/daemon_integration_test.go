// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/testsupport"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// skipIfNoUnixSockets skips a test when running on a platform without
// unix-domain socket support. Centralising the check keeps the skip message
// identical across call sites.
func skipIfNoUnixSockets(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("daemon integration tests require unix sockets")
	}
}

func TestCLIHelperProcess(t *testing.T) {
	if os.Getenv("WATCHTOWER_CLI_HELPER_PROCESS") != "1" {
		return
	}

	if os.Getenv("WATCHTOWER_TEST_FAKE_LAUNCHPAD") == "1" {
		installFakeLaunchpadTransport()
	}

	opts := &Options{
		Out:            os.Stdout,
		ErrOut:         os.Stderr,
		ExecutablePath: os.Getenv("WATCHTOWER_TEST_EXECUTABLE_PATH"),
	}
	cmd := NewRootCmd(opts)
	cmd.SetArgs(helperProcessArgs(os.Args))
	cmd.SetIn(os.Stdin)

	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func helperProcessArgs(args []string) []string {
	for i, arg := range args {
		if arg == "--" {
			return args[i+1:]
		}
	}
	return nil
}

func TestLocalDaemonLifecycleCommands(t *testing.T) {
	skipIfNoUnixSockets(t)

	wrapper := writeCLIHelperWrapper(t)
	env := append(daemonTestEnv(t, wrapper), "WATCHTOWER_TEST_FAKE_LAUNCHPAD=1")

	start := runCLIHelper(t, wrapper, env, "", "server", "start")
	if !strings.Contains(start.Stdout, "Started local server at unix://") {
		t.Fatalf("server start output = %q", start.Stdout)
	}
	t.Cleanup(func() {
		_, _ = runCLIHelperNoFail(wrapper, env, "", "server", "stop")
	})

	status := runCLIHelper(t, wrapper, env, "", "server", "status")
	if !strings.Contains(status.Stdout, "Local server running at unix://") {
		t.Fatalf("server status output = %q", status.Stdout)
	}

	stop := runCLIHelper(t, wrapper, env, "", "server", "stop")
	if !strings.Contains(stop.Stdout, "Stopped local server.") {
		t.Fatalf("server stop output = %q", stop.Stdout)
	}
}

func TestLocalDaemonStatePersistsAcrossInvocations(t *testing.T) {
	skipIfNoUnixSockets(t)

	wrapper := writeCLIHelperWrapper(t)
	env := append(daemonTestEnv(t, wrapper), "WATCHTOWER_TEST_FAKE_LAUNCHPAD=1")

	start := runCLIHelper(t, wrapper, env, "", "server", "start")
	if !strings.Contains(start.Stdout, "Started local server at unix://") {
		t.Fatalf("server start output = %q", start.Stdout)
	}
	t.Cleanup(func() {
		_, _ = runCLIHelperNoFail(wrapper, env, "", "server", "stop")
	})

	t.Run("auth persists", func(t *testing.T) {
		login := runCLIHelper(t, wrapper, env, "\n", "auth", "launchpad", "login")
		if !strings.Contains(login.Stdout, "Authenticated as: Jane Doe (jdoe)") {
			t.Fatalf("auth login output = %q", login.Stdout)
		}

		status := runCLIHelper(t, wrapper, env, "", "auth", "status")
		if !strings.Contains(status.Stdout, "Authenticated as: Jane Doe (jdoe)") {
			t.Fatalf("auth status output = %q", status.Stdout)
		}

		logout := runCLIHelper(t, wrapper, env, "", "auth", "launchpad", "logout")
		if !strings.Contains(logout.Stdout, "Removed Launchpad credentials") {
			t.Fatalf("auth logout output = %q", logout.Stdout)
		}

		status = runCLIHelper(t, wrapper, env, "", "auth", "status")
		if !strings.Contains(status.Stdout, "Not authenticated.") {
			t.Fatalf("auth status after logout output = %q", status.Stdout)
		}
	})

	t.Run("operations persist", func(t *testing.T) {
		login := runCLIHelper(t, wrapper, env, "\n", "auth", "launchpad", "login")
		if !strings.Contains(login.Stdout, "Authenticated as: Jane Doe (jdoe)") {
			t.Fatalf("auth login output = %q", login.Stdout)
		}

		trigger := runCLIHelper(t, wrapper, env, "", "-o", "json", "project", "sync", "--async")
		var job dto.OperationJob
		if err := json.Unmarshal([]byte(trigger.Stdout), &job); err != nil {
			t.Fatalf("json.Unmarshal(trigger) error = %v; stdout=%q", err, trigger.Stdout)
		}
		if job.ID == "" || job.Kind != dto.OperationKindProjectSync {
			t.Fatalf("triggered job = %+v, want project sync operation", job)
		}

		finalJob := waitForOperationStateViaCLI(t, wrapper, env, job.ID, dto.OperationStateSucceeded)
		if finalJob.Summary == "" {
			t.Fatalf("final job = %+v, want non-empty summary", finalJob)
		}

		list := runCLIHelper(t, wrapper, env, "", "-o", "json", "operation", "list")
		var jobs []dto.OperationJob
		if err := json.Unmarshal([]byte(list.Stdout), &jobs); err != nil {
			t.Fatalf("json.Unmarshal(list) error = %v; stdout=%q", err, list.Stdout)
		}
		if !containsOperationID(jobs, job.ID) {
			t.Fatalf("operation list = %+v, want job %q", jobs, job.ID)
		}

		events := runCLIHelper(t, wrapper, env, "", "-o", "json", "operation", "events", job.ID)
		var operationEvents []dto.OperationEvent
		if err := json.Unmarshal([]byte(events.Stdout), &operationEvents); err != nil {
			t.Fatalf("json.Unmarshal(events) error = %v; stdout=%q", err, events.Stdout)
		}
		if len(operationEvents) == 0 {
			t.Fatal("operation events should not be empty")
		}
	})
}

type helperCommandResult struct {
	Stdout string
	Stderr string
}

func writeCLIHelperWrapper(t *testing.T) string {
	t.Helper()

	testBinary, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}

	wrapper := filepath.Join(t.TempDir(), "watchtower-cli-helper")
	content := fmt.Sprintf(
		"#!/bin/sh\nexport WATCHTOWER_CLI_HELPER_PROCESS=1\nexec %q -test.run=TestCLIHelperProcess -- \"$@\"\n",
		testBinary,
	)
	if err := os.WriteFile(wrapper, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", wrapper, err)
	}
	return wrapper
}

func daemonTestEnv(t *testing.T, wrapper string) []string {
	t.Helper()

	env := []string{
		"HOME=" + t.TempDir(),
		"XDG_RUNTIME_DIR=" + t.TempDir(),
		"WATCHTOWER_TEST_EXECUTABLE_PATH=" + wrapper,
	}
	// The CLI helper subprocess inherits os.Environ() in runCLIHelperNoFail;
	// blank entries appended later override any forge credentials the
	// developer may have exported, so the spawned daemon and CLI start from
	// a deterministic, unauthenticated state.
	for _, name := range testsupport.ForgeCredentialEnvVars {
		env = append(env, name+"=")
	}
	return env
}

func runCLIHelper(t *testing.T, wrapper string, env []string, stdin string, args ...string) helperCommandResult {
	t.Helper()

	result, err := runCLIHelperNoFail(wrapper, env, stdin, args...)
	if err != nil {
		t.Fatalf("runCLIHelper(%v) error = %v\nstdout:\n%s\nstderr:\n%s", args, err, result.Stdout, result.Stderr)
	}
	return result
}

func runCLIHelperNoFail(wrapper string, env []string, stdin string, args ...string) (helperCommandResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, wrapper, args...)
	cmd.Env = append(os.Environ(), env...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return helperCommandResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}, err
}

func waitForOperationStateViaCLI(
	t *testing.T,
	wrapper string,
	env []string,
	id string,
	want dto.OperationState,
) dto.OperationJob {
	t.Helper()

	// why: the daemon runs in a separate process, so we can't share a
	// completion channel with it. Each iteration spawns a fresh CLI
	// subprocess (~hundreds of ms), and an empty project sync completes
	// in well under a second; 20s gives >20x headroom for slow CI.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var lastJob dto.OperationJob
	for {
		show := runCLIHelper(t, wrapper, env, "", "-o", "json", "operation", "show", id)

		var job dto.OperationJob
		if err := json.Unmarshal([]byte(show.Stdout), &job); err != nil {
			t.Fatalf("json.Unmarshal(show) error = %v; stdout=%q", err, show.Stdout)
		}
		lastJob = job
		if job.State == want {
			return job
		}

		select {
		case <-ctx.Done():
			t.Fatalf("operation %q did not reach %q; last observed job: %+v", id, want, lastJob)
			return dto.OperationJob{}
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func containsOperationID(jobs []dto.OperationJob, id string) bool {
	for _, job := range jobs {
		if job.ID == id {
			return true
		}
	}
	return false
}

type helperRoundTripFunc func(*http.Request) (*http.Response, error)

func (f helperRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func installFakeLaunchpadTransport() {
	original := http.DefaultTransport
	http.DefaultTransport = helperRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Host == "launchpad.net" && req.URL.Path == "/+request-token":
			return helperTextResponse(http.StatusOK, "oauth_token=req-token&oauth_token_secret=req-secret"), nil
		case req.URL.Host == "launchpad.net" && req.URL.Path == "/+access-token":
			return helperTextResponse(http.StatusOK, "oauth_token=access-token&oauth_token_secret=access-secret"), nil
		case req.URL.Host == "api.launchpad.net" && req.URL.Path == "/devel/people/+me":
			return helperJSONResponse(http.StatusOK, `{"name":"jdoe","display_name":"Jane Doe"}`), nil
		default:
			return original.RoundTrip(req)
		}
	})
}

func helperTextResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"text/plain"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func helperJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
