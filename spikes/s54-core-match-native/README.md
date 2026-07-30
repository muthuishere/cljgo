# s54 — clojure.core.match, natively (ADR 0096)

Prove a faithful native port of `org.clojure/core.match` (1.1.1) into cljgo.
Everything here is a **draft** under the spike dir — nothing is wired into
`core/`, `pkg/`, the loader, the AOT twin, or the shared conformance harness
this round.

## What I did

1. Resolved the real jar with the Clojure CLI
   (`~/.m2/repository/org/clojure/core.match/1.1.1/core.match-1.1.1.jar`) and
   read every source file.
2. Captured **authoritative oracle output** by running 24 representative
   `match` / `match-let` forms through the real library on the JVM
   (Clojure 1.12.5). Those values are the frozen truth in
   `draft-conformance-core.match.clj`.
3. Wrote a cljgo-native `clojure.core.match` (`draft-core_match.cljg`) and
   verified all 24 behaviors reproduce the oracle **byte-for-byte** under
   `cljgo run`.

## The pure / Java split (upstream 1.1.1)

| upstream namespace | nature | this port |
|---|---|---|
| `clojure.core.match` (the macro + engine, 2156 lines) | **Java-coupled** — `deftype`s implementing `clojure.lang.{ISeq,ILookup,IFn,Associative,Indexed,IPersistentCollection,IObj}`, `clojure.lang.Compiler/LOOP_LOCALS`, `java.io.Writer`/`print-method`, `Class/forName`, `.isArray`, `IllegalArgumentException`/`AssertionError` | **rewritten** (see below) |
| `clojure.core.match.protocols` | pure protocols/definterface, but only meaningful with the deftype tower | folded away (not needed by the linear compiler) |
| `clojure.core.match.regex` | Java — `java.util.regex.Pattern` matcher | **scoped out** |
| `clojure.core.match.date` | Java — `java.util.Date`/`Calendar` | **scoped out** |
| `clojure.core.match.java` | Java — bean reflection | **scoped out** |
| `clojure.core.match.array` | Java — primitive-array specialization | **scoped out** |
| `clojure.core.match.binary` | Java — `clojure.core.match.protocols` + byte arrays | **scoped out** |
| `clojure.core.match.debug` / `bench` | dev-only | **scoped out** |
| `cljs.core.match.*` | ClojureScript twin | n/a |

So **zero upstream namespaces port verbatim.** The main namespace's *user-
visible semantics* are what we reproduce; its *implementation* (a
Maranget decision-tree DAG built from host-interface deftypes) is not
portable to cljgo and is re-derived.

## Verbatim vs rewritten

- **Rewritten (100% of the engine):** the pattern representation is no longer
  a deftype tower. `draft-core_match.cljg` is a **linear pattern compiler** —
  each clause compiles, in source order, to a nested test/bind expression
  that yields the clause action or tail-calls the next clause's fail
  continuation (a zero-arg `fn`, so no code duplication and no
  backtracking-exception machinery). Same inputs → same return values; the
  decision-tree *optimization* and the redundancy/exhaustiveness *warnings*
  are the only things traded away, and neither changes a result.
- **Preserved exactly:** the public surface — `match`, `matchv`, `matchm`,
  `match-let` — plus the pattern syntax and option keywords `:seq`, `:or`,
  `:as`, `:guard`, `:when`, `:<<`, `:only`, `:else`, `&`, `_`, quoted
  literals — and the "No matching clause: <vals>" failure message.

## Files

- `draft-core_match.cljg` — the native port (satellite `.cljg` shape:
  opens `(clojure.core/in-ns 'clojure.core.match)`, refers core).
- `draft-conformance-core.match.clj` — 23 frozen behaviors (one combined
  vector) in the conformance freeze format, each value copied from the JVM
  oracle run.
- `VERDICT.md` — SCOPED, with the honest tier-1 scope and residual unknowns.

## Reproduce the verification

```
# oracle (authoring-time truth):
clojure -Sdeps '{:deps {org.clojure/core.match {:mvn/version "1.1.1"}}}' -M /tmp/oracle.clj

# port (strip the in-ns header into a user-ns copy, append the driver, run):
cljgo run <user-ns copy of draft-core_match.cljg + the conformance vector>
# => byte-identical to the frozen expect.
```
