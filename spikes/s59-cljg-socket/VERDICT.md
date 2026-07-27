# VERDICT — s59-cljg-socket (cljg.socket, Bun.listen analog)

**Verdict: MET-STDLIB**

`cljg.socket` is fully achievable in pure Go with the standard library `net`
package. No third-party module, no cgo, no C compiler, no running daemon.
Builds clean under `CGO_ENABLED=0` and runs the full round-trip.

## Build / run facts

- `CGO_ENABLED=0 go build -ldflags='-s -w'` — **succeeded**
- Stripped binary size: **4,761,634 bytes** (~4.5 MB)
- Ran clean: **yes** — all five round-trips completed
- Deps: **none** (stdlib only — no `require` block in go.mod)

## Dependencies

| module | license | pure Go |
|--------|---------|---------|
| (none — Go stdlib `net`, `crypto/tls`, `crypto/x509`, `bufio`, `io`) | BSD-3-Clause (Go) | yes |

## Captured run output

```
== cljg.socket feasibility round-trips (pure Go stdlib net) ==
[tcp ]  listen 127.0.0.1:63027  -> sent "hello over tcp\n"  got "hello over tcp\n"
[udp ]  listen 127.0.0.1:60389  -> sent "hello over udp"  got "hello over udp" from 127.0.0.1:60389
[unix]  listen /var/folders/cb/fw9_2_fd39n1jn_xm5r7q1dh0000gn/T/cljgsock2375368483/echo.sock  -> sent "hello over unix\n"  got "hello over unix\n"
[strm]  net.Conn as io.ReadWriteCloser, scanned lines: [line-one line-two]
[tls ]  listen 127.0.0.1:63031  -> sent "hello over tls\n"  got "hello over tls\n"
== all socket round-trips completed ==
```

## What was proven

1. **TCP** — `net.Listen("tcp", ...)` + `Accept` echo server, `net.Dial` client
   writes and reads back the same bytes. Real round-trip.
2. **UDP** — `net.ListenPacket("udp", ...)` with `WriteTo` (sendto) / `ReadFrom`
   (readfrom) on both server and client sockets. Real datagram round-trip.
3. **Stream shape** — a `net.Conn` is assigned into `io.ReadWriteCloser` /
   `io.Reader` / `io.Writer` (compile-time proof) and driven purely through the
   stream API (`bufio.Scanner`, `io.Copy`, `io.WriteString`). This is exactly
   the shape `cljg.stream` (ADR 0101) expects — a socket wraps as a stream with
   zero adapters.

## Bun parity notes

- **Unix sockets** (Bun `unix:` option): supported — `net.Listen("unix", path)` /
  `net.Dial("unix", ...)`, proven in the run.
- **TLS sockets** (Bun `tls:` option): supported — `crypto/tls` (`tls.Listen`,
  `tls.Dial`) wraps a TCP conn and is itself a `net.Conn`, so it drops straight
  into the same stream model. Proven with an in-memory self-signed cert.
- **Bun.listen semantics**: Go's model is blocking-goroutine-per-conn rather than
  Bun's callback (`socket.data`/`.open`/`.close`) event handlers. cljg would
  expose either an accept-loop returning conns or a handler callback over
  goroutines — a small API-shape decision, not a capability gap.

## Gaps / risks (honest)

- **No capability gap.** Everything Bun.listen offers at the socket layer (TCP,
  UDP, unix, TLS) is in the stdlib.
- **Datagram vs stream API split**: UDP (`PacketConn`, `ReadFrom`/`WriteTo`) is
  not a `Conn` stream by default; cljg must expose it as a separate datagram
  primitive, not fold it into `cljg.stream`. (A connected UDP `net.Dial("udp")`
  DOES give a stream `Conn`, if that shape is wanted.)
- **Platform**: unix domain sockets work on macOS/Linux; on Windows they exist
  from Win10+ but path semantics differ — worth a conformance note if Windows is
  a target.
- **Deadlines/timeouts**: Go uses `SetReadDeadline`/`SetDeadline` rather than
  Bun's per-listener timeout option — cljg maps these to socket options.

## Meaning for cljg

Ship `cljg.socket` on stdlib `net` with zero dependencies. TCP/UDP/unix/TLS all
land in one thin host-fn layer, and because a `net.Conn` already satisfies
`io.ReadWriteCloser`, sockets compose directly with `cljg.stream` (ADR 0101) —
no bridging code. Recommend GO.
