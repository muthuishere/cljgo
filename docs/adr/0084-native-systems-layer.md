# ADR 0084 — `bri.sys`: one shared native systems layer for OS/SIMD/GPU primitives — cgo-free, via purego + Go-asm + x/sys

Date: 2026-07-24 · Status: accepted (owner-directed: *"a separate spec for cross-OS
cgo behind it — all OS-level primitives against one shared native library of ours:
services management, SIMD, GPU programming primitives, per Win32/mac/Linux."*).
Realizes ADR 0044 (C FFI via purego). Backed by spike s49. Underpins ADR 0083's
`bri.service` and the primitive suite.

## Context

The owner wants a **shared native library of ours** that every OS-level primitive
builds against — services management, SIMD, GPU programming — split per platform
(Win32 / macOS / Linux). The stated mechanism was "cgo."

But **cgo forfeits cljgo's defining property**: `CGO_ENABLED=0` is why cljgo
cross-compiles to every platform from one host (`cljgo dist`, ADR 0077), ships one
static binary, and stays portable (ADR 0023). A cgo build needs a per-target C
toolchain + sysroot and is no longer a pure static binary. Making cgo the
*foundation* would surrender the very thing the project — and the just-made Charm
rejection — is built on.

Spike s49 proves the reframe: **cgo-level primitives are reachable without cgo.**
`purego` + Go-asm + `x/sys`, linked together, build `CGO_ENABLED=0` with **0 cgo**
and **cross-compile all five** ADR-0077 targets.

## Decision

### 1. `bri.sys` — one shared, cgo-FREE native systems layer

A single low-level cljgo package is the "shared native library of ours." It is
**cgo-free by construction**, built on three cgo-free foundations, and every
higher OS-level primitive (`bri.service`, `bri.gpu`, `bri.simd`, …) is a thin,
opt-in layer on it:

- **purego FFI core** (realizes ADR 0044 / spike S21): declare a native symbol,
  bind it to a Go/Clojure fn, call it — one mechanism for **Win32, Cocoa, Metal,
  CUDA, Vulkan** and any `.so`/`.dylib`/`.dll`, with **no cgo and no
  bindings-generator**. The binary stays pure-Go and cross-compiles; it `dlopen`s
  the platform library at **runtime** on the target (which already has it — a GPU
  box has its driver, a Mac has Metal, every OS has its own APIs).
- **Go-assembly SIMD kernels** (`bri.simd`): per-`GOARCH` `.s` implementations
  (AVX2/AVX-512/NEON) with a portable Go fallback, feature-detected via
  `klauspost/cpuid`. Pure-Go — the Go toolchain compiles the asm per target, so it
  cross-compiles.
- **`x/sys` OS primitives**: service management (systemd/launchd/Win32 SCM),
  signals, process, mmap. Pure-Go (already the basis of ADR 0083's `bri.service`).

### 2. Per-platform, one surface

The platform-specific parts live in build-tagged files
(`sys_windows.go` / `sys_darwin.go` / `sys_linux.go` — the owner's "specific Win32
or mac or Linux") selecting the right symbols and paths behind **one** shared
Clojure/Go surface. A caller writes `(sys/service-install …)` or
`(simd/dot f32-a f32-b)`; the platform split is invisible.

### 3. Opt-in, zero-cost, guarantee-preserving

`bri.sys` and each primitive on it are **opt-in linked** (ADR 0074/0076) — an app
pays only for what it uses, and a binary that never touches native primitives
carries none of it. The whole layer keeps `CGO_ENABLED=0` and `cljgo dist`
cross-compile (s49-proven). The runtime cost of purego is a native-lib load on
first use; the tradeoff is that the target must have the native lib present, which
for OS/GPU APIs it always does.

### 4. The one genuine cgo case → a walled, opt-in tier

cgo is unavoidable **only** to *statically link* a C library into the binary
(purego is dynamic/runtime) — e.g. a self-contained binary embedding a C lib with
no runtime dependency. That is the exception: a separate, explicitly opt-in `--cgo`
build tier the author chooses **knowing** it forfeits free cross-compile for that
build. It is never the default and nothing in the pure-Go core depends on it.

## Consequences

- cljgo gains a real systems-programming reach — GPU compute, SIMD hot paths, deep
  OS APIs, service management — as **one shared native layer**, while keeping the
  static-binary + `cljgo dist` guarantee that defines it. The owner's goal without
  the owner's stated cost.
- `bri.service` (ADR 0083) sits on `bri.sys`'s `x/sys` primitives; `bri.gpu` and
  `bri.simd` sit on the purego + asm cores. The suite composes on one foundation.
- Realizes the long-reserved ADR 0044 (purego FFI) as that foundation, and gives
  it a concrete first consumer.
- Each primitive lands on its own ADR → spec → dual-mode → gates, gated on the
  cgo-free + cross-compile + opt-in constraints s49 proved achievable.
- Risk: purego needs correct per-platform symbol/ABI declarations (hand-bound), and
  Go-asm SIMD needs per-arch kernels + the portable fallback — real work, but
  bounded and cgo-free, and each is testable against the fallback.

## Not chosen

- **cgo as the foundation** — forfeits `CGO_ENABLED=0` + `cljgo dist`; rejected on
  the project's most sacred constraint. Reserved only for the opt-in static-link
  tier.
- Scattering platform code across every primitive — one shared `bri.sys` layer,
  not N copies.
