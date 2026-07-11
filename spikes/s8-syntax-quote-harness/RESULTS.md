VERDICT: PASS — ground-truth syntax-quote conformance harness works end-to-end against real JVM Clojure 1.12.5; goldens are byte-stable across JVM runs after Go-side gensym normalization; diff harness + injection point ready to become pkg/reader's CI.

# S8 — Syntax-quote conformance harness

Machine: darwin/arm64, go1.26.3, OpenJDK 26.0.1 (Homebrew), Clojure CLI already
installed (`/opt/homebrew/bin/clojure`) — no installation needed. Babashka
fallback never engaged.

## What generated the goldens

Real JVM Clojure **1.12.5** via the `clojure` CLI. `gen_golden.clj` runs in ns
`user` and, for each corpus line, prints `(pr-str (read-string INPUT))` — so
the reader performs syntax-quote expansion (a pure data→data transform) and the
expansion is printed **unevaluated**. Inputs whose read throws (e.g. `` `~@x ``)
are recorded as `ERR: <ExceptionSimpleName>`.

## Corpus

**58 cases** in `corpus.txt` (one input per line, `;;` comments), covering:
plain/qualified symbols, special symbols (`if` `def` `fn*` `&` stay
unqualified), `fn?`/`var`/`#'x` resolution, self-evaluating atoms (note the
ground truth: `` `true ``/`` `nil `` → `(quote true)`/`(quote nil)`, but
keywords/numbers/strings/chars print bare), lists incl. `` `() `` →
`(clojure.core/list)`, `~x` / `~(f x)` collapse, `~@xs` splicing (head, middle,
tail, lone), `~'x` escape, vectors/maps/sets incl. all three empties,
auto-gensym (single, repeated-same-name in one form → SAME symbol, two names →
distinct, and `` [`x# `x#] `` — same name in two separate syntax-quotes →
DIFFERENT symbols), realistic templates (`` `(let [x# 1] x#) ``,
`` `(when ~test ~@body) ``), nesting (`` `(a `(b ~c)) ``, `` `(a `(b ~~c)) ``,
```` ``x ````), metadata (`` `^:private x ``, `` `^{:doc "d"} (a) `` →
`clojure.core/with-meta` wrap — confirms design/01-reader.md §2.7), and one
error case (`` `~@x `` → IllegalStateException "splice not in list").

Sets are kept ≤1 element: JVM hash-set print order isn't contractual, and
multi-element map flattening relies on array-map insertion order — kept small
deliberately. `~x` at top level is also captured: `(clojure.core/unquote x)`.

## Normalizer

Gensym numbers come from a global JVM counter and differ every run
(`x__153__auto__` vs `x__9001__auto__`). `normalize.Gensyms` (Go,
`normalize/normalize.go`) rewrites `__<digits>__auto__` and the `#()`-hygiene
form `p1__<digits>#`, renumbering ids **per case by order of first
appearance** starting at 1, keyed by the original id — so repeated ids stay
equal (hygiene: same `x#` = same symbol) and distinct ids stay distinct (two
syntax-quotes never share). Idempotent; unit-tested (9 table cases +
idempotence check). It is applied on BOTH sides: `cmd/mkgolden` normalizes the
raw JVM output into `golden.txt`, and `harness.Run` normalizes candidate
output before comparing. **Verified:** two independent JVM runs (different raw
counters) produce byte-identical `golden.txt` (`task verify-stable`).

## Pipeline & files

```
corpus.txt --clojure -M gen_golden.clj--> golden.raw.txt --cmd/mkgolden(normalize)--> golden.txt
golden.txt + injected ReadString --cmd/conformance / harness.Run--> pass/fail report
```

- `corpus.txt` — 58 inputs
- `gen_golden.clj` — JVM golden generator
- `golden.txt` — committed, normalized ground truth (`IN:`/`OK:`|`ERR:` records)
- `normalize/` — the reusable Go normalizer + tests
- `harness/` — `LoadGolden`, `ReadString func(string) (string, error)` injection
  point, `Run`, report printer + tests (a hygiene-breaking fake reader is
  proven to FAIL, a semantically-equal one with different gensym numbers to PASS)
- `cmd/mkgolden`, `cmd/conformance` — pipeline CLIs
- `Taskfile.yml` — `task golden | test | conformance | verify-stable`

`go test ./...` green; `task conformance` with the stub (`readStringStub`
returning "NOT IMPLEMENTED") reports 0/58 passed and exits 1 — exactly the CI
gate shape.

## How pkg/reader CI plugs in

Lift `normalize/` and `harness/` (or import the spike module) into the real
repo, commit `corpus.txt` + `golden.txt`, and replace the stub in
`cmd/conformance` with:

```go
rs := func(src string) (string, error) {
    form, err := reader.ReadString(src)   // includes syntax-quote expansion
    if err != nil { return "", err }
    return lang.PrStr(form), nil          // Clojure-compatible printer
}
```

CI then runs the diff on every push — no JVM needed at CI time (goldens are
committed); the JVM is only needed when regenerating after corpus growth. The
same `IN:/OK:/ERR:` + normalize pattern extends to the full reader corpus
(numbers, strings, reader conditionals) and later the dual-mode conformance
suite. One prerequisite it pins down: `lang.PrStr` must print like Clojure's
`pr-str` (`(quote x)` not `'x`, `clojure.core/`-qualified symbols as written).

## Notes / gotchas found

- Goldens depend on the generator ns (`user`) — asserted in the script.
- `read-string` (plain PushbackReader) attaches no line/col meta, so goldens
  carry no positional noise; our reader's pr-str must likewise not print
  position meta.
- `` `(a `(b ~c)) `` nested expansion is fully deterministic — no gensyms
  involved unless `#` names appear, so nesting needs no special normalization.
- JVM exception classes (`IllegalStateException`) are recorded informationally;
  the harness only requires that the candidate *errors*, not that messages match.
