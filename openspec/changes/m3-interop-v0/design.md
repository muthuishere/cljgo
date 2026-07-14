# Design — m3-interop-v0

Owning contracts: design/05 §1–§2 (interop surface + shaping), design/04
(emitter), ADR 0010, ADR 0005. Spike: spikes/s2-gopackages-interop (AOT path,
verified). Consumers touched: pkg/ast, pkg/analyzer, pkg/eval, pkg/emit.

## Resolution flow (single analyzer, both modes)

The emitter reuses the evaluator's analyzer (pkg/emit/compile.go: `ev.Analyzer()`)
and evaluates every top-level form at compile time (ADR 0002). Therefore the
require-go alias table and `ResolveHost` live in **pkg/eval** and serve both
paths — the emitter only *consumes* the resolved `OpHostRef`/`OpHostCall` nodes.

1. `(require-go '[strconv :as sc])` evaluates (REPL, or compile-time top-level)
   → registers `sc → "strconv"` in the current namespace's host-alias table.
2. Analyzer meets `sc/Atoi`: not a local; `ResolveVar` fails (no Clojure ns
   `sc`); `ResolveHost("sc/Atoi")` → `("strconv","Atoi",true)` → `OpHostRef`,
   or in call position `OpHostCall`.
3. `sc/Atoi!` in call position: full name misses, `!`-stripped base
   `sc/Atoi` hits → `OpHostCall{Throw:true}`. Go exports can never end in `!`,
   so the strip is unambiguous (ADR 0010).

**Precedence:** `ResolveHost` returns `ok=false` whenever the namespace
resolves as a Clojure namespace/alias — Clojure is first-class, a host alias
never shadows it. `ResolveVar` is always tried first.

## Shaping table (frozen — both paths reproduce exactly)

Detected by result *type* (static go/types in AOT; `reflect.Type` in eval),
exactly as spike S2:

| Go results        | plain call        | `!` call                    |
|-------------------|-------------------|-----------------------------|
| `T`               | `T`               | same                        |
| `(T, error)`      | `[v err]`         | `v`, throw if `err != nil`  |
| `error`           | `err` or `nil`    | `nil`, throw if non-nil     |
| `(T, bool)`       | `[v ok]`          | `v`, throw if `!ok`         |
| `(A, B, error)`   | `[a b err]`       | `[a b]`, throw on err       |
| `(A, B)`          | `[a b]`           | same                        |

- **nil normalization:** a returned ptr/iface/map/slice/chan/func that IsNil
  → Clojure `nil` (so `err` slot is truthy-testable, `nil?`/`if` behave).
- **number widening:** Go `int*`/`uint*` → `int64`, `float32/64` → `float64` —
  both paths, so the printer renders `42`, not a host-typed box, in each mode.
- **throw:** eval → `panic(goErr)` recovered at the IFn boundary; AOT →
  `panic(rt.GoError(err))`. v0 wraps the message; `ex-go-error` retrieval and
  `errors.Is` composition are M3.1.

## AOT specifics (port of S2)

`facts.Load` (go/packages `NeedName|NeedTypes`) → `*types.Func` signature →
result count + TrailingError/TrailingBool flags. Emit `import "<pkg>"`, a direct
call binding `t0, t1 := strconv.Atoi(...)`, then the shaping into an `any`
(`[]any{t0, rt.NormErr(t1)}` etc). gcexportdata disk cache keeps warm loads
~1ms/pkg (S2). Generated `go.mod`: stdlib import needs no `require`.

## Divergence guard

The dual-harness conformance suite (`compiled_test.go` already compiles+runs
each `.clj` byte-identical to eval) is the release blocker. The interpreted
path's *exact* printed form for `[v err]` on the error branch, and the int64/
float64 widening, are the two most likely divergence points — both are frozen
in `conformance/tests/interop-*.clj` and asserted through both harnesses.
