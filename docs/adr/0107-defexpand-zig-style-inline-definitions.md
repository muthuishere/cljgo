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
   **1.17–1.28× vs hand-written `swap!` interpreted** (compiled, the emitter
   already erases most of it). Sugar that taxes you is not sugar.
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
   a temporary rather than re-evaluating.)
4. **Still a value when you need one.** `defexpand` also defines a real
   function under the same name, so `(map add! …)` / `(apply add! …)` /
   passing it to a HOF keep working; only *direct* call sites are expanded.
   This is the piece `defmacro` cannot give and the reason not to just use
   macros everywhere.
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
*whether and how* to expand is Go. Existing carrier: `:inline` metadata is
already attached by `definline` (`core/core.clj` ~2466) and consumed by
nothing — the analyzer gaining a consumer is the change.

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
- Risk: expansion at every call site grows emitted code. Mitigate by keeping
  `defexpand` bodies small (they are aliases by construction) and measuring
  binary size against the ADR 0004 budget in the spike.
- Interaction to settle at design time: recursion (must be rejected or
  bounded — a self-expanding body would not terminate), variadic parameters,
  and how a `defexpand` behaves under `with-redefs` (the function fallback
  makes redefinition observable; direct call sites are already inlined —
  the spike must measure and the design must state the honest rule).

## Process

Spike (`spikes/s69-defexpand/`) proving expansion + hygiene + once-only
evaluation + the HOF fallback in BOTH harnesses, with code-size and speed
numbers vs `defn` and `defmacro`; the `with-redefs` interaction is the one
real semantic risk. Then openspec, then implement, then port `cljx.core`
(ADR 0106) onto it and re-measure to prove the tax is gone.
