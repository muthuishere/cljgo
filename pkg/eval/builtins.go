package eval

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// Out is where println writes. Package-level and swappable for tests; the
// REPL driver may point it elsewhere.
var Out io.Writer = os.Stdout

// nativeFn wraps a Go function as a lang.IFn (the pre-interned builtins of
// design/03 §8 v0). Errors panic, per the IFn-boundary convention.
type nativeFn struct {
	nm string
	fn func(args ...any) any
}

var _ lang.IFn = (*nativeFn)(nil)

func (n *nativeFn) Invoke(args ...any) any     { return n.fn(args...) }
func (n *nativeFn) ApplyTo(args lang.ISeq) any { return n.Invoke(lang.ToSlice(args)...) }
func (n *nativeFn) String() string             { return "#object[" + n.nm + "]" }

// internBuiltins pre-interns the v0 native IFns into the current
// namespace: + - * / = < > pr-str println (design/03 §8, milestone v0).
// Arithmetic goes through lang's numeric tower (int64 fast path, overflow
// checked); = is lang.Equiv.
func (e *Evaluator) internBuiltins() {
	def := func(name string, fn func(args ...any) any) {
		v := e.CurrentNS.Intern(lang.NewSymbol(name))
		v.BindRoot(&nativeFn{nm: name, fn: fn})
	}

	def("+", func(args ...any) any {
		var acc any = int64(0)
		for i, a := range args {
			if i == 0 {
				acc = a
				continue
			}
			acc = lang.Add(acc, a)
		}
		return acc
	})
	def("-", func(args ...any) any {
		if len(args) == 0 {
			panic(fmt.Errorf("wrong number of args (0) passed to: -"))
		}
		if len(args) == 1 {
			return lang.Sub(int64(0), args[0])
		}
		acc := args[0]
		for _, a := range args[1:] {
			acc = lang.Sub(acc, a)
		}
		return acc
	})
	def("*", func(args ...any) any {
		var acc any = int64(1)
		for i, a := range args {
			if i == 0 {
				acc = a
				continue
			}
			acc = lang.Multiply(acc, a)
		}
		if len(args) == 0 {
			return int64(1)
		}
		return acc
	})
	def("/", func(args ...any) any {
		if len(args) == 0 {
			panic(fmt.Errorf("wrong number of args (0) passed to: /"))
		}
		if len(args) == 1 {
			return lang.Divide(int64(1), args[0])
		}
		acc := args[0]
		for _, a := range args[1:] {
			acc = lang.Divide(acc, a)
		}
		return acc
	})
	def("=", func(args ...any) any {
		if len(args) == 0 {
			panic(fmt.Errorf("wrong number of args (0) passed to: ="))
		}
		for i := 1; i < len(args); i++ {
			if !lang.Equiv(args[i-1], args[i]) {
				return false
			}
		}
		return true
	})
	def("<", chainCompare("<", lang.LT))
	def(">", chainCompare(">", lang.GT))

	def("pr-str", func(args ...any) any {
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = lang.PrintString(a)
		}
		return strings.Join(parts, " ")
	})
	def("println", func(args ...any) any {
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = lang.ToString(a)
		}
		fmt.Fprintln(Out, strings.Join(parts, " "))
		return nil
	})
}

func chainCompare(name string, cmp func(x, y any) bool) func(args ...any) any {
	return func(args ...any) any {
		if len(args) == 0 {
			panic(fmt.Errorf("wrong number of args (0) passed to: %s", name))
		}
		for i := 1; i < len(args); i++ {
			if !cmp(args[i-1], args[i]) {
				return false
			}
		}
		return true
	}
}
