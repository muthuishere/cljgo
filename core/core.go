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
