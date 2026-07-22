## Why

ADR 0013 says a cljgo project can publish as a Clojure library, a Go library, a
C library, or an executable — but **only the executable exists**. The owner's
goal: cljgo is a citizen of **both** ecosystems — write pure Clojure once, ship
it to Go developers and to JVM-Clojure developers from one `build.cljgo`, no
`deps.edn`. ADR 0054 decides `publish`; its dependencies (ADR 0053 "never silent
nil", ADR 0052 §6 purity walk) have both landed, so it is unblocked.

## What Changes

- **`cljgo publish <target>`** — `go` and `clojars` (the `exe` default ships
  today). One `build.cljgo` is the single source of truth (ADR 0021); no second
  manifest, either direction.
  - **`publish go`**: a go-gettable Go package (real Go signatures from type
    hints, `any` otherwise; docstrings → doc comments). Validates the exported
    surface is Go-expressible, else fails `file:line`.
  - **`publish clojars`**: pure Clojure **source** (cljgo compiles to Go, never
    JVM bytecode — so it reaches the JVM only as source the JVM's own Clojure
    compiles). Consumed via `deps.edn` `:git/url`+`:sha` (git-coord first).
- **Purity gate decides eligibility.** `publish clojars` walks the library's
  **whole transitive required surface** and refuses if any reachable form uses
  **Go interop** (`require-go`/ffi), naming the offending `file:line`. The gate
  is **`uses-go-interop?`, NOT "no Java"** (S35: Java runs on the JVM, so it does
  not disqualify a clojars artifact). A pure-Clojure library is the only artifact
  that reaches both worlds.
- **No new walk.** The validator is a predicate pass over the existing ADR-0042
  transitive-require traversal `emit.CompileProgram` already performs — the
  whole-library gate is an OR over the per-namespace taint map, the per-namespace
  gate a lookup into it. Go interop is flagged by the mere presence of the five
  analyzer host nodes (`OpHostRef/OpHostCall/OpHostMethod/OpHostField/OpHostNew`).
  A pluggable predicate slot is reserved for `ffi`/`c-link` (not yet AST ops).
- **A Java-tainted (deferred-import) namespace fails LOUD and PER-NAMESPACE** —
  hard-errors at the point it's required with `file:line` and "Java interop is
  unsupported on cljgo's Go host", never `nil` (extends ADR 0053's guarantee to
  Java). Pure namespaces of the same dependency stay usable. Optional strict
  resolve-time rejection is available.
- **A `certain-java?` courtesy diagnostic** over the self-identifying JVM
  surfaces (`(System/…)`, `(Math/…)`, `import`, `new`, `java.*`) — certain-only,
  **zero false positives**, never a gate, never guesses the undecidable bare
  dot-form `(.method obj)`. It upgrades a raw downstream compiler error to a
  named one.

## Capabilities

### New Capabilities
- `publish`: the `cljgo publish go|clojars` producers, the transitive
  `uses-go-interop?` purity gate (whole-library for clojars, per-namespace for
  use), the per-namespace loud Java-taint failure, and the certain-only
  `certain-java?` courtesy diagnostic — all riding the existing
  `emit.CompileProgram` traversal, no new resolution machinery.

### Modified Capabilities
<!-- host-resolution-parity (ADR 0053) and dependency-resolution (ADR 0052) are
     prerequisites already satisfied; not modified here. -->

## Impact

- New `pkg/publish` (or `pkg/emit` extension): the per-namespace taint classifier
  over the CompileProgram traversal; the `go` and `clojars` producers.
- `cmd/cljgo/main.go`: new `case "publish"` → `runPublish`.
- `core/build.cljg` + AOT mirror: library-target / publish declaration surface.
- Conformance: taint-classifier cases (buried Go-interop caught at `file:line`;
  pure fixture zero-FP; whole-lib == AND of per-ns).
- Frozen references adopted: S34 (transitive purity/taint classifier), S35
  (certain-java? predicate).
- Owed ADR 0013 note: `c-shared`/`c-archive` producers remain its later work.
