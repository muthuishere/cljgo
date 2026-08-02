---
title: Why cljgo
description: Write Clojure, ship a single Go binary. What cljgo is, what is measured and tested today, and an honest list of what is not done yet.
---

**You write Clojure. It compiles to plain Go, and you ship one binary.**

No JVM on the server. No runtime to install. No `java -jar`. A `cljgo build`
produces a single static executable you can `scp` anywhere — the same way a Go
program is delivered — and the same code you just typed at the REPL is what
runs inside it.

## If you're new to Clojure

Clojure is a small, practical Lisp built around immutable data and a REPL that
stays live while you work. The usual objection is deployment: the JVM, the
startup time, the ops story.

cljgo removes that objection by targeting Go instead. You get Clojure's
interactive development, and Go's deployment story — one file, starts in
milliseconds.

```clojure
(require '[cljg.http :as http])

(http/serve {:port 8080
             :handler (fn [req] {:status 200 :body "hello"})})
```

That's a complete web server. `cljgo run` it now, `cljgo build` it when you're
done. Start at the [quickstart](/cljgo/quickstart/) or the
[by-example course](/cljgo/by-example/01-hello-world/).

## If you're leaving Clojure because of where the jobs are

This is the honest reason many people move to Go, and it's worth naming.

cljgo doesn't ask you to choose. What it emits is **plain Go source**, built by
the ordinary Go toolchain into an ordinary Go binary. Your artifact is one your
team's existing Go infrastructure already knows how to build, scan, deploy and
run. And the Go ecosystem is directly callable — not through a bridge, but as
if you'd imported it in Go:

```clojure
(require-go '[strings])
(require-go '[strconv])

(println (strings/ToUpper "hello"))   ; => HELLO
(println (strconv/Atoi "123"))        ; => [123 nil]   (T, error) → a vector
(println (strconv/Atoi! "456"))       ; => 456         ! unwraps, or throws
```

No wrappers, no generated stubs, no hand-written bindings. Third-party modules
are one line in `build.cljgo`:

```clojure
(go-require app "github.com/gorilla/websocket" "v1.5.3")
```

`core.async` runs on real goroutines — no CPS rewrite. So the skills stay
Clojure and the output stays Go. See the
[interop guide](/cljgo/guides/interop/).

## What is actually true today

Every number here was measured on the current release. Where something is not
done, it is in [what's not done](#whats-not-done-yet) below rather than
softened here.

**Clojure compatibility.** Against the jank
[clojure-test-suite](https://github.com/jank-lang/clojure-test-suite),
unmodified upstream: **238/242 files passing (98.3%)**, **242/242 vars
resolved**, **0 failures**. The 4 errors are missing `:cljgo` reader-conditional
branches in the suite itself, not broken semantics. Run `cljgo suite` to
reproduce it yourself.

**One binary, fast start.** `(println "hi")` compiles to a **7.1 MB** static
binary that starts in **5.5 ms** — against let-go 5.51 ms, babashka 10.9 ms,
and Clojure on the JVM 302 ms. Measured 2026-08-02; the full table, including
the rows cljgo *loses*, is on [benchmarks](/cljgo/reference/benchmarks/).

**The REPL and the binary agree.** Every semantic test runs twice, interpreted
and compiled, on every commit. A REPL↔binary divergence is a release blocker,
not a known issue.

**Web apps compile too.** `cljgo new -template web` then `cljgo build` produces
a **15 MB** static binary that serves HTTP on its own — no JVM, no app server,
nothing else installed. That is what the
[deploy guide](/cljgo/guides/deploy/)'s scratch-image Dockerfile ships.

## bri: the batteries, and they compile

`bri` is cljgo's application framework — HTTP, routing, HTML, config, auth,
tracing, a data layer — and it is **first-class, not a side project**. It AOT-
compiles to the same single static binary everything else does, verified by
building and running one.

```
cljgo new -template web myapp
cd myapp && cljgo build && ./myapp
bri: listening on http://localhost:3000
```

Start with [your first app](/cljgo/bri/tutorial/) (15 minutes).

## The design contract

Five priorities, in order. Everything is judged against them, Clojure-first.

1. **Universal interop.** Any Go module is importable and callable with zero
   hand-written bindings — the Go ecosystem *is* the standard library.
2. **Full REPL-driven development.** Live re-`def`, `defmacro` at the prompt,
   namespaces, `eval`, `resolve`, and nREPL for CIDER/Calva.
3. **Faithful Clojure.** Persistent data structures with real structural
   sharing, transients, a numeric tower, macros as plain fns, seqs, vars.
   Nothing cljgo adds may shadow or change `clojure.core` semantics.
4. **Performance in both modes**, gated in CI like tests. A perf regression is
   treated like a conformance failure.
5. **Single-file deployment.** One static binary. cgo-based Go modules (sqlite
   drivers, sensors, GUI/audio) import like anything else.

## What's not done yet

- **`clojure.core` is not complete.** 98.3% on the suite and climbing; the
  per-namespace ledger is `docs/fundamentals-audit-2026-07.md` in the repo. The
  satellite namespaces (`clojure.string`, `set`, `edn`, `walk`, `zip`, `data`,
  `repl`, `pprint`, `test`) are complete against the 1.12.5 oracle; core itself
  is younger.
- **Not the fastest at everything.** Compiled cljgo wins `tak`, `fib`,
  `loop-recur`, `persistent-map` and startup, and **loses** `map-filter` to
  let-go and `reduce` and `transducers` to babashka. Those are roadmap gaps,
  not deliberate trade-offs. The interpreter is a tree-walker and is slow — that
  is what `cljgo build` is for.
- **We need more testers.** Nearly every defect fixed in the last release cycle
  was found by a downstream project, not by our own gate — including a
  descending `range` that returned one element to `first`/`rest` while `count`
  said five, and an HTTP timeout option that was silently ignored. Both passed
  green test suites on both hosts. If you try cljgo and something behaves
  differently from JVM Clojure, that is the most valuable thing you can send
  us: [open an issue](https://github.com/muthuishere/cljgo/issues).
- **Some batteries are decisions, not code.** The pure-Go SQLite default,
  durable job queue and curated stdlib are ratified ADRs and spikes, **not
  shipped**.
- **C FFI via purego is proposed** (ADR 0044, spiked), not landed. cgo-based Go
  modules work today; direct C FFI does not.
- **comptime** (Zig-style compile-time value execution) is on the roadmap, not
  implemented.

The full ledger is on [status & roadmap](/cljgo/reference/roadmap/). Ready?
[Install](/cljgo/install/), then the [quickstart](/cljgo/quickstart/).
