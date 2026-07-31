# s70 — lock staleness policy: results

**Question.** `cljgo build` refuses when `build.cljgo` declares a dependency
`build.lock.edn` does not pin, and tells you to run a command that does not
exist. The only remedy is `rm build.lock.edn`. What should it do instead, and
what does the fix COST as a dependency graph grows?

**Measured 2026-07-31**, darwin/arm64, Apple M5 Pro, go1.26.
Harness: `pkg/deps/mvnscale_test.go`, against the same httptest Maven double
the correctness tests use — production code path, no network.

```
go test ./pkg/deps/ -run xxx -bench BenchmarkResolve -benchtime 5x
```

## The finding that made this a patch and not a design debate

`pkg/deps/lock.go:96` declares `BuildHash`. Line 125 reads it, line 248 writes
it, `resolve.go:558` carries it forward across a re-resolve. **Nothing ever
compares it.** It is a dead field whose only possible purpose is detecting
that the manifest moved out from under the lock.

So `build.go:458`'s `update := lock == nil` is not a policy choice anyone
made. It is an unfinished mechanism.

## Scale

`width` independent root libraries, each heading a `depth`-long transitive
chain; every library is one pure-Clojure namespace, so classification runs for
real. **Full** = cold cache, whole graph re-resolved (what `rm build.lock.edn`
costs today, and what a naive "re-resolve on any change" would cost on every
manifest edit). **Warm** = lock pins everything, cache hot (the "nothing
changed" path every build already pays).

| graph | coords | full re-resolve | warm | ratio |
|---|---|---|---|---|
| 10 × 3 | 30 | 23.96 ms | 2.87 ms | 8.4× |
| 50 × 3 | 150 | 119.36 ms | 14.03 ms | 8.5× |
| 100 × 3 | 300 | 247.47 ms | 27.67 ms | 8.9× |
| 50 × 6 | 300 | 264.77 ms | 29.12 ms | 9.1× |

**Both legs are linear in coordinate count** — ~0.82 ms/coord full, ~0.093
ms/coord warm — and shape barely matters: 300 coordinates cost the same
whether they are 100 wide × 3 deep or 50 wide × 6 deep (247 vs 265 ms). There
is no superlinear blowup to design around, which is the result that makes the
simple policy affordable.

Minimal re-resolve is bracketed by these two numbers by construction: its cost
is the warm path for everything unchanged, plus the full path for the changed
roots and their transitives. At the 300-coordinate scale, changing one root of
a 3-deep chain is ~28 ms + ~2.5 ms, against ~247 ms for a full re-resolve —
**roughly 8× cheaper**, and the multiple grows with the size of the graph you
did not touch.

## The caveat that makes minimal re-resolve matter MORE than this table shows

Every number above is against a local `httptest` server. This measures the
resolver's own work — POM parse, BFS walk, zip extraction, namespace
classification — and **excludes network latency entirely**. Against real
Clojars a cold coordinate fetch is one to two orders of magnitude more than
the ~0.8 ms of local work per coordinate.

So do not read "247 ms" as the user-visible cost of a full re-resolve. Read it
as the floor. The real-world gap between full and minimal is wider than 8×,
not narrower — the strategy that avoids re-fetching untouched coordinates is
avoiding the dominant term, not the measured one.

## What this settles

1. **Compare the hash.** The mechanism already exists and is inert.
2. **Hash the normalised declared SET, not the file's bytes.** Over bytes, a
   reformat or an added comment re-resolves the whole graph — a surprise
   upgrade caused by an edit that declared nothing.
3. **Re-resolve minimally.** Keep every pin whose declaration did not move.

   Note *why* this is in, because it would fail the simplicity test if it
   were here on the strength of the 8×: full re-resolve on a one-line version
   bump silently drifts every transitive to whatever is newest today, which
   is **the lockfile failing at its one job**. It is a correctness
   requirement that happens also to be faster — the free kind of
   optimisation, and the ADR must say so rather than let a reader mistake it
   for perf-driven machinery.

   It is also a set difference, not an engine: pins whose declaration is
   unchanged are kept, the rest are resolved. No strategy object, no
   pluggable diff, nothing to configure.
4. **A frozen mode is required, and it is not `Offline`.** Merges take
   `build.cljgo` from one branch and `build.lock.edn` from another. Today that
   stops. Under auto-refresh it would silently resolve a graph nobody reviewed
   and build green. Online-but-lock-is-authority is a distinct position and
   needs its own flag (`npm ci` / `cargo --locked`).

Credit: 2, 3 and 4 are koine's, from the design exchange on 2026-07-31; each
is a case the original proposal got wrong.

→ **ADR 0112.**
