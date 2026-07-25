// bri.go — embedded sources for the bri application framework
// namespaces (ADR 0041, openspec app-framework T1). The .cljg files
// live under core/bri/; pkg/bri interns the Go host shims into each
// namespace and evaluates these sources on first (require 'bri.*) —
// lazily, via the lib-provider registry, so boot cost is untouched.
package core

import _ "embed"

// BriHTTPSource is core/bri/http.cljg — bri.web.http: Ring-contract
// handlers on stdlib net/http (routes-as-data → ServeMux), the
// default-on middleware stack, param!/render/dir/health helpers, and
// the in-process test client. The Go half lives in pkg/bri.
//
//go:embed bri/http.cljg
var BriHTTPSource string

// BriHTMLSource is core/bri/html.cljg — bri.web.html: hiccup-style
// data→escaped-HTML fns, html/page, html/form (CSRF token).
//
//go:embed bri/html.cljg
var BriHTMLSource string

// BriConfigSource is core/bri/config.cljg — bri.core.config: conf.edn
// (:profiles selected by APP_PROFILE) → APP_* env, one plain map,
// optional conf.schema.edn enforcement, `cljgo config` explain.
//
//go:embed bri/config.cljg
var BriConfigSource string

// BriAuditSource is core/bri/audit.cljg — bri.core.audit: the security
// audit trail (actor/action/target/ts/severity), structured-stderr
// sink v1 with a clean one-fn seam (ADR 0069).
//
//go:embed bri/audit.cljg
var BriAuditSource string

// BriAuthSource is core/bri/auth.cljg — bri.core.security: HS256 JWT (sign/
// verify/issue, alg pinned), argon2id passwords, the composable guard
// family (guard/logged-in-only/role-only/user-only/admin-only) and
// abuse protection (auto-ban), all Ring middleware (ADR 0069).
//
//go:embed bri/auth.cljg
var BriAuthSource string

// BriDBSource is core/bri/db.cljg — bri.core.data: the one blessed data layer
// (ADR 0072). connect/query/one/one!/exec!/insert!/update!/delete!/tx/
// with-rollback/migrate! over two pure-Go drivers (modernc SQLite default,
// pgx Postgres) behind one API. The Go half lives in pkg/bri (db.go).
//
//go:embed bri/db.cljg
var BriDBSource string

// BriOtelSource is core/bri/otel.cljg — bri.core.telemetry: OPT-IN OpenTelemetry
// distributed tracing (ADR 0074). A server-span-per-request middleware
// ((otel/trace)), W3C trace-context propagation, and an OTLP exporter,
// bridging the existing request-id/metrics so logs, metrics, and traces
// correlate. NOT in api-defaults — added explicitly. The Go half (the
// OpenTelemetry SDK wiring) lives in the ISOLATED pkg/bri/otel, linked
// only when an app requires bri.core.telemetry.
//
//go:embed bri/otel.cljg
var BriOtelSource string

// BriCLISource is core/bri/cli.cljg — bri.cli: the CLI app-shape of bri
// (ADR 0078). The defcommand/defcommands DSL (mirroring bri.web.http's
// defroute/defroutes), the UNIFIED PARAMETER MODEL (one declaration → a CLI
// flag AND, in a later increment, an interactive prompt), type coercion +
// default string trim, composable validators, and cli/run (parse → resolve →
// validate → dispatch, with --help/--version/did-you-mean). Pure Clojure —
// no Go shims in this increment.
//
//go:embed bri/cli.cljg
var BriCLISource string

// BriSecretsSource is core/bri/secrets.cljg — bri.core.secrets: the OPT-IN
// pluggable secret store (ADR 0086, realizing ADR 0060 / spike S39). Fetch a
// secret by URI scheme (env://KEY, keychain://service/account) with a
// left→right fallback chain; secrets are MASKED by default (the raw value
// lives in metadata, never printed) and unwrapped only by an explicit
// `reveal`. The Go half (the pure-Go OS-keychain client) lives in the
// ISOLATED pkg/bri/secrets, linked only when an app requires this namespace.
//
//go:embed bri/secrets.cljg
var BriSecretsSource string

// BriCLIValidateSource is core/bri/cli_validate.cljg — bri.cli.validate: the
// built-in validator constructors for bri.cli parameters (ADR 0078 §3),
// conventionally aliased `v` (v/min, v/max, v/matches, v/email, v/one-of,
// and the composers v/all / v/any / v/not). A validator is a fn value →
// nil|message, so a custom validator is any fn; these are the batteries.
//
//go:embed bri/cli_validate.cljg
var BriCLIValidateSource string

// BriCLIAuthSource is core/bri/cli_auth.cljg — bri.cli.auth: built-in
// credential auth (ADR 0080, credential core). Prompt for an API key with echo
// off (bri.cli/ask-secret), store it in the OS keychain (bri.core.secrets), and
// build the Authorization header. Requiring it transitively requires the
// opt-in bri.core.secrets, so the keychain client links only into a CLI that
// uses auth.
//
//go:embed bri/cli_auth.cljg
var BriCLIAuthSource string

// BriOpenAPISource is core/bri/openapi.cljg — bri.web.openapi: an OpenAPI-driven
// typed HTTP client (ADR 0090, realizing ADR 0075's bri.openapi battery under the
// ADR 0085 taxonomy). Pure Clojure over cljg.net.http — no Go shims; the spec is
// the contract, so a client built from it routes params + attaches auth with no
// per-endpoint code. The :auth-fn seam is where ADR 0080's login-backed auth
// composes in without hard-requiring bri.cli.auth.
//
//go:embed bri/openapi.cljg
var BriOpenAPISource string

// BriCacheSource is core/bri/cache.cljg — bri.core.cache: the fundamental
// in-process cache (ADR 0093) — a TTL map with singleflight behind the `Cache`
// protocol. Pure Clojure over atoms + promise/swap-vals! + cljg.os/now; no Go
// shim, no dependency. Users bring their own backend by implementing `Cache`.
//
//go:embed bri/cache.cljg
var BriCacheSource string

// BriJobsSource is core/bri/jobs.cljg — bri.core.jobs: the fundamental
// in-process job queue (ADR 0094) — a core.async worker pool behind the `Queue`
// protocol. Pure Clojure over clojure.core.async (ADR 0040) + an atom; no Go
// shim, no dependency. Users bring a durable backend by implementing `Queue`.
//
//go:embed bri/jobs.cljg
var BriJobsSource string

// BriCLIAPISource is core/bri/cli_api.cljg — bri.cli.api: an OpenAPI client that
// logs in AUTOMATICALLY (ADR 0091, realizing ADR 0080). Pure composition of
// bri.web.openapi (client + :auth-fn seam), bri.cli.auth (credential core → OS
// keychain), and bri.cli prompts — no Go shims. A new namespace (not bri.cli) so
// requiring it opts into the transitive keychain link while plain bri.cli apps
// stay lean.
//
//go:embed bri/cli_api.cljg
var BriCLIAPISource string
