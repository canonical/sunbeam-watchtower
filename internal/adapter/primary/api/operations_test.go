package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/config"
	opsvc "github.com/canonical/sunbeam-watchtower/internal/core/service/operation"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func TestProjectsSyncAsync_EmptyConfigCreatesOperation(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEphemeralTestApp(t, &config.Config{})
	RegisterProjectsAPI(srv.API(), application)
	RegisterOperationsAPI(srv.API(), application)

	service, err := application.OperationService()
	if err != nil {
		t.Fatal(err)
	}

	resp, err := http.Post(base+"/api/v1/projects/sync/async", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var job dto.OperationJob
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		t.Fatal(err)
	}
	if job.ID == "" || job.Kind != dto.OperationKindProjectSync {
		t.Fatalf("unexpected async job: %+v", job)
	}

	finalJob := waitForOperationState(t, service, job.ID, dto.OperationStateSucceeded)
	if finalJob.Summary == "" {
		t.Fatalf("expected operation summary, got %+v", finalJob)
	}

	eventsResp, err := http.Get(base + "/api/v1/operations/" + job.ID + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer eventsResp.Body.Close()

	var eventsBody struct {
		Events []dto.OperationEvent `json:"events"`
	}
	if err := json.NewDecoder(eventsResp.Body).Decode(&eventsBody); err != nil {
		t.Fatal(err)
	}
	if len(eventsBody.Events) == 0 {
		t.Fatal("expected recorded operation events")
	}
}

func TestOperationsCancel_CancelsRunningOperation(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEphemeralTestApp(t, &config.Config{})
	RegisterOperationsAPI(srv.API(), application)

	service, err := application.OperationService()
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	done := make(chan struct{})
	job, err := service.Start(context.Background(), dto.OperationKindProjectSync, nil, func(ctx context.Context, reporter *opsvc.Reporter) (string, error) {
		reporter.Progress(dto.OperationProgress{
			Phase:         "syncing",
			Message:       "waiting for cancellation",
			Indeterminate: true,
		})
		close(started)
		<-ctx.Done()
		close(done)
		return "", ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, base+"/api/v1/operations/"+job.ID+"/cancel", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	<-done

	finalJob := waitForOperationState(t, service, job.ID, dto.OperationStateCancelled)
	if finalJob.Error != context.Canceled.Error() {
		t.Fatalf("expected cancellation error, got %+v", finalJob)
	}
}

func waitForOperationState(t *testing.T, service *opsvc.Service, id string, want dto.OperationState) dto.OperationJob {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	job, err := service.Wait(ctx, id)
	if err != nil {
		t.Fatalf("Wait(%s) error = %v", id, err)
	}
	if job == nil {
		t.Fatalf("Wait(%s) = nil, want job in state %q", id, want)
	}
	if job.State != want {
		t.Fatalf("operation %q final state = %q, want %q", id, job.State, want)
	}
	return *job
}
