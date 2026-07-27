# VERDICT — s54 clojure.core.match native port

**SCOPED** (per ADR 0027).

A faithful *verbatim* port of `org.clojure/core.match` 1.1.1 does **not**
land in tier-1: the entire upstream engine is a tower of `deftype`s
implementing JVM host interfaces (`clojure.lang.ISeq`, `ILookup`, `IFn`,
`Associative`, `Indexed`, `IPersistentCollection`, `IObj`) plus
`clojure.lang.Compiler/LOOP_LOCALS`, `java.io.Writer`/`print-method`,
`Class/forName`, `.isArray`, and `IllegalArgumentException`/`AssertionError`
— none of which exist in cljgo. Confirmed by probe: `IllegalArgumentException`
and JVM-interface `deftype` do not resolve in cljgo.

**What DOES land in tier-1 (this first cut, verified):** a pure-Clojure
*linear* reimplementation with the SAME observable semantics for the common
pattern surface, exposing the exact public API (`match`, `matchv`, `matchm`,
`match-let`) and option keywords. **All 24 oracle behaviors reproduce
byte-for-byte under `cljgo run`** (see draft-conformance). Supported:

- literals (number / string / char / bool / keyword / nil / quoted sym|kw)
- wildcard `_`, binding symbols
- vector patterns, fixed and `&` rest
- seq patterns `(... :seq)`, fixed and `&` rest
- map patterns and `:only`
- `:or`, `:as`, `:guard`, `:when` (as a plain guard), app `:<<`
- arbitrary nesting, the trailing `:else` row, and the
  "No matching clause: <vals>" throw when nothing matches

## Honest deltas vs upstream (do not change any frozen return value)

1. **No decision-tree optimization.** Upstream builds a Maranget DAG that
   tests each occurrence once; this cut tests clauses top-to-bottom. Same
   results, more generated branches → a *performance* gap, not a correctness
   one. A future rung can re-introduce column selection.
2. **No redundancy / exhaustiveness / duplicate-wildcard warnings**, and no
   `*syntax-check*` diagnostics. Malformed rows fail differently (a plain
   compile error rather than upstream's `AssertionError` prose).
3. **`:when` has no `defpred` registry** — it is treated as a `:guard`
   (predicate applied to the occurrence). Upstream's non-overlap contract is
   not enforced. Behaviorally identical for ordinary predicates.
4. **The `&env` local-shadowing rule is not implemented.** Upstream: a
   pattern symbol that names a surrounding local is a *literal* equality
   test; here every non-`_` symbol is a fresh binding. This is the one place
   a program could observe a semantic difference; flagged as the top
   integration follow-up.
5. **Binding `:or` alternatives** (alternatives that bind) are scoped out;
   literal/wildcard alternatives are supported.
6. **`matchv` vector-type and `matchm` IMatchLookup** are accepted but inert
   (cljgo has one vector rep and map matching uses `map?`/`get`).
7. **Optional matcher namespaces** (`regex`, `date`, `java`, `array`,
   `binary`) are **not ported** — all Java-only upstream, matching the S50
   "5 Java ns are the OPTIONAL date/regex/java matchers — skip them" scope.

## Go host needs

**None.** The port is pure Clojure over builtins cljgo already ships
(`nth`, `count`, `vector?`, `map?`, `seq?`, `sequential?`, `get`,
`contains?`, `subvec`, `nthnext`, `seq`, `set`, `keys`, `gensym`, `ex-info`,
`ex-message`, and the reader/macro engine). No new `cljg.*` host fn is
required for this scope.

## Residual unknowns for the integrator

- Whether the `&env` local-shadowing rule (delta #4) is in-scope for the
  first shipped cut or an explicit documented divergence. Recommend
  documenting it as a known divergence initially (it needs macro `&env`
  access; confirm cljgo exposes `&env` to `defmacro`).
- AOT: this is a macro-only namespace (all expansion at compile time), so an
  AOT binary needs **no** runtime `clojure.core.match` code — but the AOT
  twin under `pkg/coreaot/clj*` must still exist so the loader census/gates
  agree with the interpreted path (see integration steps).
- Perf budget: if a budget is attached, the linear expansion will be slower
  than a hypothetical DAG on wide matches; set the budget against this cut,
  not upstream's tree.
