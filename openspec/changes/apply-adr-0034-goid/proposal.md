# apply-adr-0034-goid

## Why

ADR 0034 (docs/adr/0034-goid-free-dynamic-binding-lookup.md, accepted, on
spike S18's evidence): 72.85% of boot CPU is spent inside
`pkg/lang/internal/goid.Get()`, which allocates a buffer, captures a full
`runtime.Stack()` trace, and text-parses "goroutine N" out of it — on
EVERY dynamic-var deref (`Var.getDynamicBinding`). `CurrentNS()` derefs
the dynamic `*ns*` on nearly every analyzer/eval step, so the whole
tree-walk interpreter pays this constantly; `runtime.Stack()` cost scales
with stack depth and is OS-sensitive, the coherent mechanism behind the
"ubuntu boots 20× slower" anomaly (ADR 0024's open question).

## What Changes

- `pkg/lang/internal/goid`: replace the stack-parse on supported
  configurations with a getg()-based read (petermattis/goid's technique,
  written fresh for this repo — zero external deps stands): a two-
  instruction assembly `getg()` returns the current `*g`; Go code reads
  the `goid uint64` field at an offset computed by the compiler from a
  mirrored prefix of Go 1.26's `runtime.g` struct (verified field-by-field
  against `$GOROOT/src/runtime/runtime2.go`).
- Compile-time selection via build tags: fast path only on
  `(amd64 || arm64) && go1.26 && !go1.27`; every other arch / future
  toolchain falls back to the existing `runtime.Stack()` parse —
  correctness everywhere, speed where vetted. No per-call runtime checks.
- Defense in depth: a one-shot `init()` cross-check (fast read vs stack
  parse) panics loudly at process start if the mirrored offset is ever
  wrong for the running toolchain — silent cross-goroutine binding
  corruption is the one unacceptable failure mode.
- Vendored-adjacent surgery → logged in `pkg/lang/PROVENANCE.md` with
  measured numbers.
- No change to the binding model, `Var` API, or any semantics; `goid.Get`
  keeps its `int64` contract (Go 1.26's `goid` is `uint64`; IDs are
  monotonically assigned and far below 2^63 in any real process).

## Impact

- Boot and steady-state dynamic-var access get dramatically cheaper
  (S18: the lookup was the dominant boot cost).
- New concurrency-stress test pins fast-path == stack-parse across many
  goroutines; full suite + `go test -race` on lang/repl/nrepl/eval are
  the correctness canaries (binding conveyance, nREPL session isolation).
- CI's CLJGO_BOOT_BUDGET may tighten AFTER boot-bench confirms on ubuntu
  (ADR 0034 step 3) — not part of this change.
