# ADR 0040 — core.async, first-class, on Go channels

Date: 2026-07-17 · Status: **proposed** (owner reviews before implementation)
Evidence: spike S19 (`spikes/s19-core-async/` — benchmarks, prototypes, JVM
core.async 1.6.681 oracle transcripts) + spike S10 (`reflect.Select` numbers).
Owner mandate (2026-07-17): core.async, first-class, on Go channels.

## Decision table

| # | question | decision | evidence |
|---|---|---|---|
| 1 | channel representation | **one wrapper struct over a real Go chan** (extend M4-v0 `*lang.Channel`), carrying buffer policy + transducer + ex-handler; never a from-scratch mutex/handler channel | wrapper tax 8–16% vs raw Go chan; ManyToManyChannel port costs 2.7–5.6× + 8 allocs/op (S19 Q1) |
| 2 | close! semantics | **done-chan close: the data chan is never `close()`d.** Parked puts survive close and deliver to later takers (JVM parity); no panic/recover shim remains | oracle `parked-put-survives-close => [:v true]`; fidelity costs +28 ns rendezvous, ~1 ns buffered (S19 Q1′) |
| 3 | dynamic `alts!` | **`reflect.Select`**; handler protocol rejected (requires the slow representation); static `alt!` → real `select` stays the AOT performance rung | 101.5 ns vs 132.7 ns at n=2; handler's n=8 win (274 vs 399 ns) can't pay for 2.7–5.6× on every put/take (S19 Q2, S10) |
| 4 | `timeout` | **fresh channel closed by `time.AfterFunc` — match close-after-N semantics, NOT the JVM's per-tick channel cache** | oracle `timeout-identical-same-tick => true` is an implementation artifact; docstring promises only the close (S19 Q3) |
| 5 | park vs block | **`<!`/`>!`/`alts!` legal on any goroutine** — the JVM's "used not in (go ...) block" throw is not mirrored | oracle throw exists only because of the IOC transform; no working JVM program observes the difference (S19 Q4) |
| 6 | namespace | **`clojure.core.async` is canonical** (embedded `core/async.cljg`); existing clojure.core names remain aliases to the same vars; new surface interns only in the async ns | M4-v0 placed primitives in clojure.core pre-ADR-0027; portable code requires the real ns (S19 Q5) |
| 7 | `(chan 0)` | **throws**, matching the JVM assert (breaks from M4-v0's leniency) | oracle `chan-zero => AssertionError "fixed buffers must have size > 0"` |
| 8 | nil-channel ops | **throw** (as M4-v0 already does; message aligned to the JVM's) | oracle refuted the "block forever" lore: 1.6.681 throws IllegalArgumentException |
| 9 | surface | tiered T1 (core) → T2 (plumbing pumps) → T3 (pipelines; `pipeline-blocking` = alias of `pipeline`) per S19 Q6 inventory | real goroutines collapse the thread-pool distinctions |

## Context

The project thesis (design/05 §4): core.async's `go`-macro CPS/IOC transform
exists solely because JVM threads are expensive — on Go, goroutines ARE the
cheap thing it emulates, so the transform is deleted; `go` blocks are real
goroutines and channels are real channels. M4-v0 shipped the primitive slice
(`chan/>!/<!/>!!/<!!/close!/go/thread/timeout/alts!/alts!!/dropping-buffer/
sliding-buffer`) into clojure.core. The rest of core.async — transducer
channels, ex-handlers, `alt!`, `put!/take!/offer!/poll!`, `promise-chan`,
`go-loop`, mult/pub/mix/pipeline — had genuine open design questions, which
spike S19 settled with measurements and a JVM oracle (core.async 1.6.681 on
Clojure 1.12.5, per the conformance discipline: the oracle applies to
libraries too).

Constraint (design/05 §1): interop is the product. A Go `chan T` received
from any Go API must work with `<!`/`>!`/`alts!` (S19/S10 verify this via
reflect, including closed-`chan int` → nil normalization), and a cljgo
channel exposes its backing Go chan (receive-only) at the interop edge.

## Decision detail

1. **Representation.** `*lang.Channel` stays the ONE channel type: a struct
   over `chan any` + `done chan struct{}` + closed flag + buffer policy +
   optional transducer step (serialized by a put-side mutex, as core.async
   serializes xf under its channel lock) + optional ex-handler. The Go
   runtime keeps doing park/wake/rendezvous/select — S19 shows user-space
   reimplementations pay 2.7–5.6×. Transducer semantics verified against
   oracle: map/filter/mapcat expansion/`reduced`→close/policy interaction.
   Documented divergence: a `mapcat` expansion larger than the free buffer
   applies backpressure mid-expansion (values identical; timing differs).

2. **Close.** `close!` flips the flag and closes `done`; takes prefer
   draining data (non-blocking probe → select data|done → final probe);
   puts check the flag up front. This makes ALL oracled close behaviors
   match: closed-read nil after draining buffer AND parked puts, put-after-
   close false, double-close no-op, blocked takers wake nil. It also
   deletes every send-on-closed panic/recover shim (including S10's alts
   recover dance). This reworks `pkg/lang/chan.go` — the only part of the
   change touching shipped behavior; it lands first and re-runs the whole
   M4-v0 conformance set.

3. **alts!/alt!.** Dynamic port vectors → the S10 `Alts` on
   `reflect.Select` with `:default`/`:priority`, extended with one `done`
   recv case per read port (2n cases; linear ~50 ns/case). `alt!`/`alt!!`
   ship as macros over `alts!`; AOT emission of a real `select` for static
   `alt!` is a recorded performance-ladder rung, not part of this change.

4. **Park names.** `<!!`/`>!!`/`alts!!`/`thread` remain aliases of
   `<!`/`>!`/`alts!`/`go` (M4-v0 contract, design/05 §4). The one-way
   portability rule (precedence principle applied to libraries): code that
   works on JVM core.async works identically here; code legal here that
   the JVM rejects (`<!` outside `go`) is a *documented* extension.

5. **Namespace.** `core/async.cljg`, embedded like core/test.cljg, interns
   the canonical vars in `clojure.core.async`; the M4-v0 clojure.core
   names stay as aliases of the same vars (shipped tests + REPL habit;
   they shadow nothing in real clojure.core so the precedence principle is
   satisfied). Everything new is async-ns-only.

## Consequences

- Users get real core.async source compatibility:
  `(require '[clojure.core.async :as async])` works, and JVM-oracled
  conformance files freeze it.
- The performance story is honest and CI-checkable: channel ops within
  8–20% of raw Go channels, alts ~100 ns — budgets can cite S19's tables
  (host-relative per ADR 0024).
- `(chan 0)` changes from "unbuffered" to "throws" — the only user-visible
  M4-v0 break, justified by oracle parity (JVM programs cannot contain it).
- The IOC-transform delete is now permanent architecture: any future
  feature needing "which go block am I in" has no transform to hook — the
  answer is always "you are a goroutine".
- STM-adjacent and executor knobs of JVM core.async
  (`*thread-pool-executor*`, dispatch buffering) have no analog and are
  documented as N/A.
- Supersedes nothing; extends design/05 §4 and the M4 milestone; M4-v0's
  clojure.core placement is ratified as aliases rather than reverted.
