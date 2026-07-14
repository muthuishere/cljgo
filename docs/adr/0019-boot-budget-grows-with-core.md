# ADR 0019 — Interpreter-boot budget scales with clojure.core size
Date: 2026-07-15 · Status: accepted

## Context
`TestBootUnderBudget` (pkg/eval/boot_test.go) times the full interpreter boot:
Go builtins → bootstrap `defmacro` → embedded `core/core.clj` loaded into
`clojure.core` → land in `user`. The threshold was 100ms and the test comment
claimed "~1ms" of headroom — both stale. By M1's destructuring landing, boot
was already ~77ms: every `defn`/`fn` in core.clj is macroexpanded through the
tree-walk evaluator (running the full `destructure` port), so boot cost grows
roughly linearly with the number of core forms.

Growing clojure.core is the explicit mandate of ADR 0013 (library-first) and
the current work — the sequence & collection library (map/filter/reduce/take/…,
~130 defs) — which is a headline usability feature. That addition pushes
interpreter boot to ~110ms, tripping the 100ms gate purely on volume, not on
any pathological regression.

This budget concerns **interpreter** boot only. AOT-compiled binaries
(design/00 §5) precompile core and target `startup < 50ms` independently; end
users are unaffected by interpret-time boot.

## Decision
Raise the `TestBootUnderBudget` threshold to **250ms** and correct the stale
comment. The test keeps its real purpose: catching a *pathological* boot
regression (an O(n²) blowup, an accidental infinite realization, a per-form
network/disk hit), not policing the linear, expected cost of a larger core.
When core.clj grows materially, this ceiling may be revisited the same way;
a sudden jump within it still signals a real regression worth investigating.

## Consequences
Interpreter cold-start is ~110ms today with ~140ms of headroom for continued
core growth. If interpreter boot ever becomes a felt cost (REPL start latency),
the fix is caching the analyzed core AST or precompiling core into the eval
binary — an optimization, not a correctness change — rather than freezing the
size of clojure.core. The compiled-binary startup budget is untouched.
