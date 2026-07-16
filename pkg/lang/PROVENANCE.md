# pkg/lang provenance

## Where this code came from

- **Upstream**: [Glojure](https://github.com/glojurelang/glojure)
  `pkg/lang` plus the four `internal/` packages it needs (`murmur3`,
  `seq`, `persistent/vector` — the elvish port with its own EPL-1.0
  LICENSE — and `goid`), vendored from the local checkout
  `refs/glojure` @ commit `c74bc07d2a8c8b39da04d6af84dd764cb984ea9d`
  (tag `v0.6.8`, 2026-07-10).
- **Via spike S4** (`spikes/s4-vendor-lang/`, 2026-07-11): this tree is
  a promotion of that spike's `lang/` + `internal/` + `identity/`
  output. The full sever-and-modernize log is
  `spikes/s4-vendor-lang/SURGERY.md`; every S4 change is marked
  `cljgo S4 surgery:` in the code. Highlights: interpreter glue deleted
  (`builtins.go`, `class.go`, `environment.go`, pkgmap coupling),
  `go4.org/intern` → stdlib `unique`, pcastools/hashstructure/testify →
  stdlib. Zero external dependencies.
- **License**: EPL-1.0, preserved as `LICENSE-glojure.md` (upstream has
  a repo-level license, no per-file headers). `internal/persistent/
  vector` keeps its upstream `LICENSE` (elvish port — also EPL-1.0).
  Design doc 02 §4 records the hard-fork decision (option 3). This tree
  stays EPL-1.0; the rest of cljgo is MIT — see the root `NOTICE`.

## Changed at promotion (M0 stage A, 2026-07-11)

- Import paths: `cljgo-spike-s4/...` →
  `github.com/muthuishere/cljgo/pkg/lang/...`; spike `identity/` moved
  to `internal/identity/` (keyword §4.4 identity-contract tests).
- **S4 defect #1 fixed — Equiv/Equals split** (`equal.go`), ground
  truth `clojure.lang.Util.equiv` vs `Util.equals`:
  - `Equiv` is now Clojure `=`: category-based numeric equality,
    structural collection equiv. No longer an alias of `Equals`.
  - `Equals` is now the type-strict Java `.equals` analog (floats by
    bit pattern, numbers never equal across concrete types).
  - `aseqEquals` (aseq.go) split from `aseqEquiv` (elements via
    `Equals`, per `ASeq.equals`).
  - `equalKey` (map.go) switched `Equals` → `Equiv` (array-map key
    dedup is `=` semantics, per `PersistentArrayMap.equalKey`).
  - Added Java-shaped `Equals` methods to `*Map`,
    `*PersistentHashMap`, `*PersistentStructMap` (→ `mapEquals`) and
    `*MapEntry` (→ `apersistentVectorEquals`) so the strict global
    `Equals` keeps collection `.equals` behavior.
  - Acceptance: `equiv_test.go` (new) + inverted defect-1 pins in
    `s4_defects_test.go`.
- **S4 defect #2 (HAMT transients) deferred** — see `TODO.md`.

## Printer fidelity surgery (M1-A, 2026-07-12, `strconv.go`)

Ground truth: real Clojure 1.12.5 CLI on JDK 26.

- `formatFloat` rewritten to Java `Double.toString` semantics (upstream
  printed `%d.0` for integral doubles and `'f'` expansion otherwise,
  zero-padding huge magnitudes): plain decimal for 1e-3 <= |v| < 1e7,
  scientific `d.dddE±x` otherwise, shortest round-trip digits, `-0.0`
  sign preserved, plus the JDK subnormal quirk (`4.9E-324`, not
  `5E-324`). Verified bit-exactly against `Double/toString` on ~108k
  doubles (random + exhaustive small-subnormal scan): zero divergences.
- `Print` now emits `##Inf` / `##-Inf` / `##NaN` for non-finite doubles
  (was `Infinity`/`NaN`; those Java names remain in `ToString`/str).
- `Print`'s ISeq branch walks `x.Seq()`, so empty lists print `()`
  instead of `(nil)`.
- Acceptance: `conformance/tests/print-double-{plain,scientific,subnormal}.clj`,
  `print-inf-nan.clj`, `print-empty-list.clj`.

## Polymorphism value types (M5, 2026-07-15, ADR 0020)

New (non-vendored, no EPL header) `instance.go` adds `*DType` (deftype
instances) and `*Record` (defrecord instances — an `IPersistentMap` with a
type identity). Minimal surgery to two vendored files so records behave
faithfully:

- `strconv.go` `Print`: a `*Record` case (`#ns.Name{:a 1, :b 2}`) placed
  BEFORE the generic `IPersistentMap` branch (a record IS a map).
- `apersistentmap.go` `apersistentmapEquiv`: a one-line `IsRecord(obj)`
  guard so `(= plain-map record)` is false (records carry a type identity;
  `Record.Equiv` enforces the record→map direction).
- Acceptance: `conformance/tests/{protocol,deftype,defrecord,extend}-*.clj`,
  dual-harness, byte-matched vs Clojure CLI 1.12.5.

## *print-length* (batch/harness-misc, 2026-07-16, ADR 0022)

- `var.go`: new `VarPrintLength` dynamic var backing `*print-length*`
  (root nil = unlimited, exactly clojure.core).
- `strconv.go` `Print`: the ISeq / IPersistentMap / IPersistentVector /
  IPersistentSet branches honor it — at most N elements then `...`
  (oracle 1.12.5: `(binding [*print-length* 3] (pr-str (range 10)))` =>
  `"(0 1 2 ...)"`). Motivation: without a bound, printing an infinite lazy
  seq never terminates — a failing clojure.test assertion over
  lazy-infinite-range hung the whole suite run. `cljgo suite` binds it to
  100 for the run (cmd/cljgo/suite.go).
- Acceptance: `conformance/tests/print-length.clj` (dual-harness,
  oracle-verified).

## Numeric tower fidelity (ADR 0029, 2026-07-16, `numberops.go` + `numbers.go`)

Ground truth: real Clojure 1.12.5 CLI (spike S13,
spikes/s13-numeric-divergences/VERDICT.md — 99/276 probes diverged;
42 remain after this surgery, all deferred cluster C (BigDecimal
representation) or G (message wording)).

- `numberops.go` `float64Ops.Quotient`/`Remainder` rewritten to the JVM
  `Numbers.quotient/remainder(double,double)` shape (cluster A). Upstream
  checked only a zero divisor in Quotient and Remainder was a bare
  `math.Mod`: a 0.0 divisor now throws `Divide by zero`
  (oracle: `(rem 10.0 0)` => THREW, was `##NaN`); a quotient outside int64
  range that is Inf/NaN throws `Infinite or NaN` (oracle: `(quot ##Inf 1)`,
  `(mod 5 ##NaN)` => THREW, were garbage BigDecimals / NaN); a finite huge
  quotient returns a double via big-integer truncation (oracle:
  `(quot 1e300 1.0)` => `1.0E300`, was a BigDecimal); Remainder computes
  `x - trunc(x/y)*y` in double arithmetic so `(rem 1 ##-Inf)` => `##NaN`
  exactly as the JVM (0*Inf = NaN), was `1.0` from math.Mod.
- `numberops.go` `AsBigInt(float64/float32)` no longer saturates through
  `int64(x)` (cluster B): new `bigIntFromFloat64` follows
  `BigDecimal.valueOf(double)` — the shortest round-trip decimal
  representation truncated toward zero (oracle:
  `(bigint 1.7976931348623157e308)` => the 309-digit integer, was
  int64-max; `(bigint 4.611686018427388E18)` => `4611686018427388000N`,
  the DECIMAL reading, not the exact binary `...7904`); Inf/NaN throw
  `Infinite or NaN`.
- `numbers.go` `IntCast`/`intCastLong` bound against Java's 32-bit int,
  not Go's platform int (cluster F): integral values outside int32 throw
  `integer overflow` (oracle: `(int 2147483648)`,
  `(int (bigint 3000000000))` => THREW, were silently returned); doubles
  outside int32 throw `Value out of range for int: <Double.toString>`
  (via `formatFloat`); a BigInt beyond int64 throws
  `Value out of range for long: <n>` (longCast fails first on the JVM).
  The unused `_is64Bit` helper was removed with it.
- Acceptance: `conformance/tests/numeric-{quot-rem-float-guards,
  bigint-from-double,int-cast-32bit,abs,even-odd-guard}.clj`
  (dual-harness, expectations byte-verified against the 1.12.5 CLI).

## Transient/sorted-set method-promotion leaks (batch/error-files, 2026-07-16, `set.go`)

Ground truth: real Clojure 1.12.5 CLI, `conformance/tests/transient.cljc`
suite file (jank clojure-test-suite).

`TransientSet` embeds `*Set` and `SortedSet` embeds `Set`; Go promotes the
embedded type's methods onto the wrapper, which silently let both wrapper
types satisfy interfaces real Clojure's equivalents do NOT: `TransientSet`
promoted `Set.Cons` (so `conj` — the non-`!` op — worked on a transient) and
`Set.AsTransient` (so `(transient (transient x))` succeeded instead of
throwing); `SortedSet` likewise promoted `Set.AsTransient` (so
`(transient (sorted-set ...))` succeeded — real `PersistentTreeSet` has no
transient form at all). Added explicit `Cons`/`AsTransient` overrides on
`TransientSet` and an `AsTransient` override on `SortedSet` that panic,
shadowing the promoted methods (oracle: all three throw on real Clojure).
- Acceptance: `conformance/tests/contains-on-string-and-transient.clj`
  covers the read-only interface (`contains?`) still working on transients;
  the throwing cases are exercised directly by the suite file above.
## `IsNil` typed-nil fix (batch/fail-files, ADR 0022, 2026-07-16, `truthiness.go`)

`IsNil` only special-cased `reflect.Ptr`, so a boxed nil value of any
other nillable Go kind (`Func`, `Chan`, `Map`, `Slice`, `Interface`,
`UnsafePointer`) hit Go's classic typed-nil trap: the `any` wrapping a
nil `func() interface{}` is `!= nil` at the interface level (it carries
a type), so `v == nil` was false and the Ptr-only check never fired —
`IsNil` unconditionally returned `false`. `LazySeq.IsRealized` calls
`IsNil(s.fn)` (`s.fn` is exactly a `func() interface{}`), so
`(realized? some-lazy-seq)` was permanently `false` even after the seq
had been forced (`Delay`/`future`/etc. dodge this because their
`IsRealized` checks a plain pointer or bool field, not `IsNil` on a
func). Fixed by switching on every nillable `reflect.Kind`. Acceptance:
`conformance/tests/lazy-seq-realized-after-force.clj` (dual-harness).

## BigDecimal representation swap (ADR 0032, 2026-07-16, `bigdecimal.go` rewritten)

Ground truth: real Clojure 1.12.5 CLI via spike S16
(spikes/s16-bigdecimal-scaled/VERDICT.md — 159-row oracle corpus; the
old representation diverged on 72/132 comparable rows and silently
mantissa-truncated long literals).

- `bigdecimal.go` rewritten wholesale: the upstream Glojure type wrapped
  an immutable `*big.Float` (binary mantissa). Replaced with Java
  BigDecimal's exact model — unscaled `*big.Int` + `int32` scale —
  ported from the S16 prototype (`spikes/s16-bigdecimal-scaled/proto/
  decimal.go`, measured 159/159 vs the oracle). Includes: the
  BigDecimal(String) ctor grammar (scale/E-notation preserved; long
  literals exact), valueOf(double) semantics (shortest decimal string;
  Inf/NaN throw `Infinite or NaN`), valueOf(long)/BigInteger scale-0
  ctors, Ratio.decimalValue via exact divide, add/sub scale = max, mul
  scale = sum, exact divide with preferred scale sx−sy throwing
  `Non-terminating decimal expansion` / `Divide by zero`,
  divideToIntegralValue + remainder preferred-scale rules (oracle:
  `(quot 10.0M 3)` => `3.0M`, `(quot 10.0M 3.0M)` => `3M`), the javadoc
  toString plain-vs-E algorithm (plain iff scale >= 0 and adjusted
  exponent >= -6), and stripTrailingZeros. Exported surface kept
  (`NewBigDecimal`, `MustBigDecimal`, `NewBigDecimalFrom*`,
  `ToBigInteger`, `Cmp`, arithmetic methods); the big.Float-shaped
  members `NewBigDecimalFromBigFloat`/`ToBigFloat` were removed (last
  callers retargeted below).
- `Hash` hashes the stripTrailingZeros-normalized value so `=`-equal
  BigDecimals hash alike (oracle finding: `(= 1.0M 1.00M)` is TRUE —
  Clojure `=` on two BigDecimals is compareTo-based; `equal.go` needed
  no change, `Equals` was already Cmp-based and cross-category-false).
- `numberops.go`: `bigDecimalOps` IsPos/IsNeg/IsZero via the new
  `Sign()`; Divide/Quotient/Remainder now exact-or-throw (oracle:
  `(/ 1M 3M)` and `(/ 1M 3)` THREW, were `0.33333333333333333334M`;
  `(/ 1M 0M)` THREW, was `+InfM`). `AsBigDecimal` sends integral types
  to scale-0 unscaled values directly (was float64-routed — precision
  loss beyond 2^53), doubles through valueOf semantics (oracle:
  `(bigdec ##Inf)` THREW, was `+InfM`; `(bigdec 1.5e300)` =>
  `1.5E+300M`, was a 301-digit plain integer), BigInt/big.Int exact,
  Ratio via exact divide. `AsInt64` BigDecimal arm truncates via
  `ToBigInteger`. Combine matrix untouched (oracle: `(+ 1.0 1.0M)` =>
  `2.0`, double still wins).
- `numbers.go`: `Rationalize` BigDecimal arm uses the exact
  `Rat()` (was a big.Float 'f' rendering); `AsFloat64` via the new
  `Float64()`.
- `bigint.go` `ToBigDecimal` builds the scale-0 value directly.
- `strconv.go` print-readably emits Java `String()` + `"M"` (was
  StripTrailingZeros — the `1.10M` => `1.1M` bug site); `str` was
  already `String()`, which now follows the javadoc algorithm.
- Acceptance: `conformance/tests/bigdec-{literal-scale,coercion,
  coercion-errors,arith-scale,division,equality-hash,printing,tower}.clj`
  (dual-harness, every row byte-verified against the live 1.12.5 CLI at
  freeze time) + `bigdecimal_test.go` toString round-trip guard (the
  emitter reconstructs constants from `String()`). S16 probes re-run:
  132/133 rows match (5 THREW-vs-THREW wording-only diffs; the `hash`
  row aborts on the pre-existing missing `hash` builtin, and the
  hash-set row covers hasheq). `with-precision` (S16 items 13–14)
  remains the ADR 0032 follow-on change.
## goid fast path (ADR 0034, 2026-07-16, `internal/goid/`)

Evidence: spike S18 (spikes/s18-ubuntu-boot-anomaly/VERDICT.md) — the
vendored `goid.Get()` allocated a 32-byte buffer, captured a full
`runtime.Stack()` trace, and text-parsed "goroutine N" out of it on
EVERY dynamic-var deref (`Var.getDynamicBinding`), measuring 72.85% of
`BenchmarkBoot` CPU (`CurrentNS()` derefs the dynamic `*ns*` on nearly
every analyzer/eval step).

- Upstream's single-file stack-parse became the shared `getSlow()`
  fallback (`goid.go`, unchanged logic). New fast path
  (`goid_fast.go` + `getg_{amd64,arm64}.s`, written fresh — zero
  external deps stands, technique per petermattis/goid): a NOSPLIT
  assembly `getg()` returns the current `*g` (dedicated `g` register on
  arm64, `(TLS)` slot on amd64) and Go code reads the `goid uint64`
  field at an offset the compiler derives from `gPrefix`, a
  field-for-field mirror of `runtime.g`'s leading fields transcribed
  from Go 1.26's runtime2.go (verified against go1.26.3 source).
- Compile-time selection: fast path gated
  `(amd64 || arm64) && go1.26 && !go1.27`; everything else builds
  `goid_fallback.go` (`Get = getSlow`). Defense in depth: `init()`
  cross-checks the fast read against the stack-parse oracle once at
  package load and panics on mismatch — a wrong offset can never
  silently mis-key dynamic bindings.
- Measured (Apple M5 Pro, go1.26.3, darwin/arm64, count=5):
  `BenchmarkGoidGet` 1231ns/32B/1alloc → **0.46ns/0B/0allocs**
  (~2600×); `BenchmarkBoot` 211.0ms/472.4k allocs →
  **23.7ms/463.7k allocs** (**8.9× faster boot**). Post-fix CPU profile
  shows `getDynamicBinding`/`CurrentNS` gone from the top-25 cumulative
  list entirely (was 72.85%). ADR 0034's second lever (CurrentNS
  caching) is therefore NOT taken — no longer measurable.
- Acceptance: `goid_test.go` — fast-vs-oracle equality on 300
  concurrent goroutines under `-race`, per-goroutine ID stability and
  uniqueness; full suite + `-race` on lang/repl/nrepl/eval (binding
  conveyance + nREPL session isolation) green.

## Stale hash cache on array-map assoc (fix/persistent-aliasing, 2026-07-16, `persistentarraymap.go`)

Ground truth: real Clojure 1.12.5 CLI; reproduced via `clojure.set/join`.

PR #30 flagged (in its commit message and inline at `core.clj`'s `merge`)
a defect where conj-ing onto a map fetched out of an existing persistent
set appeared to corrupt collection reads, and dodged it by never seeding
`merge`'s reduce from a caller-supplied map. Root cause found and FIXED
here — it was never an in-place mutation of shared storage (a systematic
audit of `Map`/`PersistentHashMap` node ops/`Set`/`SortedMap`/
`PersistentStructMap`/`Vector` + the internal persistent vector found
every persistent path copy-on-write correct). The real defect:
`Map.clone()` — the copy step under both branches of `Map.Assoc` — copied
the cached `hash`/`hasheq` fields along with the struct, so the assoc
result (different contents!) carried the ORIGINAL map's cached hash.
The trigger condition is the source map's hash already being cached,
which is precisely what set membership / map-key use does — hence "a map
fetched out of a collection". Any hash-addressed structure the result was
then stored in (`clojure.set/join` conjes the merged map into its result
set) filed it under the stale hash, making it invisible to `=`/`get`/
`contains?` probes with a structurally equal fresh map — indistinguishable
from corruption at the REPL since printing looked right. Fixed by
resetting `hash`/`hasheq` to 0 (the uncached sentinel) in `clone()`;
content-preserving copies (`WithMeta`) rightly keep their caches, and
transients are untouched. The `merge` dodge in `core.clj` is removed —
it's back to real Clojure's `reduce1 #(conj (or %1 {}) %2)` shape, which
also regains the non-map-first-arg corner ((merge '(1 2 3) 1) => (1 1 2 3),
oracle-verified) and flips the suite's `merge.cljc` to pass (217 → 218).
- Acceptance: `persistentarraymap_hash_test.go` (both Assoc branches +
  the fetch-conj-restore set pattern), `conformance/tests/
  conj-aliasing-hash.clj` (join equality + fetch-then-conj aliasing
  patterns on sets/maps/vectors, all 12 elements oracle-verified),
  `conformance/tests/merge-passthrough-and-nil-args.clj` extended with
  the regained corner; full suite + `-race` on lang/eval green.

## STM-lite, agents, future-cancel, vec aliasing (batch/deep-leftovers, ADR 0038, 2026-07-16)

- `ref.go` REWRITTEN from the Glojure stub (lock-free transaction-count
  sketch, unused elsewhere): `Ref` is a mutex cell with watches +
  validator; one global transaction lock (`RunInTransaction` backing
  dosync) with a dynamic-var in-transaction mark so nested dosync joins
  and the mark conveys like any binding; `TxAlter`/`TxSet` throw
  "No transaction running" outside a transaction (JVM oracle 1.12.5).
- `agent.go` COMPLETED from the stub: `Agent` gained a state cell and a
  serialized action queue drained by one goroutine (`NewAgent`/`Send`/
  `Await`); `future` gained cooperative cancellation (`Cancel`/
  `IsCancelled` via `sync.Once` settle — body completion and cancel race
  for the single completion).
- `slices.go`: `ToSlice` falls through to any `Seqable` (sorted-set/map
  previously panicked "Unable to convert").
- `vector.go` + `internal/persistent/vector/transient.go`: `Owning`
  constructor — a slice that fits one tail node BECOMES the vector's
  storage (no copy), giving JVM `(vec array)` aliasing semantics
  (LazilyPersistentVector.createOwning; suite vec.cljc, oracle-cited in
  conformance/tests/vec-array-aliasing.clj).
