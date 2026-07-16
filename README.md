# cljgo

[![CI](https://github.com/muthuishere/cljgo/actions/workflows/ci.yml/badge.svg)](https://github.com/muthuishere/cljgo/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/muthuishere/cljgo?sort=semver&color=00a86b)](https://github.com/muthuishere/cljgo/releases/latest)
[![Go Reference](https://pkg.go.dev/badge/github.com/muthuishere/cljgo.svg)](https://pkg.go.dev/github.com/muthuishere/cljgo)
[![Go](https://img.shields.io/badge/go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Clojure](https://img.shields.io/badge/clojure-1.12.5-5881d8?logo=clojure&logoColor=white)](https://clojure.org)
[![clojure-test-suite](https://img.shields.io/badge/clojure--test--suite-217%2F242%20(89.7%25)-brightgreen)](#status)
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

Against the [jank clojure-test-suite](https://github.com/jank-lang/clojure-test-suite):
**217/242 files passing (89.7%)**, 90.4% of non-skipped, with 240/242 vars
resolved (99.2%). Run `cljgo suite` to reproduce. Early, moving fast.

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
| Tool binary | 8.5 MB stripped (12.5 MB plain) | `go build -trimpath -ldflags="-s -w" ./cmd/cljgo` |
| Compiled binary, hello | 5.2 MB | `cljgo build hello.clj` (strips by default) |
| Compiled startup, hello | 29.8 ms | `hyperfine -N ./hello` |
| Peak RSS, hello | 24.1 MB | `/usr/bin/time -l ./hello` |
| Interpreter boot | 23.7 ms · 29 MB · 472k allocs | `go test -bench=BenchmarkBoot -benchmem -run '^$' ./pkg/eval/` |
| clojure-test-suite | 217/242 (89.7%) | `cljgo suite` |

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

cljgo and let-go were both measured here on an M5 Pro. The rest of the field is
let-go's published M1 Pro data, so **every column is normalized to
let-go = 1.00×**, which is how let-go's own table reports it and which cancels
the hardware gap. That normalization is calibrated, not assumed: re-running
let-go here reproduced its published numbers at a consistent 1.39–1.85×
(median 1.72×) across all seven. **Lower is faster.**

| Benchmark | cljgo | let-go | babashka | joker | go-joker | gloat | fennel | JVM |
|---|---|---|---|---|---|---|---|---|
| `tak` | **0.74×** | 1.00× | 0.9× | — | 0.8× | 10.3× | 5.1× | 0.3× |
| `fib` | **0.82×** | 1.00× | 0.9× | 9.5× | 0.7× | 12.7× | 0.9× | 0.3× |
| `loop-recur` | 1.80× | 1.00× | 1.0× | 10.5× | 0.2× | 15.5× | 2.6× | 6.9× |
| `persistent-map` | 3.09× | 1.00× | 0.9× | 2.5× | 1.0× | 1.6× | 180× | 24.9× |
| `map-filter` | 5.98× | 1.00× | 2.4× | 1.6× | 1.8× | 8.8× | 141× | 49.6× |
| `transducers` | 6.56× | 1.00× | 0.6× | — | 0.4× | 4.3× | 36.4× | 8.3× |
| `reduce` | **16.54×** | 1.00× | 0.5× | 37.0× | 0.2× | 5.4× | 121× | 5.5× |
| startup | 6.08× | 1.00× | 2.2× | 1.4× | 1.5× | 1.8× | 5.2× | 43.9× |
| runtime size | **8.5 MB** | 12 MB | 68 MB | 26 MB | 32 MB | 26 MB | 324 KB | 304 MB |

Two honest reads of that table.

**The good.** On `tak` and `fib` cljgo is the fastest thing in the field except
the JVM — and against **gloat**, the only other Clojure→Go AOT compiler, it is
not close: 12.5× faster on `fib`, 13.9× on `tak`, 8.6× on `loop-recur`. The
"emit plain Go" bet works. cljgo also ships the smallest real runtime here
(8.5 MB; only Fennel's Lua VM is smaller, and it isn't Clojure).

**The bad.** We win exactly the two benchmarks where the *benchmark's own code*
does the arithmetic. Every benchmark that leans on `clojure.core` — reduce,
lazy seqs, transducers, persistent maps — we lose, and `reduce` we lose by
16.5× to let-go and 3.1× to gloat.

There is one cause, and it is the same `core.clj`-at-runtime coupling above:

| | AOT binary | interpreted | speedup from compiling |
|---|---|---|---|
| `fib` — work in **user** code | 993 ms | 9683 ms | **9.7×** |
| `reduce` — work in **clojure.core** | 701 ms | 700 ms | **1.00× — none** |

`cljgo build` compiles the user's forms and nothing else. Every `clojure.core`
function an emitted binary calls is still a **tree-walk interpreted closure**,
built by evaluating `core.clj` at boot — so `(reduce + 0 (range 1e6))` runs at
interpreter speed in a "compiled" binary, and a bytecode VM beats a tree-walker
at that. Compiling buys 9.7× where it applies; it applies to almost nothing in
a real Clojure program.

So AOT-compiling `core.clj` is not a binary-size cleanup with a startup bonus,
which is how ADR 0023 framed it. It is the **top performance lever in the
tree**, and it is the same edge that owns startup, RSS and size. Tracked as
spike S19/S20.

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
