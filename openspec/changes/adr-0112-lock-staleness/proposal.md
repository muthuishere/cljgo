# adr-0112-lock-staleness — the lock refreshes itself, minimally, unless frozen

## Why

Bump a dependency version in `build.cljgo` and `cljgo build` stops with a note
telling you to `run resolve with -update`. **There is no `resolve` subcommand
and no `-update` flag.** The only remedy a user can find is
`rm build.lock.edn`, which also discards every unrelated pin. koine hit this
twice (koine#166).

It is not a policy that needs deciding — it is a mechanism that was never
finished. `pkg/deps/lock.go:96` declares `BuildHash`; three sites read, write
and carry it across a re-resolve, and **nothing compares it**. So
`pkg/build/build.go:458`'s `update := lock == nil` is the half that was left
out.

Spike **s70** (`spikes/s70-lock-policy/RESULTS.md`) measured the cost of
finishing it across 30–500 coordinates in four graph shapes, including shared
DAGs: full re-resolve **0.80–1.04 ms per distinct coordinate**, warm **0.093
ms**, both linear, and cost tracks distinct coordinates rather than edges
(8,000 edges over 180 coordinates costs the same per coordinate as 30 over
30). Nothing superlinear to design around; the simple policy is affordable.

## What Changes

- `resolveDeps` compares a hash of the **normalised declared set** against the
  lock's `BuildHash` and re-resolves when it moves.
- The re-resolve is **minimal**: unchanged declarations keep their pins.
- `cljgo build --locked` (and `CLJGO_LOCKED=1`) turns a stale lock into an
  error instead of a refresh — the CI/merge case, distinct from `Offline`.
- The note naming a command that does not exist is deleted.

## Impact

- **Affected specs:** `dependency-resolution`
- **Affected code:** `pkg/deps/lock.go`, `pkg/deps/resolve.go`,
  `pkg/build/build.go`, `cmd/cljgo/main.go`, `pkg/diag/registry.go`
- **Not affected:** the emitter, the load path, both legs' resolution order.
  A Maven coordinate is still one shape on the one resolver (ADR 0095).

## Non-goals

- Concurrent resolution. s70 measured that resolution exploits none, and
  deliberately declines to add a scheduler: real graphs are wide and shallow,
  and speeding up a shape nobody has is the trade *simplicity first, then
  performance* forbids.
- A `cljgo deps` / `cljgo resolve` verb. This change removes the reason anyone
  wanted one.
