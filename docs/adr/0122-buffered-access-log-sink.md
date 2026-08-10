# 0122 — The access-log sink buffers; logging stays default-on

Date: 2026-08-05 · Status: accepted · Spike: s80 (`spikes/s80-access-log-sink/`)

## Context

s76 decomposed the zero-config middleware stack interpreted and left one
decision explicitly for an ADR: logging is expensive AND a documented
product promise ("zero-config gives logs... out of the box"), so cutting
it from defaults was not a spike's call to make.

s80 re-measured in an AOT binary (post-ADR 0071) and dissolved the
dilemma. The compiled engine ties a bare Go `net/http` mux; the entire
39.3k-vs-66.9k req/s gap in the published s45 table is the default
stack, and inside it, one call: `default-log-sink`'s per-request
unbuffered `println` — one write syscall per request, serialized through
the `os.Stdout` mutex (13.5% of all CPU with a matching 13.5% of lock
contention, worsening with concurrency: full/lean = 0.64 at c=10, 0.58
at c=200). The logging logic is cheap; the write discipline was the cost.

## Decision

1. **Logging stays default-on.** The s76 question is answered: keep the
   promise, fix the write.
2. **The prod sink buffers.** `bri.web.http/default-log-sink`'s JSON path
   goes through a new host fn `-log-line` (`pkg/bri/logsink.go`): whole
   lines appended under one mutex into a 64 KiB `bufio.Writer` over
   stdout, flushed every 250 ms (ticker started lazily on first line),
   on high water (bufio), and on server drain — SIGTERM/SIGINT and a
   `:block false` stop handle both flush before returning.
3. **Dev keeps `println`.** A REPL wants the line now; the buffer is a
   prod-throughput discipline, not a default for interactive work.
4. **Tests are untouched.** `*log-sink*` / `set-log-sink!` capture before
   the sink is reached; a custom sink bypasses the buffer entirely.

Measured (s80, host + loopback, logs to a real file, two binaries from
one source state differing only in this line, run **interleaved** with
5 repeats at c=50 / 3 at c=10 and c=200; medians, ~2–3% run-to-run
spread within each arm):

| c   | before | after | ratio |
|----:|-------:|------:|------:|
|  10 | 46,676 | 52,192 | 1.12x |
|  50 | 59,694 | 88,317 | **1.48x** |
| 200 | 68,424 | 109,048 | **1.59x** |

The gain grows with concurrency — the signature of removing a contention
point, not a fixed per-request cost. Lines verified whole and in order;
SIGTERM loses nothing.

**What is NOT measured: Docker.** The published bri figure this revises
is a container figure, and every number above is host + loopback. The
container result could differ in either direction (the json-file log
driver is a different write path); it must be re-run, not extrapolated,
before this is quoted against the published table.

## Consequences

- A hard kill (SIGKILL, power) can lose up to one flush interval
  (≤250 ms) of log lines. Every serious access-logger (nginx buffered
  logs, JVM async appenders) makes this trade; the unbuffered behavior
  is one `set-log-sink!` away for anyone who wants it back.
- Log lines can trail the host program's own stdout writes by up to
  250 ms. Lines never tear — whole lines enter the buffer under the
  mutex, and flushes write whole-line prefixes of it.
- The s45/benchmark web table is now unsafe to quote for bri either way:
  the old row measured a binary that no longer exists, and the new
  numbers are host-side, so they are not a substitute for it. It must be
  RE-MEASURED in Docker, not edited (competitive-claims discipline).
- Passed the simplicity test: one mechanism, no second code path, no
  config surface, and the same-speed question is answered by
  correctness itself — a per-request synchronous fd write was never a
  semantic promise, only an accident of `println` being the handy sink.
