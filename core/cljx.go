package core

import _ "embed"

// CljxCoreSource is core/cljx/core.cljg — cljx.core: opt-in ergonomics (ADR
// 0106). `cljx` = "clj extensions": the tier of things that make WRITING cljgo
// nicer, none of them required. add!/del!/bump!/upd!/put-in!/upd-in!/clear!/
// toggle! are transparent aliases over the swap! form each docstring names,
// and dbg prints-and-returns so it drops into a threading pipeline. Pure
// Clojure, no Go shim, no dependency; nothing here shadows clojure.core.
//
//go:embed cljx/core.cljg
var CljxCoreSource string

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
