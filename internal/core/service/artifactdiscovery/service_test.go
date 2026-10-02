// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package artifactdiscovery

import (
	"context"
	"errors"
	"fmt"
	"path"
	"reflect"
	"strings"
	"testing"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// fakeReader is an in-memory TreeReader keyed by repoPath+filePath.
type fakeReader struct {
	files map[string]string
}

func newFakeReader(files map[string]string) *fakeReader {
	return &fakeReader{files: files}
}

func (f *fakeReader) ReadHEADFile(repoPath, filePath string) ([]byte, error) {
	content, ok := f.files[filePath]
	if !ok {
		return nil, fmt.Errorf("not found: %s", filePath)
	}
	return []byte(content), nil
}

func (f *fakeReader) FindHEADFilesByBaseName(repoPath, baseName string) ([]HeadFile, error) {
	var out []HeadFile
	for p, c := range f.files {
		if path.Base(p) == baseName {
			out = append(out, HeadFile{Path: p, Content: []byte(c)})
		}
	}
	// Mirror gitcache adapter's sort-by-path contract.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[i].Path > out[j].Path {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

// erroringReader always fails FindHEADFilesByBaseName. Used to test that the
// service surfaces reader errors verbatim when no root manifest exists.
type erroringReader struct{}

func (erroringReader) ReadHEADFile(string, string) ([]byte, error) {
	return nil, errors.New("root miss")
}
func (erroringReader) FindHEADFilesByBaseName(string, string) ([]HeadFile, error) {
	return nil, errors.New("walk broke")
}

func TestDiscoverCharmRootOnly(t *testing.T) {
	reader := newFakeReader(map[string]string{
		"charmcraft.yaml": "name: my-charm\nresources:\n  image:\n    type: oci-image\n  data:\n    type: file\n",
	})
	svc := NewService(reader, nil)
	got, err := svc.Discover(context.Background(), "/repo", dto.ArtifactCharm)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	want := []DiscoveredArtifact{{
		Name:         "my-charm",
		RelPath:      "",
		ArtifactType: dto.ArtifactCharm,
		Resources:    []string{"data", "image"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Discover() = %+v, want %+v", got, want)
	}
}

func TestDiscoverCharmMonoRepo(t *testing.T) {
	reader := newFakeReader(map[string]string{
		"charms/keystone/charmcraft.yaml": "name: keystone-k8s\n",
		"charms/glance/charmcraft.yaml":   "name: glance-k8s\nresources:\n  img:\n    type: oci-image\n",
	})
	svc := NewService(reader, nil)
	got, err := svc.Discover(context.Background(), "/repo", dto.ArtifactCharm)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	want := []DiscoveredArtifact{
		{Name: "glance-k8s", RelPath: "charms/glance", ArtifactType: dto.ArtifactCharm, Resources: []string{"img"}},
		{Name: "keystone-k8s", RelPath: "charms/keystone", ArtifactType: dto.ArtifactCharm, Resources: []string{}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Discover() = %+v, want %+v", got, want)
	}
}

func TestDiscoverCharmNestedMonoRepo(t *testing.T) {
	reader := newFakeReader(map[string]string{
		"charms/storage/foo/charmcraft.yaml": "name: foo-k8s\n",
		"charms/network/bar/charmcraft.yaml": "name: bar-k8s\n",
		"charms/network/baz/charmcraft.yaml": "name: baz-k8s\n",
	})
	svc := NewService(reader, nil)
	got, err := svc.Discover(context.Background(), "/repo", dto.ArtifactCharm)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d charms, want 3", len(got))
	}
	for i, name := range []string{"bar-k8s", "baz-k8s", "foo-k8s"} {
		if got[i].Name != name {
			t.Fatalf("got[%d].Name = %q, want %q", i, got[i].Name, name)
		}
	}
	if got[0].RelPath != "charms/network/bar" {
		t.Fatalf("got[0].RelPath = %q, want charms/network/bar", got[0].RelPath)
	}
}

func TestDiscoverCharmRootWinsOverMonoRepo(t *testing.T) {
	reader := newFakeReader(map[string]string{
		"charmcraft.yaml":               "name: top-level\n",
		"charms/nested/charmcraft.yaml": "name: ignored\n",
	})
	svc := NewService(reader, nil)
	got, err := svc.Discover(context.Background(), "/repo", dto.ArtifactCharm)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(got) != 1 || got[0].Name != "top-level" || got[0].RelPath != "" {
		t.Fatalf("got %+v, want single top-level root entry", got)
	}
}

func TestDiscoverSnapRoot(t *testing.T) {
	reader := newFakeReader(map[string]string{
		"snap/snapcraft.yaml": "name: my-snap\n",
	})
	svc := NewService(reader, nil)
	got, err := svc.Discover(context.Background(), "/repo", dto.ArtifactSnap)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	want := []DiscoveredArtifact{{Name: "my-snap", ArtifactType: dto.ArtifactSnap}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Discover() = %+v, want %+v", got, want)
	}
}

func TestDiscoverSnapRootLegacy(t *testing.T) {
	reader := newFakeReader(map[string]string{
		"snapcraft.yaml": "name: legacy-snap\n",
	})
	svc := NewService(reader, nil)
	got, err := svc.Discover(context.Background(), "/repo", dto.ArtifactSnap)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(got) != 1 || got[0].Name != "legacy-snap" {
		t.Fatalf("got %+v, want legacy-snap", got)
	}
}

func TestDiscoverSnapMonoRepo(t *testing.T) {
	reader := newFakeReader(map[string]string{
		"snaps/foo/snap/snapcraft.yaml": "name: foo\n",
		"snaps/bar/snap/snapcraft.yaml": "name: bar\n",
	})
	svc := NewService(reader, nil)
	got, err := svc.Discover(context.Background(), "/repo", dto.ArtifactSnap)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	want := []DiscoveredArtifact{
		{Name: "bar", RelPath: "snaps/bar/snap", ArtifactType: dto.ArtifactSnap},
		{Name: "foo", RelPath: "snaps/foo/snap", ArtifactType: dto.ArtifactSnap},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Discover() = %+v, want %+v", got, want)
	}
}

func TestDiscoverRockRoot(t *testing.T) {
	reader := newFakeReader(map[string]string{
		"rockcraft.yaml": "name: my-rock\n",
	})
	svc := NewService(reader, nil)
	got, err := svc.Discover(context.Background(), "/repo", dto.ArtifactRock)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	want := []DiscoveredArtifact{{Name: "my-rock", ArtifactType: dto.ArtifactRock}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Discover() = %+v, want %+v", got, want)
	}
}

func TestDiscoverRockMonoRepo(t *testing.T) {
	reader := newFakeReader(map[string]string{
		"rocks/keystone/rockcraft.yaml": "name: keystone\n",
		"rocks/glance/rockcraft.yaml":   "name: glance\n",
	})
	svc := NewService(reader, nil)
	got, err := svc.Discover(context.Background(), "/repo", dto.ArtifactRock)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	want := []DiscoveredArtifact{
		{Name: "glance", RelPath: "rocks/glance", ArtifactType: dto.ArtifactRock},
		{Name: "keystone", RelPath: "rocks/keystone", ArtifactType: dto.ArtifactRock},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Discover() = %+v, want %+v", got, want)
	}
}

func TestDiscoverMissingManifestReturnsError(t *testing.T) {
	cases := []struct {
		name         string
		artifactType dto.ArtifactType
		wantSubstr   string
	}{
		{"charm", dto.ArtifactCharm, "charmcraft.yaml"},
		{"snap", dto.ArtifactSnap, "snapcraft.yaml"},
		{"rock", dto.ArtifactRock, "rockcraft.yaml"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(newFakeReader(nil), nil)
			_, err := svc.Discover(context.Background(), "/repo", tc.artifactType)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("error %q does not mention %q", err, tc.wantSubstr)
			}
		})
	}
}

func TestDiscoverMalformedYAMLSurfacesSource(t *testing.T) {
	cases := []struct {
		name         string
		files        map[string]string
		artifactType dto.ArtifactType
		wantSource   string
	}{
		{
			name:         "charm root",
			files:        map[string]string{"charmcraft.yaml": ":\n  not: valid: yaml"},
			artifactType: dto.ArtifactCharm,
			wantSource:   "charmcraft.yaml",
		},
		{
			name:         "charm mono-repo",
			files:        map[string]string{"charms/broken/charmcraft.yaml": "::: :\n  foo: [bar"},
			artifactType: dto.ArtifactCharm,
			wantSource:   "charms/broken/charmcraft.yaml",
		},
		{
			name:         "snap root",
			files:        map[string]string{"snap/snapcraft.yaml": "::: :\n  foo: [bar"},
			artifactType: dto.ArtifactSnap,
			wantSource:   "snap/snapcraft.yaml",
		},
		{
			name:         "snap mono-repo",
			files:        map[string]string{"snaps/x/snap/snapcraft.yaml": "::: :\n  foo: [bar"},
			artifactType: dto.ArtifactSnap,
			wantSource:   "snaps/x/snap/snapcraft.yaml",
		},
		{
			name:         "rock root",
			files:        map[string]string{"rockcraft.yaml": "::: :\n  foo: [bar"},
			artifactType: dto.ArtifactRock,
			wantSource:   "rockcraft.yaml",
		},
		{
			name:         "rock mono-repo",
			files:        map[string]string{"rocks/x/rockcraft.yaml": "::: :\n  foo: [bar"},
			artifactType: dto.ArtifactRock,
			wantSource:   "rocks/x/rockcraft.yaml",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(newFakeReader(tc.files), nil)
			_, err := svc.Discover(context.Background(), "/repo", tc.artifactType)
			if err == nil {
				t.Fatalf("expected parse error, got nil")
			}
			if !strings.Contains(err.Error(), "parsing "+tc.wantSource) {
				t.Fatalf("error %q does not mention parsing %s", err, tc.wantSource)
			}
		})
	}
}

func TestDiscoverMissingNameErrors(t *testing.T) {
	cases := []struct {
		name         string
		files        map[string]string
		artifactType dto.ArtifactType
		wantSubstr   string
	}{
		{
			name:         "charm root",
			files:        map[string]string{"charmcraft.yaml": "summary: x\n"},
			artifactType: dto.ArtifactCharm,
			wantSubstr:   "does not declare a charm name",
		},
		{
			name:         "charm mono-repo",
			files:        map[string]string{"charms/x/charmcraft.yaml": "summary: x\n"},
			artifactType: dto.ArtifactCharm,
			wantSubstr:   "does not declare a charm name",
		},
		{
			name:         "snap",
			files:        map[string]string{"snap/snapcraft.yaml": "summary: x\n"},
			artifactType: dto.ArtifactSnap,
			wantSubstr:   "does not declare a snap name",
		},
		{
			name:         "rock",
			files:        map[string]string{"rockcraft.yaml": "summary: x\n"},
			artifactType: dto.ArtifactRock,
			wantSubstr:   "does not declare a rock name",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(newFakeReader(tc.files), nil)
			_, err := svc.Discover(context.Background(), "/repo", tc.artifactType)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("error %q does not contain %q", err, tc.wantSubstr)
			}
		})
	}
}

func TestDiscoverUnsupportedArtifactType(t *testing.T) {
	svc := NewService(newFakeReader(nil), nil)
	_, err := svc.Discover(context.Background(), "/repo", dto.ArtifactType(99))
	if err == nil {
		t.Fatalf("expected error for unknown type, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported artifact type") {
		t.Fatalf("error %q does not mention unsupported artifact type", err)
	}
}

func TestDiscoverReaderErrorPropagates(t *testing.T) {
	svc := NewService(erroringReader{}, nil)
	_, err := svc.Discover(context.Background(), "/repo", dto.ArtifactCharm)
	if err == nil || !strings.Contains(err.Error(), "walk broke") {
		t.Fatalf("expected walk error to propagate, got %v", err)
	}
}

func TestDiscoverCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc := NewService(newFakeReader(map[string]string{"charmcraft.yaml": "name: x\n"}), nil)
	_, err := svc.Discover(ctx, "/repo", dto.ArtifactCharm)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestNewServiceDefaultsLogger(t *testing.T) {
	s := NewService(newFakeReader(nil), nil)
	if s.logger == nil {
		t.Fatalf("logger should default to non-nil")
	}
}
