// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/config"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func TestSRUAPIReportsMissingSnapshotAndFiltersRows(t *testing.T) {
	cacheDir := t.TempDir()
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())
	application := newEphemeralTestApp(t, &config.Config{})
	t.Setenv("XDG_CACHE_HOME", cacheDir)
	RegisterSRUAPI(srv.API(), application)
	RegisterCacheAPI(srv.API(), application)

	response, err := http.Get(base + "/api/v1/sru")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("missing snapshot status = %d", response.StatusCode)
	}

	path := filepath.Join(cacheDir, "sunbeam-watchtower", "sru", "snapshot.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	snapshot := dto.SRUSnapshot{SyncedAt: time.Now().UTC(), Rows: []dto.SRURow{
		{BugID: "2167438", Package: "neutron", Archive: "uca", Series: "gazpacho", Stage: "updates"},
		{BugID: "2167438", Package: "neutron", Archive: "ubuntu", Series: "resolute", Stage: "updates"},
	}}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	response, err = http.Get(base + "/api/v1/sru?archive=uca")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d", response.StatusCode)
	}
	var listed dto.SRUSnapshot
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Rows) != 1 || listed.Rows[0].Archive != "uca" {
		t.Fatalf("filtered rows: %+v", listed.Rows)
	}

	response, err = http.Get(base + "/api/v1/sru/2167438")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("show status = %d", response.StatusCode)
	}
	response, err = http.Get(base + "/api/v1/sru/9999999")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown bug status = %d", response.StatusCode)
	}

	request, err := http.NewRequest(http.MethodDelete, base+"/api/v1/cache/sru", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("cache clear status = %d", response.StatusCode)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("SRU snapshot remains after API clear: %v", err)
	}
	request, err = http.NewRequest(http.MethodPost, base+"/api/v1/cache/sync/sru", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("unconfigured cache sync status = %d", response.StatusCode)
	}
}
