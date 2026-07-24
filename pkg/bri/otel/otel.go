// Package otel is the ISOLATED Go half of bri.core.telemetry — the OpenTelemetry
// SDK wiring for opt-in distributed tracing (ADR 0074). It is a SEPARATE
// package from pkg/bri on purpose: the OpenTelemetry SDK is a heavy
// dependency that must NOT link into a bri binary that does not require
// tracing. pkg/bri never imports this package; only the generated
// pkg/briaot/briotel sub-package (and the build-time genbri / interpreter
// briloader paths) do, so the linker keeps the SDK exactly when — and only
// when — an app requires bri.core.telemetry.
//
// Like the rest of bri's Go half this package is PURE Go (the whole
// OpenTelemetry SDK is: no cgo), so a bri.core.telemetry app still AOT-compiles to a
// CGO_ENABLED=0 static binary. It registers its shim installer with pkg/bri
// from init(), so bri.InstallShimsInto resolves bri.core.telemetry's private vars
// exactly like every other namespace once this package is linked.
package otel

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/muthuishere/cljgo/pkg/bri"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// init wires bri.core.telemetry's shim installer into pkg/bri's registry. It runs
// only when this package is linked (i.e. the app requires bri.core.telemetry), so a
// non-tracing binary never carries the OpenTelemetry SDK (ADR 0074).
func init() { bri.RegisterInstaller("bri.core.telemetry", installShims) }

var (
	mu       sync.Mutex
	provider *sdktrace.TracerProvider
	tracer   trace.Tracer
	// prop is the W3C trace-context (+ baggage) propagator — the standard
	// OTel text-map propagator used to join an inbound trace and to inject
	// context into outbound headers.
	prop = propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
)

// spanHandle is the opaque value bri.core.telemetry threads through a request: the
// live span plus the context it was started in (so injection carries the
// span's identity). Clojure holds it as an ordinary value and hands it back
// to -otel-end / -otel-traceparent.
type spanHandle struct {
	ctx  context.Context
	span trace.Span
}

// installShims interns bri.core.telemetry's private Go primitives into the namespace,
// mirroring pkg/bri's installers for the always-linked namespaces. The
// interning func is supplied by bri.InstallShimsInto.
func installShims(def func(name string, fn func(args ...any) any)) {
	def("-otel-init", func(args ...any) any {
		initProvider(str(one("-otel-init", args)))
		return nil
	})
	def("-otel-start", func(args ...any) any {
		if len(args) != 6 {
			panicf("-otel-start expects 6 args, got %d", len(args))
		}
		return startServerSpan(str(args[0]), str(args[1]), str(args[2]), str(args[3]), str(args[4]), str(args[5]))
	})
	def("-otel-end", func(args ...any) any {
		if len(args) != 3 {
			panicf("-otel-end expects 3 args, got %d", len(args))
		}
		endServerSpan(args[0], asInt(args[1]), str(args[2]))
		return nil
	})
	def("-otel-traceid", func(args ...any) any {
		if h, ok := one("-otel-traceid", args).(*spanHandle); ok && h.span.SpanContext().HasTraceID() {
			return h.span.SpanContext().TraceID().String()
		}
		return nil
	})
	def("-otel-traceparent", func(args ...any) any {
		return traceparentOf(one("-otel-traceparent", args))
	})
	def("-otel-shutdown", func(args ...any) any { shutdown(); return nil })
}

// initProvider builds the global TracerProvider once (idempotent). The OTLP
// exporter is created ONLY when an endpoint is configured
// (APP_OTEL_EXPORTER_OTLP_ENDPOINT or the standard OTEL_EXPORTER_OTLP_ENDPOINT);
// APP_OTEL_STDOUT=1 selects a stdout exporter for demos; otherwise spans are
// created and ended but not exported — the span/propagation BEHAVIOR is
// identical either way, only the destination differs (ADR 0074).
func initProvider(serviceName string) {
	mu.Lock()
	defer mu.Unlock()
	if provider != nil {
		return
	}
	if serviceName == "" {
		serviceName = firstEnv("APP_OTEL_SERVICE_NAME", "OTEL_SERVICE_NAME")
	}
	if serviceName == "" {
		serviceName = "bri-app"
	}
	res := resource.NewSchemaless(attribute.String("service.name", serviceName))
	opts := []sdktrace.TracerProviderOption{sdktrace.WithResource(res)}
	if exp := buildExporter(); exp != nil {
		opts = append(opts, sdktrace.WithBatcher(exp))
	}
	provider = sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(prop)
	tracer = provider.Tracer("bri.core.telemetry")
}

// buildExporter returns the configured span exporter, or nil (no export).
// The OTLP/HTTP exporter reads the standard OTEL_EXPORTER_OTLP_* env itself;
// an APP_OTEL_EXPORTER_OTLP_ENDPOINT override is applied explicitly on top.
func buildExporter() sdktrace.SpanExporter {
	if endpoint := firstEnv("APP_OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_ENDPOINT"); endpoint != "" {
		var o []otlptracehttp.Option
		if v := os.Getenv("APP_OTEL_EXPORTER_OTLP_ENDPOINT"); v != "" {
			o = append(o, otlptracehttp.WithEndpointURL(v))
		}
		if strings.EqualFold(os.Getenv("APP_OTEL_EXPORTER_OTLP_PROTOCOL"), "http/protobuf") ||
			strings.EqualFold(os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL"), "http/protobuf") {
			// http/protobuf is the otlptracehttp default; named here for clarity.
			_ = endpoint
		}
		if exp, err := otlptracehttp.New(context.Background(), o...); err == nil {
			return exp
		}
	}
	if truthy(os.Getenv("APP_OTEL_STDOUT")) {
		if exp, err := stdouttrace.New(stdouttrace.WithoutTimestamps()); err == nil {
			return exp
		}
	}
	return nil
}

// startServerSpan extracts an inbound W3C trace-context (traceparent /
// tracestate) so bri joins an existing trace, then opens a SERVER-kind span
// named after the matched route pattern (low cardinality) with the standard
// HTTP attributes plus the bri request-id. Returns the opaque handle.
func startServerSpan(name, method, route, requestID, traceparent, tracestate string) any {
	mu.Lock()
	t := tracer
	mu.Unlock()
	if t == nil {
		// Middleware calls -otel-init first; guard anyway so a stray call is a
		// no-op rather than a nil deref.
		initProvider("")
		mu.Lock()
		t = tracer
		mu.Unlock()
	}
	carrier := propagation.MapCarrier{}
	if traceparent != "" {
		carrier["traceparent"] = traceparent
	}
	if tracestate != "" {
		carrier["tracestate"] = tracestate
	}
	ctx := prop.Extract(context.Background(), carrier)
	attrs := []attribute.KeyValue{
		attribute.String("http.request.method", strings.ToUpper(method)),
	}
	if route != "" {
		attrs = append(attrs, attribute.String("http.route", route))
	}
	if requestID != "" {
		attrs = append(attrs, attribute.String("bri.request_id", requestID))
	}
	spanName := name
	if spanName == "" {
		spanName = strings.TrimSpace(strings.ToUpper(method) + " " + route)
	}
	ctx, span := t.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(attrs...))
	return &spanHandle{ctx: ctx, span: span}
}

// endServerSpan records the response status (attribute + span status —
// error on 5xx, following the OTel HTTP-server convention) and the resolved
// subject, then ends the span.
func endServerSpan(h any, status int, subject string) {
	sh, ok := h.(*spanHandle)
	if !ok || sh.span == nil {
		return
	}
	sh.span.SetAttributes(attribute.Int("http.response.status_code", status))
	if subject != "" {
		sh.span.SetAttributes(attribute.String("enduser.id", subject))
	}
	if status >= 500 {
		sh.span.SetStatus(codes.Error, "")
	} else {
		sh.span.SetStatus(codes.Ok, "")
	}
	sh.span.End()
}

// traceparentOf renders the span's own context as a W3C traceparent header
// value, for injecting bri's span into outbound requests (or echoing it on
// the response for correlation).
func traceparentOf(h any) any {
	sh, ok := h.(*spanHandle)
	if !ok || sh.span == nil {
		return nil
	}
	carrier := propagation.MapCarrier{}
	prop.Inject(trace.ContextWithSpan(sh.ctx, sh.span), carrier)
	if tp := carrier["traceparent"]; tp != "" {
		return tp
	}
	return nil
}

// shutdown flushes and stops the batch processor — hooked into bri.web.http's
// serve :drain so buffered spans reach the collector on SIGTERM (ADR 0074).
func shutdown() {
	mu.Lock()
	p := provider
	mu.Unlock()
	if p == nil {
		return
	}
	ctx := context.Background()
	_ = p.ForceFlush(ctx)
	_ = p.Shutdown(ctx)
}

// --- small helpers (mirrors pkg/bri's arg coercion) -------------------------

func one(name string, args []any) any {
	if len(args) != 1 {
		panicf("wrong number of args (%d) passed to: %s", len(args), name)
	}
	return args[0]
}

func str(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		return lang.ToString(v)
	}
}

func asInt(v any) int {
	switch t := v.(type) {
	case int64:
		return int(t)
	case int:
		return t
	case float64:
		return int(t)
	}
	return 0
}

func firstEnv(names ...string) string {
	for _, n := range names {
		if v := os.Getenv(n); v != "" {
			return v
		}
	}
	return ""
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func panicf(format string, a ...any) {
	panic(fmt.Errorf(format, a...))
}
