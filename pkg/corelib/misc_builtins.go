package corelib

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"
	"time"

	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// internMiscBuiltins registers a small grab-bag of clojure.core vars the
// jank clojure-test-suite harness needs (ADR 0022 batch/harness-misc):
// delay/force/delay?, the instance? substrate, add-watch/remove-watch, and
// a Thread/sleep seam for the portability shim. Wired into internBuiltins
// by ONE line (e.internMiscBuiltins(def, defPrivate)), per the
// merge-friendly discipline (three other agents editing builtins.go
// concurrently).
func internMiscBuiltins(def func(name string, fn func(args ...any) any) *lang.Var, defPrivate func(name string, fn func(args ...any) any)) {
	// --- delay / force / delay? ------------------------------------------
	//
	// cljgo's pkg/lang already vendors a Delay type (IDeref + IPending) —
	// only the clojure.core surface was missing. `delay` itself is a MACRO
	// (core.clj: (defmacro delay [& body] (-make-delay (fn* [] body...)))
	// so the body isn't evaluated until forced; -make-delay is the private
	// substrate that macro expands onto (mirrors real clojure.core/delay
	// => (new clojure.lang.Delay (fn* [] body))).
	defPrivate("-make-delay", func(args ...any) any {
		fn, ok := oneArg("-make-delay", args).(lang.IFn)
		if !ok {
			panic(fmt.Errorf("delay: not a thunk: %s", lang.PrintString(args[0])))
		}
		return lang.NewDelay(fn)
	})
	// force: forces a Delay (realizing + memoizing it), or returns any
	// other value unchanged — exactly clojure.core/force.
	def("force", func(args ...any) any {
		return lang.ForceDelay(oneArg("force", args))
	})
	// delay?: is x a Delay.
	def("delay?", func(args ...any) any {
		_, ok := oneArg("delay?", args).(*lang.Delay)
		return ok
	})

	// --- instance? substrate ----------------------------------------------
	//
	// cljgo has no java.lang.Class objects (design/05: host interop is Go
	// structs, not a JVM class hierarchy), so `instance?`'s class position
	// cannot be an ordinary value the way JVM Clojure's is. `instance?` is
	// therefore a MACRO (core.clj) that treats a literal class symbol as
	// SYNTAX — matched by NAME, exactly like `catch`'s class symbol already
	// is (CatchMatches, ex_builtins.go) — never resolved as a var. This is
	// documented as a deliberate v0 deviation in ADR 0026: `instance?` with
	// a literal symbol class only works in direct call position, not as a
	// first-class function value (so (partial instance? String) does not
	// work; (instance? String x) does).
	//
	// -instance-of-name?: (class-name-string value) -> bool. First tries a
	// deftype/defrecord type var (a real value bound to a *TypeMarker,
	// exactly what -instance? already checks for extend-type's dispatch
	// key); then a fixed table of designator names covering both cljgo's
	// dispatchKey vocabulary (String/Long/Double/...) and its host wrapper
	// types (Atom/Delay/Var/Namespace/UUID/BigInt/BigDecimal) that have no
	// clojure.core designator of their own. A qualified name's LAST dotted
	// segment is what the table matches (clojure.lang.Atom ~ Atom,
	// java.util.UUID ~ UUID), mirroring how Clojure programmers read them.
	defPrivate("-instance-of-name?", func(args ...any) any {
		name := lang.ToString(args[0])
		v := args[1]

		if vr, err := ResolveVar(lang.NewSymbol(name)); err == nil {
			if m, ok := vr.Deref().(*TypeMarker); ok {
				return dispatchKey(v) == m.name
			}
		}
		return classNameMatchesValue(name, v)
	})

	// class? (ADR 0036): true for the two things cljgo treats as classes —
	// interned ClassRef values and deftype/defrecord TypeMarkers. JVM
	// analogy: (instance? java.lang.Class x); record/type names are
	// classes there too. Used by the hierarchy fns (hierarchies.cljg):
	// derive accepts classes as tags, descendants throws on them.
	// Deliberately NOT true for Protocol values (ADR 0039): cljgo's one
	// protocol value plays both the JVM's protocol MAP and its generated
	// interface, and the hierarchy fns follow the MAP reading —
	// (descendants P) is nil, (derive P ::x) asserts — matching how the
	// suite exercises protocols; the interface reading survives only
	// inside a record/type's ancestry.
	def("class?", func(args ...any) any {
		switch oneArg("class?", args).(type) {
		case *ClassRef, *TypeMarker:
			return true
		}
		return false
	})

	// --- add-watch / remove-watch ------------------------------------------
	//
	// Generic over any lang.IRef (Atom, Var — Delay/Agent don't carry
	// watches in cljgo yet). (key ref old new) callback shape matches real
	// clojure.core exactly (pkg/lang's Atom/Var already notify in that
	// order; see atom.go/var.go notifyWatches).
	def("add-watch", func(args ...any) any {
		if len(args) != 3 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: add-watch", len(args)))
		}
		ref, ok := args[0].(lang.IRef)
		if !ok {
			panic(fmt.Errorf("add-watch: not watchable: %s", lang.PrintString(args[0])))
		}
		fn, ok := args[2].(lang.IFn)
		if !ok {
			panic(fmt.Errorf("add-watch: not a function: %s", lang.PrintString(args[2])))
		}
		return ref.AddWatch(args[1], fn)
	})
	def("remove-watch", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: remove-watch", len(args)))
		}
		ref, ok := args[0].(lang.IRef)
		if !ok {
			panic(fmt.Errorf("remove-watch: not watchable: %s", lang.PrintString(args[0])))
		}
		ref.RemoveWatch(args[1])
		return args[0]
	})

	// --- clojure.edn substrate ----------------------------------------------
	//
	// -edn-read-string backs clojure.edn/read-string's 1-arg arity
	// (core/edn.cljg): read ONE form from a string with cljgo's reader in
	// EDN-STRICT mode (reader.WithEDNStrict — clojure-test-suite
	// edn_test/read_string.cljc, ADR 0022 batch/harness-misc). Not
	// evaluated — read only. An empty/whitespace-only string reads as nil
	// (oracle: JVM (clojure.edn/read-string "") => nil, equivalent to the
	// 2-arg form's default {:eof nil}); a syntactically invalid string
	// throws, as JVM edn does. DEVIATION (documented in core/edn.cljg):
	// this is cljgo's own reader with edn's rules layered on top, not a
	// wholly separate restricted-EDN reader, so reader macros EDN forbids
	// (#(…) fn literals, `quasiquote`) do not throw here. The suite's own
	// #=(…) eval-reader probe still throws — cljgo's reader has no #= at
	// all. `.getTime` on a #inst value (the suite's epoch-millis helper)
	// works via a narrow special-case in host.go's CallGoMethod, not
	// ordinary Go-reflection interop — see pkg/reader/tagged.go's Inst doc.
	defPrivate("-edn-read-string", func(args ...any) any {
		s, ok := oneArg("-edn-read-string", args).(string)
		if !ok {
			if args[0] == nil {
				panic(fmt.Errorf("edn/read-string: nil input"))
			}
			panic(fmt.Errorf("edn/read-string expects a string, got: %s", lang.PrintString(args[0])))
		}
		form, err := reader.ReadString(s, reader.WithResolver(NSResolver()), reader.WithEDNStrict())
		if err != nil {
			if errors.Is(err, reader.ErrEOF) {
				return nil
			}
			panic(err)
		}
		return form
	})

	// -edn-read-string-opts backs clojure.edn/read-string's 2-arg arity:
	// (read-string opts s) with :eof / :default / :readers (oracle 1.12.5:
	// (edn/read-string {:eof :END} "") => :END; (edn/read-string {} " ")
	// throws — no :eof means a bare EOF is an error, not nil, matching the
	// JVM's `(read-string {:eof :eofthrow} s)` default under the hood;
	// (edn/read-string {:default (fn [_tag v] [:unknown v])} "#foo 42") =>
	// [:unknown 42]; (edn/read-string {:readers {'uuid (constantly
	// :override)}} "#uuid \"...\"") => :override, overriding even a
	// built-in tag). Same edn-strict reader rules as the 1-arg form.
	kwEOF := lang.NewKeyword("eof")
	defPrivate("-edn-read-string-opts", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: edn/read-string", len(args)))
		}
		optsArg, sArg := args[0], args[1]
		s, ok := sArg.(string)
		if !ok {
			if sArg == nil {
				panic(fmt.Errorf("edn/read-string: nil input"))
			}
			panic(fmt.Errorf("edn/read-string expects a string, got: %s", lang.PrintString(sArg)))
		}
		opts := ednOptsMap("edn/read-string", optsArg)

		form, err := reader.ReadString(s, ednReaderOptions("edn/read-string", opts)...)
		if err != nil {
			if errors.Is(err, reader.ErrEOF) {
				if opts != nil && opts.ContainsKey(kwEOF) {
					return lang.Get(opts, kwEOF)
				}
				panic(fmt.Errorf("edn/read-string: EOF while reading"))
			}
			panic(err)
		}
		return form
	})

	// --- clojure.core/read-string --------------------------------------------
	//
	// The GENERAL reader's read-string (fundamentals audit 2026-07 — distinct
	// from clojure.edn/read-string above): cljgo's full reader, NOT
	// edn-strict, so ::autoresolved keywords, #(...) fn literals and
	// syntax-quote all read. oracle (JVM 1.12.5, conformance/tests/
	// read-string-core.clj): (read-string "(+ 1 2)") => (+ 1 2);
	// (read-string "{:a 1} ignored") => {:a 1} (one form only);
	// (read-string "") throws "EOF while reading"; (read-string {:eof :done}
	// "") => :done; (read-string "::a") => :user/a in ns user.
	//
	// DEVIATION (documented): the opts arity honors :eof only — :read-cond/
	// :features are reader-conditional policy cljgo's reader does not expose
	// per-call, and #= eval-on-read does not exist in cljgo's reader at all
	// (it throws), which is the safe subset of *read-eval*'s default. The
	// stream-based `read` and *in*-based `read-line` stay unimplemented
	// pending the *in* design note (audit batch 5).
	def("read-string", func(args ...any) any {
		var opts lang.IPersistentMap
		var s any
		switch len(args) {
		case 1:
			s = args[0]
		case 2:
			if args[0] != nil {
				m, ok := args[0].(lang.IPersistentMap)
				if !ok {
					panic(fmt.Errorf("read-string: opts must be a map, got: %s", lang.PrintString(args[0])))
				}
				opts = m
			}
			s = args[1]
		default:
			panic(fmt.Errorf("wrong number of args (%d) passed to: read-string", len(args)))
		}
		str, ok := s.(string)
		if !ok {
			panic(fmt.Errorf("read-string expects a string, got: %s", lang.PrintString(s)))
		}
		form, err := reader.ReadString(str, reader.WithResolver(NSResolver()))
		if err != nil {
			if errors.Is(err, reader.ErrEOF) {
				if opts != nil && opts.ContainsKey(kwEOF) {
					return lang.Get(opts, kwEOF)
				}
				panic(fmt.Errorf("EOF while reading"))
			}
			panic(err)
		}
		return form
	})

	// --- tagged literals & reader conditionals (ADR 0050) --------------------
	//
	// The four reader-data publics the fundamentals audit deferred. These are
	// DATA constructors + predicates over the value types clojure's data
	// readers and `read`/`read-string` with {:read-cond :preserve} yield
	// (clojure.lang.TaggedLiteral / ReaderConditional). cljgo's reader does
	// not currently expose a :read-cond :preserve mode (it always selects or
	// elides conditionals — ADR 0036), so these constructors are the sole way
	// to build the values today; reader :preserve integration is scoped to a
	// follow-up (ADR 0050). Oracle 1.12.5, conformance/tests/
	// tagged-literal.clj + reader-conditional.clj.

	// tagged-literal: (tag form) -> a TaggedLiteral with :tag and :form,
	// printing as `#tag form` (oracle: (pr-str (tagged-literal 'foo 42))
	// => "#foo 42"; (:tag ..) => foo; (:form ..) => 42).
	def("tagged-literal", func(args ...any) any {
		tag, form := twoArgs("tagged-literal", args)
		return lang.NewTaggedLiteral(tag, form)
	})

	// tagged-literal?: true for a TaggedLiteral value.
	def("tagged-literal?", func(args ...any) any {
		return lang.IsTaggedLiteral(oneArg("tagged-literal?", args))
	})

	// reader-conditional: (form splicing?) -> a ReaderConditional with :form
	// and :splicing?, printing as `#?(...)` or `#?@(...)`. splicing? is
	// coerced to boolean (oracle: (:splicing? (reader-conditional '(:clj 1)
	// false)) => false).
	def("reader-conditional", func(args ...any) any {
		form, splicing := twoArgs("reader-conditional", args)
		return lang.NewReaderConditional(form, lang.BooleanCast(splicing))
	})

	// reader-conditional?: true for a ReaderConditional value.
	def("reader-conditional?", func(args ...any) any {
		return lang.IsReaderConditional(oneArg("reader-conditional?", args))
	})

	// --- map entries (clojure.walk substrate) --------------------------------
	//
	// map-entry? itself lives in predicate_builtins.go with the other type
	// predicates; two identical registrations briefly coexisted after the
	// walk and core-fns batches merged, and the second silently overwrote
	// the first. Only the private constructor seam belongs here.
	// -map-entry: (k v) -> a real MapEntry — clojure.walk's substrate for
	// rebuilding walked entries (JVM walk uses MapEntry/create so the
	// walking fn sees genuine entries, CLJ-2031), private like the other
	// `-` seams; cljgo has no MapEntry constructor surface otherwise.
	defPrivate("-map-entry", func(args ...any) any {
		k, v := twoArgs("-map-entry", args)
		return lang.NewMapEntry(k, v)
	})
	// -edn-read backs clojure.edn/read (core/edn.cljg): read ONE form from
	// a STREAM — any Go io.Reader value, *in* (os.Stdin) by default — with
	// the same edn-strict reader and the same :eof / :default / :readers
	// options as -edn-read-string-opts (oracle 1.12.5 over a
	// java.io.PushbackReader: successive reads return successive forms;
	// bare EOF throws "EOF while reading"; {:eof v} returns v). Streams
	// that are not io.RuneScanners are wrapped once and cached
	// (ednRuneScanner) so successive reads continue where the last
	// stopped; a fresh reader.Reader per call is safe because all
	// read-ahead lives in the RuneScanner (one-rune peeks are UnreadRune'd
	// back).
	defPrivate("-edn-read", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: edn/read", len(args)))
		}
		opts := ednOptsMap("edn/read", args[0])
		rs := ednRuneScanner(args[1])
		form, err := reader.New(rs, ednReaderOptions("edn/read", opts)...).ReadOne()
		if err != nil {
			if errors.Is(err, reader.ErrEOF) {
				if opts != nil && opts.ContainsKey(kwEOF) {
					return lang.Get(opts, kwEOF)
				}
				panic(fmt.Errorf("edn/read: EOF while reading"))
			}
			panic(err)
		}
		return form
	})

	// --- type --------------------------------------------------------------
	//
	// cljgo has no java.lang.Class objects (design/05), so `type` cannot
	// return a real Class the way JVM Clojure's does. What the suite
	// actually needs (num.cljc, transient.cljc) is a value that's stable
	// and comparable via `=`: (= (type n) (type (num n))) checks that a
	// no-op numeric coercion doesn't change representation, and
	// (= (type coll) (type persisted)) checks a transient round-trips to
	// the same collection shape. reflect.Type satisfies exactly that: it's
	// a comparable Go interface value (== is true iff same concrete type),
	// which Equiv's `a == b` fast path already handles — no Equalser
	// needed. Real Clojure's :type metadata override (deftype/defrecord's
	// ::type key) is NOT implemented; this is a v0 stand-in, not a full
	// `type`/`class` substrate (that's its own future ADR).
	def("type", func(args ...any) any {
		x := oneArg("type", args)
		if x == nil {
			return nil
		}
		if m, ok := x.(lang.IMeta); ok {
			if meta := m.Meta(); meta != nil {
				if t := meta.ValAt(lang.KWType); t != nil {
					return t
				}
			}
		}
		return reflect.TypeOf(x)
	})

	// --- sleep --------------------------------------------------------------
	//
	// Backs the cljgo portability shim's `sleep` (core/clojure_test_portability.cljg)
	// — the suite's own test/clojure/core_test/portability.cljc defines
	// sleep as a #?(:clj (Thread/sleep ms) ...) host call; cljgo has no
	// Thread class, so this is a direct time.Sleep seam.
	defPrivate("-sleep-ms", func(args ...any) any {
		ms := lang.AsInt64(oneArg("-sleep-ms", args))
		time.Sleep(time.Duration(ms) * time.Millisecond)
		return nil
	})
}

// ednOptsMap coerces a clojure.edn opts argument (nil or a map) into an
// IPersistentMap, panicking with the caller's name otherwise. Shared by
// -edn-read-string-opts and -edn-read.
func ednOptsMap(op string, optsArg any) lang.IPersistentMap {
	if optsArg == nil {
		return nil
	}
	m, ok := optsArg.(lang.IPersistentMap)
	if !ok {
		panic(fmt.Errorf("%s: opts must be a map, got: %s", op, lang.PrintString(optsArg)))
	}
	return m
}

// ednReaderOptions translates a clojure.edn opts map's :default / :readers
// entries into reader options on top of the edn-strict base (resolver +
// WithEDNStrict). :eof is handled by the callers — it is an EOF policy,
// not a reader option. Shared by -edn-read-string-opts and -edn-read.
func ednReaderOptions(op string, opts lang.IPersistentMap) []reader.Option {
	readerOpts := []reader.Option{reader.WithResolver(NSResolver()), reader.WithEDNStrict()}
	if opts == nil {
		return readerOpts
	}
	if fn, ok := lang.Get(opts, lang.NewKeyword("default")).(lang.IFn); ok {
		readerOpts = append(readerOpts, reader.WithDefaultReader(fn))
	}
	if rm, ok := lang.Get(opts, lang.NewKeyword("readers")).(lang.IPersistentMap); ok {
		tagReaders := make(map[string]lang.IFn)
		for seq := lang.Seq(rm); seq != nil; seq = seq.Next() {
			entry := seq.First().(lang.IMapEntry)
			tagSym, ok := entry.Key().(*lang.Symbol)
			if !ok {
				panic(fmt.Errorf("%s: :readers keys must be symbols, got: %s", op, lang.PrintString(entry.Key())))
			}
			fn, ok := entry.Val().(lang.IFn)
			if !ok {
				panic(fmt.Errorf("%s: :readers values must be functions, got: %s", op, lang.PrintString(entry.Val())))
			}
			tagReaders[tagSym.FullName()] = fn
		}
		readerOpts = append(readerOpts, reader.WithTagReaders(tagReaders))
	}
	return readerOpts
}

// ednScanners caches one RuneScanner per plain io.Reader stream handed to
// clojure.edn/read, so successive reads continue exactly where the last
// one stopped (a fresh bufio wrapper per call would buffer ahead and drop
// input). Streams that already implement io.RuneScanner (strings.Reader,
// bufio.Reader) keep their own position and are used directly. Entries
// are never evicted — a process reads from a handful of streams (usually
// just *in*), so the cache stays a few entries.
var (
	ednScannersMu sync.Mutex
	ednScanners   = map[io.Reader]io.RuneScanner{}
)

// ednRuneScanner resolves clojure.edn/read's stream argument to the
// io.RuneScanner the reader consumes.
func ednRuneScanner(stream any) io.RuneScanner {
	if rs, ok := stream.(io.RuneScanner); ok {
		return rs
	}
	r, ok := stream.(io.Reader)
	if !ok {
		panic(fmt.Errorf("edn/read expects a reader (a Go io.Reader; *in* by default), got: %s", lang.PrintString(stream)))
	}
	ednScannersMu.Lock()
	defer ednScannersMu.Unlock()
	if rs, ok := ednScanners[r]; ok {
		return rs
	}
	rs := bufio.NewReader(r)
	ednScanners[r] = rs
	return rs
}
