# ADR 0112 — lock staleness: compare the hash, re-resolve minimally, freeze on demand

Date: 2026-07-31 · Status: **accepted — implemented 2026-07-31** — spike **s70** CLOSED
(`spikes/s70-lock-policy/RESULTS.md`, measured and stress-tested).

Supersedes nothing. Completes the mechanism ADR 0052 started and ADR 0095
extended to Maven coordinates.

## Context

Bump a dependency's version in `build.cljgo` and `cljgo build` stops:

```
error: build.cljgo declares medley/medley 1.5.0 but build.lock.edn pins 1.4.0
note: run resolve with -update to re-pin
```

There is no `resolve` subcommand and no `-update` flag. The note names a
command that has never existed, so the only remedy a user can find is
`rm build.lock.edn` — which is also the *worst* remedy, because it discards
every unrelated pin at the same time. koine hit this twice and filed it as a
design call (koine#166).

**It is not a design call. It is an unfinished mechanism.**
`pkg/deps/lock.go:96` declares `BuildHash`; line 125 reads it, line 248 writes
it, `resolve.go:558` carries it forward across a re-resolve — and **nothing
ever compares it**. It is a dead field whose only possible purpose is
detecting that the manifest moved out from under the lock. So
`pkg/build/build.go:458`'s

```go
update := lock == nil // absent lock → generate it
```

is not a policy anyone chose; it is the half of the feature that was never
finished.

### What s70 measured

Against the httptest Maven double the correctness tests already use
(production code path, no network), across 30–500 coordinates in four graph
shapes — disjoint chains, deep chains, and shared DAGs:

| leg | cost |
|---|---|
| full re-resolve, cold cache | **0.80–1.04 ms per distinct coordinate** |
| warm, everything pinned | **0.093 ms per coordinate** |

Both **linear**, and — the result that matters for real graphs — cost tracks
**distinct coordinates, not edges**: a diamond of 8,000 edges over 180
coordinates costs the same per coordinate as 30 coordinates over 30 edges.
First-wins dedup is doing its job, so the simple policy is affordable on the
shapes people actually ship, not merely on the convenient one.

Two honest limits on those numbers, both recorded in the spike:

- They **exclude network entirely**. A cold coordinate fetch against real
  Clojars is one to two orders of magnitude above 0.8 ms. So the measured
  8–9× gap between full and minimal re-resolve is a **floor**; in the real
  world minimal avoids the dominant term, not the measured one.
- Resolution exploits **no concurrency** — a 500-deep chain costs what 20
  parallel chains of 25 cost. Deliberately not acted on: real graphs are wide
  and shallow, and a scheduler for a shape nobody has is the trade
  *simplicity first, then performance* forbids.

## Decision

**1. Compare the hash.** `resolveDeps` sets

```go
update := lock == nil || lock.BuildHash != declaredSetHash(plan)
```

The field already exists and is already written and carried. This finishes it.

**2. Hash the NORMALISED DECLARED SET, not the file's bytes.** The hash covers
the sorted set of `(name, version-or-ref, kind)` triples the manifest
declares, and nothing else. Over raw bytes, adding a comment or reformatting
`build.cljgo` — or renaming the `exe` artifact — would re-resolve the whole
graph: a surprise upgrade caused by an edit that declared nothing.

**3. Re-resolve MINIMALLY.** Pins whose declaration is unchanged are kept;
changed roots and everything reachable only from them are re-resolved.

This is in the decision for a **correctness** reason, not for the 8×, and the
distinction matters: a full re-resolve on a one-line version bump silently
drifts every *other* transitive to whatever is newest today, which is the
lockfile failing at its one job. It is the free kind of optimisation —
correctness wanted it anyway — and it is a **set difference, not an engine**.
No strategy object, no pluggable diff, nothing to configure.

**4. A frozen mode, and it is NOT `Offline`.** `cljgo build --locked` (and
`CLJGO_LOCKED=1`) makes a stale lock an **error** instead of a refresh.

Auto-refresh is right for a developer who just edited the manifest and wrong
for CI: a merge takes `build.cljgo` from one branch and `build.lock.edn` from
another, or someone edits the manifest and forgets to commit the regenerated
lock. Today that repo stops. Under decision 1 alone it would silently resolve
a graph nobody reviewed and build green. `Offline` does not cover this — you
can be online and still want the lock to be the authority. This is `npm ci` /
`cargo --locked`.

**5. Delete the note that names a nonexistent command.** With 1–4 there is no
command to name: an ordinary build re-pins, and `--locked` explains itself.

## Consequences

- **`cljgo build` stops having a dead end.** The failure that cost koine two
  interruptions is gone, and the remedy that discards unrelated pins is no
  longer the only path.
- **The lock keeps meaning what it says.** A version bump moves what you
  bumped. Nothing else moves.
- **CI gains a real guarantee** it did not have, in one flag.
- **Cost when nothing changed is unchanged** — the hash compare is a sorted
  set of short strings against a 0.093 ms/coordinate warm path, i.e. lost in
  the noise.
- **A manifest edit now reaches the network** where before it errored. That is
  correct and expected (you edited the dependency list), and `--locked` /
  `Offline` are there for people who mean otherwise.
- Decisions 2, 3 and 4 are **koine's**, from the design exchange of
  2026-07-31. Each is a case this ADR's first draft got wrong: it proposed
  hashing file bytes, said nothing about minimality, and guessed the wrong
  counter-argument for freezing.

## Not in scope

- Concurrent resolution (measured, deliberately declined — see above).
- A `cljgo deps` / `cljgo resolve` subcommand. Decision 1 removes the reason
  anyone wanted one; adding a verb to explain a mechanism that no longer needs
  explaining is the wrong direction.
- Version *ranges* or `dependencyManagement`; still G5011 name-errors
  (ADR 0095).
