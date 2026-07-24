---
title: FAQ
description: Questions people have actually asked about cljgo — Glojure/let-go differences, interop, the REPL, publishing to both ecosystems — answered honestly.
---


Questions people have actually asked — on Slack, Reddit, and elsewhere — answered honestly.

<details class="faq">
<summary>How is this different from Glojure and let-go?</summary>

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

</details>

<details class="faq">
<summary>If the compiler can't handle something, does it fall back to interpretation?</summary>

No. Compilation compiles or fails — there is no interpreter in the binary to
fall back to. (Interop members the compiler can't type-infer yet fall back to
reflection *within compiled code*, with a compile-time note.) The interpreter
exists on the other side: it is the REPL and macro engine, and every
conformance test runs through both paths — REPL-vs-binary divergence is a
release blocker.

</details>

<details class="faq">
<summary>Do I have to manually coerce types between Clojure and Go?</summary>

No. Values crossing into Go coerce automatically — a Clojure vector passes
where a Go slice is expected, a Clojure fn passes as a typed Go callback —
using the same table in both the REPL and compiled code (Glojure's coercion
table is the acknowledged model). The compiled path goes one step further:
when the static type already matches, no coercion code is emitted at all —
it's a direct typed Go call. That is where the lower overhead comes from.

</details>

<details class="faq">
<summary>Does a real REPL work with this approach? With persistent state?</summary>

Yes — live re-`def`, `defmacro` at the prompt, nREPL for CIDER/Calva. Session
resume is form replay, not a heap snapshot: successful top-level forms are
saved (under `~/.config/cljgo/sessions`) and replayed on `--resume`. See
[the REPL guide](/cljgo/guides/repl/).

</details>

<details class="faq">
<summary>How readable is the emitted Go?</summary>

Honestly: not very, today. It is correct, `gofmt`-clean, and fast — but it is
compiler output, not idiomatic hand-written Go. Readability improves as more
of the emitter moves from boxed to typed emission; it is a non-goal to make it
look hand-written.

</details>

<details class="faq">
<summary>What about Java interop — can existing Clojure libraries work via shims like gojava?</summary>

cljgo has no Java interop layer, by design — no JVM bytecode, no jars, ever.
The Go ecosystem is the standard library. That means Clojure libraries that
lean on Java classes don't run as-is today; pure-Clojure libraries do.
Projects like [gojava](https://github.com/gloathub/gojava) (shimming common
Java classes onto Go) are an interesting route for that gap and worth
watching, but cljgo doesn't use them yet.

</details>

<details class="faq">
<summary>Can I publish one library to both ecosystems — Go and Clojure?</summary>

Yes — this is a design goal, and both targets publish from the same
`build.cljgo`, no second manifest:

```
cljgo publish go        # a go-gettable Go module
cljgo publish clojars   # pure Clojure source for JVM-Clojure consumers
```

**Consuming from a Clojure project** — it's a git coordinate in `deps.edn`
today (a real Clojars coordinate is on the roadmap):

```clojure
{:deps {io.github.you/your-lib
        {:git/url "https://github.com/you/your-lib"
         :git/sha "abc123…"}}}
```

**Consuming from a Go project** — it's a normal Go module:

```
go get github.com/you/your-lib
```

(Wrappers currently expose `any` signatures; typed signatures from type hints
are a tracked follow-up.)

**When it fails, and how:** purity decides. A pure-Clojure library reaches
both worlds. The moment any reachable namespace uses Go interop
(`require-go`), the library is Go-side only — `publish clojars` **refuses and
names the offending file and line**, instead of shipping the JVM a broken
download. The check runs over the whole transitive required surface at
publish time. Full details in
[dependencies & publishing](/cljgo/guides/deps-publish/).

</details>

<details class="faq">
<summary>Can one source file serve both JVM Clojure and cljgo?</summary>

Yes. `.cljc` is an accepted source extension with a `:cljgo`
reader-conditional feature, so one file branches `#?(:clj … :cljgo …)` — the
same pure `.cljc` lib runs on the JVM and on cljgo, and publishes both ways as
above.

</details>

<details class="faq">
<summary>Why not just GraalVM native-image?</summary>

native-image is excellent and if your world is JVM libraries, use it. cljgo
answers a different question: what if the host ecosystem is Go? You get the Go
module universe as the standard library, `go build` compile times, trivial
cross-compilation (pure-Go programs build for any OS/arch with no target
toolchain), and no JVM anywhere in the toolchain. The trade is the one above —
no Java interop.

</details>

<details class="faq">
<summary>How does it compare to babashka?</summary>

Different tools. babashka is a mature interpreter with a huge curated library
set — for scripting it is the obvious choice. cljgo's interpreter is a
tree-walker and loses to bb; its compiled binaries are the point: on the
benchmark suite, compiled cljgo wins the recursion and data-structure rows
outright but honestly still loses `reduce` and `transducers` to babashka and
let-go. Numbers and methodology on the
[benchmarks page](/cljgo/reference/benchmarks/).

</details>

<details class="faq">
<summary>Do macros work in compiled code? What about <code>eval</code>?</summary>

Macros work fully — the tree-walk evaluator is the macro engine, so macros
(including your own) expand at compile time exactly as they do at the REPL.
What compiled binaries do not carry is runtime `eval`/`defmacro`-at-runtime:
zero interpreter is linked, by design. If you need runtime eval, that's what
the REPL side is for.

</details>

<details class="faq">
<summary>Does core.async work?</summary>

Yes, over real goroutines — `go` blocks are goroutines, channels are backed by
Go channels, no CPS rewrite. Same semantics interpreted and compiled. See
[concurrency & core.async](/cljgo/guides/concurrency/).

</details>

<details class="faq">
<summary>Can I call C?</summary>

Through Go, today: cgo-based Go modules (sqlite drivers, sensors, GUI/audio)
import like any other module. Direct C FFI without cgo (purego,
`dlopen`/`dlsym`, REPL-live) is designed and spiked but not landed yet.

</details>

<details class="faq">
<summary>How big and how fast are the binaries?</summary>

Hello-world compiles to a 5.3 MB static binary that starts in ~5 ms — dead
even with let-go's startup, no JVM, no runtime install. Emitted code currently
runs within ~5× of hand-written Go on the worst measured hot loop, and that
gap is CI-gated so it only shrinks. Full tables on the
[benchmarks page](/cljgo/reference/benchmarks/).

</details>

<details class="faq">
<summary>Is it production-ready?</summary>

No. It is 0.x and moving fast. The parts that are solid are guarded hard
(conformance against JVM 1.12.5, dual-harness REPL/binary parity, CI perf
gates); the honest gap list is the section right above this FAQ.

</details>

<details class="faq">
<summary>Why build a fourth Clojure-on-Go?</summary>

Personal, honestly: day-to-day work moved to Go, and this is one tool that
keeps Clojure — the real thing, verified against JVM 1.12.5, not a lookalike
— while shipping normal Go binaries.

</details>
