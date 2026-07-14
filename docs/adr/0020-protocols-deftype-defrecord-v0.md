# ADR 0020 — Polymorphism v0: defprotocol / deftype / defrecord as macros over a shared dispatch registry
Date: 2026-07-15 · Status: accepted (M5 stage: "deftype→struct + defprotocol→interface begin")

## Context
design/00 §6 M5 opens the polymorphism surface: `deftype→struct`,
`defprotocol→interface`. The unforgivable failure mode (design/00 §1, §2) is
REPL-vs-binary divergence. A Go-`interface`-per-protocol emission would give the
interpreter and the AOT emitter two DIFFERENT dispatch mechanisms to keep in
lockstep — exactly the divergence risk that M3.1 method calls avoided by having
BOTH modes share one reflective/registry path (ADR 0010).

## Decision
1. **v0 dispatch is a shared runtime registry, not a Go interface.** A protocol
   is a mutable value (`*eval.Protocol`) stored in a Var, holding
   `typeKey → method → fn`. Dispatch is: compute the first argument's dispatch
   key, look up the impl, apply it (or throw "No implementation of method …").
   The Go-interface target of design/00 stays a post-v0 emitter optimization; the
   registry is dual-mode-identical by construction and is the correct v0 rung.
2. **The whole surface is macros-over-builtins with ZERO new AST op.**
   `defprotocol/deftype/defrecord/extend-type/extend-protocol` are macros in
   `core/protocols.cljg` that expand onto private `-`-prefixed clojure.core
   builtins (`-protocol`, `-extend-key`, `-invoke-method`, `-new-type`,
   `-new-record`, `-map->record`, `-field`, `-type-key`, `-type-marker`,
   `-qualified-name`). Because the analyzer expands macros identically for both
   consumers and the AOT binary boots the SAME evaluator via `rt.Boot()` →
   `eval.New()` (which interns these builtins and loads protocols.cljg), a
   protocol dispatches byte-identically in the REPL and in a native binary. No
   `pkg/ast`, `pkg/analyzer`, `pkg/eval` dispatch, or `pkg/emit` change was
   needed — the layer rides existing OpInvoke/OpDef/OpFn.
3. **Instance representations** (`pkg/lang/instance.go`):
   - `*lang.DType` — a bare typed tuple (type name + positional fields), no map
     semantics, identity equality. Fields are visible as bare-symbol locals in
     method bodies (fields not shadowed by a method param) and readable via
     `(.-f x)` (GoFieldGet routes instances before reflection).
   - `*lang.Record` — an `IPersistentMap` (get/assoc/keys/vals/seq/count all work
     through the existing interface-dispatching runtime) that ALSO carries a type
     identity: `=` is by value AND type, a record is never `=` to a plain map in
     either direction (enforced in `Record.Equiv` and a one-line guard in
     `apersistentmapEquiv`), and it prints as `#ns.Name{:a 1, :b 2}` (a case in
     `lang.Print` ahead of the generic map branch). Ctors `->R` / `map->R`.
4. **Dispatch keys are strings.** deftype/defrecord instances carry their
   ns-qualified type name (baked at def/load time in the defining ns, so `->R`
   from another ns still keys correctly). Built-in Go values map to stable
   designator names (`String`, `Long`, `Double`, `Boolean`, `Keyword`, `nil`, …)
   that `extend-type`/`extend-protocol` resolve to the same string, so extending
   a protocol to a built-in type works.

## Consequences
- **Naming deviation, documented:** the JVM oracle extends built-ins by class
  (`java.lang.String`, `java.lang.Long`, `nil`); cljgo is Go-hosted and has no
  such classes, so the designators are the bare host names (`String`, `Long`,
  `nil`). BEHAVIOR is identical and conformance freezes the same output values
  the oracle produces.
- **Verified vs Clojure CLI 1.12.5:** protocol dispatch, deftype field access,
  record map semantics (`get`/`assoc`/`keys`/`count`), record `=` (value+type,
  and record≠map both ways), `->R`/`map->R`/printing, and `extend-type`/
  `extend-protocol` to deftypes, built-ins and nil — all byte-match the oracle
  and pass the dual harness (`conformance/tests/{protocol,deftype,defrecord,
  extend}-*.clj`).
- **v0 scope / deferred:** no Go-`interface` emission yet (registry is the shared
  mechanism); no `reify`; no protocol method arity checking beyond what the impl
  fn enforces; deftype printing (`#ns.Name[…]`) is a cljgo rendering (deftype has
  no reader form on the JVM either) so deftype conformance exercises behavior,
  not that literal; records-as-map-keys hashing is provided but lightly tested.
- Boot cost grows by the protocols.cljg load (ADR 0019); still within budget.
