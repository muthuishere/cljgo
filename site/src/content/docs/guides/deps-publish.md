---
title: Dependencies & publishing
description: Declare dependencies as code in build.cljgo, pin them with a committed lockfile, and publish one library to both the Go and Clojure ecosystems with cljgo publish.
---

cljgo has no `deps.edn` and no separate manifest. Dependencies are
declared as *code* in your project's `build.cljgo` (ADR 0021), resolved
by one resolver that feeds both execution legs — a dependency resolves
identically under `cljgo run` and `cljgo build` (ADRs 0052/0053).

## Declaring a dependency

From `examples/build-deps` in the repo — an app depending on a local
library:

```clojure
;; app/build.cljgo
(defn build [b]
  (dep b "greetlib" {:path "../greetlib"})
  (let [app (exe b {:name "app" :main "src/app/core.cljg"})]
    (install b app)
    (run b app)))
```

```clojure
;; app/src/app/core.cljg — greet.core lives outside this tree;
;; the dependency load path resolves it, in both legs.
(ns app.core
  (:require [greet.core :as greet]))

(defn -main [& args]
  (println (greet/hello (if (seq args) (first args) "world"))))
```

A git dependency looks the same, pinned by ref:
`(dep b "greetlib" {:git "https://…" :ref "v1.2.0"})`.

Third-party **Go modules** are one line, not a binding:
`(go-require app "github.com/gorilla/websocket" "v1.5.3")` — cljgo
synthesizes the generated `go.mod` and the emitter links the real module
(see [the interop guide](/cljgo/guides/interop/)).

## The lockfile

The first `cljgo build` writes **`build.lock.edn`** next to
`build.cljgo`. Commit it. Per dependency it records identity
(`:git/url`, `:git/ref`, `:git/sha`), a merkle `:tree/hash` verified on
every read (a git SHA alone is not a content hash), the dep's source
`:paths`, its transitive `:requires`, and whether it is `:pure?`. The
lock is authoritative: a `build.cljgo` ref that disagrees with the
locked SHA is an error naming both — never a silent re-pin.

Once locked, `cljgo run` resolves the same dependencies the same way —
one resolver, both legs, so the interpreter and the binary can never see
different library code.

Some deliberate properties of the resolver (ADR 0052):

- **Global content-verified cache** under `$XDG_CACHE_HOME/cljgo` (or
  `~/.cache/cljgo`; override with `$CLJGO_CACHE`). Entries are immutable
  read-only trees, so removal is a verb: **`cljgo cache clean`**.
- **Version conflicts are a hard error**, not silent minimal-version
  selection — the error names both requirers and both versions.
- **Transitive deps come from the lock as data.** A dependency's
  `build.cljgo` is never executed during resolution — no arbitrary code
  runs just to discover the graph.
- **`clojure.*` cannot be shadowed** by a dependency root — a deliberate,
  recorded divergence from the JVM classpath.
- A project-local `vendor/<name>/` directory overrides the cache under
  the same lock hash, for air-gapped or audited builds.

Source files may be `.clj`, `.cljc`, `.cljg`, or `.cljgo`; the build file
is probed as `build.cljgo` > `build.cljg` > `build.clj`,
most-specific-first (ADR 0055).

## Publishing: one library, both ecosystems

A cljgo project publishes from the same `build.cljgo` that builds it —
no second manifest (ADR 0054). The project declares a library artifact
(`(lib b …)`), and the target is chosen at the command line:

```
cljgo publish go        # a go-gettable Go module
cljgo publish clojars   # pure Clojure source for JVM-Clojure consumers
```

Flags (all optional): `-o dir` output directory (default
`./publish/<target>`), `-name lib` which library artifact, `-module`
override the module path/coordinate, `-runtime` cljgo source tree for
the generated `go.mod` (publish go).

**Purity decides which targets a library qualifies for**, checked at
publish time over the whole transitive required surface:

| the library uses… | `publish go` | `publish clojars` |
|---|---|---|
| pure Clojure only | yes | yes |
| `require-go` (Go interop) | yes | **refused, with `file:line`** |

A pure-Clojure library is the only artifact that reaches both worlds. The
moment a reachable namespace uses Go interop, it is Go-side only — and
`publish clojars` names the offending file and line instead of shipping a
broken download. Go developers then just `go get` the module; JVM-Clojure
developers consume the source via a git coordinate in their `deps.edn`.

### Honest edges (implemented vs deferred)

- `publish go` wrappers currently expose `any` signatures; typed
  signatures from type hints are a tracked follow-up, as is wiring a
  library's own third-party `go-require` into the published module.
- `publish clojars` is **git-coordinate distribution today** — the
  actual Clojars coordinate/source-jar upload step is deferred.
- Consuming existing JVM-Clojure libraries from cljgo is deferred: most
  carry Java interop, which cljgo does not support and fails on loudly,
  per-namespace, never silently.
- `c-shared` / `c-archive` library targets (ADR 0013) are not built yet.

To ship an executable instead, see
[Compile & ship binaries](/cljgo/guides/compile/).
