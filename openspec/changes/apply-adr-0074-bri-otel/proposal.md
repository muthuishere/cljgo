# apply-adr-0074-bri-otel

## Why

ADR 0074 (docs/adr/0074-bri-otel-opt-in-tracing.md) adds the tracing half of
bri's observability tier. bri already ships default-on Prometheus metrics,
structured JSON logs, and `X-Request-Id` propagation (ADR 0069); what is missing
is **OpenTelemetry** — spans, W3C trace-context propagation, and an OTLP
exporter — so a bri service joins a distributed trace and its logs/metrics/
traces correlate. The OpenTelemetry Go SDK is pure Go (builds `CGO_ENABLED=0`)
but heavy (~6 MB, ~15 transitive modules), so it must be **opt-in** and opt-in
must mean **zero cost when unused**: a bri app that never requires tracing must
not carry a single OpenTelemetry symbol. bri went AOT in ADR 0071 by
blank-importing the whole umbrella `pkg/briaot` into every bri binary — which is
exactly why bri.db (ADR 0072) links SQLite into every binary. bri.otel cannot
repeat that; it needs a linking mechanism that keeps the SDK out of non-tracing
binaries.

## What Changes

- **`bri.otel` becomes a real, opt-in bri namespace.** `core/bri/otel.cljg` (the
  Clojure API) + Go shims in an **isolated** `pkg/bri/otel` package (the
  OpenTelemetry SDK wiring), loaded through the same lib-provider mechanism as
  every bri namespace — identical interpreted (`cljgo dev`) and AOT-compiled
  (`CGO_ENABLED=0` static). NOT in `api-defaults`; added explicitly.
- **Tracing middleware `(otel/trace)`.** A server span per request named after
  the matched route PATTERN (low cardinality), attributes for
  method/route/status + request-id + subject, span status from the HTTP status,
  ended on response. Plus `(otel/with-tracing stack)`, `(otel/init! opts)`,
  `(otel/shutdown!)`, `(otel/current-traceparent req)`.
- **W3C trace-context propagation.** Standard OTel `TraceContext` propagator:
  extract inbound `traceparent`/`tracestate` (bri joins the caller's trace),
  echo/inject the span's own header.
- **OTLP exporter.** Configured by `OTEL_EXPORTER_OTLP_ENDPOINT` /
  `OTEL_EXPORTER_OTLP_PROTOCOL` (+ `APP_OTEL_*` overrides), batch span
  processor, `service.name` from bri.config app name (default `bri-app`),
  graceful flush via `otel/shutdown!` hooked into serve's SIGTERM `:drain`.
  No endpoint ⇒ spans created but not exported (identical behavior);
  `APP_OTEL_STDOUT=1` ⇒ stdout exporter for demos.
- **The opt-in linking mechanism (the key deliverable).** New machinery:
  `bri.Spec.OptIn` + `ShimImport`, `bri.RegisterInstaller`, a genbri-emitted
  per-opt-in-namespace `provider.go` that self-registers the lib provider and
  blank-imports the isolated shim package, and `Program.OptInBriPkgs` so the
  emitter blank-imports `pkg/briaot/briotel` into `main` ADDITIVELY — only when
  the app requires bri.otel. The umbrella `pkg/briaot` EXCLUDES opt-in
  namespaces, so the SDK links only in tracing binaries.
- **Correlation, not duplication.** The span carries the request-id; its
  trace-id threads into `:trace/id` and `:bri/ctx`; metrics stay the existing
  registry.
- **A worked example + docs.** Tracing wired into an example, exporting a span
  to a stdout/OTLP collector, plus an "add tracing" note.

## Non-goals

- Making ALL of `pkg/briaot` per-namespace conditional (investigated and found
  unsafe: bri.http's `api-defaults` does a runtime `(require 'bri.auth)` inside a
  function body that build-time discovery cannot see — a per-namespace-linked app
  using `listen`/`api-defaults` would fail at runtime). The conditional import is
  scoped to opt-in namespaces; the general path is future work.
- Making bri.db opt-in in this change (the mechanism now exists; the remaining
  step is extracting SQLite/pgx out of `pkg/bri`, which shares unexported
  helpers with the db shims — deferred to keep the db parity suite intact).
- Client-side auto-instrumentation, metrics-over-OTLP, or logs-over-OTLP (traces
  only here; bri keeps Prometheus metrics + JSON logs).
- Forcing tracing on in api-defaults, or a Go build tag as the opt-in surface
  (a `require` is the opt-in).

## Impact

- Affected specs: `bri-otel` (new capability).
- Affected code: `core/bri/otel.cljg`, `core/bri.go`, `pkg/bri/otel/` (new),
  `pkg/bri/bri.go` (Spec.OptIn/ShimImport, RegisterInstaller, registry-backed
  InstallShimsInto), `cmd/genbri/main.go` (opt-in provider generation),
  `pkg/emit/module.go` + `pkg/emit/program.go` (OptInBriPkgs additive import),
  `pkg/briloader/briloader.go` (blank-import the isolated shim pkg), regenerated
  `pkg/briaot/` (+ `briotel`), root `go.mod`/`go.sum` (OpenTelemetry deps).
