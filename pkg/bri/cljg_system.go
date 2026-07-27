// cljg_system.go — the Go half of cljg.system (ADR 0101): the process/
// environment surface clojure.core lacks — one env var, the whole
// environment, process exit, and the argument vector. Thin shims over stdlib
// os (Getenv/LookupEnv/Environ/Exit/Args) — pure Go, so CGO_ENABLED=0 +
// cljgo dist hold, and a non-OptIn namespace (os is stdlib, no dependency to
// isolate). The ergonomic API (getenv with a default, environ, exit, args) is
// portable Clojure (core/cljg/system.cljg). Interned as :private vars into
// cljg.system.
//
// SECURITY (owner doctrine): a shim NEVER logs or bakes an env VALUE — it only
// RETURNS it as data to the caller. -environ hands back an immutable map; the
// value never touches a log line or a literal.
//
// cljg.system rides the same name-generic embedded-namespace registry as bri
// and cljg.io / cljg.os (the pkg/bri package name is a legacy of bri being the
// first tenant — ADR 0087 §1).
package bri

import (
	"fmt"
	"os"
	"strings"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// installSystemShims interns cljg.system's private Go process primitives.
func installSystemShims(def func(name string, fn func(args ...any) any)) {
	// -getenv name -> the variable's value, or nil when unset. Shared with
	// the other namespaces that read env (getenvShim, http.go): one var,
	// value returned, never logged.
	def("-getenv", getenvShim)

	// -environ -> the whole environment as an immutable {name value} map.
	// os.Environ yields "K=V" strings; split on the FIRST '=' (values may
	// contain '='). Returned as data — no value is printed.
	def("-environ", func(args ...any) any {
		if len(args) != 0 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: -environ", len(args)))
		}
		env := os.Environ()
		kvs := make([]any, 0, len(env)*2)
		for _, e := range env {
			if i := strings.IndexByte(e, '='); i >= 0 {
				kvs = append(kvs, e[:i], e[i+1:])
			}
		}
		return lang.NewPersistentHashMap(kvs...)
	})

	// -exit status -> never returns; terminates the process with the int
	// status. The Clojure half defaults status to 0.
	def("-exit", func(args ...any) any {
		os.Exit(asInt(one("-exit", args)))
		return nil // unreachable
	})

	// -args -> the raw process argument vector (os.Args) as an immutable
	// vector of strings; element 0 is the program path.
	def("-args", func(args ...any) any {
		out := make([]any, len(os.Args))
		for i, a := range os.Args {
			out[i] = a
		}
		return lang.NewVectorOwning(out)
	})
}
