// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package otel

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/canonical/sunbeam-watchtower/internal/config"
)

func TestMiddlewareRecordsTemplatedRouteAndStatus(t *testing.T) {
	tel := &Telemetry{}
	tel.initRegistries()

	router := chi.NewMux()
	router.Use(tel.Middleware)
	router.Get("/api/v1/items/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/items/42", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if got, want := res.Code, http.StatusCreated; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}

	count := testutil.ToFloat64(tel.requestCounter.WithLabelValues(http.MethodGet, "/api/v1/items/{id}", "201"))
	if count != 1 {
		t.Fatalf("request counter = %v, want 1", count)
	}
}

func TestCollectorRuntimeConfigDefaults(t *testing.T) {
	enabled, interval := collectorRuntimeConfig(collectorSpec{
		name:        "releases",
		cfg:         config.OTelCollectorConfig{},
		fallback:    2 * time.Minute,
		cacheBacked: true,
	}, nil)
	if !enabled {
		t.Fatal("default-on collector should be enabled")
	}
	if interval != 2*time.Minute {
		t.Fatalf("interval = %v, want %v", interval, 2*time.Minute)
	}
}

func TestCollectorRuntimeConfigLiveCollectorsRequireAllowList(t *testing.T) {
	spec := collectorSpec{
		name:     "reviews",
		cfg:      config.OTelCollectorConfig{},
		fallback: time.Minute,
	}
	enabled, _ := collectorRuntimeConfig(spec, nil)
	if enabled {
		t.Fatal("live collector should be disabled by default")
	}

	enabled, _ = collectorRuntimeConfig(spec, []string{"reviews"})
	if !enabled {
		t.Fatal("live collector should be enabled when allow-listed")
	}
}

func TestNewStartsMetricsServersAndShutdownsCleanly(t *testing.T) {
	tel, err := New(context.Background(), config.OTelConfig{
		Metrics: config.OTelMetricsConfig{
			Self: config.OTelMetricsListenerConfig{
				Enabled:    true,
				ListenAddr: "127.0.0.1:0",
			},
			Domain: config.OTelMetricsListenerConfig{
				Enabled:    true,
				ListenAddr: "127.0.0.1:0",
				Path:       "/domain-metrics",
			},
		},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer tel.Shutdown(context.Background())

	if !tel.Enabled() {
		t.Fatal("Enabled() = false, want true")
	}
	if tel.SelfAddr() == "" || tel.DomainAddr() == "" {
		t.Fatalf("metrics listeners not initialized: self=%q domain=%q", tel.SelfAddr(), tel.DomainAddr())
	}

	resp, err := http.Get("http://" + tel.SelfAddr() + "/metrics")
	if err != nil {
		t.Fatalf("GET self metrics error = %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("self metrics status = %d, want 200", resp.StatusCode)
	}

	resp, err = http.Get("http://" + tel.DomainAddr() + "/domain-metrics")
	if err != nil {
		t.Fatalf("GET domain metrics error = %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("domain metrics status = %d, want 200", resp.StatusCode)
	}
}

func TestHelperFunctionsAndHandlers(t *testing.T) {
	if got := defaultString("", "fallback"); got != "fallback" {
		t.Fatalf("defaultString() = %q, want fallback", got)
	}
	if got := defaultString("value", "fallback"); got != "value" {
		t.Fatalf("defaultString() = %q, want value", got)
	}
	if err := errorsJoin(nil, nil); err != nil {
		t.Fatalf("errorsJoin(nil) = %v, want nil", err)
	}
	single := errors.New("boom")
	if err := errorsJoin(single); !errors.Is(err, single) {
		t.Fatalf("errorsJoin(single) = %v, want boom", err)
	}
	if err := errorsJoin(errors.New("one"), errors.New("two")); err == nil {
		t.Fatal("errorsJoin(multiple) = nil, want error")
	}

	req := httptest.NewRequest(http.MethodGet, "/items/42", nil)
	if got := routePattern(req); got != "unknown" {
		t.Fatalf("routePattern() = %q, want unknown without route context", got)
	}

	router := chi.NewMux()
	router.Get("/items/{id}", func(w http.ResponseWriter, r *http.Request) {
		if got := routePattern(r); got != "/items/{id}" {
			t.Fatalf("routePattern() = %q, want /items/{id}", got)
		}
		if got := requestSpanName(r); got != "GET /items/{id}" {
			t.Fatalf("requestSpanName() = %q, want GET /items/{id}", got)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	router.ServeHTTP(httptest.NewRecorder(), req)

	recorder := httptest.NewRecorder()
	status := &statusRecorder{ResponseWriter: recorder, statusCode: http.StatusOK}
	status.WriteHeader(http.StatusAccepted)
	if status.statusCode != http.StatusAccepted {
		t.Fatalf("statusCode = %d, want %d", status.statusCode, http.StatusAccepted)
	}

	collector := newRuntimeCollector()
	ch := make(chan *prometheus.Desc, 2)
	collector.Describe(ch)
	close(ch)
	if got := len(ch); got != 2 {
		t.Fatalf("Describe() descriptors = %d, want 2", got)
	}
	metricCh := make(chan prometheus.Metric, 2)
	collector.Collect(metricCh)
	close(metricCh)
	if got := len(metricCh); got != 2 {
		t.Fatalf("Collect() metrics = %d, want 2", got)
	}

	baseBuffer := &bytes.Buffer{}
	extraBuffer := &bytes.Buffer{}
	base := slog.New(slog.NewTextHandler(baseBuffer, nil))
	handler := newMultiHandler(
		base.Handler(),
		slog.New(slog.NewTextHandler(extraBuffer, nil)).Handler(),
	).WithAttrs([]slog.Attr{slog.String("key", "value")}).WithGroup("test")
	logger := slog.New(handler)
	logger.Info("hello")
	if baseBuffer.Len() == 0 || extraBuffer.Len() == 0 {
		t.Fatal("multiHandler should fan out records to both handlers")
	}
}
