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

Those 4 are dialect registration, not broken semantics: `abs`, `add-watch`,
`short` and `reduce` carry reader conditionals with **no `:default`** branch, so
a runtime the suite has never heard of reads them as nothing. Adding a `:cljgo`
branch — the same mechanism `:cljr` / `:lpy` / `:phel` already use — takes the
suite to **242/242 (100%)**; those branches are not upstreamed yet, so the
published number is what the suite gives as it ships (analysis:
[`docs/suite-upstream.md`](docs/suite-upstream.md)).

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

Performance is priority 4 and gated like conformance, not asserted. On let-go's
own benchmark suite (Apple M5 Pro, go1.26.3, wall-clock incl. startup), the
AOT binary (`cljgo build`) **wins every recursion and data-structure row
outright** and ties on startup:

| Benchmark | cljgo | let-go | babashka | clojure JVM |
|---|---|---|---|---|
| startup | **5.0 ms** | **5.0 ms** | 9.7 ms | 289.2 ms |
| `fib` | **24.7 ms** | 1.25 s | 1.14 s | 419.1 ms |
| `loop-recur` | **5.4 ms** | 36.6 ms | 37.8 ms | 397.5 ms |
| `persistent-map` | **9.4 ms** | 12.9 ms | 13.0 ms | 385.2 ms |
| `reduce` | 26.0 ms | 22.8 ms | **20.0 ms** | 302.5 ms |

`reduce`/`transducers` still trail the two purpose-built cores — closer than
before, but honestly lost. The emitted-vs-handwritten-Go factorial gate
measures **~4.8×** (it was ~35× before the 2026-07-23 campaign, ADRs 0063–0067);
two budgets — interpreter boot and that ratio — run inside plain `go test ./...`.

**Full numbers, methodology, the head-to-head across all runtimes, and the
campaign history: [`docs/performance.md`](docs/performance.md)** (reproduce with
`bash benchmark/run.sh`). Building an AOT binary is one command:
`cljgo build -o hello hello.clj`.

## Web framework (bri) — one static binary, tiny Docker image

bri is cljgo's batteries-included web framework (ADR 0041/0069): API-first,
JWT auth, composable guards, CORS, metrics, audit, a Compojure-style router.
It runs both **interpreted** (`cljgo dev`, live re-`def`, nREPL) and
**AOT-compiled** to a single static `CGO_ENABLED=0` binary, byte-identical
(ADR 0071) — so the dev loop is a REPL and the deploy artifact is a
scratch-image binary. `cljgo new --template web` ships a `Dockerfile`;
`docker build` gives you a ~15 MB image.

Benchmarked against the field (Docker, [`oha`](https://github.com/hatoo/oha)
15 s @ 50 conn, one container at a time — **reproduce it yourself** with
[`spikes/s45-bri-aot-docker/bench/run.sh`](spikes/s45-bri-aot-docker); don't
take our word for it):

| runtime | image | cold-start | req/s | p99 | peak RSS |
|---|--:|--:|--:|--:|--:|
| rust-axum | 140 MB | 28 ms | ~89k | 1.0 ms | 8 MB |
| deno | 277 MB | 146 ms | ~89k | 0.9 ms | 21 MB |
| clojure http-kit (JVM) | 847 MB | 1277 ms | ~82k | 1.0 ms | 353 MB |
| **bri (cljgo, compiled)** | **15.5 MB** | **~30 ms** | **~78k** | **1.4 ms** | **~16 MB** |
| bun | 333 MB | 28 ms | ~75k | 1.5 ms | 50 MB |
| clojure ring+jetty (JVM) | 858 MB | 1659 ms | ~67k | 1.5 ms | 491 MB |
| .NET (ASP.NET) | 359 MB | 172 ms | ~67k | 1.9 ms | 47 MB |
| go net/http | 7.6 MB | 30 ms | ~66k | 2.6 ms | 16 MB |
| node | 228 MB | 147 ms | ~62k | 1.8 ms | 134 MB |
| spring-boot (JVM) | 512 MB | 858 ms | ~55k | 1.7 ms | 574 MB |
| fastapi (python) | 220 MB | 381 ms | ~9k | 10 ms | 38 MB |

Throughput sits in the top tier (Rust/Deno/Bun/http-kit) and ahead of Go
net/http, Node, .NET, Spring Boot, and FastAPI — while carrying a native-Go
footprint. Against **JVM Clojure web** specifically: ~55× smaller image,
~40–55× faster cold-start, ~22–30× less memory, at comparable-or-better
throughput. Single-machine arm64/OrbStack, so throughput carries run-to-run
noise (the image/RAM/startup figures are stable) — which is exactly why the
runner is committed for you to re-run on your own hardware.

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
