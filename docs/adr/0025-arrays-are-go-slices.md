# ADR 0025 — cljgo arrays are native Go slices

Date: 2026-07-16 · Status: accepted · Relates to: ADR 0010, ADR 0022, design/05 §1

## Context

ADR 0022's cheap-breadth sweep of the jank clojure-test-suite surfaced four
missing vars gating many files: `to-array`, `int-array`, `object-array`
(4 files each) and `volatile!`/`vswap!`/`vreset!`/`volatile?` (3 files). All
five are real `clojure.core` names — the precedence principle (CLAUDE.md)
requires adding them as genuine core fns, not renamed shims.

JVM Clojure's arrays are literally `java.lang.reflect` arrays: `int[]`,
`Object[]`, `long[]`, etc. — real, mutable, typed, class-bearing values.
cljgo hosts on Go, which has no equivalent reflected-class array type, so a
representation had to be chosen and documented (owner's mandate on this
batch): **what does `(int-array 3)` return as a Go value?**

## Decision

**A cljgo "array" is a plain Go slice**, typed by element:

| Clojure ctor | Go representation | element coercion |
|---|---|---|
| `object-array`, `to-array` | `[]any` | none (already boxed) |
| `int-array`, `long-array` | `[]int64` | `lang.LongCast` |
| `float-array` | `[]float32` | `lang.FloatCast` |
| `double-array` | `[]float64` | `lang.AsFloat64` |
| `boolean-array` | `[]bool` | `lang.BooleanCast` |
| `char-array` | `[]lang.Char` | `lang.CharCast` |

`int-array`/`long-array` share one Go type (`[]int64`) because cljgo's
numeric tower already uses `int64` as the single fixnum representation
(design/08 §5 Batch 2) — there is no 32-bit `int` value anywhere else in the
runtime to make a genuinely narrower `int-array`, so a real distinction would
be cosmetic. This is a deliberate, documented divergence from the JVM (where
`int-array` and `long-array` produce different classes) — noted in the doc
comment on `pkg/eval/array_builtins.go`, not silently glossed over.

No new interop glue was needed to make this useful: `pkg/lang` already
special-cases `reflect.Slice`/`reflect.Array` receivers throughout —
`Seq` (`pkg/lang/seq.go`), `Nth` (`pkg/lang/iteration.go`), `Get`
(`pkg/lang/interfaces.go`), `ToString`/`Print` (`pkg/lang/strconv.go`), and
Go-interop argument coercion (`coerceGoValue`, `pkg/lang/apply.go:206`,
element-wise, recursively) all already walk a raw Go slice via reflection.
This is exactly the "arrays participate in seq/get/count via a runtime
bridge" line design/05 §1 called out in advance. Consequences:

- **`aset` mutates in place**, as Clojure requires — it is a real Go slice
  header pointing at the same backing array, so `lang.SliceSet` (already
  used by `pkg/lang`, unchanged) writes through.
- **A cljgo array passed to Go interop already coerces correctly** for any
  Go func taking a slice, because `coerceGoValue`'s `reflect.Slice` branch
  builds the target slice element-by-element regardless of whether the
  source is one of our typed arrays, a persistent vector, or a bare `ISeq` —
  no array-specific interop code was added or is needed.
- **Printing**: `lang.ToString`/`Print` fall through to the pre-existing
  generic-slice branch (`pkg/lang/strconv.go:116`) and render an array as a
  vector-shaped literal, e.g. `(pr-str (int-array [1 2 3]))` => `"[1 2 3]"`.
  This diverges from the JVM's `#object["[I" 0x... "[I@..."]` array printing.
  We accept the divergence: cljgo has no `class` builtin yet (out of scope
  for this batch — see PR notes) so there is no way to *observe* the JVM's
  class-tagged printing anyway, and the vector-shaped rendering is strictly
  more useful at a REPL. If `class`/reflective array printing is added
  later, this can be revisited without breaking any frozen conformance
  expectation (none of this batch's tests freeze a raw `pr-str` of an array
  — they all freeze `seq`/`vec`/`aget`/`alength` results, which are
  representation-independent).
- **`seqable?`** needed a one-line fix (Batch 1, `predicate_builtins.go`) to
  recognize raw `reflect.Slice`/`reflect.Array` values — it only matched the
  `string`/`Seqable`/`ISeq` interfaces before, so `(seqable? (object-array 3))`
  incorrectly returned `false` even though `(seq (object-array 3))` already
  worked. Required by `seqable_qmark.cljc`'s oracle-verified expectation.
- **`aget`/`aset`/`alength`/`aclone`** are new Go builtins
  (`pkg/eval/array_builtins.go`) that require their first argument to
  actually be a `reflect.Slice`/`reflect.Array` — passing a persistent
  vector panics, matching the oracle (`(aget [1 2 3] 0)` throws
  `IllegalArgumentException` on real Clojure 1.12.5: "No matching method
  aget found taking 2 args", since `aget` there is a macro that only
  expands onto real array classes).

`volatile!`/`vswap!`/`vreset!`/`volatile?` are unrelated to arrays but share
this batch. A volatile is `*lang.Volatile` (`pkg/lang/volatile.go`): a bare
mutable box with `Deref`/`Reset`/`Swap`, deliberately **not** atomic (no
CAS, no watches) — that is the actual JVM contract (`clojure.lang.Volatile`
is a plain non-thread-safe mutable field; the "volatile" name refers to Java
memory-model visibility semantics irrelevant to a single Go goroutine's
uncontended access, not to CAS retry).

## Consequences

- `int-array`/`long-array` collapse to the same Go type; `(long-array 3)`
  and `(int-array 3)` are indistinguishable in cljgo. Documented, not hidden.
- No `class` builtin exists yet, so nothing can currently observe or depend
  on an array's "JVM class" — `(class (int-array 3))` is simply not
  expressible in cljgo today. Left for whichever future batch adds `class`.
- `make-array` (JVM signature `(make-array class dims...)`) and
  `into-array` with an explicit type-hint arg were **not** added — both
  hinge on a Java `Class` value cljgo has no equivalent for. `into-array`
  is implemented in its single-arg, type-*inferring* form only (infers
  `int64`/`float64`/`bool`/`lang.Char`/`string` from the first element,
  else falls back to `[]any` — a cheap approximation of the JVM's
  `RT.into-array`'s per-element-class inference, verified against the
  oracle for the common cases).
