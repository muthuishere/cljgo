package eval

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// internChanExtras registers the M4+ concurrency additions (design/05 §4):
// timeout, alts!/alts!!, dropping-buffer/sliding-buffer, and a chan that also
// accepts a buffer spec. It is wired into internBuiltins by ONE line
// (`e.internChanExtras(def)`) so the M4 v0 channel builtins in builtins.go stay
// untouched and this track owns its own file (STAY-IN-LANE). These are
// core.async names absent from clojure.core, so this is a precedence-safe
// addition (CLAUDE.md), never a shadow/rename. All ops are Go builtins wrapping
// pkg/lang runtime helpers, so REPL and AOT binary behave identically.
func (e *Evaluator) internChanExtras(def func(string, func(...any) any) *lang.Var) {
	// (chan) / (chan n) keep M4 v0 behaviour; (chan buf) adds buffer-policy
	// channels. This rebinds chan's root AFTER builtins.go's plain chan (this
	// helper is called last in internBuiltins), extending it without editing
	// that definition.
	def("chan", func(args ...any) any {
		switch len(args) {
		case 0:
			return lang.NewChan(0)
		case 1:
			switch a := args[0].(type) {
			case int64:
				return lang.NewChan(int(a))
			case *lang.BufferSpec:
				return lang.NewChanBuffered(a)
			default:
				panic(fmt.Errorf("chan expects an integer buffer size or a buffer, got: %s", lang.PrintString(args[0])))
			}
		default:
			panic(fmt.Errorf("wrong number of args (%d) passed to: chan", len(args)))
		}
	})

	// (timeout ms) → a channel that auto-closes after ms milliseconds.
	def("timeout", func(args ...any) any {
		ms, ok := oneArg("timeout", args).(int64)
		if !ok {
			panic(fmt.Errorf("timeout expects an integer ms, got: %s", lang.PrintString(args[0])))
		}
		return lang.NewTimeout(ms)
	})

	// (dropping-buffer n) / (sliding-buffer n) → buffer specs for chan.
	bufFn := func(op string, make func(int) *lang.BufferSpec) func(...any) any {
		return func(args ...any) any {
			n, ok := oneArg(op, args).(int64)
			if !ok {
				panic(fmt.Errorf("%s expects an integer size, got: %s", op, lang.PrintString(args[0])))
			}
			return make(int(n))
		}
	}
	def("dropping-buffer", bufFn("dropping-buffer", lang.DroppingBuffer))
	def("sliding-buffer", bufFn("sliding-buffer", lang.SlidingBuffer))

	// (alts! ports) / (alts! ports :default v). alts!! is an alias (no parking
	// distinction without IOC, design/05 §4). Returns [val port]; a :default
	// hit returns [v :default].
	kwDefault := lang.NewKeyword("default")
	altsFn := func(op string) func(...any) any {
		return func(args ...any) any {
			if len(args) == 0 {
				panic(fmt.Errorf("wrong number of args (0) passed to: %s", op))
			}
			var ports []*lang.Channel
			for s := lang.Seq(args[0]); s != nil; s = s.Next() {
				c, ok := s.First().(*lang.Channel)
				if !ok {
					panic(fmt.Errorf("%s expects take-ports (channels), got: %s", op, lang.PrintString(s.First())))
				}
				ports = append(ports, c)
			}
			hasDefault := false
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
				if k == kwDefault {
					hasDefault = true
					defVal = opts[i+1]
				}
			}
			return lang.Alts(ports, hasDefault, defVal)
		}
	}
	def("alts!", altsFn("alts!"))
	def("alts!!", altsFn("alts!!"))
}
