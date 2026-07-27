package core

import _ "embed"

// CljgNetHTTPSource is core/cljg/net_http.cljg — cljg.net.http: the outbound
// HTTP client (ADR 0087), the first namespace in cljgo's cljg.* stdlib tier.
// General mechanism (any program wants an HTTP client), pure-Go over net/http.
// Its Go half (pkg/bri/net_http.go, installNetHTTPShims) is one request shim
// plus JSON/url encode; everything else is portable Clojure.
//
//go:embed cljg/net_http.cljg
var CljgNetHTTPSource string

// CljgOSSource is core/cljg/os.cljg — cljg.os: OS-level primitives (ADR 0088).
// This increment is the cron SCHEDULER (next-fire math is a Go shim over stdlib
// time in pkg/bri/os_cron.go; the loop is portable Clojure). Service management
// is the reserved next increment.
//
//go:embed cljg/os.cljg
var CljgOSSource string

// CljgIOSource is core/cljg/io.cljg — cljg.io: filesystem primitives (ADR 0089).
// The structural file/path/directory surface clojure.core lacks (slurp/spit
// stay core) plus process exec (sh/exec/sh!); its Go half (pkg/bri/io_fs.go +
// io_proc.go, installIOShims) is thin shims over stdlib os + path/filepath +
// os/exec, the ergonomic API portable Clojure.
//
//go:embed cljg/io.cljg
var CljgIOSource string

// CljDataCSVSource is core/data_csv.cljg — clojure.data.csv (ADR 0097): a
// native cljgo port of org.clojure/data.csv 1.1.0. read-csv/write-csv over a
// PURE STRING surface (cljgo has no java.io.Reader/Writer), pure Clojure over
// clojure.string/escape — NO Go shim. Loaded LAZILY (rides the same
// name-generic registry as cljg.io, bri.Specs()); NOT a boot source, so a
// binary that never requires it pays zero bytes.
//
//go:embed data_csv.cljg
var CljDataCSVSource string

// CljgCacheSource is core/cljg/cache.cljg — cljg.cache: the fundamental
// in-process cache (ADR 0093, re-homed to cljg.* by ADR 0102) — a TTL map with
// singleflight behind the `Cache` protocol. Pure Clojure over atoms +
// promise/swap-vals! + cljg.os/now; no Go shim, no dependency. Users bring their
// own backend by implementing `Cache`.
//
//go:embed cljg/cache.cljg
var CljgCacheSource string

// CljgJobsSource is core/cljg/jobs.cljg — cljg.jobs: the fundamental in-process
// job queue (ADR 0094, re-homed to cljg.* by ADR 0102) — a core.async worker
// pool behind the `Queue` protocol. Pure Clojure over clojure.core.async (ADR
// 0040) + an atom; no Go shim, no dependency. Users bring a durable backend by
// implementing `Queue`.
//
//go:embed cljg/jobs.cljg
var CljgJobsSource string

// CljgSecretsSource is core/cljg/secrets.cljg — cljg.secrets: the OPT-IN
// pluggable secret store (ADR 0086, re-homed to cljg.* by ADR 0102). Fetch a
// secret by URI scheme (env://KEY, keychain://service/account) with a left→right
// fallback chain; secrets are MASKED by default (the raw value lives in
// metadata, never printed) and unwrapped only by an explicit `reveal`. The Go
// half (the pure-Go OS-keychain client) lives in the ISOLATED pkg/bri/secrets,
// linked only when an app requires this namespace.
//
//go:embed cljg/secrets.cljg
var CljgSecretsSource string

// CljgDataCastSource is core/cljg/data_cast.cljg — cljg.data.cast: the one
// blessed data layer (ADR 0072, re-homed to cljg.* by ADR 0102).
// connect/query/one/one!/exec!/insert!/update!/delete!/tx/with-rollback/migrate!
// plus the cast/cast! input gate, over two pure-Go drivers (modernc SQLite
// default, pgx Postgres) behind one API. The Go half lives in pkg/bri/db (db.go).
//
//go:embed cljg/data_cast.cljg
var CljgDataCastSource string

// CljgSystemSource is core/cljg/system.cljg — cljg.system: process +
// environment primitives (ADR 0101). getenv/environ/exit/args over stdlib os;
// values RETURNED as data, never printed (owner secret doctrine). Non-OptIn.
//
//go:embed cljg/system.cljg
var CljgSystemSource string

// CljgDateSource is core/cljg/date.cljg — cljg.date: time primitives (ADR
// 0101). Public monotonic nano-time (promoting the `time` macro's private
// -nano-time technique), wall-clock now, and since/since-ms. Non-OptIn.
//
//go:embed cljg/date.cljg
var CljgDateSource string

// CljgStreamSource is core/cljg/stream.cljg — cljg.stream: the reducible
// readable/writable stream handle (ADR 0101, spike s56) over Go io.Reader/
// io.Writer, reused by cljg.process and cljg.net.http :as :stream.
//
//go:embed cljg/stream.cljg
var CljgStreamSource string

// CljgProcessSource is core/cljg/process.cljg — cljg.process: streaming
// subprocess spawn (ADR 0101) over exec.Cmd pipes wrapped as cljg.stream.
//
//go:embed cljg/process.cljg
var CljgProcessSource string

// CljgNetDNSSource is core/cljg/net_dns.cljg — cljg.net.dns: DNS lookups (ADR
// 0103, spike s60), the Bun.dns analog. lookup/reverse/mx/txt/srv/cname/
// ns-records over the stdlib pure-Go resolver (PreferGo). Non-OptIn.
//
//go:embed cljg/net_dns.cljg
var CljgNetDNSSource string
