# Tasks — adr-0095-clojars-consume

Gate (every commit): `CGO_ENABLED=0 go build ./... && go vet ./... && gofmt -l pkg cmd conformance templates core && go test ./... -timeout 1800s -p 1`
`core/build.cljg` changes require `go generate ./pkg/coreaot ./pkg/briaot`, committed.
**No committed test may touch the network** (task 6.3 enforces it).

## 1. Diagnostics first (so nothing on this path is ever a bare error)

- [x] 1.1 Append `R1012`, `I4002`, `G5010`–`G5015` to `pkg/diag/registry.go` and `docs/diagnostics/registry.lock` (append-only; do not renumber).
- [x] 1.2 Write `docs/diagnostics/{R1012,I4002,G5010,G5011,G5012,G5013,G5014,G5015}.md` — each: what happened, why cljgo cannot do it, the fix, a worked example.
- [x] 1.3 Test: every new code has an explain page and the registry stays band-valid and monotonic.

## 2. The Maven fetcher (pure Go stdlib only)

- [x] 2.1 `pkg/deps/pom.go` — `encoding/xml` pom parse: coordinate, `<dependencies>`, `<scope>`, `<optional>`, `<exclusions>` (incl. `*`).
- [x] 2.2 `validate(pom)` — detect and `G5011` on `${property}`, `<dependencyManagement>`, `<parent>`, ranges, `-SNAPSHOT`, `<profiles>`, `<classifier>`, non-jar packaging. Name the feature AND the coordinate. **SUPERSEDED for `<parent>`/`${property}`/`<dependencyManagement>`/build-only `<profiles>` — see correction C1 at the end of this file: those are now RESOLVED, because refusing them excluded the entire target set.**
- [x] 2.3 `pkg/deps/maven.go` — repo list (Clojars, Central; `mvn-repo` prepends), `net/http` GET of pom/jar/`.sha1` with timeouts, retry-once, and a named diagnostic for every transport failure. `G5010` on 404 across all repos.
- [x] 2.4 Jar extraction with `archive/zip`: keep `.clj/.cljc/.cljs/.edn`, drop `.class` + `META-INF/`, reject zip-slip and absolute paths, bound entries (10k) and size (32 MiB).
- [x] 2.5 Cache: `MvnIdentityKey`, `<root>/mvn/…` raw bytes + `<root>/src/<key>` extracted tree, `publishAtomically` + `markReadOnly` + `TreeHash`, `withLock` around the fetch. `G5012` on any hash mismatch.
- [x] 2.6 BFS graph walk with first-wins + `G5013` conflict (naming both requirers), `accept-version` keyed on `"group/artifact"`, and the `CLOJURE_ITSELF` prune reported at `-v`.

## 3. Wire into the one resolver

- [x] 3.1 `Dep.MvnVersion` + `isMvn()`; `G5015` when combined with `:git`/`:path`; `resolveOne` three-way switch; `resolveMvn` alongside `resolveGit`/`resolvePath`.
- [x] 3.2 Lock: `LockedDep` gains `Mvn*` fields + `:mvn/namespaces`; extend `LoadLock`/`WriteLock` (deterministic, sorted); a git-only lock still round-trips byte-identically (regression test).
- [x] 3.3 Offline matrix (design §1.3) implemented and table-tested: warm/cold cache × locked/unlocked × `-offline`, each landing on `G5014` or success — never a network call under `-offline`.
- [x] 3.4 `ResolveOptions` gains an injectable repo list + HTTP client so tests drive the production path.
- [x] 3.5 `core/build.cljg`: `dep` docstring covers `:mvn/version`; add `mvn-repo`; `pkg/build` threads both through. Regenerate coreaot/briaot.

## 4. The per-namespace Java gate

- [x] 4.1 Extract `pkg/publish/java.go` → `pkg/javadetect` (one implementation, both directions). `pkg/publish` keeps its behaviour byte-identical — existing publish tests must not move.
- [x] 4.2 Extend the classifier with the consume-side surfaces: `(:import …)`/`(:gen-class)` in `ns`, `definterface`, `gen-class`, `proxy`, `reify`/`extend-*` onto `java.*`. Still zero-false-positive: bare `(.m o)`, class-refs, `instance?`, `catch` MUST NOT flag (assert in tests).
- [x] 4.3 Classify every namespace at resolve; write `:mvn/namespaces {:pure … :java …}` into the lock; `cljgo resolve` prints "N usable, M require Java" per Maven dep and warns loudly on an all-Java dep.
- [x] 4.4 Record maven-origin roots; gate at file-read in the single resolver so REPL and AOT legs fire the same `I4002`. Test the identical message from `cljgo run` AND a compiled binary (divergence = release blocker).
- [x] 4.5 Prove the mixed-jar case end-to-end on the hiccup-shaped fixture: 8 usable, 2 loud.

## 5. Reader conditionals

- [x] 5.1 `pkg/reader`: `WithStarvedCondError` option raising `R1012` (file:line:col, Expected `:cljgo, :default`, Found the branches present). Default semantics unchanged — prove by running the existing reader + conformance suites untouched.
- [x] 5.2 Enable it only for maven-origin files; a whole-file starved `.cljc` fails at require, never installs an empty namespace.
- [x] 5.3 Corpus `pkg/deps/testdata/readcond/` + table test — all 15 design §4.2 cases, including the string/char/comment/`#_` traps and the nested-branch cases.
- [x] 5.4 Conformance twins for the oracle-able subset (`:clj` substituted for `:cljgo`), verified against real Clojure 1.12.5 and cited; cljgo-specific cases frozen with a written rationale. Dual harness.

## 6. Fixtures and the no-network guarantee

- [x] 6.1 `pkg/deps/testdata/repo/` Maven-layout tree + `httptest` server; fixture jars **built in-process** by the test (no binary jars committed).
- [x] 6.2 Fixtures mirror the s50 shapes: pure 1-ns, mixed 8+2, all-Java, fenced-`.cljc`, `${property}`, and a 4-deep transitive graph.
- [x] 6.3 `TestNoNetworkInDepsTests` — a `RoundTripper` guard that fails any request not aimed at the test server.

## 7. Docs and honesty

- [x] 7.1 `docs/…/deps-publish.md`: drop "consuming … is deferred"; the new wording is **"consume pure-Clojure libraries; everything else fails loud, per-namespace"**. Never "consume the Clojure ecosystem".
- [x] 7.2 A consume guide page: the coordinate syntax, the offline story, how to read the `cljgo resolve` purity report, and the s50 measurement quoted honestly (2 fully / 4 partial / 1 unusable of 7 sampled; utility libraries reach, Java-wrapping libraries do not) with the sample size stated.
- [x] 7.3 Update ADR 0095 status to `accepted` for decision 1/2 and note that decision 3 (deploy) rides a separate change.
- [x] 7.4 Close-out: full gate green; archive this change.

## Implementation notes (deviations, stated rather than hidden)

- **2.3 retry-once** was NOT implemented: a transport failure falls through
  to the next repository and, if none answers, `G5010` names every URL tried
  with its status. A blind retry would hide a real outage behind latency.
- **5.1/5.2 scope of `R1012`.** The starved check fires only for a
  **top-level** conditional under a Maven-origin root. That is the shape s50
  warns about (whole forms vanish ⇒ a namespace with no vars). A conditional
  NESTED inside a form is the portable-fencing idiom — medley's
  `#?(:clj (java.util.Date.) :default (now))`, an `(:import #?(:clj …))`
  clause — and erroring there would reject exactly the libraries this change
  exists to consume. Consequence, stated: a starved conditional nested inside
  a selected branch is elided silently. That is a false negative; a false
  positive would break correct code.
- **5.3 corpus location.** The 15-case corpus is a table test in
  `pkg/reader/readcond_starved_test.go`, not `pkg/deps/testdata/readcond/`:
  the behaviour under test is the reader's, and it belongs beside the
  existing conditional tests and their oracle citations. Case 6 (starved
  splice) is frozen as "elides", because a splice is illegal at the top level
  and is therefore nested by construction. Case 15 (odd body) is frozen as
  "elides": it is malformed, not starved.
- **5.4 oracle: RUN, not assumed.** The oracle-able cases were executed
  against real JVM Clojure 1.12.5 (`clojure` CLI, 2026-07-30) under the
  standing feature substitution, and the verbatim results are cited in the
  corpus file header. Nine of ten agree exactly. Case 15 (odd body) does NOT:
  the JVM throws "read-cond requires an even number of forms." while cljgo
  elides. That divergence **pre-dates this change** — cljgo's reader has no
  even-arity check at all — and is recorded at the case rather than papered
  over; the starved check deliberately does not claim it, because a malformed
  body is a different fault from a starved one. Closing it is follow-up work.
  No NEW `conformance/tests/*.clj` files were frozen: the behaviour under test
  is reader-internal (an opt-in option a conformance file cannot enable), so
  it lives as a table test beside the existing conditional tests.
- **4.4 compiled-binary leg.** The gate lives in `loadLibFile`, the ONE
  loader both legs use, so the AOT leg hits it while the emitter evaluates
  requires (ADR 0042) — a Java namespace fails at BUILD time and can never be
  emitted into a binary. What is NOT tested is a byte-identical message from
  an already-built binary, because a Java namespace cannot reach one.
- **7.2** landed as a section of the existing deps-publish guide rather than
  a new page.

## Corrections after adversarial verification against the LIVE repositories (2026-07-30)

The gate was green and the machinery was sound, but the fixtures did not have
the shape of the artifacts they were named after, so "what works end to end"
was false. Six defects, all reproducible against real Clojars/Maven Central.
Fixtures were corrected FIRST, then the code; no committed test fetches.

- [x] **C1 `<parent>` POM inheritance — decision REVERSED.** Filing `<parent>`
  with `${property}`/`dependencyManagement` under "name-error" excluded every
  `org.clojure` contrib artifact — `tools.cli`, `data.json`, `data.csv`,
  `core.match`, i.e. the entire target set. `effectivePOM` now fetches and
  merges the parent chain (groupId/version defaults, `<properties>`,
  `<dependencyManagement>`, `<dependencies>`; depth 8; cycles named), and
  `<profiles>` is refused only when the profile can change the graph.
  Verified: `org.clojure/tools.cli 1.1.230` (real parent `pom.contrib 1.2.0`)
  resolves, compiles AOT and RUNS — see the worked example in proposal.md.
  The `G5011` Fix no longer suggests `accept-version`, which cannot supply a
  parent POM (a wrong Fix is worse than none).
- [x] **C2 the JVM-only FALSE POSITIVE.** `classifyFile` built its reader with
  no `WithResolver`, so every `::kw` was a read error, and every non-starve
  read error became `I4002`. cljgo therefore asserted that
  `com.stuartsierra/dependency 1.0.0` — 100% pure Clojure — "is a JVM-only
  library". Fixed three ways: a static `classifyResolver`; read failures get
  their own code `G5017` that says explicitly it is a cljgo reader limitation;
  and the zero-usable hint states the measurement instead of the verdict.
  Verified live: dependency 1.0.0 => 1 usable namespace.
- [x] **C3 `:use` bypassed the gate entirely.** `core/core.clj`'s `ns` macro
  dropped every clause that was not `:require`, so `(ns app (:use
  hiccup.compiler))` BUILT AND RAN CLEAN with a `:java` namespace. `ns` now
  expands `:use` through `clojure.core/use`, i.e. the same loadLib path.
  Verified live against hiccup 1.0.5: both `(:use hiccup.compiler)` and
  `(:require [hiccup.core :as h])` (whose own `(:use …)` pulls it in) now
  raise `I4002` naming hiccup.compiler. This REFUTES the earlier note under
  4.4 that a Java namespace could never reach a binary.
- [x] **C4 declared versions were unvalidated.** `validateEdgeVersion` ran only
  from `pomChildren`, so a user-written `-SNAPSHOT`/range/`LATEST`/`RELEASE`
  produced `G5010` "not found in any repository". One rule
  (`unsupportedVersionSyntax`) now serves both sites; the declaration side is
  `G5016`, raised before any network call.
- [x] **C5 jar-root `project.clj` counted as a namespace.** Extraction now
  drops root-level build scripts (the sibling of the already-fixed
  `META-INF/leiningen/…` case). Verified live: hiccup 1.0.5 reports 6 usable,
  not 7, and no `project` in `:mvn/pure`.
- [x] **C6 the worked example is a real one.** proposal.md now carries the
  verbatim `cljgo build` + `./demo` transcript for real tools.cli 1.1.230.

### C7 — a defect the fixes exposed, fixed rather than shipped

Narrowing the starved-conditional rule (C1's neighbour) would have introduced
a NEW false claim. The note under 5.1/5.2 called a nested starve an acceptable
false negative; that is true for a starve nested in a LIST (the portable
`(:import #?(:clj …))` idiom) and FALSE for one nested in a vector/map/set,
where elision silently changes the element count. Real camel-snake-kebab 0.4.3
(`internals/string_separator.cljc:44`) and medley 1.8.1 (`core.cljc:456`) both
have `[… x #?(:clj … :cljs …)]` in a let binding; eliding it made them classify
as USABLE and then fail to compile with "let* requires an even number of forms
in binding vector" — a compiler error naming a library the user never wrote.
New opt-in reader option `WithStarvedCollError` makes that `R1012` at any
depth. The top-level rule is now "is anything LEFT" rather than "is there a
starve": tools.cli's single `#?(:cljs (defn- format …))` helper is elided as
JVM Clojure elides it, and REPORTED in the resolve report.

### Still not working, stated plainly

- **medley 1.8.1 does not load** (`core.cljc:456` binds
  `#?(:clj (java.util.ArrayList.) :cljs (array-list))` in a `let`). This is a
  TRUE verdict now, not the bogus `::none` read error it used to be, but
  medley is no longer in the "fully consumable" column.
- **camel-snake-kebab 0.4.3 is 5 usable / 1 not** —
  `internals.string-separator` is genuinely JVM/JS-only.
- **Parent POMs are not hashed into `build.lock.edn`.** Their influence is
  only through edges that are themselves locked coordinates.
- Only the `<parent>` merge of a NON-vendored dep is exercised: the
  `vendor/<name>/` short-circuit still skips POM-derived transitive edges,
  which pre-dates this pass.
- **`com.stuartsierra/dependency 1.0.0` classifies and LOADS but does not RUN.**
  `(dep/depend (dep/graph) :b :a)` fails at runtime with "No implementation of
  method: depend of protocol: com.stuartsierra.dependency.DependencyGraphUpdate
  found for: user.MapDependencyGraph". This is a PRE-EXISTING cljgo bug with
  no Maven involvement: a `defrecord` in a REQUIRED namespace registers its
  type under `user.` instead of the defining namespace, so protocol dispatch
  misses. Reproduced on a two-file local project with no dependencies at all
  (`(ns lib.core) (defprotocol P …) (defrecord R …)` required from `app.main`
  => "No implementation of method: go of protocol: lib.core.P found for:
  user.R"). Out of scope for this pass; filed here because "1 namespace
  usable" is a LOAD verdict and must not be read as "the library works".
