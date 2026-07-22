# ADR 0049 — `reify`: anonymous protocol-satisfying instances

Date: 2026-07-22 · Status: accepted · Evidence: spike s29 (`spikes/s29-reify/`)

## Context

`reify` is a core clojure.core form used constantly in real Clojure:
`(reify P (m [this a] body) Q (n [this] body))` produces an anonymous value
that satisfies the named protocols, its method bodies closing over the
enclosing lexical environment. Unlike `deftype`/`defrecord` it has NO named
type and NO declared fields — it captures the surrounding locals. On the JVM
it compiles to an anonymous class implementing the protocol interfaces.

cljgo's polymorphism (ADR 0043/0046; core/protocols.cljg + pkg/corelib/
protocols.go) is macros-over-builtins with NO new AST op, and its protocol
dispatch keys on TYPE: `dispatchKey(v)` → a string, then
`Protocol.lookup(typeKey, method)`. reify breaks the "keyed by type"
assumption twice: it has no type name, and two reify forms of the same
protocols need DIFFERENT method bodies — the impl table must be
per-instance, not per-type.

Spike s29 proved the two genuinely uncertain properties in both modes:
closure-capture-of-lexical-locals survives AOT emission (the emitter's
existing fn-closure capture handles it, because reify method bodies are plain
`fn` forms), and selecting an impl off a value and calling it with the value
as `this` dispatches byte-identically interpreted and compiled.

## Decision

Implement `reify` as a **pure macro over one new private builtin** — same
shape as defprotocol/deftype, **no new AST op**.

1. **`*ReifyInstance` (pkg/corelib)** — a new runtime value beside
   `Protocol`. It carries a per-instance impl table keyed by protocol
   identity → method name → `lang.IFn`, and the set of declared protocols
   (for `satisfies?`). It prints via `fmt.Stringer`
   (`#reify[user.Greet user.Sizey]`) through `lang.ToString`. Dispatch is a
   corelib concern, so the value lives in corelib, not pkg/lang.

2. **`-reify` builtin** — `(-reify [P Q] P "m" fn P "m2" fn Q "n" fn)`:
   arg 0 is the declared-protocol vector; the rest are
   `protocol method-name-string fn` triples. It builds the `*ReifyInstance`.

3. **`reify` macro (core/protocols.cljg)** — groups the tail's protocol-slot
   symbols + method forms (reusing `-group-impls`), collapses same-named
   method forms under one protocol into a single **multi-arity** `fn` (cljgo
   `fn` already dispatches multiple arities), and emits the `-reify` call.
   The method fns are plain `fn` forms, so they close over lexical locals in
   BOTH the interpreter (fn closure) and the emitter (emitted Go closure) with
   no special handling. The method's first parameter is `this`, bound to the
   instance itself at dispatch — exactly the protocol calling convention.

4. **Dispatch hooks** — `-invoke-method`, `-satisfies?` and the public
   `satisfies?` check for a `*ReifyInstance` receiver FIRST: its own table
   wins over the type-keyed registry. `dispatchKey` maps a `*ReifyInstance`
   to a synthetic `"reify"` key so it can never pollute the per-type impl map.
   Calling a protocol method the reify does not implement raises the same
   "No implementation of method" error as any other value.

Because the AOT binary boots the SAME evaluator (`rt.Boot` → `eval.New`) that
interns `-reify` and loads protocols.cljg (through pkg/coreaot after regen), a
reify dispatches identically in the REPL, `cljgo run`, and a native binary —
the shared-dispatch guarantee that already holds for deftype/defrecord.

## Consequences

- reify satisfies the precedence principle: it is real clojure.core, matched
  to JVM semantics (multiple protocols, multi-arity methods, `this`, closing
  over locals, `satisfies?`), adding nothing that shadows or renames.
- No new AST op, no new emit path; the risk surface is a corelib value + one
  builtin + a macro, all exercised by the dual-harness conformance tests
  `conformance/tests/reify-*.clj`.
- pkg/coreaot is regenerated (protocols.cljg is a boot source).
- Out of scope (as on the current deftype path, and matching what cljgo does
  not yet expose): reifying bare Java/Object interface methods like
  `toString`/`Object`. reify targets cljgo protocols. Java-interface reify is
  a separate future decision.
