# S19 results — let-go's benchmark suite, whole field, one machine

Apple M5 Pro, go1.26.3. hyperfine 3 warmup / 10 runs. Totals include startup.
Every runtime installed and measured here — no normalization. Regenerate with `report.py`.

| Benchmark | cljgo | let-go | babashka | joker | clojure-jvm |
|---|---|---|---|---|---|
| `startup` | 28.0 ms | **4.9 ms** | 10.5 ms | 8.0 ms | 295.7 ms |
| `tak` | 921.9 ms | 1.26 s | 1.14 s | 12.40 s | **492.0 ms** |
| `fib` | 961.6 ms | 1.15 s | 1.17 s | 13.16 s | **442.9 ms** |
| `loop-recur` | 68.8 ms | **37.1 ms** | 39.2 ms | 453.3 ms | 413.9 ms |
| `persistent-map` | 44.8 ms | 14.7 ms | **14.2 ms** | 32.8 ms | 412.4 ms |
| `map-filter` | 32.5 ms | **5.1 ms** | 12.4 ms | 9.6 ms | 348.6 ms |
| `transducers` | 171.8 ms | 27.9 ms | **15.7 ms** | — | 355.2 ms |
| `reduce` | 719.3 ms | 45.6 ms | **22.6 ms** | 1.48 s | 308.6 ms |

Versions: cljgo @HEAD, let-go v1.11.1, babashka v1.12.218, joker v1.9.0,
Clojure CLI 1.12.5.1645 / OpenJDK 26.0.1. joker has no transducers.
gloat + go-joker not installable (no package path / needs codegen).
