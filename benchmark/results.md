| Benchmark | cljgo-run | cljgo-aot | let-go | babashka | joker | clojure-jvm |
|---|---|---|---|---|---|---|
| `startup` | 38.5 ms | 5.0 ms | **5.0 ms** | 9.7 ms | 6.7 ms | 289.2 ms |
| `tak` | 11.47 s | **34.6 ms** | 1.33 s | 1.14 s | — | 457.9 ms |
| `fib` | 8.79 s | **24.7 ms** | 1.25 s | 1.14 s | — | 419.1 ms |
| `loop-recur` | 469.3 ms | **5.4 ms** | 36.6 ms | 37.8 ms | 437.8 ms | 397.5 ms |
| `persistent-map` | 48.5 ms | **9.4 ms** | 12.9 ms | 13.0 ms | 30.5 ms | 385.2 ms |
| `map-filter` | 39.9 ms | 5.1 ms | **4.8 ms** | 10.0 ms | 8.4 ms | 311.5 ms |
| `transducers` | 89.0 ms | 16.4 ms | 25.4 ms | **13.0 ms** | — | 315.9 ms |
| `reduce` | 61.4 ms | 26.0 ms | 22.8 ms | **20.0 ms** | 1.46 s | 302.5 ms |
