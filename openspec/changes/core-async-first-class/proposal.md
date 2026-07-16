# core-async-first-class

## Why

Owner mandate (2026-07-17): core.async, first-class, on Go channels. The
thesis (design/05 §4, the owning contract) deletes the JVM's IOC/CPS `go`
transform — goroutines are the cheap thing it emulates — and M4-v0 shipped
the primitive slice into clojure.core. Real programs need the rest:
`(require '[clojure.core.async :as async])`, transducer channels,
`alt!`, `put!/take!/offer!/poll!`, `promise-chan`, `go-loop`, mult/pub/
pipeline. Spike S19 (`spikes/s19-core-async/`) settled every open design
question with measurements and a JVM core.async 1.6.681 oracle; **ADR 0040**
(proposed) records the decisions this change implements. Relies also on
ADR 0027 (this pipeline), ADR 0024 (host-relative perf budgets), S10.

## What Changes

- **`pkg/lang/chan.go` close rework (the only behavior change):** close!
  stops closing the Go data chan; a `done` chan + closed flag give JVM
  close fidelity — parked puts survive close and deliver to later takers
  (oracle `parked-put-survives-close => [:v true]`); every send-on-closed
  panic/recover shim is deleted. `(chan 0)` becomes an error (oracle:
  AssertionError on JVM). Measured cost: +28 ns rendezvous, ~1 ns buffered.
- **Channel feature layer:** transducer + ex-handler on `chan`
  (put-side-serialized xf step; `reduced` closes; xform requires a buffer),
  `buffer`/`unblocking-buffer?`, aligned nil-channel/nil-put error messages.
- **`clojure.core.async` namespace** (embedded `core/async.cljg`, the
  core/test.cljg pattern): canonical home of ALL async vars; existing
  clojure.core names remain aliases of the same vars; new surface is
  async-ns-only.
- **T1 completion:** `alt!`/`alt!!` (macros over alts!), `go-loop`,
  `put!`/`take!` (goroutine + callback), `offer!`/`poll!` (nil on miss),
  `promise-chan`, alts done-case integration.
- **T2 plumbing:** onto-chan!/to-chan!/pipe/merge/into/reduce/transduce,
  mult/tap/untap/untap-all, pub/sub/unsub/unsub-all, split, mix/admix/
  unmix/toggle/solo-mode — goroutine pumps.
- **T3 pipelines:** `pipeline` (n goroutines), `pipeline-blocking` (alias,
  documented), `pipeline-async` (distinct callback contract).
- Conformance files for every behavior above, expectations frozen from the
  S19 oracle transcripts (JVM Clojure 1.12.5 + core.async 1.6.681); dual
  harness (REPL + AOT) per ADR 0007.

## Non-goals

- No IOC transform, ever; no park/block distinction (`<!!` stays an alias
  of `<!`; `<!` legal anywhere — ADR 0040 #5's documented extension).
- No JVM timeout-channel cache (semantics only, ADR 0040 #4).
- No AOT `select` emission for static `alt!` (recorded performance-ladder
  rung, separate change).
- No executor/thread-pool knobs (`*thread-pool-executor*` etc.) — N/A on Go.
- No user-facing Mult/Mix/Pub protocols; internal until extension is needed.
- No removal of the M4-v0 clojure.core names (they become aliases).

## Impact

- `pkg/lang` (chan.go rework + xform layer + new helpers), `pkg/eval`
  (builtins move/aliasing), new `core/async.cljg`, `conformance/tests/*`.
- Shipped M4-v0 semantics change ONLY at `(chan 0)` (now throws, JVM
  parity) and close-vs-parked-put (now JVM-faithful; previously lossy).
- Perf: channel ops stay within ~8–20% of raw Go channels (S19 tables);
  budget assertions cite S19 numbers, host-relative per ADR 0024.
