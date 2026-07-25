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
