# ADR 0093 — `bri.core.cache`: the fundamental in-process cache (TTL + singleflight)

Date: 2026-07-25 · Status: accepted (owner, this session: *"we dont need
dependencies as they will cost more than what we need … let the users take that,
we will just give fundamentals"*). Realizes the `bri.cache` battery ADR 0075
sketched, under the ADR 0085 taxonomy, but **scoped to the dependency-free
fundamental** — not the redis backend.

## Context

ADR 0075 catalogued `bri.cache` as "in-proc + redis". The app-framework T3 tasks
asked for a `local` (TTL + singleflight) backend AND a `redis` (rueidis) backend
behind one protocol. rueidis is a **new dependency**. The owner's ruling this
session is explicit: cljgo ships the **fundamental** — the pure, in-process,
zero-dependency primitive — and lets a user who wants a distributed cache bring
their own client. A new dep "costs more than what we need."

## Decision

### 0. An interface (`Cache` protocol) for all backends

Per the owner (*"also provide interfaces … for all"*), the namespace's contract is
a **protocol**, `Cache`, with `-fetch`/`-put`/`-evict`/`-clear`. Our `local` is the
built-in impl; a user who wants redis/memcached/anything **implements the same
protocol** (`reify`/`defrecord`) and passes it to the identical public
`fetch`/`put`/`evict`/`clear` fns, which dispatch through the protocol. So the
fundamental and any user backend are interchangeable — we ship the interface + one
free impl, the user brings the rest. Every fundamental this session follows this
shape (`bri.core.jobs` gets a `Queue` protocol likewise).

### 1. Ship only the pure fundamental — `local`, no redis, no new dep

`bri.core.cache` is a **fetch-through in-process cache**: a TTL map with
**singleflight** (a stampede of concurrent misses for the same key triggers the
fill function exactly once). It is **pure Clojure** over `clojure.core`'s
concurrency primitives — `atom` for state, `promise`/`deliver`/`deref` for the
singleflight barrier, `swap-vals!` to elect the single filler atomically — plus
`cljg.os/now` for the monotonic clock. **No Go shim, no new dependency**
(`install: nil`, like `bri.web.html`), so it links into any app for free and is
byte-identical interpreted and AOT.

```clojure
(require '[bri.core.cache :as cache])
(def c (cache/local {:ttl 300}))            ; entries live 300 s
(cache/fetch c :user/42 #(load-user 42))    ; miss → fill once, even under a stampede
(cache/put   c :user/42 u)                  ; write through
(cache/evict c :user/42)                    ; drop one key
(cache/clear c)                             ; drop all
```

### 2. Singleflight semantics

`fetch` returns a fresh cached value if present; otherwise exactly one caller
(the one whose `promise` wins the atomic `swap-vals!` into an `inflight` map)
runs `f`, stores `{value, expires-at}`, and delivers the promise; concurrent
callers for the same key block on that promise and receive the same value. A
throwing `f` delivers an error sentinel and is retried by a waiter rather than
wedging it. TTL is seconds; expiry is lazy (checked on read) — no reaper
goroutine, keeping the primitive inert until called.

### 3. A `redis` backend is a user's job, not ours (the fundamental line)

If a user needs a distributed/shared cache they bring their own client (rueidis,
go-redis, whatever) and implement the same three ops. We do **not** take that
dependency. This is the general rule this session sets for the batteries:
**cljgo gives the dependency-free fundamental; opinionated, dependency-heavy
backends are the user's to add.** If a blessed redis backend is ever wanted it
returns as its own opt-in ADR, isolated like `pkg/bri/db` — not here.

### 4. Placement — `bri.core.cache`

Kept where the app-framework spec names it (`bri.core.cache`), beside
`bri.core.data`/`bri.core.config`/`bri.core.secrets` under the ADR 0085 `bri.core.*`
tier. Registered in `bri.Specs()` **after `cljg.os`** (its `(require [cljg.os])`
for the clock must resolve). Pure namespace → `install: nil`.

## Consequences

- A working cache with stampede protection ships with zero new dependencies and
  zero binary cost when unused-but-linked (pure Clojure, inert until called).
- Dual-harness parity is structural (no Go shim to diverge).
- Users who outgrow in-process caching add their own backend; the fundamental
  does not grow a dependency to chase that.

## Not chosen

- **A redis backend now** — a new dependency the owner explicitly declined; the
  fundamental is the deliverable.
- **A reaper goroutine for eviction** — lazy expiry keeps the primitive inert and
  allocation-free until used; a background sweeper is a future option, not needed.
- **A Go singleflight shim (x/sync)** — the pure `promise`/`swap-vals!` election
  is correct under real goroutine concurrency and adds nothing to link.
