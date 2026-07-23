---
title: Architecture
description: One analyzer, two backends — a tree-walk evaluator that is the REPL, and an AOT emitter that compiles Clojure to plain Go source. The ClojureScript model, with Go as the JavaScript.
---

cljgo follows the **ClojureScript model, with Go as the JavaScript**: a
compiler written in Go that AOT-emits plain Go source, plus a tree-walk
evaluator that *is* the REPL and the macro engine. The same source runs
interpreted at the prompt and compiles to a single static native binary, with
byte-identical output on both paths.

## One analyzer, two backends

The one unforgivable failure mode is the REPL diverging from the compiled
binary. The architecture makes that structurally hard: one reader, one
analyzer, one AST — feeding both backends.

```
                  ┌──────────────────────────────────────────────┐
                  │              compile-time macros              │
                  │   (analyzer calls evaluator to run macro fns) │
                  │                      ▲                        │
                  ▼                      │                        │
UTF-8 text ──▶ Reader ──forms──▶ Analyzer ──*ast.Node──┬──▶ Tree-walk Evaluator ──▶ REPL / nREPL / scripts
 (pkg/reader)                   (pkg/analyzer)         │      (pkg/eval)
                                                       │
                                                       └──▶ Go Emitter ──.go──▶ go/format ──▶ go build ──▶ binary
                                                              (pkg/emit)

                  both consumers link the SAME pkg/lang runtime
```

Key invariants:

- **The emitter never re-analyzes** and holds no private special-form
  knowledge. Any new AST op lands in both consumers before merge.
- **Macros expand identically by construction**: both paths run macroexpansion
  through the same analyzer, which invokes macro fns via the same evaluator.
  The AOT compiler links the evaluator for compile time — compile time =
  eval time for macros.
- **One runtime package** (`pkg/lang`): emitted code and the evaluator link
  the same persistent data structures, numeric tower, vars, and `Apply`
  fast paths.
- **The dual-harness conformance suite is the gate**: every semantic test
  runs through both paths, oracle-cited against JVM Clojure. Interpreted
  result == compiled result, or the build fails. See
  [Compatibility](/cljgo/reference/compatibility/).

### The two backends

- **Tree-walk evaluator** (`pkg/eval`) — is the REPL and the macro engine.
  Pre-resolved locals, non-allocating fast paths, live re-`def` and
  `defmacro` at the prompt.
- **Go-source emitter** (`pkg/emit`) — AOT-emits plain Go from `go/packages`
  type facts — direct, non-reflective calls. The Go toolchain then produces a
  static native binary. Because the output is plain Go, pure-Go programs
  cross-compile for any OS/arch with no target toolchain.

Interop follows the same discipline: a per-path mechanism with one semantics.
The interpreter uses a generated registry plus reflection; the emitter uses
`go/packages` type facts and direct calls — with the same shaping rules
(`[v err]` vectors, nil-normalization, coercions) on both.

## Package layout

| Path | What it is |
|---|---|
| `pkg/lang` | THE runtime — persistent data structures, numeric tower, vars, seqs. Vendored from Glojure (EPL headers kept), reshaped and owned. |
| `pkg/corelib` | Go-native core builtins (ADR 0043). |
| `pkg/reader` | Text → data with position metadata. |
| `pkg/ast` | The shared AST: `Node{Op, Form, Sub}` + per-op payloads. The analyzer is the sole writer. |
| `pkg/analyzer` | Forms → AST. Pure, dependency-injected; never imports `pkg/eval`. |
| `pkg/eval` | The tree-walk evaluator — the REPL engine. |
| `pkg/emit` | AST → Go source → `go/format` → `go build`. |
| `pkg/coreaot` | Generated: cljgo's own core AOT-compiled. Linked by emitted binaries, never by the interpreter (ADR 0046). |
| `pkg/deps` | Dependency resolution + lockfile (ADR 0052). |
| `pkg/repl` | REPL driver; nREPL sits on it. |
| `cmd/cljgo` | The CLI: `repl` · `nrepl` · `run` · `build` · `new` · `test` · `publish` · `suite` · `check` · `explain` · … |
| `core/` | `core.clj` + satellite namespaces — the Clojure-in-Clojure standard library. |
| `templates/` | Real, runnable project templates `cljgo new` embeds (lib · cli · web). |
| `conformance/` | The dual-harness test suite, oracle-cited vs JVM Clojure. |

## Where core.clj lives, and why startup is fast

The standard library is written in Clojure (`core/core.clj` plus satellites).
One table drives both modes: the evaluator loads the sources at boot; a
generator AOT-compiles the same table, in the same order, into `pkg/coreaot`.
A compiled binary links the **compiled** core and no interpreter at all —
`pkg/eval` went from 155 symbols to **0** in the link set (ADR 0046,
CI-enforced). That is why a compiled hello starts in ~5 ms while the
interpreter boots in ~32 ms.

## Going deeper

The design docs on GitHub are the authoritative source:

- [`design/00-architecture.md`](https://github.com/muthuishere/cljgo/blob/main/design/00-architecture.md) — cross-component contracts + the M0–M5 roadmap
- [`design/08-build-comptime-compat.md`](https://github.com/muthuishere/cljgo/blob/main/design/08-build-comptime-compat.md) — the Zig-model build/roadmap doc
- [`docs/adr/`](https://github.com/muthuishere/cljgo/tree/main/docs/adr) — the decision log (binding until superseded)
