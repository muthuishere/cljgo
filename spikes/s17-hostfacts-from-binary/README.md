# S17 — Go-interop `cljgo build` from a downloaded binary

Spike under ADR 0027. Feeds ADR 0033. Closes the flagged gap in ADR 0028
("plain `cljgo build` works from a downloaded binary; interop programs
don't").

## Question

`loadHostFacts` (pkg/emit/hostfacts.go, called from pkg/emit/program.go
`EmitMain`) resolves Go type facts for `(require-go '[pkg])` via
`golang.org/x/tools/go/packages`, pointed (`cfg.Dir`) at a module
directory — today that's `opts.HostFactsDir`, falling back to
`opts.RuntimeDir`, falling back to `FindRuntimeDir()`, which walks up
looking for a checked-out `github.com/muthuishere/cljgo` source tree. A
user with only the downloaded binary + Go toolchain has no such tree —
`FindRuntimeDir()` fails loudly, so any interop program (even one that
only touches `strings`) can't build.

**What should host-facts resolution use when there is no local cljgo
tree, and does it work for third-party modules too (the build.cljgo
`go-require` path, ADR 0021 B2)?**

## Exit criterion (written before any code)

The spike closes when EITHER of these holds, with measured evidence:

1. A prototype resolves Go type facts (signature: params/results/variadic/
   trailing-error-or-bool) for (a) a stdlib package (`strings`, `net/http`)
   AND (b) one real third-party module — in a clean temp directory that
   has **no cljgo source tree anywhere on the walk-up path** (simulated by
   running outside the repo, with `CLJGO_SRC` unset and no ancestor
   `github.com/muthuishere/cljgo` go.mod) — using only what a downloaded
   binary already has to write: the generated module's own go.mod.
2. OR: a diagnosed impossibility — the specific reason go/packages cannot
   resolve one of the two cases without a local runtime tree, blocking
   the ADR-0028 story for interop.

Also record:

- Added latency: go/packages against a **fresh** module cache (cold) vs
  the existing runtime-tree path (today's baseline) vs a **warm** cache
  (second run) — is this noise, or a real per-build cost?
- What ordering change (if any) `pkg/emit/program.go`'s `WriteModule` /
  `EmitMain` needs: today `EmitMain` (which calls `loadHostFacts`) runs
  BEFORE `SynthGoMod` writes go.mod, so there is no generated go.mod yet
  when facts are loaded — candidate A requires flipping that order.

## Layout

- `prototype/` — standalone Go program (own go.mod, NOT part of the
  cljgo build) that drives `go/packages` exactly like `hostfacts.go`
  does, against a synthesized module dir it builds itself.
- `run.sh` — reproduces every measurement into a scratch dir (never the
  repo), with `HOME`/`GOPATH`/`GOMODCACHE` pointed at a throwaway cache
  to force cold-cache numbers, then rerun warm.
- `VERDICT.md` — recommendation for ADR 0033.

Nothing here changes `pkg/emit`; this only informs a future ADR.
