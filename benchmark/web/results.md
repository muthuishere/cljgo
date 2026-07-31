# Web benchmark — partial run, 2026-07-31, cljgo v0.8.2

**This is not the full table.** It is the two-entrant investigation that opened
spike s72: bri against clj-httpkit, plus bri at three middleware settings.
Regenerate the full eleven-runtime table with `bash benchmark/web/run.sh`.

darwin/arm64, OrbStack 29.4.0, 18 CPUs / 16 GB. oha, 50 connections, 12–15 s
per route after a 3 s warm-up. One container at a time.

| entrant | `/api/hello` req/s | p99 |
|---|---|---|
| clj-httpkit (JVM) | **77,706** | 0.93 ms |
| bri — `serve`, recover only | 58,222 | 2.10 ms |
| bri — `serve`, recover + negotiate | 53,971 | 3.14 ms |
| bri — `listen` (the zero-config default) | 31,404 | 5.32 ms |

The bri rows are **after** spikes s73/s75/s77, which cut `requestMap` from
2101 ns / 89 allocs to 900 ns / 36. End-to-end that moved the number by
**0–2%, i.e. not at all** — see below, because it is the most useful thing
on this page.

## What this says

**The published table is stale in a way that matters.** It has bri at 78,126
and clj-httpkit at 82,837 — bri ~6% behind. Today bri measures **32,927**
against httpkit's **77,706** on the same machine. httpkit reproduced within
6% of its own published figure, so the machine is comparable and the movement
is **ours**: bri lost roughly 2.4× since the pre-v0.7.0 measurement.

**42% of the loss is the default middleware stack.** `http/listen` applies
seven layers — request-id, logging, recover, cors, metrics, auto-ban,
negotiate. Turning them off takes bri from 32.9k to 57.1k and p99 from
5.69 ms to 2.29 ms. Per-request structured logging, metrics counters and
default-on per-IP abuse bookkeeping are not free at 50k req/s, and the
comparison is not like-for-like either: no other entrant in the corpus does
any of it.

**The request path is NOT the remaining gap, and that was worth learning.**
Spikes s73/s75/s77 made it 2.3x faster with 2.5x fewer allocations, and the
end-to-end number did not move (57,085 -> 58,222 bare, inside noise). At
58k req/s each request has ~17 us of budget and `requestMap` is ~0.9 us of
it — about 5%. Halving 5% is invisible.

Those wins are still real, they are just **language-wide rather than
HTTP-specific**: a transient hash-map build, zero-allocation hash-map reads,
and cached keyword hashing help every Clojure program on cljgo. They do not
close the httpkit gap.

**What is left, in order of size:** the default middleware stack (46%), then
roughly 25% in handler dispatch and the server path, which nothing here has
attributed yet. Spike s76's profile blamed interpreter scope allocation, but
it profiled the in-process evaluator, and these Docker numbers come from an
AOT binary that links **zero interpreter** — so that attribution does not
transfer, and the compiled path's 25% is still unexplained. That is the next
measurement, not the next optimisation.

## Reproducing the stack comparison

The bri app reads `BRI_STACK` (`full` | `lean` | `bare`), so one image
measures all three:

```bash
docker build -f benchmark/web/bri/Dockerfile -t bri-hello .
docker run -d -e BRI_STACK=bare -p 8080:8080 bri-hello
oha -z 12s -c 50 http://127.0.0.1:8080/api/hello
```
