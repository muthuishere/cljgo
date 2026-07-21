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
	def := func(name string, fn func(args ...any) any) *lang.Var {
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
	def("chan", func(args ...any) any {
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
	def("buffer", bufFn("buffer", lang.FixedBuffer))
	def("dropping-buffer", bufFn("dropping-buffer", lang.DroppingBuffer))
	def("sliding-buffer", bufFn("sliding-buffer", lang.SlidingBuffer))
	def("unblocking-buffer?", func(args ...any) any {
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
	def(">!", chanSend(">!"))
	def(">!!", chanSend(">!!"))

	// <! / <!! : blocking take; closed+drained => nil (aliases).
	chanRecv := func(op string) func(args ...any) any {
		return func(args ...any) any {
			return lang.ChanRecv(chanArg(op, oneArg(op, args)))
		}
	}
	def("<!", chanRecv("<!"))
	def("<!!", chanRecv("<!!"))

	def("close!", func(args ...any) any {
		return lang.ChanClose(chanArg("close!", oneArg("close!", args)))
	})

	// (timeout ms) → a fresh channel that auto-closes after ms
	// milliseconds (ADR 0040 #4: semantics only — no JVM tick cache, so
	// (identical? (timeout n) (timeout n)) is false here; a documented
	// divergence no portable program observes).
	def("timeout", func(args ...any) any {
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
	def("go*", func(args ...any) any {
		return lang.Go(oneArg("go*", args))
	})

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
	def("go", goMacro).SetMacro()
	def("thread", goMacro).SetMacro()

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
	def("alts!", altsFn("alts!"))
	def("alts!!", altsFn("alts!!"))

	// (put! c v) / (put! c v fn1) / (put! c v fn1 on-caller?):
	// asynchronous put — false iff already closed, else true immediately;
	// fn1 receives the completed put's boolean. on-caller? is accepted
	// and ignored (there is no dispatch pool to protect — goroutines).
	def("put!", func(args ...any) any {
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
	def("take!", func(args ...any) any {
		if len(args) < 2 || len(args) > 3 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: take!", len(args)))
		}
		return lang.ChanTakeAsync(chanArg("take!", args[0]), args[1])
	})

	// (offer! c v): non-blocking put — true when accepted, nil (not
	// false) when it would block, false when closed. (poll! c):
	// non-blocking take — value or nil (oracle offer-poll =>
	// [true nil 1 nil]).
	def("offer!", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: offer!", len(args)))
		}
		return lang.ChanOffer(chanArg("offer!", args[0]), args[1])
	})
	def("poll!", func(args ...any) any {
		return lang.ChanPoll(chanArg("poll!", oneArg("poll!", args)))
	})

	// (promise-chan): a latch — first put wins, EVERY take sees it,
	// later puts are accepted-and-ignored (oracle promise-chan-put-
	// after-first => [:a :a]).
	def("promise-chan", func(args ...any) any {
		if len(args) != 0 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: promise-chan", len(args)))
		}
		return lang.NewPromiseChan()
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
