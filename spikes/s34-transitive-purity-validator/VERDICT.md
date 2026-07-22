# S34 VERDICT — one transitive walk yields both purity granularities

Closed 2026-07-22. Exit criterion **MET** (all four fixtures). Feeds **ADR 0054**
decisions 3 (whole-lib `publish clojars` gate) and 4 (per-namespace `use` gate).

## The question

Can ONE traversal of a library's transitive required-namespace surface classify
every reachable namespace as pure / Go-interop-tainted / (abstract) Java-tainted,
such that the SAME walk yields BOTH a whole-library publish gate and a
per-namespace use gate?

**Answer: YES.** One `emit.CompileProgram` call + one pass of pluggable
predicates produces a per-namespace class map; the whole-lib gate is the OR over
that map, the per-ns gate is a lookup in it. Same map, two readers.

## What was tried

A read-only driver (`driver.go`, ~430 lines, imports `pkg/emit` + `pkg/ast`,
modifies nothing under `pkg/`) that:

1. calls `emit.CompileProgram(entry)` — the **existing** ADR-0042 transitive
   file-backed-require traversal — ONCE per library;
2. runs N independent **taint predicates** over every captured `CompiledNS`
   exactly once: a concrete `goInteropPredicate` (keys off the five `OpHost*`
   AST ops, the same nodes `pkg/emit/hostfacts.go:collectHostPaths` keys on), an
   abstract `javaStubPredicate` standing in for **S35**'s `uses-java?`, and a
   no-op `ffiPredicate` future slot;
3. builds one `map[ns]->class`, then derives `wholeLibGate()` (OR) and
   `perNSGate(ns)` (lookup) from it.

Four throwaway fixtures under `fixtures/`, laid out `src/<prefix>/…` so
`ResolveLibPath` finds them: **pure** (2 ns), **go-buried** (`require-go` 2
levels deep under two pure ns), **mixed** (one pure + one Go sibling),
**java-abstract** (leaf flagged by the injected stub predicate).

## What was MEASURED (`results.txt`, verbatim `go run`)

| fixture | whole-lib (clojars) | per-ns split | class map |
|---|---|---|---|
| pure | **PASS** | pure.core PASS, pure.util PASS | both `pure` — **no false positive** |
| go-buried | **FAIL** citing `gob.leaf` at `…/gob/leaf.clj:3` | core/mid PASS, leaf FAIL | leaf `go-interop`, mid+core `pure` |
| mixed | **FAIL** citing `mix.goside:3` | pureside PASS, goside FAIL | split correctly |
| java-abstract | **FAIL** citing `jav.leaf` | core/mid PASS, leaf FAIL | leaf `java` — **distinct third class** |

Every fixture also passed the internal consistency check
`whole-lib == AND(per-ns over all reachable)`, proving the two gates read the
SAME classification rather than diverging.

## Findings against the brief

1. **Detection mechanism is real (Go-interop).** `require-go` surfaces in the
   analyzed AST as `OpHostRef`/`OpHostCall` (carrying the Go import `Pkg`), and
   dot-forms as `OpHostMethod`/`OpHostField`/`OpHostNew`. The predicate flags a
   namespace by the *presence of these nodes* in its captured forms — no
   re-compilation, no go/packages, no linking. `ffi`/`deflib`/`c-link` are **not
   AST ops in `pkg/` today** (S7/S21/S32 + future ADR 0011/0044); the pluggable
   design reserves a predicate slot for them without inventing one.

2. **Transitive burial: caught, and the existing traversal is reusable AS-IS.**
   `gob.leaf`'s `require-go` sits 2 requires below the entry (`core→mid→leaf`)
   and the walk flagged it. `CompileProgram` already captures every transitive
   file-backed namespace with its analyzed `Forms` — purity needs **no new
   walk**, only a pass over `Program.Deps + Program.Entry`. The central ADR 0052
   de-risk claim ("one traversal serves both legs") holds for purity too: the
   emitter's own traversal is the purity traversal.

3. **Both granularities from one walk: confirmed.** whole-lib gate = OR over the
   class map (refuse if any non-pure); per-ns gate = the named ns's own class.
   `use gob.core`/`gob.mid` PASS while `use gob.leaf` FAILS, from the identical
   map that FAILS the whole-lib clojars gate. Per-ns is deliberately the ns's
   *own* class — a dep's taint is gated when THAT dep is itself used.

4. **Classification keys a hard-error-not-nil (item 5, confirmed not
   re-litigated).** Two hard-error surfaces exist: (a) a namespace that fails to
   *analyze* — e.g. `(java.util.UUID/randomUUID)`, whose namespace resolves to
   neither a Clojure ns nor a `require-go` alias — makes `CompileProgram` return
   an error with **file:line** (`compiler error at …:1:11: no such namespace:
   java.util.UUID`), observed empirically; (b) a namespace that analyzes but is
   classified `go-interop`/`java` carries a `Finding` with `path:line`, which is
   exactly the datum a require-time gate raises instead of ADR 0052 §6a's silent
   `nil`. S34 does not fix §6a; it confirms the validator produces the
   file:line classification a hard-error would key off.

5. **No false positives (item 6).** The all-pure fixture classified 100% pure;
   whole-lib PASS, every per-ns PASS. The walk only flags on concrete `OpHost*`
   nodes (or an external predicate), never on ordinary Clojure.

## Empirical Java observations (handed to S35, not S34's verdict)

Recorded in passing (`cljgo run` on scratch fixtures), because they shape the
S35 predicate and ADR 0054 decision 4:

- `(java.util.UUID/randomUUID)` → **analysis error, file:line** (`no such
  namespace: java.util.UUID`, exit 1). A Java *static* call already hard-errors
  and makes the whole ns unanalyzable — the validator sees it as a
  CompileProgram error, not a classifiable node.
- `(import '[java.util Date])` → **analysis error** (`unable to resolve symbol:
  import`, exit 1) — cljgo has no `import`.
- `(.getBytes "hello")` → **runtime error** (`no method getBytes on string`,
  exit 1) — but structurally this is `OpHostMethod`, **indistinguishable from a
  Go dot-method** `(.ToUpper x)` at the AST level. This is the S35 crux: an
  instance-method dot-form cannot be statically told apart Java-from-Go by shape
  alone; only the two static/import forms are self-identifying. **For the
  clojars gate this ambiguity is harmless** — both are non-pure and refused. It
  matters only for `publish go`, which must still reject the Java one.

## Recommendation for ADR 0054

- **Decision 3 (whole-lib clojars gate): adopt.** Refuse `publish clojars` if
  any reachable namespace is non-pure. Implement it as a predicate pass over the
  `CompileProgram` capture — no second resolver, no new walk. For clojars the
  predicate set is "ANY host interop" (`OpHost*`) ∪ java ∪ ffi; it need not
  distinguish Go from Java, since clojars forbids both.
- **Decision 4 (per-ns use gate + Java third class): adopt, with a caveat named
  by S34 and owned by S35.** Per-ns gating from the same map works. The Java
  class must be a *distinct* class (it is, via the pluggable predicate) because
  `publish go` **allows** Go interop but must **reject** Java. The caveat:
  dot-form instance methods are Go/Java-ambiguous at the AST level, so the S35
  `uses-java?` predicate cannot rely on dot-form shape alone — the two
  self-identifying Java surfaces (dotted-static-call, `import`) already
  hard-error at analysis and are the reliable signals; a bare `(.foo x)` is not.
- **Make taint predicates pluggable** (go-interop, java, ffi as independent
  predicates over one walk) rather than a hardcoded classifier — proven to
  compose cleanly here.

## Exit criterion met? **YES** — all four fixtures, one walk each, both gates
derived from one map, transitive burial caught, zero false positives, Java
surfaced as a distinct third class via the abstract predicate.

## Reproduce

```
go run ./spikes/s34-transitive-purity-validator            # all fixtures
go run ./spikes/s34-transitive-purity-validator -fixture go-buried
```

Read-only against `pkg/`; nothing here merges (ADR 0027 §5). `driver.go`'s
child-walker is copied/adapted from `pkg/emit/emit.go:eachChild`.
