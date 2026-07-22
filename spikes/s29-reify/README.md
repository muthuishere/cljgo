# Spike s29 — reify (anonymous protocol-satisfying values)

## Question

`(reify P (m [this] body) Q (n [this] body))` creates an anonymous value
that satisfies the named protocols, closing over its lexical environment
(no named type, no fields — it captures the enclosing locals). On the JVM it
is an anonymous class. HOW does an anonymous protocol-satisfying value fit
cljgo's type/protocol system in BOTH the interpreter (pkg/eval) AND the AOT
emitter (pkg/emit), given that:

1. cljgo's protocol dispatch keys on TYPE (`dispatchKey(v)` → a string, then
   `Protocol.lookup(typeKey, method)`). reify has NO named type and needs a
   PER-INSTANCE (per-form) method table, not a per-type one.
2. reify closes over lexical locals. The interpreter captures them as an fn
   closure; the AOT emitter must emit a Go closure of the captures.
3. It must dispatch byte-identically compiled == interpreted (the release
   blocker bar).

## Exit criterion (written BEFORE any code)

The smallest prototype proving ONE protocol reify works in BOTH modes:
a value whose protocol method (a) dispatches to the reify's own body, not a
type-keyed impl, and (b) closes over an enclosing `let` local, producing the
SAME output run interpreted (`cljgo run`) and compiled (`cljgo build` + exec).
If the closure-capture-through-AOT + per-value dispatch both hold, the
mechanism is proven and the design maps cleanly onto the existing
macros-over-builtins protocol layer (no new AST op). If either fails, reify
needs a design decision reserved for the owner and this spike closes with a
STOP recommendation.

## Files

- `proto-closure.clj` — the throwaway prototype. It hand-simulates reify with
  the primitives reify would use: an fn closing over a `let` local, stored in
  a value, dispatched by pulling the impl off the value and calling it with
  the value as `this`. This isolates the two genuinely uncertain properties
  (closure-capture-survives-AOT, per-value dispatch) from the mechanical
  protocol-table plumbing that deftype/defrecord already prove.

## Result

See VERDICT.md.
