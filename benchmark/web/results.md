# Web benchmark — partial run, 2026-07-31, cljgo v0.8.2

**This is not the full table.** It is the two-entrant investigation that opened
spike s72: bri against clj-httpkit, plus bri at three middleware settings.
Regenerate the full eleven-runtime table with `bash benchmark/web/run.sh`.

darwin/arm64, OrbStack 29.4.0, 18 CPUs / 16 GB. oha, 50 connections, 12–15 s
per route after a 3 s warm-up. One container at a time.

| entrant | `/api/hello` req/s | p99 |
|---|---|---|
| clj-httpkit (JVM) | **77,706** | 0.93 ms |
| bri — `serve`, recover only | 57,085 | 2.29 ms |
| bri — `serve`, recover + negotiate | 53,971 | 3.14 ms |
| bri — `listen` (the zero-config default) | 32,927 | 5.69 ms |

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

**The remaining ~26% is the request path**, and it is measurable in-process
without Docker (`go test ./pkg/bri -bench BenchmarkBriAdapt`). See spike s72.

## Reproducing the stack comparison

The bri app reads `BRI_STACK` (`full` | `lean` | `bare`), so one image
measures all three:

```bash
docker build -f benchmark/web/bri/Dockerfile -t bri-hello .
docker run -d -e BRI_STACK=bare -p 8080:8080 bri-hello
oha -z 12s -c 50 http://127.0.0.1:8080/api/hello
```
