// contrib.go — embedded sources for the org.clojure contrib libraries
// ported natively into cljgo (ADR 0097). Each is a faithful port of its
// upstream .cljc, loaded lazily on first (require …) in the interpreter and
// opt-in-linked in AOT via the same bri.Specs()/genbri/briloader path the
// bri.* and cljg.* namespaces ride (the "bri" package name is a legacy of
// bri being the registry's first tenant). Nothing here is a boot source —
// a binary that never requires a contrib namespace links zero of its bytes.
package core

import _ "embed"

// ClojureToolsCLISource is core/tools_cli.cljg — clojure.tools.cli: the
// GNU-style command-line option parser, ported near-verbatim from
// org.clojure/tools.cli 1.1.230 (spike s52-tools-cli-native, ADR 0097).
// Pure Clojure over clojure.string + clojure.core regex/format — no Go host
// (arg parsing is one-shot, so Mandate A's "hot path is a Go primitive" does
// not apply). Public API: parse-opts, summarize, make-summary-part,
// format-lines, get-default-options, and the deprecated legacy cli.
//
//go:embed tools_cli.cljg
var ClojureToolsCLISource string
