// otel_test.go — the bri.otel Go-half behavior suite (ADR 0074). No JVM
// oracle (bri.otel is bri-specific), so these are Go tests against the real
// OpenTelemetry SDK: a request-shaped span is exported with the right
// name/attributes/status, an inbound W3C traceparent is adopted (the trace
// is joined), and the span's own traceparent is injectable for outbound
// propagation. White-box (package otel) so a synchronous in-memory exporter
// can be swapped in without an env-configured collector.
package otel

import (
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// recorder installs a synchronous in-memory exporter as the global provider
// and restores the un-inited state afterward, so each test sees only its own
// spans.
func recorder(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	mu.Lock()
	provider = sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exp),
		sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", "test-svc"))))
	tracer = provider.Tracer("bri.otel")
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		provider = nil
		tracer = nil
		mu.Unlock()
	})
	return exp
}

func attrOf(kvs []attribute.KeyValue, key string) (attribute.Value, bool) {
	for _, kv := range kvs {
		if string(kv.Key) == key {
			return kv.Value, true
		}
	}
	return attribute.Value{}, false
}

// A request through start+end produces one SERVER span named after the
// route pattern, carrying method/route/status/request-id/subject and an Ok
// status for a 2xx response.
func TestServerSpanExported(t *testing.T) {
	exp := recorder(t)

	h := startServerSpan("GET /users/{id}", "get", "GET /users/{id}", "req-abc", "", "")
	endServerSpan(h, 200, "user-7")

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 exported span, got %d", len(spans))
	}
	s := spans[0]
	if s.Name != "GET /users/{id}" {
		t.Errorf("span name = %q, want the route pattern %q", s.Name, "GET /users/{id}")
	}
	if s.SpanKind.String() != "server" {
		t.Errorf("span kind = %q, want server", s.SpanKind)
	}
	if v, ok := attrOf(s.Attributes, "http.request.method"); !ok || v.AsString() != "GET" {
		t.Errorf("http.request.method = %v (ok=%v), want GET", v.AsString(), ok)
	}
	if v, ok := attrOf(s.Attributes, "http.route"); !ok || v.AsString() != "GET /users/{id}" {
		t.Errorf("http.route = %v, want the pattern", v.AsString())
	}
	if v, ok := attrOf(s.Attributes, "http.response.status_code"); !ok || v.AsInt64() != 200 {
		t.Errorf("http.response.status_code = %v, want 200", v.AsInt64())
	}
	if v, ok := attrOf(s.Attributes, "bri.request_id"); !ok || v.AsString() != "req-abc" {
		t.Errorf("bri.request_id = %v, want req-abc", v.AsString())
	}
	if v, ok := attrOf(s.Attributes, "enduser.id"); !ok || v.AsString() != "user-7" {
		t.Errorf("enduser.id = %v, want user-7", v.AsString())
	}
	if s.Status.Code != codes.Ok {
		t.Errorf("span status = %v, want Ok", s.Status.Code)
	}
}

// A 5xx response marks the span status Error (OTel HTTP-server convention).
func TestServerErrorStatus(t *testing.T) {
	exp := recorder(t)
	endServerSpan(startServerSpan("GET /", "get", "GET /", "", "", ""), 503, "")
	spans := exp.GetSpans()
	if len(spans) != 1 || spans[0].Status.Code != codes.Error {
		t.Fatalf("expected 1 span with Error status, got %+v", spans)
	}
}

// An inbound W3C traceparent is adopted: the server span's trace-id equals
// the caller's, and its parent is the caller's span-id (bri joins the trace).
func TestInboundTraceparentAdopted(t *testing.T) {
	exp := recorder(t)
	const traceID = "0af7651916cd43dd8448eb211c80319c"
	const parentSpan = "b7ad6b7169203331"
	tp := "00-" + traceID + "-" + parentSpan + "-01"

	h := startServerSpan("GET /", "get", "GET /", "", tp, "")
	sh := h.(*spanHandle)
	if got := sh.span.SpanContext().TraceID().String(); got != traceID {
		t.Errorf("adopted trace-id = %q, want inbound %q", got, traceID)
	}
	endServerSpan(h, 200, "")

	s := exp.GetSpans()[0]
	if s.Parent.SpanID().String() != parentSpan {
		t.Errorf("parent span-id = %q, want inbound %q", s.Parent.SpanID(), parentSpan)
	}
	if !s.Parent.IsRemote() {
		t.Error("parent should be marked remote (came off the wire)")
	}
}

// With no inbound traceparent, a fresh root trace is minted (a valid,
// non-zero trace-id, no remote parent).
func TestFreshTraceWhenNoParent(t *testing.T) {
	recorder(t)
	sh := startServerSpan("GET /", "get", "GET /", "", "", "").(*spanHandle)
	if !sh.span.SpanContext().HasTraceID() {
		t.Fatal("expected a minted trace-id for a parentless request")
	}
}

// traceparentOf renders the span's own W3C header, carrying its trace-id —
// the value bri injects into outbound requests / echoes on the response.
func TestTraceparentInjection(t *testing.T) {
	recorder(t)
	h := startServerSpan("GET /", "get", "GET /", "", "", "")
	sh := h.(*spanHandle)
	out := traceparentOf(h)
	tp, ok := out.(string)
	if !ok || tp == "" {
		t.Fatalf("traceparentOf = %v, want a non-empty header string", out)
	}
	if got := sh.span.SpanContext().TraceID().String(); got == "" || !containsSub(tp, got) {
		t.Errorf("traceparent %q does not carry the span trace-id %q", tp, got)
	}
}

func containsSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
