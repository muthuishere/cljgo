# ADR 0107 — `defexpand`: Zig-style compile-time expansion, written like a `defn`

Date: 2026-07-28 · Status: **proposed** (owner-directed, 2026-07-28: *"macros
should be created like zig style macros so it's easy for anyone to write,
instead of writing it in special form"* — and earlier, *"zig style … expanded
in compile time"*). Completes the ADR 0009 pair (macros for syntax, `comptime`
for values) with the missing third shape: **zero-cost functions**.

## Context

Two forces met here:

1. **The zero-cost mandate.** An `cljx.core` alias like `(add! a x)` that is a
   *function* costs a call frame on every use — measured 2026-07-28 at
   **1.17–1.28× vs hand-written `swap!` interpreted**. Sugar that taxes you is
   not sugar.

   > **Corrected by spike s69.** This ADR originally added "compiled, the
   > emitter already erases most of it". That is true only for
   > *allocation-dominated* bodies. On a cheap body — `(* x x)` × 3,000,000,
   > compiled — the `defn` wrapper costs **3.385×** and `defexpand` is at
   > **1.013×**. **Compiled is where `defexpand` pays most, not least.**
2. **The authoring cost.** The fix today is `defmacro`, which forces
   syntax-quote / unquote / gensym ceremony on the author:

   ```clojure
   (defmacro add! [a x] `(swap! ~a conj ~x))   ; backtick, tilde, quoting rules
   ```

   ADR 0009 already accepted this critique of macros ("powerful but
   ceremonial") and answered it for *values* with `comptime`. It never
   answered it for *functions* — which is the common case: "write it like a
   `defn`, but pay nothing for it."

`definline` cannot be that answer: it exists in `clojure.core`, and on the JVM
its body **returns a syntax-quoted form** — same ceremony — so re-meaning it
would violate the precedence principle. (In cljgo it is additionally inert:
`core/core.clj:2462` — "no call-site inlining happens".)

## Decision

Add **`defexpand`** — define it like a `defn`, and every call site is
**expanded at compile time** into the body with arguments substituted. No
backtick, no `~`, no gensyms.

```clojure
(defexpand add!
  "Append to a collection atom. Expands to (swap! a conj x)."
  [a x]
  (swap! a conj x))

(add! todo "buy milk")     ; compiles to exactly (swap! todo conj "buy milk")
```

Rules that make it safe and teachable:

1. **Ordinary body, ordinary reading.** The body is normal Clojure — what you
   see is what is spliced. Anyone who can write a `defn` can write one.
2. **Hygiene by construction.** Parameters and any `let` locals in the body
   are renamed to fresh names at expansion, so a `defexpand` can never capture
   or be captured by the caller's bindings. The author never writes a gensym.
3. **Arguments evaluate exactly once**, in argument order, left to right —
   the semantics a reader expects from a function call, unlike a naive macro
   that can duplicate its arguments. (A parameter used twice in the body binds
   a temporary rather than re-evaluating.) An unused parameter still
   evaluates, because a function would.
3′. **Elide the temporary for literals and true locals** — a **rule, not an
   optimisation** (spike s69). Binding every argument unconditionally is
   *slower than the call it replaces*: measured interpreted, `defn` **1.24×**,
   unconditional once-only **1.39×**, once-only + elision **0.98×**. Without
   this rule the premise of this ADR is false. Elide **only** for literals and
   symbols that resolve to locals — a symbol resolving to a *var* must be
   re-derefed, not snapshotted, or the semantics diverge from `defn`.
   Consequence: since elision splices the caller's symbols in bare, rule 2's
   body-local renaming becomes **mandatory** rather than belt-and-braces.
4. **Still a value when you need one.** `defexpand` also defines a real
   function under the same name, so `(map add! …)` / `(apply add! …)` /
   passing it to a HOF keep working; only *direct* call sites are expanded.
   This is the piece `defmacro` cannot give and the reason not to just use
   macros everywhere — **proven in s69**: there is one var per name, whichever
   of `defmacro`/`defn` runs last wins, and `(map dbl [1 2 3])` throws
   `wrong number of args (1) passed to: user/dbl`. Rule 4 is therefore
   *unimplementable* above the analyzer.
5′. **`with-redefs` still reaches an inlined site.** Each expansion is guarded
   — `(if (identical? f <pristine>) <body> (f …))` — in the ADR 0066
   dirty-flag shape, so `with-redefs`, `alter-var-root`, and **`cljx.test`'s
   `stub`/`spy` (ADR 0105) all keep working on a `defexpand`d fn**. Measured
   in s69: unguarded, a direct site does *not* see a redef; guarded, it does,
   and a spy observed both inlined sites. Rejected alternatives: "document the
   limitation" (a test that passes because the stub never fired is worse than
   a slow test) and "inline in AOT only" (REPL-vs-binary divergence, the
   unforgivable failure mode). The guard must be elidable, and ADR 0066's flag
   cannot be reused literally — it is one process-global `atomic.Bool` for
   seven arithmetic vars, so sharing it would let a redefined `+`
   de-optimise every `defexpand` in the program. Needs per-defexpand (or
   per-namespace) granularity; 0066's "once tripped, never clears" carries
   over.
5. **Dual-mode identical.** In the interpreter the expansion happens at
   analysis time; in AOT at compile time. Same semantics both ways (ADR 0002),
   dual-harness conformance-gated like everything else.
6. **Not a replacement for `defmacro`.** Full macros stay exactly as they are
   (ADR 0009 rule 1, fidelity). `defexpand` is for the "this should have been
   free" case; `defmacro` remains for genuine syntax (new binding forms,
   control flow, unevaluated arguments).

### The machinery lives in Go (owner, 2026-07-28)

**`defexpand` is a compiler feature and MUST be implemented in Go** — the
expansion, hygiene renaming and once-only argument binding belong in
`pkg/analyzer` (with `pkg/emit` cooperating where needed), **not** as a
Clojure-level macro in `core/core.clj`. Three reasons this is binding:

1. It follows the standing mandate that hot/algorithmic paths are Go host
   primitives under a thin Clojure surface (ADR 0097 mandate A) — expansion
   runs on every call site of every compile.
2. Only the analyzer sees the *call site*. A Clojure macro cannot distinguish
   a direct call from a value use, so the fn-fallback (rule 4) is
   unimplementable above the analyzer.
3. One implementation in the analyzer serves BOTH harnesses by construction,
   which is how the dual-mode guarantee (ADR 0002) stays true for free.

The Clojure side is only the `defexpand` surface form; everything that decides
*whether and how* to expand is Go.

**The carrier is `:inline` metadata — required, not merely "natural"** (s69).
`definline` already attaches it (`core/core.clj` ~2466) and **nothing has ever
read it**; the analyzer gaining a consumer *is* the change. s69 located the
seam precisely: `pkg/analyzer/parseInvoke` is the only place a call form
becomes an `OpInvoke`, and both harnesses share one Analyzer instance
(`pkg/eval/eval.go:80` builds it; `pkg/emit/compile.go:90` calls
`ev.Analyzer().Analyze`). The working prototype is **81 lines there and ZERO
lines in `pkg/emit` and `pkg/eval`** — which is why the ADR 0002 dual-mode
guarantee holds by construction rather than by discipline. Free side effect:
`definline` starts actually inlining.

### Where each tool lands (the guidance docs must lead with)

| you want | use | cost |
|---|---|---|
| compute a **value** once at build time | `comptime` (ADR 0009) | constant in the artifact |
| a **zero-cost function** | **`defexpand`** (this ADR) | expanded at the call site |
| genuine **new syntax** / unevaluated args | `defmacro` | expanded at the call site |
| everything else | `defn` | a normal call |

## Consequences

- **`cljx.core` becomes free**: `add!` / `bump!` / `del!` / `toggle!` become
  `defexpand`s that compile to the exact `swap!` form they document — sugar
  with no tax, which is the whole premise of the extensions tier (ADR 0106).
- **Users get the same power.** Anyone can write a zero-cost helper without
  learning macro ceremony — this is the differentiator ADR 0009 aims at,
  extended from values to functions.
- Requires analyzer work (expansion + hygiene renaming + once-only argument
  binding) and emitter cooperation; `:inline`-style metadata is the natural
  carrier, and making the analyzer honor it is the same machinery.
- Risk: expansion at every call site grows emitted code. Measured in s69
  **unguarded**: +1.2 emitted Go lines/site, ~231 B/site — +0.25 % of binary at
  50 sites, +1.71 % at 500. A non-event at `cljx`-shaped usage (small alias
  bodies, tens of sites); real for a big body pasted into hundreds. Keep
  `defexpand` bodies small (they are aliases by construction).
- **The guarded shape of rule 5′ is now measured** (2026-07-31,
  `spikes/s69-defexpand/gen-size.sh`, raw table in `evidence.txt`). Marginal
  cost per call site, 50 → 500 sites, each shape written out longhand and
  compiled for real:

  | shape | emitted Go | binary | vs `defn` |
  |---|---|---|---|
  | `defn` (baseline) | 13.0 lines | 330 B | — |
  | bare inline (R1–R4 + R1′) | 5.0 lines | 404 B | 0.38× lines, 1.22× bytes |
  | guarded inline (R5/5′) | 28.0 lines | 2128 B | 2.15× lines, **6.45× bytes** |

  R1′ elision pays (bare inlining emits 38 % of `defn`'s Go — the var-call
  preamble is bigger than the spliced body), and bare inlining costs 1.22×
  bytes/site, which the measured 1.013× vs 3.385× compiled speed buys back.
  The **per-site guard does not clear ADR 0004's size bar**: 6.45× the binary
  growth per site, +826 KB on a 7.05 MB binary at 500 sites (+11.7 %).
  **Therefore rule 5′'s guard is NOT emitted per call site by default.**
  `defexpand` inlines bare, and the guard is emitted only in the ADR 0066
  dirty-flag shape, where a process-global flag pays for redefinition
  visibility once instead of at every site. Openspec ratifies the guard on
  that basis, not on the per-site shape.

### v1 scope limits (s69, each currently a silent failure)

- **Self-recursion must be rejected at definition time**, with a depth-64
  budget as the backstop for the mutual case a local check cannot see. Today
  it is a raw Go `fatal error: stack overflow` with a goroutine dump — 30 s,
  2.66 GB RSS, no output — i.e. cljgo's stated unforgivable failure mode.
  `maxMacroExpansions` does not catch it (the recursion runs through
  `analyzeForm`, not the expansion loop).
- **Variadic is rejected in v1**, with a real diagnostic — today it fails with
  the confusing `unable to resolve symbol: &`. `(apply f a xs)` can never be
  expanded, so variadic is inherently "inline when literal, call otherwise": a
  strictly bigger design. JVM `definline` can't do it either. **Multi-arity IS
  supported** (demonstrated), selected per-arity via `:inline-arities`.
- **This bites `cljx.core` now** (ADR 0106): `add!`'s `& kvs` arity, `upd!`
  and `upd-in!` stay `defn` in v1; the fixed-arity shapes port.
- Arity errors gain `Expected`/`Found` per the CLAUDE.md error doctrine.
- Hygiene must share code with syntax-quote — the prototype walker does not
  understand destructuring, `letfn`, `catch` bindings, `for`/`doseq` vectors
  or `#()`.
- Expansion helpers must not be public vars in `clojure.core` (the prototype's
  `-defexpand-walk` etc. violate the precedence principle).

## Process

**Spike `spikes/s69-defexpand/` — DONE, 2026-07-28, verdict PROCEED.** Two
independent tracks off `92f6da7`: mechanism (`s69/mechanism` @ `6ec7e3d`, a
gate-green working prototype) and semantics (`s69/semantics` @ `aa53861`,
rules R1–R7 with runnable evidence). Consolidated in
`spikes/s69-defexpand/VERDICT.md`; rules 3′, 4 and 5′ above are its output.

The guarded-inline size number — the one measurement that was blocking the
proposal — was taken on 2026-07-31 (see Consequences). Next:
openspec → implement (~300 LOC + tests, single analyzer seam) → port the
fixed-arity `cljx.core` shapes (ADR 0106) onto it and re-measure to prove the
tax is gone.

Incidental defects s69 surfaced, to file separately: `load-file` fails on a
two-line file with `cannot call nil`; **core vars carry no metadata at all**
(`(meta (resolve 'min))` ⇒ `{}`) and user vars lack `:name`, defeating the
ordinary var-qualification idiom; `(.getMessage e)` fails on `ExceptionInfo`
where the JVM allows it; no `System/nanoTime`.
