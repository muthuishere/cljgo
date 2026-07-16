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
