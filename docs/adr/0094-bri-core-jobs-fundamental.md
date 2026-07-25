# ADR 0094 — `bri.core.jobs`: the fundamental in-process job queue

Date: 2026-07-25 · Status: accepted (owner, this session: *"we dont need
dependencies … we will just give fundamentals"* + *"also provide interfaces for
all"*). Sibling of ADR 0093 (`bri.core.cache`); realizes the `bri.jobs` battery
ADR 0075 sketched, scoped to the **dependency-free fundamental**.

## Context

ADR 0075 catalogued `bri.jobs` as "river/asynq-style over pgx or redis" and the
app-framework T3 tasks asked for a Postgres backend (transactional enqueue,
LISTEN/NOTIFY, retry/visibility semantics, a jobs-table schema) plus a `:memory`
backend. The Postgres backend is a large surface of **owner-territory decisions**
(schema, retry curve, visibility timeout) and couples jobs to a specific
durability story. The owner's ruling this session: ship the **fundamental** — the
pure, in-process async worker primitive that needs no dependency and no schema —
and let a user who needs durable/distributed jobs bring their own store behind
the interface.

## Decision

### 1. An interface (`Queue` protocol) + one pure impl (`local`)

Like `bri.core.cache`, the contract is a **protocol**, `Queue`
(`-submit`/`-drain`/`-stop`/`-errors`). `local` is the built-in in-process impl; a
user who wants a durable/distributed queue (Postgres, redis, SQS…) implements the
**same protocol** and passes it to the identical public `submit`/`drain`/`stop`/
`errors`. We ship the interface + fundamental; the durable backend is the user's.

```clojure
(require '[bri.core.jobs :as jobs])
(def q (jobs/local {:email/welcome (fn [{:keys [to]}] (send-welcome to))}
                   {:workers 4}))
(jobs/submit q :email/welcome {:to "a@b.c"})   ; a worker runs the handler
(jobs/drain q)                                  ; block until all submitted jobs finish
(jobs/stop q)                                   ; stop the workers
;; bring your own durable backend: (reify jobs/Queue (-submit [_ t p] …) …)
```

### 2. `local` — a core.async worker pool, no dependency

`local handlers opts` starts `:workers` (default 4) goroutines, each a
`(go (loop [] (when-let [job (<! ch)] …)))` over a buffered channel (ADR 0040
core.async — **already shipped**, no new dependency). A job is `{:type … :payload
…}`; the worker looks the type up in `handlers` and invokes `(handler payload)`.
Handler values may be **vars** (`#'h`) so they stay live like http handlers
(a var is invokable). A throwing handler is caught and recorded in `errors`; one
bad job never kills a worker. `submit` tracks an outstanding count so `drain`
blocks until every submitted job has run (the drain-and-assert seam for tests).
`stop` closes the channel. Pure Clojure — **no Go shim, no dependency**
(`install: nil`); registered last in `bri.Specs()` (after `cljg.os`/`bri.core.cache`)
so it does not shift genbri's gensym numbering.

### 3. Durability, retries, cron, Postgres — the user's job (the fundamental line)

The fundamental deliberately does **not** bake a jobs-table schema, a retry/backoff
curve, visibility timeouts, or LISTEN/NOTIFY — those are opinionated, dependency-
and-schema-heavy decisions. A user who needs at-least-once durability implements
`Queue` over their own Postgres/redis (transactional enqueue on their own `tx`,
their own retry policy). If a blessed durable backend is ever wanted it returns as
its own opt-in ADR, isolated like `pkg/bri/db` — not here.

## Consequences

- A working background-job primitive (typed dispatch, worker pool, drain, error
  capture) ships with zero new dependencies and zero binary cost when unused.
- Dual-harness parity is structural (pure Clojure over core.async, no Go shim).
- Users who need durability implement `Queue`; the fundamental does not grow a
  schema or a dependency to chase it.

## Not chosen

- **A Postgres/durable backend now** — schema + retry + visibility are
  owner-territory and dependency/coupling-heavy; the pure primitive is the
  deliverable, the durable backend is a user's `Queue` impl.
- **A retry/backoff policy in the fundamental** — an in-process best-effort worker
  has no durable store to retry from; retries belong to a durable backend.
- **Bounded/blocking submit semantics** — `submit` uses a large buffered channel;
  backpressure tuning is `:buffer`, and a durable backend would define its own.
