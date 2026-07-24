---
title: "bri.core.telemetry"
description: "Opt-in OpenTelemetry distributed tracing: a server span per request, W3C trace-context propagation, and an OTLP exporter — linked only when you require it, zero cost otherwise."
---

`bri.core.telemetry` adds **opt-in** OpenTelemetry distributed tracing (ADR 0074). It is never in `api-defaults` — you require it and add one middleware. A bri app that never requires `bri.core.telemetry` never links the OpenTelemetry SDK (the binary stays ~6 MB smaller); a bri app that does gets spans, W3C trace-context propagation, and an OTLP exporter.

What bri already ships **default-on** (do not rebuild): Prometheus metrics at `/metrics`, structured JSON access logs, and X-Request-Id propagation — all covered in [bri.web.http](/cljgo/bri/http/) and [bri.core.security](/cljgo/bri/auth/). `bri.core.telemetry` *adds* spans so a bri service joins a distributed trace and its logs/metrics/traces correlate (the span carries the request-id).

## Wire it in

```clojure
(require '[bri.web.http :as http] '[bri.core.telemetry :as otel])

(http/serve routes {:port 3000
                    :middleware (otel/with-tracing (http/api-defaults))
                    :drain [otel/shutdown!]})
```

`otel/with-tracing` adds the tracing middleware as the **outermost** entry of an existing stack, so the span wraps request-id, logging, and the handler. `otel/shutdown!` on `:drain` flushes the batch span processor on SIGTERM so buffered spans reach the collector before the process exits.

Or drop the middleware onto any stack yourself:

```clojure
(conj (http/api-defaults) (otel/trace))
```

`(otel/trace)` returns a `{:name :otel :wrap fn}` middleware value — compose it like any other. `(otel/init!)` initializes the tracer provider + exporter explicitly (idempotent; `trace` calls it for you).

## What a span records

Each request opens a **server** span named after the matched route **pattern** (low cardinality — `GET /api/notes/{id}`, not the raw path). The middleware:

- **adopts** an inbound W3C `traceparent`/`tracestate`, so bri joins the caller's trace;
- records method, route, status, the request-id, and the authenticated subject as span attributes;
- sets span status from the HTTP status;
- threads the span's trace-id into `:trace/id` and the shared `:bri/ctx`, so logs, metrics and traces line up on one id;
- **injects** the span's `traceparent` onto the response for downstream correlation.

To propagate the trace on an **outbound** call, read the current traceparent and set it as a header:

```clojure
(otel/current-traceparent req)   ; the W3C traceparent for the active span, or nil
```

With no endpoint configured, spans are still opened and closed (identical behavior across dev and AOT) — they are simply not exported. Zero cost when unused.

## Configuration

Config is env-driven; `APP_*` overrides the standard `OTEL_*`:

| env | meaning |
|---|---|
| `APP_OTEL_EXPORTER_OTLP_ENDPOINT` / `OTEL_EXPORTER_OTLP_ENDPOINT` | the collector URL |
| `APP_OTEL_EXPORTER_OTLP_PROTOCOL` / `OTEL_EXPORTER_OTLP_PROTOCOL` | e.g. `http/protobuf` |
| `APP_OTEL_SERVICE_NAME` / `OTEL_SERVICE_NAME` | `service.name` (else the `:service-name` opt, else the [bri.core.config](/cljgo/bri/config/) app name, else `bri-app`) |
| `APP_OTEL_STDOUT=1` | export spans to stdout — demos, no collector needed |

```bash
# print each span to stdout (no collector):
APP_OTEL_STDOUT=1 cljgo build run

# export to a collector:
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 cljgo build run
```

## Try it

```bash
# a fresh trace — one span named "GET /api/notes"
curl -s localhost:3000/api/notes

# JOIN an existing trace — same trace-id
curl -s -H 'traceparent: 00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01' \
     localhost:3000/api/notes
```

The response carries a `traceparent` header, and each access-log line's request-id matches the span's — logs, metrics, and traces all line up. The [`examples/web-otel`](https://github.com/muthuishere/cljgo/tree/main/examples/web-otel) app is this, end to end.

## Where next

- [bri.web.http](/cljgo/bri/http/) — the default-on observability (metrics, logs, request-ids) this builds on
- [bri.core.security](/cljgo/bri/auth/) — the authenticated subject recorded on each span
- [bri.core.config](/cljgo/bri/config/) — where `service.name` is resolved from
- [Deploy](/cljgo/guides/deploy/) — the tracing binary still ships as one static image
