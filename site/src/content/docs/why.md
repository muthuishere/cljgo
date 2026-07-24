---
title: Why cljgo
description: What cljgo is, the five priorities it is built against, where it is headed — and an honest list of what it is not yet.
---

cljgo is Clojure hosted on Go: a compiler, written in Go, that AOT-emits plain
Go source — the ClojureScript model, with Go playing the part of JavaScript —
plus a tree-walk evaluator that is the REPL and the macro engine. The same
source runs interpreted at the prompt and compiles to a single static native
binary, with byte-identical output on both paths.

That last clause is enforced, not aspired to: a dual-harness conformance suite
runs every semantic test both interpreted and compiled on every commit, and a
REPL↔binary divergence is a release blocker.

## Five priorities, in order

These are not a wishlist — they are the design contract. Everything in cljgo is
judged against them, Clojure-first.

1. **Universal interop.** Any Go module is importable and callable with zero
   hand-written bindings — the Go ecosystem *is* the standard library. C
   reaches in via cgo modules and purego FFI.
2. **Full REPL-driven development.** The tree-walk evaluator is a real Clojure
   REPL: live re-`def`, `defmacro` at the prompt, namespaces, `eval`,
   `resolve`, and nREPL for CIDER/Calva.
3. **Faithful Clojure principles.** Persistent data structures with real
   structural sharing, transients, a numeric tower, macros as plain fns, seqs,
   and vars as the indirection layer. Clojure is first-class: nothing cljgo
   adds may shadow or change `clojure.core` semantics.
4. **High performance in both modes.** A feature, gated in CI like tests, not
   asserted. A perf regression is treated like a conformance failure.
5. **Single-file deployment.** `cljgo build` produces one static binary
   (5.3 MB for hello, ~5 ms startup) — no JVM, no runtime install. cgo
   projects are supported, not tolerated: cgo-based Go modules (sqlite
   drivers, sensors, GUI/audio) import like anything else.

## Priority #1 in practice: zero-binding interop

`require-go` pulls in any Go package and calls it directly — no wrappers, no
generated stubs. The Go toolchain is the classpath, and it runs identically
interpreted and compiled:

```clojure
(require-go '[strings])
(require-go '[strconv])

(println (strings/ToUpper "hello"))
(println (strconv/Atoi "123"))    ; (T, error) → [v err] — errors as values
(println (strconv/Atoi! "456"))   ; ! suffix unwraps, or throws
```

Members (`(.Method r …)`, `(.-Field r)`), constructors (`(pkg/T. {…})`), and
core.async over **real goroutines** (no CPS rewrite) round it out. Third-party
modules are one line in `build.cljgo` —
`(go-require app "github.com/gorilla/websocket" "v1.5.3")` — with no
hand-written bindings. See the [interop guide](/cljgo/guides/interop/).

## Where it stands

Working REPL **and** native compiler. Against the jank
[clojure-test-suite](https://github.com/jank-lang/clojure-test-suite)
(unmodified upstream): **238/242 files passing (98.3%)**, 242/242 vars
resolved, 0 failures — the 4 errors are missing `:cljgo` reader-conditional
branches in the suite itself, not broken semantics. Run `cljgo suite` to
reproduce. Details on the [compatibility page](/cljgo/reference/compatibility/)
and [benchmarks](/cljgo/reference/benchmarks/).

## Where it's headed

**The Bun of Clojure** — one fast native binary, batteries included,
zero-config: Bun's ergonomics with Go's delivery and no runtime to distribute.
This is the direction (roadmap, not yet shipped): a data layer with pure-Go
SQLite as the zero-install default, a durable Postgres job queue plus
in-process cache, a curated Go-native stdlib, and Spring-Boot-style layered
config.

**The Zig model** — cljgo's "batteries" follow Zig's design, not
Leiningen's/deps.edn's: the build is a program (`build.cljgo` defines an
artifact DAG, not a data file), dependencies are code in that same file
(content-addressed, lockfile-pinned, one resolver for both the interpreted and
compiled legs), one library publishes to both the Go module ecosystem and
Clojars from one build description, and cross-compilation falls out of
emitting plain Go — pure-Go programs build for any OS/arch with no target
toolchain.

## What cljgo is NOT (yet)

Honest gaps, from the project's own status ledger:

- **`clojure.core` is not complete.** The suite score is 98.3% and climbing;
  the honest per-namespace ledger is `docs/fundamentals-audit-2026-07.md` in
  the repo. The satellite namespaces (`clojure.string`, `set`, `edn`, `walk`,
  `zip`, `data`, `repl`, `pprint`, `test`) are complete against the 1.12.5
  oracle; core itself is early and moving fast.
- **Not the fastest at everything.** Compiled cljgo wins the recursion and
  data-structure benchmark rows outright, but `reduce` and `transducers` are
  still honestly lost to babashka and let-go. The interpreter is a
  tree-walker and loses everywhere except against joker — that is what
  `cljgo build` is for.
- **The batteries are decisions, not code.** The Bun-direction items above
  (data layer, jobs/cache, curated stdlib, vault/i18n) are ratified ADRs and
  spikes, **not shipped**.
- **C FFI via purego is proposed** (ADR 0044, spiked), not landed. cgo-based
  Go modules work today; direct C FFI does not.
- **comptime** (Zig-style compile-time value execution) is on the roadmap,
  not implemented.
- **bri web apps don't AOT-compile yet.** The app framework's dev loop is
  `cljgo dev` / `cljgo test`; AOT compilation of bri apps lands with a later
  tier.

## FAQ

Questions people have actually asked, answered honestly.

**How is this different from Glojure and let-go?**
Full credit first: cljgo's runtime data structures started as a hard fork of
[Glojure](https://github.com/glojure/glojure)'s `pkg/lang` (vendored and
credited in the repo), and reading [let-go](https://github.com/nooga/let-go)
shaped several decisions. The difference is the model. Glojure is a tree-walk
interpreter whose compile story serializes an evaluated namespace, and whose
interop goes through a generated reflection registry; let-go compiles to
bytecode and bundles its VM into shipped binaries. cljgo is the ClojureScript
model with Go as the target: one analyzer, one AST, two consumers — a
tree-walk evaluator that is the REPL and macro engine, and an emitter that
compiles forms to plain Go source for `go build`. The compiled binary links
zero interpreter (CI checks the link set), and the output lives inside the Go
ecosystem — a normal Go program, a normal Go module.

**If the compiler can't handle something, does it fall back to
interpretation?**
No. Compilation compiles or fails — there is no interpreter in the binary to
fall back to. (Interop members the compiler can't type-infer yet fall back to
reflection *within compiled code*, with a compile-time note.) The interpreter
exists on the other side: it is the REPL and macro engine, and every
conformance test runs through both paths — REPL-vs-binary divergence is a
release blocker.

**Do I have to manually coerce types between Clojure and Go?**
No. Values crossing into Go coerce automatically — a Clojure vector passes
where a Go slice is expected, a Clojure fn passes as a typed Go callback —
using the same table in both the REPL and compiled code (Glojure's coercion
table is the acknowledged model). The compiled path goes one step further:
when the static type already matches, no coercion code is emitted at all —
it's a direct typed Go call. That is where the lower overhead comes from.

**Does a real REPL work with this approach? With persistent state?**
Yes — live re-`def`, `defmacro` at the prompt, nREPL for CIDER/Calva. Session
resume is form replay, not a heap snapshot: successful top-level forms are
saved (under `~/.config/cljgo/sessions`) and replayed on `--resume`. See
[the REPL guide](/cljgo/guides/repl/).

**How readable is the emitted Go?**
Honestly: not very, today. It is correct, `gofmt`-clean, and fast — but it is
compiler output, not idiomatic hand-written Go. Readability improves as more
of the emitter moves from boxed to typed emission; it is a non-goal to make it
look hand-written.

**What about Java interop — can existing Clojure libraries work via shims
like gojava?**
cljgo has no Java interop layer, by design — no JVM bytecode, no jars, ever.
The Go ecosystem is the standard library. That means Clojure libraries that
lean on Java classes don't run as-is today; pure-Clojure libraries do.
Projects like [gojava](https://github.com/gloathub/gojava) (shimming common
Java classes onto Go) are an interesting route for that gap and worth
watching, but cljgo doesn't use them yet.

**Can I publish one library to both ecosystems — Go and Clojure?**
Yes — this is a design goal, from one build description. `cljgo publish go`
emits a go-gettable Go module (type hints become real Go signatures), so Go
developers consume your Clojure library like any Go package. `cljgo publish
clojars` emits the same library as plain Clojure source with a git `deps.edn`
coordinate, so JVM Clojure consumes it with `:git/url` today (a real Clojars
coordinate is on the roadmap). The Clojure-side publish gates on Go-interop
purity with `file:line` errors, so you can't accidentally ship a lib the JVM
can't run. See [dependencies & publishing](/cljgo/guides/deps-publish/).

**Can one source file serve both JVM Clojure and cljgo?**
Yes. `.cljc` is an accepted source extension with a `:cljgo`
reader-conditional feature, so one file branches `#?(:clj … :cljgo …)` — the
same pure `.cljc` lib runs on the JVM and on cljgo, and publishes both ways as
above.

**Why not just GraalVM native-image?**
native-image is excellent and if your world is JVM libraries, use it. cljgo
answers a different question: what if the host ecosystem is Go? You get the Go
module universe as the standard library, `go build` compile times, trivial
cross-compilation (pure-Go programs build for any OS/arch with no target
toolchain), and no JVM anywhere in the toolchain. The trade is the one above —
no Java interop.

**How does it compare to babashka?**
Different tools. babashka is a mature interpreter with a huge curated library
set — for scripting it is the obvious choice. cljgo's interpreter is a
tree-walker and loses to bb; its compiled binaries are the point: on the
benchmark suite, compiled cljgo wins the recursion and data-structure rows
outright but honestly still loses `reduce` and `transducers` to babashka and
let-go. Numbers and methodology on the
[benchmarks page](/cljgo/reference/benchmarks/).

**Do macros work in compiled code? What about `eval`?**
Macros work fully — the tree-walk evaluator is the macro engine, so macros
(including your own) expand at compile time exactly as they do at the REPL.
What compiled binaries do not carry is runtime `eval`/`defmacro`-at-runtime:
zero interpreter is linked, by design. If you need runtime eval, that's what
the REPL side is for.

**Does core.async work?**
Yes, over real goroutines — `go` blocks are goroutines, channels are backed by
Go channels, no CPS rewrite. Same semantics interpreted and compiled. See
[concurrency & core.async](/cljgo/guides/concurrency/).

**Can I call C?**
Through Go, today: cgo-based Go modules (sqlite drivers, sensors, GUI/audio)
import like any other module. Direct C FFI without cgo (purego,
`dlopen`/`dlsym`, REPL-live) is designed and spiked but not landed yet.

**How big and how fast are the binaries?**
Hello-world compiles to a 5.3 MB static binary that starts in ~5 ms — dead
even with let-go's startup, no JVM, no runtime install. Emitted code currently
runs within ~5× of hand-written Go on the worst measured hot loop, and that
gap is CI-gated so it only shrinks. Full tables on the
[benchmarks page](/cljgo/reference/benchmarks/).

**Is it production-ready?**
No. It is 0.x and moving fast. The parts that are solid are guarded hard
(conformance against JVM 1.12.5, dual-harness REPL/binary parity, CI perf
gates); the honest gap list is the section right above this FAQ.

**Why build a fourth Clojure-on-Go?**
Personal, honestly: day-to-day work moved to Go, and this is one tool that
keeps Clojure — the real thing, verified against JVM 1.12.5, not a lookalike
— while shipping normal Go binaries.

---

The full picture is on [status & roadmap](/cljgo/reference/roadmap/). Ready to
try it? [Install](/cljgo/install/), then the
[quickstart](/cljgo/quickstart/).
