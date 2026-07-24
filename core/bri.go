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

// BriAuditSource is core/bri/audit.cljg — bri.audit: the security
// audit trail (actor/action/target/ts/severity), structured-stderr
// sink v1 with a clean one-fn seam (ADR 0069).
//
//go:embed bri/audit.cljg
var BriAuditSource string

// BriAuthSource is core/bri/auth.cljg — bri.auth: HS256 JWT (sign/
// verify/issue, alg pinned), argon2id passwords, the composable guard
// family (guard/logged-in-only/role-only/user-only/admin-only) and
// abuse protection (auto-ban), all Ring middleware (ADR 0069).
//
//go:embed bri/auth.cljg
var BriAuthSource string

// BriDBSource is core/bri/db.cljg — bri.db: the one blessed data layer
// (ADR 0072). connect/query/one/one!/exec!/insert!/update!/delete!/tx/
// with-rollback/migrate! over two pure-Go drivers (modernc SQLite default,
// pgx Postgres) behind one API. The Go half lives in pkg/bri (db.go).
//
//go:embed bri/db.cljg
var BriDBSource string

// BriOtelSource is core/bri/otel.cljg — bri.otel: OPT-IN OpenTelemetry
// distributed tracing (ADR 0074). A server-span-per-request middleware
// ((otel/trace)), W3C trace-context propagation, and an OTLP exporter,
// bridging the existing request-id/metrics so logs, metrics, and traces
// correlate. NOT in api-defaults — added explicitly. The Go half (the
// OpenTelemetry SDK wiring) lives in the ISOLATED pkg/bri/otel, linked
// only when an app requires bri.otel.
//
//go:embed bri/otel.cljg
var BriOtelSource string

// BriCLISource is core/bri/cli.cljg — bri.cli: the CLI app-shape of bri
// (ADR 0078). The defcommand/defcommands DSL (mirroring bri.http's
// defroute/defroutes), the UNIFIED PARAMETER MODEL (one declaration → a CLI
// flag AND, in a later increment, an interactive prompt), type coercion +
// default string trim, composable validators, and cli/run (parse → resolve →
// validate → dispatch, with --help/--version/did-you-mean). Pure Clojure —
// no Go shims in this increment.
//
//go:embed bri/cli.cljg
var BriCLISource string

// BriCLIValidateSource is core/bri/cli_validate.cljg — bri.cli.validate: the
// built-in validator constructors for bri.cli parameters (ADR 0078 §3),
// conventionally aliased `v` (v/min, v/max, v/matches, v/email, v/one-of,
// and the composers v/all / v/any / v/not). A validator is a fn value →
// nil|message, so a custom validator is any fn; these are the batteries.
//
//go:embed bri/cli_validate.cljg
var BriCLIValidateSource string
