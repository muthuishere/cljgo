# S26 VERDICT — worker queues / background jobs

Status: CLOSED. Recommendation: **ADR 0041 T3's Oban model is correct.
The smallest production-usable cut is: Postgres rows + SKIP LOCKED
dequeue + attempt/backoff columns + NOTIFY-with-poll-fallback +
transactional enqueue. core.async gives the concurrency plumbing for
free but NOT the queue — the in-memory path is genuinely tests-only.**

Measured on Postgres 17.10 (Docker, localhost), pgx v5.10.0, Go 1.26.3,
interpreted cljgo (ADR 0040 core.async). Reproduce: `./run.sh`.

## What the probe measured

| criterion | result |
|---|---|
| 1 — free part (core.async) | `chan-queue.cljg`: 550 jobs, per-type concurrency (4 workers on one channel), retry-with-backoff, all in **~60 lines of interpreted cljgo**. 550 completed, 100 retries visible (50 flaky × 2 extra attempts). |
| 2 — the gap | in-memory model loses jobs on exit (demonstrated: accept 100, crash after 60 → 40 lost). Channels structurally cannot give durability, at-least-once, cross-instance visibility, `run_at` scheduling, or dedupe. |
| 3 — durable throughput | **1 worker: 136 jobs/s; 8 workers: 787 jobs/s** (dequeue+complete, SKIP LOCKED). Every job processed exactly once at both fan-outs. |
| 4 — wake latency | LISTEN/NOTIFY insert→wake **5.0ms**; poll @1s ≈ **500ms** mean (**~99× slower**). |
| 5 — transactional enqueue | commit → 1 account + 1 job; rollback → **both vanish**. The property a broker cannot give. PASS |
| 6 — retries | attempts [1,2,3], exponential backoff (1/2/4ms), terminal `state='dead'` + `last_error` in the row. PASS |
| 8 — visibility | one `GROUP BY state` query answers queued/running/done/dead. PASS |

## Positions per question

**How much falls out of core.async for free: the concurrency, not the
queue.** ADR 0040 makes worker pools, per-type concurrency, backpressure,
and timeout-based backoff trivial — 60 lines of interpreted cljgo is a
working in-memory job runner (criterion 1). That is real and worth
documenting. But every one of those 60 lines operates on state that dies
with the process (criterion 2). core.async is the *worker* half; it is
not the *queue* half.

**The gap is durability, and it is not negotiable for "production
usable".** The five things channels structurally cannot provide —
survive-restart, at-least-once, cross-instance visibility, future
scheduling, dedupe — are exactly the five things that separate a demo
from a job system. All five are one `CREATE TABLE` away in Postgres.

**SKIP LOCKED is the whole dequeue design, and it settles the
leader-election question.** `FOR UPDATE SKIP LOCKED` let 1 and 8 workers
each drain every job exactly once with zero coordination (criterion 3).
**No leader election is needed** — multiple app instances polling the
same table with SKIP LOCKED is safe by construction. This answers
criterion 7's open question: cron/scheduled rows work the same way (a
`run_at <= now()` predicate on the dequeue), and multiple schedulers
racing to claim the same due row is resolved by the row lock, not by
electing one. (One caveat below.)

**The single-worker throughput number is the honest warning.** 136
jobs/s at 1 worker is *round-trip bound*: each job is two localhost
queries (the UPDATE...RETURNING dequeue + the completion UPDATE). This
is not a Postgres limit — it is latency × 2 per job. Two consequences
for T3: (a) scale is horizontal (8 workers → 787/s, near-linear), and
(b) a batched dequeue (`LIMIT N` claim) is the obvious optimization if a
single instance needs more. bri should ship the simple one-at-a-time
loop and document the batch claim as the tuning lever. Do not pretend a
naive loop is high-throughput.

**NOTIFY earns its place, but poll must be the floor.** NOTIFY wakes ~100×
faster than a 1s poll (criterion 4), so latency-sensitive jobs want it.
But NOTIFY is fire-and-forget: a notification sent while no worker is
listening (deploy, crash, network blip) is *lost*, and only the poll
sweep recovers those jobs. So the ADR-0041 phrasing "LISTEN/NOTIFY +
poll fallback" is exactly right and the order matters: **poll is the
correctness floor; NOTIFY is a latency optimization layered on top**,
never the sole wake path.

**Smallest production-usable cut (the recommendation):**
```
jobs(id, queue, type, args jsonb, state, attempt, max_attempt,
     run_at, last_error, inserted_at)   -- one table
dequeue: UPDATE ... FOR UPDATE SKIP LOCKED LIMIT 1 RETURNING
enqueue(tx, type, args)                 -- takes a tx handle
wake: LISTEN/NOTIFY, poll fallback (poll is the floor)
retry: attempt++, run_at = now()+backoff, dead at max_attempt
```
That is the entire durable engine. Everything else 0041 lists (unique
jobs, per-type concurrency limits, cron) is columns + predicates on this
same table, not new machinery.

## Owner calls (options + recommendation)

1. **`:memory` in dev — keep it tests-only, per 0041?** 0041 says dev
   runs the real Postgres backend (parity) and `:memory` is tests-only.
   This spike confirms the reasoning empirically: the in-memory path
   silently loses jobs (criterion 2), so a dev that used it would teach
   the wrong mental model. **Recommend: hold the 0041 line** — dev uses
   Postgres, `:memory` (the core.async runner) is drain-and-assert in
   tests only. No change requested; flagging because it is a place a
   future "make dev faster" impulse would push wrongly.

2. **`run_at` scheduling precision vs poll interval.** With poll as the
   floor, a job scheduled for `now()+30s` fires within `30s +
   poll_interval` worst case. For cron that is fine; for "send in
   exactly 5 minutes" it is a documented ±interval. Options: accept the
   ±interval (simple), or shorten the poll interval for a dedicated
   "scheduled" queue (more DB load). **Recommend: accept ±interval, make
   it configurable per queue.** Owner call because it is a promised-
   precision decision, not a mechanism one.

## What I did NOT prove

- **NOTIFY-loss recovery end to end** — I measured NOTIFY latency and
  argued (not demonstrated) that poll recovers missed notifications; I
  did not kill a listener mid-notify and show the poll sweep catching up.
- **Crash mid-job / at-least-once redelivery** — the `running` state is
  in the schema, but I did not simulate a worker dying with a job in
  `running` and show a reaper returning it to `available`. This is the
  single most important durability path and it is DESIGN ONLY here.
- **Throughput past 8 workers or with contention from a live web tier** —
  numbers are a quiet box; no measurement of dequeue under concurrent
  inserts or a shared pool.
- **Unique jobs / dedupe** — asserted as "a unique index", not built.
- **cron parsing / calendar scheduling** — only `run_at` timestamp
  scheduling is shown; recurring cron is design only.
- **Interpreted-mode reach to the Postgres backend** — the durable probe
  is pure Go; I did not run the durable queue *through* interpreted cljgo
  (the S25 Go-shim model would apply identically, but it is unproven for
  jobs).
