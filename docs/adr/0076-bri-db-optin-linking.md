# ADR 0076 — bri.db is opt-in, linked only when required (closing the ADR 0072 SQLite tradeoff)

Date: 2026-07-24 · Status: accepted (owner-directed: *"move bri.db as separate
dependency like others"*). Realizes the step ADR 0074 §4 named and deferred.
Builds on ADR 0072 (bri.db data layer) and ADR 0074 (bri.otel opt-in linking,
which built the machinery).

## Context

ADR 0072 shipped `bri.db` (pure-Go SQLite via modernc.org/sqlite + Postgres via
jackc/pgx) with its host shims in `pkg/bri/db.go`. Because `db.go` lives *inside*
`pkg/bri`, and the always-linked umbrella `pkg/briaot` imports `pkg/bri`, **every**
bri binary linked SQLite + pgx (~7 MB, plus their transitive modules) whether or
not it touched a database — the exact cost ADR 0072 accepted and flagged as a
"future optimization," and the same tradeoff ADR 0074 refused to repeat for the
OpenTelemetry SDK.

ADR 0074 solved it *for bri.otel* and, in §4, named the remaining step to close
the bri.db tradeoff verbatim: *"extract SQLite/pgx out of `pkg/bri` into an
isolated package and flip bri.db's Spec to `OptIn`."* It was deferred there only
because `db.go` shares unexported helpers with `pkg/bri` and the extraction
touches the db parity suite. The opt-in machinery it built is fully general —
`bri.Spec.OptIn`/`ShimImport`, `bri.RegisterInstaller`, genbri's per-opt-in
`provider.go` + umbrella exclusion, and the emitter's `Program.OptInBriPkgs`
additive blank-import all key off `s.OptIn`, not off a hardcoded "otel." So this
is the mechanical application of an existing, proven mechanism to a second
namespace.

Safety precondition (verified): no always-on bri namespace performs a runtime
`(require 'bri.db)` inside a function body (the hazard that made *general*
per-namespace linking unsafe in ADR 0074 §4 — bri.http/api-defaults dynamically
requires bri.auth). `bri.db` is referenced only by app-level code that requires
it statically at the top of a namespace, so build-time discovery always sees it.
`core/bri/audit.cljg` mentions bri.db only in comments (a documented future seam),
never as a live require.

## Decision

1. **Extract the heavy Go shims into an isolated package `pkg/bri/db`.** All of
   `pkg/bri/db.go` (the two pure-Go driver blank-imports, `dbOpen`/`dbQuery`/
   `dbExec`/`handleOf`/`migrationFiles`/placeholder rewriting, and
   `installDBShims`) moves to `package db` under `pkg/bri/db`. Like `pkg/bri/otel`
   it duplicates the few trivial helpers it needs (`asString`, `one`,
   `keywordName`, `getenvShim`) rather than exporting them from `pkg/bri` — the
   same choice bri.otel made — so `pkg/bri` keeps **zero** edges to SQLite/pgx.
   The package registers its installer with `pkg/bri` from `init()` via
   `bri.RegisterInstaller("bri.db", installDBShims)`, so the drivers link exactly
   when the package is linked.

2. **Flip bri.db's Spec to `OptIn`.** In `pkg/bri.Specs()`, bri.db becomes
   `install: nil, OptIn: true, ShimImport: "…/pkg/bri/db"` — identical in shape to
   bri.otel. It is thereby **excluded from the umbrella** `pkg/briaot`; its
   compiled sub-package `pkg/briaot/bridb` becomes a self-registering opt-in
   provider (genbri emits `provider.go` for it, unchanged mechanism), and the
   emitter blank-imports `pkg/briaot/bridb` into `main` **only when the app
   requires bri.db**.

3. **Interpreter path links the drivers unconditionally**, as for otel:
   `pkg/briloader` blank-imports `pkg/bri/db` so `cljgo dev`/REPL/conformance-eval
   (the `cljgo` binary already links the whole interpreter) resolve bri.db's shims
   the moment an app `(require '[bri.db])`s. The zero-cost guarantee is a property
   of the AOT **user binary** only (`pkg/briaot` sub-packages), never the `cljgo`
   tool itself.

4. **Zero machinery changes.** genbri (`writeOptInProvider`, umbrella exclusion)
   and the emitter (`module.go` discovery, `Program.OptInBriPkgs`) are already
   generic over `s.OptIn`; this ADR adds no new mechanism — it is the flip plus the
   package move, then a `go run ./cmd/genbri` regeneration.

## Consequences

- A bri binary that never requires bri.db carries **zero** SQLite/pgx symbols
  (proven by a `go list -deps` test symmetric to ADR 0074's otel proof: the
  umbrella `pkg/briaot`, `pkg/bri`, and `pkg/briaot/brihttp` link no
  `modernc.org/sqlite`; only `pkg/briaot/bridb` does) — a bri.http hello-world
  shrinks by ~7 MB, fully closing the ADR 0072 tradeoff.
- Apps that use the database are unchanged: the generated web template and the
  resource generator (ADR 0073) already `(require '[bri.db :as db])`, so
  discovery links bridb for them automatically; runtime behavior is byte-identical.
- Dual-mode parity is preserved: the single `pkg/bri/db` shim implementation drives
  both the interpreter (via briloader) and the AOT binary (via `pkg/briaot/bridb`),
  so interpreted and compiled bri.db stay structurally identical (the release
  blocker). The existing black-box db behavior suite (`pkg/bri/db_test.go`,
  `package bri_test`) drives everything through the interpreter and is unaffected
  by where the Go shims live.
- Two opt-in namespaces now exercise the same path, hardening ADR 0074's mechanism
  as the standard way every future battery (ADR 0075) links: isolate heavy deps →
  mark `OptIn` → self-register.
- Not chosen: exporting `pkg/bri`'s helpers for reuse (duplicating four trivial
  funcs keeps the isolated package self-contained and matches bri.otel); a Go build
  tag (a static `require` is a cleaner opt-in surface, per ADR 0074).
