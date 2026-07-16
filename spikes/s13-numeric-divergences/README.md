# S13 — numeric-tower divergence table

## Question

The jank `clojure-test-suite` scoreboard's biggest remaining failure bucket
is numeric: `quot` (30 fails), `mod` (25), `rem` (23), `*'` (13), `+'` (11),
`*` (9), `+` (9), `even?`/`odd?` (6 each), `int` (4), `bigdec` (3), `bigint`
(1) — spread across ~12 test files. Before anyone touches `pkg/lang`, we
need to know **exactly where cljgo's numeric tower diverges from real
Clojure 1.12.5**, cell by cell, so a fix follows from evidence instead of
guesswork.

Where does cljgo diverge from JVM Clojure across the numeric op × type
matrix, and which `pkg/lang` functions are responsible for each cluster of
divergence?

## Method

One Clojure probe file, `(prn (op args))` (or an exception-catching wrapper)
per cell, runnable unmodified by both:

- `clojure -M probes.clj` — the oracle (real Clojure 1.12.5)
- `go run ./cmd/cljgo run probes.clj` (from repo root) — the implementation
  under test

`run_probes.sh` runs both, `diff_probes.py` aligns each probe's two output
lines (oracle vs cljgo) by index and flags every mismatch. Probe rows are
drawn directly from the assertions in
`clojure-test-suite/test/clojure/core_test/{quot,mod,rem,plus,star,
plus_squote,star_squote,int,abs,even_qmark,odd_qmark,bigint,bigdec}.cljc`
so the table's coverage matches what the suite actually asserts on — plus
extra edge cases (Long/MIN_VALUE and MAX_VALUE, `-0.0`, `##Inf`/`##NaN`,
division by zero, ratios, BigInt from both `bigint` and overflow) called
for by the task brief.

## Ops covered

`quot` `rem` `mod` `+` `-` `*` `/` `+'` `-'` `*'` `inc` `dec` `inc'` `dec'`
`even?` `odd?` `int` `long` `bigint` `bigdec` `==` `abs`.

## Exit criterion

`VERDICT.md` contains:

1. A complete divergence table — every op × type-row cell in the matrix
   above, each verified against BOTH implementations (not just cljgo's
   failure), with cljgo's actual output/exception vs Clojure's.
2. A root-cause clustering: which `pkg/lang` functions are responsible for
   which groups of divergent cells (e.g. "no BigInt promotion on overflow"
   is one function, "mod truncates instead of floors" is another).
3. A recommended scope for ADR 0029 — what should be fixed, in what order,
   and what (if anything) is out of scope for a first pass.

This spike does not change `pkg/`. It only produces the table above.
