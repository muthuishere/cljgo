// cache_test.go — the cljg.cache behavior suite (ADR 0093). No JVM oracle
// (a cljgo fundamental). Covers the pure ops, singleflight under a real goroutine
// stampede, lazy TTL expiry, and — crucially — that a USER backend implementing
// the `Cache` protocol works through the same public fns (the "interface for all"
// contract). Alias `kv` (vars are process-global).
package bri_test

import "testing"

func TestBriCoreCache(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[cljg.cache :as kv])`)
	eval(t, d, `(def c (kv/local {:ttl 60}))`)

	// fetch fills on a miss, then serves the cached value (the second fn is ignored)
	if got := evalString(t, d, `(str (kv/fetch c :a (fn [] 1)))`); got != "1" {
		t.Errorf("fetch miss = %q, want 1", got)
	}
	if got := evalString(t, d, `(str (kv/fetch c :a (fn [] 99)))`); got != "1" {
		t.Errorf("fetch hit = %q, want the cached 1", got)
	}
	// put writes through; evict/clear drop entries (forcing a refill)
	eval(t, d, `(kv/put c :b 2)`)
	if got := evalString(t, d, `(str (kv/fetch c :b (fn [] :x)))`); got != "2" {
		t.Errorf("after put = %q, want 2", got)
	}
	eval(t, d, `(kv/evict c :a)`)
	if got := evalString(t, d, `(str (kv/fetch c :a (fn [] 7)))`); got != "7" {
		t.Errorf("after evict = %q, want a refill 7", got)
	}
	eval(t, d, `(kv/clear c)`)
	if got := evalString(t, d, `(str (kv/fetch c :b (fn [] 8)))`); got != "8" {
		t.Errorf("after clear = %q, want a refill 8", got)
	}

	// singleflight: 20 concurrent misses on one key fill EXACTLY once
	eval(t, d, `(def calls (atom 0))`)
	eval(t, d, `(def c2 (kv/local {:ttl 60}))`)
	eval(t, d, `(def rs (doall (map deref
	              (doall (map (fn [_] (future (kv/fetch c2 :k
	                            (fn [] (swap! calls inc) (clojure.core/-sleep-ms 60) :V))))
	                          (range 20))))))`)
	if got := evalString(t, d, `(str @calls)`); got != "1" {
		t.Errorf("singleflight fills = %s, want exactly 1", got)
	}
	if got := eval(t, d, `(every? #(= :V %) rs)`); got != true {
		t.Errorf("singleflight results not all :V (%v)", got)
	}

	// lazy TTL expiry: after ttl elapses the entry is refilled
	eval(t, d, `(def n (atom 0))`)
	eval(t, d, `(def c3 (kv/local {:ttl 0.05}))`)
	eval(t, d, `(kv/fetch c3 :e (fn [] (swap! n inc) :first))`)
	eval(t, d, `(clojure.core/-sleep-ms 90)`)
	eval(t, d, `(kv/fetch c3 :e (fn [] (swap! n inc) :second))`)
	if got := evalString(t, d, `(str @n)`); got != "2" {
		t.Errorf("expiry refills = %s, want 2 (fill, expire, refill)", got)
	}

	// the interface: a USER backend implementing Cache works via the same fns
	eval(t, d, `(def uc (atom 0))
	            (def custom (reify kv/Cache
	                          (-fetch [_ k f] (swap! uc inc) (f))
	                          (-put   [_ k v] v)
	                          (-evict [_ k] nil)
	                          (-clear [_] nil)))`)
	if got := evalString(t, d, `(str (kv/fetch custom :z (fn [] 42)))`); got != "42" {
		t.Errorf("user Cache impl fetch = %q, want 42", got)
	}
	if got := evalString(t, d, `(str @uc)`); got != "1" {
		t.Errorf("user Cache impl not dispatched (calls=%s)", got)
	}
}
