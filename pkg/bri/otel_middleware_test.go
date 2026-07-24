// otel_middleware_test.go — bri.otel's tracing MIDDLEWARE through the real
// interpreter (ADR 0074). The AOT-compiled binary runs the same middleware
// over the same pkg/bri/otel shims, so the request-visible behavior asserted
// here (span opened per request, inbound traceparent adopted, the span's
// traceparent echoed on the response, request-id correlation) is the
// dual-mode contract — a REPL↔binary divergence would be a release blocker.
package bri_test

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/repl"
)

func otelDriver(t *testing.T) *repl.Driver {
	t.Helper()
	t.Setenv("BRI_DEV", "")
	return repl.New(nil, io.Discard.(io.Writer), os.Stderr)
}

func otelEval(t *testing.T, d *repl.Driver, code string) any {
	t.Helper()
	v, err := d.EvalString(code, "otel_mw_test")
	if err != nil {
		t.Fatalf("eval %q: %v", code, err)
	}
	return v
}

const otelPrelude = `
(require '[bri.http :as http] '[bri.otel :as otel])
(defn home [req] {:status 200 :body (str "trace=" (:trace/id req))})
(def routes [["GET /users/{id}" #'home]])
(def stack (otel/with-tracing (http/defaults)))
`

// A request through (otel/trace) echoes a W3C traceparent header on the
// response and threads the span's trace-id into the request (:trace/id).
func TestOtelMiddlewareEmitsTraceparent(t *testing.T) {
	d := otelDriver(t)
	otelEval(t, d, otelPrelude)
	tp, ok := otelEval(t, d, `(get (:headers (http/request routes {:method "GET" :path "/users/7"} {:middleware stack})) "traceparent")`).(string)
	if !ok || !strings.HasPrefix(tp, "00-") {
		t.Fatalf("response traceparent = %v, want a W3C 00-... header", tp)
	}
	body := otelEval(t, d, `(:body (http/request routes {:method "GET" :path "/users/7"} {:middleware stack}))`).(string)
	if !strings.HasPrefix(body, "trace=") || body == "trace=" {
		t.Fatalf("handler saw :trace/id = %q, want a non-empty trace-id", body)
	}
}

// An inbound traceparent is adopted: the response traceparent carries the
// SAME trace-id (bri joined the caller's trace), with a fresh span-id.
func TestOtelMiddlewareAdoptsInboundTrace(t *testing.T) {
	d := otelDriver(t)
	otelEval(t, d, otelPrelude)
	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	in := "00-" + traceID + "-00f067aa0ba902b7-01"
	out, _ := otelEval(t, d, `(get (:headers (http/request routes {:method "GET" :path "/users/7"
	                    :headers {"traceparent" "`+in+`"}} {:middleware stack})) "traceparent")`).(string)
	if !strings.Contains(out, traceID) {
		t.Fatalf("response traceparent %q did not adopt inbound trace-id %q", out, traceID)
	}
	if out == in {
		t.Fatalf("span-id was not replaced — %q equals the inbound header (should be a child span)", out)
	}
}
