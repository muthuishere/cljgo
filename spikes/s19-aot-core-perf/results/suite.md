# S19 results — let-go benchmark suite, cljgo vs let-go

Apple M5 Pro, go1.26.3. hyperfine 3 warmup / 10 runs. Totals include startup.
Raw hyperfine JSON per benchmark in this directory.

| benchmark | cljgo | let-go | normalized (let-go=1.00x) |
|---|---|---|---|
| `tak` | 922.8 ms | 1243.4 ms | **0.74x** |
| `fib` | 954.1 ms | 1159.2 ms | **0.82x** |
| `loop-recur` | 67.0 ms | 37.2 ms | **1.80x** |
| `persistent-map` | 43.1 ms | 14.0 ms | **3.09x** |
| `map-filter` | 31.0 ms | 5.2 ms | **5.98x** |
| `transducers` | 165.9 ms | 25.3 ms | **6.56x** |
| `reduce` | 680.4 ms | 41.1 ms | **16.54x** |

## Calibration: let-go published (M1 Pro) vs measured here (M5 Pro)

Normalizing to let-go=1.00x is only valid if let-go reproduces consistently.
It does — a tight 1.39-1.85x band, median 1.72x:

| benchmark | published M1 | measured M5 | ratio |
|---|---|---|---|
| `fib` | 2128.0 ms | 1159.2 ms | 1.84x |
| `tak` | 2140.0 ms | 1243.4 ms | 1.72x |
| `loop-recur` | 65.3 ms | 37.2 ms | 1.76x |
| `map-filter` | 7.2 ms | 5.2 ms | 1.39x |
| `persistent-map` | 20.2 ms | 14.0 ms | 1.45x |
| `reduce` | 66.9 ms | 41.1 ms | 1.63x |
| `transducers` | 46.9 ms | 25.3 ms | 1.85x |
