| Benchmark | cljgo-run | cljgo-aot | let-go | babashka | joker | clojure-jvm |
|---|---|---|---|---|---|---|
| `startup` | 32.6 ms | 6.5 ms | **5.7 ms** | 12.5 ms | 9.1 ms | 338.0 ms |
| `tak` | 12.10 s | 858.5 ms | 1.38 s | 1.18 s | — | **531.8 ms** |
| `fib` | 9.32 s | 975.4 ms | 1.29 s | 1.16 s | — | **438.5 ms** |
| `loop-recur` | 482.3 ms | 52.1 ms | 49.8 ms | **45.5 ms** | 447.3 ms | 462.0 ms |
| `persistent-map` | 43.6 ms | **14.6 ms** | 15.7 ms | 18.9 ms | 34.8 ms | 449.1 ms |
| `map-filter` | 32.1 ms | 7.2 ms | **5.6 ms** | 10.5 ms | 9.9 ms | 340.4 ms |
| `transducers` | 88.5 ms | 20.9 ms | 26.7 ms | **16.2 ms** | — | 381.2 ms |
| `reduce` | 97.5 ms | 60.8 ms | 25.4 ms | **22.1 ms** | 1.56 s | 355.2 ms |
