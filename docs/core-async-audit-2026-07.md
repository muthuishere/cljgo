# clojure.core.async audit — 2026-07

**Companion to `docs/fundamentals-audit-2026-07.md`, same method.** That audit
measured cljgo's `clojure.core` (+ friends) surface against the JVM oracle;
this one does the same for `clojure.core.async`, the library ADR 0040 brought
first-class onto Go channels. Unlike the fundamentals audit this change is NOT
read-only: the two genuine A-list gaps (`map`, `thread-call`) are implemented
here with frozen dual-harness conformance tests, the accidental extra (`go*`)
is hidden, and every shared public is confirmed under a conformance test.

## Method

1. **Ground truth** — JVM core.async **1.6.681** on Clojure **1.12.5** (the
   ADR 0040 oracle version; conformance discipline applies to libraries too):

   ```
   clojure -Sdeps '{:deps {org.clojure/core.async {:mvn/version "1.6.681"}}}' \
     -M -e '(require (quote clojure.core.async))
            (doseq [[s v] (sort-by key (ns-publics (quote clojure.core.async)))]
              (let [m (meta v)]
                (println s "|" (boolean (:macro m)) "|"
                         (boolean (:deprecated m)) "|" (pr-str (:arglists m)))))'
   ```

   → **87 publics**, each tagged with its `:macro` / `:deprecated` / `:arglists`
   metadata. That per-var metadata (not memory) is what drives the A/B
   classification below.

2. **cljgo's actual surface** — cljgo now HAS `ns-publics`, so the surface was
   read in cljgo itself (no throwaway Go probe needed, unlike the fundamentals
   audit): `(sort (keys (ns-publics 'clojure.core.async)))` after
   `(require '[clojure.core.async :as a])`. This is the live interned registry.

3. **Diff** — `comm -23` / `-13` / `-12` under `LC_ALL=C` (the `-`/`*`/`!`
   collation trap the fundamentals audit hit applies identically here).

4. **Behaviour oracle** — every implemented behaviour (`map`, `thread-call`,
   and the three `!!` aliases the sync pass added) was run against real JVM
   core.async 1.6.681 and the exact output frozen into a
   `conformance/tests/chan-*.clj` file, never asserted from memory.

## Headline numbers

| | count |
|---|---:|
| JVM core.async 1.6.681 publics | **87** |
| cljgo `clojure.core.async` publics (before this change) | 51 |
| cljgo `clojure.core.async` publics (after this change) | **52** |
| shared (before) | 50 |
| shared (after — `+map +thread-call`) | **52** |
| cljgo extras vs JVM (before) | 1 (`go*`) |
| cljgo extras vs JVM (after — `go*` made `^:private`) | **0** |
| JVM publics still absent from cljgo | **35** = 11 deprecated + 3 T3 + 21 internal |

> **Update (T3 landed, 2026-07-22, change `apply/core-async-t3`).** The three
> T3 pipeline publics — `pipeline`, `pipeline-blocking`, `pipeline-async` — are
> now implemented (see the T3 section below), taking cljgo's
> `clojure.core.async` surface to **55** publics, still a strict subset of the
> JVM's 87 with **0** extras. JVM publics still absent = **32** = 11 deprecated
> + 21 internal; every non-deprecated, non-internal JVM public is now present
> (55 = 87 − 11 − 21). This is the FULL practical core.async surface.

Every one of cljgo's 52 publics is now a strict subset of the JVM surface (no
non-Clojure name is advertised), and every JVM public that is absent is absent
for a documented reason (deprecated-upstream, ADR-deferred, or internal
machinery cljgo replaces Go-natively).

## Classification of all 87 JVM publics

### A — fundamental, implemented in this change (2)

Confirmed NOT `:deprecated` in the oracle, real API surface, previously missing.

| var | `:arglists` | behaviour (oracle-frozen) | conformance |
|---|---|---|---|
| `map` | `([f chs] [f chs buf-or-n])` | combine N channels through f: each round takes one value from every input and delivers `(apply f vals)`, closing as soon as ANY input closes; empty `chs` closes immediately. `(map + [c1 c2])` over `[1 2 3]/[10 20 30]` ⇒ `11 22 33`. Interns ONLY in `clojure.core.async` (shadows nothing in `clojure.core` — the precedence principle; JVM does the same via `:refer-clojure :exclude [map …]`). | `chan-map.clj` |
| `thread-call` | `([f])` | run f on a real goroutine, return a channel yielding f's result once then closing (nil result sends nothing). The public fn the `thread` macro is built on — same runtime seam (`lang.Go`) as `go*`. `(thread-call (fn [] (* 6 7)))` ⇒ `42`. | `chan-thread-call.clj` |

Runtime: `map` → `lang.MapChans` (`pkg/lang/chan_pump.go`); `thread-call` →
`lang.Go` (`pkg/lang/chan.go`). Both registered via `areg` in
`pkg/corelib/chan_builtins.go` (NOT `def`), async-ns-only.

**Side-effect the `map` intern forced (fixed here):** interning
`clojure.core.async/map` shadows the `clojure.core/map` refer *inside*
`async.cljg`, exactly as the existing `reduce` intern already does. The `alt!`
macro helper `-do-alt` used a bare `map` for its seq work; it is now qualified
to `clojure.core/map` (mirroring the pre-existing `clojure.core/reduce` note in
that file). Without this the whole `alt!`/`alt!!` surface breaks in the
compiled harness — caught by `chan-alt.clj` under `TestConformanceCompiled`.

### B — deprecated in core.async itself; skipped by design (11)

All carry `:deprecated true` in the 1.6.681 oracle. core.async deprecated the
"arrow" transformers and the eager partition/unique combinators in favour of
**transducers on `chan`** (`(chan n (map f))`, `(chan n (filter p))`, …), which
cljgo already supports (`chan-xform*.clj`). The precedence principle does not
require porting vars the upstream library itself tells users to stop calling.

| var | `:arglists` | superseded by |
|---|---|---|
| `map<` | `([f ch])` | `(map f)` transducer on chan |
| `map>` | `([f ch])` | `(map f)` transducer on chan |
| `filter<` | `([p ch] [p ch buf-or-n])` | `(filter p)` transducer |
| `filter>` | `([p ch])` | `(filter p)` transducer |
| `remove<` | `([p ch] [p ch buf-or-n])` | `(remove p)` transducer |
| `remove>` | `([p ch])` | `(remove p)` transducer |
| `mapcat<` | `([f in] [f in buf-or-n])` | `(mapcat f)` transducer |
| `mapcat>` | `([f out] [f out buf-or-n])` | `(mapcat f)` transducer |
| `partition` | `([n ch] [n ch buf-or-n])` | `(partition-all n)` transducer |
| `partition-by` | `([f ch] [f ch buf-or-n])` | `(partition-by f)` transducer |
| `unique` | `([ch] [ch buf-or-n])` | `(dedupe)` transducer |

**Recommendation: skip.** Adding them would introduce non-idiomatic,
upstream-discouraged surface. If a real portability need surfaces, oracle +
implement individually, but the default is skip-with-note.

> Note — two *other* `:deprecated` vars, `onto-chan` and `to-chan`, ARE already
> implemented in cljgo (they are in the shared 52). They were shipped alongside
> their `!`/`!!` replacements for the T2 pump family as back-compat aliases;
> they are harmless and stay. This audit does not remove already-shipped
> deprecated aliases, only declines to add new ones.

### T3 — DONE (3, implemented 2026-07-22, change `apply/core-async-t3`)

ADR 0040 #9 tiered the surface T1 (core) → T2 (pumps) → **T3 (pipelines)**;
the pipeline tier was the last deferred stage and is now implemented. Not
`:deprecated`; all three present with full arities.

| var | `:arglists` | behaviour (oracle-frozen) | conformance |
|---|---|---|---|
| `pipeline` | `([n to xf from] [+ close?] [+ ex-handler])` | read `from`, transform each value through the transducer `xf` with parallelism `n`, write results to `to` **in input order**, close `to` when `from` drains unless `close?=false`; returns a completion channel that closes when done. Per-input transducer (fresh `(chan 1 xf ex-handler)` per value), so stateful xf does not accumulate across inputs. `ex-handler` (6th arg) replaces a thrown value; nil / default drops it. | `chan-pipeline`, `chan-pipeline-order`, `chan-pipeline-close`, `chan-pipeline-xform`, `chan-pipeline-ex-handler` |
| `pipeline-blocking` | `([n to xf from] [+ close?] [+ ex-handler])` | identical to `pipeline` on the Go host — the JVM's compute-vs-blocking executor split collapses to goroutines (ADR 0040 #9), documented as observable equality. | `chan-pipeline-blocking` |
| `pipeline-async` | `([n to af from] [+ close?])` | like `pipeline` but the async fn `af = (fn [val result-ch])` delivers 0+ results to `result-ch` and closes it; multi-emit and zero-emit per input, results in input order. | `chan-pipeline-async` |

Runtime: `lang.Pipeline` / `lang.PipelineAsync` (`pkg/lang/chan_pump.go`),
built on a dispatcher → n workers → single ordered writer over the T1
primitives. Registered via `areg` in `pkg/corelib/chan_builtins.go` (NOT
`def`), async-ns-only. `n` must be positive (oracle `n0 =>` AssertionError).
Every behaviour frozen against JVM core.async 1.6.681.

### C — internal machinery / protocols / IOC transform; correctly absent (21)

None of these is application API; all exist on the JVM only to support the
**IOC `go`-macro state-machine transform** or the **protocol dispatch layer**
that cljgo deliberately does not have (ADR 0040: goroutines replace the IOC
transform; the mult/mix/pub surface is implemented Go-natively over concrete
struct types, not via exposed protocols).

**Protocols (4):** `Mult`, `Mix`, `Pub`, `Mux` — cljgo backs these with the
concrete Go types `*lang.Mult` / `*lang.Mix` / `*lang.Pub` (`pkg/lang/`), never
exposing a protocol object.

**Protocol methods (12):** `admix*`, `muxch*`, `solo-mode*`, `sub*`, `tap*`,
`toggle*`, `unmix*`, `unmix-all*`, `unsub*`, `unsub-all*`, `untap*`,
`untap-all*` — the `*`-suffixed methods a user would only call to extend the
protocols; cljgo's public `admix`/`tap`/`sub`/… call into Go directly.

**IOC / macro internals (5):** `do-alt`, `do-alts`, `fn-handler`, `ioc-alts!`,
`defblockingop` — the `alt!` expander internals, the parking-handler
constructor, the IOC-aware alts, and the `>!!`/`<!!` blocking-op-defining
macro. cljgo's `alt!`/`alt!!` are self-contained in `async.cljg` (over the
public `alts!`), and there is no IOC state machine for `ioc-alts!`/`fn-handler`
to feed.

**Confirmed working despite the protocols being absent** (each has a frozen
dual-harness test): `mult`/`tap`/`untap`/`untap-all` (`chan-mult`, `chan-untap`,
`chan-untap-all`), `mix`/`admix`/`unmix`/`unmix-all`/`toggle`/`solo-mode`
(`chan-mix*`), `pub`/`sub`/`unsub`/`unsub-all` (`chan-pub-sub`, `chan-unsub`,
`chan-unsub-all`). The architectural difference — **goroutines over concrete
types, not the JVM's IOC state-machine `go` transform + protocol layer** — is
intentional (ADR 0040) and observably behaviour-preserving.

### Already-implemented shared surface (50 before / 52 after)

The T1 (core) + T2 (pump) surface ADR 0040 shipped, all present and
conformance-covered (see the sync table below):
`<!` `<!!` `>!` `>!!` `alts!` `alts!!` `alt!` `alt!!` `chan` `close!` `buffer`
`dropping-buffer` `sliding-buffer` `unblocking-buffer?` `go` `go-loop` `thread`
`timeout` `offer!` `poll!` `put!` `take!` `promise-chan` `pipe` `merge` `split`
`into` `reduce` `transduce` `take` `onto-chan` `onto-chan!` `onto-chan!!`
`to-chan` `to-chan!` `to-chan!!` `mult` `tap` `untap` `untap-all` `mix` `admix`
`unmix` `unmix-all` `toggle` `solo-mode` `pub` `sub` `unsub` `unsub-all` — plus
`map` and `thread-call` added here.

## The `go*` extra — resolved to `^:private`

cljgo shipped one public var with **no JVM counterpart**: `go*`, the runtime
seam the `go`/`thread` macros expand to (`(go body…)` ⇒
`(clojure.core.async/go* (fn* [] body…))`). Real core.async has no public `go*`
— its `go` is the IOC-transform macro cljgo deletes.

**Decision: made `^:private`** (`pkg/corelib/chan_builtins.go`, `.SetPrivate()`
on the `areg` result). Investigated exactly as the task required — *does hiding
it break the macros that emit a qualified reference to it?* **It does not:**

- The `go`/`thread` macros emit a **fully-qualified** `clojure.core.async/go*`,
  which resolves fine to a private var in **both** the interpreter and an AOT
  binary (`rt.Boot` re-runs `RegisterAll`, so the `:private` meta is live in
  compiled code too — the same `SetPrivate`/`hoistVar` mechanism the
  fundamentals audit added for compiled `ns-publics`).
- The full `chan-*` conformance set — `go`, `thread`, `go-loop`, `alt!`,
  `alt!!` — stays green in **both** harnesses with `go*` private (verified).
- `go*` now leaves `(ns-publics 'clojure.core.async)`, matching the JVM surface
  (no `go*`), taking cljgo's extras-vs-JVM count from 1 to **0**.

The M4-v0 `clojure.core` refer of `go*` (`asyncCoreAliases`) is kept for
back-compat — bare `go*` in old user code still resolves — but the canonical
namespace no longer advertises it. No working macro was destabilised for
surface cosmetics; the change is strictly a hide of an internal seam that the
tests prove is reachable by qualified name regardless.

## Sync confirmation — every shared public under a frozen dual-harness test

The sync pass grepped all 52 shared publics against `conformance/tests/chan-*.clj`.
**49 of 52 were already directly exercised.** Three were reached only through
their non-`!!` siblings and got a dedicated frozen test here:

| alias | sibling it aliases | new coverage |
|---|---|---|
| `alts!!` | `alts!` | `chan-blocking-aliases.clj` |
| `to-chan!!` | `to-chan!` | `chan-blocking-aliases.clj` |
| `onto-chan!!` | `onto-chan!` | `chan-blocking-aliases.clj` |

After this change **all 52 shared publics have direct dual-harness conformance
coverage** (interpreted AND AOT-compiled, per the release-blocker discipline).
The only non-deferred publics without coverage would be the B-deprecated set,
which is skip-by-design (not implemented, so nothing to cover), and the C-internal
set (not public API). Every T3 var is deferred by ADR.

## Appendix — raw data

Oracle metadata dump (87 rows, `name | macro | deprecated | arglists`) and the
cljgo `ns-publics` dump (52 rows) were computed with the commands in §Method;
throwaway scratch files, not checked in. Counts reconcile as
**50 shared(before) + 2 A + 11 B-deprecated + 3 T3 + 21 C-internal = 87**, with
cljgo's post-change surface = 50 + 2 = **52**, extras vs JVM = **0**. After T3
landed (2026-07-22) the 3 T3 vars join the shared set: **52 + 3 = 55**
publics, absent = 11 deprecated + 21 internal = **32**, extras still **0**.
