# s50 — Clojars/Maven consume (ADR 0095 decision 1)

**Falsifiable question.** Can a minimal **pure-Go, zero-dependency** client
resolve a real Clojars/Maven coordinate's transitive `.pom` graph, download the
jar, and extract usable `.clj` source onto a load path — and *how thin* is the
pure subset among real libraries?

**Kill condition.** Transitive `.pom` resolution needs JVM/Aether semantics we
can't slice cheaply, OR the pure subset among common libs is so thin the feature
isn't worth the resolver.

## Run

```
cd spikes/s50-clojars-consume
go run .        # needs network; hits repo.clojars.org + repo1.maven.org
```

Stdlib only (`net/http`, `archive/zip`, `encoding/xml`, `regexp`) — the proof
that ADR 0095's "no JVM, no shelling to `mvn`, zero-dependency" claim holds. No
`go.sum`, no external module.

## What it does

1. **Resolve** — given `group/artifact/version`, try repos in the tools.deps
   default order (Clojars → Maven Central), `GET` the `.pom`, parse
   `<dependencies>`, and walk the graph breadth-first (bounded depth 3).
2. **Fetch + extract** — `GET` the `.jar`, unzip in-memory, pull every
   `.clj`/`.cljc` entry.
3. **Classify purity** — a certain-only Java-taint scan (ADR 0054 dec 4 /
   S35's `certain-java?`): flag self-identifying JVM surfaces (`:import`,
   `gen-class`, `System/`, `Math/`, `java.*`, `definterface`, …). Crucially,
   distinguish **hard** Java (unconditional — cljgo cannot load the ns) from
   **fenced** Java (inside a `#?(:clj …)` reader conditional cljgo skips — the ns
   is still loadable).

## Sample & result (live, 2026-07-27)

`org.clojure/{data.json,tools.cli,core.match}` (Maven Central) ·
`medley`, `hiccup`, `cheshire`, `clj-http` (Clojars). Full log in `results.txt`.

| library | repo | transitive coords | loadable ns | hard-Java ns | verdict |
|---|---|---|---|---|---|
| `org.clojure/tools.cli` | central | 1 | 1 | 0 | **FULLY** |
| `medley` | clojars | 4 | 1 | 0 (`java.util` fenced in `.cljc`) | **FULLY** |
| `hiccup` | clojars | 2 | 8 | 2 | PARTIAL |
| `org.clojure/core.match` | central | 2 | 5 | 5 | PARTIAL |
| `cheshire` | clojars | 5 | 2 | 8 | PARTIAL |
| `clj-http` | clojars | 11 | 1 | 9 | PARTIAL |
| `org.clojure/data.json` | central | 1 | 0 | 1 | **UNUSABLE** |

**2 fully / 4 partial / 1 unusable.**

## Findings

- **Kill-condition #1 (resolver too hard): NOT triggered.** Pure-Go resolution +
  extraction worked on every graph tested, including clj-http's 11 transitive
  coordinates, with zero external dependencies. Two *slice edges* surfaced,
  neither fatal: Maven property interpolation (`${clojure.version}` in a parent
  ref → one MISS) and `dependencyManagement`-supplied versions are outside the
  minimal slice — real pure libs need neither to load their *source*. ADR 0095
  dec 4 already scopes these out; the resolver name-errors rather than guesses.
- **Kill-condition #2 (subset too thin): PARTIALLY triggered — survives, with a
  tightened scope.** The reachable set is **real but narrow, and its shape is
  predictable**: *pure-Clojure utility/algorithm libraries* consume cleanly
  (`tools.cli`, `medley`, most of `hiccup`), while *Java-wrapping libraries*
  (`cheshire`=Jackson, `clj-http`=Apache HttpComponents) do not — their one or
  two loadable namespaces are peripheral helpers, not the core you'd import for.
- **The per-namespace loud-fail policy (ADR 0054 dec 4) is validated as exactly
  right.** Purity is genuinely a per-namespace property: hiccup ships 8 pure + 2
  Java namespaces in one jar. A whole-library ban would throw away the 8; loading
  all 10 blind would silently break on the 2. Per-namespace hard-fail is the only
  honest option.
- **Reader-conditional awareness materially widens reach.** Treating `.cljc`
  Java fenced in `#?(:clj …)` as skippable moved `medley` from PARTIAL to FULLY
  consumable. Modern `.cljc` libraries are more consumable than a naive
  "contains `java.`" scan reports. (The spike's fence test is a cheap
  positional heuristic; a production gate needs balanced-paren scanning of the
  reader conditional, and must confirm cljgo actually supplies a `:cljgo`/
  `:default`/`:clj`-equivalent branch — noted for the apply phase.)

## Verdict

See `VERDICT.md`. Short form: **MET, with a scope the ADR must state honestly** —
build the resolver (it's cheap and pure-Go), target *pure-Clojure libraries*,
fail everything else loud per-namespace, and document that the reachable subset
is utility-shaped, not the Java-wrapping mainstream.
