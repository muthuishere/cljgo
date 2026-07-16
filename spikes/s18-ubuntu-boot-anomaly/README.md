# Spike S18 — why does ubuntu boot ~20x slower than local?

ADR 0027 pipeline, stage 1. Answers ADR 0024's open question. Closes
toward **ADR 0034** only if a code fix is warranted; "it's the host"
closes with no ADR.

## Question

`TestBootUnderBudget` / `BenchmarkBoot` (`pkg/eval/boot_test.go`) time the
full interpreter boot: Go builtins → bootstrap `defmacro` → embedded
`core.clj` loaded into `clojure.core` → `user` refers core's publics.
ADR 0024 recorded:

| host | boot | vs local |
|------|------|----------|
| owner's Mac (local) | 181ms | baseline |
| `macos-latest` runner | 349ms | 2× |
| `ubuntu-latest` runner | 3.0–3.55s | **~20×**, reproduced across two independent runs |

The code is identical; only the host changed, and only ubuntu is an
outlier — macos-latest's 2× is unremarkable shared-runner variance.
20× on ubuntu specifically is not.

## Exit criterion (written before any code, per ADR 0027)

The ADR 0024 experiment executed on both hosts — `BenchmarkBoot
-benchmem -count=5` run locally and (via a manually-triggered CI
workflow, see below) on both `ubuntu-latest` and `macos-latest` — with
a verdict:

- **If allocations/op (and B/op) match across hosts while wall-time
  diverges ~20×** → it's the host (shared-runner CPU class, 2-vCPU
  contention, cgroup throttling, virtualization overhead) — close, no
  ADR.
- **If allocations/op diverge** → it's ours — profile locally
  (`-cpuprofile`/`-memprofile`, `go tool pprof top`) and name the hot
  path in the boot code; recommendation feeds ADR 0034.

Secondary check: does `GOMAXPROCS=2` (± `GOGC=off`/`50`) alone
reproduce a large slowdown locally? If yes, the boot is
contention/GC-bound and the fix direction is allocation reduction (ADR
0019 already names candidates: cache the analyzed core AST, precompile
core). If GOMAXPROCS=2 barely moves the number locally, the 20x is not
explained by core-count alone and more likely reflects a slower CPU
class + noisy-neighbor contention on the shared ubuntu-latest fleet.

## Method

1. `BenchmarkBoot` already exists (`pkg/eval/boot_test.go:74`) — used
   as-is, no patch needed.
2. Local baseline: `go test -bench=BenchmarkBoot -benchmem -count=5
   ./pkg/eval/` on the owner's Mac.
3. CI numbers: `.github/workflows/boot-bench.yml` (repo root, NOT
   inside this spike dir — the one exception ADR 0027 allows),
   `workflow_dispatch`-only, matrix `[ubuntu-latest, macos-latest]`,
   runs the same bench plus `TestBootUnderBudget -v` with
   `CLJGO_BOOT_BUDGET=10s`, uploads `bench-output.txt` per OS.
   Triggered with `gh workflow run boot-bench.yml --ref
   spike/s18-ubuntu-boot`.
4. Local contention simulation: `GOMAXPROCS=2 go test -bench=BenchmarkBoot
   -benchmem -count=5 ./pkg/eval/`, and the same with `GOGC=off` /
   `GOGC=50`, to see whether reduced parallelism / different GC
   pressure alone reproduces a large slowdown on the owner's hardware.
5. Compare allocs/op and B/op across all of the above; profile if they
   diverge.

## Results

See `VERDICT.md`.
