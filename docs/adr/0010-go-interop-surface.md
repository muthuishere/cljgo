# ADR 0010 — Go interop works like Clojure's Java interop: zero-ceremony, any module
Date: 2026-07-12 · Status: accepted (surface: design/05 §1; mechanics: design/04; spikes S2+S3)

## Context
Priority #1: the Go ecosystem is our standard library, the way the JVM was
Clojure's. The bar is JVM Clojure's Java interop seamlessness: import a class,
call it, done — no bindings, no wrappers. Prior art fails this: Joker has a
frozen stdlib; Glojure's emitted code goes through a reflection registry.

## Decision
1. Surface mirrors Clojure's Java interop, adapted to Go:
   `(:require-go [net/http :as http])` in ns / `require-go` at the REPL;
   `(http/Get "https://…")` package fns; dot forms for members —
   `(.Do client req)`, field access `(.-Timeout client)`, constructor
   `(http/Client. {...})`; `go/` reserved pseudo-namespace for operators
   (`go/new`, `go/slice-of`, `go/instantiate` for generics). No
   auto-capitalization; Go names appear as themselves.
2. ANY module, zero bindings, both modes:
   - AOT: the emitter loads go/packages type facts and emits a real `import`
     plus direct, signature-coerced, non-reflective calls (S2: works; warm
     load 50–95ms; gcexportdata disk cache ~1ms/pkg).
   - Interpreted/REPL: deps file → `go get` → generated registry → project-
     local self-rebuild + exec (S3: 1.7–2.9s warm cycle) — the REPL becomes a
     binary that links every dep; calls go through cached reflection.
   - One shaping-rule table (ADR 0005 [v err]/`!`, nil normalization,
     coercions) shared by both paths — identical semantics guaranteed by the
     dual-harness suite.
3. Errors: per ADR 0005. Exceptions in emitted code are panic/recover;
   thrown values satisfy Go `error`.

## Consequences
Adding a Go dependency feels like adding a Maven dep in JVM Clojure: declare,
one command, import, call. The registry exists ONLY interpreter-side; emitted
binaries never carry it (no 9.6k-line pkgmap). Generics instantiation and
receiver-type inference land incrementally (design/05 M4+); until inferred,
unhinted member calls fall back to reflection in AOT with a compile-time note.
