| Benchmark | cljgo-aot | glojure-aot | letgo-aot |
|---|---|---|---|
| `startup` | 5.1 ms | 6.2 ms | **4.8 ms** |
| `tak` | **35.7 ms** | 52.1 ms | 61.4 ms |
| `fib` | **25.8 ms** | 38.9 ms | 67.0 ms |
| `loop-recur` | **5.7 ms** | 6.2 ms | 41.6 ms |
| `persistent-map` | 12.7 ms | **9.0 ms** | 11.3 ms |
| `map-filter` | 5.4 ms | 5.6 ms | **4.8 ms** |
| `transducers` | 17.8 ms | **11.4 ms** | 25.6 ms |
| `reduce` | 27.5 ms | **6.9 ms** | 40.3 ms |

All three columns are native binaries compiled from the same programs —
let-go's own benchmark suite (github.com/nooga/let-go), vendored unmodified
(hyperfine, 3 warmup / 10 timed runs, wall-clock mean, startup included;
compile time not measured). `cljgo-aot` = `cljgo build`. `glojure-aot` =
gloat `-E glj` (Glojure Clojure→Go→native). `letgo-aot` = gloat `-E lglvm`
(let-go IR lowered to Go with the VM runtime linked — gloat's pure `lgl`
engine is not implemented yet). Interpreted legs (cljgo run, glj, lg,
babashka, joker, Clojure JVM) are deliberately absent here; see
`results.md` for that comparison.

Measured 2026-08-02 on darwin/arm64 (Apple M5 Pro, go1.26.3): cljgo
**post-v0.8.9**, commit `f46b9a8`, rebuilt from this repo at HEAD with the
repo Go toolchain. **All three columns were rebuilt for this run** — the
2026-07-24 competitor artifacts no longer exist, so `glojure-aot` and
`letgo-aot` were re-built from source with gloat v0.1.62 (pinning Glojure
v0.7.0 and let-go v1.12.2, gloat's own pinned Go toolchain) and timed in the
same hyperfine session on the same machine. let-go's `transducers` again
needed gloat's pure-retry fallback (its LG-overrides pass failed to build).

Run-to-run σ in this session was 0.3–1.0 ms, so four rows — `startup`,
`loop-recur`, `persistent-map`, `map-filter` — are **ties inside the noise**
between cljgo and Glojure; the bolding marks the arithmetic minimum, not a
win. The rows that are outside the noise: cljgo wins `tak` (1.45×) and `fib`
(1.54×); Glojure wins `transducers` (1.47×) and `reduce` (3.67×).

Compare cells WITHIN this table only. The rebuilt competitor binaries do not
reproduce the 2026-07-24 artifacts (Glojure's `reduce` read 23.1 ms then and
7.4 ms now; its stripped binary was recorded at 7.5 MB then and measures
19.0 MB now — same pinned versions, same `-tags glj_aot_runtime -ldflags
"-s -w"` build). The old artifacts are gone, so the discrepancy cannot be
resolved; only the numbers above were actually measured.

None of the seven programs prints its result, so nothing in the harness
proves a compiler did not elide the work. Checked by hand for the one row
where it mattered: a printing variant of `reduce` at N = 1M / 4M / 16M gives
the correct sum on both and scales linearly on both — Glojure ~1.0 ns per
element, cljgo ~21.8 ns. The `reduce` gap is real, not dead-code
elimination.
