# cljgo benchmarks

The reproducible cross-implementation performance suite. This is the
push-button version of the ad-hoc spike comparisons (S22 / S23 / S24) — one
command, both cljgo legs, every comparable installed on the machine, no
hand-edited numbers.

```bash
bash benchmark/run.sh          # full suite → results.md
WARMUP=1 RUNS=3 bash benchmark/run.sh   # quick smoke
```

Requires `go` + [hyperfine](https://github.com/sharkdp/hyperfine). Auto-detects
`bb` (babashka), `joker`, `clj` (Clojure JVM), and let-go (via `$LETGO`, or
built from `../references/let-go` if the clone is present). Build artifacts land
in `benchmark/.build/` (gitignored); the corpus, harness, and `results.md` are
tracked.

## Why two cljgo columns

cljgo is a compiler, so a single number would lie in either direction. We
report **both legs of the same program**:

- **`cljgo-run`** — the tree-walking interpreter (`cljgo run file.clj`). This is
  the REPL / dev path. On user-code recursion it is slow, as any tree-walker is.
- **`cljgo-aot`** — the AOT-compiled native binary (`cljgo build file.clj`).
  This is what you ship. It is the leg that matters for a deployed program.

This mirrors what let-go now publishes (its VM vs its own AOT). The interesting
comparison is `cljgo-aot` vs the field; `cljgo-run` is shown so the compile
speedup is honest and visible (≈9–10× on user code).

## How an AOT binary is created

**One command — no extra steps:**

```bash
cljgo build -o hello hello.clj    # → ./hello, a standalone native binary
./hello
```

`cljgo build`:

- emits your forms to plain Go and invokes `go build`, so it **needs the Go
  toolchain** on `PATH` (unlike `cljgo run` / `cljgo repl`, which run from the
  cljgo binary alone);
- **strips by default** (`-ldflags="-s -w"`);
- links the **compiled** `clojure.core` (`pkg/coreaot`), not the interpreter —
  since ADR 0046 an emitted binary contains **zero** `pkg/eval` / `pkg/analyzer`
  / `pkg/ast` symbols (CI-enforced, `pkg/coreaot/imports_test.go`). That is why
  `cljgo-aot` startup is ~5 ms, not the interpreter's ~28 ms boot.

There is no separate "AOT core" step for users — cljgo's own core is compiled
ahead of time and checked in; your `cljgo build` just links it.

## Methodology

- hyperfine, **3 warmup / 10 timed runs**, wall-clock **mean ± σ**.
- **Totals include each runtime's startup** — never boot-subtracted. This is the
  owner's honesty bar (design/00 §1): a user waits for the whole process, so we
  report the whole process. Startup-subtracted splits appear only as analysis
  (see "the reduce gap" below), never as a headline claim.
- Every runtime is **built/installed and measured on the same machine** — no
  normalization, no quoted spec-sheet figures.
- The 7 programs under `programs/` are **let-go's own benchmark files**
  (github.com/nooga/let-go, EPL/MIT), vendored unmodified so the suite is
  self-contained and does not depend on the gitignored `references/` clones.

## File / binary sizes

Sizes are a first-class result, not a footnote (priority: small, no-JVM
deployables). Measured stripped, Apple M5 Pro, go1.26.3, at HEAD:

| artifact | size | what it is |
|---|---|---|
| **compiled cljgo program** (`cljgo build`) | **~5.3 MB** | what you actually ship — smallest runtime in the field (was ~4.6 MB pre-ADR-0067; dual-body emission grew it, re-measured 2026-07-23) |
| cljgo tool binary | ~12.7 MB | the `cljgo` CLI itself (grew from the 8.3 MB in the root README as keel + `cljgo new` templates + `pkg/coreaot` landed — worth re-checking on release) |
| let-go (stripped) | ~13 MB | bytecode VM |
| babashka | ~68 MB | GraalVM native image |
| joker | ~27 MB | Go tree-walk interpreter |
| Clojure JVM | ~385 MB | JDK + `clojure.jar` |

## Results

Regenerate with `bash benchmark/run.sh`; the table below is committed as
`results.md`. Best wall-clock per row in **bold**. `—` = not installed / skipped
(joker has no `transducers` and is skipped on the ~13× tree-recursion rows).

See [`results.md`](results.md) for the current committed run.

## The `reduce` gap — why it is our worst row, and how to try better

`reduce` is the one row where a purpose-built core (let-go, babashka) still
clearly beats us — **~2.4× let-go, ~2.8× babashka** — and, unlike the other
rows, **`cljgo build` does not help**: `cljgo-run` and `cljgo-aot` are both
slow. That is the tell.

**Why.** `reduce` is already a native Go builtin — the fold loop is Go, not
interpreted (ADR 0045; `pkg/corelib/hotpath_builtins.go`). But look at the loop
body:

```go
for !lang.IsNil(s) {
    acc = lang.Apply2(f, acc, s.First())   // ← the cost
    ...
    s = s.Next()
}
```

Every element pays for three things the fast runtimes avoid:

1. **Megamorphic dispatch** — `f` (here `+`) is a runtime *value*, invoked via
   `lang.Apply2`'s generic `IFn` dispatch. The emitter cannot inline it.
2. **Boxing** — `acc` and each element are `any` (`interface{}`); every
   intermediate `long` is boxed and re-unboxed per step.
3. **Seq-node allocation** — `s.First()` / `s.Next()` walks a generic `ISeq`,
   allocating a node per element even for a `range`.

Contrast `fib`, which cljgo *wins*: there `(+ a b)` is compiled to a direct
`rt.Add` intrinsic — no dispatch, no boxing, statically known. The reducing fn
in `reduce` is exactly the case that intrinsic can't reach. **Compiling doesn't
help because the loop was already native; the cost lives inside `Apply2` +
boxing, which compilation doesn't remove.** let-go specializes the fold in the
VM with an unboxed accumulator; babashka JITs it via GraalVM.

**How to try better** (Clojure-canonical, in impact order):

1. **`IReduce` / internal-reduce** — the real fix. Implement type-specific
   internal reduction on `range` and the vector/map collections (exactly how JVM
   Clojure's `reduce` bottoms out in Java via `IReduce`/`CollReduce`). Then
   `(reduce + 0 (range 1e6))` never allocates seq nodes and can keep an unboxed
   accumulator — kills costs #2 and #3 for the common case.
2. **Native seq producers** — move `map`/`filter`/`take` and the transducer
   arities to native builtins (S24 §5 candidate list). This is the same
   one-function pattern that already took `reduce` from 14.5× → 1.8×, and it is
   what the `transducers` residual (still ~1.3× babashka) is waiting on.
3. **`Apply2` fast paths / unboxing in `pkg/lang`** — the residual megamorphic
   dispatch cost (cost #1), the doc-04 §5 "performance ladder" work.

Each lands as its own small change: builtin/runtime edit + JVM-oracle
conformance cases + jank suite run + the (still-owed) `clojure.core`-mediated CI
perf gate that ADR 0037 decision #5 mandated. Sequencing this is an ADR 0045
continuation, not a spec change.

## The AOT-only head-to-head (`run-aot.sh` → `results-aot.md`)

The like-for-like comparison of the three Clojure-on-Go AOT compilers:
`cljgo build` vs Glojure vs let-go, **native binaries only, no interpreted
legs** (those live in `results.md`). Glojure and let-go binaries are built
with [gloat](https://github.com/gloathub/gloat), the official automation tool
for both, which pins its own Glojure/let-go/Go versions.

```bash
# 1. cljgo binaries (also produced by run.sh's precompile stage)
#    -> .build/aot_<name>
# 2. Glojure + let-go binaries via gloat, from let-go's own AOT variants
#    of the same programs (ns + -main wrapper; upstream benchmark/gloat/):
GLOAT=path/to/gloat/bin/gloat
SRC=path/to/let-go/benchmark/gloat
cd benchmark/.build/aotcmp
for p in startup tak fib loop-recur persistent-map map-filter transducers reduce; do
  $GLOAT           "$SRC/$p.clj" -o "$p-glj" -Xprune -f -q   # Glojure engine
  $GLOAT -E lglvm  "$SRC/$p.clj" -o "$p-lg"  -Xprune -f -q   # let-go lowered
done
# 3. time them:
bash benchmark/run-aot.sh    # -> results-aot.md
```

gloat's pure `lgl` (no-VM) engine is not implemented yet; `lglvm` (IR lowered
to Go, VM runtime linked) is its shipping AOT mode.

## Provenance

Benchmark programs derived from
[let-go](https://github.com/nooga/let-go)'s `benchmark/` suite. The comparison
method (hyperfine 3/10, wall-clock, same-machine) follows let-go's published
methodology so the two projects' numbers are directly comparable. The AOT
head-to-head compiles let-go's own `benchmark/gloat/` variants of the same
programs (identical bodies, wrapped in `(ns …)` + `(defn -main …)` because
gloat needs an entry point).
