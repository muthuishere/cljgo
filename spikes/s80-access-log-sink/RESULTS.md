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

10s, c=50, logs to /dev/null unless stated. **These are single-shot
cells, not repeated** — treat them as the decomposition that pointed at
the sink, not as precision figures. Only §4's table is interleaved and
repeated. Given the ~2–3% run-to-run spread measured there, the three
top rows (110.5 / 111.0 / 109.3k) are inside each other's noise, which
is the basis for calling it a TIE and not a win:

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

8s per cell, logs to a real file (the honest configuration). Single-shot
again; the *direction* (relative cost worsening with concurrency) is
what this table is for, and §4's repeated run independently reproduces
that direction on the real before/after pair:

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

**Method (corrected).** The first pass of this spike measured the two
arms in separate runs, one cell each — which is exactly the
cross-session diff this repo's own benchmarks discipline forbids, and it
reported no spread. Redone properly: both arms built from ONE source
state differing only in the `default-log-sink` line (the `after` binary
is the branch; the `before` binary is that tree with just that line
reverted, regenerated and rebuilt), then run **interleaved**
before/after/before/after, 5 repeats at c=50 and 3 at c=10/200, 6 s each,
logs to a real file. Medians below, with the observed range.

| c   | before (median) | after (median) | ratio | before range | after range |
|----:|----------------:|---------------:|------:|-------------:|------------:|
|  10 |          46,676 |         52,192 | 1.12x | 46.0–47.2k   | 51.0–52.9k  |
|  50 |          59,694 |     **88,317** | **1.48x** | 58.5–60.0k | 86.9–89.4k |
| 200 |          68,424 |    **109,048** | **1.59x** | 67.1–69.3k | 109.0–110.6k |

Run-to-run spread is ~2–3% within each arm — the gain at c≥50 is an
order of magnitude outside it. The gain **grows with concurrency**,
which is the signature of removing a contention point rather than a
fixed per-request cost, and it is the same signature the unbuffered
arm showed in §3.

Correctness verified on the shipped binary: lines arrive whole and in
order (valid JSON, count matches requests), and SIGTERM flushes the tail
(3 requests -> 3 lines, nothing lost).

## What this excludes

- **Docker — entirely.** Every number here is host + loopback, and the
  PUBLISHED bri figure this work revises is a Docker figure. Whether a
  container's json-file log driver moves the result up or down is
  **unmeasured**; an earlier draft of this file asserted it would make
  the gains a floor, which was reasoning, not measurement, and has been
  removed. The Docker table must be re-run before any of this is quoted
  against it.
- Network: oha over loopback, same exclusion as s45.
- Cross-platform: darwin/arm64 only; absolute ms are this-session-only
  per the benchmarks discipline — only within-table ratios travel.
- Allocation per op: this spike profiled CPU shares, not B/op; s76/s77
  carry the per-layer allocation tables and nothing here contradicts them.

## VERDICT

The AOT gap was never the engine (parity with bare Go) and no longer the
interpreter (s76's finding, retired by ADR 0071) — it was one unbuffered
`fmt.Fprintln` per request. Buffering the sink recovers **+48% at c=50 and
+59% at c=200** (+12% at c=10) while keeping "logs by default" (s76's flagged owner
decision resolves as *keep the logs, fix the write*). One mechanism
(bufio + ticker + drain flush), no second code path, no config surface.
→ ADR 0122.
