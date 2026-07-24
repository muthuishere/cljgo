## ADDED Requirements

### Requirement: bri.otel is an opt-in tracing namespace, both modes identical

`bri.otel` SHALL be a bri namespace loaded through the ADR 0071 lib-provider
mechanism, requireable as `(require '[bri.otel])` and behaving identically
interpreted (`cljgo dev`) and AOT-compiled to a `CGO_ENABLED=0` static binary.
It SHALL NOT be part of `bri.http/api-defaults` and SHALL never be applied
unless the application explicitly adds it. Its Go half SHALL be pure Go so a
tracing app still compiles to a static binary.

#### Scenario: adding tracing is explicit

- **WHEN** an app builds its stack with `(otel/with-tracing (http/api-defaults))`
  or `(conj (http/api-defaults) (otel/trace))`
- **THEN** requests are traced; an app that does neither is never traced

#### Scenario: a tracing app AOT-compiles static

- **WHEN** a bri app that requires `bri.otel` is built with `CGO_ENABLED=0`
- **THEN** it produces a single static binary that opens/ends spans at runtime

### Requirement: opt-in means zero cost when unused

The OpenTelemetry SDK SHALL live in an isolated Go package that no
always-linked bri package imports. A bri binary that does not require `bri.otel`
SHALL contain ZERO OpenTelemetry packages in its dependency closure and zero
OpenTelemetry symbols/strings; a binary that requires it SHALL link the SDK. The
emitter SHALL blank-import the opt-in sub-package into `main` only when the app
required the namespace during discovery.

#### Scenario: a non-tracing app carries no SDK

- **WHEN** a bri.http app (no `bri.otel`) is AOT-compiled
- **THEN** `pkg/briaot`, `pkg/bri`, and its always-linked sub-packages link no
  `go.opentelemetry.io` package, and the binary contains no OpenTelemetry
  strings

#### Scenario: a tracing app links the SDK

- **WHEN** a bri app requires `bri.otel` and is AOT-compiled
- **THEN** `pkg/briaot/briotel` (linked into that binary only) carries the SDK
  and the binary can export spans

### Requirement: a server span per request, named by route pattern

The `(otel/trace)` middleware SHALL open one `SpanKind=Server` span per request
named after the matched route PATTERN (`:bri.http/route`, low cardinality — not
the raw path), record `http.request.method`, `http.route`,
`http.response.status_code`, the bri request-id (`bri.request_id`), and the
authenticated subject (`enduser.id`) when a guard resolved one, set the span
status from the HTTP status (Error on 5xx, else Ok), and end the span on
response (status 500 on a thrown handler). The span's trace-id SHALL be threaded
into `:trace/id` and the shared `:bri/ctx` so logs/metrics/traces correlate.

#### Scenario: span carries request shape and correlates with the request-id

- **WHEN** a request matches `GET /users/{id}` and returns 200
- **THEN** the exported span is named `GET /users/{id}`, is Server-kind, carries
  method/route/status attributes plus the request-id, and has Ok status

#### Scenario: a 5xx marks the span an error

- **WHEN** a handler returns status 503
- **THEN** the span status is Error

### Requirement: W3C trace-context propagation

bri.otel SHALL use the standard OTel `TraceContext` propagator. An inbound
`traceparent`/`tracestate` SHALL be extracted so the server span joins the
caller's trace (same trace-id, parent = the caller's remote span-id); the span's
own `traceparent` SHALL be renderable for injection into outbound requests and
echoed on the response.

#### Scenario: an inbound trace is joined

- **WHEN** a request arrives with `traceparent: 00-<trace>-<span>-01`
- **THEN** the server span's trace-id equals `<trace>` and its parent is `<span>`
  (remote), and the response `traceparent` carries the same trace-id with a
  fresh span-id

#### Scenario: a parentless request mints a root trace

- **WHEN** a request arrives with no `traceparent`
- **THEN** a fresh, valid trace-id is minted and echoed on the response

### Requirement: OTLP exporter, config surface, graceful shutdown

bri.otel SHALL configure an OTLP exporter with a batch span processor from
`OTEL_EXPORTER_OTLP_ENDPOINT` / `OTEL_EXPORTER_OTLP_PROTOCOL` (with `APP_OTEL_*`
overrides). `service.name` SHALL come from the `:service-name` option, else the
bri.config app name, else `APP_OTEL_SERVICE_NAME`/`OTEL_SERVICE_NAME`, else
`bri-app`. With no endpoint configured, spans SHALL still be opened/ended but not
exported (behavior identical across modes); `APP_OTEL_STDOUT=1` SHALL select a
stdout exporter. `(otel/shutdown!)` SHALL flush and stop the processor and SHALL
be usable in serve's `:drain` for graceful SIGTERM shutdown.

#### Scenario: spans export to a configured collector

- **WHEN** `OTEL_EXPORTER_OTLP_ENDPOINT` (or `APP_OTEL_*`) points at a collector
- **THEN** ended spans are batched and exported to it, and `otel/shutdown!`
  flushes buffered spans on drain

#### Scenario: no collector is not an error

- **WHEN** no endpoint is configured
- **THEN** requests are still traced (spans opened/ended) but nothing is
  exported, and the app runs normally
