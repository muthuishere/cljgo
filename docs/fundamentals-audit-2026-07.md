# Fundamentals audit — live status (re-measured 2026-07-23)

**This replaces the early-July snapshot** (which measured 65% core coverage and
0% walk/zip/data/pprint — all long since fixed by the fundamentals batches).
Method is unchanged — every number below is a live diff against the real
Clojure CLI 1.12.5 oracle (`ns-publics` both sides, `LC_ALL=C` sorted before
`comm`) — but the audit now runs **in cljgo itself**, because `ns-publics`/
`ns-map`/`all-ns` exist now; the early-July version needed a throwaway Go test
just to dump the registry.

**The purity bar (the precedence principle, binding):** cljgo is *pure
Clojure* on a Go host. Every implemented var carries JVM-1.12.5-oracle-exact
semantics, frozen in the dual-harness conformance suite (REPL and compiled
binary byte-identical — a divergence is a release blocker). Nothing cljgo
adds may shadow, rename, or change clojure.core or the reader; when a new
feature's natural name collides with Clojure, the *new* feature renames
(`just`/`none`, ADR 0014), never Clojure.

## Headline numbers (2026-07-23)

| namespace | oracle vars | cljgo has | missing | % |
|---|---:|---:|---:|---:|
| `clojure.core` | 679 | 576 (+70 cljgo-only extras) | 173 | **85%** |
| `clojure.string` | 21 | 21 | 0 | **100%** |
| `clojure.set` | 12 | 12 | 0 | **100%** |
| `clojure.edn` | 2 | 2 | 0 | **100%** |
| `clojure.walk` | 10 | 10 | 0 | **100%** |
| `clojure.zip` | 28 | 28 | 0 | **100%** |
| `clojure.data` | 5 | 5 | 0 | **100%** |
| `clojure.pprint` | 26 | 26 | 0 | **100%** |
| `clojure.repl` | 13 | 14 | 0 | **100%** (+1 extra) |
| `clojure.test` | 39 | 41 | 1 (`report`) | **97%** |
| `clojure.java.io` | 19 | 0 | 19 | out of scope — JVM I/O |

The 70 cljgo-only extras: ~47 `-`-prefixed macro-support helpers, the ADR 0014
Result/Option surface (`ok`/`err`/`just`/`none` + combinators), `require-go`,
`let?`, `cljgo-version`.

Every item the old audit's "top-10 most damning gaps" named — `reify`,
`with-open`, `memoize`, `letfn`, `trampoline`, `declare`/`defonce`/`defn-`,
`with-redefs`, `slurp`/`spit`, `split-with`, all of `clojure.walk` — **exists
and is conformance-frozen** (spot-probed live, 2026-07-23; only `line-seq`
of the old headline list remains, and it is in batch A1 below). Also landed
since that snapshot: JVM-compatible hashing (ADR 0051), tagged literals +
reader conditionals (0050), typed exception classes catchable with real JVM
ancestry (0039 addendum), named arity errors and ns-map parity across both
legs (REPL-vs-binary divergence fixes), `.cljc` acceptance (0068), and the
perf campaign (0063–0067: ~35×→~5× of handwritten Go).

## The remaining 173, classified

### In-flight batches (the path to "all complete", owner-directed 2026-07-23)

- **A1 — stdio/CLI/atoms/runtime** (branch `fundamentals/batch-a1`):
  `*command-line-args*` `*err*` `read-line` `flush` `newline` `with-in-str`
  `line-seq` `file-seq` `compare-and-set!` `swap-vals!` `reset-vals!`
  `reset-meta!` `bound?` `thread-bound?` `requiring-resolve` `load-string`
  `class` `infinite?`
- **A2 — printing/reading/data-readers + `clojure.test/report`** (branch
  `fundamentals/batch-a2`): `print-method` `print-dup` `print-simple`
  `*print-level*` `*print-meta*` `*print-namespace-maps*`
  `char-escape-string` `char-name-string` `default-data-readers`
  `*default-data-reader-fn*` `*read-eval*` `read` `read+string` `munge`
  `namespace-munge` `Throwable->map` + the `report` multimethod
- **A3 — 1.11/1.12 seq & value fns** (PR #101): `partitionv`
  `partitionv-all` `splitv-at` `iteration` `comparator` `xml-seq` `Inst`
  `inst?` `inst-ms` `uri?` `byte-array` `bytes?` `make-array` `find-keyword`
  + fixes the `sequence` multi-coll transducer arity bug
- **A4 — namespaces/concurrency/chunk shims** (PR #103): `ns-unmap`
  `ns-unalias` `remove-ns` `refer-clojure` `use` `pmap` `pcalls` `pvalues`
  `seque` `with-bindings`(`*`) `with-local-vars` + the `chunk`/`chunk-first`/
  `chunked-seq?` compat family (real machinery underneath since ADR 0063)
- **Reader options** (branch `fundamentals/reader-conditionals`):
  `read-string` `{:read-cond :allow}` `:features` honored (currently
  silently ignored — a real bug), `:preserve` wiring, top-level-splice
  diagnostics, plus the full reader-conditionals-guide conformance battery
- **Tail wave (queued)**: the deprecated/compat set — structs (`defstruct`
  `struct` `struct-map` `create-struct` `accessor`), `replicate`,
  `aset-<type>` family, `unchecked-*-int` family, `vector-of`,
  `to-array-2d`, `definline`-as-defn, `with-precision` — plus
  cljgo-truthful versions of borderline JVM items where honest semantics
  exist: `cast`/`bases`/`supers` (over ADR 0039 ancestry), `iterator-seq`/
  `enumeration-seq` (over Go iterators), `bean` (over struct reflection).

### Permanently out of scope (~55 after the tail wave, each with a reason)

JVM bytecode/classloader machinery with no Go-host meaning: the `proxy`
family (runtime JVM subclassing), `gen-class`/`gen-interface`/`import`/
`add-classpath`/`compile`/`load-reader`/`loaded-libs` and the compiler-knob
dynvars, `java.util.stream` adapters (`stream-*!`), `resultset-seq`,
`PrintWriter-on`, `StackTraceElement->vec`, `send-via`/
`set-agent-send-executor!` (goroutine agents have no pluggable executor),
and `clojure.java.io` (cljgo's I/O story is Go interop + the `bri.io`
battery, ADR 0061).

## Scoreboard cross-check

jank clojure-test-suite: **238/242 files (98.3%), 242/242 vars, 0 failures**
(`cljgo suite`; the 4 errors are upstream reader-conditional files with no
`:default` branch — `docs/suite-upstream.md`). Dual-harness conformance:
~420 oracle-cited files, REPL-vs-binary byte-identical, enforced on every
commit.

## Definition of done

Every oracle var either (a) implemented with oracle-exact, conformance-frozen
semantics, or (b) listed above with a one-line reason it cannot exist on a Go
host. No third bucket. This document is re-measured and updated when the
in-flight batches merge.
