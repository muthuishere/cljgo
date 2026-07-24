# ADR 0074 — bri.otel is opt-in OpenTelemetry tracing, linked only when required

Date: 2026-07-24 · Status: accepted (owner-directed). Builds on ADR 0069
(bri API-first), ADR 0071 (bri AOT static binary), ADR 0072 (bri.db). Realizes
the tracing half of ADR 0041's observability tier.

## Context

bri already ships, **default-on** (ADR 0069): Prometheus metrics at `/metrics`
(`bri_http_requests_total`, `bri_http_request_duration_ms`), structured JSON
access logs, and `X-Request-Id` propagation (mint/honor/echo, threaded through
a `:bri/ctx` atom). What is missing is **distributed tracing**: OpenTelemetry
spans, W3C trace-context propagation, and an OTLP exporter — so a bri service
joins a trace that spans multiple services and its logs/metrics/traces
correlate.

The OpenTelemetry Go SDK (`go.opentelemetry.io/otel`, `.../sdk`,
`.../exporters/otlp/otlptrace/otlptracehttp`) is **pure Go** — verified to
build under `CGO_ENABLED=0` — so it does not threaten the sacred static-binary
constraint. But it is **heavy**: linking it adds ~6 MB to a bri binary (a
bri.http hello-world grows 20 MB → 26 MB) and pulls ~15 transitive modules
(gRPC, protobuf, genproto). Tracing is not something every bri app wants, so it
must be **opt-in**, and opt-in has to mean **zero cost when unused** — a bri app
that never requires tracing must not carry a single OpenTelemetry symbol.

This is precisely the tradeoff ADR 0072 flagged and deferred for bri.db: today
the emitted `main` blank-imports the whole umbrella `pkg/briaot`, which imports
`pkg/bri` (whose `db.go` blank-imports SQLite + pgx) and every AOT sub-package —
so **every** bri binary links SQLite (+~7 MB) whether or not it uses the
database. ADR 0072 accepted that cost and named a build-tag opt-out a "future
optimization." bri.otel cannot make the same tradeoff: an always-linked
OpenTelemetry SDK in every bri binary is not acceptable.

## Decision

### 1. bri.otel is a normal, opt-in bri namespace

`core/bri/otel.cljg` + Go shims, loaded through the same lib-provider mechanism
as every other bri namespace (ADR 0071), identical **interpreted** (`cljgo dev`)
and **AOT-compiled** (`CGO_ENABLED=0` static binary). It is **NOT** in
`api-defaults` and never forced on. Adding it is explicit:

```clojure
(require '[bri.http :as http] '[bri.otel :as otel])

(http/serve routes {:port 3000
                    :middleware (otel/with-tracing (http/api-defaults))
                    :drain [otel/shutdown!]})
;; or drop the middleware onto any stack:
;;   (conj (http/api-defaults) (otel/trace))
```

The API: `(otel/trace)` / `(otel/trace opts)` is the tracing middleware;
`(otel/with-tracing stack)` conjoins it as the outermost entry;
`(otel/init! opts)` (idempotent) builds the provider; `(otel/shutdown!)` flushes
the batch processor (add to `:drain`); `(otel/current-traceparent req)` yields
the active span's W3C header for outbound calls.

### 2. Behavior

- **Server span per request**, named after the matched route **PATTERN**
  (`:bri.http/route`, low cardinality — never the raw path), `SpanKind=Server`,
  with attributes `http.request.method`, `http.route`,
  `http.response.status_code`, `bri.request_id` (bridging the existing
  request-id), and `enduser.id` (the guard-resolved subject, read after
  handling). Span status is set from the HTTP status (Error on 5xx, else Ok).
  The span is ended on response (and on a thrown handler, status 500).
- **W3C trace-context propagation** via the standard OTel `TraceContext`
  propagator: an inbound `traceparent`/`tracestate` is extracted so bri **joins
  the caller's trace** (the server span's trace-id equals the caller's, its
  parent is the caller's remote span-id); the span's own `traceparent` is
  echoed on the response and injectable into outbound requests.
- **OTLP exporter** configured by the standard `OTEL_EXPORTER_OTLP_ENDPOINT` /
  `OTEL_EXPORTER_OTLP_PROTOCOL` env, with `APP_OTEL_*` overrides; a batch span
  processor; graceful shutdown via `otel/shutdown!` hooked into serve's existing
  SIGTERM `:drain`. `service.name` comes from `:service-name` opt, else the
  bri.config app name, else `APP_OTEL_SERVICE_NAME`/`OTEL_SERVICE_NAME`, else
  `bri-app`. With **no endpoint configured**, spans are still opened/closed
  (identical behavior across modes) but simply not exported; `APP_OTEL_STDOUT=1`
  selects a stdout exporter for demos.
- **Correlation**, not duplication: the span carries the request-id, the span's
  trace-id is threaded into `:trace/id` and the shared `:bri/ctx`, and metrics
  stay the existing registry. logs + metrics + traces line up on request-id and
  trace-id.

### 3. The linking mechanism — opt-in means zero cost when unused

The heavy OpenTelemetry SDK lives in an **isolated Go package**
`pkg/bri/otel`, which **nothing on the always-linked path imports** (`pkg/bri`,
the umbrella `pkg/briaot`, and every non-otel sub-package have zero edges to
it). bri.otel is marked `OptIn` in `bri.Specs()` and is **excluded from the
umbrella** `pkg/briaot`; instead its AOT sub-package `pkg/briaot/briotel`
self-registers its lib provider in its own `init()` and blank-imports
`pkg/bri/otel` (whose `init()` registers the shim installer with `pkg/bri` via
`RegisterInstaller`, so `pkg/bri` never references the SDK). The emitter tracks
required opt-in bri namespaces during discovery (`Program.OptInBriPkgs`) and
blank-imports `pkg/briaot/briotel` into `main` **additively, only when the app
requires bri.otel**. The Go linker then keeps the SDK exactly when — and only
when — an app traces.

**Proof (measured):** a bri.http app compiles to 20 MB with **0**
`go.opentelemetry.io` strings and **0** OpenTelemetry packages in its
dependency closure; a bri.otel app compiles to 26 MB with ~2200 such strings —
both `CGO_ENABLED=0` static. A `go list -deps` test asserts `pkg/briaot`,
`pkg/bri`, `pkg/briaot/brihttp`, `pkg/briaot/bridb` link no OpenTelemetry
package, while `pkg/briaot/briotel` does.

### 4. Why NOT the general per-namespace conditional import (the investigated
alternative)

The preferred first-class fix was to make **all** of `pkg/briaot` per-namespace
conditional (so bri.db would also shrink db-less apps, closing the ADR 0072
tradeoff). Investigation found this **unsafe as a general mechanism**: bri.http's
`api-defaults` performs a **runtime** `(require 'bri.auth)` *inside a function
body* (to reach `auto-ban`), and other bri code can dynamically require
namespaces at request time. Build-time discovery evaluates `require` forms but
does **not** execute function bodies, so a per-namespace-linked app that used
`listen`/`api-defaults` (the common case) would fail at runtime with an
unresolved `bri.auth` provider. Making it safe would require capturing runtime
requires (executing bodies, or conservatively linking the full bri closure),
which defeats the granularity and risks the bri dual-mode parity release
blocker.

Therefore the conditional import is **scoped to opt-in namespaces**: the
always-on set keeps the safe, unchanged umbrella; opt-in namespaces (whose
sources are explicitly `require`d at the top of a user ns, hence statically
discoverable, and whose own dependencies are explicit) are the ones linked
conditionally. This delivers zero-cost bri.otel now and provides the exact
mechanism (mark a Spec `OptIn` + isolate its heavy deps) for making bri.db
opt-in later — the remaining step to fully close the ADR 0072 tradeoff is to
extract SQLite/pgx out of `pkg/bri` into an isolated package and flip bri.db's
Spec to `OptIn`; it is deferred here only because `db.go` shares unexported
helpers with `pkg/bri` and the extraction touches the db parity suite.

## Consequences

- A bri app without tracing is byte-for-byte as it was (umbrella unchanged);
  adding `(require '[bri.otel])` is the only switch, and it costs the SDK only
  in that binary.
- bri.otel is pure Go, so a traced app still AOT-compiles to a `CGO_ENABLED=0`
  static binary; dev and compiled modes run the same `pkg/bri/otel` shims, so
  span/propagation behavior is structurally identical (dual-mode parity — the
  exported traces differ only in destination).
- `SynthGoMod` already carries the runtime's external requires into every
  emitted `go.mod` as indirect; the OpenTelemetry modules ride along but the
  linker drops them from any binary that does not import `pkg/briaot/briotel`
  (proven: 0 symbols), so no binary bloat.
- No JVM oracle (bri.otel is bri-specific): its behavior suite is Go tests in
  `pkg/bri/otel` (span/attributes/status/propagation/export against an in-memory
  exporter) + a dual-mode middleware test in `pkg/bri` (the same middleware the
  binary runs) + the `go list -deps` zero-cost proof in `pkg/briaot`.
- New machinery (reusable): `bri.Spec.OptIn` + `ShimImport`,
  `bri.RegisterInstaller`, genbri's per-opt-in-namespace `provider.go`, and the
  emitter's `Program.OptInBriPkgs` additive blank-import.
- Not chosen: a Go build tag for bri.otel (a per-namespace `require` is a
  cleaner opt-in surface than a build flag, and the import-graph mechanism needs
  no user-facing flag); linking the SDK into the umbrella; auto-enabling tracing
  in api-defaults.
