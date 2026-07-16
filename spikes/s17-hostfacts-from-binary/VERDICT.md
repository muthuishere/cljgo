# S17 verdict — Go-interop host facts from a downloaded binary

**Answer: YES, and it's simpler than expected.** `go/packages` resolves
stdlib host facts with **no go.mod at all** in the target directory — the
current failure is not a `go/packages` limitation, it's that
`pkg/emit/program.go`'s fallback chain calls `FindRuntimeDir()` (which
walks up looking for a checked-out `github.com/muthuishere/cljgo` tree)
before ever trying `go/packages`. Third-party modules resolve too, but
need the generated go.mod's requires fetched (`go mod tidy`/`go get`)
**before** the fact-load call — and the existing `build.cljgo` /
go-require pipeline (`pkg/build/build.go`) already does this ordering
correctly. The gap is narrower than ADR 0028 implied: only the *no
go-require, stdlib/runtime-only* interop path is actually broken.

## What was tried

A standalone prototype (`prototype/main.go`, own go.mod, zero dependency
on the cljgo module) replicates `loadHostFacts`'s exact
`packages.Config{Mode: NeedName|NeedTypes, Dir: dir}` call and signature
extraction (params/results/variadic/trailing-error/trailing-bool), so it
can be pointed at any directory to prove or disprove resolution with no
cljgo tree anywhere on the walk-up path. `run.sh` drives four cases with
a throwaway `GOMODCACHE`/`GOPATH` (never the machine's real cache):

1. **Stdlib, fresh release-pin go.mod, no go.sum yet** — the exact state
   right after `SynthGoMod` writes go.mod for a release binary, before
   any `go mod tidy`.
2. **Stdlib, after `go mod tidy`** (cold-cache-reused, then warm).
3. **Third-party (`github.com/google/uuid`), fresh go.mod requiring it,
   after `go mod tidy` on a cold cache** (cold-cache-reused, then warm).
4. **A completely empty directory, no go.mod whatsoever** — the literal
   state `EmitMain` runs in today before `SynthGoMod` has written
   anything (single-file `cljgo build hello.clj`, no build.cljgo).

Negative control: the same empty directory asked to resolve
`github.com/google/uuid` (not requestable without a module context)
fails with Go's own clear diagnostic — proving case 4's stdlib success
isn't a silent no-op.

## Measured

| Case | Result | Latency |
|---|---|---|
| Stdlib, release-pin go.mod, **no go.sum** | resolves `strings.TrimSpace`, `net/http.Get` | 86 ms |
| Stdlib, after `go mod tidy` (cold cache reused) | same | 74 ms |
| Stdlib, after `go mod tidy` (warm) | same | 75 ms |
| Stdlib, **completely empty dir, no go.mod** | resolves both | 48 ms |
| Stdlib, today's baseline (real runtime-tree dir) | resolves both | 80 ms |
| Third-party `go mod tidy` (cold, uncached module) | fetches `uuid` | 383 ms (network) |
| Third-party fact load, cold cache reused | resolves `uuid.New` | 148 ms |
| Third-party fact load, warm | same | 61 ms |
| Third-party import in empty dir (negative control) | **fails**: `no required module provides package …: go.mod file not found` | 83 ms |

Stdlib resolution costs the same ~50–90ms regardless of whether the
directory has a go.mod, a go.mod-without-go.sum, or is today's runtime
tree — **no measurable latency delta from the fix**. Third-party
resolution is fast once fetched (cold-cache-reused: 148ms; warm: 61ms);
the real cost is the one-time `go mod tidy`/proxy fetch, already
accounted for in ADR 0028's S12 numbers (44.8s global one-time backfill,
0.6–3.7s steady-state cold).

## Why stdlib needs no module context

`go/packages` resolves stdlib import paths from `GOROOT` unconditionally
— they aren't part of any module's dependency graph, so the module
graph (go.mod/go.sum) is never consulted for them. Third-party paths
*are* graph members: `go/packages` needs a `go.mod` (with the module
required) and its `go.sum` entries populated, which only exists after a
`go get`/`go mod tidy` against that go.mod.

## Ordering: what already works, what's actually broken

`pkg/build/build.go`'s `buildArtifact` (the `build.cljgo` / go-require
path, ADR 0021 B2) **already gets the ordering right**: when
`p.GoRequires` is non-empty it calls `SynthGoMod` (writing go.mod with
the pins) then `goGet` (materializing go.sum) **before** `WriteModule`
(which is what calls `EmitMain`/`loadHostFacts`), and explicitly sets
`opts.HostFactsDir = genDir`. Third-party interop through `build.cljgo`
already resolves correctly, with no local cljgo tree needed, on main
today.

The actual bug is narrower:

1. **`pkg/emit/compile.go`'s `Build`** (the single-file `cljgo build
   hello.clj` path, `cmd/cljgo/main.go:169`) creates `genDir` but never
   sets `opts.HostFactsDir = genDir` before calling `WriteModule`. So
   any interop program built this way — even `(require-go '[strings])`
   with zero third-party deps — falls through to
   `opts.RuntimeDir`/`FindRuntimeDir()` and dies with "cannot locate the
   … source tree" on a downloaded binary.
2. **`pkg/build/build.go`'s `buildArtifact`** only sets
   `opts.HostFactsDir = genDir` inside `if len(p.GoRequires) > 0`. A
   `build.cljgo` artifact that does stdlib-only interop (no
   `go-require` calls) skips that branch and hits the same
   `FindRuntimeDir()` wall.
3. **`EmitMain`'s fallback chain itself** (`program.go` lines 82–94)
   still treats `FindRuntimeDir()` as the last resort. Once (1) and (2)
   always pass a real `genDir`, this call becomes dead for the
   release-binary path — but it must stay for the two paths that still
   want an explicit runtime tree: `-runtime`/`CLJGO_SRC` overrides and
   the conformance/dev harness building the runtime itself.

## Recommendation for ADR 0033

1. Both `Build` (compile.go) and `buildArtifact` (build.go) always pass
   `opts.HostFactsDir = genDir` (the just-created/about-to-be-written
   generated module dir), unconditionally — not gated on `GoRequires`.
   `genDir` need not contain a go.mod yet for a stdlib-only program:
   `go/packages` resolves stdlib regardless (measured above).
2. No change needed to `loadHostFacts` or its `packages.Config` — the
   mechanism already does the right thing once pointed at a directory
   instead of erroring before it gets the chance.
3. No ordering change needed for the go-require/third-party path — it's
   already correct. Worth a code comment update in `EmitMain` clarifying
   that `HostFactsDir` is expected to be the generated dir, always, not
   an occasional override.
4. `FindRuntimeDir()` stays as the fallback for explicit
   `-runtime`/`CLJGO_SRC` overrides and for the conformance harness
   building against the runtime source itself — but after (1)/(2) it is
   never reached for a normal build (release or dev) with no explicit
   override, so its "no source tree" error effectively retires as a
   downloaded-binary blocker.
5. Add a conformance-style test: `cljgo build` a `(require-go
   '[strings])`-only program from a temp dir with `CLJGO_SRC` unset and
   the cwd outside the repo (mirrors the empty-dir case here) — the
   regression this spike would have caught.

## What's NOT covered here (left for the ADR 0033 spec pass)

- Whether `HostFactsDir` should get its own `go.mod` scaffold written
  *before* fact-loading even for the stdlib-only case (harmless — go.mod
  is cheap to write and doesn't change the measured numbers — vs.
  leaving it genuinely go.mod-less until `WriteModule`/`SynthGoMod`
  writes the real one). Recommend: write it regardless, for uniformity
  and because a later interop reference in the same program might need
  the module context anyway if a future change adds non-stdlib
  detection ahead of `go-require` wiring.
- Multi-module workspace / `GOWORK` interactions — untested; out of
  scope (no cljgo user surface exercises `go.work` today).
