# Spike s49 — VERDICT: the shared native systems layer is cgo-FREE (purego + Go-asm + x/sys)

Date: 2026-07-24 · Owner: *"a separate spec for cross-OS cgo behind it — all OS-level
primitives against one shared native library of ours: services management, SIMD, GPU
programming primitives, per-platform (Win32 / mac / Linux)."*

## The tension this resolves

cgo forfeits cljgo's defining property: `CGO_ENABLED=0` is **why** cljgo
cross-compiles to every platform from one host (`cljgo dist`, ADR 0077), ships one
static binary, and stays portable (ADR 0023). A cgo build needs a C toolchain +
sysroot per target and is no longer a pure static binary. So a cgo *foundation*
would trade away the thing the whole project — and the just-made Charm rejection —
is built on.

**The reframe: you do not need cgo to reach cgo-level primitives.** Everything the
owner listed has a cgo-FREE mechanism, proven here.

## Result: cgo-free, all targets

`probe.sh` links the three foundations together:

- host **`CGO_ENABLED=0` build OK** (2.9 MB), **0 cgo** in the closure.
- **cross-compiles all 5** ADR-0077 targets.

| OS-level primitive | cgo-free mechanism | note |
|---|---|---|
| **services management** (`bri.service`) | `golang.org/x/sys` — systemd / launchd / Win32 SCM | pure Go (already in ADR 0083 / s48) |
| **SIMD** (`bri.simd`) | **Go assembly** (per-`GOARCH` `.s`), `klauspost/cpuid` to feature-detect | pure Go; the Go toolchain compiles asm per target — cross-compiles |
| **GPU / native OS APIs** (Win32, Cocoa, Metal, CUDA, Vulkan, …) | **purego** (ADR 0044) — `dlopen`/`dlsym` the native lib at **runtime** | the Go binary is pure-Go and cross-compiles; it loads the platform lib on the target (which has it anyway — a GPU box has its driver, a Mac has Metal) |

## Architecture: one shared cgo-free native layer

A single low-level cljgo package — call it **`bri.sys`** (the "shared native library
of ours") — provides the platform primitives, all cgo-free:

- a **purego FFI core** (realizing ADR 0044 / spike S21): declare a native symbol,
  bind it to a Go fn, call it — one mechanism for Win32/Cocoa/Metal/CUDA/Vulkan and
  any `.so`/`.dylib`/`.dll`, no cgo, no bindings-generator.
- **Go-asm SIMD kernels** with a portable Go fallback (feature-detected), so
  hot paths use AVX/NEON where present and still run everywhere.
- **`x/sys` OS primitives** (service management, signals, process, mmap).
- **per-platform files** behind build tags (`_windows.go` / `_darwin.go` /
  `_linux.go`) selecting the right symbols/paths — the "specific Win32 or mac or
  Linux" split, with a shared Clojure/Go surface above it.

The higher primitives (`bri.service`, `bri.gpu`, `bri.simd`, …) are thin layers on
`bri.sys`; each is opt-in linked (ADR 0074/0076) so an app pays only for what it
uses, and the whole thing keeps `CGO_ENABLED=0` + `cljgo dist`.

## The one genuine cgo case → a walled, opt-in tier

cgo is only unavoidable to **statically link** a C library into the binary (purego
is dynamic/runtime) — e.g. shipping a self-contained binary that embeds a C lib
with no runtime dependency. That is the exception, not the foundation: a separate,
explicitly opt-in `--cgo` build tier that the author chooses *knowing* it forfeits
free cross-compile for that build. The default and everything above stays pure-Go.

## Recommendation

Build the shared native systems layer **cgo-free on purego + Go-asm + x/sys**
(realizes ADR 0044). It gives the owner exactly the goal — one shared native
library, OS/SIMD/GPU primitives, per-platform — **without** surrendering the static
binary or `cljgo dist`. Reserve true cgo for an opt-in static-link tier only. This
becomes ADR 0084 (the native systems layer) — a separate spec, as the owner asked,
but cgo-free by construction.
