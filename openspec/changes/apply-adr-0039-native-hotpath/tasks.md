# Tasks — apply-adr-0039-native-hotpath

## 1. Native hot-path builtins

- [x] 1.1 `pkg/eval/hotpath_builtins.go`: `internHotpathBuiltins` with native
  `reduce` (2/3-arity, `reduced` box honored, `(f)` on empty 2-arity),
  `map` (transducer / 1-coll lazy / 2-coll / N-coll), `filter` (transducer /
  lazy with reject-skipping thunk), `mapv` (2/3-arity eager), `comp`
  (0/1/N-arity, right-to-left). One call line in `internBuiltins`.
- [x] 1.2 Delete the five `core.clj` definitions (shadowing hazard: builtins
  intern before `loadCore`); pointer comments left in place; oracle
  citations live at the builtins.

## 2. Verification

- [x] 2.1 35-case oracle file (all arities, infinite-seq laziness, `reduced`
  short-circuit, transducer composition incl. `into`/`transduce`/`sequence`,
  downstream `mapcat`/`keep`/`remove`/`map-indexed`) diffed byte-identical
  against JVM Clojure 1.12.5 via the `clojure` CLI.
- [x] 2.2 Gates green (`go build/vet/gofmt/test ./...`); jank suite 234/242
  (96.7%, zero failing files) — identical to the pre-change baseline.
- [x] 2.3 Benchmarks re-run against let-go v1.11.1 built from source on the
  same machine, wall-clock totals (no boot subtraction), outputs verified
  identical before timing; results frozen in ADR 0039.

## 3. Follow-ups (tracked, NOT this change)

- [ ] 3.1 HAMT transients (pkg/lang/TODO.md S4 #2) → then re-measure
  frequencies/group-by/into before considering more native moves.
- [ ] 3.2 `math` AOT divergence: fix + a conformance file that exercises
  `require-go '[math]` on both harnesses.
- [ ] 3.3 `clojure.core`-mediated perf gate in CI (ADR 0037 decision #5).
