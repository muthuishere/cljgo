# cljgo

[![CI](https://github.com/muthuishere/cljgo/actions/workflows/ci.yml/badge.svg)](https://github.com/muthuishere/cljgo/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/muthuishere/cljgo?sort=semver&color=00a86b)](https://github.com/muthuishere/cljgo/releases/latest)
[![Go Reference](https://pkg.go.dev/badge/github.com/muthuishere/cljgo.svg)](https://pkg.go.dev/github.com/muthuishere/cljgo)
[![Go](https://img.shields.io/badge/go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Clojure](https://img.shields.io/badge/clojure-1.12.5-5881d8?logo=clojure&logoColor=white)](https://clojure.org)
[![clojure-test-suite](https://img.shields.io/badge/clojure--test--suite-234%2F242%20(96.7%25)-brightgreen)](#status)
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
**234/242 files passing (96.7%)**, 97.1% of non-skipped, with 241/242 vars
resolved (99.6%). Zero failing files — the remainder are dialect-gated or
encode JVM-only semantics (ADR 0036). Run `cljgo suite` to reproduce. Early, moving fast.

| Milestone | State | What landed |
|-----------|-------|-------------|
| **M0** | ✅ | REPL: reader (full syntax-quote), `loop*`/`recur`, dynamic vars, namespaces |
| **M1** | ✅ | Macroexpansion, `defmacro` at the prompt, embedded `core.clj`, `clojure.test` |
| **M2** | ✅ | `cljgo build` → native binary, <10 ms startup, fixed-arity calling convention |
| **M3-v0** | ✅ | **Zero-ceremony Go interop, both modes** — `require-go`, package fns/consts, `(T,error)`→`[v err]`, `!` unwrap-or-throw |
| **M3.1/3.2** | ✅ | Members: `(.Method r …)`, `(.-Field r)`, `(set! (.-Field r) v)`, ctors `(pkg/T. {…})`, `(go/new T)` |
| **M4-v0** | ✅ | Concurrency: `(chan)`/`(chan n)`, `(>! c v)`/`(<! c)`, `(close! c)`, `(go …)` over **real goroutines** — no CPS rewrite |
| **Result/Option** | ✅ | `ok`/`err`/`just`/`none` + `unwrap`/`and-then`/`map-ok` + `let?`, `#cljgo/ok` literals (ADR 0014) |
| **Diagnostics** | ✅ | `cljgo check --json` structured records, `cljgo explain <code>` (ADR 0015) |
| **nREPL** | ✅ | `cljgo nrepl` — babashka's 13-op surface, per-session `*ns*`/`*1`/`*out*` streaming, `.nrepl-port`, `doc` (ADR 0031) |
| **nREPL** | ✅ | `cljgo nrepl` — Calva/CIDER connect; 13-op surface, sessions on goroutine-keyed bindings (ADR 0031) |
| **State** | ✅ | refs/`dosync` (STM-lite), agents, `future-cancel`, watches, `with-precision` (ADR 0038/0032) |
| Next | ◦ | C FFI (purego), `alts!`/`timeout`, generics, AOT `core.clj` (binary size), edn-strictness |

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
