// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package packagesetcache

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestParseReport(t *testing.T) {
	report := []byte("Name: openstack\nDescription: Components\n\nPackages:\n - nova\n - python-oslo.config\n\nSub-package sets:\n\nUploaders:\n - a person\n")
	got, err := parseReport(report, "openstack")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"nova", "python-oslo.config"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("members = %v, want %v", got, want)
	}
	for _, bad := range [][]byte{
		[]byte("Name: other\nPackages:\n - nova\n"),
		[]byte("Name: openstack\nPackages:\n - nova\n - nova\n"),
		[]byte("Name: openstack\nPackages:\n - ../escape\n"),
		[]byte("Name: openstack\nPackages:\n\nSub-package sets:\n"),
	} {
		if _, err := parseReport(bad, "openstack"); err == nil {
			t.Fatalf("accepted invalid report %q", bad)
		}
	}
}

func TestSyncUsesCurrentDevelopmentSeriesAndPreservesSnapshotOnFailure(t *testing.T) {
	var failReport atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ubuntu":
			fmt.Fprint(w, `{"current_series_link":"https://api.launchpad.net/devel/ubuntu/stonking"}`)
		case "/packagesets/stonking/openstack":
			if failReport.Load() {
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
				return
			}
			fmt.Fprint(w, "Name: openstack\nPackages:\n - nova\n - python-os-traits\n\nSub-package sets:\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cache := NewCache(t.TempDir(), server.Client())
	cache.ubuntuURL = server.URL + "/ubuntu"
	cache.reportsURL = server.URL + "/packagesets"
	if _, err := cache.Read("openstack", "development"); !errors.Is(err, ErrNotSynced) {
		t.Fatalf("before sync: %v, want ErrNotSynced", err)
	}
	snapshot, err := cache.Sync(context.Background(), "openstack", "development")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Series != "stonking" || !reflect.DeepEqual(snapshot.Packages, []string{"nova", "python-os-traits"}) {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	failReport.Store(true)
	if _, err := cache.Sync(context.Background(), "openstack", "development"); err == nil {
		t.Fatal("failed sync unexpectedly succeeded")
	}
	read, err := cache.Read("openstack", "development")
	if err != nil || !reflect.DeepEqual(read.Packages, snapshot.Packages) {
		t.Fatalf("snapshot after failed sync = %+v, %v", read, err)
	}
	if _, err := cache.Read("openstack", "resolute"); !errors.Is(err, ErrNotSynced) {
		t.Fatalf("configured series change: %v, want ErrNotSynced", err)
	}
}
