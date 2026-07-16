package eval

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// registerTestBuiltins interns the private host seams that the interpreted
// clojure.test slice (core/test.cljg, ADR 0012) needs beyond core
// try/catch. Interned via a single internBuiltins line (like -guarded-call).
//
// Currently just -re-find?: (is (thrown-with-msg? Class #"re" body)) has to
// test the caught exception's message against a regex, and cljgo has no
// clojure.core/re-find yet. Real regex (not substring) is used — the runtime
// already carries the compiled-pattern cache (lang.CachedCompileRegexp) and
// the reader produces reader.Regex for #"..." literals — so thrown-with-msg?
// matches JVM clojure.test semantics.
func (e *Evaluator) registerTestBuiltins(defPrivate func(string, func(args ...any) any)) {
	// -re-find? reports whether re (a #"..." pattern literal) matches
	// anywhere in s — the boolean core of clojure.core/re-find, all
	// thrown-with-msg? needs. A non-string (e.g. nil) message never matches.
	defPrivate("-re-find?", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: -re-find?", len(args)))
		}
		re, ok := args[0].(*reader.Regex)
		if !ok {
			panic(fmt.Errorf("-re-find? expects a regex pattern (#\"...\"), got: %s", lang.PrintString(args[0])))
		}
		s, ok := args[1].(string)
		if !ok {
			return false
		}
		return lang.CachedCompileRegexp(re.Pattern).FindStringIndex(s) != nil
	})
}
