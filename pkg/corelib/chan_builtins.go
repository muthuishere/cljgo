package corelib

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// registerAsync interns the core.async surface (ADR 0040, openspec
// core-async-first-class T1). The canonical vars ALL live in
// clojure.core.async (lang.NSAsync — S19 Q5: portable code requires the
// real namespace); the names M4-v0 shipped into clojure.core remain as
// REFERS of the same Vars (asyncCoreAliases below), so `#'chan` in user
// and `#'clojure.core.async/chan` are one Var. New T1 surface (put!/
// take!/offer!/poll!/promise-chan/buffer/unblocking-buffer?, and the
// async.cljg macros go-loop/alt!/alt!!) interns ONLY in the async
// namespace — the precedence principle applied to libraries.
//
// All ops are Go builtins wrapping pkg/lang runtime helpers, so the
// REPL and an AOT binary behave identically (rt.Boot runs RegisterAll
// too). The macro half of the namespace (go-loop/alt!/alt!!) is
// core/async.cljg, loaded LAZILY by pkg/eval's lib provider on the
// first (require 'clojure.core.async) — macros expand at compile time,
// so compiled binaries never need the source.
//
// Every semantic below is frozen against REAL JVM core.async 1.6.681
// (spikes/s19-core-async/oracle/ + conformance/tests/chan-*.clj).
func registerAsync() {
	areg := func(name string, fn func(args ...any) any) *lang.Var {
		v := lang.NSAsync.Intern(lang.NewSymbol(name))
		v.BindRoot(&nativeFn{nm: name, fn: fn})
		return v
	}

	// (chan) / (chan buf-or-n) / (chan buf-or-n xform) /
	// (chan buf-or-n xform ex-handler). buf-or-n: nil = unbuffered,
	// n > 0 = fixed buffer, a buffer object = its size+policy. (chan 0)
	// throws — JVM parity, the ONE deliberate M4-v0 break (ADR 0040 #7;
	// oracle chan-zero => AssertionError "fixed buffers must have size
	// > 0"). A transducer requires a buffer (oracle
	// xform-unbuffered-chan-throws).
	areg("chan", func(args ...any) any {
		if len(args) > 3 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: chan", len(args)))
		}
		var c *lang.Channel
		buffered := false
		if len(args) == 0 || args[0] == nil {
			c = lang.NewChan(0)
		} else {
			switch a := args[0].(type) {
			case int64:
				if a <= 0 {
					panic(fmt.Errorf("Assert failed: fixed buffers must have size > 0"))
				}
				c = lang.NewChan(int(a))
			case *lang.BufferSpec:
				c = lang.NewChanBuffered(a)
			default:
				panic(fmt.Errorf("chan expects an integer buffer size or a buffer, got: %s", lang.PrintString(args[0])))
			}
			buffered = true
		}
		if len(args) >= 2 && args[1] != nil {
			if !buffered {
				panic(fmt.Errorf("Assert failed: buffer must be supplied when transducer is"))
			}
			var exh any
			if len(args) == 3 {
				exh = args[2]
			}
			c.SetXform(args[1], exh)
		}
		return c
	})

	// buffer / dropping-buffer / sliding-buffer → buffer specs for chan;
	// unblocking-buffer? is true for the never-block policies (oracle:
	// fixed false, dropping true, sliding true).
	bufFn := func(op string, make func(int) *lang.BufferSpec) func(...any) any {
		return func(args ...any) any {
			n, ok := oneArg(op, args).(int64)
			if !ok {
				panic(fmt.Errorf("%s expects an integer size, got: %s", op, lang.PrintString(args[0])))
			}
			return make(int(n))
		}
	}
	areg("buffer", bufFn("buffer", lang.FixedBuffer))
	areg("dropping-buffer", bufFn("dropping-buffer", lang.DroppingBuffer))
	areg("sliding-buffer", bufFn("sliding-buffer", lang.SlidingBuffer))
	areg("unblocking-buffer?", func(args ...any) any {
		b, ok := oneArg("unblocking-buffer?", args).(*lang.BufferSpec)
		return ok && b.Policy != lang.PolicyFixed
	})

	// chanArg validates a channel argument; a nil channel throws the
	// IllegalArgumentException shape (oracle nil-chan-take/put => threw
	// IllegalArgumentException — 1.6.681 throws, it does NOT block).
	chanArg := func(op string, v any) *lang.Channel {
		if v == nil {
			panic(lang.NewIllegalArgumentError(fmt.Sprintf("%s expects a channel, got nil", op)))
		}
		c, ok := v.(*lang.Channel)
		if !ok {
			panic(fmt.Errorf("%s expects a channel, got: %s", op, lang.PrintString(v)))
		}
		return c
	}

	// >! / >!! : blocking put, true unless already closed. One impl,
	// both names — `<!`/`>!`/`alts!` are legal on ANY goroutine (ADR
	// 0040 #5: there is no IOC transform, so there is no park/block
	// distinction to police; the JVM's "used not in (go ...) block"
	// throw is deliberately not mirrored).
	chanSend := func(op string) func(args ...any) any {
		return func(args ...any) any {
			if len(args) != 2 {
				panic(fmt.Errorf("wrong number of args (%d) passed to: %s", len(args), op))
			}
			return lang.ChanSend(chanArg(op, args[0]), args[1])
		}
	}
	areg(">!", chanSend(">!"))
	areg(">!!", chanSend(">!!"))

	// <! / <!! : blocking take; closed+drained => nil (aliases).
	chanRecv := func(op string) func(args ...any) any {
		return func(args ...any) any {
			return lang.ChanRecv(chanArg(op, oneArg(op, args)))
		}
	}
	areg("<!", chanRecv("<!"))
	areg("<!!", chanRecv("<!!"))

	areg("close!", func(args ...any) any {
		return lang.ChanClose(chanArg("close!", oneArg("close!", args)))
	})

	// (timeout ms) → a fresh channel that auto-closes after ms
	// milliseconds (ADR 0040 #4: semantics only — no JVM tick cache, so
	// (identical? (timeout n) (timeout n)) is false here; a documented
	// divergence no portable program observes).
	areg("timeout", func(args ...any) any {
		ms, ok := oneArg("timeout", args).(int64)
		if !ok {
			panic(fmt.Errorf("timeout expects an integer ms, got: %s", lang.PrintString(args[0])))
		}
		return lang.NewTimeout(ms)
	})

	// go* is the runtime seam: (go* thunk) spawns a goroutine running the
	// 0-arg thunk and returns its result channel (design/05 §4). `go` and
	// `thread` are macros (below) that wrap their body in (fn* [] body...)
	// and call go* — so the emitter needs NO new op: it compiles the fn
	// literal and the go* invoke like any other call, and lang.Go does the
	// real `go func(){}()` for both modes.
	//
	// go* is a cljgo-only var — real core.async has no public go* (its `go`
	// is the IOC-transform macro, which cljgo deletes per ADR 0040). It is
	// marked ^:private (fundamentals audit, core-async-audit 2026-07): the
	// go/thread macros emit a QUALIFIED clojure.core.async/go* reference,
	// which resolves fine to a private var in BOTH the interpreter and an
	// AOT binary (rt.Boot re-runs RegisterAll, so the :private meta is live
	// in compiled code too), so hiding it from ns-publics costs nothing and
	// matches the JVM surface (no go*). Verified: the full chan-* conformance
	// set — go/thread/go-loop/alt! — stays green in both harnesses with go*
	// private. The M4-v0 clojure.core refer (asyncCoreAliases) is kept for
	// back-compat, but the canonical namespace no longer advertises it.
	areg("go*", func(args ...any) any {
		return lang.Go(oneArg("go*", args))
	}).SetPrivate()

	// go / thread macros: (go body...) =>
	// (clojure.core.async/go* (fn* [] body...)). thread is an alias of go
	// (real goroutines collapse the pool distinction, ADR 0040 #5; oracle
	// thread-result => 42, thread-drains-then-nil => [7 nil]).
	goMacro := func(args ...any) any {
		// args = [&form &env body...]; wrap the body in a 0-arg fn* thunk.
		body := args[2:]
		fnParts := append([]any{symFnStar, lang.NewVector()}, body...)
		return lang.NewList(lang.NewSymbol("clojure.core.async/go*"), lang.NewList(fnParts...))
	}
	areg("go", goMacro).SetMacro()
	areg("thread", goMacro).SetMacro()

	// (alts! ports & opts) / alts!! (alias): dynamic select over read
	// ports (channels — cljgo or FOREIGN Go chans) and [chan val] write
	// ports, on reflect.Select (ADR 0040 #3). Opts: :default v (taken
	// only when nothing is ready — a CLOSED port counts as ready),
	// :priority true (listed order wins). Returns [val port].
	kwDefault := lang.NewKeyword("default")
	kwPriority := lang.NewKeyword("priority")
	altsFn := func(op string) func(...any) any {
		return func(args ...any) any {
			if len(args) == 0 {
				panic(fmt.Errorf("wrong number of args (0) passed to: %s", op))
			}
			var ports []any
			for s := lang.Seq(args[0]); s != nil; s = s.Next() {
				ports = append(ports, s.First())
			}
			hasDefault := false
			priority := false
			var defVal any
			opts := args[1:]
			for i := 0; i < len(opts); i += 2 {
				k, ok := opts[i].(lang.Keyword)
				if !ok {
					panic(fmt.Errorf("%s options must be keyword/value pairs, got: %s", op, lang.PrintString(opts[i])))
				}
				if i+1 >= len(opts) {
					panic(fmt.Errorf("%s option %s is missing a value", op, lang.PrintString(k)))
				}
				switch k {
				case kwDefault:
					hasDefault = true
					defVal = opts[i+1]
				case kwPriority:
					priority = lang.IsTruthy(opts[i+1])
				}
			}
			return lang.Alts(ports, hasDefault, defVal, priority)
		}
	}
	areg("alts!", altsFn("alts!"))
	areg("alts!!", altsFn("alts!!"))

	// (put! c v) / (put! c v fn1) / (put! c v fn1 on-caller?):
	// asynchronous put — false iff already closed, else true immediately;
	// fn1 receives the completed put's boolean. on-caller? is accepted
	// and ignored (there is no dispatch pool to protect — goroutines).
	areg("put!", func(args ...any) any {
		if len(args) < 2 || len(args) > 4 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: put!", len(args)))
		}
		var cb any
		if len(args) >= 3 {
			cb = args[2]
		}
		return lang.ChanPutAsync(chanArg("put!", args[0]), args[1], cb)
	})

	// (take! c fn1) / (take! c fn1 on-caller?): asynchronous take — fn1
	// receives the value (nil for closed+drained) from a goroutine.
	areg("take!", func(args ...any) any {
		if len(args) < 2 || len(args) > 3 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: take!", len(args)))
		}
		return lang.ChanTakeAsync(chanArg("take!", args[0]), args[1])
	})

	// (offer! c v): non-blocking put — true when accepted, nil (not
	// false) when it would block, false when closed. (poll! c):
	// non-blocking take — value or nil (oracle offer-poll =>
	// [true nil 1 nil]).
	areg("offer!", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: offer!", len(args)))
		}
		return lang.ChanOffer(chanArg("offer!", args[0]), args[1])
	})
	areg("poll!", func(args ...any) any {
		return lang.ChanPoll(chanArg("poll!", oneArg("poll!", args)))
	})

	// (promise-chan): a latch — first put wins, EVERY take sees it,
	// later puts are accepted-and-ignored (oracle promise-chan-put-
	// after-first => [:a :a]).
	areg("promise-chan", func(args ...any) any {
		if len(args) != 0 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: promise-chan", len(args)))
		}
		return lang.NewPromiseChan()
	})

	// --- T2: distribution/composition pumps (ADR 0040 §2.1–2.5, openspec
	// core-async-first-class §2). Every name interns ONLY in
	// clojure.core.async (the precedence principle applied to libraries:
	// nothing here is aliased into clojure.core). Runtime lives in
	// pkg/lang/chan_pump.go; behaviour is frozen against JVM core.async
	// 1.6.681 (conformance/tests/chan-{pipe,merge,split,pub,mix,…}.clj).
	bufOf := func(v any) int {
		if v == nil {
			return 0
		}
		if n, ok := v.(int64); ok {
			return int(n)
		}
		panic(fmt.Errorf("buffer size must be an integer, got: %s", lang.PrintString(v)))
	}

	// (onto-chan! ch coll) / (onto-chan! ch coll close?): pump coll onto
	// ch, closing ch when close? (default true); onto-chan!! is the
	// thread-variant alias, onto-chan the deprecated pre-1.5 name (all
	// identical — ADR 0040 #5 collapses the pool distinction).
	ontoChan := func(op string) func(...any) any {
		return func(args ...any) any {
			if len(args) < 2 || len(args) > 3 {
				panic(fmt.Errorf("wrong number of args (%d) passed to: %s", len(args), op))
			}
			closeWhenDone := true
			if len(args) == 3 {
				closeWhenDone = lang.IsTruthy(args[2])
			}
			return lang.OntoChan(chanArg(op, args[0]), args[1], closeWhenDone)
		}
	}
	areg("onto-chan!", ontoChan("onto-chan!"))
	areg("onto-chan!!", ontoChan("onto-chan!!"))
	areg("onto-chan", ontoChan("onto-chan"))

	// (to-chan! coll): a fresh channel of coll's values that closes after.
	toChan := func(op string) func(...any) any {
		return func(args ...any) any {
			return lang.ToChan(oneArg(op, args))
		}
	}
	areg("to-chan!", toChan("to-chan!"))
	areg("to-chan!!", toChan("to-chan!!"))
	areg("to-chan", toChan("to-chan"))

	// (pipe from to) / (pipe from to close?): copy from→to; returns to.
	areg("pipe", func(args ...any) any {
		if len(args) < 2 || len(args) > 3 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: pipe", len(args)))
		}
		closeWhenDone := true
		if len(args) == 3 {
			closeWhenDone = lang.IsTruthy(args[2])
		}
		return lang.Pipe(chanArg("pipe", args[0]), chanArg("pipe", args[1]), closeWhenDone)
	})

	// (merge chans) / (merge chans buf-or-n): fan-in of a collection of
	// channels into one, closing when all inputs close.
	areg("merge", func(args ...any) any {
		if len(args) < 1 || len(args) > 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: merge", len(args)))
		}
		var chans []*lang.Channel
		for s := lang.Seq(args[0]); s != nil; s = s.Next() {
			chans = append(chans, chanArg("merge", s.First()))
		}
		buf := 0
		if len(args) == 2 {
			buf = bufOf(args[1])
		}
		return lang.MergeChans(chans, buf)
	})

	// (split p ch) / (split p ch t-buf f-buf): [truthy-chan falsey-chan].
	areg("split", func(args ...any) any {
		if len(args) != 2 && len(args) != 4 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: split", len(args)))
		}
		tbuf, fbuf := 0, 0
		if len(args) == 4 {
			tbuf, fbuf = bufOf(args[2]), bufOf(args[3])
		}
		tc, fc := lang.Split(args[0], chanArg("split", args[1]), tbuf, fbuf)
		return lang.NewVector(tc, fc)
	})

	// (into coll ch): a channel with one value — coll with ch's values
	// conj'd on, after ch closes.
	areg("into", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: into", len(args)))
		}
		return lang.ChanInto(args[0], chanArg("into", args[1]))
	})

	// (reduce f init ch): a channel with the single fold result.
	areg("reduce", func(args ...any) any {
		if len(args) != 3 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: reduce", len(args)))
		}
		return lang.ChanReduce(args[0], args[1], chanArg("reduce", args[2]))
	})

	// (transduce xform f init ch): reduce with a transducer + completion.
	areg("transduce", func(args ...any) any {
		if len(args) != 4 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: transduce", len(args)))
		}
		return lang.ChanTransduce(args[0], args[1], args[2], chanArg("transduce", args[3]))
	})

	// (take n ch) / (take n ch buf-or-n): at most n values then close.
	areg("take", func(args ...any) any {
		if len(args) != 2 && len(args) != 3 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: take", len(args)))
		}
		n, ok := args[0].(int64)
		if !ok {
			panic(fmt.Errorf("take expects an integer count, got: %s", lang.PrintString(args[0])))
		}
		buf := 0
		if len(args) == 3 {
			buf = bufOf(args[2])
		}
		return lang.ChanTake(int(n), chanArg("take", args[1]), buf)
	})

	// (map f chs) / (map f chs buf-or-n): combine N channels through f —
	// each round takes one value from every input, delivers (apply f vals),
	// and closes as soon as any input closes (oracle map-sum => 11 22 33,
	// map-uneven => [1 10] [2 20] nil). NOT deprecated (unlike map</map>);
	// interns only in clojure.core.async (clojure.core/map is untouched —
	// the precedence principle: async's map shadows nothing in core, it is
	// a different var reached as clojure.core.async/map).
	areg("map", func(args ...any) any {
		if len(args) != 2 && len(args) != 3 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: map", len(args)))
		}
		var chans []*lang.Channel
		for s := lang.Seq(args[1]); s != nil; s = s.Next() {
			chans = append(chans, chanArg("map", s.First()))
		}
		buf := 0
		if len(args) == 3 {
			buf = bufOf(args[2])
		}
		return lang.MapChans(args[0], chans, buf)
	})

	// (thread-call f): run f on a real goroutine, returning a channel that
	// yields f's result once and then closes (a nil result sends nothing —
	// the channel just closes; oracle thread-call => 42 then nil,
	// thread-call-nil => nil). This is the public fn the `thread` macro is
	// built on; it is the same runtime seam as go* (lang.Go).
	areg("thread-call", func(args ...any) any {
		return lang.Go(oneArg("thread-call", args))
	})

	// pipeline / pipeline-blocking / pipeline-async (ADR 0040 tier T3):
	// transform `from` into `to` with parallelism n, IN INPUT ORDER, closing
	// `to` when from drains unless close?=false; returns a completion channel
	// that closes when done. n must be positive (oracle n0 => AssertionError).
	// On Go all three collapse to one goroutine-parallel engine — pipeline and
	// pipeline-blocking are observably identical (ADR 0040 #9: real goroutines
	// collapse the JVM's compute-vs-blocking thread-pool distinction). Runtime
	// in pkg/lang/chan_pump.go; frozen against JVM core.async 1.6.681
	// (conformance/tests/chan-pipeline*.clj).
	pipeN := func(op string, v any) int {
		n, ok := v.(int64)
		if !ok {
			panic(fmt.Errorf("%s expects an integer parallelism, got: %s", op, lang.PrintString(v)))
		}
		if n <= 0 {
			panic(fmt.Errorf("Assert failed: (pos? n)"))
		}
		return int(n)
	}
	pipeline := func(op string) func(...any) any {
		return func(args ...any) any {
			if len(args) < 4 || len(args) > 6 {
				panic(fmt.Errorf("wrong number of args (%d) passed to: %s", len(args), op))
			}
			n := pipeN(op, args[0])
			to := chanArg(op, args[1])
			xf := args[2]
			from := chanArg(op, args[3])
			closeWhenDone := true
			if len(args) >= 5 {
				closeWhenDone = lang.IsTruthy(args[4])
			}
			var exh any
			if len(args) == 6 {
				exh = args[5]
			}
			return lang.Pipeline(n, to, xf, from, closeWhenDone, exh)
		}
	}
	areg("pipeline", pipeline("pipeline"))
	areg("pipeline-blocking", pipeline("pipeline-blocking"))
	areg("pipeline-async", func(args ...any) any {
		if len(args) < 4 || len(args) > 5 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: pipeline-async", len(args)))
		}
		n := pipeN("pipeline-async", args[0])
		to := chanArg("pipeline-async", args[1])
		af := args[2]
		from := chanArg("pipeline-async", args[3])
		closeWhenDone := true
		if len(args) == 5 {
			closeWhenDone = lang.IsTruthy(args[4])
		}
		return lang.PipelineAsync(n, to, af, from, closeWhenDone)
	})

	// mult / tap / untap / untap-all: fan-out every value to every tap.
	multArg := func(op string, v any) *lang.Mult {
		m, ok := v.(*lang.Mult)
		if !ok {
			panic(fmt.Errorf("%s expects a mult, got: %s", op, lang.PrintString(v)))
		}
		return m
	}
	areg("mult", func(args ...any) any {
		return lang.NewMult(chanArg("mult", oneArg("mult", args)))
	})
	areg("tap", func(args ...any) any {
		if len(args) < 2 || len(args) > 3 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: tap", len(args)))
		}
		closeWhenDone := true
		if len(args) == 3 {
			closeWhenDone = lang.IsTruthy(args[2])
		}
		ch := chanArg("tap", args[1])
		multArg("tap", args[0]).Tap(ch, closeWhenDone)
		return ch
	})
	areg("untap", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: untap", len(args)))
		}
		multArg("untap", args[0]).Untap(chanArg("untap", args[1]))
		return nil
	})
	areg("untap-all", func(args ...any) any {
		multArg("untap-all", oneArg("untap-all", args)).UntapAll()
		return nil
	})

	// pub / sub / unsub / unsub-all: topic-fn routes to a per-topic mult.
	pubArg := func(op string, v any) *lang.Pub {
		p, ok := v.(*lang.Pub)
		if !ok {
			panic(fmt.Errorf("%s expects a pub, got: %s", op, lang.PrintString(v)))
		}
		return p
	}
	areg("pub", func(args ...any) any {
		if len(args) < 2 || len(args) > 3 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: pub", len(args)))
		}
		var bufFn any
		if len(args) == 3 {
			bufFn = args[2]
		}
		return lang.NewPub(chanArg("pub", args[0]), args[1], bufFn)
	})
	areg("sub", func(args ...any) any {
		if len(args) < 3 || len(args) > 4 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: sub", len(args)))
		}
		closeWhenDone := true
		if len(args) == 4 {
			closeWhenDone = lang.IsTruthy(args[3])
		}
		ch := chanArg("sub", args[2])
		pubArg("sub", args[0]).Sub(args[1], ch, closeWhenDone)
		return ch
	})
	areg("unsub", func(args ...any) any {
		if len(args) != 3 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: unsub", len(args)))
		}
		pubArg("unsub", args[0]).Unsub(args[1], chanArg("unsub", args[2]))
		return nil
	})
	areg("unsub-all", func(args ...any) any {
		if len(args) < 1 || len(args) > 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: unsub-all", len(args)))
		}
		p := pubArg("unsub-all", args[0])
		if len(args) == 2 {
			p.UnsubAll(true, args[1])
		} else {
			p.UnsubAll(false, nil)
		}
		return nil
	})

	// mix / admix / unmix / unmix-all / toggle / solo-mode: stateful
	// fan-in with mute/pause/solo (oracle mix-*).
	mixArg := func(op string, v any) *lang.Mix {
		m, ok := v.(*lang.Mix)
		if !ok {
			panic(fmt.Errorf("%s expects a mix, got: %s", op, lang.PrintString(v)))
		}
		return m
	}
	areg("mix", func(args ...any) any {
		return lang.NewMix(chanArg("mix", oneArg("mix", args)))
	})
	areg("admix", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: admix", len(args)))
		}
		mixArg("admix", args[0]).Admix(chanArg("admix", args[1]))
		return nil
	})
	areg("unmix", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: unmix", len(args)))
		}
		mixArg("unmix", args[0]).Unmix(chanArg("unmix", args[1]))
		return nil
	})
	areg("unmix-all", func(args ...any) any {
		mixArg("unmix-all", oneArg("unmix-all", args)).UnmixAll()
		return nil
	})
	areg("toggle", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: toggle", len(args)))
		}
		mixArg("toggle", args[0]).Toggle(args[1])
		return nil
	})
	areg("solo-mode", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: solo-mode", len(args)))
		}
		kw, ok := args[1].(lang.Keyword)
		if !ok {
			panic(fmt.Errorf("solo-mode expects :mute or :pause, got: %s", lang.PrintString(args[1])))
		}
		mixArg("solo-mode", args[0]).SoloMode(kw)
		return nil
	})

	// The M4-v0 clojure.core names become REFERS of the async Vars —
	// the ADR 0040 #6 aliases: same Var objects, so re-defs and identity
	// checks agree, and shipped code/REPL habit keeps working. (Refer of
	// an identical mapping is a silent no-op, so RegisterAll re-runs are
	// clean.)
	for _, name := range asyncCoreAliases {
		sym := lang.NewSymbol(name)
		v := lang.NSAsync.FindInternedVar(sym)
		if v == nil {
			panic(fmt.Errorf("registerAsync: alias %s is not interned in clojure.core.async", name))
		}
		lang.NSCore.Refer(sym, v)
	}
}

// asyncCoreAliases is the exact M4-v0 surface that shipped in
// clojure.core (design/05 §4) — kept there as refers of the canonical
// clojure.core.async vars (ADR 0040 #6). InitUserNS also refers these
// into `user` (ReferAll skips non-interned mappings, faithfully to JVM
// refer semantics, so the alias hop needs its own step). Nothing newer
// than M4-v0 may ever be added here.
var asyncCoreAliases = []string{
	"chan", ">!", ">!!", "<!", "<!!", "close!", "go", "thread", "go*",
	"timeout", "alts!", "alts!!", "dropping-buffer", "sliding-buffer",
}

// ReferAsyncAliases refers the M4-v0 alias names into ns straight from
// clojure.core.async — the tail of InitUserNS (user's ReferAll of
// clojure.core cannot carry them: they are refers there, not interns).
func ReferAsyncAliases(ns *lang.Namespace) {
	for _, name := range asyncCoreAliases {
		sym := lang.NewSymbol(name)
		if v := lang.NSAsync.FindInternedVar(sym); v != nil {
			ns.Refer(sym, v)
		}
	}
}
