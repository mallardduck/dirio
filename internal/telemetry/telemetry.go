package telemetry

import (
	"context"
	"errors"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	otlpmetrichttp "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	prometheusexporter "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/mallardduck/dirio/internal/version"
)

// Config holds telemetry configuration.
type Config struct {
	// OTLPEnabled controls whether metrics are pushed to an OTLP endpoint.
	OTLPEnabled bool
	// OTLPEndpoint is the base URL of the OTLP HTTP receiver (e.g. "http://localhost:4318").
	OTLPEndpoint string
	// OTLPInterval is how often metrics are pushed to the OTLP endpoint.
	OTLPInterval time.Duration
}

// MetadataStats is a snapshot of metadata subsystem metrics captured at scrape time.
type MetadataStats struct {
	CacheGets    uint64
	CacheMisses  uint64
	CacheEntries uint64
	DBSizeBytes  int64
}

// Provider wraps the OTel MeterProvider and exposes the Prometheus HTTP handler.
type Provider struct {
	mp             *sdkmetric.MeterProvider
	prometheusHTTP http.Handler
}

// MeterProvider returns the underlying OTel MeterProvider.
func (p *Provider) MeterProvider() metric.MeterProvider { return p.mp }

// PrometheusHandler returns the HTTP handler that serves Prometheus-format metrics.
// Mount this at /.dirio/metrics.
func (p *Provider) PrometheusHandler() http.Handler { return p.prometheusHTTP }

// Shutdown flushes and closes all exporters.  Call during graceful shutdown.
func (p *Provider) Shutdown(ctx context.Context) error { return p.mp.Shutdown(ctx) }

// Setup initialises the OTel MeterProvider with a Prometheus pull reader and,
// when cfg.OTLPEnabled is true, an OTLP HTTP push reader.  It also sets the
// global OTel MeterProvider so that otelhttp and other libraries pick it up.
func Setup(ctx context.Context, cfg Config) (*Provider, error) {
	// resource.NewSchemaless omits a Schema URL so resource.Merge does not
	// error when resource.Default() carries a different schema version.
	res, err := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(
			semconv.ServiceName("dirio"),
			semconv.ServiceVersion(version.Version),
		),
	)
	if err != nil {
		return nil, err
	}

	// Use a private registry so we don't pollute the default Prometheus registry
	// or leak Go/process metrics that aren't ours to own.
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	promExporter, err := prometheusexporter.New(
		prometheusexporter.WithRegisterer(reg),
	)
	if err != nil {
		return nil, err
	}

	opts := []sdkmetric.Option{
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(promExporter),
	}

	// OTLP push reader — opt-in.
	if cfg.OTLPEnabled {
		interval := cfg.OTLPInterval
		if interval <= 0 {
			interval = 30 * time.Second
		}
		otlpExporter, err := otlpmetrichttp.New(ctx,
			otlpmetrichttp.WithEndpoint(cfg.OTLPEndpoint),
			otlpmetrichttp.WithInsecure(),
		)
		if err != nil {
			return nil, err
		}
		opts = append(opts, sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(otlpExporter, sdkmetric.WithInterval(interval)),
		))
	}

	mp := sdkmetric.NewMeterProvider(opts...)
	otel.SetMeterProvider(mp)

	return &Provider{
		mp: mp,
		prometheusHTTP: promhttp.HandlerFor(reg, promhttp.HandlerOpts{
			EnableOpenMetrics: true,
		}),
	}, nil
}

// RegisterMetadataObservers wires up observable instruments that read from the
// metadata subsystem at each scrape/push interval.  Call this after both the
// Provider and the metadata Manager are initialised.
func (p *Provider) RegisterMetadataObservers(stats func() MetadataStats) error {
	meter := p.mp.Meter("dirio")

	errs := make([]error, 0, 4)

	_, err := meter.Int64ObservableCounter(
		"dirio.metadata.cache.gets",
		metric.WithDescription("Cumulative number of metadata cache lookups."),
		metric.WithUnit("{call}"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(int64(stats().CacheGets))
			return nil
		}),
	)
	errs = append(errs, err)

	_, err = meter.Int64ObservableCounter(
		"dirio.metadata.cache.misses",
		metric.WithDescription("Cumulative number of metadata cache misses."),
		metric.WithUnit("{call}"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(int64(stats().CacheMisses))
			return nil
		}),
	)
	errs = append(errs, err)

	_, err = meter.Int64ObservableGauge(
		"dirio.metadata.cache.entries",
		metric.WithDescription("Current number of entries in the metadata cache."),
		metric.WithUnit("{entry}"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(int64(stats().CacheEntries))
			return nil
		}),
	)
	errs = append(errs, err)

	_, err = meter.Int64ObservableGauge(
		"dirio.boltdb.size",
		metric.WithDescription("Current size of the BoltDB metadata index file in bytes."),
		metric.WithUnit("By"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(stats().DBSizeBytes)
			return nil
		}),
	)
	errs = append(errs, err)

	return errors.Join(errs...)
}
