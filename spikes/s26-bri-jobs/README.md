# S26 — bri's worker queues / background jobs (ADR 0041 T3)

ADR 0040 (core.async on Go channels) is merged: `chan`, `go`, `alts!`,
`timeout` are real goroutines over real Go channels, no IOC transform.
ADR 0041 T3 then asserts the Oban model — "jobs are rows in YOUR
Postgres, workers are goroutines, LISTEN/NOTIFY + poll fallback,
`:memory` is tests-only."

## The one question

Now that core.async is first-class, **how much of a job system falls
out of it for free** — and what is the smallest thing on top that is
genuinely production-usable?

The trap this spike exists to avoid: core.async makes an in-memory
queue so easy that it looks finished. It is not, and the gap needs to
be demonstrated rather than argued.

## Exit criteria (written before any code)

1. **The free part, measured.** A worker pool — enqueue, dispatch,
   per-type concurrency, retry with backoff — built *purely* on
   `clojure.core.async` in interpreted cljgo. LOC counted, throughput
   measured (jobs/sec). This is the honest "what you get for free"
   number.
2. **The gap, demonstrated not asserted.** A probe that shows in-memory
   jobs are LOST on process exit, with the count of jobs accepted vs
   jobs completed. Plus a by-construction list of everything channels
   structurally cannot give: durability, at-least-once, visibility,
   scheduling beyond `timeout`, dedupe, backpressure across restarts.
3. **Durable variant measured.** Postgres-backed queue with
   `FOR UPDATE SKIP LOCKED` dequeue against a real Postgres:
   throughput (jobs/sec) and dequeue latency, at 1 and N workers,
   contention shown.
4. **Wake latency: LISTEN/NOTIFY vs poll**, measured. ADR 0041 says
   "LISTEN/NOTIFY + poll fallback"; this produces the number that
   justifies (or kills) the extra machinery.
5. **Transactional enqueue demonstrated**: a job row committing
   atomically with the domain write, and a rollback losing BOTH. This
   is the single property that makes the Oban model worth choosing over
   a broker, so it gets a probe.
6. **Retries/backoff/failure** exercised on a genuinely failing
   handler: attempt counts, backoff schedule, the terminal state, and
   what an operator can see.
7. **Scheduling** (run-at / cron on the same table): the shape, and
   an answer to whether multiple app instances need leader election or
   whether SKIP LOCKED settles it.
8. **Visibility**: the smallest query set that answers "what is queued
   / running / failed / retrying", written and run.
9. **VERDICT.md** names the smallest production-usable cut, takes a
   position on `:memory` vs Postgres parity in dev, states what was
   NOT proven, and routes owner calls with options + a recommendation.

## Non-goals

Not building `bri.jobs`. No brokers, no sidecars (ADR 0041 rules them
out; this spike does not relitigate that). Not touching `pkg/` or
`core/`.

## Layout

- `probe/` — standalone Go module: the Postgres queue, the benchmarks,
  the crash/durability probe.
- `chan-queue.cljg` — criterion 1, pure interpreted cljgo on
  `clojure.core.async`.
- `run.sh` — Postgres up, run everything, PASS/FAIL per criterion.
- `VERDICT.md`.
