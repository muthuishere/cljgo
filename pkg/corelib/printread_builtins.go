// printread_builtins.go — the printing + reading extension surface
// (fundamentals batch A2, 2026-07-23): the Go half of print-method /
// print-dup / print-simple, Throwable->map, default-data-readers, and the
// stream-based read / read+string. The print-method and print-dup
// multimethods themselves are defined in core.clj (defmulti over the
// -print-class dispatch substrate here); lang.Print consults them through
// the PrintDispatch seam below, which stays completely inert — one atomic
// bool load — until the first non-:default method is registered
// (multimethod_builtins.go's -defmethod flips it), so the native printer's
// fast path is untouched (perf budgets, design/00 §1.4).
package corelib

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"

	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// --- print-method / print-dup dispatch seam ------------------------------

// resolvePrintMFs resolves the clojure.core/print-method and print-dup
// MultiFn values LIVE, on every dispatch — they are defined by core.clj
// (which loads after these builtins are interned) with plain def, so a
// core reload rebinds the vars to fresh MultiFns; a cached pointer would
// keep consulting the abandoned one while defmethod registers into the
// new (seen for real when the compiled + eval conformance harnesses share
// one process). Cost lands only on the active path — lang.Print reaches
// here at all only after PrintDispatchActive flips — so the native fast
// path stays untouched (perf budgets, design/00 §1.4).
func resolvePrintMFs() (*MultiFn, *MultiFn) {
	var pm, pd *MultiFn
	if v := coreFnVar("print-method"); v != nil && v.IsBound() {
		if m, ok := v.Get().(*MultiFn); ok {
			pm = m
		}
	}
	if v := coreFnVar("print-dup"); v != nil && v.IsBound() {
		if m, ok := v.Get().(*MultiFn); ok {
			pd = m
		}
	}
	return pm, pd
}

// nonDefaultMethod is methodFor WITHOUT the :default fallback: exact =
// match first, then the isa?/preference scan. nil when only :default would
// apply — the printer then stays on its native path.
func nonDefaultMethod(m *MultiFn, dv any) lang.IFn {
	if fn, ok := m.getMethod(dv); ok {
		return fn
	}
	if fn, ok := m.bestIsaMethod(dv); ok {
		return fn
	}
	return nil
}

// printDispatchFor is the lang.PrintDispatch hook: the user's print-dup
// (when *print-dup* is truthy) or print-method for x's dispatch value —
// keyword :type metadata first, else the value's class (printClassOf),
// exactly the JVM print-method dispatch fn (oracle 1.12.5: a value with
// {:type ::kw} meta dispatches on ::kw; a deftype dispatches on its
// class). nil when x has no extension registered.
func printDispatchFor(x any) lang.IFn {
	pm, pd := resolvePrintMFs()
	if pm == nil && pd == nil {
		return nil
	}
	var dv any
	if im, ok := x.(lang.IMeta); ok {
		if m := im.Meta(); m != nil {
			if t, ok := m.ValAt(lang.KWType).(lang.Keyword); ok {
				dv = t
			}
		}
	}
	if dv == nil {
		dv = printClassOf(x)
	}
	if pd != nil && lang.BooleanCast(lang.VarPrintDup.Deref()) {
		if fn := nonDefaultMethod(pd, dv); fn != nil {
			return fn
		}
	}
	if pm != nil {
		return nonDefaultMethod(pm, dv)
	}
	return nil
}

func init() {
	lang.PrintDispatch = printDispatchFor
}

// printClassOf is the class-position dispatch value for printing: the
// *TypeMarker for deftype/defrecord instances (the very value the type's
// name var holds, so (defmethod print-method MyType ...) matches), the
// interned ClassRef for the scalar types users spell as class names
// (String, Long, clojure.lang.Keyword, ...), and the reflect.Type `type`
// returns otherwise — a stable, =-comparable value in every case.
func printClassOf(x any) any {
	switch v := x.(type) {
	case *lang.DType:
		if m := typeMarkerFor(v.TypeName()); m != nil {
			return m
		}
	case *lang.Record:
		if m := typeMarkerFor(v.TypeName()); m != nil {
			return m
		}
	case string:
		return lookupClassRef("java.lang.String")
	case int64, int, int32, int16, int8:
		return lookupClassRef("java.lang.Long")
	case float64, float32:
		return lookupClassRef("java.lang.Double")
	case bool:
		return lookupClassRef("java.lang.Boolean")
	case lang.Char:
		return lookupClassRef("java.lang.Character")
	case lang.Keyword:
		return lookupClassRef("clojure.lang.Keyword")
	case *lang.Symbol:
		return lookupClassRef("clojure.lang.Symbol")
	case *lang.BigInt:
		return lookupClassRef("clojure.lang.BigInt")
	case *lang.BigDecimal:
		return lookupClassRef("java.math.BigDecimal")
	case *lang.Var:
		return lookupClassRef("clojure.lang.Var")
	case *lang.Atom:
		return lookupClassRef("clojure.lang.Atom")
	case *reader.UUID:
		return lookupClassRef("java.util.UUID")
	}
	return reflect.TypeOf(x)
}

// typeMarkerFor resolves a deftype/defrecord instance's qualified type
// name (e.g. "user.Pt") back to the *TypeMarker its name var holds, via
// the same typeClassVar lookup ADR 0039's class-position resolution uses.
func typeMarkerFor(name string) *TypeMarker {
	if v := typeClassVar(lang.NewSymbol(name)); v != nil && v.IsBound() {
		if m, ok := v.Get().(*TypeMarker); ok {
			return m
		}
	}
	return nil
}

// --- stream-based read / read+string -------------------------------------

// coreReadStream is clojure.core/read's stream value — cljgo's stand-in
// for the JVM's LineNumberingPushbackReader: an io.RuneScanner that can
// additionally CAPTURE the runes it hands out, which is exactly what
// read+string needs. with-in-str (core.clj) binds *in* to one via the
// -string-pushback-reader builtin; a plain io.Reader stream (os.Stdin) is
// wrapped once and cached (coreStreams) so successive reads continue
// where the last stopped, the same pattern clojure.edn/read uses
// (ednScanners, misc_builtins.go).
type coreReadStream struct {
	mu        sync.Mutex
	rs        io.RuneScanner
	capturing bool
	buf       []rune
}

func (s *coreReadStream) ReadRune() (rune, int, error) {
	r, size, err := s.rs.ReadRune()
	if err == nil && s.capturing {
		s.buf = append(s.buf, r)
	}
	return r, size, err
}

func (s *coreReadStream) UnreadRune() error {
	err := s.rs.UnreadRune()
	if err == nil && s.capturing && len(s.buf) > 0 {
		s.buf = s.buf[:len(s.buf)-1]
	}
	return err
}

func (s *coreReadStream) String() string { return "#object[PushbackReader]" }

// readOne reads the next form off the stream with cljgo's FULL reader
// (the same entry point read-string uses — ::auto keywords resolve, all
// reader macros apply). A fresh reader.Reader per call is safe: all
// read-ahead lives in the RuneScanner (one-rune peeks are UnreadRune'd
// back), exactly as documented for -edn-read.
func (s *coreReadStream) readOne() (any, error) {
	return reader.New(s, reader.WithResolver(NSResolver())).ReadOne()
}

var (
	coreStreamsMu sync.Mutex
	coreStreams   = map[io.Reader]*coreReadStream{}
)

// coreReadStreamFor resolves read's stream argument: a coreReadStream is
// used directly; any other io.Reader (os.Stdin — *in*'s root) is wrapped
// once and cached by identity.
func coreReadStreamFor(ctx string, v any) *coreReadStream {
	switch s := v.(type) {
	case *coreReadStream:
		return s
	case io.Reader:
		coreStreamsMu.Lock()
		defer coreStreamsMu.Unlock()
		if cs, ok := coreStreams[s]; ok {
			return cs
		}
		var rs io.RuneScanner
		if r, ok := s.(io.RuneScanner); ok {
			rs = r
		} else {
			rs = bufio.NewReader(s)
		}
		cs := &coreReadStream{rs: rs}
		coreStreams[s] = cs
		return cs
	}
	panic(fmt.Errorf("%s expects a reader (a Go io.Reader; *in* by default), got: %s", ctx, lang.PrintString(v)))
}

// readArgs parses read/read+string's shared arities: [] [stream]
// [opts stream] [stream eof-error? eof-value] [stream eof-error? eof-value
// recursive?] (recursive? accepted and ignored — cljgo's reader has no
// recursive-read distinction).
func readArgs(ctx string, args []any) (stream *coreReadStream, opts lang.IPersistentMap, eofError bool, eofVal any) {
	eofError = true
	switch len(args) {
	case 0:
		return coreReadStreamFor(ctx, lang.VarIn.Deref()), nil, true, nil
	case 1:
		return coreReadStreamFor(ctx, args[0]), nil, true, nil
	case 2:
		m, ok := args[0].(lang.IPersistentMap)
		if !ok {
			panic(fmt.Errorf("%s: 2-arg arity is (%s opts stream), opts must be a map, got: %s", ctx, ctx, lang.PrintString(args[0])))
		}
		return coreReadStreamFor(ctx, args[1]), m, true, nil
	case 3, 4:
		return coreReadStreamFor(ctx, args[0]), nil, lang.IsTruthy(args[1]), args[2]
	}
	panic(fmt.Errorf("wrong number of args (%d) passed to: %s", len(args), ctx))
}

// resolveReadEOF maps a reader EOF to the caller's policy: the :eof opt /
// eof-value when supplied, else the JVM's "EOF while reading" error.
func resolveReadEOF(opts lang.IPersistentMap, eofError bool, eofVal any) any {
	if opts != nil {
		if opts.ContainsKey(kwReadEOF) {
			return lang.Get(opts, kwReadEOF)
		}
		panic(fmt.Errorf("EOF while reading"))
	}
	if !eofError {
		return eofVal
	}
	panic(fmt.Errorf("EOF while reading"))
}

var kwReadEOF = lang.NewKeyword("eof")

// --- Throwable->map -------------------------------------------------------

var (
	kwCause   = lang.NewKeyword("cause")
	kwVia     = lang.NewKeyword("via")
	kwTrace   = lang.NewKeyword("trace")
	kwExType  = lang.NewKeyword("type")
	kwMessage = lang.NewKeyword("message")
	kwExData  = lang.NewKeyword("data")
)

// throwableTypeSym names an error value's "class" the way the JVM map
// does: clojure.lang.ExceptionInfo for an ex-info, and for the typed
// builtin exception values of ADR 0039's addendum (#99, exceptions.go /
// lang/error.go) the matching JVM class name — probed with the SAME
// errors.Is targets CatchMatches uses, most-specific first (ArityException
// and NumberFormatException before their IllegalArgumentException parent),
// so Throwable->map's :via :type agrees with what `catch` and `instance?`
// say about the same value. DEVIATION (documented in
// conformance/tests/throwable-map.clj): errors outside that family fall
// back to java.lang.Exception, because there is no finer JVM class on the
// Go host.
func throwableTypeSym(err error) *lang.Symbol {
	var ei lang.IExceptionInfo
	if errors.As(err, &ei) {
		return lang.NewSymbol("clojure.lang.ExceptionInfo")
	}
	switch {
	case errors.Is(err, arithmeticTarget):
		return lang.NewSymbol("java.lang.ArithmeticException")
	case errors.Is(err, arityTarget):
		return lang.NewSymbol("clojure.lang.ArityException")
	case errors.Is(err, numberFormatT):
		return lang.NewSymbol("java.lang.NumberFormatException")
	case errors.Is(err, illegalArgTarget):
		return lang.NewSymbol("java.lang.IllegalArgumentException")
	case errors.Is(err, illegalStateT):
		return lang.NewSymbol("java.lang.IllegalStateException")
	case errors.Is(err, unsupportedOpT):
		return lang.NewSymbol("java.lang.UnsupportedOperationException")
	case errors.Is(err, indexOOBTarget):
		return lang.NewSymbol("java.lang.IndexOutOfBoundsException")
	case errors.Is(err, classCastTarget):
		return lang.NewSymbol("java.lang.ClassCastException")
	case errors.Is(err, nullPointerTarget):
		return lang.NewSymbol("java.lang.NullPointerException")
	}
	return lang.NewSymbol("java.lang.Exception")
}

func throwableMessage(err error) any {
	if m, ok := err.(exMessager); ok {
		return m.Message()
	}
	return err.Error()
}

func throwableCause(err error) error {
	if c, ok := err.(exCauser); ok {
		if cause := c.Cause(); cause != nil {
			return cause
		}
		return nil
	}
	return errors.Unwrap(err)
}

// --- registration ---------------------------------------------------------

func internPrintReadBuiltins(def func(name string, fn func(args ...any) any) *lang.Var, defPrivate func(name string, fn func(args ...any) any)) {
	// print-simple: (print-simple o w) -> nil. The ^meta prefix (when
	// *print-meta*) then o's plain ToString — no readable quoting (oracle
	// 1.12.5: (print-simple [1 2] *out*) prints [1 2]; (print-simple "str"
	// *out*) prints str).
	def("print-simple", func(args ...any) any {
		o, wArg := twoArgs("print-simple", args)
		w, ok := wArg.(io.Writer)
		if !ok {
			panic(fmt.Errorf("print-simple expects a writer, got: %s", lang.PrintString(wArg)))
		}
		lang.PrintSimple(o, w)
		return nil
	})

	// -print-native: the built-in printer, bypassing print-method dispatch —
	// the substrate core.clj's (defmethod print-method :default ...) rides
	// on, so an unextended value prints byte-identically through the
	// multimethod and through plain pr.
	defPrivate("-print-native", func(args ...any) any {
		o, wArg := twoArgs("-print-native", args)
		w, ok := wArg.(io.Writer)
		if !ok {
			panic(fmt.Errorf("-print-native expects a writer, got: %s", lang.PrintString(wArg)))
		}
		lang.PrintNative(o, w)
		return nil
	})

	// -print-class: print-method's class-position dispatch value for a
	// value (see printClassOf) — core.clj's print-method/print-dup
	// dispatch fns call this.
	defPrivate("-print-class", func(args ...any) any {
		x := oneArg("-print-class", args)
		if x == nil {
			return nil
		}
		return printClassOf(x)
	})

	// Throwable->map: the JVM-shaped {:cause :via :trace} data view of an
	// exception chain (oracle 1.12.5: (Throwable->map (ex-info "boom"
	// {:a 1})) has :cause "boom", :via [{:type clojure.lang.ExceptionInfo
	// :message "boom" :data {:a 1} ...}], :data {:a 1}). DEVIATIONS
	// (documented in conformance/tests/throwable-map.clj): :trace is always
	// [] and via entries carry no :at — cljgo has no stack-frame
	// introspection; non-ex-info errors report :type java.lang.Exception.
	def("Throwable->map", func(args ...any) any {
		x := oneArg("Throwable->map", args)
		err, ok := x.(error)
		if !ok {
			panic(fmt.Errorf("Throwable->map expects a Throwable, got: %s", lang.PrintString(x)))
		}
		var via []any
		root := err
		for cur := err; cur != nil; cur = throwableCause(cur) {
			entry := lang.NewMap(kwExType, throwableTypeSym(cur), kwMessage, throwableMessage(cur))
			if d := lang.GetExData(cur); d != nil {
				entry = lang.Assoc(entry, kwExData, d).(lang.IPersistentMap)
			}
			via = append(via, entry)
			root = cur
		}
		m := lang.NewMap(
			kwVia, lang.NewVector(via...),
			kwTrace, lang.NewVector(),
			kwCause, throwableMessage(root))
		if d := lang.GetExData(root); d != nil {
			m = lang.Assoc(m, kwExData, d).(lang.IPersistentMap)
		}
		return m
	})

	// read: one form from a stream — *in* by default (oracle 1.12.5 over a
	// LineNumberingPushbackReader, conformance/tests/read-core-stream.clj):
	// successive reads return successive forms; bare EOF throws "EOF while
	// reading"; (read stream false :eof-val) returns :eof-val at EOF;
	// (read {:eof :done} stream) => :done.
	def("read", func(args ...any) any {
		stream, opts, eofError, eofVal := readArgs("read", args)
		stream.mu.Lock()
		defer stream.mu.Unlock()
		form, err := stream.readOne()
		if err != nil {
			if errors.Is(err, reader.ErrEOF) {
				return resolveReadEOF(opts, eofError, eofVal)
			}
			panic(err)
		}
		return form
	})

	// read+string: like read but also returns the (whitespace-trimmed)
	// string that was consumed, as [form string] (oracle 1.12.5:
	// (with-in-str "  [1 2] tail" (read+string *in*)) => [[1 2] "[1 2]"],
	// then => [tail "tail"]).
	def("read+string", func(args ...any) any {
		stream, opts, eofError, eofVal := readArgs("read+string", args)
		stream.mu.Lock()
		defer stream.mu.Unlock()
		stream.buf = stream.buf[:0]
		stream.capturing = true
		form, err := stream.readOne()
		consumed := strings.TrimSpace(string(stream.buf))
		stream.capturing = false
		if err != nil {
			if errors.Is(err, reader.ErrEOF) {
				return lang.NewVector(resolveReadEOF(opts, eofError, eofVal), consumed)
			}
			panic(err)
		}
		return lang.NewVector(form, consumed)
	})

	// -string-pushback-reader: (s) -> a read/read+string stream over a
	// string — the substrate with-in-str (core.clj) binds *in* to, cljgo's
	// LineNumberingPushbackReader-over-StringReader.
	defPrivate("-string-pushback-reader", func(args ...any) any {
		s, ok := oneArg("-string-pushback-reader", args).(string)
		if !ok {
			panic(fmt.Errorf("-string-pushback-reader expects a string, got: %s", lang.PrintString(args[0])))
		}
		return &coreReadStream{rs: strings.NewReader(s)}
	})

	// default-data-readers: the built-in tag -> reader-fn map, exactly the
	// JVM's two entries (oracle 1.12.5: (sort (map key
	// default-data-readers)) => (inst uuid)). The fns are the same
	// validation paths the reader's own #inst / #uuid literals use.
	instFn := &nativeFn{nm: "default-inst-reader", fn: func(args ...any) any {
		s, ok := oneArg("default-inst-reader", args).(string)
		if !ok {
			panic(fmt.Errorf("Instant literal expects a string, got: %s", lang.PrintString(args[0])))
		}
		inst, err := reader.NewInst(s)
		if err != nil {
			panic(err)
		}
		return inst
	}}
	uuidFn := &nativeFn{nm: "default-uuid-reader", fn: func(args ...any) any {
		s, ok := oneArg("default-uuid-reader", args).(string)
		if !ok {
			panic(fmt.Errorf("UUID literal expects a string, got: %s", lang.PrintString(args[0])))
		}
		u, ok := reader.NewUUID(s)
		if !ok {
			panic(fmt.Errorf("Invalid UUID string: %s", s))
		}
		return u
	}}
	lang.InternVarReplaceRoot(lang.NSCore, lang.NewSymbol("default-data-readers"),
		lang.NewMap(lang.NewSymbol("inst"), instFn, lang.NewSymbol("uuid"), uuidFn))
}
