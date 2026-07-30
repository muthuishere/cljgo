# Design — consume pure-Clojure libraries by Maven coordinate

Authority: **ADR 0095** decision 1/2/4 · **ADR 0052** (load path, lock, cache,
conflict policy) · **ADR 0054** decision 4 (per-namespace Java policy) ·
**ADR 0053** (never silent `nil`) · evidence **spike s50** (CLOSED, MET).

Everything s50 proved is taken as given: pure-Go stdlib resolution + extraction
works; the pure subset is real but utility-shaped. This doc settles the six
things the ADR and the spike left to apply time, and is explicit at the end
about what I decided that neither of them decided.

---

## 1. Where the Maven path hooks into `pkg/deps`

### 1.1 Surface in `build.cljgo`

```clojure
(defn build [b]
  (dep b "org.clojure/tools.cli" {:mvn/version "1.1.230"})     ; NEW
  (dep b "medley/medley"         {:mvn/version "1.4.0"})
  (dep b "greetlib"              {:git "https://…" :ref "v1.2.0"})
  (dep b "sibling"               {:path "../sibling"})
  (mvn-repo b "https://nexus.internal/repository/maven-public") ; NEW, optional
  …)
```

- The dep **name IS the coordinate**, `group/artifact`. A single-segment name
  (`"medley"`) means `group == artifact` — the Maven/Leiningen convention. The
  name is also the load-path/lock identity, so a coordinate and a git dep can
  never silently collide: they are different names.
- `{:mvn/version "…"}` is the only required opt. `:mvn/version` present **and**
  `:git`/`:path` present is a declaration error (`G5015`), not a precedence rule.
- `(mvn-repo b url)` **prepends** to the default list. Default list, in order:
  `https://repo.clojars.org`, then `https://repo1.maven.org/maven2` — the
  tools.deps default set, reused. First repo that answers 200 for the `.pom`
  wins for that coordinate, and the winning repo is recorded in the lock, so a
  later resolve does not re-shop the list.
- `Dep` gains `MvnVersion string`; `d.isMvn()` is `MvnVersion != ""`.
  `resolveOne` becomes a three-way switch `isPath / isMvn / git`.

### 1.2 Cache location

Reuses `CacheRoot()` (`$CLJGO_CACHE` → `$XDG_CACHE_HOME/cljgo` → `~/.cache/cljgo`).
Two new slots beside `src/` and `dl/`:

```
<root>/mvn/<sha256(repo ‖ group ‖ artifact ‖ version ‖ ext)>      # raw .pom / .jar bytes
<root>/src/<MvnIdentityKey(repo,group,artifact,version)>          # extracted source tree
```

`MvnIdentityKey` mirrors `IdentityKey`: identity **locates**, content
**verifies**. The extracted tree is published atomically via the existing
`publishAtomically` + `markReadOnly`, and `TreeHash` (unchanged) is what the lock
records and re-verifies on every read. A `vendor/<name>/` override works exactly
as it does for git — same slot, same lock-hash check, no new code path.

Extraction from the jar keeps only `**/*.clj`, `**/*.cljc`, `**/*.cljs`†,
`**/*.edn` and drops `.class`, `META-INF/`, `*.jar`, absolute/`..` zip paths
(zip-slip), entries over 32 MiB, and anything after 10 000 entries. Dropping
`META-INF/` alone removes a real s50 false positive: `clj-http` shipped
`META-INF/leiningen/…/project.clj`, which the spike's scanner flagged as a Java
namespace. † `.cljs` is kept only so a `.cljc`'s sibling doesn't look missing;
it is never a load-path candidate.

### 1.3 Offline behaviour — identical to git, no new policy

| state | behaviour |
|---|---|
| lock has the coord, cache warm | resolves, no network, tree-hash verified |
| lock has the coord, cache cold, `-offline` | `G5014` naming the coord and the cache path |
| lock has the coord, cache cold, online | re-fetch, verify `:mvn/sha256` against the lock, `G5012` on mismatch |
| no lock entry, `-offline` | `G5014` — "not in build.lock.edn, so it cannot be resolved without a remote" |
| lock/build version divergence | hard error naming both versions, `-update` to re-pin (same shape as the existing git divergence error) |

The purity map lives in the lock (§3.3), so `cljgo resolve` can *report* what is
and is not usable **fully offline**, before a byte is fetched.

---

## 2. The `.pom` resolution algorithm, and the scope line

```
queue ← [declared mvn deps]
while queue not empty:
  c ← pop
  if seen[group/artifact]:                      # first-wins, BFS order
      if seen version ≠ c version → G5013 (names both requirers + both versions)
                                    unless accept-version "group/artifact" pinned it
      continue
  pom ← GET <repo>/<group as path>/<artifact>/<v>/<artifact>-<v>.pom   (repo list, first 200)
  validate(pom)                                  # §2.2 — name-error unsupported features
  for each <dependency> in <dependencies>:
      skip if <scope> ∈ {test, provided, system, import}
      skip if <optional>true</optional>
      skip if excluded by an ancestor <exclusions> entry (group:artifact, `*` wildcard)
      skip if group/artifact ∈ CLOJURE_ITSELF                            # §2.3
      push
jar ← GET …/<artifact>-<v>.jar ; verify against …/<artifact>-<v>.jar.sha1 when the repo serves one
extract → cache → mount as a load-path root (§1.2)
```

BFS + first-wins matches tools.deps' "top of the graph wins", but a *disagreeing*
version is the **same hard error** as ADR 0052 decision 4, not silent
newest-wins. `accept-version` (already in `build.cljg`) is the override; its key
for a Maven dep is the coordinate string `"group/artifact"`.

### 2.1 In scope (the slice real pure-Clojure libraries need)

`<groupId>`/`<artifactId>`/`<version>`, `<dependencies>`, `<scope>`,
`<optional>`, `<exclusions>` (incl. `*` wildcards), `<packaging>jar</packaging>`,
`.sha1` checksum verification, multi-repo fallback.

### 2.2 Out of scope — **name-errored, never half-supported** (s50 finding 1, binding)

Each of these is detected in `validate(pom)` and raised as `G5011`, naming the
feature, the coordinate, the offending element, and what to do instead
(`accept-version`, or a `:git`/`:path` dep):

| feature | why it must error, not guess |
|---|---|
| `${property}` interpolation in a `<version>` | an uninterpolated version is a *wrong* version; s50 saw it live (`core.match` → `org.clojure/clojure ${clojure.version}` 404) |
| `<dependencyManagement>` supplying a missing `<version>` | same — a silently-omitted version resolves to nothing or to the wrong thing |
| `<parent>` POM inheritance | would be needed only to serve the two above |
| version **ranges** `[1.0,2.0)` | requires a solver; cljgo has none by design |
| `-SNAPSHOT` versions | mutable identity, incompatible with a content-verified lock |
| `<profiles>`, `<classifier>`, `<packaging>` ≠ `jar`/`bundle` | Aether surface, out per ADR 0095 dec 4 |

A `G5011` is raised **at the coordinate that needs it**, so the message can say
"`org.clojure/core.match 1.1.0` needs `${clojure.version}`", not "a pom failed".

### 2.3 `org.clojure/clojure` is never fetched — **a decision this change makes**

`org.clojure/clojure`, `org.clojure/spec.alpha` and `org.clojure/core.specs.alpha`
are added to a `CLOJURE_ITSELF` skip set and pruned from every transitive graph.
Rationale: cljgo **is** the Clojure implementation; its `clojure.core` is
embedded (ADR 0043). Fetching the JVM's own `clojure.jar` would mount a second,
Java-riddled `clojure/core.clj` on the load path — actively harmful. It also
happens to remove the single most common `${property}` case in the wild
(`medley` pulled all three; `core.match`'s 404 was exactly this). The skip is
reported, not silent: `cljgo resolve -v` lists pruned coordinates.

---

## 3. The per-namespace purity gate

**The granularity is the namespace, not the library** — s50 finding 3, confirmed
on real data (hiccup ships 8 pure + 2 Java in one jar; core.match 5+5). A
whole-library gate is wrong in both directions: it would reject hiccup's usable
8, and it would let a "mostly pure" library's Java namespace through.

### 3.1 Where it fires

A Maven dep's extracted tree is mounted as an **ordinary** load-path root (slot
3) and its directory is recorded in a resolver-owned `mavenOrigin map[string]Coord`.
When `ResolveLibPath` returns a file **under a maven-origin root**, the loader
runs the classifier (§3.2) on that file's *read* forms before the namespace is
installed:

- **pure** → loads normally. Nothing else changes; the analyzer/emitter see just
  another `.clj` file.
- **Java-tainted** → `I4002`, hard, naming the namespace, the coordinate, the
  `file:line` of the offending form, and the form itself.

This fires **on require**, not on resolve, which is exactly the ADR 0054 dec 4
policy: the library is installed, the pure namespaces are usable, and only the
*use* of a Java namespace fails. Because there is one resolver (design/06 §0),
the REPL leg and the AOT leg hit the same gate — parity by construction. The AOT
leg is the important one: the emitter discovers namespaces by evaluating
requires (ADR 0042), so a Java namespace can never be silently emitted.

### 3.2 The classifier

Reuses `pkg/publish.CertainJava` (`pkg/publish/java.go` — zero-false-positive by
construction, S35 precision 10/10), moved to a shared `pkg/javadetect` package so
both directions call one implementation, extended with the consume-side surfaces
the publish side never needed:

- `(:import …)` / `(:gen-class)` inside an `ns` form (the dominant real signal —
  `(:import` is what flagged cheshire, clj-http, hiccup/compiler, hiccup/util);
- `definterface`, `gen-class`, `proxy`, `reify` of a `java.*` interface,
  `extend-type`/`extend-protocol` onto a Java class;
- `set!` on a Java static field, `Class/member` from the existing bare-class table.

It still MUST NOT flag bare `(.method obj)` — undecidable, Go-valid — nor
class-ref values, `(instance? String x)` or `(catch Exception e)`. **The
classifier runs on the reader's output, after reader conditionals are resolved**
(§4), which is what makes `medley` fully consumable: its `java.util` lives in a
`#?(:clj …)` branch cljgo never reads, so it is not in the forms and cannot be
flagged.

### 3.3 The purity map is recorded in the lock

`cljgo resolve` classifies every namespace in the extracted tree once and writes
the result, so the report is available offline and the load-time gate is a cheap
lookup:

```clojure
{:name "hiccup/hiccup"
 :mvn/group "hiccup" :mvn/artifact "hiccup" :mvn/version "1.0.5"
 :mvn/repo "https://repo.clojars.org"
 :mvn/sha256 "…"            ; the jar bytes
 :mvn/pom-sha256 "…"
 :tree/hash "sha256:…"      ; the extracted source merkle
 :paths [""]                ; jar root IS the source root
 :requires []
 :pure? true                ; ADR 0052 impurity — a maven dep declares no :ffi/:cgo/:go-require
 :mvn/namespaces {:pure ["hiccup.core" "hiccup.page" …]
                  :java {"hiccup.compiler" "hiccup/compiler.clj:12 (:import …)"
                         "hiccup.util"     "hiccup/util.clj:9 (:import …)"}}}
```

`cljgo resolve` then prints, per Maven dep:
`hiccup/hiccup 1.0.5 — 8 namespaces usable, 2 require Java (hiccup.compiler, hiccup.util)`.
A dep whose namespaces are **all** Java (s50: `org.clojure/data.json`) still
resolves and locks, but resolve emits a loud warning naming it as contributing
zero usable namespaces — the honest report, not a fake success and not a fatal
error (the user may have it only as a transitive edge they never require).

**Note the lock's two "pure" notions are different and both kept:** `:pure?` is
ADR 0052 capability purity (ffi/cgo/go-require) — a Maven dep has none.
`:mvn/namespaces` is Java-taint. They are not merged.

---

## 4. Reader conditionals in `.cljc`

s50 finding 4 calls this load-bearing and asks for balanced-paren scanning. The
production answer is **stronger than a scan: use the real reader.**
`pkg/reader/readcond.go` already parses `#?(…)` and `#?@(…)` with full form
structure — it cannot be fooled by a `#?(` inside a string, a char literal, a
`;` comment, or a `#_` discard, which is precisely where a text scan fails. So:

- **No text scanning anywhere in the gate.** The classifier consumes reader
  output. This is the balanced-paren requirement satisfied in its strongest form,
  and it is what promotes `medley` PARTIAL → FULLY honestly rather than
  heuristically.
- **Which branches cljgo supplies: `:cljgo` and `:default`. Not `:clj`.** This
  is already fixed by `pkg/reader/readcond.go:35-58` and is not reopened here
  (see §7 for why supplying `:clj` was rejected).

### 4.1 The starved conditional — new, and the point of the whole section

JVM Clojure reads `#?(:clj a :cljs b)` on an unknown platform as *no value*, and
that is legal. For **project** code that stays legal (conformance depends on it).
For a **maven-origin** file it is a trap: a `.cljc` whose real body is `:clj`-only
would install a namespace with no vars, and the first call would be a resolve
error pointing at the *caller*, blaming the wrong thing.

New reader option `WithStarvedCondError`, enabled **only** for files under a
maven-origin root:

> a `#?`/`#?@` form with at least one branch, of which **none** is `:cljgo` or a
> caller-supplied feature or `:default`, raises **`R1012`**, naming the file,
> line, column, and the branch features actually present.

```
error: reader conditional supplies no branch for this platform at hiccup/util.cljc:14:3
       Expected one of: :cljgo, :default
       Found: :clj, :cljs
help: this namespace is written for the JVM only; cljgo cannot load it. run `cljgo explain R1012`
```

If a whole file is starved, the namespace fails **loud on require** — never an
empty namespace. Default reading semantics are unchanged, so no conformance
file moves.

### 4.2 The test corpus (s50 asks for this explicitly)

`pkg/deps/testdata/readcond/*.cljc` + a table test, one file per case:

| # | case | expected |
|---|---|---|
| 1 | `#?(:cljgo A :clj B)` | reads `A` |
| 2 | `#?(:clj B :default D)` | reads `D` |
| 3 | `#?(:clj B :cljs C)` | **R1012** (starved) |
| 4 | `#?(:cljs C)` — single non-matching branch | **R1012** |
| 5 | `#?@(:cljgo [a b] :clj [c])` splicing | splices `a b` |
| 6 | `#?@(:clj [c])` starved splice | **R1012** |
| 7 | nested `#?` inside a selected branch | inner resolved too |
| 8 | nested `#?` inside a **non**-selected branch | never read, never errors |
| 9 | `"#?(:clj x)"` inside a string / `\#` char / `;` comment / `#_` discard | **not** a conditional — the text-scan trap |
| 10 | `#?(:clj (java.util.Date.) :default (now))` | pure — the **medley** case, classifier sees no Java |
| 11 | `(ns x (:import #?(:clj [java.util Date])))` | pure on cljgo; the import never arrives |
| 12 | whole-file `:clj`-only body | namespace fails loud on require, not empty |
| 13 | unterminated `#?(` | existing **R1001**, unchanged |
| 14 | `#?(:clj a :cljgo b :default c)` | `:cljgo` wins over `:default`; first match wins by order |
| 15 | odd number of forms in `#?(…)` | existing reader error, unchanged |

Cases 1–2, 5, 7–9, 13–15 also get a JVM-oracle-verified conformance twin with
`:clj` substituted for `:cljgo` (the mapping `pkg/reader/phase2_test.go:77` already
uses), dual-harness. Cases 3–4, 6, 10–12 are cljgo-specific — frozen with a
written rationale, per conformance discipline.

---

## 5. Diagnostics

Append-only to `pkg/diag/registry.go` + `docs/diagnostics/registry.lock`; each
gets a `docs/diagnostics/<CODE>.md` explain page. Bands: reader → `R`, Java
interop → `I`, resolution → `G` (deps errors have no band of their own and
adding one is a bigger decision than this change should make; see §7).

| code | title | what it says |
|---|---|---|
| **R1012** | reader conditional supplies no branch for this platform | file:line:col, Expected `:cljgo`/`:default`, Found the branches present, `help:` that the namespace is JVM-only |
| **I4002** | namespace requires Java interop and cannot load on cljgo | names ns + coordinate + `file:line` + the offending form; `Fix`: "N other namespaces in `<coord>` are usable — see `cljgo resolve`"; states plainly that cljgo compiles to Go and runs no `.class` |
| **G5010** | maven coordinate not found | coordinate, every repo URL tried, the HTTP status from each; `Fix` on a plausible typo (Levenshtein over already-locked coords) |
| **G5011** | unsupported Maven POM feature | the feature (`${property}` / `dependencyManagement` / `<parent>` / range / SNAPSHOT / profile / classifier), the coordinate, the offending element; `Fix`: pin with `accept-version`, or depend via `:git`/`:path` |
| **G5012** | maven artifact checksum mismatch | coordinate, Expected sha256 (lock or repo `.sha1`), Found, cache path; `Fix`: `cljgo cache clean` |
| **G5013** | maven version conflict | coordinate, both versions, **both requirers**; `Fix`: `(accept-version b "group/artifact" "1.2.3")` |
| **G5014** | offline: maven coordinate unavailable | coordinate, whether the lock has it, the cache path checked; `Fix`: drop `-offline`, or `cljgo resolve -update` |
| **G5015** | conflicting dependency coordinates | dep name + the coordinate keys given (`:mvn/version` with `:git`/`:path`); `Fix`: keep one |

That is 8 codes (7 new + the R band one). Every message follows the doctrine:
name the thing, locate it, Expected vs Found, registered code, suggestions as
`Fix`es. **No raw Go panic and no bare `fmt.Errorf` on this path** — including
the network path: a DNS failure, a TLS error, a 500, and a truncated zip each
map to a named diagnostic, never a wrapped `*url.Error` dumped at the user.

---

## 6. Testing — no committed test touches the network

- **`httptest` repo double.** A `pkg/deps/testdata/repo/` tree laid out in Maven
  layout (`group/path/artifact/version/artifact-version.pom|jar|jar.sha1`) served
  by an `httptest.Server`; the repo list is injectable in `ResolveOptions`, so
  the same code path under test is the production one.
- **Fixture jars are built by the test**, in-process with `archive/zip`, from
  `testdata/` source trees — no binary jars committed, and the fixtures encode
  the s50 shapes verbatim: a 1-ns pure lib (`tools.cli`), a mixed 8+2 lib
  (`hiccup`), a fully-Java lib (`data.json`), a fenced-`.cljc` lib (`medley`),
  a `${property}` lib (`core.match`), and a 4-deep transitive graph.
- **A network-touching test is a broken test.** A `TestNoNetworkInDepsTests`
  guard installs an `http.RoundTripper` that fails any request not aimed at the
  test server, so a future test cannot regress this by accident.
- The live-network evidence stays where it belongs: frozen in
  `spikes/s50-clojars-consume/results.txt`.

---

## 7. Genuinely open points the ADR and the spike left, and what I decided

1. **Does cljgo supply `:clj` when reading a Clojars `.cljc`?** Neither
   document says. **Decided: no.** `pkg/reader` already fixes `:cljgo` +
   `:default` as the platform features, and supplying `:clj` would deliberately
   pull the Java branch a library fenced *away* — turning medley from FULLY
   consumable back into broken. The cost is honest and visible: `:clj`-only
   `.cljc` files fail loud (`R1012`) instead of pretending. A per-dep
   `{:mvn/features #{:clj}}` escape hatch was considered and **rejected** — it is
   a footgun that produces Java forms the Go host cannot run, and the failure
   would surface later and further away.
2. **`org.clojure/clojure` in the transitive graph.** Nobody decided this.
   **Decided: pruned** (§2.3), reported not silent.
3. **Where the gate fires — resolve time or require time.** ADR 0054 dec 4 says
   "per-namespace, loud", not *when*. **Decided: classify at resolve (recorded in
   the lock, so the report is offline and cheap), fail at require.** Failing at
   resolve would be the whole-library gate the spike says is wrong.
4. **`<exclusions>`.** ADR 0095 dec 4 puts "`<exclusions>` beyond the minimum"
   out of scope without defining the minimum. **Decided: in scope**, including
   `*` wildcards — real poms use it, it is ~20 lines, and without it §2.3's prune
   would be the only way to avoid junk edges.
5. **`.sha1` verification.** Not mentioned for the consume side. **Decided: verify
   when the repo serves one**, and always record our own sha256 in the lock —
   the lock is the real integrity story, `.sha1` is a cheap corroboration.
6. **Diagnostic band for dependency resolution.** The registry has no deps band.
   **Decided: reuse `G5xxx`** rather than open a `D6xxx` band in this change — a
   new band is a registry-shape decision that deserves its own ADR, and `G` is
   documented as "general". Flagged for the owner as reversible-later.
7. **A dep contributing zero usable namespaces** (data.json). **Decided: warn
   loudly at resolve, do not fail** — it may be an unused transitive edge; the
   `I4002` at require is the real failure.
8. **`:paths` for a Maven dep.** No manifest exists in a jar. **Decided:
   `[""]`** — the jar root is the source root, and a missing
   `cljgo.manifest.edn` continues to mean "pure" as `readManifest` already does.
