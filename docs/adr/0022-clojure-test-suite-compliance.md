# ADR 0022 — clojure.core compliance via the jank clojure-test-suite
Date: 2026-07-15 · Status: proposed (owner-directed 2026-07-15)

## Context
cljgo has no external compatibility proof; let-go passes the jank-owned
**clojure-test-suite** and cites it as evidence of readiness. Owner decision
(2026-07-15): make **passing the clojure-test-suite** a first-class,
CI-tracked goal. The suite (`clojure-workspace/clojure-test-suite`,
jank-lang) is 235 `clojure.core` `.cljc` files + `edn`/`string`, written with
`clojure.test` (`deftest`/`is`/`are`/`testing`) and gated per-var by a
`when-var-exists` **portability macro** — so a dialect passes *incrementally*:
each `clojure.core` fn cljgo implements faithfully unlocks its test file. cljgo
already has clojure.test (`is`/`are`/`thrown?`, ADR 0012), a Phase-2 reader
(`.cljc` + `#?(:default …)`, 233/235 files use it), and invokable collections
(`({:a 1} :a)`, `([v] i)`) — the prerequisites exist.

## Decision
1. **Adopt the suite as the north-star compatibility metric.** Track a single
   ratcheting number: files fully passing / total. A CI gate forbids regression
   (let-go's bench-ratchet discipline; owner mandate that quality only moves
   forward).
2. **Provide a cljgo runner + portability shim, not edits to the suite's
   tests.** cljgo supplies `clojure.core-test.portability` with
   `when-var-exists` (expands to the body iff the symbol resolves in
   clojure.core at macroexpand time, else nothing) and the suite's `thrown?`
   hook, plus a runner that loads every `test/**/*.cljc`, runs `clojure.test`,
   and writes a per-file pass/fail/error scoreboard (EDN/JSON). Contributed
   back as `doc/cljgo.md` + a bb task upstream when green enough.
3. **The suite drives clojure.core completeness.** Failures are the backlog:
   the numeric tower (ratios, `bigint`/`bigdec`, `bit-*` completeness),
   missing seq/coll fns, `add-watch`/`derive`/hierarchy, arrays (`aclone`),
   `bound-fn`, char/string edge cases, metadata, exact printing. Each gap is
   closed as a normal ADR→OpenSpec→apply unit with its own conformance test
   *and* the suite file it turns green.
4. **Dual-harness still governs cljgo's OWN conformance/**; the suite is an
   additional, external gate run interpreted (clojure.test is interpreted per
   ADR 0012). Suite files are not required to run AOT in v0.

## Consequences
- A concrete, externally-defined definition of "how much Clojure is cljgo" that
  can be reported honestly (a %) instead of a feature checklist.
- Near-term work reprioritizes toward core-fn fidelity and the numeric tower —
  the areas the suite exercises hardest and where cljgo is thinnest vs let-go.
- Requires: the portability shim, the `.cljc` runner (reader must take
  `:default` and cleanly elide `:cljs`/`:clj` branches — verify Phase-2 does),
  the scoreboard + ratchet in CI, and a baseline run to set the starting %.
  Scoped in design/08 + an OpenSpec change (`/opsx:propose test-suite-compat`).
- Non-goal: passing tests for host-specific vars cljgo will never have
  (JVM-array internals, `bean`, etc.) — `when-var-exists` legitimately skips
  those; the denominator is "vars cljgo claims", tracked explicitly.

## Batch 0 landed — measured baseline (2026-07-15)
The harness is real: `cljgo suite [--dir …]` loads every `test/**/*.cljc`,
runs `clojure.test`, and writes an EDN+JSON scoreboard. Delivered:
`resolve`/`find-var`/`ns-resolve`/`var?`/`eval` var reflection
(`pkg/eval/var_builtins.go`), a minimal `ns` macro, the cljgo
`clojure.core-test.portability` shim (`when-var-exists` + `big-int?`/`lazy-seq?`,
pre-loaded at boot), the `p/thrown?` hook in `clojure.test/is`, and
reader-conditional tag-suppression so unknown foreign tags (`#cpp`) in elided
branches no longer error.

**Baseline over 242 test files (interpreted):** 34 pass, 8 fail, 82 error,
118 skipped.
- **North-star ratchet metric — files fully passing / total: 34/242 = 14.0%.**
- Vars cljgo resolves (non-skipped): 124/242 = 51.2%.

The reader was verified (decision 2 / consequence above): it takes `:default`
and elides `:cljs`/`:clj`/`:jank` in both `:require` and body forms. Most of the
82 errors are files whose var-under-test IS implemented but whose bodies call
other unimplemented core fns — Batch 1 breadth unlocks them. The CI ratchet (T2)
should pin passing-file count ≥ 34.
