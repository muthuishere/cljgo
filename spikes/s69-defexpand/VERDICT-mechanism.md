# s69 — `defexpand` (ADR 0107), track: MECHANISM · branch `s69/mechanism`

**Question:** CAN the analyzer expand a call site at compile time, and what
does it cost?

**Verdict: MET.** A call site was really inlined (proved in emitted Go), in
BOTH harnesses, with one seam in the analyzer, zero emitter changes, and the
full repo gate green. Measured numbers below.

Base commit `92f6da7`. Prototype quality — see "What is prototype-grade".

---

## 1. The seam

`pkg/analyzer/analyzer.go` → **`parseInvoke`**, first thing after the context
setup (line ~944). That single function is the ONLY place a call form becomes
an `OpInvoke` node, and both harnesses go through it:

- interpreter: `pkg/eval/eval.go:80` builds the one `analyzer.Analyzer`
- AOT: `pkg/emit/compile.go:90` — `ev.Analyzer().Analyze(form)` — the SAME
  analyzer instance

So **the interpreter and AOT paths do NOT need separate work.** That was the
biggest open question and the answer is clean.

New code: `tryInlineExpand(seq, env)` (81 lines with comments). It:
1. requires the operator to be a symbol not shadowed by a local and not a
   special (mirrors `macroexpand1`'s shadowing rule);
2. `ResolveVar`s it, reads `:inline` off `v.Meta()` (already an `IPersistentMap`
   on every Var — the carrier existed, `core/core.clj:2474`, and nothing
   consumed it);
3. gate on `:inline-arities` (an `IFn` — a set works, exactly like the JVM);
4. `inline.Invoke(argForms...)` under a `recover`, then
   `a.analyzeForm(replacement, env)`.

**The HOF fallback is free.** `(map add! …)` never reaches `parseInvoke` for
`add!` — that is `analyzeSymbol`, which builds an `OpVar`. Only *direct* call
sites are rewritten. No extra machinery was needed for ADR rule 4.

`defexpand` itself is 114 lines of Clojure at the end of `core/core.clj`
(a walker + an expansion-site emitter + the macro). Nothing in Go knows the
word "defexpand" — the analyzer only knows `:inline`.

**Side effect, free:** cljgo's previously-inert `definline` now actually
inlines (t1 case 6) — closing the documented performance-only divergence from
the JVM.

## 2. Proof it inlined (emitted Go)

`inl.clj` has one `defexpand add!` call and one `defn wrap!` call, both
`(swap! todo conj "…MARKER")`. Emitted Go (`emitted-main.go.txt`, lines
100–115):

```go
// the defexpand call site — NO wrapper call, the swap! is right here
{
    tmp32 := v_user_todo.Get()
    var a__x12533 any = tmp32
    var x__x12634 any = "INLINED-MARKER"
    tmp35 := v_clojure_DOT_core_swap_BANG_.Get()
    tmp36 := v_clojure_DOT_core_conj.Get()
    tmp37 := lang.Apply3(tmp35, a__x12533, tmp36, x__x12634)
    tmp31 = tmp37
}
// the defn call site, for contrast — a real call through the var
tmp38 := v_user_wrap_BANG_.Get()
tmp39 := v_user_todo.Get()
tmp40 := lang.Apply2(tmp38, tmp39, "WRAPPED-MARKER")
```

(That dump predates the "simple argument" optimisation, which now substitutes
literals/symbols directly instead of binding a temp — the current emission is
strictly smaller.)

## 3. Semantics — `/tmp/cljgo-s69 run spikes/s69-defexpand/t1-basic.clj`

```
1. value: [buy milk ship spike]
2. fn?: true
2b. doseq: [1 2 3]
2c. apply: [:via-apply]
2d. map over the var: (#{10} #{20 10})
3. eval order/count: [:first :second] -> [99]
4. hygiene: [:caller-x :caller-x] x= :caller-x c= :caller-c
5. shadowed: :shadowed
6. definline: 25 (1 4 9) true
```

ADR rules 1–4 all hold: correct value; `fn?` true and `apply`/`map`/`doseq`
work (rule 4); arguments evaluated once each, left to right (rule 3); the
caller's `x` and `c` are untouched by a body that binds `c` (rule 2); a local
shadowing the name suppresses expansion.

Hygiene has a *second*, unplanned source: because non-simple arguments are
bound in an OUTER `let*` before the body runs, an argument form can never be
captured by a `let` inside the body — the once-only wrapper buys
capture-avoidance for free.

## 4. Speed (best-of-5, mac arm64, go 1.26.3)

### bench.clj — `(swap! a conj i)` × 300 000 (allocation-dominated)

| | interpreted ms | ratio | compiled ms | ratio |
|---|---|---|---|---|
| hand-written | 159.3 | 1.000 | 66.9 | 1.000 |
| `defn` wrapper | 204.1 | **1.281** | 68.0 | 1.016 |
| `defmacro` | 158.3 | 0.994 | 67.9 | 1.015 |
| **`defexpand`** | 159.9 | **1.004** | 69.3 | 1.036 |

Confirms the ADR's 1.17–1.28× interpreted baseline (1.28× measured) and that
`defexpand` erases it exactly.

### bench2.clj — `(* x x)` × 3 000 000 (call-overhead-dominated, no alloc)

| | interpreted ms | ratio | compiled ms | ratio |
|---|---|---|---|---|
| hand-written | 1440.8 | 1.000 | 23.2 | 1.000 |
| `defn` wrapper | 1857.2 | **1.289** | 78.6 | **3.385** |
| `defmacro` | 1452.4 | 1.008 | 23.0 | 0.991 |
| **`defexpand`** | 1429.3 | **0.992** | 23.5 | **1.013** |

**This corrects the ADR.** "Compiled, the emitter already erases most of it"
is true only for allocation-dominated bodies. On a cheap body the compiled
`defn` wrapper costs **3.4×** and `defexpand` is at parity with hand-written.
Compiled is where `defexpand` pays *most*, not least.

## 5. Code size (`gen-size.sh`)

Same program, N call sites of `(add! todo i)`, `defn` vs `defexpand`:

| call sites | emitted `main.go` | stripped binary |
|---|---|---|
| defn, 1 | 110 lines | 6 684 722 B |
| defn, 50 | 306 lines | 6 684 722 B |
| defn, 500 | 2 106 lines | 6 767 282 B |
| defexpand, 1 | 135 lines | 6 684 738 B |
| defexpand, 50 | 380 lines | 6 701 250 B |
| defexpand, 500 | 2 630 lines | 6 882 882 B |

- Emitted Go: **~4.0 lines/site** (defn) vs **~5.2 lines/site** (defexpand)
  — +1.2 Go lines per inlined call site for this one-form body.
- Binary: **+115 600 B over 500 call sites = ~231 B/site, +1.71 %**; at 50
  sites +16 528 B, **+0.25 %**.

Against the ADR 0004 budget mindset this is a non-event for realistic
`cljx`-shaped usage (aliases, small bodies, tens of call sites). It is NOT
free for a big body pasted into hundreds of sites — the design should say so
and keep `defexpand` bodies small, as the ADR already anticipates.

Build time was not a measurable factor (expansion is one `IFn` call per site).

## 6. Risks settled (`t2-risks.clj`, `t3-recursion.clj`)

```
1. with-redefs: [:direct :REDEFFED]
3. wrong arity: ERR: wrong number of args (4) passed to: user/add!
4. variadic: ERR: ... unable to resolve symbol: & in this context
5. cross-ns: [:x]
```

- **`with-redefs` — the honest rule, measured:** a direct call site is already
  expanded, so it does **not** see the redefinition; the value/HOF path does.
  Exactly the JVM's behaviour for `:inline`'d `clojure.core` fns. The design
  must state this plainly; it is not fixable without giving up the mechanism.
- **Arity mismatch is clean:** `:inline-arities` rejects, the call falls
  through to the real fn, and the user gets the ordinary named arity error.
- **Variadic is NOT supported** by the prototype and must be rejected at
  definition time with a real diagnostic (JVM `definline` can't do variadic
  either). Today it fails with a confusing "unable to resolve symbol: &".
- **Recursion is a hard stack overflow** — `(defexpand cd [n] (if (zero? n)
  :done (cd (dec n))))` then `(cd 3)` produces a raw Go
  `fatal error: stack overflow` goroutine dump. That is cljgo's *unforgivable*
  failure mode (CLAUDE.md). `maxMacroExpansions` does not help: the recursion
  goes through `analyzeForm`, not the expansion loop. **The implementation
  MUST detect a self-referential body at definition time (or carry an
  expansion-depth counter through `Env`).** This is the single must-fix.
- **Cross-namespace works** — the body's free symbols are qualified to their
  home namespace at definition time, so a call from another ns expands
  correctly.

## 7. Repo gate

```
go build ./... && go vet ./... && gofmt -l pkg cmd conformance templates   # clean
go test ./pkg/...                                                          # all ok
go test ./conformance/... -timeout 1800s -p 1                              # ok 90.2s
```

Green **after** `go generate ./pkg/coreaot && go generate ./pkg/briaot`
(mandatory — `core.clj` changed). Nothing in the existing corpus carries
`:inline`, so turning the analyzer seam on changed no existing behaviour;
the dual conformance harness confirms no REPL-vs-binary divergence.

## 8. Feasibility — how big is the real change

| area | work | rough LOC |
|---|---|---|
| `pkg/analyzer` | `tryInlineExpand` + the `parseInvoke` hook | ~90 |
| `pkg/analyzer` | expansion-depth guard in `Env` (recursion must-fix) | ~20 |
| `core/core.clj` | `defexpand` + hygiene walker + expansion emitter | ~150 |
| `core/core.clj` | reject variadic / multi-arity / self-reference, with `diag` codes | ~40 |
| `pkg/emit` | **none** | 0 |
| `pkg/eval` | **none** | 0 |
| conformance | new `conformance/tests/defexpand-*.clj` (dual harness) | ~8 files |
| regen | `go generate ./pkg/coreaot ./pkg/briaot` | — |

**~300 LOC plus tests.** The emitter needs no cooperation at all, and there is
one analyzer, so there is no interpreter/AOT split.

### What is prototype-grade here (do not ship as-is)

- **Single fixed arity only.** `cljx.core`'s `add!`/`bump!` are multi-arity;
  porting them (ADR 0107's endgame) needs `:inline-arities` to select a
  per-arity template. Straightforward but unwritten.
- **The hygiene walker is naive.** It gensyms `let`/`let*`/`loop`/`fn`/`fn*`
  binding names and qualifies everything else that `resolve`s to a var. It
  does not understand destructuring, `letfn`, `catch` bindings, `for`/`doseq`
  binding vectors, or `#()`. A real one should share code with syntax-quote.
- Symbol qualification goes through `(subs (str v) 2)` because corelib
  builtins carry no `:ns` meta — a real implementation should fix the Var
  metadata instead.
- No diagnostics codes; errors are bare strings.
- `-defexpand-walk` / `-defexpand-replace` / `-defexpand-emit` are public
  vars in `clojure.core` (precedence-principle violation — must be private
  or moved to a `cljgo.*` namespace before shipping).

## 9. Recommendation

Proceed. The mechanism is real, cheap, single-seam, dual-mode by
construction, and measurably delivers the zero-cost promise (1.28× → 1.00×
interpreted; 3.39× → 1.01× compiled on a cheap body). Before the openspec:

1. decide the recursion rule (reject at definition — cheapest and clearest);
2. decide multi-arity (needed for `cljx.core`);
3. write the `with-redefs` rule into the design as a stated, tested
   limitation, not a bug;
4. reject variadic with a real diagnostic.
