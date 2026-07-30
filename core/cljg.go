package core

import _ "embed"

// CljgHTTPSource is core/cljg/http.cljg — cljg.http: the raw HTTP server
// primitive (ADR 0103 wave 1), extracted from bri.web.http. serve (port +
// handler fn, request map in / response map out — the same Ring shape
// bri.web.http speaks at the Go boundary), TLS opts, addr, graceful stop.
// Its Go half (pkg/bri/cljg_http.go, installCljgHTTPShims) owns the shared
// http.Server core bri.web.http's -serve also rides.
//
//go:embed cljg/http.cljg
var CljgHTTPSource string

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

// CljgSecuritySource is core/cljg/security.cljg — cljg.security: the security
// primitive namespace (ADR 0103, renamed from bri.core.security): HS256 JWT
// (sign/verify/issue, alg pinned), argon2id passwords, the composable guard
// family + auto-ban (Ring middleware), sha256/hmac/random/token/uuid/
// base64/hex, and the s65 unified keychain (save/get/delete, native OS store
// with an age-encrypted-file fallback). Crypto/JWT shims live in pkg/bri
// (security.go); the keychain trio lives in the ISOLATED opt-in
// pkg/bri/security so the keyring client links only when required.
//
//go:embed cljg/security.cljg
var CljgSecuritySource string

// CljgSocketSource is core/cljg/socket.cljg — cljg.socket: raw sockets (ADR
// 0103, spike s59). TCP/unix listen+accept+dial (plain or TLS) with
// connections as cljg.stream-composable duplex handles, plus UDP datagrams.
//
//go:embed cljg/socket.cljg
var CljgSocketSource string

// CljgNetDNSSource is core/cljg/net_dns.cljg — cljg.net.dns: DNS lookups (ADR
// 0103, spike s60), the Bun.dns analog. lookup/reverse/mx/txt/srv/cname/
// ns-records over the stdlib pure-Go resolver (PreferGo). Non-OptIn.
//
//go:embed cljg/net_dns.cljg
var CljgNetDNSSource string

// CljgCompressSource is core/cljg/compress.cljg — cljg.compress: stdlib
// compression codecs (ADR 0103 wave 1, spike s61) — gzip/deflate/zlib
// compress+decompress over Go's compress/* (pure stdlib, zero deps), plus
// decompress-on-read streaming wrappers composing with cljg.stream.
// zstd/brotli are DEFERRED to a later opt-in package (binary-size cost).
//
//go:embed cljg/compress.cljg
var CljgCompressSource string
