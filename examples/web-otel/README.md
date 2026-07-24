# web-otel — adding OpenTelemetry tracing to a bri app (ADR 0074)

bri ships observability **default-on**: Prometheus metrics at `/metrics`,
structured JSON access logs, and `X-Request-Id` propagation. This example adds
the one thing that is **opt-in** — **distributed tracing**.

## Add tracing in two lines

```clojure
(ns app.main
  (:require [bri.http :as http]
            [bri.otel :as otel]))          ; 1. require it

(http/serve routes {:port 3000
                    :middleware (otel/with-tracing (http/api-defaults))   ; 2. wrap the stack
                    :drain [otel/shutdown!]})                             ; flush on SIGTERM
```

Or drop the middleware onto any stack: `(conj (http/api-defaults) (otel/trace))`.
It is **never** in `api-defaults` — a bri app that doesn't require `bri.otel`
never links the OpenTelemetry SDK (~6 MB smaller, zero otel symbols).

## What you get

- A **server span per request**, named after the matched route **pattern**
  (`GET /api/notes/{id}`, low cardinality), with `http.request.method`,
  `http.route`, `http.response.status_code`, the bri **request-id**
  (`bri.request_id`), and the authenticated subject (`enduser.id`). Span status
  follows the HTTP status.
- **W3C trace-context propagation**: an inbound `traceparent` is adopted (bri
  **joins** the caller's trace), and the span's `traceparent` is echoed on the
  response / injectable into outbound calls (`otel/current-traceparent`).
- **Correlation**: the span's trace-id is threaded into `:trace/id` and the
  shared `:bri/ctx`, so logs, metrics, and traces line up on request-id and
  trace-id.

## Run it

```bash
# no collector needed — print each span to stdout:
APP_OTEL_STDOUT=1 cljgo build run

# export to a real OTLP collector (Jaeger/Tempo/otel-collector on :4318):
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 cljgo build run
```

```bash
curl -s localhost:3000/api/notes                       # a fresh trace
curl -s -H 'traceparent: 00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01' \
     localhost:3000/api/notes                           # JOINS that trace
```

The response carries a `traceparent`; with `APP_OTEL_STDOUT=1` you'll see the
exported span with the same trace-id, and the access-log line's request-id
matches the span's `bri.request_id`.

## Config surface

| env | meaning |
|-----|---------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` / `APP_OTEL_EXPORTER_OTLP_ENDPOINT` | collector URL (OTLP/HTTP) |
| `OTEL_EXPORTER_OTLP_PROTOCOL` / `APP_OTEL_EXPORTER_OTLP_PROTOCOL` | `http/protobuf` (default) |
| `OTEL_SERVICE_NAME` / `APP_OTEL_SERVICE_NAME` | `service.name` (else the bri.config app name, else `bri-app`) |
| `APP_OTEL_STDOUT=1` | export spans to stdout (demos; no collector) |

With no endpoint configured, requests are still traced (spans opened/ended) —
they are simply not exported. Behavior is identical interpreted (`cljgo dev`)
and AOT-compiled.
