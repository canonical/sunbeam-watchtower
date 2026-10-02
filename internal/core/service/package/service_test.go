// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package pkg

import (
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"

	distro "github.com/canonical/sunbeam-watchtower/pkg/distro/v1"
)

// mockCache implements port.DistroCache for testing.
type mockCache struct {
	data         map[string][]distro.SourcePackage       // keyed by source name
	detailedData map[string][]distro.SourcePackageDetail // keyed by source name
	baseDir      string                                  // custom cache dir for FindDsc tests
}

func (m *mockCache) Update(_ context.Context, name string, _ []dto.SourceEntry) error {
	return nil
}

func (m *mockCache) Query(_ context.Context, name string, opts dto.QueryOpts) ([]distro.SourcePackage, error) {
	pkgs := m.data[name]
	if pkgs == nil {
		return nil, nil
	}

	pkgFilter := make(map[string]bool, len(opts.Packages))
	for _, p := range opts.Packages {
		pkgFilter[p] = true
	}
	suiteFilter := make(map[string]bool, len(opts.Suites))
	for _, s := range opts.Suites {
		suiteFilter[s] = true
	}

	var result []distro.SourcePackage
	for _, pkg := range pkgs {
		if len(pkgFilter) > 0 && !pkgFilter[pkg.Package] {
			continue
		}
		if len(suiteFilter) > 0 && !suiteFilter[pkg.Suite] {
			continue
		}
		result = append(result, pkg)
	}
	return result, nil
}

func (m *mockCache) Status() ([]dto.CacheStatus, error) {
	var statuses []dto.CacheStatus
	for name, pkgs := range m.data {
		statuses = append(statuses, dto.CacheStatus{
			Name:        name,
			EntryCount:  len(pkgs),
			LastUpdated: time.Now(),
		})
	}
	return statuses, nil
}

func (m *mockCache) CacheDir() string {
	if m.baseDir != "" {
		return m.baseDir
	}
	// Tests that reach this fallback set baseDir; returning an empty path
	// surfaces any unexpected dependency on an on-disk fixture.
	return ""
}
func (m *mockCache) Close() error { return nil }

func (m *mockCache) QueryDetailed(_ context.Context, name string, opts dto.QueryOpts) ([]distro.SourcePackageDetail, error) {
	pkgs := m.detailedData[name]
	if pkgs == nil {
		return nil, nil
	}

	pkgFilter := make(map[string]bool, len(opts.Packages))
	for _, p := range opts.Packages {
		pkgFilter[p] = true
	}
	suiteFilter := make(map[string]bool, len(opts.Suites))
	for _, s := range opts.Suites {
		suiteFilter[s] = true
	}

	var result []distro.SourcePackageDetail
	for _, pkg := range pkgs {
		if len(pkgFilter) > 0 && !pkgFilter[pkg.Package] {
			continue
		}
		if len(suiteFilter) > 0 && !suiteFilter[pkg.Suite] {
			continue
		}
		result = append(result, pkg)
	}
	return result, nil
}

func newTestService() *Service {
	cache := &mockCache{
		data: map[string][]distro.SourcePackage{
			"ubuntu": {
				{Package: "nova", Version: "1:29.0.0-0ubuntu1", Suite: "noble", Component: "main"},
				{Package: "nova", Version: "1:29.1.0-0ubuntu1", Suite: "noble-updates", Component: "main"},
				{Package: "keystone", Version: "2:27.0.0-0ubuntu1", Suite: "noble", Component: "main"},
			},
			"debian": {
				{Package: "nova", Version: "29.0.0-1", Suite: "trixie", Component: "main"},
				{Package: "keystone", Version: "27.0.0-1", Suite: "trixie", Component: "main"},
			},
		},
	}
	return NewService(cache, nil)
}

func TestDiff(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	results, err := svc.Diff(ctx, DiffOpts{
		Packages: []string{"nova", "keystone"},
		Sources: []ProjectSource{
			{Name: "ubuntu"},
			{Name: "debian"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Results should be sorted by package name.
	if results[0].Package != "keystone" {
		t.Errorf("expected first result to be keystone, got %q", results[0].Package)
	}
	if results[1].Package != "nova" {
		t.Errorf("expected second result to be nova, got %q", results[1].Package)
	}

	// Nova should have entries from both ubuntu and debian.
	novaVersions := results[1].Versions
	if len(novaVersions["ubuntu"]) != 2 {
		t.Errorf("expected 2 ubuntu nova versions, got %d", len(novaVersions["ubuntu"]))
	}
	if len(novaVersions["debian"]) != 1 {
		t.Errorf("expected 1 debian nova version, got %d", len(novaVersions["debian"]))
	}
}

func TestShow(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	result, err := svc.Show(ctx, "nova", []ProjectSource{
		{Name: "ubuntu"},
		{Name: "debian"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Package != "nova" {
		t.Errorf("expected nova, got %q", result.Package)
	}
	if len(result.Versions) != 2 {
		t.Errorf("expected versions from 2 sources, got %d", len(result.Versions))
	}
}

func TestShowMissing(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	result, err := svc.Show(ctx, "nonexistent", []ProjectSource{
		{Name: "ubuntu"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Package != "nonexistent" {
		t.Errorf("expected nonexistent, got %q", result.Package)
	}
	if len(result.Versions) != 0 {
		t.Errorf("expected 0 versions, got %d", len(result.Versions))
	}
}

func TestList(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	pkgs, err := svc.List(ctx, "ubuntu", dto.QueryOpts{
		Suites: []string{"noble"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages in noble, got %d", len(pkgs))
	}
}

func TestReverseDepends(t *testing.T) {
	cache := &mockCache{
		data: map[string][]distro.SourcePackage{},
		detailedData: map[string][]distro.SourcePackageDetail{
			"ubuntu": {
				{
					SourcePackage: distro.SourcePackage{Package: "nova", Version: "1:29.0.0-0ubuntu1", Suite: "noble", Component: "main"},
					BuildDepends:  []string{"debhelper-compat", "python3-oslo.messaging", "python3-neutron-lib"},
				},
				{
					SourcePackage: distro.SourcePackage{Package: "keystone", Version: "2:27.0.0-0ubuntu1", Suite: "noble", Component: "main"},
					BuildDepends:  []string{"debhelper-compat", "python3-oslo.messaging"},
				},
				{
					SourcePackage: distro.SourcePackage{Package: "neutron", Version: "2:24.0.0-1", Suite: "noble", Component: "main"},
					BuildDepends:  []string{"debhelper-compat", "python3-neutron-lib"},
				},
			},
		},
	}
	svc := NewService(cache, nil)
	ctx := context.Background()

	results, err := svc.ReverseDepends(ctx, "python3-neutron-lib", []ProjectSource{{Name: "ubuntu"}}, dto.QueryOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 rdepends, got %d", len(results))
	}

	names := map[string]bool{}
	for _, r := range results {
		names[r.Package] = true
	}
	if !names["nova"] || !names["neutron"] {
		t.Errorf("expected nova and neutron, got %v", names)
	}
}

func TestReverseDependsPythonAlias(t *testing.T) {
	cache := &mockCache{
		data: map[string][]distro.SourcePackage{},
		detailedData: map[string][]distro.SourcePackageDetail{
			"ubuntu": {
				{
					SourcePackage: distro.SourcePackage{Package: "nova", Version: "1:29.0.0-0ubuntu1", Suite: "noble", Component: "main"},
					BuildDepends:  []string{"python3-oslo.messaging"},
				},
				{
					SourcePackage: distro.SourcePackage{Package: "keystone", Version: "2:27.0.0-0ubuntu1", Suite: "noble", Component: "main"},
					BuildDepends:  []string{"python-oslo.messaging"},
				},
			},
		},
	}
	svc := NewService(cache, nil)
	ctx := context.Background()

	// Searching python3-oslo.messaging should also match python-oslo.messaging.
	results, err := svc.ReverseDepends(ctx, "python3-oslo.messaging", []ProjectSource{{Name: "ubuntu"}}, dto.QueryOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 rdepends with python alias, got %d", len(results))
	}
}

func TestFindDsc(t *testing.T) {
	// Create a temp cache dir with a raw uncompressed Sources file.
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "sources", "ubuntu")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sourcesContent := `Package: nova
Version: 1:29.1.0-0ubuntu1
Directory: pool/main/n/nova
Files:
 abc123 1234 nova_29.1.0-0ubuntu1.dsc
 def456 5678 nova_29.1.0.orig.tar.gz

Package: keystone
Version: 2:27.0.1-0ubuntu1
Directory: pool/main/k/keystone
Files:
 aaa111 100 keystone_27.0.1-0ubuntu1.dsc
 bbb222 200 keystone_27.0.1.orig.tar.gz
`

	// Write as a .gz file.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write([]byte(sourcesContent)); err != nil {
		t.Fatal(err)
	}
	gw.Close()

	fname := "noble-updates_main_Sources.gz"
	if err := os.WriteFile(filepath.Join(srcDir, fname), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	cache := &mockCache{
		data: map[string][]distro.SourcePackage{
			"ubuntu": {
				{Package: "nova", Version: "1:29.1.0-0ubuntu1", Suite: "noble-updates", Component: "main"},
				{Package: "keystone", Version: "2:27.0.1-0ubuntu1", Suite: "noble-updates", Component: "main"},
			},
		},
		baseDir: tmpDir,
	}
	svc := NewService(cache, nil)
	ctx := context.Background()

	pairs := []PackageVersionPair{
		{Package: "nova", Version: "1:29.1.0-0ubuntu1"},
		{Package: "keystone", Version: "2:27.0.1-0ubuntu1"},
		{Package: "nonexistent", Version: "1.0"},
	}

	sources := []ProjectSource{
		{
			Name: "ubuntu",
			Entries: []dto.SourceEntry{
				{Mirror: "http://archive.ubuntu.com/ubuntu", Suite: "noble-updates", Component: "main"},
			},
		},
	}

	results, err := svc.FindDsc(ctx, pairs, sources)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Nova should have a .dsc URL.
	if len(results[0].URLs) != 1 {
		t.Fatalf("expected 1 URL for nova, got %d", len(results[0].URLs))
	}
	expected := "http://archive.ubuntu.com/ubuntu/pool/main/n/nova/nova_29.1.0-0ubuntu1.dsc"
	if results[0].URLs[0] != expected {
		t.Errorf("expected URL %q, got %q", expected, results[0].URLs[0])
	}

	// Keystone should have a .dsc URL.
	if len(results[1].URLs) != 1 {
		t.Fatalf("expected 1 URL for keystone, got %d", len(results[1].URLs))
	}

	// Nonexistent should have no URLs.
	if len(results[2].URLs) != 0 {
		t.Errorf("expected 0 URLs for nonexistent, got %d", len(results[2].URLs))
	}
}
