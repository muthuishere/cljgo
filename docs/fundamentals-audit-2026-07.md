# Fundamentals audit — 2026-07

**Read-only audit. No fixes in this change.** The jank clojure-test-suite
scoreboard (238/242 files, `cljgo suite`) only tests what jank happens to
test. This document measures cljgo's *actual* clojure.core (+ friends)
surface against the real oracle and classifies every gap.

## Method

1. **Ground truth** — `clojure -M -e '(doseq [s (sort (keys (ns-publics
   (quote <ns>))))] (println s))'` against real Clojure CLI 1.12.5
   (`clojure --version` confirmed) for `clojure.core`, `.string`, `.set`,
   `.edn`, `.walk`, `.zip`, `.data`, `.repl`, `.pprint`, `.java.io`, `.test`.
2. **cljgo's actual surface** — cljgo has no `ns-publics`/`ns-map` (that's
   itself a gap, see below), so the surface was measured directly off the
   live registry: a temporary Go test (`pkg/eval/zzz_audit_dump_test.go`,
   deleted before commit — not part of this PR's diff) called `eval.New()`
   to boot, then walked `lang.AllNamespaces()` → `ns.Mappings()`, printing
   every var owned by its namespace with `IsPublic()`/`IsMacro()`/`HasRoot()`
   flags. This is the live interned registry, not docs.
3. **Diff** — `comm -23` (oracle − cljgo) per namespace, `LC_ALL=C` sort
   (default locale collates `-`/`*`/`?` differently from Clojure's symbol
   sort and silently corrupts the diff — hit and fixed during this audit).
4. **Sanity probes** — every "present" item called out by the task brief
   was `cljgo run` against real inline snippets (not just presence-checked):
   `for`, `defmulti`/`defmethod`/`methods`, `condp`, `case`, `dotimes`,
   `get-in`/`assoc-in`/`update-in`, `select-keys`, `every-pred`, `flatten`,
   `frequencies`, `partition-all`, `split-at`, `zipmap`, `sort-by`,
   `re-seq`, `bit-shift-left`, `char-array`, `juxt`, `partial`, `cycle`,
   `iterate`, `repeatedly`, `reduce-kv` — **all produced correct, real-Clojure-
   matching output.** No "present but broken" cases were found among these.
5. **Macro-specific pass** — filtered the dump to `:macro` vars and diffed
   against real Clojure's macro set, since a missing macro doesn't surface as
   a runtime error in an untested program — it just silently doesn't exist.

## Headline numbers

| namespace | oracle vars | cljgo has | missing | % covered |
|---|---:|---:|---:|---:|
| `clojure.core` | 679 | 439 (of 522 total; 83 are cljgo-only helpers/renames, see below) | **240** | 65% |
| `clojure.string` | 21 | 20 | 1 | 95% |
| `clojure.set` | 12 | 12 | **0** | 100% |
| `clojure.edn` | 2 | 1 | 1 | 50% |
| `clojure.repl` | 13 | 2 | 11 | 15% |
| `clojure.test` | 39 | 22 | 17 | 56% |
| `clojure.walk` | 10 | 0 | 10 | **0%** |
| `clojure.zip` | 28 | 0 | 28 | **0%** |
| `clojure.data` | 5 | 0 | 5 | **0%** |
| `clojure.pprint` | 26 | 0 | 26 | **0%** |
| `clojure.java.io` | 19 | 0 | 19 | out of scope (C) — JVM I/O, expected absent |

cljgo's `clojure.core` also carries **~83 entries with no oracle
counterpart**: ~50 internal helper fns whose names start with `-`
(`-for-expand`, `-case-emit`, `-defmulti`, …, clearly not meant to be public
API but currently interned as public — a hygiene issue, not a fundamentals
gap, noted but out of scope here) plus the intentional ADR 0014-style renames
(`just`/`none`/`ok`/`err`/`unwrap`/`unwrap-or`/`option?`/`result?`/`and-then`/
`map-ok`/`map-err`/`err?`/`ok?`) and channel/agent primitives (`chan`, `go`,
`<!`, `>!`, `alts!`, …) that are real Clojure-adjacent (core.async-shaped)
but not `clojure.core` names in real Clojure.

## Multimethods and `for` — both confirmed present and working

- **Multimethods: yes.** `defmulti`/`defmethod` are macros in `core/core.clj`
  expanding to `-defmulti`/`-defmethod` (`pkg/corelib/multimethod_builtins.go`).
  `methods`, `get-method`, `remove-method` all exist and work (probed: a
  `:circle`/`:default` dispatch resolved correctly, `methods` returned the
  dispatch-val→fn table). **`prefer-method` and `remove-all-methods` are
  missing** (confirmed by comment at `multimethod_builtins.go:23`, "No
  isa?/hierarchy/prefer-method" — stale re: hierarchy, since `isa?`/`derive`/
  `ancestors`/`parents`/`descendants`/`make-hierarchy` *do* exist via
  `core/hierarchies.cljg`, but accurate re: `prefer-method`, which is a real
  gap for the rare case of ambiguous multiple dispatch).
- **`for`: yes**, a real macro (`core/core.clj:1192`), confirmed to produce
  correct nested-binding cross-products (probed: `(for [x [1 2 3] y [10 20]]
  (+ x y))` → `(11 21 12 22 13 23)`, matches real Clojure).

## Top-10 most damning `clojure.core` gaps (A-list, by real-world frequency)

Frequency judgment: these are idioms that show up in almost any
non-trivial Clojure codebase (per owner's and general community usage —
no formal corpus survey run; flagged where jank suite / conformance both
miss the gap, which is *why* 98.3%/238-of-242 didn't catch it).

| # | gap | why it matters | suite coverage |
|---|---|---|---|
| 1 | `reify` | anonymous protocol/interface impl — a core idiom once `defprotocol`/`extend-protocol` exist (both present); its absence is the sharpest asymmetry in the surface | not exercised by jank suite or `conformance/tests/` |
| 2 | `with-open` | deterministic resource cleanup (the `try/finally` + `.close` pattern) — completely absent, no oracle-equivalent found | not covered |
| 3 | `memoize` | the single most common caching idiom in idiomatic Clojure | not covered |
| 4 | `letfn` | local mutually-recursive fn bindings — a core `core.clj`-of-Clojure idiom for helper fns inside a function body | not covered |
| 5 | `trampoline` | the standard non-stack-growing mutual-recursion escape hatch | not covered |
| 6 | `declare` / `defonce` / `defn-` | basic def-family: forward declarations, singleton init, private fns — used in nearly every multi-fn namespace | not covered |
| 7 | `with-redefs` (+ `with-redefs-fn`) | the standard test-time stubbing macro — notable because `clojure.test` itself is partially present (`deftest`/`is`/`testing`) but its most common companion macro isn't | not covered |
| 8 | `slurp` / `spit` | trivial file-read/write convenience wrappers — likely deferred pending the Go-interop I/O story (design/05), but real programs reach for these constantly | not covered |
| 9 | `split-with` | `split-at` (index-based) exists but its predicate-based sibling doesn't — an easy, cheap, asymmetric gap | not covered |
| 10 | `clojure.walk` (whole namespace: `postwalk`/`prewalk`/`walk`/`keywordize-keys`/`stringify-keys`) | tree-transform primitives used constantly for macro-writing, EDN/JSON post-processing, and generic data massaging | not covered (whole namespace absent, so no test could exercise it) |

Honorable mentions just below the top 10: `reductions`, `tree-seq`,
`line-seq`, `lazy-cat`, `update-vals`/`update-keys` (Clojure 1.11+, common
map-transform idiom), core's own `read-string`/`read`/`read-line` (note:
`clojure.edn/read-string` *does* exist and works — it's the general reader
`clojure.core/read-string` that's missing), `future?` (the only member of
the `future`/`future-call`/`future-cancel`/`future-cancelled?`/
`future-done?` family missing), `vary-meta`, `distinct?`, `comparator`,
`bounded-count`, `all-ns`/`the-ns`/`ns-name` (meta-programming — and the
proximate cause this audit had to shell out to Go instead of running
`(ns-publics 'clojure.core)`, since `ns-publics`/`ns-map`/`ns-interns`/
`ns-refers`/`ns-imports`/`ns-aliases`/`ns-unmap`/`ns-unalias` are *all*
missing too).

## Full classification

### A — fundamental, expected by real programs (missing)

Grouped by shape, all confirmed absent from the live registry:

- **Control-flow / binding macros**: `letfn`, `with-open`, `with-redefs`,
  `with-redefs-fn`, `declare`, `defonce`, `defn-`, `locking`, `time`,
  `trampoline` (fn), `lazy-cat` (macro)
- **Seq/collection fns**: `reductions`, `tree-seq`, `line-seq`, `split-with`,
  `update-keys`, `update-vals`, `distinct?`, `bounded-count`, `vary-meta`,
  `future?`
- **Functional idioms**: `memoize`, `reify`
- **I/O convenience**: `slurp`, `spit`
- **Reader**: `read`, `read-string`, `read-line`
- **Whole namespace**: `clojure.walk` (`postwalk`, `prewalk`, `walk`,
  `postwalk-replace`, `prewalk-replace`, `keywordize-keys`,
  `stringify-keys`, `postwalk-demo`, `prewalk-demo`) — 10/10 missing
- **Multimethod completeness**: `prefer-method`, `remove-all-methods`
- **Introspection needed for tooling** (borderline A/B — flagged A because
  their absence blocked this very audit): `ns-publics`, `ns-map`,
  `ns-interns`, `ns-refers`, `ns-imports`, `ns-aliases`, `ns-name`, `all-ns`,
  `the-ns`

Count: **~45 A-list items** (core) + 10 (`clojure.walk`, counted above).

### B — niche but real (missing)

- **REPL/dev tooling** (`clojure.repl`, 11/13 missing): `apropos`, `dir`,
  `dir-fn`, `find-doc`, `pst`, `root-cause`, `source`, `source-fn`,
  `demunge`, `stack-element-str`, `set-break-handler!`, `thread-stopper`
  (`doc`, `print-doc` are the only two present)
- **`clojure.pprint`** — whole namespace absent (26/26): `pprint`,
  `pp`, `print-table`, `cl-format`, `write`, etc. — dev convenience, not
  program logic
- **`clojure.zip`** — whole namespace absent (28/28): zipper-based tree
  editing (`zipper`, `up`/`down`/`left`/`right`, `edit`, `root`, …) — real
  but a specialist tool, not everyday code
- **`clojure.data`** — whole namespace absent (5/5): `diff` and friends
- **`clojure.test` internals** (17/39 missing, beyond the 22 that work):
  `test-ns`, `run-test-var`, `with-test`, `try-expr`/`assert-expr`
  (the multimethod extension point for custom `is` forms), `file-position`,
  `*load-tests*`, `*test-out*`/`with-test-out` — cljgo's own `cljgo test`
  command may intentionally replace `test-ns`-style namespace running
  (ADR-worthy question, not this audit's call), but `with-test` and the
  `assert-expr` extension point are real gaps for anyone porting real test
  suites.
- **Concurrency/agent lifecycle**: `await-for`, `await1`, `agent-errors`,
  `clear-agent-errors`, `error-handler`, `error-mode`, `set-error-handler!`,
  `set-error-mode!`, `release-pending-sends`, `send-via`,
  `set-agent-send-executor!`, `set-agent-send-off-executor!`,
  `shutdown-agents`, `ref-history-count`, `ref-min-history`,
  `ref-max-history`, `ensure`, `sync` — `agent`/`send`/`send-off`/
  `restart-agent` work, but the full lifecycle/tuning surface is absent.
- **Printer/reader extension points**: `*print-dup*`, `*print-meta*`,
  `*print-namespace-maps*`, `*print-level*`, `print-dup`, `print-method`,
  `print-simple`, `print-ctor`, `PrintWriter-on`, `*reader-resolver*`,
  `*default-data-reader-fn*`, `*read-eval*`, `*suppress-read*`,
  `tagged-literal`, `tagged-literal?`, `reader-conditional`,
  `reader-conditional?`
- **Numeric/array coercion niceties**: `unchecked-byte/char/short/int/long/
  float/double/negate-int/dec-int/inc-int`, `byte-array`, `short-array`,
  `bytes`/`shorts`/`booleans`/`chars`/`doubles`/`floats`/`ints`/`longs`
  (type-hint coercion helpers — cljgo already has `int-array`/`long-array`/
  `float-array`/`double-array`/`boolean-array`/`char-array`/`object-array`
  working; the gap is the smaller/rarer element types plus the `bytes?`-
  style predicates), `amap`, `areduce` (array-comprehension macros)
- **Hashing internals** (used when hand-rolling `equals`/`hashCode`-style
  semantics): `hash`, `hash-combine`, `hash-ordered-coll`,
  `hash-unordered-coll`, `mix-collection-hash`
- **Misc**: `bean`, `vector-of`, `partitionv`, `partitionv-all`,
  `splitv-at`, `pmap`, `pcalls`, `pvalues`, `iteration`, `seque`,
  `requiring-resolve`, `comparator`, `inst?`/`inst-ms`/`inst-ms*`,
  `uri?`, `bytes?`, `munge`, `namespace-munge`, `char-escape-string`,
  `char-name-string`, `io!`, `memfn`, `..`, chunked-seq internals
  (`chunk`, `chunk-append`, `chunk-buffer`, `chunk-cons`, `chunk-first`,
  `chunk-next`, `chunk-rest`, `chunked-seq?`)
- `clojure.string/re-quote-replacement` (the one `clojure.string` gap)
- `clojure.edn/read` (the one `clojure.edn` gap — `read-string` is present
  and works)

### C — JVM-only, honestly out of scope

- `clojure.java.io` — whole namespace (19/19): `reader`, `writer`,
  `input-stream`, `output-stream`, `file`, `resource`, `copy`, `delete-file`,
  etc. — all JVM `java.io`/`java.net` bound.
- Class/reflection: `class`, `cast`, `bases`, `supers`, `extends?`,
  `extenders`, `find-protocol-impl`, `find-protocol-method`
- Compilation/classloading: `compile`, `load`, `load-reader`, `load-string`,
  `loaded-libs`, `add-classpath`, `*compile-path*`, `*compiler-options*`,
  `*fn-loader*`, `*source-path*`, `*command-line-args*`, `*compile-files*`,
  `*allow-unresolved-vars*`, `*verbose-defrecords*`, `*use-context-
  classloader*`
- `gen-class`, `gen-interface`, `proxy`, `proxy-super`, `proxy-mappings`,
  `proxy-name`, `construct-proxy`, `init-proxy`, `update-proxy`,
  `get-proxy-class`, `definterface`, `definline`
- Deprecated struct-maps: `defstruct`, `struct`, `struct-map`,
  `create-struct`, `accessor`
- JVM-exception introspection: `StackTraceElement->vec`, `Throwable->map`,
  `stack-element-str`
- JVM interop seq adapters: `resultset-seq`, `enumeration-seq`,
  `iterator-seq` (borderline — could map onto a Go iterator concept, but the
  oracle form is `java.util.Iterator`-shaped), `xml-seq`
- `import`, `use`, `refer-clojure` — Java-package-shaped `ns` clauses;
  cljgo has its own `require`/`require-go` story (ADR-relevant, not a
  straight gap)
- `to-array-2d`, `into-array` (present, works) vs `make-array` (missing,
  JVM array-class reflection)
- `with-loading-context`, `with-local-vars`, `with-bindings`/
  `with-bindings*`, `with-in-str` — technically portable but tightly coupled
  to JVM classloading / `Var` internals in their real implementations;
  flagged C-leaning-B, not prioritized.

## Macro-specific gap list

31 real Clojure **macros** are missing from `clojure.core` (found only by
filtering the live dump to `:macro` vars — a missing macro never surfaces
as a suite failure since nothing calls it):

`declare`, `defn-`, `defonce`, `defstruct`, `definline`, `definterface`,
`letfn`, `locking`, `time`, `with-open`, `with-redefs`, `with-bindings`,
`with-in-str`, `with-loading-context`, `with-local-vars`, `reify`, `proxy`,
`proxy-super`, `pvalues`, `refer-clojure`, `sync`, `gen-class`,
`gen-interface`, `import`, `io!`, `lazy-cat`, `memfn`, `amap`, `areduce`,
`..`, `vswap!` (present, but as a **function** in cljgo rather than a
macro — semantically usable the same way at call sites, flagged as a
minor form-divergence, not a missing-feature).

## Recommended batch plan (A-list, ordered)

1. **Batch 1 — control-flow bread-and-butter**: `declare`, `defonce`,
   `defn-`, `letfn`, `with-open`, `with-redefs`(+ `-fn`), `locking`, `time`.
   All are macro-only or thin wrappers over existing primitives (atoms,
   `try`/`finally`, `alter-var-root`) — lowest implementation risk, highest
   frequency payoff.
2. **Batch 2 — functional idioms**: `memoize`, `trampoline`, `reify`.
   `reify` is the highest-value/highest-effort item (needs the same
   protocol-dispatch machinery `defrecord`/`extend-protocol` already use,
   just anonymously) — pair with a conformance file per ADR discipline.
3. **Batch 3 — seq fns**: `reductions`, `tree-seq`, `line-seq`,
   `split-with`, `update-keys`, `update-vals`, `lazy-cat`, `future?`.
   Cheap, mechanical, high test-suite leverage.
4. **Batch 4 — `clojure.walk`**: whole namespace, ~10 fns, all pure
   data-manipulation (no host/interop dependency) — self-contained port.
5. **Batch 5 — reader**: `read`, `read-string`, `read-line` (core, distinct
   from the already-working `clojure.edn` versions) — depends on exposing
   `pkg/reader` through a `clojure.core`-shaped API; likely needs a small
   design note on eval-on-read safety (`*read-eval*`) before an ADR.
6. **Batch 6 — introspection**: `ns-publics`/`ns-map`/`ns-interns`/
   `ns-refers`/`ns-name`/`all-ns`/`the-ns` — needed for any REPL tooling,
   and would have let this very audit measure cljgo's surface in cljgo
   itself instead of via a throwaway Go test.
7. Everything else in the B-list is real but lower-frequency; triage
   per ADR as concrete use cases surface. C-list is JVM-bound and stays
   out of scope by design (host is Go, not the JVM).

## Appendix — raw data

Oracle dumps (`clojure -M -e '(ns-publics ...)'`) and the cljgo live-registry
dump used for this audit are not checked in (throwaway scratch files); the
counts above were computed with `comm -23`/`comm -12` under `LC_ALL=C` from:

- oracle: `clojure.core.txt` (679), `clojure.string.txt` (21),
  `clojure.set.txt` (12), `clojure.edn.txt` (2), `clojure.walk.txt` (10),
  `clojure.zip.txt` (28), `clojure.data.txt` (5), `clojure.repl.txt` (13),
  `clojure.pprint.txt` (26), `clojure.java.io.txt` (19),
  `clojure.test.txt` (39), plus `clojure.core.macros.txt` (79 macros)
- cljgo: live dump of `lang.AllNamespaces()` → 8 real namespaces populated
  (`clojure.core`, `clojure.string`, `clojure.set`, `clojure.edn`,
  `clojure.repl`, `clojure.test`, plus `clojure.core-test.portability` and
  `cljgo.build`, both cljgo/suite-internal, not counted against the oracle)
