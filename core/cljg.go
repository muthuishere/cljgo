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

// CljgStreamSource is core/cljg/stream.cljg — cljg.stream: the ONE reducible
// stream abstraction (ADR 0101, spike s56), shared by cljg.process and
// cljg.net.http. A readable stream is Seqable (reduce/into/doseq over chunks in
// constant memory) plus read-line/read-bytes/close; a writable stream is
// buffered write + close. Its Go half (pkg/bri/stream.go, installStreamShims)
// is thin shims over stdlib bufio/io; the surface is portable Clojure.
//
//go:embed cljg/stream.cljg
var CljgStreamSource string

// CljgProcessSource is core/cljg/process.cljg — cljg.process: streaming
// subprocess spawn (ADR 0101). `spawn` keeps a child process live with
// stdin/stdout/stderr wired to cljg.stream handles ({:in :out :err :wait
// :kill}); cljg.io's run-to-completion exec/sh/sh! stay in cljg.io. Its Go half
// (pkg/bri/proc_spawn.go, installProcSpawnShims) is a thin shim over os/exec's
// StdinPipe/StdoutPipe/StderrPipe.
//
//go:embed cljg/process.cljg
var CljgProcessSource string
