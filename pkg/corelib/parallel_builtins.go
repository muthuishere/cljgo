package corelib

import (
	"fmt"
	"runtime"
	"sync/atomic"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// parallel_builtins.go — Go-native support for clojure.core's
// parallelism/binding surface (fundamentals batch A4, 2026-07-23):
// seque, plus two private helpers core.clj rides on — -num-cpus (pmap's
// lookahead window, the JVM's availableProcessors analog) and
// -create-local-var (with-local-vars' anonymous dynamic Var). Registered
// into internBuiltins by ONE line (internParallelBuiltins(def)), per the
// merge-friendly discipline.
//
// Oracle-verified vs JVM Clojure 1.12.5 (2026-07-23); frozen evidence in
// conformance/tests/seque-*.clj and with-local-vars-*.clj.

// localVarCounter names the anonymous vars with-local-vars creates, so
// each prints distinctly (#'clojure.core/local-var-N; the JVM prints
// #<Var: --unnamed--> — a cosmetic deviation, the vars are equally
// un-interned).
var localVarCounter atomic.Int64

func internParallelBuiltins(def func(name string, fn func(args ...any) any) *lang.Var) {
	// (seque s) / (seque n-or-q s) -> a seq drawing from s, with a
	// producer goroutine keeping up to n items (default 100) realized
	// ahead of the consumer. PARALLELISM MODEL (goroutine-natural,
	// documented honestly): where the JVM fills a LinkedBlockingQueue
	// from the agent pool, cljgo runs ONE producer goroutine feeding a
	// buffered channel of capacity n; the producer blocks when the
	// buffer is full (so it may be realized up to n+1 elements ahead —
	// n buffered + 1 in hand), and a panic while realizing the source is
	// re-raised on the consuming side in order, after all
	// already-buffered items. Results are the source's items, in order
	// (deterministic). Like the JVM's, a seque whose consumer walks away
	// mid-stream keeps its producer parked (there, an agent thread on a
	// full queue; here, a goroutine on a full channel).
	// oracle: (seque (range 10)) => (0 1 2 3 4 5 6 7 8 9);
	// (seque 3 (map inc (range 5))) => (1 2 3 4 5); the result is a
	// lazy seq.
	def("seque", func(args ...any) any {
		var n int64 = 100
		var s any
		switch len(args) {
		case 1:
			s = args[0]
		case 2:
			n = lang.AsInt64(args[0])
			s = args[1]
		default:
			panic(fmt.Errorf("wrong number of args (%d) passed to: seque", len(args)))
		}
		if n < 1 {
			n = 1
		}
		type item struct {
			v   any
			err any
		}
		ch := make(chan item, n)
		go func() {
			defer close(ch)
			defer func() {
				if r := recover(); r != nil {
					ch <- item{err: r}
				}
			}()
			for sq := lang.Seq(s); sq != nil; sq = sq.Next() {
				ch <- item{v: sq.First()}
			}
		}()
		var pull func() any
		pull = func() any {
			it, ok := <-ch
			if !ok {
				return nil
			}
			if it.err != nil {
				panic(it.err)
			}
			return lang.NewCons(it.v, lang.NewLazySeq(pull))
		}
		return lang.NewLazySeq(pull)
	})

	// -num-cpus: the host's logical CPU count — pmap's (+ 2 processors)
	// lookahead window (core.clj), runtime.NumCPU where the JVM asks
	// availableProcessors.
	def("-num-cpus", func(args ...any) any {
		if len(args) != 0 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: -num-cpus", len(args)))
		}
		return int64(runtime.NumCPU())
	})

	// -create-local-var: a fresh, un-interned, DYNAMIC Var with no root —
	// what with-local-vars (core.clj) let-binds its names to before
	// push-thread-bindings gives each a thread-local value, mirroring the
	// JVM's (Var/create).setDynamic. Un-interned: no namespace mapping is
	// created (the clojure.core namespace only lends its name for
	// printing).
	def("-create-local-var", func(args ...any) any {
		if len(args) != 0 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: -create-local-var", len(args)))
		}
		name := fmt.Sprintf("local-var-%d", localVarCounter.Add(1))
		return lang.NewVar(lang.NSCore, lang.NewSymbol(name)).SetDynamic()
	})
}
