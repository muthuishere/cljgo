// cljg_socket.go — the Go half of cljg.socket (ADR 0103, spike s59): the
// Bun.listen analog over stdlib net + crypto/tls. TCP + unix-domain
// listen/accept/dial (plain or TLS) and UDP datagrams. A net.Conn IS an
// io.ReadWriteCloser, so a connection's two directions are handed back as the
// SAME Readable/Writable stream handles cljg.stream, cljg.process, and
// cljg.net.http use (stream.go) — ONE stream abstraction, zero adapters
// (spike s59's compile-time proof). Pure Go, so CGO_ENABLED=0 + cljgo dist
// hold, and cljg.socket stays a non-OptIn namespace (net and crypto/tls are
// stdlib, no dependency to isolate). The ergonomic API (listen/accept/dial/
// close, udp-*) is portable Clojure (core/cljg/socket.cljg). Interned as
// :private vars into cljg.socket.
//
// cljg.socket rides the same name-generic embedded-namespace registry as bri
// and the other cljg.* namespaces (the pkg/bri package name is a legacy of
// bri being the first tenant — ADR 0087 §1).
package bri

import (
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// socketListener is the opaque listener handle behind a listen map's
// :-listener key. Never inspected by Clojure directly.
type socketListener struct {
	ln net.Listener
}

// socketConn is the opaque connection handle behind a connection map's
// :-conn key — held only so `close` can shut the underlying socket.
type socketConn struct {
	conn net.Conn
}

// udpSocket is the opaque datagram-socket handle behind a udp-listen map's
// :-socket key.
type udpSocket struct {
	pc net.PacketConn
}

// installSocketShims interns cljg.socket's private Go primitives. All handles
// are opaque to Clojure; these shims are the only way to touch them, and the
// ergonomic listen/accept/dial/close + udp-* API lives in
// core/cljg/socket.cljg.
func installSocketShims(def func(name string, fn func(args ...any) any)) {
	// -socket-listen (opts) -> {:port :addr :-listener} (tcp) or
	// {:path :addr :-listener} (unix). See socketListenShim.
	def("-socket-listen", socketListenShim)
	// -socket-accept (listener-handle) -> connection map. Blocks until a
	// client connects; accepting on a closed listener is an error.
	def("-socket-accept", func(args ...any) any {
		sl, ok := one("-socket-accept", args).(*socketListener)
		if !ok {
			panic(fmt.Errorf("cljg.socket: accept expects a listener handle (from listen), got: %s", lang.PrintString(args[0])))
		}
		conn, err := sl.ln.Accept()
		if err != nil {
			panic(fmt.Errorf("cljg.socket: accept on %s: %w", sl.ln.Addr(), err))
		}
		return socketConnMap(conn)
	})
	// -socket-dial (opts) -> connection map. See socketDialShim.
	def("-socket-dial", socketDialShim)
	// -socket-close (handle) -> nil. Accepts a listener, connection, or udp
	// handle; idempotent.
	def("-socket-close", func(args ...any) any {
		switch h := one("-socket-close", args).(type) {
		case *socketListener:
			_ = h.ln.Close()
		case *socketConn:
			_ = h.conn.Close()
		case *udpSocket:
			_ = h.pc.Close()
		default:
			panic(fmt.Errorf("cljg.socket: close expects a listener, connection, or udp socket handle, got: %s", lang.PrintString(args[0])))
		}
		return nil
	})
	// -udp-listen (opts) -> {:port :addr :-socket}.
	def("-udp-listen", func(args ...any) any {
		if len(args) != 1 {
			panic(fmt.Errorf("-udp-listen expects 1 arg (opts), got %d", len(args)))
		}
		opts, _ := args[0].(lang.IPersistentMap)
		addr := socketHostPort(opts)
		pc, err := net.ListenPacket("udp", addr)
		if err != nil {
			panic(fmt.Errorf("cljg.socket: udp-listen on %s: %w", addr, err))
		}
		port := int64(0)
		if ua, ok := pc.LocalAddr().(*net.UDPAddr); ok {
			port = int64(ua.Port)
		}
		return lang.NewMap(
			lang.NewKeyword("port"), port,
			lang.NewKeyword("addr"), pc.LocalAddr().String(),
			lang.NewKeyword("-socket"), &udpSocket{pc: pc},
		)
	})
	// -udp-send (socket-handle host port data) -> nil. data is a string or
	// byte-array; one datagram per call.
	def("-udp-send", func(args ...any) any {
		if len(args) != 4 {
			panic(fmt.Errorf("-udp-send expects 4 args (socket host port data), got %d", len(args)))
		}
		us, ok := args[0].(*udpSocket)
		if !ok {
			panic(fmt.Errorf("cljg.socket: udp-send expects a udp socket handle (from udp-listen), got: %s", lang.PrintString(args[0])))
		}
		host, port := asString(args[1]), asInt(args[2])
		var payload []byte
		switch d := args[3].(type) {
		case string:
			payload = []byte(d)
		case []byte:
			payload = d
		default:
			panic(fmt.Errorf("cljg.socket: udp-send expects a string or byte-array payload, got: %s", lang.PrintString(args[3])))
		}
		dst, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err != nil {
			panic(fmt.Errorf("cljg.socket: udp-send to %s:%d: %w", host, port, err))
		}
		if _, err := us.pc.WriteTo(payload, dst); err != nil {
			panic(fmt.Errorf("cljg.socket: udp-send to %s:%d: %w", host, port, err))
		}
		return nil
	})
	// -udp-recv (socket-handle opts) -> {:data <string> :host :port}. Blocks
	// for one datagram; opts {:timeout-ms n} bounds the wait.
	def("-udp-recv", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("-udp-recv expects 2 args (socket opts), got %d", len(args)))
		}
		us, ok := args[0].(*udpSocket)
		if !ok {
			panic(fmt.Errorf("cljg.socket: udp-recv expects a udp socket handle (from udp-listen), got: %s", lang.PrintString(args[0])))
		}
		opts, _ := args[1].(lang.IPersistentMap)
		if ms := optInt(opts, "timeout-ms"); ms > 0 {
			_ = us.pc.SetReadDeadline(time.Now().Add(time.Duration(ms) * time.Millisecond))
			defer func() { _ = us.pc.SetReadDeadline(time.Time{}) }()
		}
		buf := make([]byte, 65536)
		n, from, err := us.pc.ReadFrom(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				panic(fmt.Errorf("cljg.socket: udp-recv on %s: timed out after %dms", us.pc.LocalAddr(), optInt(opts, "timeout-ms")))
			}
			panic(fmt.Errorf("cljg.socket: udp-recv on %s: %w", us.pc.LocalAddr(), err))
		}
		fromHost, fromPort := "", int64(0)
		if ua, ok := from.(*net.UDPAddr); ok {
			fromHost, fromPort = ua.IP.String(), int64(ua.Port)
		}
		return lang.NewMap(
			lang.NewKeyword("data"), string(buf[:n]),
			lang.NewKeyword("host"), fromHost,
			lang.NewKeyword("port"), fromPort,
		)
	})
}

// socketListenShim opens a TCP (or unix-domain, or TLS-wrapped TCP) listener.
// opts: :port (0 = ephemeral) :host (default 127.0.0.1) :unix path
// :tls {:cert path :key path}.
func socketListenShim(args ...any) any {
	if len(args) != 1 {
		panic(fmt.Errorf("-socket-listen expects 1 arg (opts), got %d", len(args)))
	}
	opts, _ := args[0].(lang.IPersistentMap)

	var ln net.Listener
	var err error
	if path := optStr(opts, "unix"); path != "" {
		ln, err = net.Listen("unix", path)
		if err != nil {
			panic(fmt.Errorf("cljg.socket: listen on unix socket %s: %w", path, err))
		}
	} else {
		addr := socketHostPort(opts)
		ln, err = net.Listen("tcp", addr)
		if err != nil {
			panic(fmt.Errorf("cljg.socket: listen on %s: %w", addr, err))
		}
	}
	if tlsOpts := optMap(opts, "tls"); tlsOpts != nil {
		certFile, keyFile := optStr(tlsOpts, "cert"), optStr(tlsOpts, "key")
		if certFile == "" || keyFile == "" {
			panic(fmt.Errorf("cljg.socket: listen :tls expects {:cert path :key path}, got: %s", lang.PrintString(tlsOpts)))
		}
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			_ = ln.Close()
			panic(fmt.Errorf("cljg.socket: listen: loading TLS key pair %s / %s: %w", certFile, keyFile, err))
		}
		ln = tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{cert}})
	}

	kvs := []any{
		lang.NewKeyword("addr"), ln.Addr().String(),
		lang.NewKeyword("-listener"), &socketListener{ln: ln},
	}
	if ta, ok := ln.Addr().(*net.TCPAddr); ok {
		kvs = append(kvs, lang.NewKeyword("port"), int64(ta.Port))
	} else {
		kvs = append(kvs, lang.NewKeyword("path"), ln.Addr().String())
	}
	return lang.NewMap(kvs...)
}

// socketDialShim connects out over tcp/unix, optionally wrapping in TLS.
// opts: :host (default 127.0.0.1) :port (required unless :unix) :unix path
// :tls true|{:server-name s} :timeout-ms n.
func socketDialShim(args ...any) any {
	if len(args) != 1 {
		panic(fmt.Errorf("-socket-dial expects 1 arg (opts), got %d", len(args)))
	}
	opts, _ := args[0].(lang.IPersistentMap)
	dialer := &net.Dialer{}
	if ms := optInt(opts, "timeout-ms"); ms > 0 {
		dialer.Timeout = time.Duration(ms) * time.Millisecond
	}

	if path := optStr(opts, "unix"); path != "" {
		conn, err := dialer.Dial("unix", path)
		if err != nil {
			panic(fmt.Errorf("cljg.socket: dial unix socket %s: %w", path, err))
		}
		return socketConnMap(conn)
	}

	host := optStr(opts, "host")
	if host == "" {
		host = "127.0.0.1"
	}
	port := optInt(opts, "port")
	if port <= 0 {
		panic(fmt.Errorf("cljg.socket: dial needs a :port (or a :unix path), got opts: %s", lang.PrintString(args[0])))
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))

	tlsVal := lang.Get(opts, lang.NewKeyword("tls"))
	if tlsVal != nil && tlsVal != false {
		cfg := &tls.Config{ServerName: host}
		if tm, ok := tlsVal.(lang.IPersistentMap); ok {
			if sn := optStr(tm, "server-name"); sn != "" {
				cfg.ServerName = sn
			}
		}
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, cfg)
		if err != nil {
			panic(fmt.Errorf("cljg.socket: tls dial %s: %w", addr, err))
		}
		return socketConnMap(conn)
	}

	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		panic(fmt.Errorf("cljg.socket: dial %s: %w", addr, err))
	}
	return socketConnMap(conn)
}

// socketConnMap wraps a live net.Conn as the duplex connection map handed to
// Clojure: the read side and write side are the SAME stream handles
// cljg.stream operates on (a net.Conn is an io.ReadWriteCloser), so a socket
// composes with reduce/lines/transducers like any other stream. Closing
// either stream — or the :-conn via close — closes the socket.
func socketConnMap(conn net.Conn) lang.IPersistentMap {
	return lang.NewMap(
		lang.NewKeyword("in"), newReadableStream(conn, conn),
		lang.NewKeyword("out"), newWritableStream(conn, conn),
		lang.NewKeyword("local-addr"), conn.LocalAddr().String(),
		lang.NewKeyword("remote-addr"), conn.RemoteAddr().String(),
		lang.NewKeyword("-conn"), &socketConn{conn: conn},
	)
}

// socketHostPort renders opts' :host/:port (defaults 127.0.0.1 / 0) as a
// host:port bind address.
func socketHostPort(opts lang.IPersistentMap) string {
	host := optStr(opts, "host")
	if host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, strconv.Itoa(optInt(opts, "port")))
}
