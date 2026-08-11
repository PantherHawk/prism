// Package telemetry wires the OpenTelemetry SDK for prism's own traces and
// metrics.
//
// This is self-observability and has nothing to do with the metrics prism
// scrapes and displays. Exporter endpoints, headers and protocols are read by
// the SDK from the standard OTEL_* environment variables; this package only
// decides whether to start at all and what to call the service.
package telemetry

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Config describes prism's own telemetry.
type Config struct {
	Enabled     bool   `yaml:"enabled"`
	ServiceName string `yaml:"service_name"`
}

// Default returns the configuration used when none is supplied. Telemetry is
// off by default: a terminal tool that phones home unasked is a bad citizen.
func Default() Config {
	return Config{
		Enabled:     false,
		ServiceName: "prism",
	}
}

// Setup installs the global tracer and meter providers and returns the
// function that flushes and stops them. The returned function is safe to call
// even when telemetry is disabled, so callers never need to branch.
func Setup(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	if !cfg.Enabled {
		return noop, nil
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(semconv.ServiceNameKey.String(cfg.ServiceName)),
	)
	if err != nil {
		return noop, fmt.Errorf("build resource: %w", err)
	}

	traceExporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		return noop, fmt.Errorf("build trace exporter: %w", err)
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)

	metricExporter, err := otlpmetricgrpc.New(ctx)
	if err != nil {
		// The tracer provider is already live, so it has to come down before
		// this failure propagates or its batcher goroutine leaks.
		return tracerProvider.Shutdown, fmt.Errorf("build metric exporter: %w", err)
	}

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(res),
	)

	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return func(shutdownCtx context.Context) error {
		return errors.Join(
			tracerProvider.Shutdown(shutdownCtx),
			meterProvider.Shutdown(shutdownCtx),
		)
	}, nil
}

// noop is the shutdown function returned when there is nothing to shut down.
func noop(context.Context) error {
	return nil
}
