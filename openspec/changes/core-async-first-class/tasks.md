# Tasks — core-async-first-class

Sequenced T1 → T3 (ADR 0040 / S19 Q6). Every task ends with the gates
(`go build ./... && go vet ./... && gofmt -l pkg cmd conformance && go test
./...`) green; every semantic task adds conformance `.clj` files whose
`;; expect:` lines are frozen from the S19 oracle transcripts
(`spikes/s19-core-async/oracle/`) or a fresh
`clojure -Sdeps '{:deps {org.clojure/core.async {:mvn/version "1.6.681"}}}'`
run, cited in a comment.

## 1. T1 — the channel core

- [x] 1.1 **Close rework** (`pkg/lang/chan.go`, ADR 0040 #2 — lands first):
  done-chan close per S19 `gobacked2.go` — close! never closes the data
  chan; Take = probe → select data|done → final probe; Put checks the
  closed flag; delete all send-on-closed recovers. `(chan 0)` now throws
  (oracle chan-zero). Re-run the ENTIRE existing M4-v0 conformance set
  (chan-*.clj) + new conformance: parked-put-survives-close,
  closed-read-drains-buffer, put-after-close false, double-close no-op.
  `go test -race` on pkg/lang. Gates green.
- [x] 1.2 **Transducer + ex-handler channels**: `(chan buf-or-n xform)`,
  `(chan buf-or-n xform ex-handler)`; xf step serialized on the put side;
  `reduced` → close; xform without buffer throws (oracle
  xform-unbuffered-chan-throws); ex-handler return nil skips, non-nil
  replaces (oracle xform-ex-handler*); no-ex-handler observed behavior
  (put completes, poisoned value dropped) frozen as conformance. Policy
  interaction: dropping+xform => [1 2 nil], sliding+xform => [4 5 nil].
  Add `buffer`, `unblocking-buffer?`. Gates green.
- [x] 1.3 **`clojure.core.async` namespace** (`core/async.cljg`, embedded
  per the core/test.cljg pattern): canonical vars for the whole surface;
  M4-v0 clojure.core names re-pointed as aliases of the SAME vars; new
  names async-ns-only; ns docstring records the ADR 0040 #4/#5
  extensions. Conformance: `(require '[clojure.core.async :as async])` +
  aliased-and-canonical names resolve to identical vars. Gates green.
- [x] 1.4 **alts done-integration + alt! macros**: extend lang.Alts (S10)
  with per-read-port done cases (drain-probe on fire); `alt!`/`alt!!`
  macros over alts! incl. `:default`/`:priority` and write ports
  `[c v]`; conformance from oracle alts-* lines. Gates green.
- [x] 1.5 **T1 stragglers**: `go-loop` (macro), `put!`/`take!` (goroutine
  + callback; put! true before taker — oracle), `offer!`/`poll!` (nil on
  miss — oracle offer-poll => [true nil 1 nil]), `promise-chan` (first
  put wins, all takes see it, later puts accepted-and-ignored — oracle
  [:a :a]), `timeout` stays uncached (ADR 0040 #4; conformance asserts
  close-after-ms only). Align nil-channel/nil-put error messages with the
  JVM's. Gates green.
- [x] 1.6 **Perf budget**: benchmark file pinning channel-op tax vs raw Go
  chans (rendezvous, buffered throughput, alts n=2/n=8) with
  host-relative assertions per ADR 0024, thresholds from the S19 tables
  (wrapper ≤ ~1.5× raw; alts ≤ ~5× static select at n=2). Gates green.

## 2. T2 — plumbing (goroutine pumps)

- [x] 2.1 `onto-chan!` / `to-chan!` / `pipe` / `split` / `merge` (pumps;
  close propagation oracled per fn). Also the deprecated non-bang
  `onto-chan`/`to-chan` and the `onto-chan!!`/`to-chan!!` thread-variant
  aliases (identical — ADR 0040 #5), plus `take` (bounded pump).
  pkg/lang/chan_pump.go + registerAsync; conformance chan-{to-chan,
  onto-chan,pipe,split,merge,take}.clj. Gates green.
- [x] 2.2 `into` / `reduce` / `transduce` (take-loop returning a result
  chan; reduced short-circuit oracled). conformance chan-{into,reduce,
  transduce}.clj. Gates green.
- [x] 2.3 `mult` / `tap` / `untap` / `untap-all` (registry + fan-out pump;
  slow-tap-blocks-all via blocking per-tap sends — JVM parity).
  conformance chan-{mult,untap,untap-all}.clj. Gates green.
- [x] 2.4 `pub` / `sub` / `unsub` / `unsub-all` (topic-fn → per-topic
  mult). `unsub-all` covers both the all-topics and single-topic arities.
  conformance chan-{pub-sub,unsub,unsub-all}.clj. Gates green.
- [x] 2.5 `mix` / `admix` / `unmix` / `unmix-all` / `toggle` / `solo-mode`
  (stateful fan-in pump; mute/pause/solo oracled — determinism via the
  atomic toggle-add-in-state, JVM parity). conformance chan-{mix,mix-mute,
  mix-pause,mix-solo,mix-solo-pause,mix-unmix,mix-unmix-all}.clj. Gates
  green.

## 3. T3 — pipelines (DONE 2026-07-22, change `apply/core-async-t3`)

- [x] 3.1 `pipeline` (n worker goroutines, ordered results — order
  guarantee oracled: `chan-pipeline-order.clj` proves in-order output
  under non-monotonic per-input latency) + `pipeline-blocking` as a
  documented observable-equal engine (ADR 0040 #9 — goroutines collapse
  the compute/blocking pool split). Also `ex-handler` (replace/drop) and
  `close?`. `lang.Pipeline` in `pkg/lang/chan_pump.go`, `areg`-registered.
  Gates green.
- [x] 3.2 `pipeline-async` (af `(fn [val result-ch])` delivers 0+ then
  closes result-ch; multi-emit / zero-emit / close? oracled). Concurrency
  bounded by the `jobs`/`results` channels (cap n). `lang.PipelineAsync`.
  Conformance: `chan-pipeline{,-order,-close,-xform,-ex-handler,-blocking,-async}.clj`.
  Gates green.

## 4. Wrap-up

- [ ] 4.1 design/05 §4 updated to cite ADR 0040 + final numbers; S19
  spike remains frozen. `openspec archive core-async-first-class` after
  owner sign-off. Gates green.
