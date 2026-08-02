# Tasks — apply-adr-0074-bri-otel

## 1. The isolated Go shims (pure Go, no pkg/eval)

- [x] 1.1 `pkg/bri/otel/otel.go`: a SEPARATE package holding the OpenTelemetry
  SDK wiring — `installShims` interning `-otel-init` `-otel-start` `-otel-end`
  `-otel-traceid` `-otel-traceparent` `-otel-shutdown`. Imports the OTel SDK
  (`otel`, `sdk/trace`, `sdk/resource`, `exporters/otlp/otlptrace/otlptracehttp`,
  `exporters/stdout/stdouttrace`, `propagation`, `trace`). No import of pkg/eval.
- [x] 1.2 `init()` registers the installer with `pkg/bri` via `RegisterInstaller`
  — present only when this package is linked (zero-cost).
- [x] 1.3 TracerProvider: batch OTLP exporter from `OTEL_EXPORTER_OTLP_*` +
  `APP_OTEL_*`; stdout via `APP_OTEL_STDOUT`; none ⇒ no exporter (spans still
  open/end). `service.name` resolution. `shutdown()` flush.
- [x] 1.4 W3C `TraceContext` propagator: extract inbound traceparent/tracestate,
  inject the span's own header.

## 2. The Clojure API

- [x] 2.1 `core/bri/otel.cljg` (ns `bri.otel`): `init!` `trace` `with-tracing`
  `shutdown!` `current-traceparent`. Server span named by route pattern, attrs
  (method/route/status/request-id/subject), status from HTTP status, ends on
  response, adopts inbound trace, threads `:trace/id` into `:bri/ctx`.
- [x] 2.2 Embed the source in `core/bri.go` (`//go:embed bri/otel.cljg` →
  `BriOtelSource`).

## 3. The opt-in linking mechanism

- [x] 3.1 `pkg/bri/bri.go`: `Spec.OptIn` + `Spec.ShimImport`,
  `RegisterInstaller` + an installer registry, `InstallShimsInto` falls back to
  the registry when `install` is nil. Add the `bri.otel` Spec (OptIn,
  ShimImport = pkg/bri/otel).
- [x] 3.2 `cmd/genbri/main.go`: blank-import the isolated shim pkg (so the
  installer is present at compile time); EXCLUDE OptIn specs from the umbrella
  `briaot.go`; emit a per-OptIn-namespace `provider.go` that self-registers the
  lib provider and blank-imports the ShimImport package.
- [x] 3.3 `pkg/emit`: `Program.OptInBriPkgs` recorded during discovery;
  `Options.OptInBriPkgs`; `main` blank-imports `pkg/briaot/<pkg>` additively for
  each opt-in namespace required.
- [x] 3.4 `pkg/briloader/briloader.go`: blank-import the isolated shim pkg so the
  interpreter (`cljgo dev`) has the installer.
- [x] 3.5 Add the OpenTelemetry deps to root `go.mod`/`go.sum`; regenerate
  `pkg/briaot` via `go run ./cmd/genbri -o pkg/briaot` (adds `pkg/briaot/briotel`
  + its `provider.go`). `SynthGoMod` carries the requires automatically.

## 4. Tests (no JVM oracle)

- [x] 4.1 `pkg/bri/otel/otel_test.go` (white-box, in-memory exporter): server
  span exported with name/kind/attributes/status; 5xx ⇒ Error; inbound
  traceparent adopted (trace-id + remote parent); parentless ⇒ fresh trace;
  traceparent injection carries the trace-id.
- [x] 4.2 `pkg/bri/otel_middleware_test.go` (interpreter, the same middleware the
  binary runs — dual-mode contract): `(otel/trace)` echoes a W3C traceparent and
  threads `:trace/id`; an inbound trace is adopted (same trace-id, fresh span-id).
- [x] 4.3 `pkg/briaot/optin_linking_test.go` (`go list -deps`): `pkg/briaot`,
  `pkg/bri`, `pkg/briaot/brihttp`, `pkg/briaot/bridb` link NO OpenTelemetry
  package; `pkg/briaot/briotel` does.
- [x] 4.4 Manual binary proof (recorded in the ADR): a bri.http app = 20 MB / 0
  otel strings; a bri.otel app = 26 MB / ~2200 otel strings; both CGO_ENABLED=0
  static; the traced binary joins an inbound trace and exports a correct span.

## 5. Gates + example/docs

- [x] 5.1 `go build ./... && go vet ./... && gofmt -l … && go test ./pkg/bri/…
  ./pkg/briaot/… ./pkg/emit/ ./cmd/cljgo/` green; `go test ./conformance/` green
  (emitter/genbri linking touched).
- [x] 5.2 An example wiring `bri.otel` into a bri app, exporting a span
  (stdout/OTLP) + a short "add tracing" note.
- [x] 5.3 `openspec validate apply-adr-0074-bri-otel --strict` passes; archive on
  completion.
