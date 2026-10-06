// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package sru

import (
	"testing"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func TestVersionCellsCurrency(t *testing.T) {
	for _, test := range []struct {
		name  string
		cells []dto.SRUVersionCell
		want  []string
	}{
		{"cloud suffix", []dto.SRUVersionCell{{Version: "2.17.12-0ubuntu0.22.04.1"}, {Version: "2.17.12-0ubuntu0.22.04.1~cloud0"}, {Version: "2.17.9-0ubuntu0.22.04.1~cloud0"}, {Version: "2.17.9-0ubuntu0.22.04.1~cloud0"}}, []string{"current", "current", "behind", "behind"}},
		{"cloud rebuilds differ between pockets", []dto.SRUVersionCell{{Version: "2.17.12-0ubuntu0.22.04.1"}, {Version: "2.17.12-0ubuntu0.22.04.1~cloud2"}, {Version: "2.17.12-0ubuntu0.22.04.1~cloud1"}, {Version: "2.17.12-0ubuntu0.22.04.1~cloud0"}}, []string{"current", "current", "behind", "behind"}},
		{"cloud rebuilds use Debian numeric ordering", []dto.SRUVersionCell{{Version: "2-1"}, {Version: "2-1~cloud2"}, {Version: "2-1~cloud10"}, {Version: "2-1~cloud1"}}, []string{"current", "behind", "current", "behind"}},
		{"packaging revision matters", []dto.SRUVersionCell{{Version: "1:2.0-0ubuntu2"}, {Version: "1:2.0-0ubuntu1~cloud2"}}, []string{"current", "behind"}},
		{"staging ahead of parent", []dto.SRUVersionCell{{Version: "2-1"}, {Version: "3-1~cloud0"}, {Version: "3-1~cloud0"}, {Version: "2-1~cloud0"}}, []string{"behind", "current", "current", "behind"}},
		{"unknown does not mean current", []dto.SRUVersionCell{{Version: "3-1"}, {State: "unknown"}, {Version: "2-1"}, {}}, []string{"unknown", "unknown", "behind", "empty"}},
		{"missing parent", []dto.SRUVersionCell{{State: "unconfigured"}, {Version: "3-1"}}, []string{"unconfigured", "unknown"}},
		{"invalid version", []dto.SRUVersionCell{{Version: "not a version"}, {Version: "3-1"}}, []string{"unknown", "unknown"}},
		{"all empty", []dto.SRUVersionCell{{}, {}}, []string{"empty", "empty"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := VersionCells(test.cells)
			for i, state := range test.want {
				if got[i].State != state {
					t.Fatalf("cells=%+v", got)
				}
			}
		})
	}
}
