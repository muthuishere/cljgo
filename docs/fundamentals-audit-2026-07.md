# Fundamentals audit — live status (re-measured 2026-07-23, post-tail-wave)

**This replaces the mid-campaign snapshot** taken while the fundamentals
batches were in flight. Method is unchanged — every number below is a live
diff against the real Clojure CLI 1.12.5 oracle (`ns-publics` both sides,
`LC_ALL=C` sorted before `comm`), run **in cljgo itself**.

**The purity bar (the precedence principle, binding):** cljgo is *pure
Clojure* on a Go host. Every implemented var carries JVM-1.12.5-oracle-exact
semantics, frozen in the dual-harness conformance suite (REPL and compiled
binary byte-identical — a divergence is a release blocker). Nothing cljgo
adds may shadow, rename, or change clojure.core or the reader; when a new
feature's natural name collides with Clojure, the *new* feature renames
(`just`/`none`, ADR 0014), never Clojure.

## Headline numbers (2026-07-23, post-tail-wave)

| namespace | oracle vars | cljgo has | missing | % |
|---|---:|---:|---:|---:|
| `clojure.core` | 679 | 632 (+74 cljgo-only extras) | 47 — all permanently-JVM | **93%** |
| `clojure.string` | 21 | 21 | 0 | **100%** |
| `clojure.set` | 12 | 12 | 0 | **100%** |
| `clojure.edn` | 2 | 2 | 0 | **100%** |
| `clojure.walk` | 10 | 10 | 0 | **100%** |
| `clojure.zip` | 28 | 28 | 0 | **100%** |
| `clojure.data` | 5 | 5 | 0 | **100%** |
| `clojure.pprint` | 26 | 26 | 0 | **100%** |
| `clojure.repl` | 13 | 14 | 0 | **100%** (+1 extra) |
| `clojure.test` | 39 | 42 | 0 | **100%** |
| `clojure.java.io` | 19 | 0 | 19 | out of scope — JVM I/O |

The 74 cljgo-only extras: ~50 `-`-prefixed macro-support helpers, the ADR
0014 Result/Option surface (`ok`/`err`/`just`/`none` + combinators),
`require-go`, `let?`, `cljgo-version`.

**Every implementable clojure.core var now exists.** The remaining 47 are
the permanently-JVM set below — each with a one-line reason it cannot exist
on a Go host. There is no third bucket.

## Complete — landed 2026-07-23

The fundamentals batches all merged, then the tail wave closed the rest:

- **A1 — stdio/CLI/atoms/runtime** (PR #105); **A2 — printing/reading/
  data-readers + clojure.test/report** (PR #106); **A3 — 1.11/1.12 seq &
  value fns** (PR #101); **A4 — namespaces/concurrency/chunk shims**
  (PR #103); **reader options** (`:read-cond`/`:features`/`:preserve`).
- **Tail wave** (this change): the deprecated struct family (`create-struct`
  `defstruct` `struct` `struct-map` `accessor` — over array-maps, basis
  order oracle-exact), the **functional protocol core** (`extend`
  `extends?` `extenders` `find-protocol-impl` `find-protocol-method` over
  the same registry the extend-type/extend-protocol macros feed), `cast`/
  `bases`/`supers` over the real ADR 0039 ancestry, the typed-array
  micro-API (`aset-boolean|byte|char|double|float|int|long|short`, the
  array casts `booleans`…`shorts`, `to-array-2d`), `vector-of` (ctor-coerced
  over the persistent vector — observable contract oracle-exact, JVM's
  gvec representation documented as divergent), `iterator-seq`/
  `enumeration-seq` (over the Go host's iterator shapes: HasNext/Next
  method pairs and channels — documented receiver set), `bean` (Go struct
  reflection, kebab-cased keyword keys — documented analogue), `replicate`,
  `test`, `definline` (defn + :inline metadata, no call-site inlining —
  performance-only divergence), `*repl*` (root false; bound true in the
  interactive REPL/nREPL session frame), `await1`,
  `seq-to-map-for-destructuring` **plus the destructure wiring it was the
  fix point for** — `(defn f [& {:keys [a]}] a)` is now callable as
  `(f :a 1)`, `(f {:a 1})` AND `(f :a 1 {:b 2})` (previously all nils),
  `unquote`/`unquote-splicing` (root-unbound placeholders), `->Eduction`,
  `print-ctor`, and **`#uuid` literals as compiled constants** (the
  `reader.MustUUID` emit case, closing the last known REPL-vs-binary
  reader-literal gap; the two eval-only conformance waivers are lifted).

Conformance: `structs-basics.clj`, `extend-fn.clj`, `extend-extenders.clj`,
`cast-and-ancestry.clj`, `aset-typed-array-casts.clj`, `to-array-2d.clj`,
`vector-of.clj`, `kwargs-destructure.clj`, `iterator-enumeration-seq.clj`,
`bean-go-struct.clj`, `tail-wave-misc.clj`, `uuid-literal-aot.clj` — all
dual-harness, each divergence documented in-file.

## Permanently out of scope — the final 47, each with its reason

JVM bytecode / classloader machinery (no Go-host meaning):

- `proxy` `proxy-super` `proxy-call-with-super` `proxy-mappings`
  `proxy-name` `construct-proxy` `get-proxy-class` `init-proxy`
  `update-proxy` — runtime JVM subclass generation; Go has no runtime
  class synthesis.
- `gen-class` `gen-interface` `definterface` — emit JVM bytecode/class
  files; a Go host compiles Go source (the emitter IS that path).
- `import` — brings JVM classes into a namespace; cljgo's host types come
  from `require-go` (ADR 0036 keeps the class table fail-closed).
- `compile` `load` `load-reader` `loaded-libs` `add-classpath` — the JVM
  classpath/AOT-classfile loading model; cljgo loads namespaces from
  source/registered providers (ADR 0042/0046).
- `*compile-path*` `*compiler-options*` `*fn-loader*` `*source-path*`
  `*allow-unresolved-vars*` `*suppress-read*` `*use-context-classloader*`
  `*verbose-defrecords*` `*reader-resolver*` — knobs of that same JVM
  compiler/classloader stack (cljgo's reader resolver is wired
  programmatically, not via a dynvar).

JVM-library adapters (the wrapped library does not exist here):

- `stream-into!` `stream-reduce!` `stream-seq!` `stream-transduce!` —
  java.util.stream adapters.
- `resultset-seq` — java.sql.ResultSet.
- `PrintWriter-on` — java.io.PrintWriter.
- `StackTraceElement->vec` — JVM stack-frame objects (cljgo has no
  stack-frame introspection; `Throwable->map`'s `:trace []` documents it).
- `send-via` `set-agent-send-executor!` `set-agent-send-off-executor!` —
  pluggable j.u.c executors; cljgo agents run on goroutines, there is no
  executor object to plug (documented in the agent conformance files).
- `with-loading-context` — classloader-binding wrapper for the JVM load
  stack.

clojure.core-internal machinery whose substrate is JVM-specific:

- `->Vec` `->VecNode` `->VecSeq` `EMPTY-NODE` `->ArrayChunk` — gvec's
  primitive-array internals (`->ArrayChunk` even takes an ArrayManager);
  cljgo's `vector-of` implements the observable contract over the
  persistent vector, so these ctors have no honest backing value.
- `-cache-protocol-fn` `-reset-methods` — the JVM protocol call-site
  cache; cljgo protocols dispatch through the corelib registry with no
  per-site cache to reset.
- `method-sig` `primitives-classnames` — java.lang.reflect.Method /
  primitive Class table helpers for gen-class/definline inlining.

## Scoreboard cross-check

jank clojure-test-suite: **238/242 files (98.3%), 242/242 vars, 0 failures**
(`cljgo suite`; the 4 errors are upstream reader-conditional files with no
`:default` branch — `docs/suite-upstream.md`). Dual-harness conformance:
~430 oracle-cited files, REPL-vs-binary byte-identical, enforced on every
commit.

## Definition of done — met

Every oracle var is either (a) implemented with oracle-exact,
conformance-frozen semantics, or (b) listed above with a one-line reason it
cannot exist on a Go host. No third bucket. Re-measure this document if the
oracle moves (a new Clojure release) or a listed reason stops being true.
