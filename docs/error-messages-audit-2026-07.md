# Error-messages audit — 2026-07

**Read-only audit + doctrine. No renderer, no runtime rewiring in this
change** (that is ADR 0048 / spike s28, the next step). This document maps
every error-raising site in cljgo against the Rust diagnostic anatomy the
owner set as the bar — *"very detailed error messages like rust and very
llm friendly"* — classifies every gap, measures the three consistency
contexts (REPL / `cljgo run` / compiled binary), and lays out a prioritized
plan. The binding doctrine distilled from it lives in `CLAUDE.md` →
*"How to write error messages"*.

## The bar — Rust's diagnostic anatomy

Every Rust diagnostic carries seven elements. We measure each cljgo error
site against them:

1. **Stable error code** (`E0308`) — a versioned handle for tooling + docs.
2. **One-line primary message** — what went wrong, in one sentence.
3. **Source snippet with line numbers** — the offending lines, shown.
4. **Caret/underline on the exact span** (`^^^^`) with an **inline label**.
5. **`help:` line(s) with a concrete suggested fix** — the *replacement
   text*, not prose.
6. **`note:` line(s)** — context (why, or a secondary position).
7. **A pointer to a fuller explanation** (`cljgo explain <code>`).

"LLM-friendly" adds an eighth requirement, already modelled: a clean
machine-readable form — the `diag.Envelope` (`--json`) — carrying *all* of
the above, and **consistency**: an agent (or a human) must read the same
error in the REPL, under `cljgo run`, and from a compiled binary.

cljgo already has a strong *data model* for this (`pkg/diag.Diagnostic`:
`ErrorCode`, `Severity`, `Message`, `Location` with end-span, `Expected`,
`Found`, `Fixes[]` with byte-accurate `Replacement`, `Related[]`,
`ExplainURL`). The finding of this audit is that the model is **severely
under-rendered and under-populated**: the human render is a single bare
line, the rich fields are populated only for reader/analyzer errors, and
the entire runtime error surface bypasses the model completely.

## Method

1. **Enumerate** — `grep` for `panic(`, `fmt.Errorf`, `errors.New`,
   `NewIllegalArgumentError` / typed `lang` error constructors, `a.errf` /
   `a.errPos`, `diag.` across `pkg/reader`, `pkg/analyzer`, `pkg/eval`,
   `pkg/emit`, `pkg/corelib`, `pkg/lang`, `pkg/repl`, `pkg/nrepl`,
   `pkg/bri`, `cmd/cljgo` (test files excluded from every count).
2. **Classify each site** — does it carry a code? a location/span? a clear
   message? expected-vs-found? a fix/suggestion? an explain pointer? Does it
   render richly or bare?
3. **Trace the render** — follow each raised error to the byte that reaches
   the user, in each of the three contexts.
4. **Score against the anatomy** — which of the seven elements fire, where.

## Headline numbers — error-raising sites per subsystem

Counts are non-test occurrences of the raising construct. "Positioned?"
means the raised value carries a source location the moment it is thrown.

| subsystem | `panic(` | `fmt.Errorf`/`errors.New` | how errors are raised | positioned at throw? | code? | rich render? |
|---|---:|---:|---|---|---|---|
| `pkg/reader` | 1 | 24 | `reader.Error{Pos, Start, Err}` (exported fields) | **yes** (`Pos`, and `Start` for the opening delimiter) | via `diag.FromError` → R1xxx | one-line only |
| `pkg/analyzer` | 0 | 1 | `a.errf` / `a.errPos` (**62 call sites**) → `lang.CompilerError{file,line,col}` | **yes** (CompilerError) | via `diag.FromError` → A2xxx | one-line only |
| `pkg/eval` | 23 | 27 | typed `*arityError` + bare `fmt.Errorf` panics | **no** | no | bare |
| `pkg/emit` | 14 | 24 | `*emitErr` (compile-time) + *emits* runtime `panic(...)` into the binary | compile-time: partial (provenance comment); runtime: **no** | no | bare / Go panic |
| `pkg/corelib` | 409 | 378 | bare `fmt.Errorf` panics (≈133 "…passed to: NAME" arity sites) | **no** | no | bare |
| `pkg/lang` | 224 | 93 | typed error structs (`IllegalArgument/State/Arithmetic/…`); only `CompilerError` carries a position | **no** (except CompilerError) | no | bare |
| `pkg/bri` | ~20 | — | bare `fmt.Errorf` panics (http helpers) | **no** | no | bare |
| `pkg/repl` | 0 | 12 | renders others' errors; owns did-you-mean | n/a | n/a | `error: %v` + did-you-mean |
| `pkg/nrepl` | 0 | (few) | `err.Error()` + Go type name in `ex` | **no** | no | text over the wire |
| `cmd/cljgo` | few | 4 | bare `error:` prints; `check`/`explain` use `diag` | n/a | only `check`/`explain` | only `check` (one-line) |

**Read of the table:** the whole *compile-time* front (reader + analyzer)
is positioned and code-mappable — ~86 raising sites feed a model that
already knows where they are. The whole *runtime* surface (eval + corelib +
lang + bri ≈ **1,100+ raising sites**) is bare `fmt.Errorf`/`panic` with no
code, no position, and no path into `diag` at all. That asymmetry is the
audit's central finding.

## The render, traced — where each anatomy element actually fires

There is no snippet renderer anywhere. The richest human render in the tree
is `cmd/cljgo/diagnostics.go:humanLine` — a positioned one-liner
`file:line:col: message [CODE]` — and it is reached only by `cljgo check`.
Everything else is `fmt.Fprintf(errOut, "error: %v\n", err)`
(`pkg/repl/driver.go:319`) or `fmt.Fprintln(os.Stderr, "error:", err)`
(`cmd/cljgo/main.go:107,116,122,181,199,203`).

| anatomy element | reader/analyzer | eval/corelib/lang (runtime) | where it renders |
|---|---|---|---|
| 1. stable code | modelled + assigned (R1xxx/A2xxx) | **absent** | shown only by `cljgo check` (`[CODE]`) and `--json` |
| 2. one-line message | yes | yes | everywhere (bare) |
| 3. source snippet + line numbers | **never rendered** (position is known) | **never** (position unknown) | nowhere |
| 4. caret/underline + label | **never rendered** (`Location` has `EndLine/EndColumn`) | **never** | nowhere |
| 5. `help:` concrete fix | `Fix{Replacement,ByteRange}` modelled, **never populated, never rendered** | absent | nowhere; did-you-mean is prose-only + REPL-only |
| 6. `note:` context | `Related` populated by reader ("form starts here"), **never rendered in human text** | absent | `--json` only |
| 7. explain pointer | `ExplainURL` + `cljgo explain <code>` | absent | `--json` + the `explain` verb; never in the human error |

**did-you-mean** (`pkg/repl/ergonomics.go`, Levenshtein ≤2, nearest 3) is
the one suggestion affordance that exists — and it fires **only** in the
interactive REPL driver (`reportEvalError`), is prose (`did you mean …?`)
rather than an applicable `Fix`, and covers only unresolved-symbol errors.

## The three consistency contexts (REPL / run / compiled) — measured

The owner's requirement is that errors "read the same" across all three.
Today they read three (really five) different ways:

| context | entry point | render | code | snippet/caret | did-you-mean | fix/note |
|---|---|---|---|---|---|---|
| **REPL** (`cljgo repl`) | `driver.reportEvalError` | `error: %v` + `did you mean …?` | no | no | **yes** (only here) | no |
| **run** (`cljgo run`) | `main.runFile` → `EvalReader` → `error: %v` | one bare line | no | no | **no** | no |
| **compiled binary** | emitted `func main()` — **no top-level recover** (`pkg/emit/program.go:256-267`) | **raw Go panic + goroutine stack trace** | no | no | no | no |
| **check** (`cljgo check`) | `diag` pipeline → `humanLine` | `file:line:col: msg [CODE]` (+ `--json` Envelope) | **yes** | no | no | no |
| **nREPL** (editors) | `evalErrorReply` | `err` = `err.Error()`, `ex` = Go type name | no | no | no | no |

Two divergences are severe:

- **Compiled binaries have no error boundary at all.** The emitted
  `func main()` calls `Load()` then `lang.Apply(main, args)` with no
  `recover()`. A runtime arity/type/cast error in a compiled binary prints
  a Go runtime panic *with a Go goroutine stack trace* — a completely
  different artifact from the REPL/run `error:` line. This is the single
  worst REPL-vs-binary divergence in the error surface and, per the
  conformance doctrine ("REPL-vs-binary divergence is THE unforgivable
  failure mode"), a release-grade concern for M2+.
- **did-you-mean is REPL-only.** The exact same unresolved-symbol error
  under `cljgo run` or in a binary gets no suggestion.

## The arity error — the canonical gap, in detail

`(defn f [x] x)` then `(f 1 2 3)`:

- **JVM Clojure** → `Wrong number of args (3) passed to: user/f`
- **cljgo** → `wrong number of args (3) passed to: fn`

Root cause: `pkg/eval/fn.go`. `evalFn.Invoke` throws
`&arityError{actual, name: f.name()}` (line 47), and `evalFn.name()`
(lines 111-116) returns the fn*'s *internal self-name* if it has one, else
the literal string `"fn"`. A `defn` binds an **anonymous** `fn*` into a Var;
the Var's qualified name (`user/f`) is never threaded onto the closure, so
the arity error cannot name the function. The typed `*arityError` is
otherwise well-formed (macroexpand1 uses it to hide `&form`/`&env`), it just
**loses the name** and carries **no call-site position** (JVM has no
position here either, but cljgo's analyzer *does* have the call-site span at
the invoke node — it is discarded before the runtime throw).

Note the asymmetry: **corelib builtins name themselves correctly** — 133
sites like `passed to: range`, `passed to: assoc`. It is specifically the
*user-defined fn* path that degrades to `fn`. Fixing the name is small
(carry the Var name onto `evalFn` at `def`-time); carrying the *span* is the
hard, general problem below.

## Prioritized gap list (by severity × anatomy element missing)

**P0 — kills consistency / kills LLM-consumability**

1. **Compiled binaries have no error boundary** — runtime errors surface as
   raw Go panics + stack traces, not cljgo errors. (anatomy: all; context:
   compiled) → an emitted top-level `recover()` that renders through the
   same path as REPL/run.
2. **Runtime errors carry no code and no position** — the ~1,100-site
   eval/corelib/lang surface bypasses `diag` entirely. No agent can
   `explain` or apply-fix a runtime error. (anatomy 1,3,4,7)
3. **No snippet + caret renderer exists** — even reader/analyzer errors,
   which *have* `Location` with an end-span, never show the source line or a
   `^^^^`. This is the headline visible gap vs Rust. (anatomy 3,4)

**P1 — named-thing + suggestions + fixes**

4. **Arity error loses the fn name** (`passed to: fn` → `passed to: user/f`).
   (anatomy 2) — small fix, high visibility.
5. **`Fixes[]` is modelled but never populated or rendered.** No `help:`
   line with replacement text anywhere. (anatomy 5)
6. **did-you-mean is REPL-only and prose-only** — must fire in run +
   compiled too, and should be emitted as an applicable `Fix`, not just a
   `did you mean …?` string. (anatomy 5; context)
7. **`Related[]` never renders in human text** — reader already sets
   "form starts here"; it shows only in `--json`. (anatomy 6)

**P2 — coverage + polish**

8. **nREPL sends bare text, not a structured diagnostic** — editors get
   `err.Error()` + a Go type name; they should get the `Envelope`. (anatomy
   all; context: editors)
9. **Explain pointer never shown in the human error** — `ExplainURL` exists
   but the human render never says `for more information, run \`cljgo
   explain A2001\``. (anatomy 7)
10. **Emitter (E3xxx) and interop (I4xxx) bands are registered but unused** —
    no emitter/interop error is classified yet; all fall to `G5000`.
11. **`classify()` is a substring matcher over English prose** — the code
    assignment in `diag/adapt.go` reverse-engineers a code from the message
    string. It is fragile (a reworded message silently drops to `G5000`) and
    should be replaced by codes attached *at the raise site* over time.

## Implementation plan — what the overhaul needs (ADR 0048 / spike s28)

This audit does **not** implement any of the following; it scopes them.

### A. A Rust-style renderer (`diag.Render(d, source) string`)

A single function from a `Diagnostic` + the source bytes to the multi-line
block. All the inputs already exist on the model:

- header: `error[<ErrorCode>]: <Message>`
- locus: `  --> <file>:<line>:<column>`
- snippet: the source line(s) for `Location.Line`, gutter-numbered, sliced
  from the source bytes the compiler read.
- caret: `^` run under `[Column, EndColumn)` (the model already carries
  `EndLine`/`EndColumn`), plus `Expected`/`Found` as the inline label.
- `help:` per `Fix` (title + the `Replacement`), `note:` per `Related`.
- footer: `for more information, run \`cljgo explain <ErrorCode>\``.

This is pure and testable with golden files, and it is the **one render**
every context calls — REPL, `run`, compiled `main`, and nREPL (as the
`err` string) — which is how consistency is achieved by construction. It
must degrade gracefully: no `Location` → header + message only (today's
behavior), so nothing regresses.

### B. Threading source spans into runtime errors (the hard part)

The analyzer has every call-site span; the runtime throw discards it. Two
complementary mechanisms:

1. **Carry the code + span on the thrown value.** Introduce a positioned
   runtime error type (or extend the existing `lang.EvalError`, which
   *already* has a `StackFrame{Filename,Line,Column}` slice and an
   `AddStack` method — currently a `// TODO: Revisit` and unused by the hot
   path). Raise sites that know their position (analyzer-emitted invoke
   nodes, corelib builtins that know their fn name) attach it; the tree-walk
   evaluator wraps a bare panic at the nearest node that has a `Form`
   position, so even un-annotated `fmt.Errorf` panics gain a location as
   they unwind. The `pickMethod`-fails path in `evalFn.Invoke` is the
   worked example for the spike: the analyzer knows the call-site `Location`,
   so the `arityError` should carry it.
2. **A code catalogue for the runtime bands.** Register the common runtime
   errors — arity (a new band or a reserved `G`-range), type/cast, class-
   cast, index-out-of-bounds, arithmetic (divide-by-zero) — so the ~1,100
   bare panics migrate to coded, positioned diagnostics *incrementally*
   (start with the top ~10 by frequency; `classify()` bridges the rest until
   then). Attach the code **at the raise site**, not by prose-matching.

### C. Suggestions everywhere, as applicable fixes

Move did-you-mean out of `pkg/repl` into a shared helper the renderer calls,
so REPL, run, and compiled binaries all get it. Emit it as a `Fix`
(`Title: "did you mean X?"`, `Replacement: "X"`, `ByteRange` = the symbol's
span) so agents can apply it, not just read it.

### D. The emitted error boundary (fixes P0 #1)

`pkg/emit/program.go` `func main()` gains a `defer recover()` that routes
the thrown value through `rt.Recover` → the shared `diag.Render`, so a
compiled binary prints exactly what the REPL prints. This is the mechanical
change that closes the worst consistency gap; the spike must produce a
before/after for one runtime error (arity) proving REPL == run == binary
byte-for-byte on the rendered block.

### Lifecycle (per CLAUDE.md "ADR → propose → apply")

- **ADR 0048** (reserved) — *the error-message overhaul contract*: ratifies
  the Rust-style human render as the default (superseding ADR 0015's "human
  output remains byte-for-byte unchanged" for the error path specifically),
  the one-renderer/all-contexts rule, the positioned-runtime-error model,
  and the append-only runtime-code bands.
- **spike s28** (reserved) — prototype `diag.Render` + prove span-carrying
  end-to-end for **one** runtime error (the arity error): produce the
  before/after block, and demonstrate REPL/run/compiled parity. Close it
  with a VERDICT.md per ADR 0027 before the mass migration.
- Then **`/opsx:propose`** the spec deltas and migrate incrementally: top-N
  runtime codes first, renderer wired into all four contexts, nREPL adapter,
  emitter/interop bands populated as those errors are touched.

Do **not** mass-migrate the 1,100 raise sites in one change. The renderer +
boundary + the arity worked-example land first (they make *every* existing
error better the moment they ship, because the renderer degrades
gracefully); codes accrue at raise sites thereafter, one subsystem at a time.

## Before / after — the arity error (the target shape)

Today (`cljgo run`, and — with a Go stack trace — in a compiled binary):

```
error: wrong number of args (3) passed to: fn
```

Target (ADR 0048, identical in REPL / run / compiled binary / nREPL `err`):

```
error[A2004]: wrong number of args (3) passed to: user/f
  --> demo.clj:4:1
   |
 4 | (f 1 2 3)
   | ^^^^^^^^^ user/f is defined with 1 arg [x], got 3
   |
help: user/f takes 1 argument — call it as (f 1)
note: user/f is defined at demo.clj:1:1
for more information, run `cljgo explain A2004`
```

Every field in that block already exists on `diag.Diagnostic`
(`ErrorCode`, `Message`, `Location` + end-span, `Expected`="1 arg",
`Found`="3", a `Fix`, a `Related`, `ExplainURL`). The overhaul is renderer +
population + span-carrying, not new data model.

## Appendix — key source anchors

- render sites: `pkg/repl/driver.go:319`, `cmd/cljgo/main.go:107,116,122`,
  `cmd/cljgo/diagnostics.go:153` (`humanLine`, the richest today).
- data model: `pkg/diag/diag.go` (`Diagnostic`, `Fix`, `Related`,
  `Envelope`), `pkg/diag/registry.go` (banded codes + lock),
  `pkg/diag/adapt.go` (`FromError`, `classify`).
- did-you-mean: `pkg/repl/ergonomics.go` (`reportEvalError`,
  `nearbySymbols`, `editDistance`).
- arity gap: `pkg/eval/fn.go:23-48` (`arityError`, `Invoke`),
  `fn.go:111-116` (`name()` → `"fn"`).
- positioned compile errors: `pkg/analyzer/analyzer.go:1180-1190`
  (`errf`/`errPos` → `lang.NewCompilerError`); `pkg/lang/error.go:36-41,
  168-179` (`CompilerError`); reader position via `reader.Error`
  (`pkg/diag/adapt.go:34-49`).
- unused positioned-runtime scaffolding: `pkg/lang/error.go:48-223`
  (`EvalError`/`StackFrame`/`AddStack`, `// TODO: Revisit`).
- compiled-binary boundary gap: `pkg/emit/program.go:256-267` (emitted
  `func main()` with no `recover()`); emitted runtime arity panics
  `pkg/emit/emit.go:1006,1010`.
- nREPL bare reply: `pkg/nrepl/server.go:412-416` (`evalErrorReply`).
</content>
</invoke>
