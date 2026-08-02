# Performance

**The published, current numbers live on the
[benchmarks page](https://muthuishere.github.io/cljgo/reference/benchmarks/)**
(source: `site/src/content/docs/reference/benchmarks.md`), and the raw tables
the harness writes are `benchmark/results.md`, `benchmark/results-aot.md` and
`benchmark/web/results.md`. This file used to carry its own copy of all of
them; it does not any more, because a third copy of a number is a number that
rots. What is left here is the material that exists nowhere else: the two CI
budgets, the `--seal-core` measurement, and the campaign history.

Performance is priority 4 and gated like conformance, not asserted. Reproduce
the head-to-heads with **`bash benchmark/run.sh`** and
**`bash benchmark/run-aot.sh`**; see
[`benchmark/README.md`](../benchmark/README.md) for methodology.

**Building an AOT binary is one command** — `cljgo build -o hello hello.clj`.
It emits Go and invokes `go build`, so it needs the Go toolchain on `PATH`
(`cljgo run` / `cljgo repl` do not); it strips by default; and it links the
**compiled** core and no evaluator (ADR 0046), which is why a compiled binary
starts in ~5 ms instead of the interpreter's ~39 ms boot. It does still link
the reader — `read`/`read-string` are runtime core fns — so the accurate
claim is *evaluator-free*, not *interpreter-free*.

## The two CI budgets

Both run inside plain `go test ./...`, and are host-relative because a CI
runner is not your laptop (ADR 0024) — override with `CLJGO_BOOT_BUDGET` and
`CLJGO_PERF_RATIO_MAX`:

- **Interpreter boot** — `TestBootUnderBudget`, 250 ms locally
  (`pkg/eval/boot_test.go`). Measured **38.8 ms** (2026-08-02), up from
  31.7 ms at v0.8.2; a 250 ms ceiling does not notice that kind of drift.
- **Emitted vs handwritten Go** — `TestFactorialPerfBudget`, 15× ceiling
  (`pkg/emit/perf_test.go`). Measured **~3.8×** (2026-08-02) — under the ~10×
  target of design/00 §1.4 (ADR 0067; naive emission was ~168×, boxed
  emission ~35×). The 15× gate is a regression guard, not the target.

## The 2026-07-23 campaign — ADRs 0063–0067

Five decisions moved emitted code from "correct but boxed" to competitive:
chunk-aware `map`/`filter`/`count`/`keep` (JVM 32-element realization
parity, ADR 0063), the IFn2 2-arg seam (no `[]any` box per reduce step),
direct-call emission for statically-known local fns (0064), the sealed-core
dirty-flag (guard elision with full `with-redefs` liveness, 0066), and an
int64 numeric-inference pass that lifts monomorphic kernels to raw typed Go
(`func tak(x, y, z int64) int64`, 0067).

### `cljgo build --seal-core` — an opt-in that is NOT a speed knob (ADR 0108)

`--seal-core` drops the redefinition guard from core-arithmetic call sites
entirely (no operator var, no dirty-flag load), giving JVM Clojure's `:inline`
semantics: a `with-redefs`/`def`/`alter-var-root` of `+ - * / < > = <= >=` is
then **not seen** at a direct 2-arg site. It is off by default and the default
emission is byte-identical to what it was before the flag existed.

Measured, so nobody has to guess (Apple M5 Pro, hyperfine `-N -w 3 -r 20`,
whole binary): `(fact 15)` × 2M **66.0 ms → 64.8 ms** (1.02× ± 0.04); a
boxed float accumulate × 5M **70.8 ms → 71.8 ms** (no gain). At the intrinsic
level `Add2` goes 5.89 → 5.85 ns/op and `LTBool` 5.06 → 4.96 ns/op. In other
words ADR 0066's dirty flag already took the win, and the single
`atomic.Bool` load it leaves behind costs ~nothing. **Turn `--seal-core` on if
you want the JVM's inlining semantics — not for speed.** Reproduce with
`CLJGO_SEAL_BENCH=1 go test ./pkg/emit -run TestSealCoreMeasure -v` and
`go test ./pkg/emit/rt -bench 'Guard|Sealed'`.

### Head-to-head tables have moved

The interpreted-runtime comparison (cljgo run / cljgo AOT / let-go / babashka
/ joker / Clojure JVM), the AOT-vs-AOT-vs-AOT head-to-head against Glojure and
let-go, and the 2026-07-23 before/after table are all on the
[benchmarks page](https://muthuishere.github.io/cljgo/reference/benchmarks/),
each carrying its own date and cljgo version. Do not re-copy them here.

## Earlier campaigns, kept honest

**AOT core (ADR 0046, spikes S22/S23).** Compiled `core.clj` and the boot
sources through cljgo's own emitter (`pkg/coreaot`): a compiled binary links
the **compiled** core and no evaluator at all (`pkg/eval` 155 → **0**
symbols in the link set, `pkg/analyzer` 63 → 0, `pkg/ast` 14 → 0 —
CI-enforced, `pkg/coreaot/imports_test.go`). At the time that took startup
27.5 → 5.5 ms; boot growth from the fundamentals batches later pushed it to
~9.5 ms, and the 2026-07-23 clawback (bulk boot refer + boot GC deferral,
now gated by `TestBootStartupBudget`) brought a hello binary to **~5 ms**,
where it still measures (5.1 ms ± 0.5, 2026-08-02). It also settled the A/B
that used to indict `cljgo build`: work in user code compiles to ~9.7× its interpreted speed, and core-heavy
programs stopped being indistinguishable between modes.

**What AOT core did not buy: size.** ADR 0023 §2 predicted ~2 MB per
compiled program. Measured then: 5.3 → 4.6 MB stripped — the tree-walker
left, but ~13k lines of compiled core arrived, and what remains is the
runtime a compiled binary genuinely needs (`pkg/lang`'s data structures and
numeric tower, `pkg/corelib`'s ~700 symbols, the reader). Dual-body emission
(ADR 0067) plus the fundamentals batches have since grown it to **7.1 MB**
(7,083,298 bytes, measured 2026-08-02). Size from here is a
dead-code and dual-body-trimming problem, not an AOT-core problem; ADR 0046
records the ~2 MB prediction as superseded by measurement.

**Boot.** Interpreter boot got 8.9× faster in v0.2.0 (211 ms → 23.7 ms) by
replacing a stack-scraping goroutine-ID lookup that was burning 73% of boot
CPU with a `getg()`-based one (ADR 0034, spike S18). It has since grown with
the core it boots — 38.8 ms today, 2026-08-02 (ADR 0019 says the budget grows
with the core, and the 250 ms gate holds — arguably too loosely).
`.github/workflows/boot-bench.yml` is a manual ubuntu-vs-macos boot
comparison kept as a permanent diagnostic.

## Web framework (bri) vs the field — ADR 0071 / spike s45

bri (cljgo's web framework) AOT-compiles to a single static `CGO_ENABLED=0`
binary and deploys as a minimal Docker image, byte-identical to the
interpreter path (dual-mode parity).

**The table that used to be here was the 2026-07-24 one, and its bri row is
superseded** — it claimed 78,126 req/s and "comparable-or-better throughput
… top tier with Rust/Deno/Bun/http-kit", which did not reproduce. The current
measured table, with the claims that survived and the ones that did not, is on
the [benchmarks page](https://muthuishere.github.io/cljgo/reference/benchmarks/);
the raw harness output is `benchmark/web/results.md`. Reproduce with
`bash benchmark/web/run.sh`. Full original write-up:
`spikes/s45-bri-aot-docker/VERDICT.md`.
