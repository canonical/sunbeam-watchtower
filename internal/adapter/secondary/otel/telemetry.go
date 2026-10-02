// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package otel

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime"
	runtimemetrics "runtime/metrics"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otlploggrpc "go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	otlploghttp "go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	otlptracegrpc "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otlptracehttp "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/canonical/sunbeam-watchtower/internal/config"
)

type Telemetry struct {
	logger *slog.Logger
	source *SnapshotSource
	cfg    config.OTelConfig

	selfRegistry   *prometheus.Registry
	domainRegistry *prometheus.Registry

	selfServer   *http.Server
	domainServer *http.Server

	selfListener   net.Listener
	domainListener net.Listener

	requestCounter   *prometheus.CounterVec
	requestDuration  *prometheus.HistogramVec
	inflightRequests *prometheus.GaugeVec
	collectorRefresh *prometheus.CounterVec
	collectorErrors  *prometheus.CounterVec
	collectorLastRun *prometheus.GaugeVec
	collectorLatency *prometheus.HistogramVec

	authAuthenticated *prometheus.GaugeVec
	operationCount    *prometheus.GaugeVec
	operationOldest   *prometheus.GaugeVec
	projectConfigured *prometheus.GaugeVec
	projectRepoCached *prometheus.GaugeVec
	buildCount        *prometheus.GaugeVec
	buildOldest       *prometheus.GaugeVec
	releasePresent    *prometheus.GaugeVec
	releaseRevision   *prometheus.GaugeVec
	releaseReleased   *prometheus.GaugeVec
	releaseResource   *prometheus.GaugeVec
	reviewCount       *prometheus.GaugeVec
	reviewOldest      *prometheus.GaugeVec
	commitCount       *prometheus.GaugeVec
	bugCount          *prometheus.GaugeVec
	packageCount      *prometheus.GaugeVec
	excusesCount      *prometheus.GaugeVec
	cacheEntries      *prometheus.GaugeVec
	cacheLastUpdated  *prometheus.GaugeVec

	tracerProvider *sdktrace.TracerProvider
	logProvider    *sdklog.LoggerProvider
	tracer         trace.Tracer
	logHandler     slog.Handler
	traceEnabled   bool
	logsEnabled    bool

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func New(ctx context.Context, cfg config.OTelConfig, logger *slog.Logger, source *SnapshotSource) (*Telemetry, error) {
	if logger == nil {
		logger = slog.Default()
	}
	telemetry := &Telemetry{
		logger: logger,
		source: source,
		cfg:    cfg,
	}
	if err := telemetry.initProviders(ctx); err != nil {
		return nil, err
	}
	telemetry.initRegistries()
	if err := telemetry.startMetricsServers(); err != nil {
		_ = telemetry.Shutdown(context.Background())
		return nil, err
	}
	telemetry.startCollectors(ctx)
	return telemetry, nil
}

func (t *Telemetry) Enabled() bool {
	return t != nil && (t.selfServer != nil || t.domainServer != nil || t.traceEnabled || t.logsEnabled)
}

func (t *Telemetry) Logger(base *slog.Logger) *slog.Logger {
	if t == nil || t.logHandler == nil {
		return base
	}
	if base == nil {
		return slog.New(t.logHandler)
	}
	return slog.New(newMultiHandler(base.Handler(), t.logHandler))
}

func (t *Telemetry) Middleware(next http.Handler) http.Handler {
	if t == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		ctx := r.Context()

		var span trace.Span
		if t.traceEnabled {
			ctx, span = t.tracer.Start(ctx, requestSpanName(r), trace.WithAttributes(
				semconv.HTTPRequestMethodKey.String(r.Method),
				semconv.URLPath(r.URL.Path),
			))
			defer span.End()
			r = r.WithContext(ctx)
		}

		labels := prometheus.Labels{
			"method": r.Method,
			"route":  routePattern(r),
			"status": "000",
		}
		t.inflightRequests.With(labels).Inc()
		defer t.inflightRequests.With(labels).Dec()

		next.ServeHTTP(rec, r)

		labels["route"] = routePattern(r)
		labels["status"] = fmt.Sprintf("%d", rec.statusCode)
		t.requestCounter.With(labels).Inc()
		t.requestDuration.With(labels).Observe(time.Since(start).Seconds())

		if span != nil {
			span.SetAttributes(
				attribute.String("http.route", labels["route"]),
				attribute.Int("http.response.status_code", rec.statusCode),
			)
			if rec.statusCode >= 500 {
				span.SetStatus(codes.Error, http.StatusText(rec.statusCode))
			}
		}
	})
}

func (t *Telemetry) SelfAddr() string {
	if t == nil || t.selfListener == nil {
		return ""
	}
	return t.selfListener.Addr().String()
}

func (t *Telemetry) DomainAddr() string {
	if t == nil || t.domainListener == nil {
		return ""
	}
	return t.domainListener.Addr().String()
}

func (t *Telemetry) Shutdown(ctx context.Context) error {
	if t == nil {
		return nil
	}
	if t.cancel != nil {
		t.cancel()
	}
	var errs []error
	if t.selfServer != nil {
		if err := t.selfServer.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if t.domainServer != nil {
		if err := t.domainServer.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if t.tracerProvider != nil {
		if err := t.tracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if t.logProvider != nil {
		if err := t.logProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	t.wg.Wait()
	return errorsJoin(errs...)
}

func (t *Telemetry) initProviders(ctx context.Context) error {
	res, err := buildResource(ctx, t.cfg)
	if err != nil {
		return err
	}
	if t.cfg.Traces.Enabled {
		exporter, err := newTraceExporter(ctx, t.cfg.Traces)
		if err != nil {
			return fmt.Errorf("create trace exporter: %w", err)
		}
		sampler := sdktrace.TraceIDRatioBased(t.cfg.Traces.SamplingRatio)
		if t.cfg.Traces.SamplingRatio == 0 {
			sampler = sdktrace.TraceIDRatioBased(0.1)
		}
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(exporter),
			sdktrace.WithSampler(sampler),
			sdktrace.WithResource(res),
		)
		t.tracerProvider = tp
		t.traceEnabled = true
		t.tracer = tp.Tracer("watchtower.server")
		otel.SetTracerProvider(tp)
	} else {
		t.tracer = otel.Tracer("watchtower.server")
	}
	if t.cfg.Logs.Enabled {
		exporter, err := newLogExporter(ctx, t.cfg.Logs)
		if err != nil {
			return fmt.Errorf("create log exporter: %w", err)
		}
		provider := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)), sdklog.WithResource(res))
		t.logProvider = provider
		t.logsEnabled = true
		t.logHandler = otelslog.NewHandler("watchtower", otelslog.WithLoggerProvider(provider))
	}
	return nil
}

func buildResource(ctx context.Context, cfg config.OTelConfig) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		semconv.ServiceName(defaultString(cfg.ServiceName, "sunbeam-watchtower")),
	}
	if cfg.ServiceNamespace != "" {
		attrs = append(attrs, semconv.ServiceNamespace(cfg.ServiceNamespace))
	}
	attrs = append(attrs, semconv.ServiceInstanceID(uuid.NewString()))
	for key, value := range cfg.ResourceAttributes {
		attrs = append(attrs, attribute.String(key, value))
	}
	return resource.New(ctx, resource.WithAttributes(attrs...))
}

func (t *Telemetry) initRegistries() {
	t.selfRegistry = prometheus.NewRegistry()
	t.domainRegistry = prometheus.NewRegistry()

	t.requestCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "watchtower",
		Subsystem: "http_server",
		Name:      "requests_total",
		Help:      "HTTP requests served by the Watchtower API server.",
	}, []string{"method", "route", "status"})
	t.requestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "watchtower",
		Subsystem: "http_server",
		Name:      "request_duration_seconds",
		Help:      "HTTP request latency for the Watchtower API server.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "route", "status"})
	t.inflightRequests = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "watchtower",
		Subsystem: "http_server",
		Name:      "inflight_requests",
		Help:      "In-flight HTTP requests for the Watchtower API server.",
	}, []string{"method", "route", "status"})
	t.collectorRefresh = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "watchtower",
		Subsystem: "telemetry",
		Name:      "collector_refresh_total",
		Help:      "Collector refresh runs by collector and result.",
	}, []string{"collector", "result"})
	t.collectorErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "watchtower",
		Subsystem: "telemetry",
		Name:      "collector_errors_total",
		Help:      "Collector refresh errors by collector.",
	}, []string{"collector"})
	t.collectorLastRun = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "watchtower",
		Subsystem: "telemetry",
		Name:      "collector_last_success_unix",
		Help:      "Collector last successful refresh time as Unix seconds.",
	}, []string{"collector"})
	t.collectorLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "watchtower",
		Subsystem: "telemetry",
		Name:      "collector_duration_seconds",
		Help:      "Collector refresh duration in seconds.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"collector", "result"})

	for _, collector := range []prometheus.Collector{
		t.requestCounter,
		t.requestDuration,
		t.inflightRequests,
		t.collectorRefresh,
		t.collectorErrors,
		t.collectorLastRun,
		t.collectorLatency,
		newRuntimeCollector(),
	} {
		t.selfRegistry.MustRegister(collector)
	}
	t.authAuthenticated = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "watchtower", Subsystem: "auth", Name: "credentials_present", Help: "Whether credentials are present for one auth provider.",
	}, []string{"provider"})
	t.operationCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "watchtower", Subsystem: "operations", Name: "count", Help: "Operations by kind and state.",
	}, []string{"kind", "state"})
	t.operationOldest = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "watchtower", Subsystem: "operations", Name: "oldest_age_seconds", Help: "Age in seconds of the oldest operation by kind and state.",
	}, []string{"kind", "state"})
	t.projectConfigured = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "watchtower", Subsystem: "projects", Name: "configured", Help: "Configured projects by forge and artifact type.",
	}, []string{"project", "forge", "artifact_type"})
	t.projectRepoCached = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "watchtower", Subsystem: "projects", Name: "repo_cached", Help: "Whether the project repository is present in the local cache.",
	}, []string{"project"})
	t.buildCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "watchtower", Subsystem: "builds", Name: "count", Help: "Builds by project, artifact type, backend, and state.",
	}, []string{"project", "artifact_type", "backend", "state"})
	t.buildOldest = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "watchtower", Subsystem: "builds", Name: "oldest_age_seconds", Help: "Age in seconds of the oldest build by project and state.",
	}, []string{"project", "artifact_type", "backend", "state"})
	t.releasePresent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "watchtower", Subsystem: "releases", Name: "channel_present", Help: "Whether an artifact channel is currently present.",
	}, []string{"project", "artifact_type", "artifact", "track", "risk", "branch"})
	t.releaseRevision = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "watchtower", Subsystem: "releases", Name: "target_revision", Help: "Current release target revision by artifact channel and architecture.",
	}, []string{"project", "artifact_type", "artifact", "track", "risk", "branch", "architecture"})
	t.releaseReleased = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "watchtower", Subsystem: "releases", Name: "target_released_unix", Help: "Current release target timestamp by artifact channel and architecture.",
	}, []string{"project", "artifact_type", "artifact", "track", "risk", "branch", "architecture"})
	t.releaseResource = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "watchtower", Subsystem: "releases", Name: "resource_revision", Help: "Current charm resource revision by artifact channel and resource.",
	}, []string{"project", "artifact_type", "artifact", "track", "risk", "branch", "resource"})
	t.reviewCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "watchtower", Subsystem: "reviews", Name: "count", Help: "Merge requests by project, forge, and state.",
	}, []string{"project", "forge", "state"})
	t.reviewOldest = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "watchtower", Subsystem: "reviews", Name: "oldest_age_seconds", Help: "Age in seconds of the oldest merge request by project, forge, and state.",
	}, []string{"project", "forge", "state"})
	t.commitCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "watchtower", Subsystem: "commits", Name: "count", Help: "Commits by project, merge-request state, and bug-reference presence.",
	}, []string{"project", "merge_request_state", "has_bug_ref"})
	t.bugCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "watchtower", Subsystem: "bugs", Name: "count", Help: "Bug tasks by project, forge, and assignment state.",
	}, []string{"project", "forge", "assigned"})
	t.packageCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "watchtower", Subsystem: "packages", Name: "count", Help: "Package counts by configured source dimensions.",
	}, []string{"source", "distro", "release", "component"})
	t.excusesCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "watchtower", Subsystem: "excuses", Name: "entries", Help: "Excuse entry counts by tracker.",
	}, []string{"tracker"})
	t.cacheEntries = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "watchtower", Subsystem: "cache", Name: "entries", Help: "Cache entry counts by cache kind and scope.",
	}, []string{"kind", "scope"})
	t.cacheLastUpdated = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "watchtower", Subsystem: "cache", Name: "last_updated_unix", Help: "Last updated time for one cache kind and scope.",
	}, []string{"kind", "scope"})
	for _, collector := range []prometheus.Collector{
		t.authAuthenticated,
		t.operationCount,
		t.operationOldest,
		t.projectConfigured,
		t.projectRepoCached,
		t.buildCount,
		t.buildOldest,
		t.releasePresent,
		t.releaseRevision,
		t.releaseReleased,
		t.releaseResource,
		t.reviewCount,
		t.reviewOldest,
		t.commitCount,
		t.bugCount,
		t.packageCount,
		t.excusesCount,
		t.cacheEntries,
		t.cacheLastUpdated,
	} {
		t.domainRegistry.MustRegister(collector)
	}
}

func (t *Telemetry) startMetricsServers() error {
	if t.cfg.Metrics.Self.Enabled && t.cfg.Metrics.Self.ListenAddr != "" {
		server, listener, err := startMetricsServer(t.cfg.Metrics.Self.ListenAddr, defaultString(t.cfg.Metrics.Self.Path, "/metrics"), t.selfRegistry)
		if err != nil {
			return fmt.Errorf("start self metrics server: %w", err)
		}
		t.selfServer = server
		t.selfListener = listener
		t.logger.Info("listening for self metrics", "addr", listener.Addr().String(), "path", defaultString(t.cfg.Metrics.Self.Path, "/metrics"))
	}
	if t.cfg.Metrics.Domain.Enabled && t.cfg.Metrics.Domain.ListenAddr != "" {
		server, listener, err := startMetricsServer(t.cfg.Metrics.Domain.ListenAddr, defaultString(t.cfg.Metrics.Domain.Path, "/metrics"), t.domainRegistry)
		if err != nil {
			return fmt.Errorf("start domain metrics server: %w", err)
		}
		t.domainServer = server
		t.domainListener = listener
		t.logger.Info("listening for domain metrics", "addr", listener.Addr().String(), "path", defaultString(t.cfg.Metrics.Domain.Path, "/metrics"))
	}
	return nil
}

func startMetricsServer(addr, path string, registry *prometheus.Registry) (*http.Server, net.Listener, error) {
	mux := http.NewServeMux()
	mux.Handle(path, promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, err
	}
	go func() {
		_ = server.Serve(listener)
	}()
	return server, listener, nil
}

func newTraceExporter(ctx context.Context, cfg config.OTelSignalConfig) (sdktrace.SpanExporter, error) {
	switch strings.ToLower(cfg.Protocol) {
	case "http":
		options := []otlptracehttp.Option{otlptracehttp.WithEndpoint(cfg.Endpoint)}
		if cfg.Insecure {
			options = append(options, otlptracehttp.WithInsecure())
		}
		if len(cfg.Headers) > 0 {
			options = append(options, otlptracehttp.WithHeaders(cfg.Headers))
		}
		return otlptracehttp.New(ctx, options...)
	default:
		options := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(cfg.Endpoint)}
		if cfg.Insecure {
			options = append(options, otlptracegrpc.WithInsecure())
		}
		if len(cfg.Headers) > 0 {
			options = append(options, otlptracegrpc.WithHeaders(cfg.Headers))
		}
		return otlptracegrpc.New(ctx, options...)
	}
}

func newLogExporter(ctx context.Context, cfg config.OTelSignalConfig) (sdklog.Exporter, error) {
	switch strings.ToLower(cfg.Protocol) {
	case "http":
		options := []otlploghttp.Option{otlploghttp.WithEndpoint(cfg.Endpoint)}
		if cfg.Insecure {
			options = append(options, otlploghttp.WithInsecure())
		}
		if len(cfg.Headers) > 0 {
			options = append(options, otlploghttp.WithHeaders(cfg.Headers))
		}
		return otlploghttp.New(ctx, options...)
	default:
		options := []otlploggrpc.Option{otlploggrpc.WithEndpoint(cfg.Endpoint)}
		if cfg.Insecure {
			options = append(options, otlploggrpc.WithInsecure())
		}
		if len(cfg.Headers) > 0 {
			options = append(options, otlploggrpc.WithHeaders(cfg.Headers))
		}
		return otlploggrpc.New(ctx, options...)
	}
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func routePattern(r *http.Request) string {
	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		return "unknown"
	}
	pattern := rctx.RoutePattern()
	if pattern == "" {
		return "unknown"
	}
	return pattern
}

func requestSpanName(r *http.Request) string {
	return r.Method + " " + routePattern(r)
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

type runtimeCollector struct {
	descs map[string]*prometheus.Desc
}

func newRuntimeCollector() prometheus.Collector {
	return &runtimeCollector{descs: map[string]*prometheus.Desc{
		"goroutines": prometheus.NewDesc("watchtower_runtime_goroutines", "Number of goroutines.", nil, nil),
		"uptime":     prometheus.NewDesc("watchtower_process_uptime_seconds", "Process uptime in seconds.", nil, nil),
	}}
}

var processStart = time.Now()

func (c *runtimeCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range c.descs {
		ch <- desc
	}
}

func (c *runtimeCollector) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(c.descs["goroutines"], prometheus.GaugeValue, float64(runtime.NumGoroutine()))
	ch <- prometheus.MustNewConstMetric(c.descs["uptime"], prometheus.GaugeValue, time.Since(processStart).Seconds())
	for _, sample := range []string{"/memory/classes/heap/objects:bytes", "/gc/cycles/total:gc-cycles"} {
		_ = sample
	}
	_ = runtimemetrics.All()
}

func errorsJoin(errs ...error) error {
	var out []error
	for _, err := range errs {
		if err != nil {
			out = append(out, err)
		}
	}
	if len(out) == 0 {
		return nil
	}
	if len(out) == 1 {
		return out[0]
	}
	return fmt.Errorf("%v", out)
}

type multiHandler struct {
	handlers []slog.Handler
}

func newMultiHandler(handlers ...slog.Handler) slog.Handler {
	filtered := make([]slog.Handler, 0, len(handlers))
	for _, handler := range handlers {
		if handler != nil {
			filtered = append(filtered, handler)
		}
	}
	return &multiHandler{handlers: filtered}
}

func (h *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *multiHandler) Handle(ctx context.Context, record slog.Record) error {
	var errs []error
	for _, handler := range h.handlers {
		if !handler.Enabled(ctx, record.Level) {
			continue
		}
		if err := handler.Handle(ctx, record.Clone()); err != nil {
			errs = append(errs, err)
		}
	}
	return errorsJoin(errs...)
}

func (h *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		handlers = append(handlers, handler.WithAttrs(attrs))
	}
	return &multiHandler{handlers: handlers}
}

func (h *multiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		handlers = append(handlers, handler.WithGroup(name))
	}
	return &multiHandler{handlers: handlers}
}

var _ otellog.Logger = nil
