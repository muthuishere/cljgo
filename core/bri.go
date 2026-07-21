// bri.go — embedded sources for the bri application framework
// namespaces (ADR 0041, openspec app-framework T1). The .cljg files
// live under core/bri/; pkg/bri interns the Go host shims into each
// namespace and evaluates these sources on first (require 'bri.*) —
// lazily, via the lib-provider registry, so boot cost is untouched.
package core

import _ "embed"

// BriHTTPSource is core/bri/http.cljg — bri.http: Ring-contract
// handlers on stdlib net/http (routes-as-data → ServeMux), the
// default-on middleware stack, param!/render/dir/health helpers, and
// the in-process test client. The Go half lives in pkg/bri.
//
//go:embed bri/http.cljg
var BriHTTPSource string

// BriHTMLSource is core/bri/html.cljg — bri.html: hiccup-style
// data→escaped-HTML fns, html/page, html/form (CSRF token).
//
//go:embed bri/html.cljg
var BriHTMLSource string

// BriConfigSource is core/bri/config.cljg — bri.config: conf.edn
// (:profiles selected by APP_PROFILE) → APP_* env, one plain map,
// optional conf.schema.edn enforcement, `cljgo config` explain.
//
//go:embed bri/config.cljg
var BriConfigSource string
