# ADR 0077 — `cljgo dist`: one command cross-compiles a cljgo program to every platform

Date: 2026-07-24 · Status: accepted (owner-directed: *"distribute with a command
which creates exes across all matrix possible since its golang"* — and *"i need
as core"*). Builds on ADR 0001 (single-file build), ADR 0021 (project
`build.cljgo`), ADR 0028 (release-pin go.mod). Core tooling, not a bri namespace.

## Context

cljgo compiles a Clojure program to a single **pure-Go, `CGO_ENABLED=0`** static
binary (ADR 0023, the sacred constraint). That constraint has a payoff nobody has
cashed in yet: because there is **no cgo**, the Go toolchain cross-compiles a
cljgo program to *any* `GOOS/GOARCH` with **zero extra toolchain** — just
`GOOS=… GOARCH=… CGO_ENABLED=0 go build`. No C cross-compiler, no per-platform
CI runner, no container. This is something JVM Clojure structurally cannot do: a
`.jar` needs a JVM on the target, and GraalVM `native-image` needs a builder per
target OS/arch. "Write a CLI in Clojure, ship a native binary to every desktop
and server" is a capability unique to cljgo — but only if producing that matrix
is one command instead of a hand-rolled loop.

Today `cljgo build` produces exactly one binary for the host. Shipping a release
means manually invoking it under five different `GOOS/GOARCH` combinations and
hashing the outputs. That friction is the whole gap.

This is a **core** capability (owner: *"i need as core"*), not a bri feature: it
applies to every cljgo program — `lib`, `cli`, `web` — because the pure-Go
guarantee is a property of the compiler, not of bri. It lives beside `build` /
`run` / `test` / `new`.

## Decision

### 1. `cljgo dist` — cross-compile the matrix in one command

```
cljgo dist                          # the default matrix (below) → ./dist/
cljgo dist --target linux/amd64,windows/amd64   # explicit subset
cljgo dist --all                    # every GOOS/GOARCH `go tool dist list` reports
cljgo dist -o release               # output directory (default ./dist)
cljgo dist <file.clj>               # single-file input, same as `cljgo build`
```

It accepts the **same input** as `cljgo build`: a bare invocation (or a step) uses
the project `build.cljgo` install artifact (ADR 0021); a `.clj`/`.cljc`/`.cljg`
positional is the single-file fast path (ADR 0001). `dist` is distribution, so —
unlike `build` — its default is the **matrix**, never host-only (host-only is
already `cljgo build`).

### 2. Default matrix — the five mainstream desktop/server targets

`darwin/arm64`, `darwin/amd64`, `linux/amd64`, `linux/arm64`, `windows/amd64` —
Apple Silicon + Intel Mac, x86-64 + ARM Linux, and Windows: effectively every real
end user. `--target os/arch,…` selects an explicit subset (validated against
`go tool dist list`, so a typo is a named error, not a cryptic `go build`
failure); `--all` builds every pair Go supports (the long tail — freebsd, riscv64,
wasm, … — is opt-in, not noise in the default).

### 3. Output layout + integrity

```
dist/
  <name>_darwin-arm64
  <name>_darwin-amd64
  <name>_linux-amd64
  <name>_linux-arm64
  <name>_windows-amd64.exe
  checksums.txt          # sha256, one `<hex>  <file>` line per artifact (sha256sum -c format)
```

`<name>` is the project/artifact name (or the single-file derived name); Windows
artifacts carry `.exe`. `checksums.txt` is `sha256sum -c`-compatible so a
downloader can verify. A final summary table prints each target, size, and status.

### 4. Prepare once, link per target (the implementation seam)

The generated Go module is **target-independent** (pure Go, identical for every
`GOOS/GOARCH`), and generating it is the expensive part (compile + emit + for a
project, dependency resolution + `go get` + `go mod tidy`). So `dist` **generates
the module once** and runs `go build` **once per target** against that one module
— it does not recompile the Clojure per target. Two small, reusable seams make
this exact:
- `emit.GoBuildTarget(dir, out, goos, goarch)` — `GoBuild` with `GOOS`/`GOARCH`
  set and `CGO_ENABLED=0` forced; the existing `emit.GoBuild` delegates to it with
  an empty (host) target, so nothing about the host build changes.
- module preparation is split from the single `go build` in both the single-file
  path (`emit.PrepareModule`) and the project path
  (`build.Plan`'s prepare step, exposed as `Plan.DistInstall`), each returning the
  ready-to-link genDir. `dist` calls prepare once, then loops `GoBuildTarget`.

The `-ldflags=-s -w -trimpath` release stripping (ADR 0023) applies to every
target unchanged.

## Consequences

- One command turns a cljgo project into a full set of native binaries for every
  mainstream platform, ready for a GitHub Release / Homebrew tap / `curl | sh` —
  the concrete thing that makes "ship a CLI written in Clojure" real, and a
  capability JVM Clojure cannot match.
- Zero new build infrastructure: cross-compilation is the Go toolchain doing what
  it already does for pure-Go code; `dist` is orchestration (matrix, output
  layout, checksums) over the existing, unchanged build pipeline. The `CGO_ENABLED=0`
  guarantee is what makes it free — a future cgo dependency would break `dist`,
  which is one more reason the pure-Go constraint stays sacred.
- The host build path is untouched (`GoBuild` still exists, still host-only), so
  no risk to `cljgo build` / the conformance dual-harness.
- Scope of v1: the install artifact (an executable). Library projects (no
  executable artifact) are out of scope — `dist` errors clearly, pointing at
  `cljgo publish` (ADR 0054). A `dist` step in `build.cljgo` and archive/tarball
  packaging (`.tar.gz`/`.zip` per target, for release uploaders) are natural
  follow-ups, deferred here.
- Pairs with ADR 0078 (bri.cli): bri.cli makes the CLIs first-class, `dist` ships
  them to every platform — together, the "native CLI in Clojure" story.
