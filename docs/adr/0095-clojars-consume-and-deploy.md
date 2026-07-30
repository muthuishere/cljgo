# ADR 0095 — Clojars, both directions: consume pure Clojure by coordinate, deploy for real

Date: 2026-07-27 · Status: **partially accepted** — decisions **1, 2 and 4
(consume)** are IMPLEMENTED (openspec change `adr-0095-clojars-consume`:
`pkg/deps` Maven arm, `pkg/javadetect` per-namespace gate, reader `R1012`).
Decision **3 (deploy)** remains **proposed** and rides a separate change.
Evidence in: spikes **S50** (consume)
and **S51** (deploy) both **closed MET** (2026-07-27), so the falsifiable claims
below survived; the scope language is now tightened by S50's finding (the pure
subset is real but *utility-shaped*, not the Java-wrapping mainstream). Awaits
owner ratify + `/opsx:propose`. Extends **ADR 0054**, which
built `publish go|clojars` but deliberately deferred *both* items this ADR
takes on: (a) **consuming** Maven/Clojars libraries (0054 Consequences, "out of
scope"), and (b) the **real Clojars coordinate / source-jar upload** (0054 §64,
"git-coord first; Clojars coord later"). Rides ADR 0052's load path + lock +
content-cache and ADR 0053's "never silent `nil`" — reuse, not new machinery.

## Context

Owner goal (2026-07-27): *"build.cljgo only gets git-based entries — no
clojars — and publish to clojars should also be there."* Today `dep`
(`core/build.cljg:64`) consumes only `{:git …}` / `{:path …}`; `go-require`
consumes Go modules; `publish clojars` emits a pure source tree plus a
**git-coordinate `deps.edn` stub** (`pkg/publish/clojars.go:153`) — it never
touches clojars.org. So cljgo is a half-citizen of the Clojure ecosystem: it can
be *git-depended-on*, but it cannot consume a Clojars artifact and cannot deploy
one.

The framing owner set earlier — *"similar instead of full, that way we will use
it"* — is the design constraint, not a footnote: **do not grow a parallel
tools.deps/`deps.edn` subsystem inside cljgo.** A Clojars dependency must be
*the same kind of thing* as a git dependency — one more coordinate shape on the
one resolver, one lockfile, one content-cache — differing only in how the source
bytes are fetched. The `full` alternative (a complete Maven/Aether resolver, a
JVM to run it, `deps.edn` semantics) is explicitly rejected.

Two constraints inherited verbatim from ADR 0054 still bind and shape everything
below:

1. **cljgo compiles to Go, not JVM bytecode, and grows no JVM backend.** A
   Clojars artifact is usable *only* as the pure Clojure **source** the jar
   carries — never its `.class` files, never Java interop.
2. **cljgo does not do Java.** Consuming a Java-tainted namespace must fail
   exactly as loudly as an unlinked Go module (ADR 0053), per-namespace, with
   `file:line` — never `nil`, never a blanket library ban (ADR 0054 decision 4,
   whose *policy* was decided there for exactly this now-arriving case).

**The honest scope limit, stated up front:** most Clojars libraries carry Java,
so the *consumable pure subset is thin* — but it is real and worth having:
`org.clojure/data.json`, `org.clojure/tools.cli`, `medley`, `org.clojure/core.match`,
and much of the `org.clojure/*` pure tier compile as source with no Java. This
ADR makes that subset consumable and fails everything else loudly, rather than
pretending the whole ecosystem is reachable. S50 measures how thin.

## Decision

### 1. Consume by coordinate — one more shape on `dep`, not a new subsystem

A Maven/Clojars dependency is declared with the tools.deps coordinate idiom,
symmetric with the git and path shapes already on `dep`:

```clojure
(defn build [b]
  (dep b "org.clojure/data.json" {:mvn/version "2.5.1"})   ; NEW — Clojars/Maven
  (dep b "greetlib"              {:git "https://…" :ref "v1.2.0"})
  (dep b "sibling"               {:path "../sibling"})
  …)
```

- **Coordinate, not a URL.** `name` is the Maven `group/artifact`; `:mvn/version`
  is the version. `:mvn/repos` (optional, project-wide) overrides the default
  repository list, which is **Clojars then Maven Central** — the tools.deps
  default set, reused, not reinvented.
- **Resolution is a fetch, not an interpreter.** A minimal **pure-Go**
  Maven/Clojars HTTP client (ADR 0052's zero-dependency ethos — no shelling to
  `mvn`, no JVM) does: `GET …/<a>-<v>.pom` → parse `<dependencies>` for the
  transitive graph → `GET …/<a>-<v>.jar` → extract the jar's `.clj`/`.cljc`
  source roots. The extracted source becomes **a source root on the existing ADR
  0052 / S30 load path** — identical to how a git dep's checkout already mounts.
  No emitter change: to the analyzer/emitter a Clojars dep is just more `.clj`
  files on the path.
- **Version conflicts are the same hard error** as ADR 0052 decision 4 (names
  both requirers + both versions), overridable by the existing `accept-version`.
  We adopt tools.deps' *top-of-the-graph-wins* only as the tie the user resolves,
  not as silent newest-wins.

### 2. The lock, the cache, and purity — all reused, none new

- **Lock.** `build.lock.edn` gains a `:mvn` coordinate alongside the git shape it
  already records (ADR 0052 §S33): `{:mvn/group … :mvn/artifact … :mvn/version …
  :mvn/repo <url> :sha256 <artifact-hash> :tree/hash <extracted-source-merkle>}`.
  Both the jar's published checksum *and* the extracted-source merkle are
  verified on every read — a coordinate alone is not a content hash. Transitive
  deps come from the lock **as data**, never by executing a dependency's build.
- **Cache.** Extracted source lands in the same immutable read-only
  content-verified tree under `$CLJGO_CACHE` (`cljgo cache clean` to remove).
  `vendor/<name>/` still overrides, for air-gapped builds.
- **Purity gate = ADR 0054 decision 4, now firing.** A required namespace that
  uses Java (the self-identifying surfaces — `import`, `new`, `System/…`,
  `Math/…`, `java.*/`, `clojure.java.*` — that already hard-error at analysis)
  fails **at the point it is required**, `file:line`, "Java interop is
  unsupported on cljgo's Go host" — while the artifact's *pure* namespaces stay
  usable. A jar carrying only `.class` (AOT, no source) is a hard error: "no
  consumable Clojure source." **`clojure.*` still cannot be shadowed** by a
  Clojars root (ADR 0052). Optional resolve-time strict mode (reject any dep
  whose manifest declares Java taint anywhere) rides ADR 0054 decision 4's
  already-specified hook — same default-deny spirit as capability gating.

### 3. Deploy for real — complete ADR 0054's deferred upload

`publish clojars` keeps its git-coordinate output as the **default, offline,
account-free** mode, and gains a real deploy:

```
cljgo publish clojars                 # today's git-coordinate source tree (default)
cljgo publish clojars --deploy        # build a Maven artifact + PUT to Clojars
cljgo publish clojars --deploy --repo https://repo.example/…   # any Maven repo
```

- **Artifact.** From the `(lib b {… :module "org.clojure/data.json" :version …})`
  declaration, build a standard Maven **source-bearing** artifact: a generated
  `pom.xml` (group/artifact/version from the coordinate; pure-Clojure `<packaging>jar</packaging>`;
  the transitive `:mvn` deps as `<dependencies>` so a JVM consumer's resolver
  pulls them) plus a `.jar` whose payload is the pure `.clj` source tree
  `publish clojars` already produces — the same purity gate (decision 3 of ADR
  0054, `uses-go-interop?`) still refuses a Go-tainted library with `file:line`
  before anything is packaged.
- **Upload.** A pure-Go Maven deploy (`PUT` the `.jar`, `.pom`, and their
  `.sha1`/`.md5` to the repo's path) to Clojars' HTTP deploy endpoint. **No new
  network stack** beyond decision 1's client.
- **Auth is env-only, never baked.** Credentials come from `CLOJARS_USERNAME`
  and `CLOJARS_PASSWORD` (a Clojars **deploy token**, not the account password),
  read from the process environment at the point of the `PUT` and **never**
  written to a file, a lockfile, generated source, or a log. Absent → a named
  error telling the user which env vars to set and how to mint a deploy token.
  This is the security baseline, stated in the ADR so no implementation can drift
  from it.

### 4. The boundary this ADR draws (so "similar, not full" stays true)

- **In:** pure-Clojure-source consumption by coordinate; transitive `.pom`
  resolution; real Clojars/Maven deploy of a pure library.
- **Out, deliberately:** any Java `.class` execution (never — constraint 2); a
  full Maven/Aether feature surface (graph-affecting profiles, classifier
  trees, version ranges, SNAPSHOT metadata races) — we take the slice real
  pure Clojure libraries actually need and name-error the rest, not silently
  half-support it; and a `deps.edn` reader (cljgo has no `deps.edn` — ADR 0021).

#### 4.1 Amendment, 2026-07-30 — `<parent>` POM inheritance is IN

The implementation first filed `<parent>`, `${property}` interpolation and
`<dependencyManagement>` version supply together under "name-error, don't
half-support" (spike s50 finding 1). Verification against the live repositories
showed that reading of the boundary was wrong: **every `org.clojure` contrib
artifact carries `<parent>org.clojure/pom.contrib</parent>`**, so the refusal
excluded `tools.cli`, `data.json`, `data.csv` and `core.match` — the whole s50
sample set and the reason this ADR exists. A boundary that excludes the target
set is not a conservative boundary; it is a non-working feature.

Amended boundary: `<parent>` chains ARE resolved and merged (properties,
dependencyManagement, dependencies, groupId/version defaults), `${property}`
IS interpolated **from properties that actually exist**, and
`<dependencyManagement>` DOES supply a missing version **when the merged map
has one**. What still name-errors is unchanged in spirit and sharper in fact:
an *undefined* property, a version-less dependency with *no* managed entry, a
range, a `-SNAPSHOT`/`LATEST`/`RELEASE`, a graph-affecting profile, a
classifier, non-jar packaging. The principle stands — never guess a version —
it is only applied where guessing would actually occur.

Evidence: `org.clojure/tools.cli 1.1.230` now resolves through its real parent,
AOT-compiles and runs (transcript in
`openspec/changes/adr-0095-clojars-consume/proposal.md`).

## Consequences

- **cljgo becomes a full Clojure-ecosystem citizen in both directions** for the
  pure subset: it consumes pure Clojars libraries and deploys pure libraries to
  Clojars, from the one `build.cljgo`, no `deps.edn`.
- **No parallel resolver.** One load path, one lock schema (a `:mvn` shape added
  to the existing git shape), one content-cache, one conflict policy. A Clojars
  dep and a git dep differ only in the fetcher. This is the whole point of
  "similar instead of full."
- **The thin-subset limit is a documented feature, not a bug.** The consume
  guide must state plainly which libraries qualify (pure Clojure source, no
  Java) and that everything else fails loud per-namespace — honesty discipline,
  same bar as the competitive-claims rule.
- **Depends on ADR 0053** (never silent `nil`) — the per-namespace Java failure
  is only safe once the interpreter hard-errors; already landed.
- **Supersedes the two deferred ADR-0054 items**; ADR 0054's decisions 1–4
  (targets, purity gate, `file:line` failure, Java policy) stand unchanged — this
  ADR *fires* decision 4's consume-side policy and *completes* the clojars
  producer.
- **New surface to keep honest in docs:** `deps-publish.md`'s "Honest edges"
  section drops "consuming … is deferred" and "coordinate/upload … is deferred,"
  replacing both with the shipped behavior + the thin-subset caveat.

## Spikes (must close before this re-issues from `proposed`)

| spike | falsifiable question | outcome |
|---|---|---|
| **S50** | Can a minimal pure-Go client resolve a real Clojars coordinate's transitive `.pom` graph, extract its source, and mount it on the ADR 0052 load path — *and* how thin is the pure subset really? | **MET.** Pure-Go stdlib resolved every graph tested (incl. `clj-http`'s 11 transitive coords), extracted source from all jars — kill-condition #1 not triggered. Subset: 2 fully / 4 partial / 1 unusable of 7; **real but utility-shaped** — `tools.cli`/`medley`/most-of-`hiccup` consume, Java-wrapping libs (`cheshire`, `clj-http`) don't. Reader-conditional-fenced Java in `.cljc` is skippable (moved `medley` PARTIAL→FULLY). Per-namespace loud-fail confirmed correct (hiccup ships 8 pure + 2 Java in one jar). |
| **S51** | Does a pure-Go Maven deploy (`pom.xml` + source `.jar` + checksums, deploy-token auth) round-trip — publish, then consume-what-we-published via decision 1? | **MET.** `greetlib` built→deployed(local Maven layout)→consumed back **byte-identical**, checksums verified, transitive dep recovered from pom, all pure-Go. gpg signing does not bind (Clojars accepts unsigned). Auth env-only + gated. Only residual: one owner-run live-Clojars `PUT` smoke test (public side effect). |

Both closed with a `VERDICT.md` per ADR 0027 §2
(`spikes/s50-clojars-consume/`, `spikes/s51-clojars-deploy/`). Implementation
follows `/opsx:propose`; the lockfile `:mvn` shape (touching `pkg/deps`) and the
deploy client (`pkg/publish`) are the two apply-time work items. **S50's scope
finding is binding on the docs:** the consume guide must say "consume
pure-Clojure *utility* libraries; everything else fails loud per-namespace" — not
"consume the Clojure ecosystem."
