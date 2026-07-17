# Spike S22 — What does `cljgo build` actually compile?

Opened 2026-07-16. Feeds **ADR 0037** (reserved).

## The one question

**In an AOT-compiled cljgo binary, is `clojure.core` compiled Go or an
interpreted tree-walk closure — and what does that cost us?**

ADR 0023 framed AOT-compiling `core.clj` as a **binary-size** fix (6.6MB → ~2MB)
with a startup side benefit (ADR 0019). This spike asks whether that framing
undersells it: if `main → rt.Boot → eval.New` means emitted binaries call
*interpreted* `clojure.core` functions, then AOT-core is a **performance**
lever, not a size cleanup — and the size/startup wins are the side benefit.

## Exit criterion (written before any code, per ADR 0027)

Run let-go's own benchmark suite (`references/let-go/benchmark/`, 7 files,
valid Clojure) on cljgo and on let-go, plus the decisive A/B:
the **same** benchmark run AOT-compiled vs interpreted on cljgo.

- **If AOT ≈ interpreted on a `clojure.core`-heavy benchmark** (ratio ≤ ~1.3×)
  **while AOT ≫ interpreted on a user-code-heavy one** → `clojure.core` is
  interpreted in binaries. The spike closes **yes**: AOT-core is a performance
  lever, and ADR 0037 must reframe ADR 0023's decision #2 accordingly.
- **If AOT ≫ interpreted on both** → `clojure.core` is already compiled or the
  cost lies elsewhere. The spike closes **no**, ADR 0023 stands as written, and
  the reduce/transducer gap needs a different explanation (runtime data
  structures, seq machinery) — no ADR, re-open as a new spike.

A finding either way must be reproducible from the committed commands.

## Method

- Corpus: `references/let-go/benchmark/*.clj`, unmodified — the competitor's
  own suite, so we cannot be accused of picking the workloads.
- Methodology: let-go's published one — `hyperfine`, 3 warmup / 10 runs,
  `/usr/bin/time -l` for RSS. Their `results.md` is the oracle for the rest of
  the field (babashka / joker / go-joker / gloat / fennel / JVM).
- Both `cljgo` and `let-go` v1.11.1 built from source on the same machine with
  identical `-trimpath -ldflags="-s -w"`.
- Field comparison normalized to **let-go = 1.00×** (their table's own
  convention), which cancels the M1-Pro-vs-M5-Pro hardware gap. The
  normalization is calibrated, not assumed — see `results/calibration.md`.
- Boot cost attributed with `-cpuprofile` / `-memprofile` on `BenchmarkBoot`.

Host: Apple M5 Pro, go1.26.3, darwin/arm64.

## Results

See `VERDICT.md`. Raw data in `results/`.

## Note on scope

This spike measures and attributes only. It writes no `pkg/` code — the
feasibility of actually emitting `core.clj` (the `pkg/eval` → `pkg/lang`
builtin move, the `rt` → `eval` edges, multi-namespace emission) is a
separate question and belongs to its own spike.
