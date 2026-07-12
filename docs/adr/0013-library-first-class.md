# ADR 0013 — Every cljgo project is a first-class library, Zig-style
Date: 2026-07-12 · Status: accepted (implementation via OpenSpec, M2+)

## Context
Zig treats libraries as the default unit: anything builds as exe or lib,
consumable across the C ABI. cljgo's emitted output is already a normal Go
module (ADR 0001) — that unlocks library-ness in every direction if we make
it a product feature rather than an accident.

## Decision
A cljgo project builds, from ONE codebase, as:
1. **A Clojure library** — source consumed by other cljgo projects via deps
   (path/git), loaded interpreted or AOT'd by the consumer.
2. **A Go library** — `cljgo build --lib` emits a tidy, stable, go-gettable
   Go package (exported fns get real Go signatures from type hints where
   available, boxed `any` otherwise, doc comments from docstrings): plain Go
   programs import Clojure-written libraries with `go get`. Interop becomes
   bidirectional — the Go ecosystem consumes US.
3. **A C library** — `cljgo build --c-shared/--c-archive` via Go's own
   buildmodes: .so/.a + header, callable from C/Python/Ruby/anything (the
   Gloat precedent proves viability).
4. **An executable** — `cljgo build` (default) — plus WASM later via GOOS=js/
   wasip1, same pipeline.
Naming/munging, init (Load ordering), and the exported-surface annotation
(e.g. ^:export or an exports map in the project file) are settled in the
OpenSpec design round.

## Consequences
"Write it in Clojure, ship it to anyone" — including Go teams who will never
run a Lisp toolchain. Requires emitted-API stability guarantees (munging
scheme becomes a public contract from M2 — choose once, carefully) and go.mod
versioning discipline for emitted libs. Differentiator: no other Clojure
emits libraries a Go program can natively import.
