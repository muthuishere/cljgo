## Why

ADR 0010 (accepted) makes Go interop cljgo's priority #1: the Go ecosystem is
our standard library the way the JVM was Clojure's — import a package, call it,
done, no bindings. Spike S2 proved the AOT half end-to-end (zero-binding direct
calls via go/packages, `grep -c reflect` → 0); the interpreted half follows
Glojure's reflect-registry model. This change lands the first *dual-mode*,
dual-harness-verified slice so the thesis is real and running, not just spiked.

## What Changes

- **Surface (both modes):** `(:require-go [strings :as s] [strconv])` in `ns`
  and `require-go` at the REPL register a Go package as a Clojure namespace;
  `(strconv/Itoa 42)` calls package fns, `math/Pi` reads consts/vars.
- **Shaping table (shared):** ADR 0005's `(T,error)` → `[v err]` vector,
  comma-ok `(T,bool)` → `[v ok]`, `!`-suffix (`strconv/Atoi!`) unwraps-or-throws,
  nil normalization, Go int→int64 / float→float64 return widening. One rule set,
  applied by the reflect path AND the emitted Go, so behavior is identical.
- **AST contract (landed):** `OpHostRef` (value position) / `OpHostCall`
  (invoke), analyzer `ResolveHost` hook — Clojure resolves first, host is the
  fallback (precedence principle).
- **Interpreted path:** `require-go` builtin + per-ns alias table + a
  reflect-backed seed registry (strings/strconv/math/fmt) + `evalHost`.
- **AOT path:** `genHost` resolves the callee via go/packages type facts, emits
  a real `import` + a direct non-reflective call + the shaping, and pins the
  package in the generated `go.mod` (stdlib needs no pin).
- **Conformance:** `conformance/tests/interop-*.clj` run through BOTH harnesses
  (eval + compiled), byte-identical; expectations cited against the Go stdlib
  semantic oracle.

## Non-goals

- **Member access** `(.Method x)` / `(.-Field x)` / struct ctors `(T. {...})` —
  deferred to M3.1 (S2 did not spike selector emission / receiver inference).
- **Third-party modules** via `deps.edn` `go get` + self-rebuild (design/05 M2)
  — v0 is stdlib seed only; any-module lands next.
- **genpkg auto-generation** of the interpreted registry — v0 hand-seeds a
  handful of packages; the go/types walker that registers whole packages is
  follow-up.
- **Generics, channels/go, FFI/purego** — later milestones (design/05 M3–M6).
- No change to ADR 0005 raw `[v err]` semantics or to any clojure.core name.
