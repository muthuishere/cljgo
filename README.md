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
| **AOT core** | ✅ | Compiled binaries link the **compiled** `core.clj`, never the interpreter — `pkg/eval` 155 → **0** symbols in the link set; startup 27.5 → **5.5 ms** (ADR 0046) |
| Next | ◦ | `with-precision`, C FFI (purego), `alts!`/`timeout`, generics, lazy core namespaces (the last ~4 ms of startup), persistent-collection aliasing fix |

## Performance

Performance is priority 4 and gated like conformance, not asserted. Measured
on Apple M5 Pro, go1.26.3, with `hello.clj` = `(println "hi")`:

| | cljgo | reproduce |
|---|---|---|
| Tool binary | 8.3 MB stripped (12.1 MB plain) | `go build -trimpath -ldflags="-s -w" ./cmd/cljgo` |
| Compiled binary, hello | 4.6 MB | `cljgo build hello.clj` (strips by default) |
| Compiled startup, hello | 5.5 ms | `hyperfine -N ./hello` |
| Peak RSS, hello | 11.7 MB | `/usr/bin/time -l ./hello` |
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

**Where the startup goes.** 5.5 ms, of which ~1.5 ms is the floor for *any*
Go binary on this machine. The other ~4 ms is `rt.Boot()` building
`clojure.core` — now by running **compiled Go** (`pkg/coreaot`), not by
tree-walking `core.clj` on every start: ADR 0046 cut the
`main → rt.Boot → eval.New` edge, and a compiled binary no longer links the
interpreter at all (`pkg/eval` 155 → **0** symbols, `pkg/analyzer` 63 → 0,
`pkg/ast` 14 → 0 — CI-enforced, `pkg/coreaot/imports_test.go`). Startup went
27.5 → 5.5 ms and RSS 23.1 → 11.7 MB.

**What it did not do: binary size.** ADR 0023 §2 predicted ~2 MB once core
was AOT-compiled. Measured: 5.3 → 4.6 MB stripped — 14%, not 60%. The
tree-walker left, but ~13k lines of *compiled core* arrived, and what
dominates the remainder is the runtime a compiled binary genuinely needs:
`pkg/lang`'s data structures and numeric tower, `pkg/corelib`'s ~700 symbols,
and the reader (`read-string` is a real core fn). Size from here is a
dead-code problem in the runtime, not an AOT-core problem. That prediction is
superseded by this measurement, not still pending.

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
| startup | 5.5 ms | **4.7 ms** | 10.6 ms | 7.2 ms | 336.3 ms |
| `tak` | 901.0 ms | 1.23 s | 1.13 s | 12.33 s | **447.8 ms** |
| `fib` | 889.7 ms | 1.18 s | 1.16 s | 12.84 s | **449.7 ms** |
| `loop-recur` | 40.2 ms | 38.1 ms | 38.2 ms | 445.3 ms | 399.1 ms |
| `persistent-map` | **10.0 ms** | 13.1 ms | 13.8 ms | 30.4 ms | 378.1 ms |
| `map-filter` | **5.6 ms** | 6.3 ms | 11.0 ms | 9.9 ms | 330.4 ms |
| `transducers` | 18.9 ms | 26.0 ms | **13.3 ms** | — | 318.5 ms |
| `reduce` | 54.0 ms | 41.6 ms | **21.4 ms** | 1.48 s | 305.3 ms |
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
ships the smallest runtime in the field at 8.3 MB. With AOT core landed
(ADR 0046) it now also **wins `persistent-map` and `map-filter` outright** and
beats let-go on `transducers` — rows it used to lose 2.7× and 4.8×.

**The bad.** `reduce` is still 1.3× let-go and **2.5× babashka**, and
`transducers` 1.4× babashka. The `clojure.core`-routed rows improved a lot,
but the two runtimes with a purpose-built core still lead them.

**What changed.** ADR 0046 compiled `core.clj` and the 12 `.cljg` boot sources
through cljgo's own emitter (`pkg/coreaot`) and cut `rt.Boot() → eval.New()`:
a compiled binary now links the **compiled** core and no interpreter at all.
Measured on this machine, same benchmarks, before (ADR 0045 HEAD) → after:

| Benchmark | before | after | |
|---|---|---|---|
| startup | 27.5 ms | **5.5 ms** | 5.0× |
| `persistent-map` | 33.1 ms | **10.0 ms** | 3.3× |
| `map-filter` | 28.0 ms | **5.6 ms** | 5.0× |
| `transducers` | 62.8 ms | **18.9 ms** | 3.3× |
| `loop-recur` | 64.3 ms | **40.2 ms** | 1.6× |
| `reduce` | 82.3 ms | **54.0 ms** | 1.5× |
| `fib` / `tak` | 947 / 905 ms | 890 / 901 ms | ~1.0× |

**Read that honestly: these totals include startup, and ~22 ms of every row is
the startup delta.** On `persistent-map`, `map-filter` and `loop-recur` that is
essentially the whole win — the *work* was already native or already compiled;
what vanished was tree-walking `core.clj` on every start. `transducers` is the
row where compiled core shows up beyond startup (−44 ms against a −22 ms
startup delta), and `fib`/`tak` are flat because their work was always the
benchmark's own arithmetic. Big, real, and not evenly distributed.

The A/B that used to indict `cljgo build` has moved with it:

| | AOT binary | interpreted | speedup from compiling |
|---|---|---|---|
| `fib` — work in **user** code | 901.9 ms | 8770.8 ms | **9.72×** |
| `reduce` — work in **clojure.core** | 52.8 ms | 82.9 ms | **1.57×** |

That second row read **1.01×** before this change, and its meaning is what
moved: `clojure.core` is no longer interpreted in an emitted binary, so AOT and
interpreted are no longer indistinguishable. It is 1.57× rather than S23's
predicted ~5.8× because `reduce` itself is already native Go in **both** modes
(ADR 0045) — what compiling bought here is the boot and the core plumbing
around it, not the inner loop. The fns ADR 0045 did *not* hand-port are the
ones that gained the most (see `transducers`), which is exactly the argument
ADR 0037 made: emit core, don't hand-port it.

**What AOT core did not buy: size.** ADR 0023 §2 predicted ~2 MB. Measured
5.3 → 4.6 MB stripped. The tree-walker left the link set (`pkg/eval` 155 → 0
symbols) and ~13k lines of compiled core arrived; what remains is the runtime a
compiled binary genuinely needs (`pkg/lang`, `pkg/corelib`'s ~700 symbols, the
reader). Size from here is a dead-code problem in the runtime, not an AOT-core
problem — ADR 0046 records that prediction as superseded by measurement rather
than still pending.

**What is left.** `reduce`'s remaining gap and the `pkg/lang` costs S23
attributed to boxing / `IFn` dispatch / seq allocation (~4% of the original
gap, now a much larger share of what is left) are the doc 04 §5 performance
ladder — a separate lever. Startup has ~4 ms left above the 1.5 ms Go floor,
almost all of it `pkg/coreaot`'s eager `Load()` of all 13 boot namespaces;
making the satellites (clojure.test, cljgo.build, …) lazy through the provider
registry is the next obvious move, and it trades against interpreted/compiled
parity for `(all-ns)`, so it gets its own decision.

Spikes [S22](spikes/s22-aot-core-perf/VERDICT.md) and
[S23](spikes/s23-aot-core-prize/VERDICT.md) have the evidence that motivated
this; ADR 0037 carries the decision, ADR 0042 (multi-namespace emission),
ADR 0043 (`pkg/corelib`) and ADR 0046 (the cutover) are how it shipped.

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
