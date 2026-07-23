---
title: Install
description: Install cljgo via Homebrew, go install, or a prebuilt release binary — and what needs the Go toolchain (only cljgo build does).
---

## Homebrew (macOS & Linux)

```bash
brew install muthuishere/tap/cljgo
```

## go install

With a Go 1.26+ toolchain:

```bash
go install github.com/muthuishere/cljgo/cmd/cljgo@latest
```

## Prebuilt binaries

Grab a binary for your platform from
[the latest release](https://github.com/muthuishere/cljgo/releases/latest)
(macOS/Linux/Windows, amd64 + arm64).

## What needs the Go toolchain

`cljgo repl`, `cljgo run` and Go interop work from the binary alone — **no Go
toolchain installed**.

**`cljgo build` additionally needs the Go toolchain on `PATH`**: it emits Go
source and invokes `go build`. A release binary pins the published runtime
module in the generated `go.mod`
(`require github.com/muthuishere/cljgo v<version>`), and the first build
fetches it from the Go module proxy once per machine (~1 MB, a few seconds).
For cgo-based interop features you also need a C toolchain with
`CGO_ENABLED=1`.

## Verify

```bash
cljgo version
```

Then head to the [quickstart](/cljgo/quickstart/).
