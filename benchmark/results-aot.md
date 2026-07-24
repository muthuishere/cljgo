| Benchmark | cljgo-aot | glojure-aot | letgo-aot |
|---|---|---|---|
| `startup` | 4.7 ms | **3.6 ms** | 5.1 ms |
| `tak` | **36.4 ms** | 50.6 ms | 59.6 ms |
| `fib` | **24.1 ms** | 37.4 ms | 65.8 ms |
| `loop-recur` | 5.9 ms | **3.7 ms** | 37.3 ms |
| `persistent-map` | 10.5 ms | **7.4 ms** | 12.6 ms |
| `map-filter` | 6.3 ms | **3.8 ms** | 5.3 ms |
| `transducers` | 17.0 ms | **9.9 ms** | 25.4 ms |
| `reduce` | 26.8 ms | **23.2 ms** | 39.4 ms |

All three columns are native binaries compiled from the same programs —
let-go's own benchmark suite (github.com/nooga/let-go), vendored unmodified
(hyperfine, 3 warmup / 10 timed runs, wall-clock mean, startup included;
compile time not measured). `cljgo-aot` = `cljgo build`. `glojure-aot` =
gloat `-E glj` (Glojure Clojure→Go→native). `letgo-aot` = gloat `-E lglvm`
(let-go IR lowered to Go with the VM runtime linked — gloat's pure `lgl`
engine is not implemented yet). Interpreted legs (cljgo run, glj, lg,
babashka, joker, Clojure JVM) are deliberately absent here; see
`results.md` for that comparison.

Measured 2026-07-24: cljgo @HEAD (repo Go toolchain) · gloat v0.1.62
pinning Glojure v0.7.0 and let-go v1.12.2 (gloat builds with its own
pinned Go toolchain). let-go's `transducers` needed gloat's pure-retry
fallback (its LG-overrides pass failed to build).
