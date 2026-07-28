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
