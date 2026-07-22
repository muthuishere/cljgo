# Spike s28 — richer, LLM-friendly error rendering

Status: CLOSED — see `VERDICT.md` for the result and the ADR 0048 recommendation.
Lifecycle: ADR 0027 spike → close with VERDICT → ADR 0048 → spec → apply.
Owner bar (as rescoped mid-spike): *"no need exactly like rust, just some
more details, that's enough."* The Rust source-snippet + `^^^^` caret block
is explicitly **out of scope**; the target is ONE richer error line plus a
cheap `help:` pointer, consistent across REPL / `cljgo run` / compiled.

## Exit criteria (written BEFORE the code, per ADR 0027)

The spike answers, with real captured before/after output:

1. **`diag.Render(d Diagnostic) string`** — a single reusable renderer that
   turns the EXISTING `diag.Diagnostic` model into the lighter detailed line:
   `<message> (expects <E>, got <F>) at <file>:<line>:<col>` followed by
   `help:` lines (did-you-mean, and `run \`cljgo explain <CODE>\``). It
   degrades gracefully: no location → no ` at …`; no expected/found → no
   `(expects …)`; no code → no explain pointer. Lives in `pkg/diag`
   (keepable). NO snippet, NO caret. (anatomy: name + location + expected/
   found + code pointer — the cheap 80%.)

2. **Name the thing + carry a location into a RUNTIME error.** The hard case
   is the arity error: the analyzer/AST has the call-site position and the
   resolved Var, but the runtime panic in `pkg/eval/fn.go` loses both,
   degrading `user/f` → `f`/`fn` and dropping the position. Prove ONE path
   end to end (REPL + `cljgo run`): an arity error that names `user/f` (from
   the resolved Var, JVM-accurate) and carries `at file:line:col` (from the
   call form's `:line/:column` meta) and `(expects 1: [x])`.

3. **P0 boundary.** The emitted `func main()` (`pkg/emit/program.go`) has NO
   `recover()`, so a compiled binary prints a raw Go panic + goroutine stack
   trace. Add a top-level `recover()` that routes the thrown value through
   the SAME renderer, so a compiled binary prints a clean one-line Clojure
   error. Prove REPL / `cljgo run` / compiled all print the SAME rendered
   line for one location-less runtime error (kills the REPL-vs-binary
   divergence and the Go-stack-trace leak).

4. **did-you-mean as a structured `Fix`**, rendered as a `help:` line, firing
   under `cljgo run` too (not only the REPL) for the unresolved-symbol case.

## What this spike deliberately does NOT do

- No source-snippet-with-line-numbers, no `^^^^` caret (owner rescope).
- No mass migration of the ~1,100 runtime raise sites — only the arity worked
  example is span/name-carried; the rest render bare-but-coded via `FromError`.
- Error `.Error()` strings are left UNCHANGED (conformance freezes them via
  `strings.Contains`); all new detail is added at the RENDER layer only.

## Files (prototype)

- `pkg/diag/render.go` + `render_test.go` — the reusable renderer (keepable).
- `pkg/diag/adapt.go` — a `Carrier` interface so a runtime error can hand
  `FromError` a fully-formed Diagnostic (span/name-carry hook).
- `pkg/eval/fn.go` + `pkg/eval/arity_diag.go` — the arity error now carries a
  Diagnostic (name from the resolved Var, location from the call form meta,
  expects from the fn params) when raised through an `OpInvoke` call site.
- `pkg/repl/driver.go` + `ergonomics.go` — one `renderError` path used by both
  REPL and run; did-you-mean becomes a `Fix`.
- `cmd/cljgo/main.go` — `cljgo run` renders through the same path.
- `pkg/emit/program.go` + `pkg/emit/rt/diag.go` — emitted `func main()`
  `recover()` → the same renderer.

## Reproduce

```
# build the fresh cljgo (rebuilds from source)
go build -o /tmp/cljgo-s28 ./cmd/cljgo

# the probe: arity, unresolved+did-you-mean, type/cast, reader error
bash spikes/s28-rust-diagnostics/probe/run-probe.sh
```

`run-probe.sh` runs each probe `.clj` under `cljgo run` AND compiled
(`cljgo build` + exec), and the arity + unresolved cases through an in-REPL
capture, dumping all three for the VERDICT.

## Gates

```
go build ./... && go vet ./... && gofmt -l pkg cmd conformance templates && go test ./...
```
