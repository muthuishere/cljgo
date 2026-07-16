# clojure-test-suite files blocked on upstream `:cljgo` branches

Status of the four load-path suite failures diagnosed 2026-07-17 (baseline
234/242). In all four, cljgo's reader was verified to match JVM Clojure
1.12.5 elision semantics exactly (`read-string {:read-cond :allow
:features #{:cljgo}}` oracle, mirror-image method of ADR 0036 /
`pkg/reader/phase2_test.go`): a reader conditional with no matching
feature key and no `:default` key reads as NOTHING. These files gate
per-dialect forms on `:cljr/:lpy/:phel/:cljs/:clj` with no `:default`,
so under `#{:cljgo :default}` (ADR 0036) the elision leaves a broken
form — and real Clojure restricted to the same feature set errors
identically. **Not cljgo bugs.** The fix is upstream: `:cljgo` branches
in the suite, exactly as `:jank`/`:lpy`/`:phel` were added for those
dialects. Each proposed patch below was verified against a patched suite
copy (2026-07-17): abs and short go `error → pass`.

## abs.cljc — runtime, not reader

Under `:default` the file takes `[r/min-int (* -1 r/min-int)]`
(line 26). On any checked-arithmetic host the EXPECTED-value expression
`(* -1 r/min-int)` itself throws (cljgo: "integer overflow"; JVM would
throw "long overflow" — which is why `:clj` has its own branch). cljgo's
`abs` has the same 2's-complement wrap as the JVM: `(abs min-int)` ⇒
`min-int`. Upstream one-liner — alongside line 25's `:clj` branch:

```clojure
:cljgo [r/min-int r/min-int]
```

Verified: file passes with this patch. (jank escapes because C++ i64
arithmetic wraps rather than throws, so `(* -1 min)` ⇒ `min` and the
comparison holds; only checked non-JVM dialects hit this.)

## add_watch.cljc — elided catch class

Lines 20–24 (and 93–96, 162–164): `(catch #?(:cljr … :cljs :default
:clj clojure.lang.ExceptionInfo) e …)` has no `:default` KEY, so the
class position elides ⇒ `(catch e (let …))` ⇒ "bad binding form".
Identical under the JVM oracle. cljgo's catch matches class names by
string and `(catch clojure.lang.ExceptionInfo e …)` works (ADR 0036),
so the upstream patch is `:cljgo clojure.lang.ExceptionInfo` in each of
the three catch conditionals. Verified: the parse unblocks; the file
then stops at `agent-error` (cljgo capability gap — `agent-error` /
`restart-agent` are not yet implemented; tracked separately from this
reader diagnosis).

## short.cljc — elided instance? class

Line 16: `(instance? #?(:cljr System.Int16 :clj java.lang.Short)
(short 0))` elides to `(instance? (short 0))` — 1 arg, an arity error on
any Clojure. Upstream: `:cljgo java.lang.Short` (cljgo interns JVM class
names as class refs; `(instance? java.lang.Short (short 0))` ⇒ `true`).
Verified: file passes with this patch.

While diagnosing this, one real cljgo divergence surfaced and was fixed:
macro arity errors counted the hidden `&form`/`&env` args ("wrong number
of args (3)"); JVM Compiler.macroexpand1 hides them (`ArityException
(e.actual - 2, e.name)` ⇒ "(1)"). Fixed in `pkg/eval/macro.go` +
`pkg/eval/fn.go`; conformance:
`conformance/tests/macro-arity-error-hides-hidden-args.clj`.

## reduce.cljc — elided map VALUES

Lines 15–43: the `interop` map's six values are conditionals with no
`:default`, so each elides and leaves the map literal odd ⇒ "Map literal
must contain an even number of forms" at 8:3 — byte-identical to the JVM
oracle's error for `{:a 1 :Integer #?(:cljr X :lpy Y)}`. Upstream:
`:cljgo` entries for the six values (`:int-new` ⇒ `int`, type slots ⇒
`java.lang.Long`/`java.lang.Double`/`java.lang.Boolean`). Verified: the
file then READS, but still errors at runtime — cljgo's `into-array` is
1-arity only (ADR 0025 arrays are Go slices); the typed
`(into-array (:Integer interop) …)` calls need a 2-arity `into-array`
(CLJS accepts-and-ignores the type; a reasonable cljgo model). That is a
capability gap to take separately, not a load-path bug.
