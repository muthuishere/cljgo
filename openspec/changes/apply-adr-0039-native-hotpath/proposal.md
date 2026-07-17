# apply-adr-0039-native-hotpath

## Why

ADR 0039 (docs/adr/0039-native-hotpath-builtins.md, accepted, owner-directed
2026-07-17, on spikes S19/S21 evidence — spike/aot-core branch): `clojure.core`
is tree-walk-interpreted in BOTH modes, so `cljgo build` output ran `reduce`
at interpreter speed — 16× behind let-go on its own benchmark suite's worst
row, while user-code rows (`tak`, `fib`) win outright. Every fast Clojure
hosts its hot core fns natively (let-go's reduce is handwritten Go; joker's
core is Go; babashka's is GraalVM-compiled; JVM Clojure bottoms out in Java
via IReduce). cljgo already draws this exact line ~292 times in
`internBuiltins` — the hot fns simply sat on the interpreted side.

## What Changes

- New `pkg/eval/hotpath_builtins.go` (`internHotpathBuiltins`, one call line
  added in `internBuiltins`): native `reduce`, `map`, `filter`, `mapv`,
  `comp` — all arities, including the transducer forms of `map`/`filter`
  and the `reduced` short-circuit box.
- The five `core.clj` definitions are deleted (builtins intern before
  `loadCore`; a surviving defn would shadow the native) and replaced with
  pointer comments; oracle citations move to the builtins.
- Discipline (ADR 0039 §3): no bulk migration — further fns move only when
  measurement names them, one fn per PR, after re-measuring on top of this.

## Impact

- reduce (1e6): 719.3 ms → 89.4 ms wall-clock (16× → 2.0× vs let-go);
  transducers 171.8 → 69.8 ms; map+sum 1481.7 → 195.5 ms; mapv 915.7 →
  104.7 ms (1.02× — dead heat); comp chain 2027.3 → 226.3 ms. REPL improves
  identically (same fn, same var, both modes — design/00 §2 satisfied
  trivially).
- Correctness: 35-case oracle byte-identical to JVM Clojure 1.12.5; jank
  suite 234/242 (96.7%, zero failing) — identical to baseline; full gates.
- Remaining losses are named, not mysterious: 30 ms boot dominates
  small-benchmark wall-clock totals (ADR 0037 / multi-namespace emission,
  ADR 0042 — in flight on main's branches), and frequencies/group-by/into
  residuals trace to missing HAMT transients (pkg/lang/TODO.md S4 #2).
- Found en route, filed in ADR 0039, NOT fixed here: `(require-go '[math])`
  works interpreted but fails AOT ("no such namespace: math") — a live
  REPL↔binary divergence with no conformance coverage.
