| Benchmark | cljgo-run | cljgo-aot | let-go | babashka | joker | clojure-jvm |
|---|---|---|---|---|---|---|
| `startup` | 44.3 ms | 9.5 ms | **5.7 ms** | 11.3 ms | 8.0 ms | 317.0 ms |
| `tak` | 11.65 s | **47.3 ms** | 1.33 s | 1.14 s | — | 457.4 ms |
| `fib` | 8.81 s | 743.6 ms | 1.25 s | 1.13 s | — | **433.3 ms** |
| `loop-recur` | 475.1 ms | **9.8 ms** | 37.2 ms | 38.5 ms | 435.9 ms | 379.1 ms |
| `persistent-map` | 52.5 ms | 14.5 ms | 12.8 ms | **12.1 ms** | 30.3 ms | 389.6 ms |
| `map-filter` | 45.4 ms | 9.4 ms | **4.9 ms** | 10.3 ms | 8.9 ms | 316.3 ms |
| `transducers` | 94.9 ms | 20.9 ms | 25.8 ms | **13.3 ms** | — | 322.8 ms |
| `reduce` | 66.8 ms | 31.3 ms | 24.4 ms | **21.8 ms** | 1.48 s | 318.4 ms |
