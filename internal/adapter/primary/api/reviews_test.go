// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/canonical/sunbeam-watchtower/internal/config"
)

func TestReviewGet_Returns409WhenCacheNotSynced(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEphemeralTestApp(t, &config.Config{
		Projects: []config.ProjectConfig{{
			Name:         "snap-openstack",
			ArtifactType: "snap",
			Code: config.CodeConfig{
				Forge:   "github",
				Owner:   "canonical",
				Project: "snap-openstack",
			},
		}},
	})
	RegisterReviewsAPI(srv.API(), application)

	resp, err := http.Get(base + "/api/v1/reviews/snap-openstack/%2342")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["detail"] == "" {
		t.Fatalf("body = %+v, want conflict detail", body)
	}
}
