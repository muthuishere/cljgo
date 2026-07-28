# s69 / semantics — VERDICT

**Question:** WHAT must `defexpand` (ADR 0107) *mean*?
**Method:** a macro-based prototype (`prototype.clj`) that approximates
`defexpand` without touching the analyzer, plus eight runnable demos. Every
number and every quoted output in this file is reproduced by
`./capture-evidence.sh` into `evidence.txt`.
**Base:** `92f6da7`, branch `s69/semantics`, darwin/arm64, `go build -o
/tmp/cljgo-s69-semantics ./cmd/cljgo`.
**Status:** semantics settled. **The mechanism must NOT be a macro** — see §6.

---

## RECOMMENDED SEMANTICS

*(This section is written to be absorbed into ADR 0107 verbatim.)*

A `defexpand` is a **function** whose direct call sites the compiler may
splice. Its meaning is defined by one invariant and four expansion rules.

### The invariant

> **Observational equivalence with `defn`.** For any call site, a `defexpand`
> must produce the same value, the same side effects, in the same order, as
> the `defn` with the identical parameter vector and body — *except* for the
> one divergence named in R5. If a rule below ever conflicts with this
> invariant, the invariant wins and the compiler emits an ordinary call.

### R1 — Arguments evaluate exactly once, left to right

Every argument is bound to a fresh temporary in one sequential `let`, in
argument order, **before** the body runs. A parameter used twice in the body
reads the temporary twice; it does not re-evaluate the argument. A parameter
**not** used in the body is still evaluated (a function would).

*Evidence (`01-once-only.clj`):* naive macro evaluates its argument **2**
times → `3`; `defexpand` **1** time → `2`; `defn` **1** time → `2`.
Order `[:a :b :c]` for both `defexpand` and `defn`. Unused parameter: log
`[:a :b]`, i.e. `:b` still evaluated.

### R1′ — Elide the temporary when it is provably free (REQUIRED, not an optimisation)

Bind an argument only when it is **not simple**. *Simple* = a literal
(number, string, keyword, char, `nil`, `true`/`false`) or a bare symbol
resolving to a local. Simple arguments are spliced directly.

This rule is load-bearing, not cosmetic. A source-level `let` wrapper is
**more expensive than the call it removed** in the tree-walk evaluator:

| interpreted, 300k × `(add! a 1)` | median | vs raw `swap!` |
|---|---|---|
| raw `(swap! a conj 1)` | 171.5 ms | 1.00× |
| `defn add-fn!` | 212 ms | **1.24×** |
| `defexpand`, R1 unconditional | 238 ms | **1.39×** ← *worse than the fn* |
| `defexpand`, R1 + R1′ | 168 ms | **0.98×** |
| naive substitution (unsafe) | 170 ms | 0.99× |

*Evidence (`08-cost-of-once-only.clj`, 3 rounds).* Without R1′ the whole
premise of ADR 0107 ("sugar with no tax") is false in interpreted mode: the
tax gets **bigger**. With R1′ the tax is gone and the sugar costs the same as
the form it documents. R1′ costs nothing semantically — a simple form is
effect-free and idempotent, so eliding its temporary is unobservable
(re-verified in `08` §C: `(add! box (t :once 1))` still logs `[:once]`).

### R2 — Rename every local the body introduces

Every `let` / `let*` / `loop` / `fn` / `when-let` / `if-let` binding in the
body is renamed to a fresh name at expansion. The author never writes a
gensym.

R2 is **not optional once R1′ is in force** — R1′ splices the caller's own
symbols into the body, so an un-renamed body local captures them:

```
H. R1 elided + no body-local renaming = capture is BACK:
   expansion: (do (clojure.core/let [a 100] (clojure.core/* a a)))
   (let [a 2] (scale-nre a)) => 10000  (want 200)
   (let [a 2] (scale-ok  a)) => 200  (want 200)
```

*Evidence (`02-hygiene.clj` §B, §E, §H).*

### R3 — Free symbols resolve in the DEFINING namespace, at expansion time

Every symbol in the body that is neither a parameter nor a body local is
rewritten to its fully-qualified var name, resolved **in the namespace where
the `defexpand` was written**, **at expansion time** (not definition time).

Renaming alone (R2) does not give hygiene — the body's *free* references are
still capturable:

```
D. body free reference `limit` (a var = 10), caller has (let [limit 0] ...):
   naive                  => 0
   defexpand, no qualify  => 0     ← R2 alone is NOT enough
   defexpand + qualify    => 5     ← correct
   defn                   => 5
```

Resolving at **expansion** time rather than definition time is what makes
forward references work — a body that calls a `defn` written later in the
file still resolves, and still cannot be hijacked by a caller local:

```
G. body calls `helper`, defined AFTER the defexpand:
   expansion: (do (user/helper 2))
   (call-later 2)                                   => 2000
   (let [helper (fn [_] :hijacked)] (call-later 2))  => 2000
```

**Hygiene = R2 + R3.** Recommend stating exactly that in the ADR: parameters
and body locals are renamed; body free references are resolved in the defining
namespace. Neither half suffices alone.

### R4 — Only *direct* call sites expand; the name is still a real function

`(map add! …)`, `(apply add! …)`, `(partial add! …)`, `#'add!` all see an
ordinary fn. This is **not implementable as a macro** (§6) and is the decisive
argument for the `:inline`-metadata mechanism.

### R5 — Redefinition: inline sites are guarded, and the guard is elidable

**Recommendation: option (b), the ADR 0066 dirty-flag shape — not (a), not (c).**

The honest rule to write in the ADR:

> A `defexpand`'s inlined call sites remain **live**: redefining the var
> (`with-redefs`, `alter-var-root`, a later `def`) is seen at every call site,
> because each inlined site is emitted as
> `(if <pristine?> <inlined-body> <call-through-the-var>)` and the pristine
> test is elided until the process's first redefinition of a sealed
> `defexpand`. Once anything is redefined, the guard is permanent for the rest
> of the process (ADR 0066's monotonic dirty flag, applied per-`defexpand`).

This was built and measured, not assumed (`03-with-redefs.clj` §D–§G):

```
D. defexpand-guarded (ADR 0066 shape):
   expansion: (let [a__111 3 b__112 4]
                (if (identical? user/gmul-fn user/dx-pristine-gmul)
                    (do (* a__111 b__112))
                    (user/gmul-fn a__111 b__112)))
   normal  => 12
   redefed => 7        ← with-redefs IS seen at the inlined site
   after   => 12       ← and restored
E. alter-var-root => -1   ← the other root writer is covered too
```

**Can ADR 0066's dirty flag cover `defexpand`? Yes, and it is the right fit.**
0066's machinery is exactly this problem: `(*Var).Seal()` + `tripIfSealed()`
on `BindRoot`/`AlterRoot` (the only two root writers, which is what makes it
sound), and `with-redefs` already rides `alter-var-root`, so it trips the flag
for free. Two adaptations are needed:

1. **Per-`defexpand` (or per-band) flag, not the one global arithmetic flag.**
   0066's flag is process-global for the seven arithmetic vars; reusing that
   literal flag would mean redefining `+` de-optimises every `defexpand` in the
   program. Give `defexpand` its own flag (one `atomic.Bool` per defexpand var,
   or one flag per namespace) so the blast radius matches the cause.
2. **The fallback branch must exist in the emitted code**, which is the real
   cost: emitted size grows by the guard + the call, and 0066's caveat applies
   verbatim ("once tripped, never clears"). The size question belongs to the
   other track; this track's finding is that the guard is *semantically*
   sufficient and cheap (one `identical?` compare, branch-predicted).

**Why not (a) "accept it and document":** it breaks `cljx.test` (ADR 0105).
`stub`/`spy` are built on `with-redefs`, and a user who "optimises" a fn into
a `defexpand` would silently lose the ability to stub it — a test that passes
because the stub never took effect is worse than a slow test. Measured
directly (`03` §F–§G):

```
F. cljx.test-style stub via with-redefs-fn:
   stub a defn        => :stubbed
   stub a defexpand   => (no var to stub — the name is a macro)
   stub a guarded one => :stubbed
G. cljx.test-style spy on the guarded form:
   result => [10 14]
   calls  => [5 7]     ← the spy SAW both inlined call sites
```

**So the explicit answer the ADR owes ADR 0105: yes, a `defexpand`d fn can
still be stubbed and spied — but only under R5's guarded shape.** Under an
unconditional inline it cannot be, at all.

**Why not (c) "never inline interpreted, only AOT":** it violates ADR 0002 /
the dual-harness rule outright. The REPL would see redefinition and the binary
would not — REPL-vs-binary divergence, which CLAUDE.md names the unforgivable
failure mode. (c) also removes the interpreted win that §R1′ shows is real
(1.24× → 0.98×). Reject it.

*One thing that already works either way and should be documented as the
common case:* stubbing a fn that the `defexpand` **body calls** works
unconditionally, because the body's calls go through vars like any other code
(`03` §C: `[real:a real:a]` → `[STUB:a STUB:a]`).

### R6 — Recursion is rejected at definition time, with a depth budget as backstop

A self-referential body **does not terminate** and does not error — measured,
not asserted:

| case | outcome | peak RSS |
|---|---|---|
| `(defexpand fact [n] (if (<= n 1) 1 (* n (fact (- n 1)))))`, then `(fact 5)` | no output, killed at **30 s** | **2.66 GB** |
| mutual `od`/`evn`, then `(evn 4)` | no output, killed at **30 s** | 116 MB |

In both cases *definition* succeeded silently; the divergence is deferred to
the first call site. That is the worst possible failure shape (a hang at
compile time, with a memory blowup, far from the code that caused it).

Recommended rule, in order:

1. **Reject direct self-reference at definition time.** The check is one line
   and precise — `(some #{name} (symbols-of body))` — verified in `04` §A
   (`fact` ⇒ `true`, `add!` ⇒ `false`). Error text per the CLAUDE.md doctrine:
   name the defexpand, locate it, and say `help:` → "a defexpand may not call
   itself; use defn, or call the fn fallback".
2. **A hard expansion-depth budget (suggest 64) as the backstop**, because the
   local check cannot see mutual recursion — at `od`'s definition time `evn`
   does not exist yet (`04` §A). The budget catches both shapes and turns a
   hang into a diagnostic (`04` §B).
3. **Do not** attempt "expand once, then resolve the self-call to the fn". It
   works (`04` §C: `(call-fact 5) => 120`) but it makes a `defexpand` mean two
   different things at two depths, and the payoff — inlining one level of a
   recursive fn — is not what `defexpand` is for.

### R7 — v1: fixed arity, multiple arities allowed, variadic rejected

- **Multi-arity: accept.** Dispatch on the call site's argument count is
  exactly what an expander already has. Demonstrated working
  (`05` §D — `(bump-hand! c1)` / `(… c2 :a)` / `(… c2 :a 5)`, all equal to
  their `swap!` forms).
- **Variadic (`& xs`): reject in v1.** It is mechanically expressible — a
  hand-written variadic expander preserves once-only and order (`05` §B, log
  `[:1 :2 :3]`) — but `(apply f a xs)` can never be expanded, because the arity
  is unknown until runtime. A variadic `defexpand` is therefore *inherently*
  "inline when literal, call the fn otherwise", which is a strictly bigger
  design (it presupposes R4's fn fallback and R5's guard). Ship it after v1 if
  `cljx.core` needs it; the current rejection error is already clear
  (`05` §A). Note this bites `cljx.core` immediately: `add!`, `upd!` and
  `upd-in!` are variadic today, so their variadic arities stay `defn`.
- **Arity errors** must name the defexpand *and* its arities. cljgo already
  names the macro and the count (`05` §E: `wrong number of args (4) passed to:
  user/bump-hand!`) but gives no `Expected`; add `(expects 1: [a], 2: [a k],
  3: [a k n])` per the CLAUDE.md doctrine.

---

## §6 — THE DECISIVE FINDING: the mechanism cannot be a macro

ADR 0107 rule 4 (a real fn under the same name) is **provably impossible** in
the macro approach, and this is the strongest argument for the other track's
`:inline`-metadata mechanism.

```
A. defexpand'd name used as a VALUE:
   (dbl 4)                  => 8            ; direct call: fine
   (map dbl [1 2 3])        => THREW: wrong number of args (1) passed to: user/dbl
   (apply dbl [4])          => THREW: wrong number of args (1) passed to: user/dbl

B. can a name be BOTH a macro and a fn value?
   after (defmacro mname ...): (mname 1)   => 2
   after (defn mname ...):     (mname 1)   => 0   ; the macro is GONE
   ; whichever def runs last wins. There is no 'both'.

C. `definline` today (metadata stored, no call-site inlining):
   (fn? dsqr)        => true
   (map dsqr [1 2 3])=> (1 4 9)
   (:inline (meta (var dsqr))) present? => true
```

One namespace holds one var per name. `defmacro` binds that var to a macro fn
and flags it `:macro`; `defn` of the same name overwrites it. There is no
"both". Worse, the *error* a user gets from `(map add! …)` is
`wrong number of args (1) passed to: user/add!` — a confusing message about
the macro's internal calling convention.

`definline` already proves the alternative shape exists in cljgo **today**: the
var is a real fn (`(fn? dsqr)` ⇒ `true`, `(map dsqr [1 2 3])` ⇒ `(1 4 9)`) and
simultaneously carries `:inline` metadata. It is inert only because nothing
acts on the metadata (`core/core.clj:2462`). `defexpand` is therefore
**`definline` with an ordinary body, hygiene, and an analyzer that honours the
metadata** — and R1–R7 above are the semantics that analyzer must implement.

The prototype's own workaround makes the point: `defexpand-guarded` has to
emit the fn under a *different* name (`gmul-fn`), and `(map gdbl [1 2 3])`
still throws (`06` §D). The name split is an artefact of the mechanism, not of
the semantics.

---

## §7 — `cljx.core` port (07-cljx-port.clj)

`add!` (2-arity), `add-kv!`, `bump!`, `bump-k!`, `toggle!`, `del!`, `clear!`
ported to the prototype and checked against their documented `swap!`/`reset!`
forms in the equivalence style of
`conformance/tests/cljx-core-atom-verbs.clj`:

```
B. equivalence to the raw swap!/reset! forms:
   [true true true true true true true {harsh 2, shaama 1} true true]
   all true? => true
C. once-only + order inside the ported sugar:
   evaluation log => [:atom :val]
   under (let [a :caller] ...) => [:caller]
D. interpreted cost, 300k iterations:
   defn add-fn!   216.6 ms
   defexpand add! 171.3 ms
   raw swap!      173.9 ms
```

The expansions are literally the documented forms — `(add! todo "x")` ⇒
`(do (clojure.core/swap! todo clojure.core/conj x))` as printed by `println`
(which strips the string's quotes; the form carries the string literal).
**The ADR 0106 premise holds: with R1′ in place the sugar costs the raw form,
and the fn costs 1.24×.** Note the multi-arity/variadic caveat in R7 — the
`& kvs` arity of `add!` and the whole of `upd!`/`upd-in!` stay `defn` in v1.

---

## Honest limits of this spike

- **It is a macro, so §6's limitation is baked in.** Everything about *values*
  (`map`/`apply`/HOFs), and everything about REPL-vs-binary parity, is
  untested here by construction. The other track owns the mechanism.
- **The prototype's substitution does not respect `quote`.** A quoted symbol
  equal to a parameter or a body local is rewritten. The real analyzer walks a
  typed AST and does not have this bug; do not read it as a design constraint.
- **Destructuring is not handled.** `dx-locals` collects plain symbols from
  binding vectors only. Map/vector destructuring in a `defexpand` body or
  parameter vector is untested — the ADR should say whether v1 allows it
  (recommend: allow in the body, defer in the parameter vector).
- **`dx-freemap` qualifies via `pr-str` of the var** (`#=(var ns/name)`)
  because cljgo vars carry no `:name` in their meta. Spike-grade.
- **All perf numbers are interpreted only**, from cljgo's `time` macro, single
  machine, 3 rounds. They are directionally solid (the elision effect is
  ~40% and reproduces every round) but they are not the AOT numbers ADR 0107
  asks for; that is the other track's measurement.
- **No repo gate was run and no repo code was touched.** This track made zero
  changes to `pkg/analyzer`, `pkg/eval`, `pkg/emit` — everything lives under
  `spikes/s69-defexpand/`. `go build ./...` is unaffected.

## Incidental findings (unrelated to defexpand, worth a ticket each)

1. **`load-file` is broken.** `(load-file "tiny.clj")` on a two-line file
   fails with `error: cannot call nil - this may be an unbound var or a
   function that returned nil`, even though `(bound? #'load-file)` is `true`.
   This is why every demo here is run by concatenating `prototype.clj` in
   front of it (`run.sh`).
2. **Core vars carry no metadata.** `(meta (resolve 'min))` ⇒ `{}` — no
   `:ns`, `:name`, `:doc` or `:arglists`. User vars carry `:ns` but still no
   `:name`. This defeats the ordinary `(symbol (str (:ns m)) (str (:name m)))`
   idiom for qualifying a var.
3. **`(.getMessage e)` fails on `ExceptionInfo`**: `no method getMessage on
   *lang.ExceptionInfo`. `ex-message` works. On the JVM `.getMessage` works on
   `ExceptionInfo`, so this is a divergence.
4. **No `System/nanoTime`** (`no such namespace: System`); `time` is the only
   clock available to a script.

## Files

| file | what |
|---|---|
| `prototype.clj` | the macro-based `defexpand` + `-naive` / `-unqualified` / `-no-rename` / `-no-elide` / `-guarded` variants |
| `01-once-only.clj` | R1: once-only + left-to-right order |
| `02-hygiene.clj` | R2 + R3, and why neither half suffices |
| `03-with-redefs.clj` | R5: the divergence, the ADR-0066-shaped guard, cljx.test stub/spy |
| `04-recursion.clj` | R6: the definition-time check + depth budget (terminating half) |
| `04a-…` / `04b-…` | R6: the two divergent cases, run under `timeout` |
| `05-variadic-multiarity.clj` | R7 |
| `06-fn-fallback.clj` | §6, the decisive finding |
| `07-cljx-port.clj` | §7, the cljx.core port + equivalence table |
| `08-cost-of-once-only.clj` | R1′, the elision measurement |
| `run.sh` / `run-recursion.sh` / `capture-evidence.sh` | drivers |
| `evidence.txt` | full captured output (regenerate with `./capture-evidence.sh`) |
