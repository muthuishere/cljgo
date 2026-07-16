VERDICT: core.async lands as `clojure.core.async` on ONE wrapper type over a
real Go chan (the M4-v0 shape, with close! reworked to a done-chan so parked
puts survive close, +8–16% over raw Go channels); dynamic `alts!` stays on
`reflect.Select` (S10 confirmed, handler protocol rejected — its channel
representation costs 2.7–5.6× on every basic op); `timeout` matches
close-after-N semantics but not the JVM's per-tick channel cache; `<!`/`>!`
work everywhere (documented looseness — JVM code unaffected); T1→T3 surface
tiered below. Proceed to ADR 0040.

# S19 — core.async, first-class, on Go channels

Machine: darwin/arm64 (Apple M5 Pro), go1.26.3, `-benchtime 2s -count 3`
(medians; full runs in `bench-results.txt`). Semantics frozen against REAL
JVM core.async **1.6.681** on Clojure 1.12.5 (`oracle/transcript.txt`,
`transcript2.txt`, `probe3.txt` — every claim below marked *oracle* is a
line in those files). All prototypes pass `go test -race -count=2`.

## Q1 — Channel representation: one wrapper over a real Go chan

Three candidates prototyped and measured:

| representation | rendezvous | buffered(100) throughput | allocs/op |
|---|---|---|---|
| raw Go `chan any` (baseline) | 100.5 ns | 25.9 ns | 0 |
| **(c) `GoBacked`** — wrapper over Go chan, xform/policy on the put side | 108.9 ns (+8%) | 30.2 ns (+16%) | 0 |
| (c′) `GoBacked2` — same + close-fidelity via done-chan (see below) | 137.3 ns (+37%) | 31.0 ns (+20%) | 0 |
| (a) `AsyncChan` — ManyToManyChannel port (mutex + handler queues) | 272 ns (**2.7×**) | 144 ns (**5.6×**) | 8 (352 B) |

- The Go runtime's park/wake/rendezvous machinery is unbeatable from user
  space: the mutex+handler-queue port pays 8 allocs and 2.7–5.6× per op
  because every blocking op must build a callback + signal chan — exactly
  the machinery the Go scheduler gives us for free.
- **Transducers work on the Go-chan-backed wrapper** (`gobacked.go`): the
  xf step is serialized by a put-side mutex (core.async serializes it under
  the channel lock the same way). Verified against oracle: `map` (xform-map
  => 2), `filter` (=> 3), `mapcat` expansion (=> [1 1 1]), `reduced` →
  channel closes (xform-reduced-closes => [1 2 nil false]), dropping+xform
  (=> [1 2 nil]), sliding+xform (=> [4 5 nil]), xform-requires-buffer
  (assert, oracle throws). Identity-xform tax on the buffered path: 33.6 ns
  vs 30.2 (+11%).
- Option (b) (two kinds — raw Go chan for plain channels) is rejected: the
  put/take/close fns and `instance?` would fork on channel kind, and the
  close-fidelity fix below needs the wrapper anyway. Plain `(chan)` through
  the wrapper costs +8% — not worth two types.
- **Interop stays whole**: a FOREIGN Go `chan T` (from any Go API) works as
  an alts port / take source via reflect unchanged (tests
  `TestAltsReflectForeignChan`, closed `chan int` normalizes to nil not 0 —
  S10's finding re-verified). Our channel exposes its backing chan
  (`Raw() <-chan any`) at the interop edge; AOT can pass it to Go code
  taking `<-chan any` directly, and a receive-only view keeps outsiders
  from bypassing the xform or closing it.

### The oracle surprise: close! must not kill parked puts

*oracle probe3*: `parked-put-survives-close => [:v true]` — on the JVM a
put parked before `close!` stays parked, **is delivered** to a taker that
arrives after the close, and returns true. And
`timeout-put-still-parked-after-close => :still-parked-800ms-after-close` —
close never rejects a parked put at all. M4-v0's shape (close the Go chan,
recover the send-on-closed panic) returns false and **loses the value** — a
real semantic divergence, and Go's close-with-blocked-senders panic makes
it unfixable while we call `close()` on the data chan.

`GoBacked2` fixes it without recovers: close! never closes the data chan;
a separate `done` chan signals closure; Take drains data preferentially
(non-blocking probe → blocking select on data|done → final drain probe) and
puts check a closed flag up front. All close semantics then match the
oracle: closed-read => nil, buffer drains first ([1 2 nil]), put-after-close
=> false, double-close no-op, blocked takers wake with nil, parked puts
survive. Cost of fidelity: +28 ns on rendezvous, ~1 ns on buffered ops,
zero allocs. **Recommendation: ship the GoBacked2 close design merged with
GoBacked's xform/policy layer.** (Side-effect: no panic/recover shim left
anywhere in the channel runtime; the S10 closed-send recover dance in alts
also disappears, because the data chan is never closed.)

Documented divergence (values never differ, only backpressure timing): a
`mapcat` expansion larger than the free buffer blocks the putter mid-
expansion here, while the JVM completes it into a temporarily over-full
buffer (*oracle* xform-mapcat-expansion => [1 1 1] both ways).

## Q2 — alts!: reflect.Select wins; handler protocol rejected

Both candidates prototyped head-to-head (one ready port per call, cost
includes per-call case construction):

| mechanism | n=2 | n=8 | allocs n=2 |
|---|---|---|---|
| static Go `select` (compiled `alt!` fast path) | 31.6 ns | — | 0 |
| **`reflect.Select`** (`alts_reflect.go`, = S10) | 101.5 ns | 399 ns | 3 (208 B) |
| handler protocol on AsyncChan (`asyncchan.go`) | 132.7 ns | 274 ns | 10 (404 B) |

The handler protocol only exists on the mutex-channel representation — Q1
and Q2 are one decision. It wins at n=8 (274 vs 399 ns: no per-case reflect
value boxing) but loses at the common n=2 AND drags in a representation
that is 2.7–5.6× slower on every put/take in the program. reflect.Select's
linear ~50 ns/case is irrelevant next to any real work an alts arm does,
and foreign Go `chan T` ports come free. **reflect.Select, confirming S10;
static `alt!` emission to a real `select` stays the AOT performance rung.**

`:default` and `:priority` both verified on the JVM (*oracle*: alts-default
=> :none with port :default; alts-priority-first-wins => :a; combined =>
:dflt) and both already implemented in the S10 prototype (priority = ordered
non-blocking pass, then blocking select).

GoBacked2 interaction: alts read-cases must each carry a second
`done`-recv case (2n cases, fires → final drain probe → nil). Linear
scaling measured at ~50 ns/case puts n=8 at roughly the old n=16 (~800 ns)
— accepted; still micro-scale.

## Q3 — timeout: match the semantics, not the cache

*oracle*: `timeout-identical-same-tick => true`,
`timeout-identical-after-gap => false` — the JVM caches timeout channels
per ~10 ms bucket, so two calls in one tick return the SAME object.
`timeout-closes => nil` after the delay.

Decision: **implement `(timeout ms)` as a fresh channel closed by
`time.AfterFunc` — semantics only, no cache.** The docstring promises "a
channel that will close after msecs"; channel identity across calls is an
implementation artifact (and a footgun — a put to a cached timeout is
visible to every sharer). Divergence is observable only via
`(identical? (timeout n) (timeout n))`, which no portable program does.
Documented looseness; a cache can be added later behind the same fn if a
real program ever needs the dedup (the JVM does it to avoid timer churn —
Go's runtime timers are cheap).

## Q4 — Park vs block: `<!`/`>!` legal everywhere (documented looseness)

*oracle*: `take-outside-go => AssertionError "<! used not in (go ...)
block"`, same for `>!`. The JVM throw exists because `<!` is a marker the
IOC transform rewrites — outside `go` there is nothing to rewrite, so it
throws. Here there is no transform; `<!` IS `<!!`.

Decision: **allow `<!`/`>!`/`alts!` on any goroutine; do not mirror the
throw.** The portability test is one-directional and passes: code that
works on JVM core.async only ever uses `<!` inside `go`, where behavior is
identical — no working JVM program observes the difference (nobody relies
on getting an AssertionError). Mirroring the throw would require tracking
"am I inside a go block" per goroutine — cost and machinery for the sole
purpose of rejecting programs that would work. This is the thesis of
design/05 §4 made visible: the park/block distinction is deleted, not
emulated. Documented in the ns docstring + spec as an extension.

## Q5 — Namespace: `clojure.core.async` is canonical; core keeps aliases

M4-v0 interned `chan/>!/<!/>!!/<!!/close!/go/thread/timeout/alts!/alts!!/
dropping-buffer/sliding-buffer/go*` directly into **clojure.core**
(`pkg/eval/builtins.go` + `pkg/eval/chan_builtins.go`, comments citing
design/05 §4; no ADR placed them — the batch predates the ADR-0027
pipeline). None of those names exist in real clojure.core, so it was a
precedence-safe addition — but it is not where portable code looks:
`(require '[clojure.core.async :as async])` must work.

Decision: **`clojure.core.async` becomes the canonical namespace**
(embedded `core/async.cljg`, the established pattern — cf. core/test.cljg,
core/set.cljg), with every var interned there. The existing clojure.core
names REMAIN as aliases to the same vars (removing them would break shipped
conformance tests and REPL muscle memory; keeping them shadows nothing).
New surface (T1 additions and everything after) interns ONLY in
clojure.core.async. Edge semantics locked by oracle:

| behavior | JVM oracle | ours |
|---|---|---|
| nil put | throws IllegalArgumentException "Can't put nil on channel" | panic, same message ✅ (already) |
| closed read | nil; buffer/parked puts drain first | ✅ with GoBacked2 |
| **nil-channel ops** | **throws** IllegalArgumentException (1.6.681) — NOT block-forever | panic (M4-v0 already panics; message aligns in T1) |
| `(chan 0)` | throws AssertionError "fixed buffers must have size > 0" | **currently allowed = divergence; T1 makes it throw** |
| put on closed | returns false | ✅ |
| double close | no-op nil | ✅ |
| xform without buffer | throws | prototype matches |
| xform exception, no ex-handler | put completes; poisoned value dropped; channel usable (*oracle* xform-no-exh) | match observed behavior; T1 conformance test |

Note the task-brief assumption "nil-channel ops block forever" is
**refuted by the oracle** — 1.6.681 throws (`nil-chan-take/put =>
[:threw IllegalArgumentException]`). Oracle discipline working as intended.

## Q6 — Surface inventory (tiers for the spec)

Effort: S = hours, M = ~a day, L = multi-day. "pump" = plain goroutine
loop over channel ops — on real goroutines these are trivial.

**T1 — core (the language of channels):**

| fn | JVM semantics note | our mapping | effort |
|---|---|---|---|
| `chan` (n / buffer / +xform / +ex-handler) | buffer required with xform; (chan 0) throws | GoBacked2 + xform layer; fix (chan 0) | M |
| `buffer`, `dropping-buffer`, `sliding-buffer`, `unblocking-buffer?` | buffer objects | BufferSpec (exists); add `buffer`, predicate | S |
| `>!` `<!` `>!!` `<!!` | park/block pairs | one impl, four names (Q4) | S (exists) |
| `alts!` `alts!!` | dynamic select, :default/:priority | reflect.Select (S10/Q2) + done-case | M |
| `alt!` `alt!!` | macro over alts! with result exprs | macro → alts! now; AOT `select` later rung | M |
| `timeout` | closes after ms (cached on JVM) | AfterFunc-closed chan, no cache (Q3) | S |
| `go` `go-loop` `thread` | IOC vs real thread; returns result chan (thread's never nil-closes early) | real goroutine (exists); `go-loop` = `(go (loop ...))` macro | S |
| `close!` | parked puts survive (Q1) | done-chan close | M (the GoBacked2 rework) |
| `put!` `take!` | async + callback, no goroutine burn on JVM | goroutine + callback (goroutines are the cheap thing) | S |
| `offer!` `poll!` | non-blocking; nil (not false) when they don't succeed | select/default; return nil on miss (*oracle* offer-poll => [true nil 1 nil]) | S |
| `promise-chan` | first put wins, every take sees it, later puts true-but-ignored (*oracle* [:a :a]) | latch struct: closed done + stored value | M |

**T2 — plumbing (all goroutine pumps + a mutex'd registry each):**

| fn group | note | effort |
|---|---|---|
| `onto-chan!` `to-chan!` | seq→chan pump | S |
| `pipe` | chan→chan pump, close propagates | S |
| `merge` | n→1 pump (NOT an alts substitute — S10's fan-in analysis) | S |
| `into` `reduce` `transduce` | take-loop into collection/rf; returns a chan | M |
| `mult` `tap` `untap` `untap-all` | registry + fan-out pump; slow tap blocks all (JVM parity) | M |
| `pub` `sub` `unsub` `unsub-all` | topic-fn → per-topic mult | M |
| `mix` `admix` `unmix` `toggle` `solo-mode` | states (mute/pause/solo) on a fan-in pump | L |
| `split` | pred → two chans | S |

**T3 — pipelines:**

| fn | note | effort |
|---|---|---|
| `pipeline` | JVM: n compute threads. Here: n goroutines | M |
| `pipeline-blocking` | JVM: separate blocking pool. Here: **collapses into `pipeline`** — same goroutines (document as alias) | S |
| `pipeline-async` | af callback contract differs (user closes result chan) — stays its own fn | M |

Out of scope (JVM API, decided against or deferred): `ioc-macros` internals
(nothing to port), `defblockingop`, `assert-unread`, deprecated `map<`/
`filter<`/etc. (removed upstream), `Mult`/`Mix`/`Pub` protocols exposed as
user protocols (internal here until someone needs to extend them).

## Recommendation

1. ADR 0040: representation = one wrapper over Go chan with done-chan
   close (Q1/Q1′), alts on reflect.Select + static `alt!` rung (Q2),
   timeout semantics-only (Q3), `<!` everywhere (Q4), clojure.core.async
   canonical + core aliases (Q5).
2. OpenSpec change `core-async-first-class`, tasks sequenced T1→T3, every
   semantic task carrying oracle-cited conformance files (the transcripts
   here are the citations).
3. The GoBacked2 close rework is the only change touching existing
   behavior (`pkg/lang/chan.go` close/put paths) — it must come first and
   re-run the whole M4-v0 conformance set.
