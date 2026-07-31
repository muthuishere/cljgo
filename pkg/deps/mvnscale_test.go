package deps

// Scale measurements for spike s70 (the lock-staleness policy, ADR 0112).
//
// The policy question is what `cljgo build` does when build.cljgo declares a
// dependency the lock does not pin. Today `resolveDeps` sets
// `update := lock == nil`, so an EXISTING lock never refreshes and the only
// remedy is deleting it. The proposal is to compare a hash of the declared
// set and re-resolve when it moves — but "re-resolve" has two very different
// costs, and the choice between them is a scaling decision, not a taste one:
//
//	FULL     throw the lock away and resolve the whole graph again
//	MINIMAL  keep every pin whose declaration did not move; re-resolve only
//	         the changed roots and whatever they reach
//
// These benchmarks measure both against graphs of increasing width and depth,
// served by the same httptest double the correctness tests use — so the code
// path is production's and nothing touches the network.
//
// Run: go test ./pkg/deps/ -run xxx -bench BenchmarkResolve -benchtime 10x

import (
	"fmt"
	"strings"
	"testing"
)

// publishGraph publishes `width` independent root libraries, each the head of
// a `depth`-long transitive chain. Total coordinates = width * depth. Every
// library is one pure-Clojure namespace, so classification runs for real.
func publishGraph(tb testing.TB, r *mvnRepoDouble, width, depth int) []Dep {
	tb.Helper()
	var roots []Dep
	for w := 0; w < width; w++ {
		for d := 0; d < depth; d++ {
			c := Coord{Group: "scale", Artifact: fmt.Sprintf("lib%d-%d", w, d), Version: "1.0.0"}
			var edges string
			if d+1 < depth {
				edges = depsXML(depXML("scale", fmt.Sprintf("lib%d-%d", w, d+1), "1.0.0"))
			}
			ns := fmt.Sprintf("scale.lib%d_%d", w, d)
			src := fmt.Sprintf("(ns scale.lib%d-%d)\n(defn v [] %d)\n", w, d, d)
			r.publish(c, edges, map[string]string{ns + ".clj": src})
		}
		roots = append(roots, Dep{
			Name:        fmt.Sprintf("scale/lib%d-0", w),
			MvnVersion:  "1.0.0",
			MvnDeclared: true,
		})
	}
	return roots
}

// benchResolve measures ONE full resolution of a width×depth graph with a cold
// cache — the cost a manifest edit pays today if the lock is deleted, and the
// cost a FULL re-resolve would pay on every edit under the proposal.
func benchResolve(b *testing.B, width, depth int) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		r := newMvnRepo(b)
		roots := publishGraph(b, r, width, depth)
		newCache(b)
		o := r.opts(b)
		o.Update = true
		b.StartTimer()

		res, err := Resolve(roots, o)
		if err != nil {
			b.Fatalf("resolve: %v", err)
		}
		if len(res.Lock.Deps) != width*depth {
			b.Fatalf("want %d coordinates, got %d", width*depth, len(res.Lock.Deps))
		}
	}
}

func BenchmarkResolveGraph10x3(b *testing.B)  { benchResolve(b, 10, 3) }
func BenchmarkResolveGraph50x3(b *testing.B)  { benchResolve(b, 50, 3) }
func BenchmarkResolveGraph100x3(b *testing.B) { benchResolve(b, 100, 3) }
func BenchmarkResolveGraph50x6(b *testing.B)  { benchResolve(b, 50, 6) }

// BenchmarkResolveWarmLock measures resolution when a lock already pins the
// whole graph and the cache is warm — the "nothing changed" path every build
// pays, and the floor a MINIMAL re-resolve approaches as the changed set
// shrinks to zero. The gap between this and BenchmarkResolveGraph* is the
// entire prize the minimal strategy is competing for.
func benchWarm(b *testing.B, width, depth int) {
	r := newMvnRepo(b)
	roots := publishGraph(b, r, width, depth)
	newCache(b)
	o := r.opts(b)
	o.Update = true
	res, err := Resolve(roots, o)
	if err != nil {
		b.Fatalf("seed resolve: %v", err)
	}
	warm := r.opts(b)
	warm.Lock = res.Lock
	warm.Update = false

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Resolve(roots, warm); err != nil {
			b.Fatalf("warm resolve: %v", err)
		}
	}
}

func BenchmarkResolveWarm10x3(b *testing.B)  { benchWarm(b, 10, 3) }
func BenchmarkResolveWarm50x3(b *testing.B)  { benchWarm(b, 50, 3) }
func BenchmarkResolveWarm100x3(b *testing.B) { benchWarm(b, 100, 3) }
func BenchmarkResolveWarm50x6(b *testing.B)  { benchWarm(b, 50, 6) }

// --- stress: the shapes the first benchmark did not have -------------------
//
// publishGraph builds DISJOINT chains: every root reaches its own private
// transitives and no coordinate is ever reached twice. Real dependency graphs
// are nothing like that — they are DAGs with heavy sharing, where first-wins
// dedup is doing most of the work. A scaling claim measured only on disjoint
// chains is a claim about a shape nobody ships.
//
// publishDiamond gives the other extreme: `width` roots that ALL depend on the
// same shared `shared` coordinates. If dedup works, cost tracks the number of
// DISTINCT coordinates, not the number of edges.

func publishDiamond(tb testing.TB, r *mvnRepoDouble, width, shared int) []Dep {
	tb.Helper()
	var edges []string
	for s := 0; s < shared; s++ {
		c := Coord{Group: "scale", Artifact: fmt.Sprintf("shared%d", s), Version: "1.0.0"}
		r.publish(c, "", map[string]string{
			fmt.Sprintf("scale/shared%d.clj", s): fmt.Sprintf("(ns scale.shared%d)\n(def v %d)\n", s, s),
		})
		edges = append(edges, depXML("scale", fmt.Sprintf("shared%d", s), "1.0.0"))
	}
	body := depsXML(edges...)
	var roots []Dep
	for w := 0; w < width; w++ {
		c := Coord{Group: "scale", Artifact: fmt.Sprintf("root%d", w), Version: "1.0.0"}
		r.publish(c, body, map[string]string{
			fmt.Sprintf("scale/root%d.clj", w): fmt.Sprintf("(ns scale.root%d)\n(def v %d)\n", w, w),
		})
		roots = append(roots, Dep{Name: fmt.Sprintf("scale/root%d", w), MvnVersion: "1.0.0", MvnDeclared: true})
	}
	return roots
}

// TestDiamondDedupIsByDistinctCoordinate is the correctness half of the stress:
// 50 roots × 40 shared deps is 2000 EDGES but only 90 distinct coordinates. If
// the lock carried 2000 rows, or the repo were hit 2000 times, the linear
// scaling measured on disjoint chains would be a fiction on real graphs.
func TestDiamondDedupIsByDistinctCoordinate(t *testing.T) {
	r := newMvnRepo(t)
	roots := publishDiamond(t, r, 50, 40)
	newCache(t)
	o := r.opts(t)
	o.Update = true

	res, err := Resolve(roots, o)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got, want := len(res.Lock.Deps), 90; got != want {
		t.Errorf("lock has %d coordinates, want %d distinct (2000 edges must dedup)", got, want)
	}
	for path, n := range r.hits {
		if n > 1 && strings.HasSuffix(path, ".jar") {
			t.Errorf("refetched %s %d times: dedup is not preventing network work", path, n)
		}
	}
}

func benchDiamond(b *testing.B, width, shared int) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		r := newMvnRepo(b)
		roots := publishDiamond(b, r, width, shared)
		newCache(b)
		o := r.opts(b)
		o.Update = true
		b.StartTimer()
		if _, err := Resolve(roots, o); err != nil {
			b.Fatalf("resolve: %v", err)
		}
	}
}

// 50x40 = 2000 edges / 90 coordinates; 100x80 = 8000 edges / 180 coordinates.
// Cost must track the second number, not the first.
func BenchmarkResolveDiamond50x40(b *testing.B)  { benchDiamond(b, 50, 40) }
func BenchmarkResolveDiamond100x80(b *testing.B) { benchDiamond(b, 100, 80) }

// Deep, to check the BFS does not degrade with chain length.
func BenchmarkResolveGraph20x25(b *testing.B) { benchResolve(b, 20, 25) }
func BenchmarkResolveGraph1x500(b *testing.B) { benchResolve(b, 1, 500) }
