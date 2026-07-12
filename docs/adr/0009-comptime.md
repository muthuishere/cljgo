# ADR 0009 — Clojure macros unchanged, plus Zig-style `comptime` for compile-time values
Date: 2026-07-12 · Status: proposed (owner-directed; design via OpenSpec, target post-M2)

## Context
Clojure macros are powerful but ceremonial: syntax-quote, unquote, gensyms,
returning *forms*. Zig showed a simpler model for a large class of use cases —
`comptime`: ordinary code executed at compile time, whose *value* is embedded
in the binary. cljgo is uniquely positioned to offer both: the AOT compiler
already links the tree-walk evaluator to run macros at compile time, so
compile-time execution costs nothing new. Owner mandate: keep Clojure macros
exactly as they are ("go like how it is"), add Zig's simplicity alongside.

## Decision
1. **Clojure macros stay 100% faithful.** No changes, ever, to `defmacro`
   semantics, expansion order, or hygiene. Fidelity priority 3 governs.
2. **Add `(comptime <body>)`**: body is ordinary Clojure evaluated ONCE at
   compile time by the same evaluator that runs macros; its **result value**
   (not a form) is embedded in the compiled artifact as a literal constant.
   - In interpreted/REPL mode, compile time = eval time: `comptime` evaluates
     inline with identical semantics (dual-mode consistency, ADR 0002).
   - The result must be *embeddable*: readable Clojure data (nil, booleans,
     numbers, strings, keywords, symbols, lists/vectors/maps/sets, nested).
     Fns, Go handles, channels, and other opaque values are a compile error
     with a positioned message.
3. **Companion forms** (same machinery, Zig-inspired, named at design time):
   `(comptime-assert <pred> <msg>)` — build fails if false;
   `(embed-file "path")` — file contents as a string/bytes constant at build
   time (the disciplined replacement for the `#=` read-eval we rejected).
4. Macros vs comptime guidance: macros transform *syntax* (control flow, new
   binding forms); `comptime` computes *values* (tables, parsed configs,
   precomputed data, environment-derived constants). Documentation leads with
   this split.

## Consequences
- The 80% case for "I need a macro" (precompute a value) drops to zero
  ceremony while full macros remain for the syntactic 20%.
- Emitted binaries can carry precomputed lookup tables and embedded assets
  with no runtime cost and no init-order concerns (constants in Load()).
- Requires an embeddability checker + literal emitter in pkg/emit — lands
  with/after M2 via an OpenSpec change (`/opsx:propose comptime`), which will
  settle naming, laziness interaction, and rebuild-invalidation (a comptime
  that reads a file must participate in build caching honestly).
- Differentiator vs every other Clojure: none of JVM/CLJS/Glojure/jank offer
  a value-level comptime with embedded-asset semantics.
