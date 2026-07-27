# VERDICT — s62-cljg-http-serve

**Capability:** `cljg.http/serve` (Bun.serve analog) — the raw HTTP SERVER primitive.

**Verdict:** `MET-STDLIB` (+ one optional pure-Go dep, `golang.org/x/net`, only for h2c cleartext HTTP/2).

- `cgoFree`: **true** — `CGO_ENABLED=0 go build -ldflags='-s -w'` succeeded.
- `ranClean`: **true** — every round-trip asserted its body and negotiated protocol.
- Stripped binary: **6,949,634 bytes** (~6.6 MB) at `/tmp/s62-cljg-http-serve.bin`.

## What is stdlib vs what needs a dep

| Sub-capability | Package | Status |
|---|---|---|
| HTTP/1.1 server + client GET | `net/http`, `net` | stdlib |
| HTTPS, in-process self-signed cert | `crypto/tls`, `crypto/x509`, `crypto/ecdsa` | stdlib |
| HTTP/2 over TLS (ALPN `h2` auto-negotiated) | `net/http` (`ServeTLS` auto-configures h2) | stdlib |
| Graceful shutdown (drain in-flight, stop accepting) | `http.Server.Shutdown` | stdlib |
| **h2c** (HTTP/2 cleartext, no TLS) | `golang.org/x/net/http2/h2c` + `golang.org/x/net/http2` | **needs dep** |

The ENTIRE server primitive — bind, handle, HTTPS with an in-process self-signed
cert, HTTP/2 over TLS, graceful drain — is Go standard library, zero deps, CGO-free.
The **only** thing outside stdlib is **h2c** (prior-knowledge HTTP/2 over plaintext),
which is niche; most deployments terminate TLS and get h2 for free from stdlib.

## Deps

| Module | Version | License | Pure Go | CGO=0 | Needed for |
|---|---|---|---|---|---|
| `golang.org/x/net` | v0.57.0 | BSD-3-Clause (Go Authors) | yes (no `.c`/`.h` in `http2/`) | yes (built in the CGO=0 binary above) | h2c only |
| `golang.org/x/text` | v0.40.0 | BSD-3-Clause | yes | yes | indirect (x/net) |

If h2c is dropped, the spike compiles with **zero** third-party modules.

## Captured run output (real stdout)

```
== s62 cljg.http/serve feasibility ==
[1] plain HTTP/1.1 server + client GET
  body: "hello world over HTTP/1.1"  proto: HTTP/1.1
  OK  http1 body == "hello world over HTTP/1.1"
[4] graceful shutdown of HTTP/1.1 server
  OK  in-flight request drained == "drained-cleanly"
  OK  server refuses new connections post-Shutdown
[2] HTTPS (self-signed, in-process) + HTTP/2 auto-negotiation
  body: "hello tls over HTTP/2.0"  proto: HTTP/2.0  tls-version: 0x304
  OK  https body == "hello tls over HTTP/2.0"
  OK  negotiated proto (ALPN h2) == "HTTP/2.0"
[3] h2c (HTTP/2 cleartext) via golang.org/x/net/http2/h2c
  body: "hello cleartext over HTTP/2.0"  proto: HTTP/2.0
  OK  h2c body == "hello cleartext over HTTP/2.0"
  OK  h2c negotiated proto == "HTTP/2.0"
== ALL ROUND-TRIPS PASSED ==
```

- TLS version `0x304` = TLS 1.3.
- `[2]` proves ALPN negotiated `h2` from stdlib `ServeTLS` alone (no x/net on the TLS path).

## Risks / caveats (honest)

- **h2c is the only non-stdlib piece.** It pulls `golang.org/x/net` (+ indirect
  `x/text`). Both pure Go, BSD-3-Clause, widely used, maintained by the Go team —
  the safest possible dep. But it *is* a go.mod entry. If cljgo wants a truly
  zero-dep `serve`, expose h2c behind a build tag / optional and keep the default
  path stdlib-only.
- Self-signed cert generation is fully in-process (`ecdsa.GenerateKey` +
  `x509.CreateCertificate`), no `openssl`, no files on disk — good for a
  `serve` with `:tls :self-signed` dev mode.
- No perf work done here; this is a correctness/feasibility spike.
- `x/net/http2` retains the pclntab in the stripped binary (same as any Go code);
  size is dominated by `net/http` + `crypto/tls`, not h2c.

## What it means for cljgo

`cljg.http/serve` can be a thin stdlib wrapper: a `net.Listener` on `127.0.0.1:0`
(or a configured addr), an `http.Handler` bridging to a cljg fn, and
`Server.Shutdown` for graceful stop. HTTPS/h2/dev-cert all come free from stdlib.
Only opt into `x/net` when a user asks for cleartext HTTP/2.

## Extraction from `bri.web.http` (ADR 0103 plan) — assessment

Low-to-moderate effort. The server primitive already lives inside `bri.web`'s
HTTP host fns, so extracting a standalone `cljg.http/serve` is mostly *moving* the
`http.Server` + listener + graceful-shutdown plumbing down into a new
`pkg/cljghttp` (or `cljg.http`) layer with a minimal handler-fn contract, then
having `bri.web` build routing/middleware on top of that primitive instead of
owning the server. The main care-point is the handler bridge (cljg fn ⇄
`http.HandlerFunc`) and request/response value shapes staying stable so `bri.web`
is a pure superset — but nothing here needs cgo, a daemon, or a redesign.
