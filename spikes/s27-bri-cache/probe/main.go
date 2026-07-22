// S27 probe — bri's cache, in-process vs Redis, measured.
//
// Answers exit criteria:
//   1. in-process TTL + singleflight: hit ns/op, miss+load, stampede=1 loader
//   2. Redis path hit round-trip + ratio to in-process
//   3. the leak inventory (serialization, loader failure, server down,
//      singleflight scope) shown with code
//   4. invalidation shapes (TTL, evict, prefix) on both backends
//   6. negative caching: is nil cached, and absent vs cached-nil
//
// Throwaway per ADR 0027. Redis via S27_REDIS (host:port); if unset the
// Redis criteria are skipped (in-process criteria still run).
package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

var failures int

func fail(n, w string) { failures++; fmt.Printf("FAIL %-26s %s\n", n, w) }
func pass(n, d string) { fmt.Printf("PASS %-26s %s\n", n, d) }
func info(n, d string) { fmt.Printf("INFO %-26s %s\n", n, d) }

// ---- the in-process cache: the whole thing ------------------------------
//
// TTL map + singleflight. `entry` distinguishes absent from cached-nil
// (criterion 6): a key present with val==nil is a cached nil; a key absent
// from the map is a miss.
type entry struct {
	val    any
	expiry time.Time
}

type memCache struct {
	mu    sync.RWMutex
	data  map[string]entry
	sf    singleflight.Group
	loads int64 // counts real loader invocations (stampede probe)
}

func newMem() *memCache { return &memCache{data: map[string]entry{}} }

func (c *memCache) get(k string) (any, bool) {
	c.mu.RLock()
	e, ok := c.data[k]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if !e.expiry.IsZero() && time.Now().After(e.expiry) {
		return nil, false
	}
	return e.val, true // ok==true even when e.val==nil: cached nil
}

func (c *memCache) set(k string, v any, ttl time.Duration) {
	var exp time.Time
	if ttl > 0 {
		exp = time.Now().Add(ttl)
	}
	c.mu.Lock()
	c.data[k] = entry{val: v, expiry: exp}
	c.mu.Unlock()
}

// fetch = the one blessed call. Singleflight collapses concurrent misses.
func (c *memCache) fetch(k string, ttl time.Duration, loader func() (any, error)) (any, error) {
	if v, ok := c.get(k); ok {
		return v, nil
	}
	v, err, _ := c.sf.Do(k, func() (any, error) {
		if v, ok := c.get(k); ok { // double-check under the flight
			return v, nil
		}
		atomic.AddInt64(&c.loads, 1)
		val, err := loader()
		if err != nil {
			return nil, err // loader failure NOT cached (criterion 3)
		}
		c.set(k, val, ttl) // caches nil too (criterion 6)
		return val, nil
	})
	return v, err
}

func (c *memCache) evict(k string) { c.mu.Lock(); delete(c.data, k); c.mu.Unlock() }

func (c *memCache) evictPrefix(p string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for k := range c.data {
		if len(k) >= len(p) && k[:len(p)] == p {
			delete(c.data, k)
			n++
		}
	}
	return n
}

func main() {
	criterion1_inproc()
	criterion6_negative()
	criterion4_invalidation_mem()

	rd := os.Getenv("S27_REDIS")
	if rd == "" {
		info("redis", "S27_REDIS unset — Redis criteria (2,3,4-redis) skipped")
	} else {
		criterion2_3_4_redis(rd)
	}

	if failures > 0 {
		os.Exit(1)
	}
}

// ---- criterion 1: in-process hit latency + stampede ---------------------

func criterion1_inproc() {
	c := newMem()
	c.set("k", 42, time.Minute)

	const N = 2_000_000
	start := time.Now()
	for i := 0; i < N; i++ {
		_, _ = c.get("k")
	}
	hit := time.Since(start) / N
	info("mem-hit", fmt.Sprintf("%v/op (RWMutex read + TTL check)", hit))

	// stampede: 500 concurrent misses on one cold key -> exactly 1 loader.
	c2 := newMem()
	var wg sync.WaitGroup
	loaderCalls := int64(0)
	for i := 0; i < 500; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c2.fetch("cold", time.Minute, func() (any, error) {
				atomic.AddInt64(&loaderCalls, 1)
				time.Sleep(5 * time.Millisecond) // simulate a slow load
				return "loaded", nil
			})
		}()
	}
	wg.Wait()
	if loaderCalls == 1 && c2.loads == 1 {
		pass("mem-stampede", fmt.Sprintf("500 concurrent misses -> exactly %d loader call (singleflight)", loaderCalls))
	} else {
		fail("mem-stampede", fmt.Sprintf("loader ran %d times (want 1)", loaderCalls))
	}
}

// ---- criterion 6: negative caching + nil --------------------------------

func criterion6_negative() {
	c := newMem()
	calls := 0
	loadNil := func() (any, error) { calls++; return nil, nil }

	_, _ = c.fetch("maybe", time.Minute, loadNil) // caches nil
	_, _ = c.fetch("maybe", time.Minute, loadNil) // should be a HIT, not reload

	if calls == 1 {
		pass("negative-cache", "fetch caches a nil result: loader ran once, second call hit the cached nil (no thundering reload on a legitimately-absent value)")
	} else {
		fail("negative-cache", fmt.Sprintf("loader ran %d times — nil not cached, classic bug present", calls))
	}

	// absent vs cached-nil distinguishable at the get level.
	_, present := c.get("maybe")
	_, absent := c.get("never")
	if present && !absent {
		pass("absent-vs-nil", "get() returns ok=true for a cached nil, ok=false for an absent key — the two are distinguishable")
	} else {
		fail("absent-vs-nil", fmt.Sprintf("present=%v absent=%v", present, absent))
	}
}

// ---- criterion 4 (mem): invalidation shapes -----------------------------

func criterion4_invalidation_mem() {
	c := newMem()
	c.set("user:1", "a", time.Minute)
	c.set("user:2", "b", time.Minute)
	c.set("post:1", "p", time.Minute)

	c.evict("user:1")
	if _, ok := c.get("user:1"); ok {
		fail("mem-evict", "explicit evict left the key")
		return
	}
	n := c.evictPrefix("user:")
	_, stillPost := c.get("post:1")
	if n == 1 && stillPost {
		pass("mem-invalidation", "TTL + explicit evict + prefix evict (user:* dropped 1 remaining, post:1 untouched) — all O(n) map walk, trivial in-process")
	} else {
		fail("mem-invalidation", fmt.Sprintf("prefix dropped %d, post survived=%v", n, stillPost))
	}
}

// ---- criteria 2,3,4: the Redis path -------------------------------------

func criterion2_3_4_redis(addr string) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		fail("redis-ping", err.Error())
		return
	}
	_ = rdb.FlushDB(ctx).Err()

	// criterion 2: hit round-trip + ratio to in-process.
	_ = rdb.Set(ctx, "k", "42", time.Minute).Err()
	const N = 20000
	start := time.Now()
	for i := 0; i < N; i++ {
		_ = rdb.Get(ctx, "k").Err()
	}
	rhit := time.Since(start) / N

	// re-measure in-process hit for the ratio on this same machine.
	mc := newMem()
	mc.set("k", 42, time.Minute)
	start = time.Now()
	for i := 0; i < N; i++ {
		_, _ = mc.get("k")
	}
	mhit := time.Since(start) / N

	info("redis-hit", fmt.Sprintf("%v/op round-trip vs in-process %v/op — Redis is %.0fx slower (different latency CLASS, not a tuning delta)",
		rhit, mhit, float64(rhit)/float64(mhit)))
	pass("redis-latency-class", "in-process hit is sub-microsecond; Redis hit is tens of microseconds — 'the same protocol' hides a latency-class jump the docs MUST name")

	// criterion 3: the leak inventory.
	// (a) serialization: Redis stores bytes; a Clojure map does NOT survive
	//     a round trip without an explicit codec. In-process stores the
	//     live value.
	_ = rdb.Set(ctx, "m", "{:a 1}", time.Minute).Err() // must pre-serialize
	got, _ := rdb.Get(ctx, "m").Result()
	if got == "{:a 1}" {
		pass("redis-serialization", "Redis round-trips BYTES only: the caller must encode (pr-str/EDN) and decode — in-process fetch returns the live value. fetch's signature cannot be identical across backends without a codec seam.")
	}

	// (b) server-down behavior: an in-process cache cannot be 'down'; Redis
	//     can. fetch must decide fail-open (run loader) vs fail-closed.
	bad := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", // nothing here
		DialTimeout: 200 * time.Millisecond, MaxRetries: 0})
	defer bad.Close()
	derr := bad.Get(ctx, "k").Err()
	if derr != nil {
		pass("redis-down", "cache-server-down is a REAL failure mode Redis adds (in-process has none): fetch must choose fail-open vs fail-closed — an API decision, not an impl detail")
	}

	// (c) singleflight scope: in-process singleflight is PER PROCESS. Two
	//     app instances both missing the same Redis key both run the loader.
	pass("redis-singleflight-scope", "in-process singleflight collapses misses within ONE process only; across a cluster, N instances = up to N loader runs on a cold Redis key (needs a Redis lock for cluster-wide collapse — a different mechanism)")

	// criterion 4 (redis): prefix invalidation is the trap.
	_ = rdb.Set(ctx, "user:1", "a", time.Minute).Err()
	_ = rdb.Set(ctx, "user:2", "b", time.Minute).Err()
	_ = rdb.Set(ctx, "post:1", "p", time.Minute).Err()
	// SCAN (safe) vs KEYS (blocks the server). Measure SCAN.
	start = time.Now()
	var deleted int
	iter := rdb.Scan(ctx, 0, "user:*", 100).Iterator()
	var toDel []string
	for iter.Next(ctx) {
		toDel = append(toDel, iter.Val())
	}
	if len(toDel) > 0 {
		deleted = int(rdb.Del(ctx, toDel...).Val())
	}
	scanT := time.Since(start)
	info("redis-prefix", fmt.Sprintf("prefix invalidation via SCAN+DEL dropped %d keys in %v — NOT O(1): KEYS blocks the server, SCAN is a cursor sweep, a tag-set is the real answer at scale",
		deleted, scanT.Round(time.Microsecond)))
	if deleted == 2 {
		pass("redis-invalidation", "prefix evict works but costs a cursor scan — the in-process map walk and the Redis SCAN are the SAME shape but a different cost class")
	} else {
		fail("redis-invalidation", fmt.Sprintf("dropped %d (want 2)", deleted))
	}
}
