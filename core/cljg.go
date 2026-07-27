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
