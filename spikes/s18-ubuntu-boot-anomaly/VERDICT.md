# Spike S18 verdict — ubuntu boot anomaly

Closed 2026-07-16. Recommendation feeds **ADR 0034** (code fix warranted).

## Exit criterion: PARTIALLY MET, with a stronger-than-required finding

The plan was to run `BenchmarkBoot -benchmem` on ubuntu-latest via a
manually-triggered workflow and compare allocs/op against local. That
half was **blocked**: `gh workflow run boot-bench.yml --ref
spike/s18-ubuntu-boot` returned `404 workflow not found on the default
branch` — GitHub only registers `workflow_dispatch` workflows that
already exist on the repo's default branch, so a workflow added on a
feature branch cannot be dispatched before it merges. This is a known
platform limitation, not a bug in the workflow file. `.github/workflows/boot-bench.yml`
is still included in this PR (per the task's allowed exception) —
once merged to `main` it becomes dispatchable and is the tool to close
the loop below.

Falling back to the sanctioned alternative (local `GOMAXPROCS`/`GOGC`
simulation) plus CPU/mem profiling of `BenchmarkBoot` produced a
**direct, code-level finding** that answers the "ours vs host"
question independently of the missing ubuntu allocs number.

## 1. Local numbers (owner's Mac, Apple M5 Pro, darwin/arm64, 18 logical CPUs)

`go test -bench=BenchmarkBoot -benchmem -count=5 -run '^$' ./pkg/eval/`

| config | ns/op (mean of 5) | B/op | allocs/op |
|---|---|---|---|
| default (GOMAXPROCS=18) | ~204.8ms | ~28.87MB | **472,455** |
| `GOMAXPROCS=2` | ~213.6ms | ~28.40MB | **472,376** |
| `GOMAXPROCS=2 GOGC=off` | ~203.8ms | ~28.38MB | **472,352** |
| `GOMAXPROCS=2 GOGC=50` | ~219.0ms | ~28.44MB | **472,403** |

Raw data: `results/local-bench*.txt`. Host fingerprint:
`results/local-host-fingerprint.txt`.

**allocs/op is stable (~472.4k) across every local configuration** —
GOMAXPROCS and GOGC don't move the allocation count, as expected (they
change scheduling/GC cadence, not the code path). Wall-time barely
moves either: 204ms → 204–219ms, at most a 7% swing. **Reducing to
2 cores and disabling/relaxing GC on comparable-class hardware does
not reproduce anything close to a 2×, let alone 20×, slowdown.** This
weakens "it's just a contended 2-vCPU runner" as a *complete*
explanation — on this hardware, core count and GC pressure alone
aren't the lever.

## 2. Profiling: the actual hot path

`go tool pprof -top` on a `-cpuprofile` of `BenchmarkBoot` (20 iterations):

```
flat  flat%  cum    cum%   function
430ms  9.7%  740ms  16.7%  runtime.recordForPanic
360ms  8.1%  450ms  10.2%  runtime.step
260ms  5.9% 1050ms  23.8%  runtime.gwrite
250ms  5.7%  250ms   5.7%  runtime.memmove
240ms  5.4%  850ms  19.2%  runtime.pcvalue
...
```

`-cum` view traces this straight to application code:

```
85.75%  runtime.systemstack
75.57%  analyzer.(*Analyzer).analyzeForm / analyzeSeq
73.08%  lang.(*Var).Deref
72.85%  lang.(*Var).getDynamicBinding
72.85%  lang.getGoroutineID (inline)
72.85%  lang/internal/goid.Get
72.85%  runtime.Stack
72.85%  runtime.traceback / traceback1 / traceback2
72.62%  eval.(*Evaluator).CurrentNS (inline)
```

**72.85% of all CPU time in `BenchmarkBoot` is spent inside
`pkg/lang/internal/goid.Get()`.** That function (`pkg/lang/internal/goid/goid.go`):

```go
func Get() int64 {
	buf := make([]byte, 32)      // allocates every call
	n := runtime.Stack(buf, false)  // full stack-trace capture + text format
	buf = buf[:n]
	// parses "goroutine <N> [running]: ..." out of the text
	...
}
```

It is called by `Var.getDynamicBinding()` (`pkg/lang/var.go:239-254`),
which every `Var.Deref()` invokes whenever the var has ever been
dynamically bound. `Evaluator.CurrentNS()` (`pkg/eval/eval.go:83`)
derefs the dynamic `clojure.core/*ns*` var on essentially every
analyzer/eval step (namespace resolution happens constantly during
`analyzeForm`/`analyzeSeq`/`parseInvoke`/`resolveVar`), so this single
inefficient goroutine-ID lookup — a full `runtime.Stack()` capture +
byte-slice allocation + text parse, done to extract one `int64` — is
the dominant cost of interpreter boot.

The memory profile corroborates: top allocators are `Map.clone`,
`Reader.annotate`, `Map.Assoc`, `Eval` — all expected core-loading
work — plus a steady per-call 32-byte allocation from `goid.Get`
spread across ~472k Var derefs.

## 3. Why this plausibly explains a Linux-specific 20×, not just "host"

`runtime.Stack()`'s cost is proportional to current goroutine stack
depth (it walks and formats the *entire* call stack) and its internals
(`traceback`, `pcvalue`, PC/line-table lookups, page-backed stack
growth) are OS/arch-sensitive — the walking code paths differ between
darwin/arm64 and linux/amd64. cljgo's tree-walk evaluator recurses
deeply per form (analyze → analyzeSeq → parseInvoke → analyzeBody →
... , deeper still under macroexpansion), so every one of those ~472k
derefs pays a traceback whose cost scales with a stack that is already
non-trivially deep. A slower per-core clock plus a different (often
less optimized in practice) stack-walk path on the shared
`ubuntu-latest` fleet, multiplied over ~472k calls, is a coherent
mechanism for turning "runner is a bit slower" into "20× slower",
where `macos-latest` — also shared/contended, but darwin/arm64 stack
walking — only pays a 2× tax.

This is inference, not a measured ubuntu allocs/op numbers (blocked,
see §Exit criterion) — but it does not need the ubuntu number to be
actionable: **the hot path itself is a genuine, fixable inefficiency
present in the code regardless of host**, and it is disproportionate
enough (72.85% of boot CPU) that fixing it is warranted on its own
merits, and is very likely to shrink the ubuntu gap substantially as a
side effect.

## Verdict: OURS (high confidence on the bug; host-amplification is the
plausible but unconfirmed multiplier for why it hits ubuntu hardest)

Recommendation for **ADR 0034**:

1. Replace `pkg/lang/internal/goid.Get()`'s `runtime.Stack()`-based
   goroutine-ID lookup with a cheap mechanism — e.g. the
   `//go:linkname`-to-`runtime.getg` trick (`petermattis/goid`'s
   approach: reads the `*g` pointer directly as an `int64` key, no
   stack capture, no allocation, no string parsing), or restructure
   `Var`'s dynamic-binding storage to avoid a global `map[int64]*glStorage`
   lookup keyed by goroutine ID altogether (e.g. true goroutine-local
   storage via a `sync.Map` keyed by `*g`, or thread a binding frame
   through `Evaluator` instead of going through a process-wide map).
2. Specifically for boot: `CurrentNS()` re-derefs the dynamic `*ns*`
   var on every analyzer step even though boot is single-threaded and
   `*ns*` changes rarely (only on `in-ns`/`ns`) — caching the resolved
   namespace pointer between rebinds would cut a large fraction of
   these calls independent of fix (1).
3. This is vendored code (`pkg/lang`, from Glojure) — per CLAUDE.md,
   log the surgery in `pkg/lang/PROVENANCE.md`/`TODO.md` when applied.
4. After the fix, merge `.github/workflows/boot-bench.yml` (already in
   this PR) and dispatch it once on `main` to get real ubuntu
   allocs/op + wall-time, confirming whether the 20× collapses toward
   macos's ~2×. If it does, ADR 0024's open question closes cleanly.
   If a large gap remains even after the fix, that's a *second*,
   still-unexplained factor worth a follow-up spike.

No further action beyond ADR 0034 is proposed here; this spike does
not touch `pkg/lang` (spike code never merges into `pkg/`, ADR 0027).
