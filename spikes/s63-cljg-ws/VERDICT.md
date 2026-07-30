# Spike s63-cljg-ws — VERDICT

**Capability:** `cljg.ws` — raw websocket connection primitive (duplex read-frame / write-frame stream).

**Verdict: MET-PUREGO-DEP**

Go stdlib has **no** websocket implementation (`golang.org/x/net/websocket` is
frozen/discouraged and not stdlib). A pure-Go third-party lib is required.
`github.com/coder/websocket` (the maintained successor to `nhooyr.io/websocket`)
does the job with **zero transitive dependencies**, builds cleanly under
`CGO_ENABLED=0`, and gives a clean duplex `Read(ctx) -> (type, []byte)` /
`Write(ctx, type, []byte)` shape that maps directly onto a `cljg.stream`.

## Build / run facts

- `CGO_ENABLED=0 go build -ldflags='-s -w'` — **succeeded** (cgoFree = true).
- Stripped binary: **6,123,634 bytes** (~5.8 MB) at `/tmp/s63-cljg-ws.bin`.
- Round-trip **ran clean** (ranClean = true): text + binary frames echoed and asserted.

## Dependencies

| module | version | license | pure Go? | transitive deps |
|---|---|---|---|---|
| github.com/coder/websocket | v1.8.15 | ISC | yes (no C files, CGO=0 OK) | **none** — only stdlib + its own `internal/*` |

`go list -m all` shows exactly one third-party module. `go.sum` has 2 lines
(the module + its go.mod hash). No C sources anywhere in the module cache dir.

Alternative: `github.com/gorilla/websocket` — also pure Go, BSD-3, zero external
deps, widely used. coder/websocket is preferred here: `context`-native API,
built-in `net/http.Handler`-style `Accept`, and a slightly leaner surface.

## Real captured run output

```
echo ws server listening on 127.0.0.1:63014
client dialled ws://127.0.0.1:63014
TEXT   sent="hello cljg.ws — unicode ✓" recv="hello cljg.ws — unicode ✓" type=MessageText
TEXT   OK (echoed payload + type match)
BINARY sent=00 01 02 fe ff 7f 80 recv=00 01 02 fe ff 7f 80 type=MessageBinary
BINARY OK (echoed bytes + type match)
RESULT: PASS - duplex ws round-trip (text + binary) succeeded
```

## Risks / caveats (honest)

- Adds one module to `go.mod` (`github.com/coder/websocket`). It is pure Go,
  ISC-licensed, actively maintained, zero transitive deps — about as low-risk as
  a third-party dep gets, but it is still not stdlib.
- coder/websocket's `Read`/`Write` are **whole-message**; for streaming large
  frames you'd use `Reader`/`Writer`. The cljg.ws primitive can start
  message-oriented (matches the demo) and add streaming later.
- Per-message compression: coder/websocket dropped permessage-deflate in newer
  versions (was experimental). If cljg.ws needs deflate, gorilla still has it.
- Wasm: coder/websocket also compiles to `js/wasm` (uses the browser WebSocket).
  Not exercised here but relevant if cljg ever targets wasm.

## What it means for cljg

Ship `cljg.ws` as a thin wrapper over coder/websocket. The Accept/Dial + duplex
Read/Write pair is exactly the `cljg.stream` duplex contract, both server-upgrade
and client-dial covered. No cgo, no daemon, no build toolchain. Add the one dep.
