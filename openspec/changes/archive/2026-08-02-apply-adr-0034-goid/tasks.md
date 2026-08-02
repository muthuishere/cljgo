# Tasks — apply-adr-0034-goid

## 1. Fast goroutine-ID lookup

- [x] 1.1 `pkg/lang/internal/goid`: split the stack-parse into a shared
  `getSlow()`; add assembly `getg()` (amd64: `(TLS)`, arm64: `g`
  register) + mirrored `runtime.g` prefix (offset via `unsafe.Offsetof`,
  verified against Go 1.26's runtime2.go) behind
  `(amd64 || arm64) && go1.26 && !go1.27`; fallback file with the
  inverse tag keeps `Get() = getSlow()`. init() cross-check panics on
  offset mismatch. Gates green.
- [x] 1.2 Tests: `Get()` == stack-parse on the main goroutine and across
  hundreds of concurrent goroutines (run under -race); microbenchmark
  `BenchmarkGoidGet`. Gates green.

## 2. Verification + provenance

- [x] 2.1 Full suite + `go test -race ./pkg/lang/ ./pkg/repl/
  ./pkg/nrepl/ ./pkg/eval/` green (binding conveyance + nREPL session
  isolation canaries).
- [x] 2.2 `BenchmarkBoot -benchmem -count=5` before/after +
  `BenchmarkGoidGet` numbers recorded; `pkg/lang/PROVENANCE.md` surgery
  entry written with the measurements.
- [x] 2.3 Evaluate ADR 0034's second lever (CurrentNS()/no-bindings fast
  path) by measurement; include only if clearly correct, else record
  why not.

## Notes

- 2.3 outcome: NOT taken. After the goid fix, a fresh CPU profile of
  BenchmarkBoot shows getDynamicBinding / CurrentNS gone from the
  top-25 cumulative list entirely (remaining samples are GC/scheduler
  noise); a CurrentNS cache would add binding-model coupling for an
  unmeasurable win. The goid fix alone sufficed, as ADR 0034 allowed.
- Boot-budget test under -race exceeds the local 250ms wall-clock
  budget on main too (pre-existing, 468ms there vs 298ms with this
  change); race canaries run with the sanctioned CLJGO_BOOT_BUDGET
  override (ADR 0024), all green.
