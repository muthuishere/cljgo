# Spike S24 — Is "move hot core fns to Go builtins" the simple fix?

Opened 2026-07-17. Feeds **ADR 0045**. Follows S22/S23.

## Context

S22/S23 established the defect (`clojure.core` is interpreted in both modes)
and priced the structural fix (AOT-core: ~86% of the gap, but gated on
multi-namespace emission, all-or-nothing, a milestone). Before committing to
big work, this spike asks whether the competition's answer is simpler.

Research finding that motivates it: **let-go's `reduce` is not bytecode — it
is handwritten Go** (`references/let-go/pkg/rt/native_prims.go:332-357`,
439 lines of native prims). joker's core is native Go; babashka's core is
GraalVM-compiled Clojure. Every fast Clojure-on-X hosts its hot core fns
natively. cljgo already has the same mechanism — ~292 Go builtins, `range`
among them (`pkg/eval/coll_builtins.go`) — but `reduce` sits on the
interpreted side: `core/core.clj:543`.

## The one question

**If `reduce` alone moves from interpreted `core.clj` to a Go builtin — the
same pattern as the existing 292 — how much of the 15.8× let-go gap closes,
in BOTH modes, with zero semantic drift?**

## Exit criterion (written before any code, per ADR 0027)

Prototype builtin `reduce` (both arities, `reduced` short-circuit box
honored, empty/nil-coll seeding per JVM Clojure), delete the `core.clj`
definition, then:

1. **Perf**: `reduce.clj` (1e6) must drop from ~719 ms to **≤ 150 ms** total
   (≈ the S23 all-compiled floor of ~96 ms + margin). If it does → the
   incremental-native path is validated; ADR 0045 decides the hot-fn list and
   its relation to AOT-core. If it lands > 300 ms → per-element cost lives in
   the seq machinery, not the fn body; native prims are NOT the lever and the
   S23 decomposition needs revisiting.
2. **Correctness**: full gates green (`go build/vet/gofmt/test ./...`), the
   two core.clj reduce oracle cases hold, and `transducers`/`map-filter`
   benchmarks still produce correct output.
3. **Dual-mode**: the same builtin serves REPL and binary (one
   implementation, two consumers — design/00 §2). Interpreted `reduce`
   (`cljgo run`) must improve by a similar factor, proving the fix is not
   AOT-only.

## Why this can be simpler than AOT-core (the hypothesis to test)

- **Incremental** — one fn at a time; no multi-namespace emission, no
  all-or-nothing migration.
- **Existing pattern** — `range` already lives there; this is moving fns
  across a line the codebase already draws, not new architecture.
- **Both modes win** — a builtin binds one Go fn value into the var; REPL and
  binary call the same code. AOT-core only helps binaries.
- **It does NOT collect the size/startup prize** — the interpreter stays
  linked, `core.clj` still boots. AOT-core (ADR 0037) remains the structural
  goal; this spike tests the cheap 80% now.

## Method

Prototype patch to `pkg/eval` + `core/core.clj` in this worktree, measured
with the S22 harness (hyperfine, 3 warmup / 10 runs, M5 Pro, go1.26.3),
then **reverted** — the diff is frozen as `prototype.patch` in this dir;
spike code never merges (ADR 0027). Production landing goes through
ADR 0045 → OpenSpec.

## Results

See `VERDICT.md`. Raw data in `results/`.
