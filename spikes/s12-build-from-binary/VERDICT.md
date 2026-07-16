# S12 verdict — build from a downloaded binary

**Answer: YES.** A generated module with a bare
`require github.com/muthuishere/cljgo v0.1.0` — no `replace`, no repo
checkout, no `CLJGO_SRC` — builds and runs correctly in a clean directory
outside the repo, with all source fetched from the public Go module proxy.
No change to the emitted code is needed; the ONLY change is what
`SynthGoMod` writes into go.mod.

## What was tried

1. Built the cljgo CLI from this tree (`cljgo version` → 0.1.0), ran
   `cljgo build -gen … fixtures/hello.clj` to get a real emitted
   `main.go` + today's replace-based go.mod.
2. Copied that `main.go` into a fresh directory outside the repo with a
   hand-written go.mod: `require github.com/muthuishere/cljgo v0.1.0`,
   no replace. Fresh `GOMODCACHE`, `GOPROXY=https://proxy.golang.org`.
3. `go mod download` → `go mod tidy` → `go build` → ran the binary.
   Output identical to the replace-based build (`hello …` / `55`).
4. Probed the skew edges: an unpublished tag, and an untagged pushed
   commit.

Everything is reproducible via `./run.sh` (host: macOS arm64, Go 1.26.3,
2026-07-16).

## Measured

| Metric | Value |
|---|---|
| Proxy fetch of v0.1.0, first-ever (proxy backfill from GitHub) | 44.8 s — one-time cost per version, global, paid by whoever asks first |
| Proxy fetch, steady-state cold local cache | **0.6–3.7 s** across runs |
| Module zip / extracted size in GOMODCACHE | **832 KB zip / 3.9 MB extracted** |
| `go mod tidy` after download | 0.04 s |
| Cold-GOCACHE build, no-replace | 2.4 s |
| Cold-GOCACHE build, replace-based (same flags) | 2.1 s |
| Warm rebuild after touching main.go | 0.16 s |
| Stripped binary (`-trimpath -ldflags "-s -w"`), v0.1.0 via proxy | 4.98 MB |
| Stripped binary, HEAD via replace | 5.10 MB |

Build-time and binary-size deltas are noise / source drift between v0.1.0
and HEAD — the fetch mechanism itself costs nothing after the one-time
download.

## Blocker inventory — all clear

- **Imports**: the emitted main.go imports exactly two packages, both
  module-internal: `pkg/emit/rt` and `pkg/lang`. Their transitive deps
  are 100% stdlib. `golang.org/x/tools` (repo go.mod) is only imported by
  compiler-side packages, so Go module-graph pruning keeps it out of the
  downstream go.sum entirely — the generated module's go.sum is 2 lines.
- **`go:embed` of `core/*.clj[g]`**: works from the proxy zip. The test
  program exercises `println`/`reduce`/`map`/`inc`/`range`, all defined in
  the embedded `core.clj` — correct output proves clojure.core loaded from
  the fetched module.
- **`refs/` fence**: `refs/` is gitignored, so it is not in the tag and
  not in the module zip; its stub go.mod cannot poison anything. (Nested
  modules are excluded from zips anyway.)
- **Zip hygiene (non-blocker)**: the zip carries `design/`, `docs/`,
  `conformance/`, `spikes/`, `site/` — harmless at 832 KB, but a future
  runtime-only module split would shrink it further if we ever care.

## Version skew

- The binary that emits code and the module version go.mod requires must
  expose the same rt/lang API. Pinning the require to the emitting
  binary's own `pkg/version.Version` makes that true **by construction**
  for release binaries: the v0.1.0 binary emits code compiled against the
  v0.1.0 module.
- Unknown/unpublished tag fails loudly and clearly:
  `404 … unknown revision v0.9.9` — no silent wrong-version build.
- An untagged **pushed** commit is still addressable as a pseudo-version
  (`go list -m …@main` → `v0.1.1-0.20260716…-64fac871314a`), so even a
  commit-built binary could pin exactly — but a **dirty** tree has no
  address at all; dev builds must keep today's replace/CLJGO_SRC path.
- Gap found: `pkg/version.Version` defaults to `"0.1.0"` in source, so a
  source-built dev binary is indistinguishable from the release binary.
  ADR 0028 needs a dev marker (default `-dev` suffix in source, release
  builds override via the existing `-ldflags -X` hook).

## Recommendation for ADR 0028

`SynthGoMod` picks the go.mod shape by mode:

1. **Release binary** (Version is a plain tag): write
   `require github.com/muthuishere/cljgo v<Version>`, **no replace**.
   This becomes the default for downloaded binaries — the adoption story
   is "download cljgo, `cljgo build hello.clj`, done" (first build pays
   ~4 s of module download, once per machine).
2. **Dev/dirty binary** (Version carries the dev marker) **or** explicit
   `-runtime`/`CLJGO_SRC`: today's `replace` path, unchanged. Precedence:
   `-runtime` flag > `CLJGO_SRC` > release-pin > walk-up repo detection.
3. Change `pkg/version.Version`'s in-source default to `0.1.0-dev` (or
   next-version`-dev`) and set the real tag in release ldflags (goreleaser
   already has the hook — see version.go's own comment).
4. go.mod stays user-owned once written (unchanged); the failure mode for
   a hand-edited bad version is Go's own clear 404.
