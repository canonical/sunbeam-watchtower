// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"strings"
	"testing"
	"time"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func TestSRUViewShowsTaskArchiveAndVerificationSeparately(t *testing.T) {
	model := newRootModel(nil, true)
	model.activeView = viewSRU
	model.width = 220
	model.sru.snapshot = &dto.SRUSnapshot{SyncedAt: time.Now(), Rows: []dto.SRURow{{
		BugID: "2167438", Title: "[SRU] Neutron", Package: "neutron", Archive: "uca",
		Series: "gazpacho", TaskStatus: "Fix Released", Stage: "updates", Verification: "done",
		Version: "2:28.0.2-0ubuntu1~cloud0", Evidence: []dto.SRUArchiveEvidence{{Stage: "updates", Version: "2:28.0.2-0ubuntu1~cloud0", URL: "https://api.launchpad.net/devel/sourcepub/1"}},
	}}}
	rendered := model.renderSRU()
	for _, want := range []string{"Fix Released", "updates", "done", "2167438", "28.0.2", "sourcepub/1"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("SRU view missing %q", want)
		}
	}
}
