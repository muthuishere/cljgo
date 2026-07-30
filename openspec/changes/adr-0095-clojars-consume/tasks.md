# Tasks — adr-0095-clojars-consume

Gate (every commit): `CGO_ENABLED=0 go build ./... && go vet ./... && gofmt -l pkg cmd conformance templates core && go test ./... -timeout 1800s -p 1`
`core/build.cljg` changes require `go generate ./pkg/coreaot ./pkg/briaot`, committed.
**No committed test may touch the network** (task 6.3 enforces it).

## 1. Diagnostics first (so nothing on this path is ever a bare error)

- [ ] 1.1 Append `R1012`, `I4002`, `G5010`–`G5015` to `pkg/diag/registry.go` and `docs/diagnostics/registry.lock` (append-only; do not renumber).
- [ ] 1.2 Write `docs/diagnostics/{R1012,I4002,G5010,G5011,G5012,G5013,G5014,G5015}.md` — each: what happened, why cljgo cannot do it, the fix, a worked example.
- [ ] 1.3 Test: every new code has an explain page and the registry stays band-valid and monotonic.

## 2. The Maven fetcher (pure Go stdlib only)

- [ ] 2.1 `pkg/deps/pom.go` — `encoding/xml` pom parse: coordinate, `<dependencies>`, `<scope>`, `<optional>`, `<exclusions>` (incl. `*`).
- [ ] 2.2 `validate(pom)` — detect and `G5011` on `${property}`, `<dependencyManagement>`, `<parent>`, ranges, `-SNAPSHOT`, `<profiles>`, `<classifier>`, non-jar packaging. Name the feature AND the coordinate.
- [ ] 2.3 `pkg/deps/maven.go` — repo list (Clojars, Central; `mvn-repo` prepends), `net/http` GET of pom/jar/`.sha1` with timeouts, retry-once, and a named diagnostic for every transport failure. `G5010` on 404 across all repos.
- [ ] 2.4 Jar extraction with `archive/zip`: keep `.clj/.cljc/.cljs/.edn`, drop `.class` + `META-INF/`, reject zip-slip and absolute paths, bound entries (10k) and size (32 MiB).
- [ ] 2.5 Cache: `MvnIdentityKey`, `<root>/mvn/…` raw bytes + `<root>/src/<key>` extracted tree, `publishAtomically` + `markReadOnly` + `TreeHash`, `withLock` around the fetch. `G5012` on any hash mismatch.
- [ ] 2.6 BFS graph walk with first-wins + `G5013` conflict (naming both requirers), `accept-version` keyed on `"group/artifact"`, and the `CLOJURE_ITSELF` prune reported at `-v`.

## 3. Wire into the one resolver

- [ ] 3.1 `Dep.MvnVersion` + `isMvn()`; `G5015` when combined with `:git`/`:path`; `resolveOne` three-way switch; `resolveMvn` alongside `resolveGit`/`resolvePath`.
- [ ] 3.2 Lock: `LockedDep` gains `Mvn*` fields + `:mvn/namespaces`; extend `LoadLock`/`WriteLock` (deterministic, sorted); a git-only lock still round-trips byte-identically (regression test).
- [ ] 3.3 Offline matrix (design §1.3) implemented and table-tested: warm/cold cache × locked/unlocked × `-offline`, each landing on `G5014` or success — never a network call under `-offline`.
- [ ] 3.4 `ResolveOptions` gains an injectable repo list + HTTP client so tests drive the production path.
- [ ] 3.5 `core/build.cljg`: `dep` docstring covers `:mvn/version`; add `mvn-repo`; `pkg/build` threads both through. Regenerate coreaot/briaot.

## 4. The per-namespace Java gate

- [ ] 4.1 Extract `pkg/publish/java.go` → `pkg/javadetect` (one implementation, both directions). `pkg/publish` keeps its behaviour byte-identical — existing publish tests must not move.
- [ ] 4.2 Extend the classifier with the consume-side surfaces: `(:import …)`/`(:gen-class)` in `ns`, `definterface`, `gen-class`, `proxy`, `reify`/`extend-*` onto `java.*`. Still zero-false-positive: bare `(.m o)`, class-refs, `instance?`, `catch` MUST NOT flag (assert in tests).
- [ ] 4.3 Classify every namespace at resolve; write `:mvn/namespaces {:pure … :java …}` into the lock; `cljgo resolve` prints "N usable, M require Java" per Maven dep and warns loudly on an all-Java dep.
- [ ] 4.4 Record maven-origin roots; gate at file-read in the single resolver so REPL and AOT legs fire the same `I4002`. Test the identical message from `cljgo run` AND a compiled binary (divergence = release blocker).
- [ ] 4.5 Prove the mixed-jar case end-to-end on the hiccup-shaped fixture: 8 usable, 2 loud.

## 5. Reader conditionals

- [ ] 5.1 `pkg/reader`: `WithStarvedCondError` option raising `R1012` (file:line:col, Expected `:cljgo, :default`, Found the branches present). Default semantics unchanged — prove by running the existing reader + conformance suites untouched.
- [ ] 5.2 Enable it only for maven-origin files; a whole-file starved `.cljc` fails at require, never installs an empty namespace.
- [ ] 5.3 Corpus `pkg/deps/testdata/readcond/` + table test — all 15 design §4.2 cases, including the string/char/comment/`#_` traps and the nested-branch cases.
- [ ] 5.4 Conformance twins for the oracle-able subset (`:clj` substituted for `:cljgo`), verified against real Clojure 1.12.5 and cited; cljgo-specific cases frozen with a written rationale. Dual harness.

## 6. Fixtures and the no-network guarantee

- [ ] 6.1 `pkg/deps/testdata/repo/` Maven-layout tree + `httptest` server; fixture jars **built in-process** by the test (no binary jars committed).
- [ ] 6.2 Fixtures mirror the s50 shapes: pure 1-ns, mixed 8+2, all-Java, fenced-`.cljc`, `${property}`, and a 4-deep transitive graph.
- [ ] 6.3 `TestNoNetworkInDepsTests` — a `RoundTripper` guard that fails any request not aimed at the test server.

## 7. Docs and honesty

- [ ] 7.1 `docs/…/deps-publish.md`: drop "consuming … is deferred"; the new wording is **"consume pure-Clojure libraries; everything else fails loud, per-namespace"**. Never "consume the Clojure ecosystem".
- [ ] 7.2 A consume guide page: the coordinate syntax, the offline story, how to read the `cljgo resolve` purity report, and the s50 measurement quoted honestly (2 fully / 4 partial / 1 unusable of 7 sampled; utility libraries reach, Java-wrapping libraries do not) with the sample size stated.
- [ ] 7.3 Update ADR 0095 status to `accepted` for decision 1/2 and note that decision 3 (deploy) rides a separate change.
- [ ] 7.4 Close-out: full gate green; archive this change.
