# S27 VERDICT — cache

Status: CLOSED. Recommendation: **ADR 0041 T3's in-process TTL +
singleflight behind `(cache/fetch c k f)` is exactly right and cheap to
build. But "the same protocol over Redis when outgrown" is only HALF
true, and the honest version is: same CALL SHAPE, different CONTRACT.
The one-line ambition ships if — and only if — the docs and the type
name the differences the measurement exposes. One owner call.**

Measured on Go 1.26.3, Redis 7 (Docker, localhost), the real shipped
`bri.config`. Reproduce: `./run.sh`.

## What the probe measured

| criterion | result |
|---|---|
| 1 — in-process hit | **34 ns/op** (RWMutex read + TTL check) |
| 1 — stampede | 500 concurrent misses on one cold key → **exactly 1 loader call** (singleflight) |
| 2 — Redis hit | **57.6 µs/op** round-trip — **1744× slower** than in-process on the same machine |
| 3 — serialization | Redis round-trips **bytes only**; a Clojure map needs an explicit EDN codec. In-process returns the live value. |
| 3 — server-down | in-process cannot be "down"; Redis can — `fetch` must choose fail-open vs fail-closed |
| 3 — singleflight scope | in-process singleflight is **per-process**; a cluster of N instances runs up to N loaders on a cold Redis key |
| 4 — mem invalidation | TTL + evict + prefix evict, all an O(n) map walk — trivial |
| 4 — Redis prefix | SCAN+DEL dropped 2 keys in **190 µs** — NOT O(1); KEYS blocks the server, a tag-set is the real answer at scale |
| 5 — config | `APP_CACHE__TTL=60` won over `conf.edn`'s 300 at `[:cache :ttl]` — proven by running the REAL `bri.config`, not quoted |
| 6 — negative cache | `fetch` caches a `nil` result: second call hit the cached nil, loader ran once — no thundering reload on a legitimately-absent value |
| 6 — absent vs nil | `get` returns ok=true for a cached nil, ok=false for an absent key — distinguishable |

## Positions per question

**The one-protocol question: same call shape, DIFFERENT contract — and
that must be surfaced, not hidden.** `(cache/fetch c k f)` can read
identically against both backends. But the probe shows four things that
differ by *kind*, not degree:
1. **Latency class** (34 ns vs 57.6 µs = 1744×). In-process is a memory
   read; Redis is a network round-trip. Code that treats a cache hit as
   free is correct for in-process and wrong for Redis.
2. **Serialization.** In-process caches the live Clojure value; Redis
   caches bytes and needs an EDN/codec seam. `fetch`'s value type cannot
   be honestly identical across both without that seam being visible.
3. **Failure modes.** In-process has none; Redis can be down, and
   `fetch` must pick fail-open (run the loader, degrade to no-cache) or
   fail-closed (error). That is an API decision the in-process case never
   forces.
4. **Singleflight scope.** In-process collapses a stampede within one
   process; cluster-wide collapse needs a Redis-side lock — a different
   mechanism, not a config flag.

So the recommendation is a REFINEMENT of 0041's line, not a reversal:
ship the one `fetch` call, but (a) name the backend in the type/config so
nobody mistakes the latency class, (b) make the codec a visible seam on
the Redis backend (default EDN via the reader/`pr-str`), (c) default
Redis to **fail-open** (a cache outage should degrade to slow, not to
errors), and (d) document that singleflight is per-process. The "same
protocol" is a genuine ergonomic win precisely because the differences
are made explicit rather than pretended away.

**In-process cache is ~40 lines and should ship first.** The entire
in-process cache (TTL map + singleflight + the absent/cached-nil
distinction) is the `memCache` in `probe/main.go` — trivial, fast (34 ns
hits), and correct on the two classic bugs (stampede, negative caching).
T3 should ship it as the default `:memory` backend and add Redis as the
"when outgrown" backend behind the same `fetch`.

**Negative caching is day-one correct here, and 0041 should say so.**
The classic cache bug is re-running an expensive loader every time it
legitimately returns nothing. The probe shows `fetch` caching nil while
keeping absent-vs-cached-nil distinguishable (`entry` present with
val=nil vs key absent). bri should ship this behavior and document it —
it is a real trap the one-liner would otherwise hide.

**Config composition already works.** Cache settings resolve through the
shipped `bri.config` with the documented precedence (`APP_CACHE__TTL=60`
beat the file's 300 at `[:cache :ttl]`, criterion 5) — no new config
machinery needed; cache is just another section of the one map.

## Owner call (options + recommendation)

**Fail-open vs fail-closed on a Redis outage — the default.** When Redis
is unreachable, `fetch` can:
- (a) **fail-open**: run the loader, return the value, skip the cache —
  the app gets slower but stays up;
- (b) **fail-closed**: return an error/Result — the caller decides.
**Recommend (a) fail-open as the default, with (b) available per call.**
A cache is an optimization; its outage should degrade latency, not
availability. This is an owner call because it is a user-visible default
that trades a silent slowdown (a) against a loud failure (b), and
different shops want different defaults — but the safe default for "the
15-minute app" is that a dead cache does not take the site down.

## What I did NOT prove

- **cljgo-value round trip through the Redis codec** — I showed Redis
  stores bytes and asserted an EDN seam is needed; I did NOT run a real
  Clojure map through `pr-str` → Redis → `read-string` and confirm it
  survives (nested maps, keywords, instants). This is the seam most
  likely to leak and it is unproven.
- **Redis under concurrency / pipelining** — hit latency is a serial
  warm loop; no measurement of the connection pool under load or
  MGET/pipeline batching.
- **Tag-set invalidation** — I measured SCAN+DEL and argued a tag-set is
  the scalable answer; I did not build the tag-set.
- **TTL expiry races / active eviction** — the in-process cache expires
  lazily on read; I did not test a background sweeper or memory growth
  under many expired-but-unread keys (a real leak in a naive TTL map).
- **fetch as running cljgo** — `cache/fetch` does not exist; the probe is
  Go. Only the config composition ran through interpreted cljgo. The
  S25 Go-shim model would carry Redis into interpreted mode identically,
  but that is unproven for cache.
- **singleflight cluster lock** — named as the cluster-wide fix, not
  built or measured.
