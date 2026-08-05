# s80 — where the AOT bri web gap actually is: the access-log write

**Question.** The s45 Docker table has bri at 39.3k req/s where plain Go
does 66.9k (59%). s76 decomposed the middleware stack *interpreted* and
found "the interpreter is the layer"; ADR 0071 has since removed the
interpreter from compiled binaries. In an AOT binary, where does the
remaining 41% go — and does it scale with load, or is it a fixed tax?

**Method.** Host (darwin/arm64, Apple M5 Pro, go1.26), not Docker: the s45
benchmark app (`benchmark/web/bri`) AOT-compiled with `cljgo build`, the
s45 Go comparator built from `benchmark/web/compare/go`, both loaded with
oha over loopback at three concurrencies. CPU via `net/http/pprof`
compiled into the emitted module (20s captures under sustained load).
`BRI_STACK=full|lean|bare` selects the middleware stack — the same knob
the Docker image exposes, so the thing measured is the thing that ships.

## 1. The engine is not the gap

10s, c=50, logs to /dev/null unless stated:

| variant                          | req/s   | vs Go bare |
|----------------------------------|--------:|-----------:|
| Go bare mux (s45 comparator)     | 110.5k  |       1.00 |
| bri `bare` (recover only)        | 111.0k  |   **1.00** |
| bri `lean` (recover + negotiate) | 109.3k  |   **0.99** |
| bri `full`, logs → /dev/null     |  81.5k  |       0.74 |
| bri `full`, logs → file          |  64.2k  |       0.58 |

**bri's compiled HTTP path is at parity with a bare Go mux.** The whole
published gap is the default stack — and 0.58 reproduces the Docker
table's 39.3/66.9 = 0.59 almost exactly, so the host decomposition
explains the published number.

## 2. It is one layer, and inside it one call

The profile of `full` (stdout on /dev/null — the *cheapest possible*
sink) is unambiguous:

- `fmt.Fprintln` → `os.Stdout.Write`: **13.5% of all CPU**, with a
  matching ~13.5% in `runtime.lock2` — every request takes one write
  syscall serialized through the single `os.File` mutex.
- Everything else the stack does is a footnote: request-id `crypto/rand`
  1.4%, `jsonEncode` 1.3%, `PushThreadBindings` 1.2%, `Keyword.Invoke`
  0.3%. s76's "negotiate is the most expensive layer" does not survive
  AOT — `lean` (which includes negotiate) ties the bare mux.

The logging *logic* is cheap. The **write discipline** was the cost: one
unbuffered fd write per request.

## 3. Scaling — the gap grows with concurrency

8s per cell, logs to a real file (the honest configuration):

| c   | full (unbuffered) | lean    | full/lean |
|----:|------------------:|--------:|----------:|
|  10 |             52.4k |  81.9k  |      0.64 |
|  50 |             64.8k | 109.9k  |      0.59 |
| 200 |             72.5k | 123.9k  |      0.58 |

Not a fixed tax: the serialized write is a contention point, so the
relative cost *worsens* as parallelism rises. 241 MB of log left behind
by ~1.5M requests (~160 B/line) is the other half of the story — the
volume is real; the synchronous discipline is what's optional.

## 4. The fix, measured — buffer the sink (ADR 0122)

`-log-line`: whole lines appended under one mutex into a 64 KiB
`bufio.Writer`, flushed every 250 ms, on high water, and on server drain.
Same app, same file sink, lines verified complete (valid JSON, count
matches requests; SIGTERM flushes the tail — 3 requests → 3 lines):

| c   | full unbuffered | full buffered | gain  | vs lean |
|----:|----------------:|--------------:|------:|--------:|
|  10 |           52.4k |         59.5k |  +14% |    0.73 |
|  50 |           64.8k |     **93.6k** | +44%  |    0.85 |
| 200 |           72.5k |    **113.1k** | +56%  |    0.91 |

At c=200 the full zero-config stack now clears the bare Go mux's c=50
number. The remaining full-vs-lean gap (~9–27%, shrinking with load) is
the sum of the small honest costs above (request-id crypto/rand, JSON
encode, thread bindings) — no single lever left worth a mechanism.

## What this excludes

- Docker (json-file log driver makes the unbuffered write *worse*, so
  these host gains are a floor for the container story, not a ceiling).
- Network: oha over loopback, same exclusion as s45.
- Cross-platform: darwin/arm64 only; absolute ms are this-session-only
  per the benchmarks discipline — only within-table ratios travel.
- Allocation per op: this spike profiled CPU shares, not B/op; s76/s77
  carry the per-layer allocation tables and nothing here contradicts them.

## VERDICT

The AOT gap was never the engine (parity with bare Go) and no longer the
interpreter (s76's finding, retired by ADR 0071) — it was one unbuffered
`fmt.Fprintln` per request. Buffering the sink recovers **+44–56%**
throughput at c≥50 while keeping "logs by default" (s76's flagged owner
decision resolves as *keep the logs, fix the write*). One mechanism
(bufio + ticker + drain flush), no second code path, no config surface.
→ ADR 0122.
