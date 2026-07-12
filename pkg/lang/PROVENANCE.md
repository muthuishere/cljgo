# pkg/lang provenance

## Where this code came from

- **Upstream**: [Glojure](https://github.com/glojurelang/glojure)
  `pkg/lang` plus the four `internal/` packages it needs (`murmur3`,
  `seq`, `persistent/vector` — the elvish port with its own BSD
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
  vector` keeps its upstream `LICENSE` (elvish port). Design doc 02 §4
  records the hard-fork decision (option 3).

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
