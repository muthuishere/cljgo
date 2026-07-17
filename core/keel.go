// keel.go — embedded sources for the keel application framework
// namespaces (ADR 0041, openspec app-framework T1). The .cljg files
// live under core/keel/; pkg/keel interns the Go host shims into each
// namespace and evaluates these sources on first (require 'keel.*) —
// lazily, via the lib-provider registry, so boot cost is untouched.
package core

import _ "embed"

// KeelHTTPSource is core/keel/http.cljg — keel.http: Ring-contract
// handlers on stdlib net/http (routes-as-data → ServeMux), the
// default-on middleware stack, param!/render/dir/health helpers, and
// the in-process test client. The Go half lives in pkg/keel.
//
//go:embed keel/http.cljg
var KeelHTTPSource string

// KeelHTMLSource is core/keel/html.cljg — keel.html: hiccup-style
// data→escaped-HTML fns, html/page, html/form (CSRF token).
//
//go:embed keel/html.cljg
var KeelHTMLSource string

// KeelConfigSource is core/keel/config.cljg — keel.config: conf.edn
// (:profiles selected by APP_PROFILE) → APP_* env, one plain map,
// optional conf.schema.edn enforcement, `cljgo config` explain.
//
//go:embed keel/config.cljg
var KeelConfigSource string
