# c-ffi

## Why

Owner mandate (design/05 §C-FFI, priority 5): "any ffi can be imported and
used directly" — raw C libraries with no Go binding, dlopen-live at the
REPL, no C toolchain required. ADR 0011 (spike S7) settled the marshaling
patterns (strings, pointer out-params, callbacks, `:rc` error convention)
against real sqlite3 on darwin/arm64, and left two questions open: can the
interpreter's runtime-typed declaration actually drive purego's
compile-time-shaped registration API, and where does the `purego`
dependency live given cljgo's zero-third-party-deps root module. Spike
S21 (`spikes/s21-c-ffi-purego/`) answers both; **ADR 0044** (proposed)
records the decisions this change implements.

## What Changes

- **`pkg/host` type-keyword table** (owned, single source of truth): the
  union of S7's and S21's `Kind`s (`:string :int32 :int64 :float32
  :float64 :bool :ptr :ptr!out :cstr!out :callback :rc :void`) mapped to
  concrete Go/reflect types, shared by both registration strategies.
- **`ffi/deflib` special form (interpreter/eval path):** dynamic
  registration per S21's `deflib.go` — `reflect.FuncOf` builds the C
  signature from declared keywords, `reflect.New` + `purego.RegisterFunc`
  bind it, calls go through arity/type-checked `reflect.Value.Call`.
  Declaration-time failures (missing lib, missing symbol, variadic
  declaration) are named, positioned errors; a `Lib` is never left
  half-registered.
- **`ffi/deflib` emission (AOT path):** the analyzer emits the static
  form (S7-style compile-time-typed func var + `purego.RegisterLibFunc`
  in a package `init()`) whenever the declaration is comptime-visible —
  the normal case. Both paths funnel through the same purego primitives.
- **`ffi/fn`** one-off REPL binding (`(ffi/fn "lib" "symbol" [arg-kinds]
  ret-kind)`), always dynamic (no comptime declaration to emit statically).
- **`ffi/callback`**: wraps `purego.NewCallback`, cached by fn identity
  (purego's per-process callback slot ceiling, per S7).
- **Dependency placement:** `cljgo` (interpreter/REPL binary) takes
  `github.com/ebitengine/purego` as an ordinary dependency. A compiled
  program's own `go.mod` (ADR 0028, independent per program) gains
  `purego` only if that program uses `ffi/`; FFI-free programs keep a
  zero-third-party-deps `go.mod`.
- Conformance files per declared behavior (declaration success, each
  failure class, REPL-liveness re-declare, dual-mode static/dynamic
  parity), frozen against S7/S21 spike transcripts where the behavior is
  purego-mechanical (not Clojure-semantic) rather than a fresh JVM oracle
  run — there is no JVM analog for a C FFI form.

## Non-goals

- No struct-by-value marshaling (design/05's existing "wrap it in Go/cgo"
  position stands; S21 does not revisit it).
- No `ffi/from-header` (parsing real C headers to close the
  wrong-signature gap) — named as the only structural fix in ADR 0044,
  explicitly out of scope here.
- No variadic C function support of any kind (ADR 0011, reaffirmed S21).
- No new conditional-import/build-tag machinery — dependency placement
  rides the EXISTING per-program module (ADR 0028), nothing new invented.
- No perf budget gate (ADR 0024) in this change's first tier — S21's
  numbers (purego static 132ns, dynamic 221ns/call on darwin/arm64) are
  disclosed as a baseline; a CI-enforced budget is a task in its own
  right once conformance tests exist to budget against (tracked below).
- No non-Tier-1 platform conformance (BSDs, 386/arm/etc.) — documented as
  best-effort only, per ADR 0044 §4.

## Impact

- `pkg/host` (new type-keyword table), `pkg/eval` (`ffi/deflib` interpreted
  form, `ffi/fn`, `ffi/callback`), `pkg/analyzer`+emitter (static `ffi/deflib`
  emission), `cmd/cljgo`'s `go.mod` (first non-tooling dependency: purego),
  `conformance/tests/ffi-*.clj`.
- No change to any existing clojure.core/reader semantics (precedence
  principle: `ffi/*` is a new namespace, never touches `clojure.core`).
