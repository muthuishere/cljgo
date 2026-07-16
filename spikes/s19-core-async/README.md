# Spike S19 — core.async, first-class, on Go channels

ADR 0027 pipeline, stage 1. Closes toward **ADR 0040** and the OpenSpec
change `core-async-first-class`. Owner mandate (2026-07-17): core.async as
a first-class library on real Go channels — settle the design before any
implementation.

Prior art this spike builds on (not repeated here):

- **S10** (`spikes/s10-dynamic-alts/RESULTS.md`) — dynamic `alts!` on
  `reflect.Select`: 94.7 ns @ 2 ports vs 37.5 ns static `select`, linear
  growth, all core.async semantics reproduced, `testing/synctest` works.
- **M4-v0** (shipped): `*lang.Channel` wraps `chan any` + a buffer-policy
  field (`pkg/lang/chan.go`), `chan/>!/<!/close!/go/thread/timeout/alts!/
  dropping-buffer/sliding-buffer` interned into **clojure.core**
  (`pkg/eval/builtins.go`, `pkg/eval/chan_builtins.go`).
- design/05 §4 — the thesis: no IOC transform; `go` blocks are real
  goroutines; interop and core.async are the same fabric.

## Questions (each needs a measured or oracle-cited answer)

**Q1 — Channel representation (load-bearing).** core.async channels carry
fixed/dropping/sliding buffers, transducers (`(chan 10 (map inc))`), and
ex-handlers; raw Go channels carry none. Candidates:

- (a) own channel type (mutex + condvar / handler queues, i.e. a Go port
  of `ManyToManyChannel`), Go chan only at the interop edge;
- (b) two kinds — raw Go chan for plain cases, wrapper only when
  xform/policy is requested;
- (c) always one wrapper struct holding a Go chan + buffer/xform logic
  (the M4-v0 shape, extended).

Constraint (design/05): interop is the product — a Go `chan T` received
FROM interop must work with `<!`/`>!`/`alts!`, and a cljgo channel should
reach Go APIs wherever the type system allows.

**Q2 — `alts!` mechanism.** `reflect.Select` (S10's answer) vs a
core.async-style handler protocol (per-op commit flag, parked-op queues —
only possible on a mutex-channel representation, so Q1 and Q2 are one
decision). Prototype BOTH, measure, pick. Must support `:default` and
`:priority`.

**Q3 — `timeout` semantics.** Closed after N ms — but JVM core.async
*caches* timeout channels per ~10 ms bucket (two calls in the same window
return the SAME channel object). Oracle the real behavior; decide whether
we match the caching or only the close-after-N semantics.

**Q4 — Park vs block.** On the JVM `<!` outside a `go` block throws; here
both are just channel ops on a real goroutine. Oracle the JVM error, then
decide: mirror the throw for portability, or allow `<!` anywhere as
documented looseness (code that works on JVM core.async must work
identically here; the reverse may not hold).

**Q5 — Namespace + migration.** Real code does
`(require '[clojure.core.async :as async])`; our primitives live in
clojure.core (M4-v0 placed them there as a precedence-safe addition —
none of the names exist in real clojure.core). Decide the canonical
namespace, the alias/migration story, and oracle the edge semantics
against REAL core.async 1.6.681 on JVM Clojure 1.12.5: nil-put throws,
closed-read → nil, nil-channel ops block forever.

**Q6 — Surface inventory.** Tier the full core.async API (T1 core, T2
plumbing, T3 pipelines) with a JVM-semantics note + our mapping + effort
class per entry — the input to the OpenSpec task list.

## Exit criteria (written before any code, per ADR 0027)

1. A benchmark table: rendezvous latency + buffered throughput for raw Go
   chan / Go-chan-backed wrapper / mutex-condvar channel — the tax of each
   representation is a number, not a guess.
2. A working transducer-on-put prototype on the leading representation,
   including expansion (`mapcat`), filtering, and `reduced` → close,
   validated against JVM core.async oracle output.
3. A minimal handler-protocol alts prototype benchmarked against S10's
   `reflect.Select` numbers on the same shape (n ports, one ready).
4. Oracle transcripts (`oracle/`) for: timeout caching identity, `<!`
   outside go, nil-put, closed-read, nil-channel behavior, alts
   `:default`/`:priority`, dropping/sliding interaction with xform.
5. `VERDICT.md` with a per-question recommendation and the numbers that
   justify it.

Prototype code is throwaway (own `go.mod`, never merges into `pkg/`).
