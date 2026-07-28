package core

import _ "embed"

// CljxTestSource is core/cljx/test.cljg — cljx.test: Bun-flavoured first-class
// testing (ADR 0105), the first member of the `cljx.*` DEVELOPER EXPERIENCE
// tier (ADR 0085's fourth tier). Mocks, spies, stubs, expect matchers,
// lifecycle hooks and output-capture-by-default, built ON clojure.test (a
// cljx.test test IS a clojure.test test — one runner, one report, dual
// harness for free; clojure.test itself is untouched, per the precedence
// principle). Pure Clojure over the primitives spike s66 proved behave
// identically interpreted and compiled — NO Go shim. Loaded LAZILY (a row in
// bri.Specs()); NOT a boot source, so a binary that never requires it pays
// zero bytes.
//
//go:embed cljx/test.cljg
var CljxTestSource string
