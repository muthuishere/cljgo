| Benchmark | cljgo-run | cljgo-aot | let-go | babashka | joker | clojure-jvm |
|---|---|---|---|---|---|---|
| `startup` | 47.0 ms | **5.5 ms** | 5.5 ms | 10.9 ms | 7.0 ms | 302.0 ms |
| `tak` | 11.17 s | **37.6 ms** | 1.34 s | 1.13 s | — | 435.7 ms |
| `fib` | 8.53 s | **24.6 ms** | 1.25 s | 1.14 s | — | 427.6 ms |
| `loop-recur` | 461.5 ms | **6.0 ms** | 36.8 ms | 38.3 ms | 432.5 ms | 385.4 ms |
| `persistent-map` | 56.6 ms | **11.1 ms** | 12.8 ms | 12.5 ms | 30.3 ms | 394.1 ms |
| `map-filter` | 48.7 ms | 5.8 ms | **4.9 ms** | 10.5 ms | 8.6 ms | 314.2 ms |
| `transducers` | 96.1 ms | 17.8 ms | 25.3 ms | **13.5 ms** | — | 321.2 ms |
| `reduce` | 71.0 ms | 28.6 ms | 23.9 ms | **21.8 ms** | 1.47 s | 304.0 ms |

Measured 2026-08-02 on darwin/arm64 (Apple M5 Pro, go1.26.3), hyperfine
3 warmup / 10 timed runs, wall-clock mean, startup included. Versions:
cljgo **post-v0.8.9**, commit `f46b9a8`, rebuilt from this repo at HEAD ·
let-go `main` @ `0e56abd` (2026-07-14, untagged — after v1.11.1, includes
the loop-fusion pass), built from source with the same toolchain and flags ·
babashka v1.12.218 · joker v1.9.0 · Clojure CLI 1.12.5.1645 on OpenJDK
26.0.1. joker has no `transducers` and is skipped on `fib`/`tak`. Glojure
is absent from this table by construction; its AOT leg is in
`results-aot.md`. Run-to-run σ was under 1 ms on every sub-10 ms cell, so
`startup` (cljgo-aot 5.50 vs let-go 5.51) and `map-filter` are ties, not
wins. Compare cells WITHIN this table only.
