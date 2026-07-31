| Benchmark | cljgo-aot | glojure-aot | letgo-aot |
|---|---|---|---|
| `startup` | 6.6 ms | **3.0 ms** | 4.8 ms |
| `tak` | **36.2 ms** | 51.5 ms | 58.1 ms |
| `fib` | **25.4 ms** | 37.4 ms | 66.1 ms |
| `loop-recur` | 6.0 ms | **4.3 ms** | 37.2 ms |
| `persistent-map` | 11.4 ms | **7.3 ms** | 12.6 ms |
| `map-filter` | 6.7 ms | **4.7 ms** | 6.2 ms |
| `transducers` | 17.7 ms | **9.8 ms** | 25.7 ms |
| `reduce` | 28.1 ms | **23.6 ms** | 40.0 ms |

All three columns are native binaries compiled from the same programs —
let-go's own benchmark suite (github.com/nooga/let-go), vendored unmodified
(hyperfine, 3 warmup / 10 timed runs, wall-clock mean, startup included;
compile time not measured). `cljgo-aot` = `cljgo build`. `glojure-aot` =
gloat `-E glj` (Glojure Clojure→Go→native). `letgo-aot` = gloat `-E lglvm`
(let-go IR lowered to Go with the VM runtime linked — gloat's pure `lgl`
engine is not implemented yet). Interpreted legs (cljgo run, glj, lg,
babashka, joker, Clojure JVM) are deliberately absent here; see
`results.md` for that comparison.

Measured 2026-07-31 on darwin/arm64: cljgo **v0.8.1** (rebuilt from this
repo at HEAD with the repo Go toolchain). The `glojure-aot` and `letgo-aot`
binaries were **not rebuilt** — they are the same artifacts produced on
2026-07-24 by gloat v0.1.62 pinning Glojure v0.7.0 and let-go v1.12.2
(gloat builds with its own pinned Go toolchain), re-timed here in the same
hyperfine session on the same machine. So the timings are directly
comparable; the competitor *versions* are the 2026-07-24 ones, and a claim
about a newer Glojure or let-go release would need them rebuilt. let-go's
`transducers` needed gloat's pure-retry fallback (its LG-overrides pass
failed to build).
