# m2-emitter-v0 — design

## Pipeline (design/00 §2, ADR 0002)

`cljgo build` = read (pkg/reader) → analyze (pkg/analyzer, macroexpanding via
the evaluator) → **evaluate each top-level form at compile time** (so
`defmacro`/`def` in the file affect later forms — compile time = eval time,
exactly the JVM model) → collect the analyzed nodes → emit one Go file →
`go/format.Source` → write generated module → `go build`. The emitter never
re-analyzes and dispatches exhaustively on `ast.Op` (`default:` = error).

## Emission shape (ports S1 + S5, proven)

- `gen(node) → rvalue string`; `""` iff the node transferred control (recur).
  Compounds declare `var tmpN any`, branches assign; `_ = x` after every
  declaration; discard skips `""`/`nil`.
- Locals resolve by **`*ast.BindingNode` pointer** (the analyzer already did
  scoping) — no name-shadow bookkeeping; fresh suffixed Go names per binding.
- `loop*`/fn-method recur: labeled `for {}` + `continue`, label emitted only
  when a `recur` with the frame's LoopID exists in the body (full-walk match
  on LoopID; IDs are unique gensyms so no boundary logic needed).
  Simultaneous rebinding through temps. Captured carriers (an OpLocal whose
  Binding is a carrier, reached under an OpFn in the body or a later
  binding's init) get the S5 fix: immutable binding var + separate carrier +
  per-iteration copy at for-body top. Uncaptured carriers collapse to the S1
  emission. (S5 rules 1–3; capture detection is an emitter walk for now —
  the analyzer annotation stays a TODO noted in code.)
- `binding` (OpDynBind) emits flat `PushThreadBindings` … body …
  `PopThreadBindings` — no closure, keeping the no-IIFE invariant. Panic
  safety is irrelevant in v0: there is no catch, an unwinding panic ends the
  process. v1's try/catch introduces the real finally mechanism. The
  analyzer already rejects recur across `binding`.

## Calling convention (ADR 0004 / S6)

- Fn values: single method, non-variadic, arity ≤ 4 → `lang.FnFunc{N}`
  closure (params are real Go params). Multi-arity / variadic / arity > 4 →
  `lang.FnFunc` with `switch len(args)`, variadic method as `default` with
  floor check, arity panic matching the evaluator's message
  (`wrong number of args (N) passed to: name`); empty rest args bind `nil`
  (matches evalFn). Self-name binds via a pre-declared captured Go var.
- Call sites: `lang.Apply0..4(v_f.Get(), …)` — `Get()` is one atomic load
  (per-call deref, liveness on); `Apply{N}`'s type switch dispatches
  `FnFunc{N}` directly, so the S6 winning shape (fixed-arity + deref, no
  `[]any`) is realized with zero new runtime surface. > 4 args →
  `lang.Apply(f, []any{…})`.
- **Guarded arithmetic intrinsics (`pkg/emit/rt`)**: a 2-arg call of a
  core arithmetic builtin (`+ - * / < > =`) emits as `rt.Add2(v, x, y)`
  etc.; comparisons in `if`-test position use unboxed `rt.LTBool`-style
  variants. Each helper derefs the var per call and identity-compares
  against the boot-time builtin — pristine → open-coded int64 fast path,
  redefined → normal `Apply2` through the new value. Liveness (ADR 0004)
  is fully preserved; JVM Clojure's `:inline` arithmetic is the
  precedent (it doesn't even deref). Measured effect: naive emission
  168× raw Go on factorial → 35× with intrinsics. The remaining gap to
  the ~10× budget is Var.Get weight (pkg/lang), boxing, and helper
  inlining — the design/04 §5 primitive-hints rungs, post-M2 (S6's 7.8×
  already assumed raw-op arithmetic); perf_test.go records the numbers
  and guards regression at 60×.

## Hoisting & interning (design/00 §4.4, ADR 0006)

Package-level, sorted for deterministic output:
- Vars: `var v_<munged ns/name> = lang.InternVarName(lang.NewSymbol("ns"),
  lang.NewSymbol("name"))`, `.SetDynamic()` chained when the compile-time
  var's meta has `:dynamic` (so emitted `binding`/`set!` work). Interning is
  idempotent/order-free, so package init before `eval.New()` is safe.
- Keywords `kw_…` via `lang.InternKeyword`, quoted symbols `sym_…` via
  `lang.NewSymbol`. Quoted collections construct inline (pure constructors).
Go-name collisions after munging get a numeric suffix (munging is not
injective; the dedup keeps identifiers unique per file).

## Generated module & bootstrap

One input file → one `main` package (`main.go` + `go.mod`): hoisted interns,
`var loaded bool` + `Load()` with the forms in source order, `main()` =
`rt.Boot()` (constructs the evaluator: builtins + embedded core.clj — the
pragmatic v0 macro/core story per design/04; ~5 ms — and snapshots the
pristine builtins for the guarded intrinsics) → `Load()` → invoke `-main`
var if the file defined one (args as strings). `go.mod`: `go 1.26`, requires
`github.com/muthuishere/cljgo` with a `replace` to the compiler source tree
(resolved via `-runtime` flag / `$CLJGO_SRC` / walking up from cwd &
executable). Publishing a real module version is deferred with ADR 0013's
change.

## Conformance dual harness (ADR 0007)

Canonical output of a run = everything printed during load + `pr-str` of the
last top-level value + `\n`. Eval side: capture `eval.Out` + append pr-str.
Compiled side: emit with `PrintLastValue` (main prints the last form's
value); binary stdout must be **byte-identical**. Directives:
`;; harness: eval — reason` (compiled skip; all `expect-error` files carry
it: v0 has no compiled error-output contract), `;; oracle: skip — reason`.
`ORACLE=1 go test ./conformance` rewrites each value file so its last form is
wrapped in `(clojure.core/prn …)` (via reader end-positions), runs
`clojure -M`, and compares the last stdout line to the frozen expectation;
error files assert non-zero exit.
