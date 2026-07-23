---
title: Concurrency & core.async
description: Real goroutines from Clojure — go blocks with no CPS rewrite, real Go channels, the full practical core.async surface, plus atoms, agents, refs, and futures.
---

core.async's `go` macro on the JVM is a CPS/state-machine rewrite that
exists because JVM threads are expensive and `<!` must *park*. On Go,
goroutines already **are** the cheap thing core.async emulates — so cljgo
deletes the transform. `(go body)` runs the body in a real goroutine;
channels are real Go channels under one wrapper type; `<!`/`>!` simply
block that goroutine (ADR 0040).

```clojure
(def c (chan 3))
(>! c 10)
(>! c 20)
(>! c 30)
(close! c)
[(<! c) (<! c) (<! c) (<! c)]   ; => [10 20 30 nil]
```

`(go body)` returns a result channel that receives the body's value then
closes, so `(<! (go ...))` composes:

```clojure
(<! (go (+ 1 2)))   ; => 3
```

`alts!` waits on multiple ports, with `:default` for non-blocking;
`(timeout ms)` is a channel that closes after `ms`:

```clojure
(def c (chan 1))
(>! c 42)
(first (alts! [c]))                       ; => 42
(alts! [(chan)] :default :none)           ; => [:none :default]
(<! (timeout 20))                         ; => nil (closed after 20 ms)
```

The canonical namespace is `clojure.core.async`; the channel primitives
are also aliased in `clojure.core` for REPL convenience:

```clojure
(require '[clojure.core.async :as async])
(def c (chan 3))
(>! c 1) (>! c 2) (close! c)
(async/<!! (async/go-loop [acc 0]
             (if-let [v (async/<! c)]
               (recur (+ acc v))
               acc)))                     ; => 3
```

## What exists

cljgo implements **55 publics — every non-deprecated, non-internal var
of JVM core.async 1.6.681**, each behavior frozen against the real JVM
library as oracle (audit: `docs/core-async-audit-2026-07.md`; tests:
`conformance/tests/chan-*.clj`, 55 files). That includes:

- **Core:** `chan` (fixed/`dropping-buffer`/`sliding-buffer`), `close!`,
  `>!`/`<!`, `put!`/`take!`, `offer!`/`poll!`, `go`, `go-loop`, `thread`,
  `thread-call`, `timeout`, `promise-chan`, `alts!`, `alt!`
- **Transducers on channels:** `(chan n (map f) ex-handler)` — expansion,
  `reduced`→close, and buffer-policy interaction all oracle-verified
- **Plumbing:** `mult`/`tap`/`untap`, `pub`/`sub`/`unsub`, `mix` (with
  solo/mute/pause), `pipe`, `merge`, `split`, `map`, `into`, `reduce`,
  `transduce`, `onto-chan!`, `to-chan!`
- **Pipelines:** `pipeline`, `pipeline-blocking`, `pipeline-async` —
  ordered results, parallelism `n`, `ex-handler` support

## What is deliberately absent

- The **11 upstream-deprecated vars** (`map<`, `map>`, `filter<`,
  `filter>`, `remove<`/`remove>`, `mapcat<`/`mapcat>`, `partition`,
  `partition-by`, `unique`) — core.async itself tells you to use
  transducers on `chan` instead, and cljgo already supports those.
- The **21 internal/implementation vars** (IOC machinery, executor
  knobs) — cljgo replaces that machinery with the Go runtime itself.

## Documented differences from the JVM

All values are identical to JVM core.async; the differences are about
mechanism, and each is recorded, not accidental:

- **`<!`/`>!`/`alts!` are legal on any goroutine.** The JVM's "used not
  in (go ...) block" throw exists only because of its IOC transform.
  `<!!`/`>!!`/`alts!!` and `thread` exist as aliases for source
  compatibility — without parking there is no park/block distinction.
  Portability is one-way by design: JVM core.async code runs here
  unchanged; `<!` outside `go` is a documented extension the JVM rejects.
- `(chan 0)` throws and nil-channel ops throw, matching the JVM.
- Parked puts survive `close!` and deliver to later takers (JVM parity,
  verified).
- `(timeout ms)` is a fresh channel per call, not the JVM's per-tick
  cached channel — the docstring's promise (closes after ms) holds; the
  identity artifact does not.
- `pipeline-blocking` is observably identical to `pipeline` — the JVM's
  compute-vs-blocking executor split collapses on goroutines.

Channels obtained from Go APIs work directly with `<!`/`>!`/`alts!` —
interop and core.async are the same fabric (see
[the interop guide](/cljgo/guides/interop/)).

## Reference types

Beyond channels, the standard Clojure reference types are implemented
and conformance-tested:

- **Atoms** — `swap!`/`reset!`/CAS, validators, watches.
- **Agents** — `send`/`send-off` (both enqueue to the agent's goroutine
  mailbox), `await`, error modes and restarts.
- **Refs & `dosync`** — an STM-lite (ADR 0038): `ref`, `alter`,
  `ref-set`, ref watches; "No transaction running" outside `dosync`,
  and nested transactions join the outer one
  (`conformance/tests/stm-lite-refs-agents.clj`).
- **`future` / `promise`** — real goroutines behind `IDeref`;
  `future-cancel` is cooperative (a pending future cancels, deref then
  throws; the body goroutine is not interrupted).
- **Dynamic vars** — `binding` works per-goroutine, and `bound-fn`
  conveys bindings into `future`s.
- `locking`, `volatile!`, `add-watch`/`remove-watch` — present.

Not present: `pmap` is not implemented yet.
