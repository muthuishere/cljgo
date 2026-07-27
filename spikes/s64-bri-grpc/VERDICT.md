# Spike s64-bri-grpc — VERDICT

**Capability:** `bri.grpc` — offer gRPC in cljgo.
**The hard question:** can we do it in PURE Go, CGO_ENABLED=0, with NO external
protoc toolchain (no `protoc`, no `protoc-gen-go`, no C compiler)?

## Verdict: MET-PUREGO-DEP (GO-WITH-DEP)

gRPC works end to end in pure Go with **zero external tooling**. The driver
stands up a real gRPC server + client and completes a unary RPC round-trip, and
critically the service is defined from a `.proto` **string compiled in-process**
— no `protoc` anywhere on the machine, no generated `.pb.go`.

- `CGO_ENABLED=0 go build` succeeded.
- Stripped binary: **12,166,578 bytes (~11.6 MB)**.
- 154 packages compiled in; **no `.c`/`.h` files** in any dep.

## The two no-protoc paths — both proven in one driver

1. **In-process compilation** — `github.com/bufbuild/protocompile` parses AND
   links the `.proto` source string entirely in Go (with `WithStandardImports`
   for the well-known types). This is the same parser buf uses; it needs no
   protoc binary.
2. **No-codegen dynamic serving** — from the resulting descriptor we register a
   gRPC service at runtime via `grpc.ServiceDesc` + a generic unary handler, and
   carry request/response as `google.golang.org/protobuf/types/dynamicpb`
   messages. No generated Go types. `dynamicpb.Message` satisfies the proto
   codec because it exposes `ProtoReflect()`, so grpc's default codec marshals
   it transparently.

Together: a cljgo user (or a `bri.grpc` macro) can hand us a `.proto` — as a
file or a REPL form — and we define, serve, and call the service with nothing
installed but the cljgo binary.

## Deps (all pure Go, permissive)

| module | version | license | pure Go |
|---|---|---|---|
| google.golang.org/grpc | v1.82.1 | Apache-2.0 | yes |
| google.golang.org/protobuf | v1.36.11 | BSD-3-Clause | yes |
| github.com/bufbuild/protocompile | v0.14.1 | Apache-2.0 | yes |
| golang.org/x/net | v0.53.0 (indirect) | BSD-3-Clause | yes |
| golang.org/x/sync,sys,text | (indirect) | BSD-3-Clause | yes |
| google.golang.org/genproto/googleapis/rpc | (indirect) | Apache-2.0 | yes |

The `go.mod` require set is small. `go list -m all` lists ~45 modules but most
(otel, envoy/xds, spiffe, gonum, testify) are grpc's **test-only / optional**
deps and are NOT in the 154-package build graph — the compiled binary pulls
only the table above.

## Captured run output (real stdout)

```
compiled in-process (no protoc): service=demo.Greeter method=SayHello
  input=demo.HelloRequest output=demo.HelloReply rpc=/demo.Greeter/SayHello
gRPC server up at 127.0.0.1:63082 (service registered from runtime descriptor)
client sent name="cljgo"
server replied message="hello, cljgo!"
ROUND-TRIP OK: unary gRPC via protocompile + dynamicpb, NO protoc, NO codegen
```

## Honest risks / caveats

- **Size.** grpc + protobuf + protocompile is the heaviest dep set cljgo would
  take: ~11.6 MB stripped for a hello-world. That is roughly 2x a plain
  cljgo hello binary (~5.3 MB). If `bri.grpc` links unconditionally it inflates
  EVERY binary — must be an **opt-in per-namespace link** (ADR 0074 style), not
  in the default core.
- **Ergonomics of the dynamic path.** dynamicpb is stringly-typed
  (`Fields().ByName("name")`, `protoreflect.ValueOfString`). Great for a
  runtime/REPL-driven `bri.grpc`, but a real API surface wants a thin Clojure
  wrapper (map <-> dynamicpb) so users write `{:name "cljgo"}`, not reflection
  calls. That wrapper is real work but pure data-shuffling.
- **Only unary proven here.** Streaming (`grpc.StreamDesc` handlers) is the same
  machinery but not exercised in this spike — treat as a follow-up.
- **protocompile carries golang.org/x/tools, mod, sync** as its parsing deps —
  all pure Go, but they add to the tree. Acceptable.
- **No TLS in the spike** (insecure creds for the loopback test). Production
  bri.grpc must wire `credentials/tls` — pure stdlib crypto/tls, no new risk.

## What it means for cljgo

GO — but gated. gRPC is genuinely achievable with the cljgo purity contract
intact: CGO=0, no protoc, no daemon. The killer result is the **no-protoc
story** — competitors' gRPC-in-language always assumes a protoc install; cljgo
can parse `.proto` in-process and serve dynamically, so "add gRPC" is `cljgo`
+ a `.proto` and nothing else. Ship it as an **opt-in `bri.grpc` namespace**
that only links the grpc stack when used, keeping the default binary small,
with a Clojure map<->dynamicpb wrapper as the user-facing surface.
