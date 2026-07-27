// match.go — embedded source for clojure.core.match (ADR 0097), the
// org.clojure/core.match public API natively. Like the cljg.* stdlib and bri
// namespaces it is lazy + opt-in: loaded on first (require
// 'clojure.core.match) via the lib-provider registry, never at boot, so a
// program that never matches pays nothing. The heavy compiler is the Go host
// primitive -match-compile (pkg/bri/match.go, installMatchShims); this .cljg
// file is the thin macro surface.
package core

import _ "embed"

// CoreMatchSource is core/match.cljg — clojure.core.match: match / matchv /
// matchm / match-let, compiling to a Maranget decision tree. The compiler
// proper is the interned :private -match-compile primitive; the macros here
// parse the clause forms and call it at macroexpand time.
//
//go:embed match.cljg
var CoreMatchSource string
