# bri.otel — opt-in OpenTelemetry tracing

Opt-in distributed tracing (ADR 0074): a server span per request, W3C
trace-context propagation, and an OTLP exporter. NEVER in `api-defaults` —
you require it and add one middleware. An app that never requires `bri.otel`
never links the OpenTelemetry SDK (~6 MB smaller).

bri already ships default-on: Prometheus `/metrics`, structured JSON access
logs, X-Request-Id propagation. bri.otel ADDS spans so logs/metrics/traces
correlate (the span carries the request-id).

Full guide on the site: https://muthuishere.github.io/cljgo/bri/otel/

## Wire it in

```clojure
(require '[bri.http :as http] '[bri.otel :as otel])

(http/serve routes {:port 3000
                    :middleware (otel/with-tracing (http/api-defaults))
                    :drain [otel/shutdown!]})
```

`with-tracing` adds the tracing middleware as the OUTERMOST stack entry (the
span wraps request-id, logging, handler). `shutdown!` on `:drain` flushes
buffered spans on SIGTERM. Or drop it onto any stack:

```clojure
(conj (http/api-defaults) (otel/trace))
```

`(otel/init!)` initializes the provider + exporter (idempotent; `trace`
calls it). `(otel/current-traceparent req)` gives the active span's W3C
traceparent to inject into outbound calls.

## What a span records

Named after the matched route PATTERN (low cardinality). Adopts an inbound
`traceparent`/`tracestate`; records method/route/status + request-id +
authenticated subject; sets status from the HTTP status; threads the
trace-id into `:trace/id` and `:bri/ctx`; injects `traceparent` on the
response. With no endpoint configured, spans still open/close (identical
dev vs AOT) — just not exported. Zero cost unused.

## Config (env; APP_* overrides OTEL_*)

- `APP_OTEL_EXPORTER_OTLP_ENDPOINT` / `OTEL_EXPORTER_OTLP_ENDPOINT` — collector URL
- `APP_OTEL_EXPORTER_OTLP_PROTOCOL` / `OTEL_EXPORTER_OTLP_PROTOCOL` — e.g. `http/protobuf`
- `APP_OTEL_SERVICE_NAME` / `OTEL_SERVICE_NAME` — `service.name` (else `:service-name`
  opt, else the bri.config app name, else `bri-app`)
- `APP_OTEL_STDOUT=1` — export spans to stdout (demos, no collector)

```bash
APP_OTEL_STDOUT=1 cljgo build run
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 cljgo build run
```

## See also

- `docs/guides/bri-http.md` — the default-on observability this builds on
- `examples/web-otel/` — this, end to end
