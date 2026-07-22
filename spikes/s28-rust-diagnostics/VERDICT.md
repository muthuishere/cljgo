# VERDICT — spike s28: richer, LLM-friendly error rendering

Closed 2026-07-22. Prototype landed behind the existing error paths; gates
green. This spike answers the rescoped brief (owner: *"no need exactly like
rust, just some more details, that's enough"*) — a single richer error line,
consistent across REPL / `cljgo run` / compiled, NOT a Rust snippet+caret
block.

## Exit criteria — proven vs not

| # | criterion | verdict |
|---|---|---|
| 1 | `diag.Render(d) string` — one reusable renderer, graceful degradation | **PROVEN** — `pkg/diag/render.go`, golden test `render_test.go`. |
| 2 | Name + location + expected/found carried into a RUNTIME arity error (REPL + run) | **PROVEN** — `user/f` from the resolved Var, `at file:line:col` from the call form meta, `(expects 1: [x])` from the params. |
| 3 | P0 boundary — emitted `func main()` recover() → same renderer; REPL/run/compiled parity | **PROVEN for a location-less error** (divide-by-zero): all three print the identical line, no Go stack trace. Arity in a *compiled* binary prints clean but WITHOUT name/location (its panic is a separate bare `fmt.Errorf` in the emitter — see gaps). |
| 4 | did-you-mean as a structured `Fix`, firing under `cljgo run` too | **PROVEN** — moved into `Driver.RenderError`; renders as a `help:` line in REPL and run. |

## What worked

- **The data model needed nothing new.** `diag.Diagnostic` already carried
  `ErrorCode / Location / Expected / Found / Fixes / Related / ExplainURL`.
  The whole win is a renderer + populating those fields — as the audit
  predicted.
- **`Carrier` interface is the clean span/name-carry hook.** A runtime error
  that computed its own `Diagnostic` at the raise site hands it to
  `diag.FromError` verbatim (winning over prose-classification). `pkg/eval`'s
  `*arityError` implements it; `pkg/diag` stays dependency-free of the runtime
  packages (the interface inverts the edge).
- **`.Error()` stays byte-stable → conformance stays green.** All new detail
  lives on the diagnostic and surfaces only through `diag.Render`. Conformance
  matches `err.Error()` via `strings.Contains`, so `passed to: f` is unchanged
  in the raw string; only the rendered line shows `user/f`.
- **The emit recover() closes the worst divergence** with ~6 lines: a
  compiled binary now prints `error: <rendered>` and `os.Exit(1)` instead of a
  Go panic + goroutine stack trace.

## Actual captures (before → after)

Reproduce: `bash spikes/s28-rust-diagnostics/probe/run-probe.sh`.

### Arity — `(def f (fn* f [x] x))` then `(f 1 2 3)` (criterion 2)

BEFORE (documented in the audit — today's bare, unnamed line):
```
error: wrong number of args (3) passed to: fn
```

AFTER — `cljgo run probe/arity.clj` (paths trimmed for readability):
```
error: wrong number of args (3) passed to: user/f (expects 1: [x]) at probe/arity.clj:2:1
help: run `cljgo explain A2004`
```

AFTER — REPL (`cljgo repl`, `(f 1 2 3)` on input line 1):
```
error: wrong number of args (3) passed to: user/f (expects 1: [x]) at REPL:1:1
help: run `cljgo explain A2004`
```

Named `user/f` (from the resolved Var), located, expected-arity shown — and
byte-identical between REPL and run. (JVM 1.12.5 oracle for the same input:
`Wrong number of args (1) passed to: user/f--1`.)

### Unresolved symbol + did-you-mean (criterion 4)

`cljgo run probe/unresolved.clj` — `(pritnln "hi")`:
```
error: unable to resolve symbol: pritnln in this context at probe/unresolved.clj:1:2
help: did you mean println?
help: run `cljgo explain A2001`
```
did-you-mean now fires under `run`, not only the REPL — as a `Fix`
(`Replacement: "println"`), rendered as a `help:` line.

### Type/cast runtime error — `(inc "not-a-number")`
```
error: cannot convert string to Ops
help: run `cljgo explain G5000`
```
Bare-but-coded (no raise-site code yet → classifies to G5000). No location
(the corelib panic is unpositioned) — the honest current state for the
un-migrated runtime tail.

### Reader error — unterminated list (positioned, with a `note`)

`cljgo run probe/reader.clj`:
```
error: EOF while reading list, expected ")" to close it at probe/reader.clj:3:1
note: form starts here at probe/reader.clj:1:1
help: run `cljgo explain R1001`
```
Reader errors were already positioned; the renderer now surfaces the locus
AND the `Related` "form starts here" note (previously `--json`-only).

### P0 boundary — compiled binary parity (criterion 3)

`cljgo build` evaluates top-level forms, so a *top-level* runtime error fails
at build; to exercise the compiled binary's runtime path the error lives in
`-main`: `(defn -main [& args] (throw (ex-info "boom at runtime" {:code 42})))`.

BEFORE — compiled binary with NO recover() (built with the emitter reverted),
`exit 2`:
```
panic: boom at runtime

goroutine 1 [running]:
main.Load.func1({0x1013629a0?, 0x795669ab9090?, 0x1012f0400?})
	cljgo.gen/main/main.go:48 +0x130
github.com/muthuishere/cljgo/pkg/lang.FnFunc.Invoke(...)
	.../pkg/lang/ifn.go:22 +0x34
github.com/muthuishere/cljgo/pkg/lang.Apply(...)
	.../pkg/lang/apply.go:32 +0x94
main.main()
	cljgo.gen/main/main.go:62 +0x160
```

AFTER — compiled binary WITH the emit recover(), `exit 1`:
```
error: boom at runtime
help: run `cljgo explain G5000`
```
And the interpreter (`cljgo run` of the same throw at top level) prints the
identical `error: boom at runtime` line — same renderer, REPL == run ==
compiled by construction. The Go panic + goroutine stack trace is gone.

## What I did NOT prove (honest gaps)

- **Compiled arity error is clean but not named/located.** The emitter emits
  its own bare `panic(fmt.Errorf("wrong number of args..."))`
  (`pkg/emit/emit.go:1006,1010`), separate from the interpreter's
  `*arityError`. It now renders via the recover() boundary (no stack trace)
  but as an unlocated `G5000` without `user/f` or `(expects …)`. True
  REPL==compiled parity for the *arity* case needs the emitter to emit a
  Carrier-style positioned error too — deferred to ADR 0048.
- **Only the arity site is span/name-carried.** The other ~1,100 runtime
  raise sites (corelib/lang/bri) still render bare-but-classified (mostly
  `G5000`, no position). The `invokeAt` enrichment only catches `*arityError`.
- **Per-call defer.** `invokeAt` wraps every `OpInvoke` in a deferred recover.
  The tree-walk evaluator is not perf-gated (the budgets measure *compiled*
  binaries), so this is invisible to gates — but it is not free and MUST NOT
  ship as-is. ADR 0048 should thread the call-site position onto the analyzer
  invoke node (or use `lang.EvalError`'s existing unused `StackFrame` slice)
  so the hot path pays nothing.
- **No nREPL wiring, no `--json` change, no snippet/caret** (the last by
  owner rescope; notes below).

## Surprise worth flagging for ADR 0048

`cljgo build` **evaluates top-level forms at build time** (a top-level
`(println "x")` prints during `build`, and a top-level `(/ 1 0)` / `(throw …)`
*fails the build*). So a top-level runtime error never reaches the emitted
binary's `main()` — it dies at compile. The recover() boundary only guards
errors raised from code that runs at binary-runtime (e.g. inside `-main`).
This narrows what "compiled runtime error" even means and is why the parity
proof uses a `-main` throw. ADR 0048 should decide whether that build-time
evaluation is intended (it changes the blast radius of the boundary).

## Recommendation — the shape of ADR 0048

ADR 0048 should ratify the *lighter* target (this spike), superseding the
Rust-block language in the audit/CLAUDE doctrine (already updated in this PR):

1. **Renderer + boundary land first, unconditionally.** `diag.Render` + the
   emit recover() + `Driver.RenderError` make *every* existing error better
   the moment they ship (graceful degradation), with zero migration. Keep
   them from this spike.
2. **Span/name-carry via a cheap mechanism, NOT a per-call defer.** Thread a
   `*diag.Location` (or the analyzer's invoke-node position) so the runtime
   arity error is born located; drop `invokeAt`'s defer. Extend the same
   Carrier pattern to the emitter's arity panic so compiled == interpreted.
3. **Code assignment: attach at raise site, top-N first.** Decide whether
   runtime arity keeps `A2004` (reused here to match the approved format) or
   gets its own runtime band — the audit flags reusing an analyzer code for a
   runtime error as a smell. Register the top ~10 runtime errors (arity,
   type/cast, class-cast, index-out-of-bounds, divide-by-zero); leave the long
   tail on `classify()`/`G5000` until touched.
4. **Rollout order:** renderer + boundary → arity worked example (name +
   location, both interpreter & emitter) → did-you-mean everywhere (done here)
   → nREPL Envelope adapter → top-N runtime codes, one subsystem at a time.
   Snippet+caret stays an OPTIONAL future rung, gated on demand — not the bar.

## Files (prototype — keepable vs spike-only)

- **Keepable:** `pkg/diag/render.go` (+ test), the `Carrier` interface in
  `pkg/diag/adapt.go`, `pkg/emit/rt/diag.go`, the emit recover() in
  `pkg/emit/program.go`, `Driver.RenderError` + did-you-mean-as-Fix.
- **Spike-only / revisit in ADR 0048:** `pkg/eval/invokeAt` per-call defer
  (replace with threaded position), reuse of `A2004` for the runtime arity.
