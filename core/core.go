// Package core embeds the bootstrap clojure.core source (design/00 §6,
// M1). pkg/eval loads Source into the clojure.core namespace at startup,
// after the Go builtins and the hand-built defmacro are interned; the
// embed lives here (its own top-level package) because go:embed cannot
// reach outside a package directory, and pkg/eval imports it.
package core

import _ "embed"

// Source is the contents of core.clj.
//
//go:embed core.clj
var Source string

// StringSource is the contents of string.cljg — the clojure.string
// namespace, built on the regex core fns and the private `-str-*` host
// primitives (pkg/eval/string_builtins.go). pkg/eval loads it into the
// clojure.string namespace after clojure.core is up (loadClojureString).
// The loader accepts the .cljg extension per ADR 0017.
//
//go:embed string.cljg
var StringSource string

// TestSource is the contents of test.cljg — the interpreted clojure.test
// slice (ADR 0012 / openspec testing-first-class). pkg/eval loads it into
// the clojure.test namespace after clojure.core is up (loadClojureTest).
// The loader accepts the .cljg extension per ADR 0017.
//
//go:embed test.cljg
var TestSource string
