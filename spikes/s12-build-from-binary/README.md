# S12 — can `cljgo build` work from a downloaded binary?

Spike under ADR 0027. Feeds ADR 0028.

## Question

Today `cljgo build` synthesizes a generated module whose go.mod says
`require github.com/muthuishere/cljgo v0.0.0` + `replace … => <local tree>`
(ADR 0013 v0), so a build needs a full repo checkout (or `CLJGO_SRC`).
That kills the "download one binary and go" story.

**Can the generated go.mod instead say
`require github.com/muthuishere/cljgo v0.1.0` — no replace, no checkout —
and have `go build` succeed in a clean directory, with modules fetched from
the public Go module proxy?**

## Exit criterion (written before any code)

The spike closes when ALL of the following are answered with measured
evidence, in either direction:

1. A hand-written generated module (go.mod with a bare
   `require github.com/muthuishere/cljgo v0.1.0`, main.go copied from real
   `cljgo build --keep-gen` output) either builds to a working binary in a
   directory outside the repo with default `GOFLAGS` and
   `GOPROXY=https://proxy.golang.org`, **or** we have the exact error and a
   root-cause diagnosis.
2. Numbers recorded: cold-cache `go mod download` time + downloaded size;
   warm no-replace build time vs today's replace-based build time; binary
   size delta (if any).
3. Blocker inventory: imports of the emitted main.go outside the cljgo
   module; whether `go:embed` of `core/*.clj[g]` survives a proxy fetch;
   whether the repo-root go.mod / `refs/` fence poisons downstream.
4. A version-skew sketch: how the emitter should pin the require
   (pkg/version.Version), and what a dirty/dev binary should do.

Recommendation lands in `VERDICT.md`; decision lands in ADR 0028.

## Layout

- `fixtures/hello.clj` — the program under test.
- `run.sh` — reproduces every measurement (writes into a scratch dir,
  never into the repo).
- Generated module dirs and binaries are gitignored (`.gitignore` here).
