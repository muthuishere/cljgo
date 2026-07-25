## ADDED Requirements

### Requirement: one channel representation over real Go channels
Every cljgo/core.async channel SHALL be a single wrapper type over a real
Go channel (buffer policy, optional transducer, optional ex-handler on the
wrapper; park/wake/rendezvous/select done by the Go runtime). There SHALL
be no second, from-scratch channel implementation, and no IOC/CPS rewrite
of `go` bodies — `(go ...)` runs a real goroutine (ADR 0040 #1; design/05
§4).

#### Scenario: interop channels are the same fabric
- **WHEN** a Go `chan T` obtained from a Go API is used with `<!`, or as an
  `alts!` port
- **THEN** it works unchanged, and a closed `chan T` read normalizes to
  nil (never the element type's zero value)

#### Scenario: the wrapper tax stays bounded
- **WHEN** the channel-op benchmarks run (rendezvous, buffered throughput)
- **THEN** wrapper ops stay within the ADR 0040-cited budget relative to
  raw Go channels on the same host (ADR 0024 host-relative discipline)

### Requirement: JVM-faithful close semantics
`close!` SHALL be idempotent, SHALL wake blocked takers with nil, SHALL
NOT reject or lose puts parked before the close (they remain deliverable
to later takers, returning true on delivery), SHALL cause subsequent puts
to return false immediately, and reads SHALL drain buffered values and
parked puts before yielding nil. The implementation SHALL NOT close the
backing Go data channel (oracle: parked-put-survives-close => [:v true]).

#### Scenario: parked put survives close
- **WHEN** a put parks on a full/unbuffered channel, the channel is then
  closed, and a taker arrives afterwards
- **THEN** the taker receives the parked value and the put returns true

#### Scenario: closed channel drains then nils
- **WHEN** a channel holding buffered values is closed and drained
- **THEN** takes yield the buffered values in order, then nil forever

### Requirement: transducer and ex-handler channels
`(chan buf-or-n xform)` and `(chan buf-or-n xform ex-handler)` SHALL apply
the transducer to each put on the put side: filtered values produce
nothing, expansions produce each output, and a `reduced` result closes the
channel. A transducer without a buffer SHALL be an error. An ex-handler
SHALL receive transducer-step exceptions; a nil return skips the value, a
non-nil return is put instead. All behaviors SHALL match the frozen JVM
core.async 1.6.681 oracle lines (spikes/s19-core-async/oracle/).

#### Scenario: reduced closes the channel
- **WHEN** `(chan 5 (take 2))` receives three puts
- **THEN** two values are takable, the third take yields nil, and the
  post-close put returns false (oracle: [1 2 nil false])

#### Scenario: buffer policies compose with xform
- **WHEN** `(chan (dropping-buffer 2) (map inc))` and
  `(chan (sliding-buffer 2) (map inc))` each receive puts 0..4 and close
- **THEN** drains yield [1 2 nil] and [4 5 nil] respectively

### Requirement: dynamic alts! on reflect.Select with default and priority
`alts!`/`alts!!` SHALL accept dynamic port vectors of read ports and
`[chan val]` write ports, SHALL support `:default` (taken only when no
port is ready) and `:priority` (listed order when several are ready),
SHALL return `[val port]`, and SHALL be implemented on `reflect.Select`
(ADR 0040 #3). `alt!`/`alt!!` SHALL be macros over the same mechanism.

#### Scenario: default only when nothing ready
- **WHEN** `(alts!! [c] :default :none)` runs against an idle channel
- **THEN** it returns `[:none :default]` without blocking

#### Scenario: priority is deterministic
- **WHEN** two ports are both ready and `:priority true` is passed
- **THEN** the first listed port wins (oracle: alts-priority-first-wins)

### Requirement: park names are aliases; ops legal on any goroutine
`<!!`, `>!!`, `alts!!`, and `thread` SHALL behave identically to `<!`,
`>!`, `alts!`, and `go` respectively, and all SHALL be usable on any
goroutine. The JVM's "used not in (go ...) block" error is intentionally
not mirrored; this SHALL be documented as a one-way portability extension
(JVM-working code behaves identically here; ADR 0040 #5).

#### Scenario: take outside go works
- **WHEN** `(<! c)` runs outside any `go` block against a ready channel
- **THEN** it returns the value (no error)

### Requirement: canonical namespace clojure.core.async with core aliases
All async vars SHALL be interned in `clojure.core.async` (embedded
`core/async.cljg`); the M4-v0 clojure.core names SHALL remain as aliases
resolving to the same vars; surface added by this change SHALL exist only
in `clojure.core.async`. No name SHALL shadow or alter anything in real
clojure.core (precedence principle).

#### Scenario: portable require works
- **WHEN** `(require '[clojure.core.async :as async])` then `(async/<!!
  (async/go 42))`
- **THEN** it yields 42, and `#'clojure.core.async/chan` is the same var
  as clojure.core's `chan` alias

### Requirement: oracle-locked edge semantics
The following SHALL match JVM core.async 1.6.681 exactly, with conformance
files citing the oracle transcript: nil put throws ("Can't put nil on
channel"); ops on a nil channel throw IllegalArgumentException-equivalent
errors (NOT block); `(chan 0)` is an error ("fixed buffers must have size
> 0"); `offer!`/`poll!` return nil (not false) on miss; `put!` returns
true when the put is accepted before any taker; `promise-chan` delivers
its first value to every take and silently accepts later puts; `(timeout
ms)` yields a channel that closes after ms (channel identity across calls
is NOT guaranteed — documented divergence from the JVM's tick cache,
ADR 0040 #4).

#### Scenario: chan 0 rejected
- **WHEN** `(chan 0)` is evaluated
- **THEN** an error is thrown (JVM parity; supersedes M4-v0's leniency)

#### Scenario: promise-chan latches
- **WHEN** a promise-chan receives :a then :b, then is taken twice
- **THEN** both takes yield :a (oracle: [:a :a])

### Requirement: plumbing and pipelines on goroutine pumps
T2 (`onto-chan!` `to-chan!` `pipe` `merge` `into` `reduce` `transduce`
`mult`/`tap`/`untap`/`untap-all` `pub`/`sub`/`unsub`/`unsub-all` `split`
`mix`/`admix`/`unmix`/`toggle`/`solo-mode`) and T3 (`pipeline`,
`pipeline-blocking` as its documented alias, `pipeline-async`) SHALL be
implemented as goroutine pumps over the T1 primitives, each with
oracle-verified conformance for its JVM-observable contract (close
propagation, slow-tap backpressure, pipeline result ordering).

#### Scenario: pipeline preserves order
- **WHEN** `(pipeline 4 out (map slow-inc) in)` processes 0..9
- **THEN** out yields 1..10 in order despite parallel workers

#### Scenario: pipeline-blocking is pipeline
- **WHEN** `pipeline-blocking` is called
- **THEN** behavior is identical to `pipeline` (real goroutines make the
  blocking/compute pool split meaningless; documented alias)
