# s79 — should cljgo read `:deps` (and test paths) from `deps.edn`? results

**Question.** ADR 0119 makes cljgo read **`:paths` only** from a project's
`deps.edn`. The owner wants to go further: read `:deps` and test paths too, so
a pure `.cljc` Clojure library carrying only a `deps.edn` runs on cljgo with no
cljgo-specific file at all. The strategic goal is adoption by existing library
authors.

The concern this spike was opened to confirm or kill: **partially implementing
tools.deps semantics could produce a version set that agrees with NEITHER
host** — worse than not reading `:deps` at all. cljgo already resolves
Maven/Clojars, git and local-path coordinates (`pkg/deps/resolve.go:21-45`), so
the common case might be a clean *translation* rather than a second resolver.

### The ruling this spike is measured against (owner, 2026-08-01)

> "If there are ambiguous [forms], let it fail — the idea of `.cljc` is pure."

`.cljc`'s promise is *identical behaviour on both hosts*. A `deps.edn` form
cljgo cannot translate **exactly** is therefore a form that would break the
promise, and refusing it is the contract being enforced, not a limitation.
This reverses how divergence must be scored:

- Where cljgo is **STRICTER** than tools.deps — it errors where tools.deps
  silently picks — that is **correct**, not a gap. It is the safe direction.
- The only true blocker is the **opposite** case: a form cljgo would resolve
  **silently differently** from tools.deps, without erroring.
- A high refusal rate is then an **adoption cost to be weighed**, not a
  correctness problem — so it must be quantified, not softened.

Three questions: **coverage** (what fraction of real projects would work
unchanged versus hit a coded refusal), **divergence** (specifically: is there
any silent one), and **cost**.

**Measured 2026-08-01**, darwin/arm64 (macOS 26.4), Apple M5 Pro, go1.26.3, on
`fix/repl-project-resolution` @ 78c1d66. Harnesses:
`cmd/cljgo/depsedncost_bench_test.go` (cost, through `resolveRunDeps`, the
production entry point s72 benchmarked) and
`pkg/deps/spike79_m2corpus_test.go` (divergence — drives the **production** POM
walker `parsePOM`/`effectivePOM`/`pomChildren` over locally cached poms, no
network, env-gated so it never runs in CI).

---

## 0. The corpus, stated honestly — and it is WEAK

Every `deps.edn` reachable offline on this machine:

| | count |
|---|---|
| `deps.edn` files found (`~/muthu`, `~/.gitlibs`, `~/.clojure`, `~/Downloads`, …) | 168 |
| of which duplicate copies inside cljgo's own `.claude/worktrees/` | 134 |
| **distinct by content hash** | **24** |

Provenance of the 24:

| origin | files |
|---|---|
| the repo owner's own toolnexus / koine spikes and examples | 18 |
| cljgo's own benchmark fixtures (`clj-httpkit`, `clj-ring-jetty`) | 2 |
| `refs/let-go`, `refs/glojure/scripts/rewrite-core` | 2 |
| `io.github.cognitect-labs/test-runner` (the only genuinely foreign library) | 1 |
| `clojure-test-suite`, `~/.clojure/deps.edn` (user-level, not a project) | 2 |

**Independently authored, third-party project `deps.edn` files: about four.**
Eighteen of twenty-four share one author and one house style. This is not a
corpus you can generalise a percentage from, and no number in §1 should be
quoted as "N% of Clojure libraries". It is a *shape* sample, not a
distribution. Building a real one needs a network crawl of Clojars/GitHub,
which this spike could not do — named as an exclusion in §5.

A **second, stronger corpus** carries §2: the local Maven cache,
**290 poms / 225 parsed / 159 distinct group:artifact**, and the **58 real
coordinates** the Clojure CLI itself resolved from the 24 projects
(`clojure -Spath`, run offline, succeeded for **24 of 24**). That one is real
third-party data — those are the poms tools.deps actually consumed.

---

## 1. COVERAGE — which `deps.edn` forms appear

Key/coordinate presence across the 24 distinct files (comment-stripped literal
detection, presence only):

| form | files | % |
|---|---|---|
| `:paths` | 22 | 91.7% |
| `:deps` | 20 | 83.3% |
| `{:mvn/version …}` | 20 | 83.3% |
| `:aliases` | 4 | 16.7% |
| `:extra-paths` | 3 | 12.5% |
| `:extra-deps` | 3 | 12.5% |
| `:main-opts` | 3 | 12.5% |
| `:exec-fn` | 2 | 8.3% |
| `:git/tag` + `:git/sha` (inside an alias) | 2 | 8.3% |
| `:ns-default` | 1 | 4.2% |
| `:local/root` · `:override-deps` · `:default-deps` · `:replace-deps` · `:replace-paths` · `:exclusions` · `:mvn/repos` · `:classifier` · `:deps/root` · `:sha` (old form) | **0** | **0%** |

Cumulative feature levels — a file is at the *highest* level it needs:

| level | files | % | cumulative |
|---|---|---|---|
| **L0** `:paths` only, no `:deps` | 3 | 12.5% | 12.5% |
| **L1** `:paths` + `:deps`, `:mvn/version` only | 17 | 70.8% | **83.3%** |
| **L2** + top-level `:git/*` or `:local/root` | 0 | 0% | 83.3% |
| **L3** needs `:aliases` | 4 | 16.7% | 100% |
| **L4** needs overrides / exclusions / `:mvn/repos` / classifiers | **0** | **0%** | 100% |

**Shape of the distribution: a cliff, not a tail.** 83% of files sit at L0/L1 —
paths plus flat `:mvn/version` coordinates, the single simplest form there is.
Nothing in the corpus reaches L4. Declared top-level dependency counts are
`[0,0,0,0,1,1,2×15,3,3,10]` — **median 2, max 10**. There is no scaling
question in `:deps` at all.

Two facts inside that 83% matter more than the percentage:

**(a) 18 of the 20 files with `:deps` (90%) declare `org.clojure/clojure`.**
The single most common dependency in every real `deps.edn` is the one cljgo
must *not* resolve — it is the host, not a library. This is already handled:
`clojureItself` prunes `org.clojure/clojure`, `spec.alpha` and
`core.specs.alpha` from every graph (`pkg/deps/mvncoord.go:97-101`, applied
`mvnpom.go:326-332`) and **reports** the prune rather than doing it silently
(`resolve.go:280-282`). Confirmed empirically: `clojure -Spath` on `koine`
returns exactly `src` + those three jars; after the prune, cljgo would resolve
**zero** coordinates and the two hosts agree on the source root.

**(b) The test-path question does not have the answer it looks like it has.**
6 of 24 projects have a `test/` directory; only **3 of those 6 declare
`:extra-paths ["test"]`**. The other three rely on tools.deps' *root*
`deps.edn`, baked into the jar, which gives **every** project
`:test {:extra-paths ["test"]}` for free
(`clojure/tools/deps.clj:101-106` reading `clojure/tools/deps/deps.edn:1-20`).
So "read `:aliases :test :extra-paths`" **misses half the projects that have
tests**, and to not miss them cljgo would have to reproduce tools.deps' root
deps.edn — i.e. bake in the same default. The convention is the thing; the
declaration is optional.

---

## 2. DIVERGENCE — scored by DIRECTION, not by frequency

Under the ruling, every difference falls into one of three buckets:

| bucket | meaning | verdict |
|---|---|---|
| **STRICTER** | cljgo errors; tools.deps silently picks | **correct** — the contract enforcing itself |
| **AGREES** | same result | fine |
| **SILENT** | cljgo resolves differently and says nothing | **the only blocker** |

### 2a. Policy, source against source

tools.deps citations are from **org.clojure/tools.deps 0.22.1492**, read out of
the jar cached in `~/.m2` (the language checkout in `references/clojure` does
not contain tools.deps).

| | tools.deps 0.22.1492 | cljgo | bucket |
|---|---|---|---|
| graph walk | BFS over a queue, memoized children (`deps.clj:399-445`) | BFS over a queue, `seen` by coordinate key (`resolve.go:144-178`) | AGREES |
| a lib declared at TOP level, redeclared transitively at another version | **top-dep-wins, silently**: the transitive occurrence is dropped whatever its version (`deps.clj:339-340`, `:use-top`) | all roots are enqueued before any child (`resolve.go:146`), so the top pin is first-seen — but a differing transitive version is **hard error G5013** (`resolve.go:152-167`) | **STRICTER** |
| two TRANSITIVE paths, different versions | **newest-wins**, `dominates?` → `(pos? (compare-versions …))`, losers orphaned (`deps.clj:315-318`, `:358-363`) | **hard error G5013**, naming both versions and both requirers (`resolve.go:152-167`) | **STRICTER** |
| that conflict, overridden | n/a — never asks | `(accept-version b lib v)` ⇒ the **first-seen** version stands, *not* the newest (`resolve.go:159`) | STRICTER (opt-in, cljgo-side) |
| tie / incomparable versions | throws (`extensions.clj:131-134`) | cannot arise (one procurer per name) | AGREES |
| version comparator | Aether `GenericVersionScheme` (`extensions/maven.clj:132-141`) | none — no comparison is ever made | STRICTER |
| ranges / `LATEST` / `RELEASE` / SNAPSHOT | canonicalized to a concrete version before resolution (`extensions/maven.clj:59-83`) | **refused, G5018** (`mvnpom.go:382-403`) | **STRICTER** |
| scopes | `compile` + `runtime` only (`extensions/maven.clj:143-153`) | skips `test`/`provided`/`system`/`import` (`mvnpom.go:121-123`, `:317-319`) | AGREES |
| `<optional>true` | dropped (`extensions/maven.clj:146`) | skipped (`mvnpom.go:320-322`) | AGREES |
| parent POM / `<properties>` / `dependencyManagement` | Aether's full Maven model builder (`extensions/maven.clj:106-115`, `util/maven.clj:190-211`) | own implementation, depth ≤ 8 (`mvnpom.go:45`, `:158-209`, `:269-288`) | AGREES on the whole corpus (§2b) |
| BOM `<scope>import</scope>` | resolved by Aether | **not followed** (`import` ∈ `skippedScopes`, `mvnpom.go:122`); a version supplied only by the BOM is then missing ⇒ **G5011** (`mvnpom.go:406-417`) | **STRICTER** |
| `<profiles>` that can change the graph | evaluated by Aether | **refused, G5011** (`mvnpom.go:89-91`, `:299-301`) | **STRICTER** |
| `<classifier>` | supported, lib named `g/a$classifier` (`util/maven.clj:265-279`) | **refused, G5011** (`mvnpom.go:351-353`) | **STRICTER** |
| `<packaging>` other than jar/bundle | supported | **refused, G5011** (`mvnpom.go:302-304`) | **STRICTER** |
| two git refs for one lib | ancestry compare, picks a winner (`extensions/git.clj:130-147`) | error (`resolve.go:164-166`) | **STRICTER** |
| `org.clojure/clojure` + `spec.alpha` + `core.specs.alpha` | ordinary top deps, supplied by the root deps.edn at 1.12.0 | **pruned from every graph and REPORTED** (`mvncoord.go:97-101`, report `resolve.go:280-282`) | deliberate, and **not silent** |
| result | a **classpath** of jars | **source roots**; `.clj/.cljc/.cljs/.edn` extracted out of the jar (`mvnresolve.go:186-198`) | by design |
| **repository order** | **central first, then clojars**, unconditionally (`util/maven.clj:159-166`) | **clojars first, then central** (`mvncoord.go:87-90`), first repo answering 200 wins (`mvnfetch.go:60-115`) | **⚠ SILENT** |
| **`<relocation>`** | followed by Aether's model builder | **not implemented** — zero hits for `relocat` in `pkg/deps` | **⚠ SILENT** |
| **`<exclusions>` inside `<dependencyManagement>`** | inherited by Aether onto the managed dep | **dropped** — `mergePOMInto` copies only `<version>` out of managed entries (`mvnpom.go:218-228`) | **⚠ SILENT** (superset) |
| **`<exclusions>` reached by two paths** | **narrowed to the intersection**, difference re-enqueued (`deps.clj:216-243`) | first edge's set stands (`mvnpom.go:358-370`) | **⚠ SILENT** |
| **`:paths` as a map, or entries that are alias keywords** | a legal form (`specs.clj:49-50`, `:87`) | silently skipped, project gets **no** source roots (`depsedn.go:53-69`) | **⚠ SILENT** (ADR 0119, today) |

### 2b. The hunt for SILENT divergences — five, and how big each is

This is the only bucket that blocks. Every one was confirmed from both
sources, then sized against the 289-pom local Maven cache.

| # | silent divergence | frequency in `~/.m2` | who has it |
|---|---|---|---|
| 1 | **repository order** Clojars-first vs Central-first | **unbounded** — every coordinate published to both | anyone |
| 2 | `<relocation>` not followed | 2 / 289 (0.7%) | `wagon-http`, `jetty-client` — **both Java** |
| 3 | managed `<exclusions>` dropped | 9 / 289 (3.1%) | maven, maven-resolver, wagon, jetty-project, spice-parent — **all Java** |
| 4 | exclusion intersection not narrowed | 17 / 643 dep elements carry `<exclusions>` (2.6%) | Java parents |
| 5 | `:paths` map / alias-keyword entries skipped | 0 / 24 corpus files | — |

**Four of the five are Java-only and one is empty.** Not a single Clojure
library in the cache uses `<relocation>`, managed `<exclusions>`, or a BOM
import. #5 is a *missing* root, not a wrong one — it fails later as a
namespace-not-found, so it is a bad diagnostic rather than a wrong program, but
it is still silence and ADR 0119 should report it.

**#1 is the one real blocker, and it exists on `main` today.**
`DefaultMvnRepos` is Clojars-then-Central and its own comment calls that
"the tools.deps default set" (`mvncoord.go:87-90`) — tools.deps orders **Central
first, always** (`util/maven.clj:159-166`). With first-200-wins fetching, a
coordinate published to both repositories resolves to a **different artifact**
on the two hosts, with nothing said. It is not caused by reading `:deps`; it is
exposed by it, because reading `:deps` is what makes the same coordinate list
resolve on both hosts in the first place. Under the `.cljc`-purity ruling this
must be fixed before `:deps` is read, and the fix is a two-line reorder plus a
corrected comment, not a mechanism.

**Everything else diverges in the STRICTER direction**, which the ruling makes
correct by definition: cljgo raises a coded, located G5011 / G5013 / G5018 with
a fix and stops, where tools.deps would have quietly chosen for you.

Two of those STRICTER rows are worth naming as adoption friction, because they
fire on code that works fine on the JVM:

- **Top-pinning is the standard tools.deps idiom for overriding a transitive
  version** — declare the lib at top level and `:use-top` silently wins
  (`deps.clj:339-340`). On cljgo the same file is a **G5013** unless the author
  also writes `(accept-version …)` in a `build.cljgo` — which is exactly the
  file this feature exists to avoid needing. It never fired in this corpus
  (§2c), but it is the most likely first refusal a real adopter meets.
- **Newest-wins is invisible on the JVM.** An author who has never seen a
  version conflict has one anyway; cljgo will be the first tool to tell them.

### 2c. Empirically, on real poms — cljgo's production walker, offline

### 2b. Empirically, on real poms — cljgo's production walker, offline

`clojure -Spath` was run in all 24 corpus projects (offline, all 24 succeeded),
yielding **58 distinct Maven coordinates** that tools.deps actually put on a
classpath. Each was then walked as a root through cljgo's **production** POM
code (`parsePOM` → `effectivePOM` → `pomChildren`, with a parent fetcher
reading `~/.m2`).

| jar contents | roots | cljgo clean | cljgo REFUSED | version conflicts |
|---|---|---|---|---|
| pure Clojure (`.clj`/`.cljc`, **no** `.class`) | 29 | **25 (86%)** | 4 | 0 |
| mixed (Clojure + `.class`) | 5 | 5 | 0 | 0 |
| pure Java (`.class` only) | 24 | 1 | **23 (96%)** | 0 |
| **total** | **58** | **31 (53%)** | **27 (47%)** | **0** |

**Zero version conflicts across all 58 closures.** The G5013 hard-error policy
— the sharpest STRICTER divergence — **never fired once** on real data. Under
the ruling that policy is correct regardless; the measurement says its adoption
cost in this corpus is zero.

**Zero silent divergences observed.** None of the five candidates in §2b fired
on any of the 58: no relocation, no managed exclusion, no BOM import, no
double-path exclusion, no map-form `:paths`. Divergence #1 (repository order)
cannot be observed offline — it is a *fetch*-time difference and every pom here
was already cached — so it is asserted from source, not measured. Named as an
exclusion in §4.

The 47% refusal rate is not the number it looks like. **Every refusal is a
Java library, or a Clojure library whose refusal comes from a Java library
beneath it**:

- 23 of 24 pure-Java roots refused — Jetty ×13, slf4j-api, commons-io,
  commons-fileupload2-core, commons-codec — all `G5011: a <profile> that can
  change the dependency graph`.
- The 4 pure-Clojure refusals are `ring/ring-core`, `ring/ring-jetty-adapter`,
  `org.ring-clojure/ring-jakarta-servlet` and `crypto-random`, each refused
  *because of* commons-io / Jetty / commons-codec underneath.
- Everything that could actually run on cljgo resolved clean: `edamame`,
  `medley`, `cuerdas`, `lambdaisland/uri`, `babashka/http-client`,
  `core.match`, `data.csv`, `data.json`, `tools.cli`, `tools.namespace`,
  `tools.reader`, `rewrite-clj`, `ring-codec`, `ring-core-protocols`, `koine`.

The clearest single case: **`spikes/s03-dependency-purity`, an 11-library pure
Clojure project.** tools.deps resolved 14 jars; after cljgo's clojure-itself
prune that is 11 coordinates, and cljgo's walker resolved **11 of 11 clean,
same versions, no conflicts** — an exactly identical set.

### 2d. The adoption cost, per PROJECT — the number to weigh

The unit that matters is not the coordinate, it is the project. Rolling the 58
outcomes back onto the 24 corpus projects (a project is clean only if **every**
non-pruned coordinate in its `clojure -Spath` closure resolves clean):

| outcome | projects | % |
|---|---|---|
| **works unchanged** — full closure resolves, same versions | **23** | **96%** |
| **hits a coded refusal** | **1** | **4%** |

The single refusal is cljgo's own `benchmark/web/compare/clj-ring-jetty`
fixture — a **deliberate JVM baseline** built to be compared *against* cljgo,
pulling Jetty 12 and Apache commons. It refuses with G5011 naming
`commons-io`, `commons-fileupload2-core`, `commons-codec` and
`crypto-random`, each with a `:git`/`:path`/vendor fix. It was never a cljgo
target and would fail at require time regardless.

**Stated as the trade: 96% of this corpus would work unchanged; 4% would get a
coded refusal naming a `<profile>` in a Java POM.** Weigh that against the
corpus caveat in §0 — 24 files, ~4 independent authors — and against the shape
of the refusals, which is the more durable finding: *the refusal rate tracks
Java content, not `deps.edn` complexity.* A library that could run on cljgo at
all resolves; a library that could not, refuses at resolve instead of at
require. That is the same verdict arriving earlier and with a better message.

### 2e. A bug this turned up, unrelated to the question

`parsePOM` uses `encoding/xml` with no `CharsetReader`, so **any POM declaring
a non-UTF-8 encoding fails to parse** and the Go-internal message leaks
verbatim into a user-facing G5011:

```
error: cannot parse the POM of org.apache.commons/commons-parent 52:
  xml: encoding "ISO-8859-1" declared but Decoder.CharsetReader is nil
```

4 of the 290 cached poms (1.4%) declare `ISO-8859-1` — all `commons-parent`,
the parent of a large slice of Apache Java. This is a real defect
(`mvnpom.go:126-133`) and it violates *How to write error messages*: a Go
library's internal wording is not a diagnostic. Recorded, not fixed here.

---

## 3. COST — deps.edn versus build.cljgo

One `go test` invocation, so every row below is comparable to every other
(CLAUDE.md: absolute ms compare only within a table). `-benchtime 20x
-benchmem`, `resolveRunDeps("")` = the production path.

### 3a. The two project descriptions, same entry point

| project description | N deps | ms/op | B/op | allocs/op |
|---|---|---|---|---|
| bare directory (the floor) | — | 0.0048 | 744 | 17 |
| **deps.edn only** | 0 | **0.0198** | 9,009 | 143 |
| **deps.edn only** | 1 | **0.0213** | 11,640 | 200 |
| **deps.edn only** | 5 | **0.0342** | 22,753 | 426 |
| **deps.edn only** | 20 | **0.0873** | 80,095 | 1,473 |
| build.cljgo, no deps, no lock | 0 | **39.39** | 55,430,401 | 889,033 |
| build.cljgo + lock | 1 | **78.41** | 110,878,828 | 1,779,086 |
| build.cljgo + lock | 5 | **78.18** | 110,980,280 | 1,781,276 |
| build.cljgo + lock | 20 | **78.47** | 111,441,018 | 1,789,365 |

**At N=20 the deps.edn path is 900× faster and allocates 1,390× less.** At
N=0 it is ~1,990× faster than the cheapest build.cljgo shape. That is not an
optimisation margin; it is a different cost class, and the reason is
structural, not clever: **`build.cljgo` is EVALUATED by a fresh tree-walking
interpreter; `deps.edn` is PARSED as data.**

| piece | ms/op | B/op | allocs/op |
|---|---|---|---|
| `eval.New()` — one interpreter boot | **40.18** | 54,182,082 | 874,786 |
| `build.LoadPlan`, 0 deps | 42.38 | 55,437,386 | 889,107 |
| `build.LoadPlan`, 200 deps | 40.22 | 57,302,663 | 925,576 |

### 3b. Growth

`DepsEDNPaths` alone (read + full EDN parse + stat), five sizes:

| N | µs/op | B/op | allocs/op |
|---|---|---|---|
| 0 | 19.3 | 8,631 | 125 |
| 1 | 21.5 | 11,305 | 182 |
| 5 | 32.5 | 22,423 | 408 |
| 20 | 83.1 | 79,752 | 1,455 |
| 50 | 186.5 | 183,122 | 3,272 |
| 200 | 634.7 | 725,953 | 12,557 |

**Linear and trivial: ~3.1 µs and ~3.6 KB per declared dependency**, against a
fixed ~19 µs. Even 200 dependencies — twenty times the largest file in the
corpus — cost 0.63 ms, still **63× cheaper than one interpreter boot**.

`build.cljgo` grows at ~27 µs/dep on a ~39 ms constant (s72 §2, re-confirmed
above): flat, because N is not the cost there either.

**Reading `:deps` from `deps.edn` is free.** Cost cannot be an argument
against this feature, and it is not an argument for it — the resolution that
follows a parsed `:deps` costs exactly what the same coordinates cost from a
`build.cljgo`, and on a cold cache that is network time (s70), orders of
magnitude above both columns.

---

## 4. What this measurement EXCLUDES

- **The network, entirely.** No coordinate was fetched. §2b walks poms already
  in `~/.m2`, and `clojure -Spath` ran warm. One of 58 roots had an uncached
  transitive edge (`borkdude/edamame`), counted and skipped, not fetched.
  cljgo's *own* dependency cache was never exercised at all — the resolver's
  fetch/verify/extract path (`mvnfetch.go`, `mvnresolve.go`) is untested here.
- **A representative `deps.edn` corpus.** §1 rests on 24 files, ~4 of them
  independently authored. Percentages in §1 describe this machine, not Clojars.
  A real answer needs a crawl of Clojars POMs and GitHub `deps.edn` files.
- **Running the code.** §2b measures whether cljgo would *resolve* the same
  set, never whether the resolved namespaces *load*. `http-kit` resolves clean
  and is a Java library — cljgo would refuse it at require time with I4002
  (`mvnclassify.go:9-15`, "classify at resolve, fail at require"), which is
  correct and outside what was measured.
- **Alias semantics beyond presence.** `:extra-deps`/`:override-deps`/
  `:replace-deps` merge order was read from tools.deps source (`deps.clj:156-171`,
  `:839-862`), not executed and compared.
- **Version-conflict behaviour under conflict.** Zero conflicts occurred, so
  the *observed* rate is 0/58 — that bounds the frequency in this corpus, it
  does not validate cljgo's policy. Bracketed: 23% of the 159 distinct
  group:artifact pairs in `~/.m2` exist locally at more than one version, but
  that is an accumulation across many resolutions, an upper bound on nothing
  in particular.
- **Silent divergence #1 (repository order) is UNMEASURED.** It is a fetch-time
  difference and every pom here was already cached, so nothing in §2c could
  have exposed it. It is asserted source-to-source (`mvncoord.go:87-90` versus
  `util/maven.clj:159-166`) and its blast radius — every coordinate published
  to both Clojars and Central — is an argument, not a measurement. Bracketing
  it needs a network resolve of a doubly-published coordinate against both
  hosts, which this spike could not run.
- **`:local/root` recursion.** tools.deps resolves a `:local/root` directory
  through the child's own `deps.edn`, **ignoring the child's `:aliases`**
  (`extensions/deps.clj:23-35`). Zero corpus files use `:local/root`, so this
  is unmeasured.
- **Process spawn.** §3 is in-process; add ~10 ms of binary startup for an
  end-to-end figure (s72 §4).

---

## 5. VERDICT

**Read `:deps` — but fix the repository order FIRST.** Translate the flat
`:mvn/version` / `:git/*` / `:local/root` coordinate forms, refuse everything
else with a coded diagnostic, and do NOT touch `:aliases`. The premise that
this is "just a translation" holds for the subset that matters, and the
divergence fear does not survive contact with real poms.

Four findings carry the verdict:

1. **96% of the corpus works unchanged; 4% hits a coded refusal** (§2d), and
   the refusal names a `<profile>` in a Java POM on a project that was a JVM
   baseline, never a cljgo target. The refusal rate tracks **Java content, not
   `deps.edn` complexity** — which is the durable finding, since it means the
   adoption cost falls exactly on libraries that could not have run anyway.
2. **Coverage is a cliff at L1.** 83% of the corpus is `:paths` + flat
   `:mvn/version` coordinates; **0%** uses `:exclusions`, `:override-deps`,
   `:replace-deps`, `:mvn/repos` or classifiers. Whatever the real
   distribution is, the *shape* is unambiguous: the exotic tools.deps surface
   is not what libraries are written in.
3. **Every divergence but one is in the STRICTER direction, and therefore
   correct.** Five silent-divergence candidates were found (§2b): three are
   Java-only (0.7–3.1% of cached poms, **zero** Clojure libraries), one is
   empty in the corpus, and **exactly one is a real blocker — the repository
   order.** Zero silent divergences were *observed* across 58 coordinates and
   24 projects; the blocker is a source-to-source finding that no offline
   measurement could have exposed (§4).
4. **Cost is not a consideration.** 900× cheaper than the build.cljgo path,
   linear at 3.1 µs/dep.

### The blocker, and it must be fixed before `:deps` is read

`DefaultMvnRepos` puts **Clojars before Central**; tools.deps puts **Central
first, unconditionally** (`mvncoord.go:87-90` vs `util/maven.clj:159-166`).
With first-200-wins fetching (`mvnfetch.go:60-115`) a coordinate published to
both resolves to a **different artifact** on the two hosts, silently — a direct
violation of `.cljc` purity. Reading `:deps` does not cause this; it *exposes*
it, because sharing one coordinate list across both hosts is the whole point.
Reorder the slice and correct the comment that wrongly calls the current order
"the tools.deps default set". Two lines, no mechanism.

The other four silent candidates (§2b) are Java-only and rare, but under the
ruling "rare and silent" is still silent. Cheapest honest handling, in order of
what it costs:

- **`<relocation>`** — refuse with G5011 naming the relocation target
  (one `if`), rather than resolving the stub.
- **managed `<exclusions>`** — refuse the managed entry with G5011 rather than
  dropping the exclusions.
- **exclusion intersection** — leave as is *for now*, but say so in the ADR;
  it needs a second path to reach a lib, which needs `<exclusions>` in a
  Clojure POM, which does not occur in the cache. Do not build the narrowing
  logic on speculation.
- **`:paths` map / alias-keyword entries (ADR 0119, today)** — report the
  skipped form instead of returning `nil`. A missing source root that says
  nothing is how #185 stayed invisible in the first place.

### The subset to translate

Translate directly into `deps.Dep` (`resolve.go:21-45`) — the fields already
exist, one for one:

| deps.edn form | `Dep` field | note |
|---|---|---|
| `{:mvn/version "1.2.3"}` | `MvnVersion`, `MvnDeclared` | the 83% case |
| `{:git/url … :git/sha …}` | `GitURL`, `GitRef` | prefer `:git/sha` over `:git/tag`; a tag is provenance, a sha is identity (`lock.go:62-63`) |
| `{:local/root "../x"}` | `Path` | resolve relative to the deps.edn's directory |
| `org.clojure/clojure`, `spec.alpha`, `core.specs.alpha` | — | already pruned and **reported** (`mvncoord.go:97-101`) — this is the single most common entry in the file and it must stay a report, never silence |

### What must be REFUSED LOUDLY, never guessed

Each gets a coded diagnostic naming **the form**, **why cljgo cannot honour it
exactly**, and **the fix** (declare that dependency in a `build.cljgo`
instead) — never a silent skip. Under the ruling these refusals are the
contract, so their message is the feature:

```
error: deps.edn declares :aliases, which cljgo cannot honour exactly
       at deps.edn:5:2
  note: alias semantics depend on which aliases are selected (-M:test);
        cljgo has no selection, so any reading of them would differ from
        the JVM's
  help: declare these dependencies in build.cljgo instead
  help: run `cljgo explain G50xx`
```

The forms:

- **`:aliases`, in full.** Not `:extra-deps`, not `:extra-paths`, not
  `:override-deps`/`:default-deps`/`:replace-deps`/`:replace-paths`. Their
  semantics are selection-dependent (`deps.clj:839-862`) — there is no such
  thing as "the aliases" without knowing which were selected on the command
  line, and cljgo has no `-M:foo` to select them with. Reading them
  unconditionally is exactly the halfway implementation that agrees with
  neither host.
- **`:exclusions`** — cljgo's inheritance is not tools.deps' (no intersection
  narrowing). 0% of the corpus uses it; refuse until someone asks.
- **`:mvn/repos`** — refuse. It is a map keyed by repo *name* whose ordering
  tools.deps re-imposes itself (`util/maven.clj:159-166`); cljgo's list is
  ordered by declaration. There is no exact translation, only a guess about
  order — and order decides which artifact you get.
- **`:paths` entries that are alias keywords** — a legal tools.deps form
  (`specs.clj:49-50`), meaningless without alias selection. ADR 0119 already
  skips non-string entries; make that a report rather than silence.
- **Version ranges / `LATEST` / `RELEASE` / SNAPSHOT** — already G5018.

### Test paths: convention, not `:aliases`

Do not read `:aliases :test :extra-paths` — it misses **half** the corpus
projects that have tests (§1b), because tools.deps supplies that alias from its
own root `deps.edn`. The simple, correct move is the one that needs no reading
at all: **add `test/` as a source root when the directory exists**, which is
also what cljgo's own templates already lay out. One `os.Stat`, no new
mechanism, no dialect of `:aliases`.

### Do NOT build

No cache, no strategy object, no "tools.deps compatibility layer", no
configurable resolver, and above all **no second resolution path**. The
translation is a function from an already-parsed EDN map to `[]deps.Dep`, and
`resolveRunDeps` already has the branch it belongs in
(`cmd/cljgo/main.go:337`). If the implementation grows a mode flag, it has
been misread.

### On the `import` command idea

An explicit `cljgo import` that writes a reviewable `build.cljgo` was on the
table as the safe alternative. The data does not support paying for it: the
translatable subset covers the corpus, the untranslatable forms refuse loudly
rather than guess, and a generated file is one more artifact to keep in sync
with the `deps.edn` that remains the JVM's source of truth. **Two files that
must agree is a worse failure mode than one file read narrowly.** Reject it —
and if the direct read later proves wrong, `import` is still available and
nothing has been built in the meantime.

---

## 6. What surprised us

1. **The elephant is already handled, and it is 90% of the data.** The single
   most common `:deps` entry in real files is `org.clojure/clojure` — the one
   coordinate cljgo must never resolve. `clojureItself` has pruned it (and
   reported the prune) since ADR 0095. Going in, this looked like the thing
   that would sink the feature; it is done.
2. **The hard-error conflict policy never fired.** Newest-wins versus
   G5013-and-stop reads like the central divergence risk, and across 58 real
   coordinates and 24 real projects it produced **zero** disagreements. The
   loudest documented difference between the two resolvers is, on this data,
   invisible.
3. **The dangerous divergence is not in the resolver at all — it is in the
   repository list.** Going in, the fear was version arithmetic: newest-wins
   versus first-wins, MVS, solvers. All of that turned out to be STRICTER and
   therefore safe. The one thing that can silently hand two hosts different
   *bytes* is a slice literal with two URLs in the wrong order
   (`mvncoord.go:87-90`), carrying a comment that asserts the opposite of what
   tools.deps does. Sixteen rows of resolution policy, and the blocker is the
   one nobody would have thought to check.
4. **cljgo's Maven resolver is not a stub — it is most of Maven.** Parent-POM
   chains to depth 8, `dependencyManagement` merging, `${property}`
   interpolation with a 16-round limit, exclusion inheritance with wildcards,
   scope and `<optional>` filtering. 314 of 643 dependency elements in the
   local corpus omit their version (supplied by `dependencyManagement`) and 117
   use `${property}` versions — a resolver missing either would have failed on
   almost everything, and cljgo's failed on **none** of it. The work this ADR
   would need is already paid for.
5. **The real divergence is not in versions, it is in the repository list.**
   `DefaultMvnRepos` puts Clojars before Central and its comment claims that is
   "the tools.deps default set" — tools.deps orders Central first,
   unconditionally (`util/maven.clj:159-166`). With first-200-wins fetching,
   the two hosts can pull a *different artifact* for the same coordinate, and
   nothing says so. That is a bug on main today, independent of `:deps`.
6. **A Go stdlib default leaks into a user-facing diagnostic.** 1.4% of cached
   poms declare `ISO-8859-1` and `encoding/xml` refuses them by default,
   surfacing `Decoder.CharsetReader is nil` inside a G5011 (§2c).
7. **`clojure -Spath` works fully offline against a warm `~/.m2`** — 24 of 24
   projects. That, plus the tools.deps sources shipping inside its own jar, is
   what turned Q2 from an argument-from-documentation into a source-to-source
   and pom-to-pom comparison. Worth remembering for future dependency spikes.
8. **The honest weak point is not the design, it is the corpus.** Every
   structural conclusion here is well supported; every *percentage* rests on 24
   files with roughly four independent authors. Before this ships publicly with
   a coverage claim attached, crawl Clojars.
