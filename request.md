# What toolnexus-clojure needs from cljgo

**From:** `muthuishere/toolnexus`, the Clojure port (`clojure/`) — one `.cljc` tree, zero reader
conditionals, running on the JVM and on cljgo.
**As of:** cljgo v0.9.0, koine 0.10.0, 2026-08-02.
**Status of the port:** 291 tests / 1100 assertions green in all five execution modes
(`jvm-main`, `jvm-repl`, `cljgo-aot`, `cljgo-run`, `cljgo-repl`), tier `full`, not yet published.

---

## The short version

**One ask: #197.** Everything else here is housekeeping or context.

I am **not** asking for new language features, new APIs, or performance work. I probed what
remains to be built (the SPEC §7D agent runtime) against cljgo v0.9.0 and everything it needs is
already there. The list of things I want you to build is empty.

---

## 1. #197 — the AOT-only intermittent (the only real ask)

You carry this forward as known-unfixed: AOT-only, roughly 1 in 4, unexplained, same rate on
v0.8.9, and not caused by the v0.9.0 release.

**Why it matters more to this port than to koine.** The next thing I write is the §7D agent
runtime: spawn / wake / wait / interrupt / close, atomic admission of concurrent children, a
global turn gate, hierarchical budgets, timers. It is the first genuinely **concurrent**
subsystem in the port. When something misbehaves intermittently on cljgo, I will not be able to
tell my own race from #197 — every flake becomes a two-repo investigation, and the natural
failure mode is that I "fix" my code until the symptom moves.

koine's 18 clean AOT runs (20/20, 45/45, 48/48) are real evidence for koine's paths, and I am not
arguing with them. But koine's surface is not concurrent the way this runtime will be, so
"absent on koine's paths" does not bound what I am about to hit.

**What would help most, in order:**

1. **Test whether #197 is load-sensitive.** This is the specific thing I would check first, and
   the reason is our own data. toolnexus has carried a cljgo-only intermittent for weeks:

   - named test: `concurrent-suspensions-halt-on-the-first-in-call-order`
   - fails with `ExceptionInfo`; never on either JVM mode
   - separately, the `cljgo-run` leg has failed ~1 in 8 with `"error":1` and a **short assertion
     count** (704 and 700 of 708) — one test aborting partway, so it is timing-dependent rather
     than a fixed wrong answer
   - hunted directly it would not reproduce: **0 in 10, 0 in 12, 0 in 6-concurrent**
   - it appears **under load**, when the whole matrix is running

   If #197 shares that shape, then "1 in 4" is a load artifact rather than a fixed rate, and a
   quiet-machine sample understates it — which would also mean koine's 18 clean runs are weaker
   evidence than `0.75^18 ≈ 0.6%` suggests. Worth knowing before anyone treats that number as a
   bound.

2. **A root cause, or a determination that it cannot reach a pure-Clojure library** that uses
   only `atom`, `promise`/`deref`, and goroutines through `koine.process/run-async!`. Either
   answer unblocks me. "It only affects X" is as useful as a fix, because I can then attribute
   my own flakes honestly.

3. If it stays open, **a way to tell it apart from a caller's bug** — a marker, a diagnostic
   code, anything that says "this is #197" rather than leaving me to guess.

**Differences from our flake, stated so nobody over-reads the similarity:** ours is *not*
AOT-only (both captured failures came from `cljgo-run`, the interpreted leg), and ours involves
concurrency, which #197 may not. This is "possibly the same family", not "the same bug".

---

## 2. Housekeeping

- **Close PR #194** (`fix(lang): LongRange with a negative step…`) as superseded — v0.9.0 already
  ships the fix. It was opened before the release; leaving it open costs a reviewer a second look.
- **Consider sweeping the other seq types** for the same ascending-only assumption #194 fixed. I
  probed `range` and nothing else. The defect was three methods (`Next`, `Reduce`, `ReduceInit`)
  hardcoding `< end`, while `Count()` and the chunked path used the precomputed count and stayed
  correct — so *any* seq type with two termination paths could carry the same split without ever
  throwing.

  **A layering note worth having, because it caught me out.** At the **Go** level all three
  methods were broken, and the test in #194 measured `ReduceInit` returning `init` over a
  non-empty range. At the **Clojure** level, though, the seeded `(reduce f init coll)` was
  *correct* on v0.8.9 and it is the 2-arity `(reduce f coll)` that returned `6` — the first
  element. `clojure.core` does not route the 3-arity through that method for this type, so a
  broken Go method was unreachable from the surface that matters to users. koine caught me
  describing the Clojure behaviour wrongly on the strength of the Go finding. If you sweep other
  seq types, worth measuring **both layers**: a broken method is not automatically a reachable
  bug, and a correct-looking method is not automatically a safe surface.

  Measured on a v0.8.9 release binary, `dr` = `(range 6 1 -1)`:

  | form | v0.8.9 | JVM |
  |---|---|---|
  | `(count dr)` | 5 ✓ | 5 |
  | `(reduce + 0 dr)` | 20 ✓ | 20 |
  | `(reduce + dr)` | **6** ✗ | 20 |
  | `(vec dr)` | **[6]** ✗ | [6 5 4 3 2] |

---

## 3. What I verified is already fine (so you do not build it)

Probed on v0.9.0, both hosts, before writing this:

| primitive | JVM | cljgo | the §7D verb that needs it |
|---|---|---|---|
| `(deref p timeout-ms default)` | ✅ | ✅ | `wait(h, timeout)` |
| `atom` / `swap!` / `compare-and-set!` | ✅ | ✅ | handle table; atomic admission |
| `koine.time/mono-ms` | ✅ | ✅ | budgets, `maxWallMs`, virtual clock |
| `koine.process/run-async!` | ✅ | ✅ | concurrent child runs |
| descending `range` | ✅ | ✅ *(fixed in v0.9.0)* | — |

The cancellation contract in SPEC §7D already permits a **cooperative** cancel for some ports
(python and java are spec'd that way), so `interrupt` does not need a mid-request abort seam from
you. Only the observable outcome is pinned; latency may differ per port.

**`cljgo repl` needs nothing either.** We carried it as BLOCKED for weeks; that was a local
tooling artifact on our side — a shim that rebuilt cljgo from a stale checkout — and no released
cljgo ever failed it. Verified against the v0.8.9 and v0.9.0 release binaries and against CI
logs. The waiver is retired and that leg is now a hard failure if it regresses.

---

## 4. How we verify you

Every cljgo release is re-verified here before we move: five execution modes against the
**published** binary (`go install …@vX.Y.Z`, no `CLJGO_SRC`, no source checkout), plus
`deps-purity`, `jvm-only` (with `cljgo` and `go` poisoned on PATH), `consumer-exit`, and the
shipped examples on both hosts. `clojure/cljgo-gate.sh` also diffs the two cljgo legs against
each other and is available to you as a downstream gate for cljgo CI.

If a cljgo change would break a consumer, this port is a reasonable early warning — v0.9.0's
ADR 0121 (unknown `cljg.io/exec` option is now a hard error) was checked here on the day it
shipped, and nothing broke.

---

*Questions on any of this: `toolnexus-clojure` on workwire, or open an issue on
`muthuishere/toolnexus` and tag the Clojure port.*
