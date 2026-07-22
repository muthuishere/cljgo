# S27 — bri's cache (ADR 0041 T3)

ADR 0041 T3 gives cache one line: "in-process TTL + singleflight behind
`(cache/fetch c k f)`; same protocol over Redis when outgrown." One
line is the right ambition for a cache — this spike checks whether it
is also achievable, and what "the same protocol over Redis" silently
costs.

## The one question

Can ONE call — `(cache/fetch c k f)` — honestly serve both an
in-process cache and a shared Redis-shaped cache, or does the shared
case leak enough (serialization, failure modes, latency class,
invalidation) that pretending they are one thing is a lie?

## Exit criteria (written before any code)

1. **In-process TTL + singleflight built and measured**: hit latency
   (ns/op), miss+load latency, and a stampede probe — N concurrent
   misses on one key must produce exactly 1 loader invocation, counted.
2. **Redis path measured** against a real Redis in Docker: `fetch` hit
   round-trip, and the ratio to the in-process hit. If the ratio is
   three orders of magnitude, "the same protocol" is a documentation
   problem and the VERDICT must say so.
3. **The leak inventory, demonstrated.** For each of: value
   serialization (what Clojure values actually survive a round trip),
   loader-failure behavior, cache-server-down behavior, and
   singleflight scope (per-process vs cluster-wide) — show the
   difference between the two backends with code, not prose.
4. **Invalidation shapes costed**: TTL-only vs explicit evict vs
   prefix/tag invalidation — implemented and measured on both
   backends. Prefix invalidation on Redis is the one with a real trap
   (`KEYS` vs `SCAN` vs a tag set); measure it.
5. **Composition with `bri.config` shown for real.** Cache settings
   (`:cache {:backend ... :ttl ...}`) resolved through the SHIPPED
   `bri.config` — `conf.edn` + `APP_*` env, `APP_CACHE__TTL` →
   `[:cache :ttl]` — with precedence proven by running it, not quoted
   from the guide.
6. **Negative caching + `nil`**: does `fetch` cache a `nil` result,
   and how is "absent" distinguished from "cached nil"? Answered with
   a probe; this is the classic API bug and bri should not ship it.
7. **VERDICT.md**: position on the one-protocol question, the smallest
   honest surface, what was NOT proven, and owner calls with options +
   a recommendation.

## Non-goals

Not building `bri.cache`. Not evaluating memcached or a distributed
cache with its own consistency model. Not touching `pkg/` or `core/`.

## Layout

- `probe/` — standalone Go module: the in-process cache, the Redis
  path, the benchmarks, the stampede counter.
- `config-probe.cljg` — criterion 5, run through the real shipped
  `bri.config`.
- `run.sh` — Redis up, run everything, PASS/FAIL per criterion.
- `VERDICT.md`.
