| Benchmark | cljgo-aot | glojure-aot | letgo-aot |
|---|---|---|---|
| `startup` | 5.1 ms | **3.9 ms** | 4.8 ms |
| `tak` | **35.9 ms** | 51.6 ms | 58.3 ms |
| `fib` | **24.6 ms** | 37.3 ms | 65.2 ms |
| `loop-recur` | 5.5 ms | **3.6 ms** | 36.7 ms |
| `persistent-map` | 10.8 ms | **7.2 ms** | 12.7 ms |
| `map-filter` | 5.7 ms | **3.6 ms** | 5.5 ms |
| `transducers` | 18.7 ms | **9.7 ms** | 25.6 ms |
| `reduce` | 28.6 ms | **23.1 ms** | 40.1 ms |

All three columns are native binaries compiled from the same programs —
let-go's own benchmark suite (github.com/nooga/let-go), vendored unmodified
(hyperfine, 3 warmup / 10 timed runs, wall-clock mean, startup included;
compile time not measured). `cljgo-aot` = `cljgo build`. `glojure-aot` =
gloat `-E glj` (Glojure Clojure→Go→native). `letgo-aot` = gloat `-E lglvm`
(let-go IR lowered to Go with the VM runtime linked — gloat's pure `lgl`
engine is not implemented yet). Interpreted legs (cljgo run, glj, lg,
babashka, joker, Clojure JVM) are deliberately absent here; see
`results.md` for that comparison.

Measured 2026-07-31 on darwin/arm64: cljgo **v0.8.2** (rebuilt from this
repo at HEAD with the repo Go toolchain). This is a FRESH session — every
cell, cljgo's and the competitors', was re-timed together. Absolute numbers
shifted against the v0.8.1 session in both directions (Glojure's startup read
3.0 ms there and 3.9 ms here), so compare WITHIN a table, never across two. The `glojure-aot` and `letgo-aot`
binaries were **not rebuilt** — they are the same artifacts produced on
2026-07-24 by gloat v0.1.62 pinning Glojure v0.7.0 and let-go v1.12.2
(gloat builds with its own pinned Go toolchain), re-timed here in the same
hyperfine session on the same machine. So the timings are directly
comparable; the competitor *versions* are the 2026-07-24 ones, and a claim
about a newer Glojure or let-go release would need them rebuilt. let-go's
`transducers` needed gloat's pure-retry fallback (its LG-overrides pass
failed to build).
