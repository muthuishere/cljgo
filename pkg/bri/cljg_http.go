// cljg_http.go — cljg.http's Go half (ADR 0103 wave 1): the raw HTTP
// server core CARVED OUT of bri.web.http's -serve (http.go). This file
// owns the pieces both servers share — the http.Server construction with
// production timeouts and the graceful Shutdown — plus cljg.http's own
// -http-serve shim: one handler fn for the whole server (no mux, no
// routing), request map in / response map out through the SAME
// requestMap/writeResponse conversion bri.web.http uses, so the Ring
// shape at the Go boundary is one contract, not two.
//
// bri.web.http's serveShim (http.go) delegates its server construction
// and drain to newHTTPServer/shutdownServer below; its behavior
// (blocking, SIGTERM drain, the listening line, ops endpoints) is
// unchanged — the framework keeps its opinion, the server became a
// primitive.
package bri

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/muthuishere/cljgo/pkg/lang"
)

var (
	kwHost     = lang.NewKeyword("host")
	kwTLS      = lang.NewKeyword("tls")
	kwCertFile = lang.NewKeyword("cert-file")
	kwKeyFile  = lang.NewKeyword("key-file")
	kwAddr     = lang.NewKeyword("addr")
)

// installCljgHTTPShims interns cljg.http's private Go primitives.
func installCljgHTTPShims(def func(name string, fn func(args ...any) any)) {
	def("-http-serve", cljgHTTPServeShim)
}

// newHTTPServer builds the shared http.Server: production timeouts are
// DEFAULT ON (ADR 0041: the safe stack is what you didn't type) for both
// cljg.http/serve and bri.web.http/serve.
func newHTTPServer(h http.Handler) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// shutdownServer is the shared graceful stop: in-flight requests finish
// (10 s deadline), the listener closes.
func shutdownServer(srv *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

// rawHandler adapts a Clojure handler fn (request-map → response-map)
// into an http.HandlerFunc — the same boundary conversion as bri's adapt,
// minus the route pattern/params (a raw server has no routes). The
// last-resort recover keeps one request from killing the server loop.
func rawHandler(ifn lang.IFn) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				fmt.Fprintf(os.Stderr, "cljg.http: unrecovered handler panic on %s %s: %v\n",
					r.Method, r.URL.Path, rec)
				w.WriteHeader(http.StatusInternalServerError)
				io.WriteString(w, "internal error")
			}
		}()
		res := lang.Apply(ifn, []any{requestMap(r, nil, "")})
		writeResponse(w, res)
	}
}

// cljgHTTPServeShim is cljg.http/-http-serve: bind, serve in the
// background, return the handle {:port :addr :stop}. Never blocks, never
// prints — the raw primitive; blocking + signal drain is bri.web.http's
// framework behavior.
func cljgHTTPServeShim(args ...any) any {
	if len(args) != 2 {
		panic(fmt.Errorf("wrong number of args (%d) passed to: cljg.http/-http-serve (expects 2: [handler opts])", len(args)))
	}
	ifn, ok := args[0].(lang.IFn)
	if !ok {
		panic(fmt.Errorf("cljg.http/serve: :handler must be a function (request-map -> response-map), got: %s", lang.PrintString(args[0])))
	}
	opts := args[1]

	port := 0
	switch v := lang.Get(opts, kwPort).(type) {
	case int64:
		port = int(v)
	case int:
		port = v
	case nil:
		// default 0: bind a free port, read it back via :addr/:port
	default:
		panic(fmt.Errorf("cljg.http/serve: :port must be an int, got: %s", lang.PrintString(v)))
	}
	host := ""
	switch v := lang.Get(opts, kwHost).(type) {
	case string:
		host = v
	case nil:
	default:
		panic(fmt.Errorf("cljg.http/serve: :host must be a string, got: %s", lang.PrintString(v)))
	}

	ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		panic(fmt.Errorf("cljg.http/serve: %w", err))
	}
	srv := newHTTPServer(rawHandler(ifn))

	if tlsOpts := lang.Get(opts, kwTLS); tlsOpts != nil {
		cert, _ := lang.Get(tlsOpts, kwCertFile).(string)
		key, _ := lang.Get(tlsOpts, kwKeyFile).(string)
		if cert == "" || key == "" {
			_ = ln.Close()
			panic(fmt.Errorf("cljg.http/serve: :tls needs :cert-file and :key-file (both file paths), got: %s", lang.PrintString(tlsOpts)))
		}
		go func() { _ = srv.ServeTLS(ln, cert, key) }()
	} else {
		go func() { _ = srv.Serve(ln) }()
	}

	actual := ln.Addr().(*net.TCPAddr).Port
	return lang.NewMap(
		kwPort, int64(actual),
		kwAddr, ln.Addr().String(),
		kwStop, lang.NewFnFunc(func(args ...any) any { shutdownServer(srv); return nil }),
	)
}
