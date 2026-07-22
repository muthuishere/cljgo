# s29 VERDICT — reify is feasible; implement as macro-over-builtin (→ ADR 0049)

## What was tried

`proto-closure.clj` hand-simulates reify: `(make-anon "Hi, ")` returns a value
holding `(fn [self] (str prefix "world"))` — an fn closing over the `let`/param
local `prefix` — and dispatch pulls that fn off the value and calls it with the
value itself as `this`. This isolates the two uncertain properties from the
already-proven protocol-table plumbing.

## What was measured

```
--- interpreted (cljgo run) ---
Hi, world
--- compiled (cljgo build + exec) ---
Hi, world      <- compile-time top-level eval (ADR 0002)
Hi, world      <- the native binary
```

Interpreted output == compiled output. Both properties hold:

1. **Closure capture survives AOT.** The emitter's existing fn-closure capture
   (`markCaptured`/`capturedParams` in pkg/emit/emit.go) emits the `let` local
   `prefix` into the Go closure. reify method bodies are ordinary `fn` forms,
   so they get this for free — no new emit path.
2. **Per-value dispatch works in both modes.** Selecting the impl off the value
   and applying it with the value as first arg (`this`) is byte-identical
   interpreted and compiled.

## Recommendation: IMPLEMENT (no owner decision reserved)

reify maps cleanly onto cljgo's macros-over-builtins protocol layer with **no
new AST op**, exactly like defprotocol/deftype/defrecord (design/00 §6; ADR
0043/0046). The design (→ ADR 0049):

- A new corelib value `*ReifyInstance` carries a per-instance impl table keyed
  by protocol identity → method name → fn, plus the declared protocol set (for
  `satisfies?`). It lives in pkg/corelib beside `Protocol` (dispatch is entirely
  a corelib concern); it prints via `fmt.Stringer` through `lang.ToString`.
- A new private builtin `-reify` builds it: `(-reify [P Q] P "m" (fn [this] ..)
  Q "n" (fn [this] ..))`. The method fns are plain `fn` forms, so they close
  over lexical locals in BOTH modes automatically.
- `reify` is a pure macro in core/protocols.cljg, grouping same-named method
  forms into one multi-arity fn (cljgo `fn` already dispatches multi-arity).
- Dispatch hooks: `-invoke-method` and `-satisfies?`/`satisfies?` check for a
  `*ReifyInstance` receiver FIRST (its own table wins) before the type-keyed
  path. `dispatchKey` gets a synthetic key so a reify never leaks into the
  per-type registry.

Because the compiled binary boots the SAME evaluator (`rt.Boot` → `eval.New`)
that interns `-reify` and loads protocols.cljg (via pkg/coreaot after regen),
a reify dispatches identically in the REPL, `cljgo run`, and a native binary —
the shared-dispatch guarantee that already holds for deftype.

Spike code stays here; it never merges into pkg/ (ADR 0027 §5).
