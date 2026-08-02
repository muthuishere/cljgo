# adr-0095-clojars-consume — consume pure-Clojure libraries by Maven coordinate

## Why

ADR 0095 decision 1, de-risked by spike **s50** (CLOSED, verdict **MET**,
`spikes/s50-clojars-consume/`). Today `pkg/deps` has **no Clojars/Maven path at
all**: `(dep b name opts)` accepts `{:git …}` and `{:path …}` only. cljgo can be
git-depended-on and (since ADR 0054) emit a source tree for Clojars, but it
cannot consume a Clojars artifact. That is the gap.

s50 proved, on live network across 7 libraries, that a **pure-Go stdlib**
client (`net/http` + `archive/zip` + `encoding/xml`) resolves the transitive
`.pom` graph (deepest: `clj-http`, 11 coordinates) and extracts usable `.clj`
source. No JVM, no `mvn`, no Aether. That question is settled and is not
re-litigated here.

s50 also measured the honest limit: of 7 libraries, **2 fully consumable, 4
partial, 1 unusable**. The reachable set is *utility/algorithm* libraries
(`tools.cli`, `medley`, most of `hiccup`), **not** the Java-wrapping mainstream
(`cheshire`, `clj-http`, and — the sharpest data point — `org.clojure/data.json`,
which is 100% Java-tainted and consumes as *nothing*).

## What Changes

A Maven/Clojars dependency becomes **one more coordinate shape on the one
resolver** — not a parallel subsystem. Concretely:

- `(dep b "org.clojure/tools.cli" {:mvn/version "1.1.230"})` in `build.cljgo`,
  plus `(mvn-repo b url)` to override the default repo list (Clojars, then
  Maven Central).
- A new `pkg/deps/maven.go` fetcher: pom → transitive graph → jar → extracted
  source root, mounted on the **existing** ADR 0052 load path (slot 3). One
  cache, one lock, one conflict policy. `pkg/emit` untouched.
- The lock gains an `:mvn/*` shape beside the existing `:git/*` shape, carrying
  the artifact checksum, the extracted-source merkle, **and the per-namespace
  purity map** so `cljgo resolve` reports "8 usable, 2 Java" up front.
- A **per-namespace** Java gate: a mixed jar (hiccup = 8 pure + 2 Java) mounts
  normally; the 8 pure namespaces load; the 2 Java ones fail loud with
  `file:line` **on require**, never `nil`, never a whole-library ban.
- Reader conditionals in `.cljc` resolved by the **real reader** (features
  `:cljgo` + `:default`), plus a new **starved-conditional** error so a
  `:clj`-only `.cljc` fails loud instead of loading an empty namespace.
- 9 new diagnostic codes (append-only) with explain pages.

### The worked example — verified against the real artifact, 2026-07-30

Not a sketch. `org.clojure/tools.cli 1.1.230` fetched from Maven Central,
resolved through its real `<parent>org.clojure/pom.contrib 1.2.0</parent>`,
compiled AOT into a static binary, and run:

```clojure
;; build.cljgo
(defn build [b]
  (dep b "org.clojure/tools.cli" {:mvn/version "1.1.230"})
  (let [app (exe b {:name "demo" :main "src/app/main.cljg"})]
    (install b app)
    (run b app)))

;; src/app/main.cljg
(ns app.main
  (:require [clojure.tools.cli :as cli]))

(defn -main [& args]
  (let [r (cli/parse-opts ["-v" "extra"] [["-v" "--verbose" "Verbose"]])]
    (println "options:"   (:options r))
    (println "arguments:" (:arguments r))
    (println (:summary r))))
```

```
$ cljgo build
cljgo deps: org.clojure/tools.cli 1.1.230 — 1 namespace(s) usable
  reader conditionals elided a top-level form in clojure.tools.cli (as JVM Clojure would on a platform those branches do not name)
  inherited from the <parent> POM org.clojure/pom.contrib 1.2.0
  pruned org.clojure/clojure ${clojure.version} (cljgo IS the Clojure implementation; its clojure.core is embedded)
cljgo build: installed ./demo

$ ./demo
options: {:verbose true}
arguments: [extra]
  -v, --verbose  Verbose
```

The elided form is `cli.cljc:74`'s `#?(:cljs (defn- format …))` — a ClojureScript
helper JVM Clojure elides too. It is reported rather than glossed, because
"it loads" and "it is the same namespace you get on the JVM" are different
claims.

Deliberately **out of this change**: the deploy direction (ADR 0095 decision 3,
spike s51) — a separate change, `adr-0095-clojars-deploy`.

## Impact

- `pkg/deps`: new `maven.go`, `pom.go`, `purity.go`; `Dep`/`LockedDep`/lock
  reader+emitter extended; `resolveOne` gains a third arm.
- `pkg/reader`: one new opt-in option (`WithStarvedCondError`) — **no change to
  default reading semantics**, so conformance is untouched.
- `pkg/eval` load path: a maven-origin root triggers the per-namespace gate at
  file-read time; because there is exactly one resolver (design/06 §0), REPL and
  AOT legs move together — parity by construction.
- `core/build.cljg`: `dep` docstring + new `mvn-repo`.
- Docs: `deps-publish.md` "Honest edges" — the wording is **"consume
  pure-Clojure libraries; everything else fails loud, per-namespace"**, never
  "consume the Clojure ecosystem" (s50 finding 2, same bar as the
  competitive-claims rule).
- Tests are **fixture/httptest only**. No committed test may touch the network.
