# s69 — `defexpand` — CONSOLIDATED VERDICT

Date: 2026-07-28 · ADR **0107** · Status: **CLOSED — proceed to openspec**

Two independent tracks, same base (`92f6da7`), no shared code:

| track | question | branch | verdict |
|---|---|---|---|
| mechanism | *can* the analyzer expand a call site, and what does it cost? | `s69/mechanism` @ `6ec7e3d` | **MET** |
| semantics | what must `defexpand` *mean*? | `s69/semantics` @ `aa53861` | **MET-WITH-CAVEATS** |

Per-track detail: `VERDICT-mechanism.md`, `VERDICT-semantics.md`. Evidence is
regenerable (`capture-evidence.sh`, `run.sh`, `gen-size.sh`).

## The four findings that change ADR 0107

### 1. The mechanism is real, and it is one seam

`pkg/analyzer/parseInvoke` is the *only* place a call form becomes an
`OpInvoke`, and **both harnesses share one Analyzer instance**
(`pkg/eval/eval.go:80` builds it, `pkg/emit/compile.go:90` calls
`ev.Analyzer().Analyze`). An 81-line `tryInlineExpand` there is the entire
change — **`pkg/emit` and `pkg/eval` need ZERO lines**. Dual-mode identity
(ADR 0002) therefore holds by construction, not by discipline.

Proof, from real emitted Go (`emitted-main.go.txt`) — the same
`(swap! todo conj …)` written both ways:

```go
// defexpand call site — the swap! is spliced in, no wrapper
tmp35 := v_clojure_DOT_core_swap_BANG_.Get()
tmp37 := lang.Apply3(tmp35, a__x12533, tmp36, x__x12634)

// defn call site — a var lookup and a call, to reach the same swap!
tmp38 := v_user_wrap_BANG_.Get()
tmp40 := lang.Apply2(tmp38, tmp39, "WRAPPED-MARKER")
```

Free side effect: cljgo's `definline` — which has stored `:inline` metadata
that **nothing has ever read** (`core/core.clj:2474`) — starts actually
inlining. The consumer *is* the change.

### 2. The fn fallback is provably impossible as a macro — so `:inline` is mandatory, not "natural"

ADR 0107 currently hedges: *"`:inline`-style metadata is the natural carrier"*.
The semantics track closed that question. One var per name; whichever of
`defmacro`/`defn` runs last wins; `(map dbl [1 2 3])` throws
`wrong number of args (1) passed to: user/dbl`. Rule 4 (still a value when you
need one) **cannot be built above the analyzer at all**. `definline` already
demonstrates the working shape in cljgo today — `(fn? dsqr)` true,
`(map dsqr [1 2 3])` ⇒ `(1 4 9)`, `:inline` present, inert only for want of a
consumer.

### 3. Once-only, done naively, costs MORE than the tax it removes

The biggest surprise, and it inverts the ADR's premise. Wrapping arguments in a
`let` to guarantee single evaluation is *slower in the tree-walk evaluator than
the function call it replaced*:

| shape | interpreted, vs raw |
|---|---|
| `defn` (what we're replacing) | 1.24× |
| `defexpand`, once-only unconditional | **1.39×** ← worse |
| `defexpand`, once-only + elision | **0.98×** ← parity |

So **R1′ — elide the temporary for literals and bare locals — is a RULE, not an
optimisation.** Without it ADR 0107's own "sugar with no tax" claim is false
interpreted. Independently reproduced on the real `cljx.core` port:
`defn add-fn!` 216.6 ms · `defexpand add!` 171.3 ms · raw `swap!` 173.9 ms.

Hygiene tightens as a consequence: once R1′ elides the binding, the caller's
symbols are spliced in bare, so body-local renaming (R2) becomes **mandatory**
rather than belt-and-braces — measured: `(let [a 2] (scale-nre a))` ⇒ `10000`,
want `200`.

### 4. `with-redefs` — the ADR's flagged risk — has a real answer, and it costs code size

The two tracks measured *different designs* and both numbers are true:

- **Unguarded** (mechanism track): a direct call site does **not** see
  `with-redefs` — `[:direct :REDEFFED]`. This is exactly the JVM's behaviour
  for `:inline`'d `clojure.core` fns, so it is defensible.
- **Guarded** (semantics track, ADR 0066 dirty-flag shape): it **does** —
  `normal 12 / redefed 7 / after 12`, and `alter-var-root` too. Crucially,
  **cljx.test's `stub` and `spy` both work through it** (spy saw both inlined
  sites: `calls => [5 7]`).

**Recommendation: guarded.** ADR 0105 promises `stub`/`spy`; a test that passes
because the stub never fired is worse than a slow test. "Inline in AOT only"
was rejected outright — that is REPL-vs-binary divergence, the unforgivable
failure mode.

Two honest caveats the design must carry:
- ADR 0066's flag **cannot be reused literally**: it is one process-global
  `atomic.Bool` for seven arithmetic vars. Sharing it would mean redefining `+`
  de-optimises every `defexpand` in the program. Needs a per-defexpand (or
  per-namespace) flag; 0066's "once tripped, never clears" carries over.
- The guard costs emitted size on top of the inline — and the mechanism track's
  size numbers were taken **unguarded**, so they are a floor, not the answer.
  Unguarded: +1.2 Go lines/site, ~231 B/site (+1.71% over 500 sites, +0.25% at
  50). Guarded must be re-measured against the ADR 0004 budget **before**
  openspec ratifies R5.

## Where the win actually is (correcting the ADR)

ADR 0107 says *"compiled, the emitter already erases most of it"*. That holds
only for **allocation-dominated** bodies. On a cheap body it is badly wrong:

`(* x x)` × 3,000,000, compiled: `defn` wrapper **3.385×**, `defexpand`
**1.013×**. Compiled is where `defexpand` pays **most**, not least.

## Blockers before implementation

1. **Self-recursion is a raw Go `fatal error: stack overflow`** with a
   goroutine dump — cljgo's stated unforgivable failure mode. 30 s wall,
   2.66 GB RSS, no output. `maxMacroExpansions` does not catch it (the
   recursion runs through `analyzeForm`, not the expansion loop). Reject
   self-reference at definition time; depth-64 budget as the backstop for the
   mutual case, which a local check cannot see.
2. **Variadic** fails today with `unable to resolve symbol: &`. Reject at
   definition time with a real diagnostic. (JVM `definline` can't do variadic
   either.) **This bites `cljx.core` now**: `add!`'s `& kvs` arity and
   `upd!`/`upd-in!` stay `defn` in v1. Multi-arity IS feasible and was
   demonstrated.
3. **Arity errors need `Expected`** per the CLAUDE.md doctrine — today
   `wrong number of args (4) passed to: user/bump-hand!` is named but has no
   expected-vs-found.
4. `-defexpand-walk` / `-defexpand-replace` / `-defexpand-emit` were **public
   vars in `clojure.core`** in the prototype — a precedence-principle
   violation. Private, or moved out.
5. The hygiene walker is naive (no destructuring, `letfn`, `catch`, `for`/
   `doseq` vectors, `#()`); it should share code with syntax-quote. R1′'s
   `dx-simple?` must elide **only** literals and true locals — a bare symbol
   resolving to a *var* must not be snapshotted.

## Incidental repo defects found (file separately, none blocking)

- `load-file` fails on a two-line file with `cannot call nil` despite
  `(bound? #'load-file)` being true.
- **Core vars carry no metadata at all** — `(meta (resolve 'min))` ⇒ `{}`, no
  `:ns`/`:name`/`:doc`/`:arglists`; user vars have `:ns` but no `:name`. This
  defeats the ordinary var-qualification idiom and forced the prototype into
  `(subs (str v) 2)`. Likely to bite other work soon.
- `(.getMessage e)` fails on `ExceptionInfo` (`no method getMessage`) where the
  JVM allows it; `ex-message` works.
- No `System/nanoTime` (`no such namespace: System`).
- **Orchestration hazard:** parallel tracks sharing one `/tmp/cljgo-*` binary
  path race and produce boot panics. Never share a built-CLI path across
  worktrees.

## Verdict

**PROCEED to openspec** — mechanism proven, single-seam, ~300 LOC plus tests,
semantics settled as R1–R7 (see `VERDICT-semantics.md`, adopt verbatim).

**Do not propose until the guarded-inline size number exists**, since R5 and
R1′ both have emitted-size consequences that must be priced against ADR 0004.
