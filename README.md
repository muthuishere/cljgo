# cljgo

[![CI](https://github.com/muthuishere/cljgo/actions/workflows/ci.yml/badge.svg)](https://github.com/muthuishere/cljgo/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/muthuishere/cljgo?sort=semver&color=00a86b)](https://github.com/muthuishere/cljgo/releases/latest)
[![Go Reference](https://pkg.go.dev/badge/github.com/muthuishere/cljgo.svg)](https://pkg.go.dev/github.com/muthuishere/cljgo)
[![Go](https://img.shields.io/badge/go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Clojure](https://img.shields.io/badge/clojure-1.12.5-5881d8?logo=clojure&logoColor=white)](https://clojure.org)
[![core.async](https://img.shields.io/badge/core.async-1.6.681-5881d8?logo=clojure&logoColor=white)](https://github.com/clojure/core.async)
[![clojure-test-suite](https://img.shields.io/badge/clojure--test--suite-238%2F242%20(98.3%25)-brightgreen)](#status)
[![License](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

**[muthuishere.github.io/cljgo](https://muthuishere.github.io/cljgo/)** — docs, examples, and the live status board.

Clojure hosted on Go: a compiler (written in Go) that AOT-emits plain Go
source — the ClojureScript model with Go as the JavaScript — plus a
tree-walk evaluator that is the REPL and the macro engine. The same source
runs interpreted at the prompt and compiles to a single static native
binary, with byte-identical output on both paths.

## Why

1. **Universal interop** — any Go module importable and callable with zero
   bindings; the Go ecosystem is the standard library. C via cgo modules and
   purego FFI.
2. **Full REPL-driven development** — live re-`def`, `defmacro` at the
   prompt, nREPL for CIDER/Calva.
3. **Faithful Clojure principles** — persistent data structures, macros,
   seqs, vars. Clojure is first-class: nothing cljgo adds may shadow or
   change clojure.core semantics.
4. **High performance in both modes** — a feature, gated in CI like tests,
   not asserted.
5. **Single-file deployment** — `cljgo build` produces one static binary
   (5.3 MB for hello, ~5 ms startup), no JVM, no runtime install.

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
seconds). (v0.1.0 binaries predate that and need a repo checkout via
`CLJGO_SRC`.)

## Quickstart

```clojure
;; hello.clj
(require-go '[strings])
(require-go '[strconv])

(println (strings/ToUpper "hello from go's stdlib"))
(println "Atoi! ->" (strconv/Atoi! "42"))   ; unwraps, or throws
(println "Atoi  ->" (strconv/Atoi "oops"))  ; => [0 <error>], errors-as-values
```

```
$ cljgo run hello.clj        # interpreted
$ cljgo build hello.clj      # -> ./hello, a static native binary
$ ./hello                    # byte-identical output
```

The Go ecosystem is the standard library: `(require-go '[net/http :as http])`
and call it — no bindings, no wrappers, the Go toolchain is the classpath.

Projects, dependencies, publishing:

```
$ cljgo new myapp            # generate a project: --template lib (default) | cli | web
$ cljgo test                 # run the project's tests (clojure.test)
$ cljgo build                # resolve declared deps (build.cljgo + build.lock.edn), compile
$ cljgo publish go           # publish the library to the Go module ecosystem — or `clojars`
```

Sources may be `.clj`, `.cljg`, or `.cljgo` (ADR 0055); dependency
resolution and lockfiles are ADR 0052/0053, publishing to both ecosystems is
ADR 0054.

Editor REPL: `cljgo nrepl`, then connect Calva ("Connect to a running
REPL") or CIDER (`cider-connect-clj`) to the printed port — `.nrepl-port`
makes it auto-discoverable.

Errors carry registered codes with explain pages: `cljgo check file.clj
--json` for structured diagnostics, `cljgo explain A2004` for the long-form
page (ADRs 0015/0048).

## Status

Working REPL **and** native compiler. The same source runs interpreted at the
prompt and AOT-compiles to a static Go binary — byte-identical output on both
paths (a dual-harness conformance suite, 416 oracle-cited files, enforces
this on every commit; a REPL↔binary divergence is a release blocker).

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
get from the suite as it ships (analysis: `docs/suite-upstream.md`).

| Area | State | What landed |
|-----------|-------|-------------|
| **Language core (M0–M2)** | ✅ | Reader (full syntax-quote, tagged literals, reader conditionals — ADRs 0036/0050), macros, `defmacro` at the prompt, protocols/`deftype`/`defrecord`/`reify` (ADRs 0020/0049), numeric tower (0029/0032), JVM-compatible hashing (0051), `cljgo build` → native binary |
| **Go interop (M3)** | ✅ | Zero-ceremony, both modes — `require-go`, package fns/consts, `(T,error)`→`[v err]`, `!` unwrap-or-throw, members `(.Method r …)` / `(.-Field r)` / `set!`, ctors `(pkg/T. {…})`, `(go/new T)` |
| **core.async** | ✅ | Over **real goroutines** — no CPS rewrite. 55 publics = every non-deprecated, non-internal var of JVM core.async 1.6.681, including `alts!`/`alt!`, `timeout`, transducers, `mult`/`pub`/`mix`/`pipe`, `pipeline`(-`blocking`/-`async`) (ADR 0040; audit: `docs/core-async-audit-2026-07.md`) |
| **Satellite namespaces** | ✅ | `clojure.string` · `set` · `edn` · `walk` · `zip` · `data` · `repl` · `pprint` complete against the 1.12.5 oracle; `clojure.test` complete (39 oracle vars) |
| **Result/Option** | ✅ | `ok`/`err`/`just`/`none` + `unwrap`/`and-then`/`map-ok` + `let?`, `#cljgo/ok` literals (ADR 0014) |
| **Diagnostics** | ✅ | Banded error codes + explain pages, `cljgo check --json`, `cljgo explain <code>` (ADRs 0015/0048) |
| **nREPL** | ✅ | `cljgo nrepl` — Calva/CIDER connect; babashka's 13-op surface, per-session `*ns*`/`*1`/`*out*` streaming, `.nrepl-port` (ADR 0031) |
| **Projects & toolkit** | ✅ | `cljgo new` (real runnable templates: lib/cli/web, ADR 0047), dependency resolution + lockfile (`build.cljgo`/`build.lock.edn`, ADRs 0052/0053), `cljgo publish go\|clojars` (0054), `.clj`/`.cljg`/`.cljgo` (0055); app scaffolding `cljgo dev`/`config`/`routes` (ADR 0041 T0/T1) |
| **AOT core** | ✅ | Compiled binaries link the **compiled** `core.clj`, never the interpreter — `pkg/eval` 155 → **0** symbols in the link set (ADR 0046) |
| **Perf campaign** | ✅ | ADRs 0063–0067: chunk-aware seq ops, IFn2 reduce seam, direct-call emission, sealed-core guard elision, int64 numeric inference — emitted factorial ~35× → **4.8×** handwritten Go ([details](#performance)) |
| Next | ◦ | ADR 0067 follow-ups (float64, multi-arity/variadic specialization, capturing-closure lift); `reduce`/`transducers` vs babashka's core (the two rows still lost); app framework T2 (ADR 0041); C FFI purego (ADR 0044, proposed, spike S21); batteries direction (ADRs 0056–0062, ratified on `feat/batteries` — decisions recorded, **not shipped**) |

`clojure.core` is **complete** as of 2026-07-23: **632 of 679** oracle vars
(93%) implemented with oracle-exact, conformance-frozen semantics; the other
47 are JVM bytecode/classloader machinery (`proxy`, `gen-class`,
`java.util.stream`, compiler knobs) each documented with a one-line reason —
no third bucket. Every satellite namespace (string/set/edn/walk/zip/data/
repl/pprint/test) is 100%. The per-var ledger is
`docs/fundamentals-audit-2026-07.md`.

## Performance

Performance is priority 4 and gated like conformance, not asserted. Measured
2026-07-23 on Apple M5 Pro, go1.26.3, cljgo @HEAD, with `hello.clj` =
`(println "hi")`:

| | cljgo | reproduce |
|---|---|---|
| Tool binary | 12.7 MB stripped | `go build -trimpath -ldflags="-s -w" ./cmd/cljgo` |
| Compiled binary, hello | 5.3 MB | `cljgo build hello.clj` (strips by default) |
| Compiled startup, hello | 5.0 ms | `hyperfine -N ./hello` |
| Peak RSS, hello | 14.7 MB | `/usr/bin/time -l ./hello` |
| Interpreter boot | 31.7 ms · 44.5 MB · 733k allocs | `go test -bench BenchmarkBoot -benchmem ./pkg/eval/` |
| clojure-test-suite | 238/242 (98.3%) | `cljgo suite` |

**Building an AOT binary is one command** — `cljgo build -o hello hello.clj`.
It emits Go and invokes `go build`, so it needs the Go toolchain on `PATH`
(`cljgo run` / `cljgo repl` do not); it strips by default; and it links the
**compiled** core, never the interpreter (ADR 0046), which is why a compiled
binary starts in ~10 ms instead of the interpreter's ~32 ms boot.

Two budgets run inside plain `go test ./...`, and are host-relative because a
CI runner is not your laptop (ADR 0024) — override with `CLJGO_BOOT_BUDGET`
and `CLJGO_PERF_RATIO_MAX`:

- **Interpreter boot** — `TestBootUnderBudget`, 250 ms locally (`pkg/eval/boot_test.go`).
- **Emitted vs handwritten Go** — `TestFactorialPerfBudget`, 15× ceiling
  (`pkg/emit/perf_test.go`). Measured: **~4.8×** — under the ~10× target of
  design/00 §1.4 for the first time (ADR 0067; naive emission was ~168×,
  boxed emission ~35×). The 15× gate is a regression guard, not the target.

### The 2026-07-23 campaign — ADRs 0063–0067

Five decisions moved emitted code from "correct but boxed" to competitive:
chunk-aware `map`/`filter`/`count`/`keep` (JVM 32-element realization
parity, ADR 0063), the IFn2 2-arg seam (no `[]any` box per reduce step),
direct-call emission for statically-known local fns (0064), the sealed-core
dirty-flag (guard elision with full `with-redefs` liveness, 0066), and an
int64 numeric-inference pass that lifts monomorphic kernels to raw typed Go
(`func tak(x, y, z int64) int64`, 0067). Numbers below.

### Head-to-head vs let-go

[let-go](https://github.com/nooga/let-go) (v1.11.1) is the closest comparable —
Clojure on Go, but a bytecode VM rather than AOT-to-Go-source. Both built from
source on the same machine with the same toolchain and the same
`-trimpath -ldflags="-s -w"`, so this is not a spec-sheet comparison:

Run on **let-go's own benchmark suite**, unmodified, with let-go's published
methodology (hyperfine, 3 warmup / 10 runs). All 7 files compile and run on
cljgo with no edits.

Reproduce it yourself with **`bash benchmark/run.sh`** — the committed harness
(corpus + runner + report). It reports **both cljgo legs**, interpreted
(`cljgo run`) and AOT (`cljgo build`), the way let-go now reports its VM against
its own AOT; the `cljgo` column below is the AOT leg, which is what you ship.
See [`benchmark/README.md`](benchmark/README.md) for methodology, binary sizes,
how AOT binaries are built, and the `reduce` gap analysis.

Every runtime below was **installed and measured on the same machine** (Apple
M5 Pro, go1.26.3) — no normalization, no quoted numbers, wall-clock mean of 10
runs. Totals include each runtime's startup. Best per row in bold.

`cljgo run` is the interpreter, `cljgo` is the AOT binary (`cljgo build`) —
what you ship, and the column to read against the field.

| Benchmark | cljgo run | cljgo | let-go | babashka | joker | clojure JVM |
|---|---|---|---|---|---|---|
| startup | 38.5 ms | **5.0 ms** | **5.0 ms** | 9.7 ms | 6.7 ms | 289.2 ms |
| `tak` | 11.47 s | **34.6 ms** | 1.33 s | 1.14 s | — | 457.9 ms |
| `fib` | 8.79 s | **24.7 ms** | 1.25 s | 1.14 s | — | 419.1 ms |
| `loop-recur` | 469.3 ms | **5.4 ms** | 36.6 ms | 37.8 ms | 437.8 ms | 397.5 ms |
| `persistent-map` | 48.5 ms | **9.4 ms** | 12.9 ms | 13.0 ms | 30.5 ms | 385.2 ms |
| `map-filter` | 39.9 ms | 5.1 ms | **4.8 ms** | 10.0 ms | 8.4 ms | 311.5 ms |
| `transducers` | 89.0 ms | 16.4 ms | 25.4 ms | **13.0 ms** | — | 315.9 ms |
| `reduce` | 61.4 ms | 26.0 ms | 22.8 ms | **20.0 ms** | 1.46 s | 302.5 ms |
| runtime size | — | **12.7 MB** | 13 MB | 67.9 MB | 27.4 MB | 385.0 MB |

Versions: cljgo @HEAD (post-ADR-0067), let-go v1.11.1 (tag, built from source),
babashka v1.12.218, joker v1.9.0, Clojure CLI 1.12.5.1645 on OpenJDK 26.0.1.
`joker` has no `transducers` and is skipped on `fib`/`tak` (~13× slower there).
Runtime size is the stripped binary for cljgo / let-go, the installed binary for
babashka / joker, and JDK + `clojure.jar` (381.0 + 4.0 MB) for the JVM; a
*compiled cljgo program* is 5.3 MB. Re-measured 2026-07-23 via
`bash benchmark/run.sh`; every cell above was run
on this machine, and all seven benchmarks produce identical values across all
five runtimes (`persistent-map` differs only in hash-map print order, so it is
compared on `[count, sum-keys, sum-vals, (get m 9999)]`).
Not measured: **gloat** (its module exposes no installable package path) and
**go-joker** (needs a source clone + codegen) — let-go's published M1 Pro data
puts gloat at 12.7× let-go on `fib` and 5.4× on `reduce`.

Two honest reads of that table.

**The good.** cljgo-aot now **wins every recursion and data-structure row
outright**: `tak` 34.6 ms and `fib` 24.7 ms (13× and 17× faster than the JVM —
pure int64 recursion is exactly what the ADR 0067 numeric-inference pass lifts
to raw Go: `func fib(n int64) int64`, direct typed recursion, zero boxing),
`loop-recur` 5.4 ms (6.8× ahead of let-go), and `persistent-map` 9.4 ms (ahead
of both let-go and babashka). `startup` is a dead heat with let-go at 5.0 ms —
the +3 ms regression the previous run reported was attributed (to boot-time
per-symbol refer and GC churn, *not* the seal/dual-body work, which measured
+0.0 ms) and clawed back with a bulk-refer snapshot + boot GC deferral, now
CI-gated so it cannot drift again. `map-filter` is 0.3 ms from let-go — a tie.
The emitted-vs-handwritten-Go factorial gate measures **~5×** — it was ~35×
before 2026-07-23.

**The bad.** `transducers` (16.4 vs babashka's 13.0 ms) and `reduce` (26.0 vs
babashka's 20.0, let-go's 22.8) remain behind the two purpose-built cores —
closer than the 2.4–2.8× of the previous run, but honestly lost. The residual
is per-element `Apply2` dispatch on the reducing fn plus `LongChunk.Nth`
boxing; ADR 0067's follow-ups (float64, multi-arity specialization, broader
lift) and an unboxed internal-reduce are the named path. The interpreter leg
(`cljgo run`) is a tree-walker and loses everywhere except against joker —
that is what `cljgo build` is for.

**What changed (2026-07-23, ADRs 0063–0067, two rounds).** Round 1: chunk-aware
`map`/`filter`/`count`/`keep` (JVM 32-element realization parity), the IFn2
2-arg seam (no `[]any` box per reduce step), direct-call emission for known
local fns, the sealed-core dirty-flag (guard elision with full `with-redefs`
liveness), and the int64 numeric-inference pass. Round 2: `<=`/`>=` joined the
unboxed compare set (fib's entire remaining cost), and startup was attributed
and clawed back. Same machine, morning → evening:

| Benchmark | before | after | |
|---|---|---|---|
| `fib` | 975.4 ms | **24.7 ms** | 39× |
| `tak` | 858.5 ms | **34.6 ms** | 25× |
| `loop-recur` | 52.1 ms | **5.4 ms** | 9.6× |
| `reduce` | 60.8 ms | **26.0 ms** | 2.3× |
| startup | 6.5 ms | **5.0 ms** | after a mid-day 9.5 ms peak |

### Earlier campaigns, kept honest

**AOT core (ADR 0046, spikes S22/S23).** Compiled `core.clj` and the boot
sources through cljgo's own emitter (`pkg/coreaot`): a compiled binary links
the **compiled** core and no interpreter at all (`pkg/eval` 155 → **0**
symbols in the link set, `pkg/analyzer` 63 → 0, `pkg/ast` 14 → 0 —
CI-enforced, `pkg/coreaot/imports_test.go`). At the time that took startup
27.5 → 5.5 ms; boot growth from the fundamentals batches later pushed it to
~9.5 ms, and the 2026-07-23 clawback (bulk boot refer + boot GC deferral,
now gated by `TestBootStartupBudget`) brought a hello binary to **~5.0 ms
today**. It also settled the A/B that used to indict `cljgo build`:
work in user code compiles to ~9.7× its interpreted speed, and core-heavy
programs stopped being indistinguishable between modes.

**What AOT core did not buy: size.** ADR 0023 §2 predicted ~2 MB per
compiled program. Measured then: 5.3 → 4.6 MB stripped — the tree-walker
left, but ~13k lines of compiled core arrived, and what remains is the
runtime a compiled binary genuinely needs (`pkg/lang`'s data structures and
numeric tower, `pkg/corelib`'s ~700 symbols, the reader). Dual-body emission
(ADR 0067) has since grown it back to **5.3 MB**. Size from here is a
dead-code and dual-body-trimming problem, not an AOT-core problem; ADR 0046
records the ~2 MB prediction as superseded by measurement.

**Boot.** Interpreter boot got 8.9× faster in v0.2.0 (211 ms → 23.7 ms) by
replacing a stack-scraping goroutine-ID lookup that was burning 73% of boot
CPU with a `getg()`-based one (ADR 0034, spike S18). It has since grown with
the core it boots — 31.7 ms today (ADR 0019 says the budget grows with the
core, and the 250 ms gate holds). `.github/workflows/boot-bench.yml` is a
manual ubuntu-vs-macos boot comparison kept as a permanent diagnostic.

## Development

Authority chain: `docs/adr/` (decisions) › `design/00-architecture.md`
(contracts + M0–M5 roadmap) › `design/01–07` (component internals) ›
`openspec/` (active change proposals). Process for non-trivial work:
ADR → OpenSpec propose/design → apply.

Gates before every commit:

```
go build ./... && go vet ./... && gofmt -l pkg cmd conformance templates && go test ./...
```

```
pkg/lang     runtime (persistent data structures, vendored from Glojure)
pkg/corelib  Go-native core builtins (ADR 0043)
pkg/reader   pkg/ast   pkg/analyzer   pkg/eval   pkg/repl   pkg/emit
pkg/coreaot  the compiled clojure.core a built binary links (ADR 0046)
pkg/deps     dependency resolution + lockfile (ADR 0052)
cmd/cljgo    CLI (repl · nrepl · run · build · new · test · publish · suite · check · explain · …)
core/        core.clj + satellite namespaces — Clojure-in-Clojure
templates/   real, runnable project templates `cljgo new` embeds (lib · cli · web)
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
