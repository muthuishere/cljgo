VERDICT: VALIDATED — flattening (labeled for + temp-var rebinding) survives every recur/loop edge case, BUT one real divergence was found and fixed: S1's naive emission breaks Clojure semantics when a closure captures a loop local (Go captures variables by reference, Clojure captures the value at that iteration). The fix (per-iteration copies + binding-var/carrier split) is load-bearing and must ship in pkg/emit, driven by an analyzer "captured" annotation.

# S5 — recur/loop emission edge cases

Machine: darwin/arm64, go1.26.3, Clojure CLI 1.12.5.1645 (the oracle).
Run with `go run .` from this directory: every case is emitted → `go build` →
run, then the SAME program (using the real `when`/`and`/`or` macros where the
AST hand-builds their expansion) is run through real Clojure via
`clojure -M <file.clj>` (script mode — `-e` would echo top-level values), and
the two outputs are diffed byte-for-byte, plus checked against a hand-computed
expectation so "both wrong" can't slip through.

Starting point: S1's emitter copied verbatim (module renamed `cljgo-spike-s5`),
then extended. S1's code is untouched.

## Per-case outcomes

| case | what it stresses | clojure | emitted (fixed) | outcome |
|---|---|---|---|---|
| 1 closure over loop local (body) | `(loop [i 0 fs []] … (conj fs (fn [] i)) …)`, call the 3 closures after the loop | `0 1 2` | `0 1 2` | MATCH (after fix; naive = DIVERGES, see below) |
| 1b closure in loop-binding INIT + in recur args | `(loop [i 0 f (fn [] i)] …)` returning `(f)`; same with `(recur … (fn [] i))` | `0` and `2` | `0` and `2` | MATCH (needs binding-var/carrier split) |
| 2 shadowing | loop `x` shadows outer let `x`; let `x` inside loop body shadows loop `x`; recur rebinds the LOOP `x` | `3`, `5` | `3`, `5` | MATCH |
| 3 macro-expansion shapes | recur in tail of `when` (`if`+`do`), `and` (`let`+`if`, recur in then), `or` (`let`+`if`, recur in else) — hand-built expansions vs real macros | `45`, `45`, `5` | same | MATCH |
| 4 simultaneous rebinding | `(loop [n 0 a 1 bb 2] … (recur (+ n 1) bb a) …)` — 5 swaps | `2 1` | `2 1` | MATCH (temps; sequential assignment would print `2 2`) |
| 5 fn-level recur (no loop) | `fn*` tail recur: factorial, 100k-iteration sum (constant stack), and recur-inside-loop-inside-fn targeting the LOOP | `3628800`, `5000050000`, `55` | same | MATCH |
| 5b recur across try | `(loop [i 0] (try (recur …) (catch …)))` | **compile error**: `Syntax error (UnsupportedOperationException) compiling recur … Cannot recur across try` | n/a — never reaches the emitter | Clojure rejects at compile time; **the ANALYZER owns this rejection** |
| 5c closure over self-recurring fn's PARAM | params are recur carriers too | `0 1 2` | `0 1 2` | MATCH (same per-iteration-copy machinery as loops) |
| 6 loop as expression | two loops inside one call's args, loop as non-final call arg | `12`, `40` | same | MATCH (monotonic temp counter ⇒ no naming collisions by construction) |

## The one real divergence (case 1) — found, root-caused, fixed

**Naive S1 emission** (loop locals = `var x any` outside the `for {}`, recur =
temp-assign + `continue`): the emitted closures print **`3 3 3`**; real Clojure
prints **`0 1 2`**. Root cause is a genuine semantic mismatch, not a bug in the
flattening idea:

- **Clojure**: JVM closures copy the local's value into a field at closure
  creation → each closure keeps its iteration's `i`. (Verified against the CLI,
  not assumed.)
- **Go**: closures capture the *variable* by reference. Go 1.22's per-iteration
  loop-var change is **irrelevant here** — it applies only to variables declared
  in the `for` statement's init; our carriers are declared *outside* `for {}`,
  so all closures alias one variable that ends at `3`.

**Fix (now in this spike's emitter, toggleable via `NoCaptureFix` for the
demo):** for each loop binding that is closed over by an `fn`:

1. **Per-iteration copy** — at the top of the `for` body emit
   `i8 := i_c6` and rebind the Clojure name to the copy for all body reads.
   Closures created anywhere in the body (including inside recur args, case 1b
   second form) capture that iteration's fresh variable → `0 1 2` / `2`.
2. **Binding-var/carrier split** (case 1b first form) — a closure in a loop
   *binding init* runs before the `for`, so a copy at loop-body-top can't help
   it. Emit the binding var (`var i3 = init`, never reassigned; init-position
   closures and later inits read it) and a separate carrier
   (`var i_c6 any = i3`) that is the only thing recur reassigns → the init
   closure sees `0` forever, matching Clojure.
3. recur always writes the **carriers** (recurFrame holds carrier names);
   reads resolve through the scope stack to copy/binding-var as appropriate.

The copies are semantically transparent (carriers are only written immediately
before `continue`), so "always copy" would also be correct — capture detection
only avoids the per-iteration assignment when nothing is captured. Uncaptured
bindings collapse to exactly S1's proven emission.

## Other findings

- **fn-level recur needs no goto — and the goto folklore was wrong anyway.**
  Implemented as: params bound from `args`, then a labeled `for {}` whose
  non-recur paths `return` directly; recur = rebind params + `continue`.
  `for {}` with no `break` is a terminating statement, so Go needs no trailing
  return. S1's RESULTS flagged "goto-over-declarations" as the untested
  restriction; measured here: Go rejects only **forward** gotos over
  declarations (`goto L jumps over declaration of x`) — a **backward** goto
  over body temps compiles and runs fine, so doc 04's `goto` sketch would have
  worked. for/continue is still the right choice: one recurFrame shape shared
  with `loop*`, no block-structure reasoning.
- **Shadowing costs nothing.** S1's fresh-suffixed-name scheme (`x3`, `x7`…)
  makes case 2 fall out: the recurFrame holds the loop binding's Go names, so a
  let-shadow inside the body can't be rebound by recur even though the Clojure
  names collide. No divergence, no emitter change.
- **Macro-expansion shapes fall out of the `""`-r-value convention.** recur in
  the tail of `if`-under-`do`-under-`if` (`when`), and of `if` branches whose
  test is a let-temp (`and`/`or`) needed zero changes: the branch that recurs
  assigns nothing to the if's result temp, the `_ = tmp` defense keeps Go's
  unused-var check quiet. Note `or` with recur in a NON-last operand is
  non-tail and rejected by Clojure — only last-operand recur ever reaches the
  emitter.
- **Simultaneous rebinding was already right** (S1's temps-then-assign); case 4
  is now a regression test proving sequential assignment would collapse the
  swap.
- **Two loops in one expression can't collide** — temps, labels, and locals all
  draw from one monotonic counter per emitted file.

## Rules the real pkg/emit + analyzer must encode

1. **Analyzer annotates each loop/fn-method binding with `captured?`** —
   "referenced under an fn* nested in my body, or in a later sibling binding's
   init, respecting shadowing". This spike's `capturedIdx`/`loopCaptured`
   re-walk is a shortcut; the analyzer already resolves every local to its
   binding site, so it's a bit on the binding, not a second traversal.
   (Same argument S1 made for `recursDirectly` — that one also stays.)
2. **Emitter, for `captured?` recur carriers:** binding var (immutable) +
   carrier (recur target) + per-iteration copy at for-body top; scope maps the
   Clojure name to init-scope=binding-var, body-scope=copy. Uncaptured
   carriers keep the single-variable S1 emission.
3. **fn-method with recur = labeled `for {}` + continue**, params as carriers,
   non-recur paths return directly. Same recurFrame machinery as `loop*`; no
   goto, no IIFE, no trailing-return special case. Multi-arity: this per-method
   emission repeats inside each `switch len(args)` case unchanged.
4. **`recur` across `try` is the analyzer's error**, matching Clojure's
   compile-time `Cannot recur across try` (UnsupportedOperationException at
   the recur form's location). The emitter may assert-panic on a recur whose
   frame crosses a boundary, but user-facing rejection (with source position)
   happens at analysis, before emission. Ditto non-tail recur (`Can only recur
   from tail position`).
5. **Don't rely on Go 1.22 loop-var semantics for anything** — emitted carriers
   live outside the `for` header, so the 1.22 change neither helps nor hurts;
   correctness comes entirely from rule 2.
