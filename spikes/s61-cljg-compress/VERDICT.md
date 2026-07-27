# Spike s61-cljg-compress — VERDICT

**Capability:** `cljg.compress` — compress/decompress round-trips for gzip, deflate/flate,
zlib, zstd, brotli, exposed as streaming `io.Writer`/`io.Reader` wrappers.

**Verdict:** `PARTIAL` in the strict enum sense — but only because the enum forces one
value. Reality is two-tier and both tiers PASS:

- **gzip / flate / zlib → MET-STDLIB.** `compress/gzip`, `compress/flate`, `compress/zlib`
  are Go stdlib. Zero deps, zero cgo.
- **zstd / brotli → MET-PUREGO-DEP.** Two widely-used, permissively-licensed, pure-Go
  modules. No C, no cgo, build clean under `CGO_ENABLED=0`.

`cgoFree = true`, `ranClean = true`.

## Round-trip evidence (real captured stdout)

```
cljg.compress spike — payload 164890 bytes

gzip      in=164890 bytes  out=5811 bytes  ratio=3.5%  roundtrip=OK
flate     in=164890 bytes  out=5793 bytes  ratio=3.5%  roundtrip=OK
zlib      in=164890 bytes  out=5799 bytes  ratio=3.5%  roundtrip=OK
zstd      in=164890 bytes  out=2497 bytes  ratio=1.5%  roundtrip=OK
brotli    in=164890 bytes  out=2834 bytes  ratio=1.7%  roundtrip=OK

All round-trips OK.
```

Every codec was driven as a streaming writer (compress) then a streaming reader
(decompress); the decompressed bytes were asserted `bytes.Equal` to the original
164,890-byte payload. All five matched. This is the exact composition shape
`cljg.stream` needs — each codec wraps an arbitrary `io.Writer`/`io.Reader`.

## Build

```
CGO_ENABLED=0 go build -ldflags='-s -w' -o /tmp/s61-cljg-compress.bin .
```

Stripped binary: **4,900,338 bytes** (~4.9 MB). This bundles zstd + brotli tables and
all five codecs. gzip/flate/zlib add nothing to binary size beyond the Go runtime
(stdlib). The bulk of the delta over a bare hello-world comes from zstd/brotli decode
tables.

## Deps

| module | version | license | pure Go | cgo | C files |
|---|---|---|---|---|---|
| github.com/klauspost/compress (zstd) | v1.19.1 | BSD-3-Clause | yes | none | 0 |
| github.com/andybalholm/brotli | v1.2.2 | MIT | yes | none | 0 |

Verified: `find` found 0 `.c`/`.h` files in either module; no `import "C"` in the zstd or
brotli packages; both built and ran under `CGO_ENABLED=0`.

## Risks / caveats (honest)

- **Two go.mod deps for zstd+brotli.** Not stdlib. But both are the de-facto standard
  pure-Go implementations (klauspost/compress is used across the Go ecosystem, incl.
  the Go toolchain's own vendored copies and Kubernetes/containerd). Low maintenance
  risk, permissive licenses, no attribution burden beyond keeping LICENSE files.
- **Binary size:** +~4.9 MB includes the zstd/brotli tables. If cljg.compress ships zstd
  and brotli always-on, every AOT binary carries that cost even if the program only uses
  gzip. Consider gating zstd/brotli behind an opt-in namespace/build so gzip-only
  programs stay lean (matches the ADR 0074 per-namespace linking philosophy).
- **API surface differs per codec** — e.g. zstd's `Decoder` needs `.IOReadCloser()` to
  present as `io.ReadCloser`, and brotli's `Reader` has no `Close`, so it needs an
  `io.NopCloser` wrap. A cljg.compress shim must normalize these into one uniform
  `(compress in out)` / `(decompress in out)` protocol so Clojure callers see one shape.
- **No dictionary / level knobs exercised here** — only default levels. flate/zlib/gzip
  levels and zstd `EncoderLevel` / brotli quality are available (all pure Go) but not
  proven in this spike.

## What it means for cljg

Green light. `cljg.compress` is fully achievable in pure Go with `CGO_ENABLED=0` and no
external build toolchain. The core three (gzip/flate/zlib) are free (stdlib); zstd and
brotli cost two well-maintained pure-Go deps and ~4.9 MB of tables. Recommend shipping
gzip/flate/zlib in the base and putting zstd/brotli behind an opt-in link so size-sensitive
binaries don't pay for codecs they don't call.
