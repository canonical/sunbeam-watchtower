// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gboutry/sunbeam-watchtower/internal/config"
)

func TestPackageSetReadsCachedLaunchpadMembership(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)
	application := NewApp(&config.Config{Packages: config.PackagesConfig{
		Sets: map[string][]string{"custom": {"nova"}},
		LaunchpadSets: map[string]config.LaunchpadSetConfig{
			"openstack": {Series: "development"},
		},
	}}, nil)

	if _, err := application.PackageSet("openstack"); !errors.Is(err, ErrPackageSetNotSynced) {
		t.Fatalf("missing cached set: %v, want ErrPackageSetNotSynced", err)
	}
	path := filepath.Join(cacheHome, "sunbeam-watchtower", "packagesets", "openstack.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{
		"name": "openstack", "configured_series": "development", "series": "stonking",
		"packages": []string{"nova", "python-os-traits"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := application.PackageSet("openstack")
	if err != nil || !reflect.DeepEqual(got, []string{"nova", "python-os-traits"}) {
		t.Fatalf("cached set = %v, %v", got, err)
	}
	got, err = application.PackageSet("custom")
	if err != nil || !reflect.DeepEqual(got, []string{"nova"}) {
		t.Fatalf("static set = %v, %v", got, err)
	}
	if _, err := application.PackageSet("missing"); !errors.Is(err, ErrUnknownPackageSet) {
		t.Fatalf("missing set: %v, want ErrUnknownPackageSet", err)
	}
}
