package corelib

import (
	"fmt"
	"os"
	"strings"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// io_builtins.go — slurp/spit, clojure.core's file-convenience pair
// (fundamentals audit 2026-07, A-list "I/O convenience"). On the Go host
// f is a FILE PATH string (the JVM coerces String through io/reader's
// URL-then-file rules; cljgo has no clojure.java.io — C-class in the
// audit — so URL/socket/stream coercions are out of scope and a non-string
// f throws). Registered into internBuiltins by ONE line
// (internIOBuiltins(def)), per the merge-friendly discipline.
//
// Oracle (JVM Clojure 1.12.5, 2026-07-21, conformance/tests/slurp-spit*.clj):
//   (spit p "hello\n") => nil, (slurp p) => "hello\n"
//   (spit p 42) coerces content via str => (slurp p) => "42"; nil => "";
//   :kw => ":kw"; {:a 1} => "{:a 1}"
//   (spit p "b" :append true) appends; :append false truncates
//   (slurp "no-such-file") throws (message names the file)
//   :encoding "UTF-8" accepted on both (the default)
//
// DEVIATION (documented): only UTF-8 :encoding is supported — Go strings
// are UTF-8 and cljgo pulls in no charset tables; any other :encoding
// throws instead of silently mis-reading.

// ioOpts parses slurp/spit's trailing keyword options.
type ioOpts struct {
	append bool
}

func parseIOOpts(ctx string, args []any) ioOpts {
	if len(args)%2 != 0 {
		panic(fmt.Errorf("%s: options must be key-value pairs, got odd count %d", ctx, len(args)))
	}
	var o ioOpts
	for i := 0; i < len(args); i += 2 {
		k, ok := args[i].(lang.Keyword)
		if !ok {
			panic(fmt.Errorf("%s: option keys must be keywords, got: %s", ctx, lang.PrintString(args[i])))
		}
		switch k.Name() {
		case "append":
			o.append = lang.IsTruthy(args[i+1])
		case "encoding":
			enc, _ := args[i+1].(string)
			if !strings.EqualFold(enc, "UTF-8") && !strings.EqualFold(enc, "UTF8") {
				panic(fmt.Errorf("%s: unsupported :encoding %s — cljgo's Go host supports UTF-8 only", ctx, lang.PrintString(args[i+1])))
			}
		default:
			// The JVM silently ignores unknown io/reader opts; match it.
		}
	}
	return o
}

func ioPath(ctx string, v any) string {
	s, ok := v.(string)
	if !ok {
		panic(fmt.Errorf("%s: cannot coerce %s to a file path — cljgo supports file-path strings only (no clojure.java.io reader/writer/URL coercions on the Go host)", ctx, lang.PrintString(v)))
	}
	return s
}

func internIOBuiltins(def func(name string, fn func(args ...any) any) *lang.Var) {
	// (slurp f & opts) -> the file's contents as a string.
	def("slurp", func(args ...any) any {
		if len(args) < 1 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: slurp", len(args)))
		}
		parseIOOpts("slurp", args[1:])
		data, err := os.ReadFile(ioPath("slurp", args[0]))
		if err != nil {
			panic(fmt.Errorf("slurp: %w", err))
		}
		return string(data)
	})

	// (spit f content & opts) -> nil. Writes (str content) to the file,
	// truncating unless :append is truthy.
	def("spit", func(args ...any) any {
		if len(args) < 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: spit", len(args)))
		}
		opts := parseIOOpts("spit", args[2:])
		path := ioPath("spit", args[0])
		content := ""
		if args[1] != nil { // (str nil) => "", per clojure.core
			content = lang.ToString(args[1])
		}
		flags := os.O_WRONLY | os.O_CREATE
		if opts.append {
			flags |= os.O_APPEND
		} else {
			flags |= os.O_TRUNC
		}
		f, err := os.OpenFile(path, flags, 0o644)
		if err != nil {
			panic(fmt.Errorf("spit: %w", err))
		}
		defer f.Close()
		if _, err := f.WriteString(content); err != nil {
			panic(fmt.Errorf("spit: %w", err))
		}
		return nil
	})
}
