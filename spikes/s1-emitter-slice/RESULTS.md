VERDICT: VALIDATED

# S1 — Emitter thin slice

Machine: darwin/arm64, go1.26.3. Run with `go run .` from this directory
(regenerates `gen/`, builds, runs, verifies, measures; uses a fresh temp
GOCACHE so "cold" numbers are honest and the user cache is untouched).

## What was built

- `ast/` — micro `Node{Op, Sub}` with typed payloads for Const, If, Let, Do,
  VarRef, Local, Fn, Invoke, Loop, Recur, Def (the doc 03 §1 shape, minus
  `Form` since there is no reader here).
- `lang/` — micro runtime: `Fn func(args ...any) any` (+`Invoke` ⇒ IFn),
  `Apply`/`Apply0..2` fast paths, `IsTruthy`, `CheckArity`, `Var` with
  atomic root + idempotent `InternVar` registry, int64 arithmetic/comparison
  builtins, `PrintlnFn`, `Init()`.
- `emit/` — flattening emitter: every `gen(node)` writes statements and
  returns an r-value name (temp/local/literal). Zero IIFEs. Scope stack with
  suffixed names, hoisted package-level var interns (sorted, deterministic),
  guarded `Load()` per doc 04 §1, `go/format.Source` as the syntax gate.
- `main.go` — driver: 4 hand-built AST programs → emit → write generated
  module (own `go.mod`, `replace` → this spike for `lang`) → `go build` →
  run → diff against expected output → measure.

## Programs (all outputs exact-matched)

| program | proves | output |
|---|---|---|
| `fact-recursive` | def + fn* + if + self-call through the Var, `(fact 10)` | `3628800` |
| `let-nested-if` | if-as-expression inside a let binding init, do in body | `in-let` / `11` |
| `loop-recur-100k` | loop/recur ×100k constant stack (wrapped 100000! = 0, sum 1..100k = 5000050000) + **nested loops with the inner loop in non-tail position** (inside the outer recur's args) | `0` / `5000050000` / `1000` |
| `closure-capture` | fn closing over a let local, escaping its block | `106` / `107` |

## Measurements

| metric | value |
|---|---|
| `go build`, cold cache (fresh GOCACHE: stdlib + lang + program) | **~1.3 s** |
| `go build`, warm cache, other programs first build | 61–138 ms |
| `go build`, rebuild after touching main.go | **62–67 ms** |
| `go build`, no-op | ~64 ms |
| binary size | **2.41 MB** (1.61 MB with `-ldflags "-s -w"`) |
| startup+run of emitted binary (min of 10) | **~2.3 ms** for the trivial programs |
| loop-recur-100k full run | ~13 ms wall (≈300k boxed `Apply2` + var derefs — that's compute, not startup) |

Doc 00 M2 target "startup < 50 ms": beaten by >20×. Dev loop (emit → build →
run) is ~70 ms warm — REPL-adjacent, fine.

## What worked (first `go build` passed on all four programs)

- **Statement flattening is mechanically simple.** The whole emitter is
  ~330 lines including comments. The invariants that make it compose:
  (1) `gen` returns an r-value string, `""` iff the node transferred control
  (recur); (2) every discard site checks for `""`/`"nil"` before writing
  `_ = rv`; (3) compound expressions declare `var tmpN any` up front and
  branches assign it.
- **`go/format.Source` as the gate is real**: it parses, so malformed output
  dies at emit time with a Go parse error, never at `go build`. Emitting
  sloppy unindented text and letting gofmt fix it is entirely workable —
  don't spend emitter code on pretty-printing.
- **Go closures = Clojure closures for free.** The fn-inside-let capture
  needed *zero* emitter work: locals are real Go vars, the func literal
  captures by reference, escape analysis heap-allocates `base5` on its own.
  No env structs, no lifting (the thing Compiler.java burns hundreds of
  lines on).
- **Simultaneous rebinding for recur** (all new values into temps, then
  assign, then `continue label`) is exactly as doc 04 §4 describes, and the
  nested-loop-in-recur-args case worked because the recur frame is captured
  *before* generating args (args may push/pop inner loop frames).
- **Hoisted package-level `lang.InternVar` vars** (doc 00 §4.4 rationale) are
  clean: interning is idempotent/order-independent, so package init order
  never matters, and `Load()` stays purely about binding + side effects.
- **`replace`-directive module wiring** works for dev: the generated module
  requires `cljgo-spike-s1 v0.0.0` with `replace` → the spike dir. No
  publish needed to iterate.

## What fought back (all resolved, all worth encoding in pkg/emit)

1. **Go's unused-variable and unused-label errors are the #1 hazard class.**
   Three distinct manifestations: (a) a discarded invoke temp needs `_ = tmp`;
   (b) `_ = nil` is *illegal* (untyped nil) so the discard helper must skip
   the `nil` literal; (c) a loop label with no `continue` targeting it is a
   compile error, so the emitter walks the loop body (`recursDirectly`,
   stopping at fn/loop boundaries) and emits the label only when a recur
   targets it. The cheap blanket defense adopted here: emit `_ = x` right
   after every temp/local declaration — gofmt keeps it, the compiler elides
   it, and whole categories of "declared and not used" vanish (e.g. an
   if-expression whose branches both recur never assigns its temp).
2. **Dead code after `continue` must not be emitted as an assignment.**
   Handled by the `""`-r-value convention (Glojure's `generateRecur` returns
   `""` too). Forgetting a single `rv != ""` check produces unreachable
   `tmp = ...` after `continue` — which Go *accepts* silently, so it's a
   correctness trap, not a compile error. Centralize the check.
3. **Labeled continue is needed as a *mechanism*, but note:** because recur
   is always in tail position of its own loop, every `continue` this spike
   emits is textually inside its own `for` with any nested `for` already
   closed — plain `continue` would have bound correctly. Keep labels anyway
   (they're free, self-documenting, and S5's stress cases — `recur` under
   macro-expanded `when`/`and` inside other statement contexts — are where
   an unlabeled form could get captured by an emitter-introduced loop).
4. **Varargs "fast paths" aren't.** `Apply1(f, a)` still materializes a
   1-elem slice because `Fn` is variadic — Go allocates the slice at every
   variadic call. The real win requires fixed-arity fn types
   (`Fn1 func(any) any` …) on the performance ladder; today's Apply1/2 only
   save the `[]any{...}` literal + re-dispatch. Don't oversell these in
   pkg/lang docs.

## Advice for the real pkg/emit

- Keep the exact contract proven here: `gen(node) → rvalue string`, `""` for
  control transfer, temps monotonic, `var tmpN any` + branch assignment for
  every expression-position compound. It survived nesting 4 deep
  (loop→if→recur-args→loop) with zero special cases.
- Ship the `_ = x`-after-every-declaration defense from day one; revisit
  only if generated-code aesthetics start to matter.
- `recursDirectly` needs to come from the analyzer, not an emitter re-walk:
  the analyzer already knows each recur's loop-id (doc 03), so "does loop L
  get recurred to" should be an AST annotation. The emitter-side walk here
  is a spike shortcut and would double-traverse every loop body.
- The `nil` literal is a footgun in three places (discard, `var x any = nil`
  is fine, `_ = nil` is not). Consider making Const-nil return a temp-free
  sentinel the writer understands, or just keep the centralized `discard`.
- Cold build ~1.3 s is a one-time cache fill; steady-state edit-build is
  ~65 ms for one package. Budget per-namespace: `go build` parallelizes per
  package, so the ns→package mapping (doc 04 §1) is also the incremental
  compilation strategy — don't invent one.
- Binary floor is ~2.4 MB (1.6 stripped) for hello-world-class programs;
  the real pkg/lang (persistent colls, seqs) will add a few MB. Fine; note
  it so nobody is surprised.
- Multi-arity fn (`switch len(args)`), fn-level recur (`goto`), and named-fn
  self-binding were NOT exercised here — they're the same techniques
  (doc 04 §4) but S5 should cover fn-level `goto` recur explicitly since
  goto-over-declarations is the one Go restriction this spike didn't touch.
