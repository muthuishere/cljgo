# cljgo

[![CI](https://github.com/muthuishere/cljgo/actions/workflows/ci.yml/badge.svg)](https://github.com/muthuishere/cljgo/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/muthuishere/cljgo?sort=semver&color=00a86b)](https://github.com/muthuishere/cljgo/releases/latest)
[![Go Reference](https://pkg.go.dev/badge/github.com/muthuishere/cljgo.svg)](https://pkg.go.dev/github.com/muthuishere/cljgo)
[![Go](https://img.shields.io/badge/go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Clojure](https://img.shields.io/badge/clojure-1.12.5-5881d8?logo=clojure&logoColor=white)](https://clojure.org)
[![clojure-test-suite](https://img.shields.io/badge/clojure--test--suite-238%2F242%20(98.3%25)-brightgreen)](#status)
[![License](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

**[muthuishere.github.io/cljgo](https://muthuishere.github.io/cljgo/)** — docs, examples, and the live status board.

Clojure hosted on Go: a compiler (written in Go) that AOT-emits plain Go
source — the ClojureScript model with Go as the JavaScript — plus a
tree-walk evaluator that is the REPL and the macro engine.

## Install

```bash
go install github.com/muthuishere/cljgo/cmd/cljgo@latest
```

Or grab a prebuilt binary for your platform from
[the latest release](https://github.com/muthuishere/cljgo/releases/latest)
(macOS/Linux/Windows, amd64 + arm64).

`cljgo repl`, `cljgo run` and Go interop work from the binary alone, with no Go
toolchain installed. **`cljgo build` additionally needs the Go toolchain.**
From v0.2.0, that is the whole story: a release binary pins the published
runtime module in the generated go.mod
(`require github.com/muthuishere/cljgo v<version>`, ADR 0028), and the first
build fetches it from the Go module proxy once per machine (~1 MB, a few
seconds).

v0.1.0 binaries predate that and still need a checkout of this repo — their
generated module `replace`s the runtime to a local source tree, so point
`CLJGO_SRC` at your clone or run inside it:

```bash
git clone https://github.com/muthuishere/cljgo && export CLJGO_SRC=$PWD/cljgo
```

```
$ cljgo --version
cljgo CLI version 0.1.0 (Go 1.26.3, Clojure 1.12.5)

$ cljgo repl
cljgo 0.1.0 (Go 1.26.3, Clojure 1.12.5)
user=> (clojure-version)
"1.12.5"
```

## Priorities

1. **Universal interop** — any Go module importable and callable with zero
   bindings; the Go ecosystem is the standard library. C via cgo modules and
   purego FFI.
2. **Full REPL-driven development** — live re-`def`, `defmacro` at the
   prompt, nREPL for CIDER/Calva.
3. **Faithful Clojure principles** — persistent data structures, macros,
   seqs, vars.
4. **High performance in both modes** — a feature, not an option.
5. **cgo builds are first-class** — `CGO_ENABLED=1` projects are supported,
   not tolerated.

## Status

Working REPL **and** native compiler. The same source runs interpreted at the
prompt and AOT-compiles to a static Go binary — byte-identical output on both
paths (a dual-harness conformance suite enforces this on every commit; a
REPL↔binary divergence is a release blocker).

Against the [jank clojure-test-suite](https://github.com/jank-lang/clojure-test-suite)
(upstream @164a4b3, unmodified): **238/242 files passing (98.3%)**, with 242/242
vars resolved (100%), 0 failures and 4 errors. Run `cljgo suite` to reproduce.

Those 4 are dialect registration, not broken semantics. `abs`, `add-watch`,
`short` and `reduce` carry reader conditionals with **no `:default`** branch
(e.g. `#?(:cljr System.Int16 :clj java.lang.Short)`), so a runtime the suite has
never heard of reads them as nothing — `(instance? (short 0))` then fails with
"wrong number of args (1)". Adding a `:cljgo` branch is the same mechanism
`:cljr` / `:lpy` / `:phel` already use, and cljgo's spellings are truthful
(`(instance? java.lang.Short (short 0))` is genuinely `true` here, as on the
JVM). With those four branches applied the suite reads **242/242 (100%)** — but
they are **not upstreamed yet**, so the number published above is the one you
get from the suite as it ships. Early, moving fast.

| Milestone | State | What landed |
|-----------|-------|-------------|
| **M0** | ✅ | REPL: reader (full syntax-quote), `loop*`/`recur`, dynamic vars, namespaces |
| **M1** | ✅ | Macroexpansion, `defmacro` at the prompt, embedded `core.clj`, `clojure.test` |
| **M2** | ✅ | `cljgo build` → native binary, fixed-arity calling convention ([perf](#performance)) |
| **M3-v0** | ✅ | **Zero-ceremony Go interop, both modes** — `require-go`, package fns/consts, `(T,error)`→`[v err]`, `!` unwrap-or-throw |
| **M3.1/3.2** | ✅ | Members: `(.Method r …)`, `(.-Field r)`, `(set! (.-Field r) v)`, ctors `(pkg/T. {…})`, `(go/new T)` |
| **M4-v0** | ✅ | Concurrency: `(chan)`/`(chan n)`, `(>! c v)`/`(<! c)`, `(close! c)`, `(go …)` over **real goroutines** — no CPS rewrite |
| **Result/Option** | ✅ | `ok`/`err`/`just`/`none` + `unwrap`/`and-then`/`map-ok` + `let?`, `#cljgo/ok` literals (ADR 0014) |
| **Diagnostics** | ✅ | `cljgo check --json` structured records, `cljgo explain <code>` (ADR 0015) |
| **nREPL** | ✅ | `cljgo nrepl` — babashka's 13-op surface, per-session `*ns*`/`*1`/`*out*` streaming, `.nrepl-port`, `doc` (ADR 0031) |
| **nREPL** | ✅ | `cljgo nrepl` — Calva/CIDER connect; 13-op surface, sessions on goroutine-keyed bindings (ADR 0031) |
| Next | ◦ | `with-precision`, C FFI (purego), `alts!`/`timeout`, generics, AOT `core.clj` (binary size), persistent-collection aliasing fix |

## Performance

Performance is priority 4 and gated like conformance, not asserted. Measured
on Apple M5 Pro, go1.26.3, with `hello.clj` = `(println "hi")`:

| | cljgo | reproduce |
|---|---|---|
| Tool binary | 8.3 MB stripped (12.1 MB plain) | `go build -trimpath -ldflags="-s -w" ./cmd/cljgo` |
| Compiled binary, hello | 5.1 MB | `cljgo build hello.clj` (strips by default) |
| Compiled startup, hello | 28.9 ms | `hyperfine -N ./hello` |
| Peak RSS, hello | 23.4 MB | `/usr/bin/time -l ./hello` |
| Interpreter boot | 22.3 ms · 28.5 MB · 459k allocs | `go test -bench=BenchmarkBoot -benchmem -run '^$' ./pkg/eval/` |
| clojure-test-suite | 238/242 (98.3%) | `cljgo suite` |

Two budgets run inside plain `go test ./...`, and are host-relative because a
CI runner is not your laptop (ADR 0024) — override with `CLJGO_BOOT_BUDGET`
and `CLJGO_PERF_RATIO_MAX`:

- **Interpreter boot** — `TestBootUnderBudget`, 250 ms locally (`pkg/eval/boot_test.go`).
- **Emitted vs handwritten Go** — `TestFactorialPerfBudget`, 60× ceiling
  (`pkg/emit/perf_test.go`).

**Where we actually stand on those two.** Emitted factorial runs at ~35×
handwritten Go today; naive emission was ~168×, and the §1.4 target is ~10×
via the doc 04 performance ladder. The 60× gate is a regression guard against
sliding back to naive emission — it is not the budget, and the gap to ~10× is
open work.

**Where the startup goes.** ~28 of those 30 ms are `core.clj` booting at
runtime. An emitted binary today links the entire interpreter, because
`main → rt.Boot → eval.New` loads core.clj on start (ADR 0023) — an empty Go
binary starts in 2.0 ms on the same machine, and the M2-era "2.3 ms startup"
spike number predates that edge. AOT-compiling `core.clj` cuts it, and is the
single biggest lever in the tree: it takes startup, RSS **and** binary size
(→ ~2 MB, roughly the raw-Go static baseline) in one move. It is the top item
on the roadmap for exactly that reason.

### Head-to-head vs let-go

[let-go](https://github.com/nooga/let-go) (v1.11.1) is the closest comparable —
Clojure on Go, but a bytecode VM rather than AOT-to-Go-source. Both built from
source on the same machine with the same toolchain and the same
`-trimpath -ldflags="-s -w"`, so this is not a spec-sheet comparison:

Run on **let-go's own benchmark suite**, unmodified, with let-go's published
methodology (hyperfine, 3 warmup / 10 runs). All 7 files compile and run on
cljgo with no edits.

Every runtime below was **installed and measured on the same machine** (Apple
M5 Pro, go1.26.3) — no normalization, no quoted numbers, wall-clock mean of 10
runs. Totals include each runtime's startup. Best per row in bold.

| Benchmark | cljgo | let-go | babashka | joker | clojure JVM |
|---|---|---|---|---|---|
| startup | 28.9 ms | **4.7 ms** | 10.6 ms | 7.2 ms | 336.3 ms |
| `tak` | 889.9 ms | 1.23 s | 1.13 s | 12.33 s | **447.8 ms** |
| `fib` | 982.3 ms | 1.18 s | 1.16 s | 12.84 s | **449.7 ms** |
| `loop-recur` | 67.9 ms | **38.1 ms** | 38.2 ms | 445.3 ms | 399.1 ms |
| `persistent-map` | 36.0 ms | **13.1 ms** | 13.8 ms | 30.4 ms | 378.1 ms |
| `map-filter` | 30.1 ms | **6.3 ms** | 11.0 ms | 9.9 ms | 330.4 ms |
| `transducers` | 64.2 ms | 26.0 ms | **13.3 ms** | — | 318.5 ms |
| `reduce` | 82.3 ms | 41.6 ms | **21.4 ms** | 1.48 s | 305.3 ms |
| runtime size | **8.3 MB** | 12.2 MB | 67.9 MB | 27.4 MB | 385.0 MB |

Versions: cljgo @HEAD (post-ADR-0045), let-go v1.11.1 (tag, built from source),
babashka v1.12.218, joker v1.9.0, Clojure CLI 1.12.5.1645 on OpenJDK 26.0.1.
`joker` has no `transducers`. Runtime size is the stripped binary for cljgo /
let-go, the installed binary for babashka / joker, and JDK + `clojure.jar`
(381.0 + 4.0 MB) for the JVM. Re-measured 2026-07-17; every cell above was run
on this machine, and all seven benchmarks produce identical values across all
five runtimes (`persistent-map` differs only in hash-map print order, so it is
compared on `[count, sum-keys, sum-vals, (get m 9999)]`).
Not measured: **gloat** (its module exposes no installable package path) and
**go-joker** (needs a source clone + codegen) — let-go's published M1 Pro data
puts gloat at 12.7× let-go on `fib` and 5.4× on `reduce`.

Two honest reads of that table.

**The good.** On `tak` and `fib` cljgo is the fastest thing here except the
JVM — ahead of both a bytecode VM (let-go) and a GraalVM native image
(babashka), and **13.1× ahead of joker**, the other Go tree-walker. cljgo also
ships the smallest runtime in the field at 8.3 MB. The "emit plain Go" bet
works.

**The bad.** Every row that leans on `clojure.core` still loses. `reduce` is
2.0× let-go and **3.8× babashka**; `map-filter` is 4.8× let-go; `transducers`
4.8× babashka; `loop-recur` and `persistent-map` roughly 1.8× and 2.7×. We win
exactly the two benchmarks where the *benchmark's own code* does the
arithmetic.

**What changed, and what didn't.** ADR 0045 moved `reduce`/`map`/`filter`/
`mapv`/`comp` — the five fns whose per-element cost dominates these workloads —
into native Go. `reduce` went 719 ms → **82 ms** and `transducers` 172 ms →
**64 ms**, which closed the worst gap from 15.8× to 2.0× and moved cljgo off
joker's shoulder: on `reduce` we are now **18× ahead** of the other Go
tree-walker, not sitting next to it. That was the single largest perf move in
the tree to date.

But it treated the symptom on five fns, not the cause. The A/B below is the
same shape it was before:

| | AOT binary | interpreted | speedup from compiling |
|---|---|---|---|
| `fib` — work in **user** code | 979.7 ms | 8877.9 ms | **9.06×** |
| `reduce` — work in **clojure.core** | 80.0 ms | 80.7 ms | **1.01× — none** |

Read that second row carefully, because its *meaning* inverted while its value
did not. It used to read 1.00× because `reduce` was **interpreted in both
modes**. It now reads 1.01× because `reduce` is **native Go in both modes** —
same ratio, opposite cause. What it still proves is that `cljgo build` did not
compile it: AOT and interpreted are indistinguishable because neither path
emits `clojure.core`.

And the ~292 core fns that did *not* go native are still interpreted closures in
a "compiled" binary. Every remaining loss above is that.

`cljgo build` compiles the user's forms and nothing else. Apart from the ~300
fns that are native Go (the ~292 long-standing builtins plus ADR 0045's five),
every `clojure.core` function an emitted binary calls is still a **tree-walk
interpreted closure**, built by evaluating `core.clj` at boot — and a bytecode
VM beats a tree-walker at that. Compiling buys ~9× where it applies; it still
applies to almost nothing in a real Clojure program.

Which is why ADR 0045 is a floor, not a fix. Hand-porting core fns to Go one at
a time does not scale to `clojure.core`, and each port is a fresh chance to
drift from Clojure semantics — the ADR 0045 review caught exactly that, a
`next`-for-`rest` slip that made `map` and `filter` realize one element too
many. The scalable answer is to **emit `core.clj`** like any other namespace, so
every core fn is compiled Go and none of it is hand-maintained (ADR 0037; the
gating prerequisite, multi-namespace emission, is ADR 0042).

So AOT-compiling `core.clj` is not a binary-size cleanup with a startup bonus,
which is how ADR 0023 framed it. It is the **top performance lever in the
tree**, and it is the same edge that owns startup, RSS and size.

How much it buys is measured, not assumed (spike S23). Compiling `reduce`'s
algorithm instead of interpreting it is **5.83×**; with no interpreted core in
the hot loop at all, `reduce` goes from 674 ms to 96 ms — closing **~86%** of
the gap:

| cause of the 16.5× `reduce` gap | share | fix |
|---|---|---|
| `clojure.core` interpreted | ~86% | AOT-core |
| `core.clj` boot | ~5% | same edge |
| `pkg/lang` — boxing, `IFn` dispatch, seqs | ~4% | doc 04 §5 ladder |

That still lands at ~2.26× of let-go, **not parity** — a 7× improvement that
converts a catastrophic loss into a respectable one. Parity needs the
performance ladder as well. And it is a milestone, not a patch: multi-namespace
emission doesn't exist yet and is a hard prerequisite, and the linker win is
all-or-nothing (a half-migrated core still links the interpreter and measures
as zero).

Spikes [S22](spikes/s22-aot-core-perf/VERDICT.md) and
[S23](spikes/s23-aot-core-prize/VERDICT.md) have the full evidence; ADR 0037
carries the decision (proposed), and ADR 0045 took the interim step of moving
the five hottest fns to native Go.

Boot got 8.9× faster in v0.2.0 (211 ms → 23.7 ms) by replacing a
stack-scraping goroutine-ID lookup that was burning 73% of boot CPU with a
`getg()`-based one (ADR 0034, spike S18). `.github/workflows/boot-bench.yml`
is a manual (`workflow_dispatch`) ubuntu-vs-macos boot comparison kept as a
permanent diagnostic.

### Try it

```clojure
;; hello.clj
(require-go '[strings])
(require-go '[strconv])

(println (strings/ToUpper "hello from go's stdlib"))
(println "Atoi! ->" (strconv/Atoi! "42"))   ; unwraps, or throws
(println "Atoi  ->" (strconv/Atoi "oops"))   ; => [0 <error>], errors-as-values
```

```
$ cljgo run hello.clj        # interpreted
$ cljgo build hello.clj      # -> ./hello, a static native binary
$ ./hello                    # byte-identical output
```

The Go ecosystem is the standard library: `(require-go '[net/http :as http])`
and call it — no bindings, no wrappers, the Go toolchain is the classpath.

Editor REPL: `cljgo nrepl`, then connect Calva ("Connect to a running
REPL") or CIDER (`cider-connect-clj`) to the printed port — `.nrepl-port`
makes it auto-discoverable.

## Development

Authority chain: `docs/adr/` (decisions) › `design/00-architecture.md`
(contracts + M0–M5 roadmap) › `design/01–07` (component internals) ›
`openspec/` (active change proposals). Process for non-trivial work:
ADR → OpenSpec propose/design → apply.

Gates before every commit:

```
go build ./... && go vet ./... && gofmt -l pkg cmd conformance && go test ./...
```

```
pkg/lang     runtime (persistent data structures, vendored from Glojure)
pkg/reader   pkg/ast   pkg/analyzer   pkg/eval   pkg/repl   pkg/emit
cmd/cljgo    CLI (repl · nrepl · run · build · version)
core/        core.clj — Clojure-in-Clojure
conformance/ dual-harness tests (eval + compiled), oracle-cited vs JVM Clojure
design/      architecture + component design docs
docs/adr/    decision log        openspec/   spec-driven change proposals
```

Toolchain: Go 1.26.

## Credits

cljgo stands on work by people who solved the hard parts first.

- **[Clojure](https://github.com/clojure/clojure)** — Rich Hickey and
  contributors. The language, and cljgo's specification: every semantic
  behavior in `conformance/` is verified against real JVM Clojure as the
  oracle, and the Java source (`LispReader.java`, `Compiler.java`,
  `PersistentVector.java`, `PersistentHashMap.java`) is the reference the
  reader, analyzer and data structures were built from.
- **[Glojure](https://github.com/glojurelang/glojure)** — the runtime under
  `pkg/lang` is a hard fork of Glojure's persistent data structures, seqs,
  symbols, keywords and vars (v0.6.8). Roughly 17k lines that would have
  taken months to write from scratch. It stays EPL-1.0; our surgery on it is
  logged in `pkg/lang/PROVENANCE.md`.
- **[Elvish](https://github.com/elves/elvish)** — the persistent vector in
  `pkg/lang/internal/persistent/vector` is a port from the Elvish shell.
- **[cljs2go](https://github.com/hraberg/cljs2go)** — Håkan Råberg's 2015
  Clojure→Go experiment. Read as reference for the emitter's per-op emission
  strategy and AFn machinery; proof the reader→analyzer→emitter split works
  with Go as a target. No code taken.
- **[let-go](https://github.com/nooga/let-go)** — reference for treating Go
  channels and goroutines as first-class Clojure concurrency rather than
  reimplementing core.async's CPS transform. No code taken.
- **[ClojureScript](https://github.com/clojure/clojurescript)** — the model
  this project follows: a compiler that emits host source, with the AST "op"
  vocabulary cljgo's analyzer keeps.

## License

- **cljgo's own code** — MIT (see [LICENSE](LICENSE)).
- **`pkg/lang/`** — Eclipse Public License 1.0, as vendored from Glojure. The
  MIT grant does not extend to it.

[NOTICE](NOTICE) has the full breakdown of which license covers what.
