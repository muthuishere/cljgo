# Spike S34 — One transitive walk that classifies every reachable namespace by purity

Opened 2026-07-22. Feeds **ADR 0054** (decisions 3 and 4 — publish gates).
Rides on **ADR 0042** (multi-namespace emission) and **ADR 0052** (§6 purity
axis, §6a nil-divergence blocker). Follows the ADR 0027 lifecycle: this README
and its exit criterion are written **before any code**.

## Scope (narrowed 2026-07-22, coordinator)

The hard **Java-detection primitive** — deciding whether a namespace uses Java
host interop — is owned by a dedicated spike **S35** and is explicitly OUT of
S34's scope. S34 treats Java taint as an **abstract third class** supplied by an
external, pluggable predicate `uses-java?(ns)`; S35 proves that predicate. S34's
job is the WALK and the GRANULARITY, with a **pluggable taint-predicate** design
(go-interop, java, ffi as independent predicates over the same walk), and a
concrete, working **go-interop** predicate as the exemplar.

Empirical Java observations seen in passing are recorded (they inform S35), but
S34's verdict does not rest on distinguishing a Java method-call from a Go one.

## Context — the verified current state

- `pkg/emit/module.go` `CompileProgram(srcPath)` already **walks the transitive
  file-backed required-namespace surface**: it installs `moduleCompiler.load`
  as the evaluator's `LibLoader`, so every `(require …)` that resolves to a
  file (`pkg/eval/libload.go` `ResolveLibPath`) is compiled through the same
  analyze-and-eval pipeline and captured as a `CompiledNS{Name, Path, Forms,
  Requires}`. The result `Program{Entry, Deps}` holds **every reachable
  file-backed namespace, dependency-first**, each with its analyzed forms.
  This is the traversal S34 must reuse rather than reinvent.
- Go host-interop appears in the analyzed AST as five node ops:
  `OpHostRef` / `OpHostCall` (a `require-go` alias member, carrying the Go
  import `Pkg`), and `OpHostMethod` / `OpHostField` / `OpHostNew` (dot-form
  interop on a Go value, receiver-based, no `Pkg`). `pkg/emit/hostfacts.go`
  `collectHostPaths` already walks the same forms for `OpHostRef`/`OpHostCall`
  to pre-load go/packages facts — proof the emitter treats these ops as the
  interop markers.
- `require-go` is the only concrete Go-dep marker in `pkg/` today. `ffi` /
  `deflib` / `c-link` are **not yet AST ops** (they are S7/S21/S32 spike-level
  and future ADR 0011/0044 surface). So the go-interop predicate keys off the
  `OpHost*` nodes; `ffi` is designed-for as a future predicate slot, not
  implemented.

## The one question

**Can ONE traversal of a library's transitive required-namespace surface
classify every reachable namespace as pure-Clojure / Go-interop-tainted /
(abstract) Java-tainted, such that the SAME walk yields BOTH (a) a whole-library
gate for `publish clojars` (refuse if ANY reachable namespace is non-pure) and
(b) a per-namespace gate for plain `use` (only the tainted namespace fails)?**

## Exit criterion (written before any code, per ADR 0027)

Four throwaway fixture libraries, each an entry namespace plus file-backed
requires, driven through `emit.CompileProgram` + a pluggable-predicate walk:

1. **pure** — entry `pure.core` requires `pure.util`, both pure Clojure. The
   walk classifies **100% pure**: whole-lib clojars gate PASSES, every
   per-namespace gate PASSES. **No spurious taint** (item 6).
2. **go-buried** — entry `gob.core` is itself pure Clojure but transitively
   requires `gob.mid` which requires `gob.leaf`, and **`gob.leaf` (2 levels
   deep) contains a `require-go` + a Go host-call**. The walk classifies
   `gob.leaf` **Go-tainted** while `gob.core` and `gob.mid` stay pure. ⇒
   whole-lib clojars gate FAILS naming `gob.leaf` (item 3 — transitive burial);
   a per-namespace `use gob.core` / `use gob.mid` still PASSES, `use gob.leaf`
   FAILS (item 4 — both granularities, one walk).
3. **mixed** — entry `mix.core` requires two siblings `mix.pureside` (pure) and
   `mix.goside` (Go host-call). The walk classifies `mix.pureside` pure and
   `mix.goside` Go-tainted **independently** — per-namespace `use mix.pureside`
   PASSES while the whole-lib clojars gate FAILS (item 4).
4. **java-abstract** — a fixture whose `jav.leaf` is flagged by an injected
   stub `uses-java?` predicate (standing in for S35). The walk surfaces it as a
   **distinct third class**, not merged into Go-taint and not silently dropped
   (item 5 — the classification is what a hard-error keys off).

The criterion is MET iff, from a **single** `CompileProgram` walk per fixture:

- pure ⇒ whole-lib PASS, all per-ns PASS (no false positive);
- go-buried ⇒ whole-lib FAIL citing `gob.leaf` at a file:line, per-ns FAIL only
  for `gob.leaf`, PASS for `gob.core`/`gob.mid`;
- mixed ⇒ whole-lib FAIL, per-ns split pure/tainted correctly;
- java-abstract ⇒ third class reported distinctly via the pluggable predicate;
- and the whole-lib and per-ns answers are **derived from the same
  classification map** (whole-lib = OR over the per-ns map), not two walks.

Anything less — e.g. burial 2 levels deep is missed, or the two granularities
need separate traversals, or a pure lib shows spurious taint — closes S34 **no**
and forces ADR 0054 to redesign decisions 3/4.

## What must additionally be reported

1. Is `CompileProgram`'s transitive traversal reusable as-is for purity, or
   does purity need its own walk? (item 3)
2. Does the classification key a **hard-error-not-nil** at require time, and how
   does that relate to ADR 0052 §6a? (item 5 — confirm, do not re-litigate)
3. What did the Java fixture actually DO when analyzed by cljgo (for S35)?
