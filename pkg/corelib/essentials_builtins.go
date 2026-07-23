package corelib

import (
	"fmt"
	"io"
	"math"
	"os"
	"reflect"
	"strings"

	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// essentials_builtins.go — stdio, atom-vals, var-reflection and runtime
// essentials (fundamentals batch A1, core gap audit 2026-07-23): read-line /
// flush / newline / line-seq / file-seq substrate, compare-and-set! /
// swap-vals! / reset-vals! / reset-meta!, bound? / thread-bound?, class,
// infinite?, load-string. Registered into internBuiltins by ONE line
// (internEssentialsBuiltins(def, defPrivate)), per the merge-friendly
// discipline. The pure-Clojure halves (with-in-str, line-seq, file-seq,
// requiring-resolve, ..) live in core/core.clj on top of the private
// substrate here.
//
// Oracle (JVM Clojure 1.12.5, 2026-07-23, scratch a1batch/oracle-a1.clj):
//   (with-in-str "a\nb" [(read-line) (read-line) (read-line)]) => ["a" "b" nil]
//   (with-in-str "a\r\nb" [(read-line) (read-line)]) => ["a" "b"]
//   (with-out-str (prn (newline))) => "\nnil\n"
//   (flush) => nil; *flush-on-newline* => true; *command-line-args* => nil
//   (let [a (atom 1)] [(compare-and-set! a 1 2) @a (compare-and-set! a 99 3) @a])
//     => [true 2 false 2]
//   (let [a (atom 1)] [(swap-vals! a inc) (swap-vals! a + 10)]) => [[1 2] [2 12]]
//   (let [a (atom 1)] [(reset-vals! a 5) @a]) => [[1 5] 5]
//   (let [a (atom 1 :meta {:a 1})] [(reset-meta! a {:b 2}) (meta a)])
//     => [{:b 2} {:b 2}]
//   (def bx 1) (def by) [(bound? #'bx) (bound? #'by) (bound? #'bx #'by)]
//     => [true false false]
//   (def ^:dynamic bz 1) [(thread-bound? #'bz)
//                         (binding [bz 2] (thread-bound? #'bz))
//                         (thread-bound? #'bx)] => [false true false]
//   (load-string "(def lsx 41) (+ lsx 1)") => 42; (load-string "") => nil
//   (class nil) => nil; (= (class 1) (type 1)) => true;
//   [(type (with-meta {} {:type :T}))
//    (= (class (with-meta {} {:type :T})) (class {}))] => [:T true]
//   (infinite? ##Inf) => true; (infinite? ##-Inf) => true;
//   (infinite? 1.5) / (infinite? 1) / (infinite? 1/2) / (infinite? ##NaN)
//     => false; (infinite? "a") throws ClassCastException
//
// DEVIATIONS (documented):
//   - compare-and-set! compares via Go interface == (lang.Atom.CompareAndSet):
//     identity for pointer-shaped values (collections), VALUE equality for
//     unboxed scalars (int64/float64/string). The JVM compares boxed-object
//     identity, so (compare-and-set! (atom 100000) 100000 x) is false on the
//     JVM (uncached Long boxes) but true here — a strict superset of JVM
//     successes for scalars; collection cases match (identity both sides).
//   - read-line treats '\n' (and "\r\n") as the line terminator; a bare '\r'
//     (classic-Mac ending) is NOT a terminator — byte-at-a-time reading from
//     an arbitrary io.Reader has no pushback for the JVM's \r-peek.
//   - the file-seq substrate (-directory?/-dir-children) represents files as
//     PATH STRINGS, consistent with how slurp/spit treat paths (io_builtins.go
//     — cljgo has no java.io.File); children come back in os.ReadDir's sorted
//     order where the JVM's listFiles order is OS-dependent.
//   - load-string needs `eval`, so in an AOT-compiled binary it fails with
//     the honest ADR 0046 stub error (the CLJS model: no analyzer is linked).

// inReader resolves *in* to an io.Reader for read-line/line-seq — the
// mirror of outWriter for the read side (with-in-str binds *in* to a
// -string-reader).
func inReader(ctx string) io.Reader {
	if r, ok := lang.VarIn.Deref().(io.Reader); ok {
		return r
	}
	panic(fmt.Errorf("%s: *in* is not a reader: %s", ctx, lang.PrintString(lang.VarIn.Deref())))
}

// readLineFrom reads one line from r byte-at-a-time (no over-read, so
// successive calls on the same reader continue where the last stopped):
// the line without its terminator, or nil at EOF with nothing read.
func readLineFrom(r io.Reader) any {
	var sb strings.Builder
	buf := make([]byte, 1)
	readAny := false
	for {
		n, err := r.Read(buf)
		if n > 0 {
			readAny = true
			if buf[0] == '\n' {
				return strings.TrimSuffix(sb.String(), "\r")
			}
			sb.WriteByte(buf[0])
		}
		if err != nil {
			if !readAny {
				return nil
			}
			return strings.TrimSuffix(sb.String(), "\r")
		}
	}
}

// flusher is the optional flush seam an *out* target may implement
// (bufio.Writer's Flush; os.File has Sync instead, handled separately).
type flusher interface{ Flush() error }

func internEssentialsBuiltins(def func(name string, fn func(args ...any) any) *lang.Var, defPrivate func(name string, fn func(args ...any) any)) {
	// --- stdio ------------------------------------------------------------

	// read-line: one line from *in*, nil at EOF (oracle above).
	def("read-line", func(args ...any) any {
		if len(args) != 0 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: read-line", len(args)))
		}
		return readLineFrom(inReader("read-line"))
	})

	// flush: flush *out*, return nil. Go's os.Stdout is unbuffered so this
	// is usually a no-op; a buffered writer bound to *out* gets Flush()ed.
	def("flush", func(args ...any) any {
		if len(args) != 0 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: flush", len(args)))
		}
		if f, ok := outWriter().(flusher); ok {
			_ = f.Flush()
		}
		return nil
	})

	// newline: write a newline to *out*, return nil.
	def("newline", func(args ...any) any {
		if len(args) != 0 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: newline", len(args)))
		}
		fmt.Fprint(outWriter(), "\n")
		return nil
	})

	// -read-line-from: line-seq's substrate — one line from an explicit
	// reader (any Go io.Reader; with-in-str's -string-reader qualifies).
	defPrivate("-read-line-from", func(args ...any) any {
		r, ok := oneArg("-read-line-from", args).(io.Reader)
		if !ok {
			panic(fmt.Errorf("line-seq expects a reader (a Go io.Reader; *in* qualifies), got: %s", lang.PrintString(args[0])))
		}
		return readLineFrom(r)
	})

	// -string-reader: with-in-str's substrate — an in-memory io.Reader over s.
	defPrivate("-string-reader", func(args ...any) any {
		s, ok := oneArg("-string-reader", args).(string)
		if !ok {
			panic(fmt.Errorf("with-in-str expects a string, got: %s", lang.PrintString(args[0])))
		}
		return strings.NewReader(s)
	})

	// --- file-seq substrate ----------------------------------------------

	// -directory?: is the path a directory (false for missing paths, like
	// the JVM's File.isDirectory on a nonexistent file).
	defPrivate("-directory?", func(args ...any) any {
		p, ok := oneArg("-directory?", args).(string)
		if !ok {
			panic(fmt.Errorf("file-seq expects a file-path string, got: %s", lang.PrintString(args[0])))
		}
		fi, err := os.Stat(p)
		return err == nil && fi.IsDir()
	})

	// -dir-children: the directory's entries as joined path strings, in
	// os.ReadDir's sorted order; nil (not an error) when unreadable, so a
	// racing deletion doesn't blow up a lazy walk. Joined with "/" on
	// EVERY OS (Go's os accepts it on Windows too) so file-seq output —
	// and its frozen conformance expectation — is platform-stable.
	defPrivate("-dir-children", func(args ...any) any {
		p, ok := oneArg("-dir-children", args).(string)
		if !ok {
			panic(fmt.Errorf("file-seq expects a file-path string, got: %s", lang.PrintString(args[0])))
		}
		entries, err := os.ReadDir(p)
		if err != nil || len(entries) == 0 {
			return nil
		}
		prefix := strings.TrimSuffix(p, "/")
		children := make([]any, 0, len(entries))
		for _, e := range entries {
			children = append(children, prefix+"/"+e.Name())
		}
		return lang.NewList(children...)
	})

	// --- atom vals --------------------------------------------------------

	// compare-and-set!: set iff the current value is the expected one
	// (see the == DEVIATION note above); returns whether it swapped.
	def("compare-and-set!", func(args ...any) any {
		if len(args) != 3 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: compare-and-set!", len(args)))
		}
		a, ok := args[0].(*lang.Atom)
		if !ok {
			panic(fmt.Errorf("compare-and-set! expects an atom, got: %s", lang.PrintString(args[0])))
		}
		return a.CompareAndSet(args[1], args[2])
	})

	// swap-vals!: like swap! but returns [old new].
	def("swap-vals!", func(args ...any) any {
		if len(args) < 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: swap-vals!", len(args)))
		}
		a, ok := args[0].(*lang.Atom)
		if !ok {
			panic(fmt.Errorf("swap-vals! expects an atom, got: %s", lang.PrintString(args[0])))
		}
		f, ok := args[1].(lang.IFn)
		if !ok {
			panic(fmt.Errorf("swap-vals! expects a function, got: %s", lang.PrintString(args[1])))
		}
		rest := lang.NewList(args[2:]...)
		for {
			old := a.Deref()
			nw := f.ApplyTo(lang.NewCons(old, rest))
			if a.CompareAndSet(old, nw) {
				return lang.NewVector(old, nw)
			}
		}
	})

	// reset-vals!: like reset! but returns [old new].
	def("reset-vals!", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: reset-vals!", len(args)))
		}
		a, ok := args[0].(*lang.Atom)
		if !ok {
			panic(fmt.Errorf("reset-vals! expects an atom, got: %s", lang.PrintString(args[0])))
		}
		for {
			old := a.Deref()
			if a.CompareAndSet(old, args[1]) {
				return lang.NewVector(old, args[1])
			}
		}
	})

	// reset-meta!: wholesale-replace an iref's metadata map, returning it.
	// Note: on a Var, cljgo's SetMeta re-tags :ns (var.go), so the map the
	// var then carries is m + :ns — the returned value is m itself, as on
	// the JVM.
	def("reset-meta!", func(args ...any) any {
		ref, mv := twoArgs("reset-meta!", args)
		m, ok := mv.(lang.IPersistentMap)
		if !ok {
			panic(fmt.Errorf("reset-meta! expects a map, got: %s", lang.PrintString(mv)))
		}
		switch r := ref.(type) {
		case *lang.Atom:
			r.SetMeta(m)
		case *lang.Var:
			r.SetMeta(m)
		case *lang.Namespace:
			r.ResetMeta(m)
		default:
			panic(fmt.Errorf("reset-meta! expects an atom, var or namespace, got: %s", lang.PrintString(ref)))
		}
		return m
	})

	// --- var state --------------------------------------------------------

	// bound?: all vars have a value — a root binding or an in-effect
	// thread binding (Var.IsBound, the JVM's isBound).
	def("bound?", func(args ...any) any {
		if len(args) == 0 {
			panic(fmt.Errorf("wrong number of args (0) passed to: bound?"))
		}
		for _, x := range args {
			v, ok := x.(*lang.Var)
			if !ok {
				panic(fmt.Errorf("bound? expects vars, got: %s", lang.PrintString(x)))
			}
			if !v.IsBound() {
				return false
			}
		}
		return true
	})

	// thread-bound?: all vars have a thread binding in effect on the
	// calling goroutine (the JVM's getThreadBinding != null).
	def("thread-bound?", func(args ...any) any {
		if len(args) == 0 {
			panic(fmt.Errorf("wrong number of args (0) passed to: thread-bound?"))
		}
		for _, x := range args {
			v, ok := x.(*lang.Var)
			if !ok {
				panic(fmt.Errorf("thread-bound? expects vars, got: %s", lang.PrintString(x)))
			}
			if !v.HasThreadBinding() {
				return false
			}
		}
		return true
	})

	// --- runtime essentials ----------------------------------------------

	// class: the value's host type — nil for nil, otherwise exactly what
	// `type` returns MINUS the :type-metadata override (the one JVM
	// class-vs-type difference, frozen here the same way: cljgo's stand-in
	// class value is reflect.Type, comparable via =, misc_builtins.go).
	def("class", func(args ...any) any {
		x := oneArg("class", args)
		if x == nil {
			return nil
		}
		return reflect.TypeOf(x)
	})

	// infinite?: is the number (cast to double, like the JVM's ^double
	// param) positive or negative infinity. Mirrors NaN? (sorted_builtins.go).
	def("infinite?", func(args ...any) any {
		x := oneArg("infinite?", args)
		if !lang.IsNumber(x) {
			panic(fmt.Errorf("infinite?: not a number: %s", lang.PrintString(x)))
		}
		return math.IsInf(lang.AsFloat64(x), 0)
	})

	// load-string: read every form from s and eval them in order,
	// returning the last value (nil for an empty string). Rides the
	// `eval` var, so it works wherever eval does (REPL/`cljgo run`;
	// the ADR 0046 stub error in AOT binaries — DEVIATION note above).
	def("load-string", func(args ...any) any {
		s, ok := oneArg("load-string", args).(string)
		if !ok {
			panic(fmt.Errorf("load-string expects a string, got: %s", lang.PrintString(args[0])))
		}
		r := reader.New(strings.NewReader(s), reader.WithResolver(NSResolver()))
		forms, err := r.ReadAll()
		if err != nil {
			panic(err)
		}
		evalVar := lang.NSCore.FindInternedVar(lang.NewSymbol("eval"))
		if evalVar == nil {
			panic(fmt.Errorf("load-string: eval is not available"))
		}
		evalFn, ok := evalVar.Deref().(lang.IFn)
		if !ok {
			panic(fmt.Errorf("load-string: eval is not available"))
		}
		var last any
		for _, form := range forms {
			last = evalFn.Invoke(form)
		}
		return last
	})
}
