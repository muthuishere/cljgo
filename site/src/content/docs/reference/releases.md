---
title: Releases & changelog
description: Every tagged cljgo release, newest first — what shipped, what broke, and what was measured. Sourced from the annotated git tags, so this page and the GitHub release notes cannot drift apart.
---

Every release is an annotated git tag, built by goreleaser into binaries for
six platforms (linux/macOS/Windows × amd64/arm64) plus `checksums.txt`. This
page is the same content as the
[GitHub releases](https://github.com/muthuishere/cljgo/releases), distilled —
the tag annotation is the single source, so the two cannot drift apart.

Install the newest with the instructions on [Install](/cljgo/install/), or grab
a specific version's assets from the release page linked in each heading.

:::note[Versioning, honestly]
cljgo is pre-1.0 and the version has no stability promise attached yet.
Minor bumps carry features and may adjust behavior; patch bumps are fixes
and measurements. What *is* promised, at every version, is the dual-harness
contract: **a program must behave identically under `cljgo run` and as a
compiled binary.** REPL-vs-binary divergence is a release blocker, not a
known issue.
:::

## [v0.8.9](https://github.com/muthuishere/cljgo/releases/tag/v0.8.9) — 2026-08-01

*A dev build claimed to be a release — and quietly built against the
published runtime instead of your working tree.*

- **A local `go build` reported as a release, and `cljgo build` then compiled
  against the PUBLISHED runtime.** Go stamps `BuildInfo.Main.Version` from the
  nearest VCS tag even for a plain local build inside the repo, so once
  v0.8.5 was tagged, `IsRelease()` was true for every dev build (a regression
  from ADR 0116, v0.8.5).

  The wrong version line was the visible half. The other: a true `IsRelease()`
  makes the generated `go.mod` pin `require …/cljgo v0.8.5` with **no
  `replace`** — so anyone building a project with their own locally-built
  cljgo silently compiled against the released runtime, with changes to
  `pkg/lang`, `pkg/emit` and everything else ignored and nothing to indicate
  it. It stayed hidden because CI and normal development set `CLJGO_SRC`,
  which outranks the pin.

  The discriminator is exact: `go install module@vX.Y.Z` resolves through the
  module proxy and records a checksum but **no** `vcs.*` settings, while any
  build from a checkout records `vcs=git`. VCS data means built-from-source,
  which is what a release is not. (#189)
- **`cljgo version` in a git worktree — an honest hook, not a fake fix.** This
  cannot be fixed at runtime: `debug.BuildInfo` records no build directory,
  the binary may run anywhere, and a worktree build is indistinguishable from
  a main-checkout build from the inside. The truth exists only at build time,
  so there is now a hook for it:

  ```bash
  go build -ldflags "-X github.com/muthuishere/cljgo/pkg/version.DevCommit=$(git rev-parse HEAD)"
  ```

  Set, it wins over buildvcs; unset, nothing changes. Releases are unaffected.
  The misleading `module vX.Y.Z` fragment is also gone for source builds — it
  means *"the user asked for @vX.Y.Z"*, and Go's nearest-tag stamp is a
  different claim. (#180)
- **Non-UTF-8 Maven POMs parse.** `parsePOM` had no `CharsetReader`, so
  `encoding/xml` refused any declared encoding other than UTF-8 and the Go
  error leaked verbatim into a `G5011` saying *"the repository served
  something that is not a Maven POM"*. It is one — cljgo simply refused to
  read it, and nothing about the coordinate was wrong. Real artifacts hit
  this (`commons-parent`).

Worth recording how #189 survived: the test fixture paired a module version
**with** `vcs.revision` — a combination that never occurs. The two cases are
told apart by exactly the VCS data that fiction gave to both.

## [v0.8.8](https://github.com/muthuishere/cljgo/releases/tag/v0.8.8) — 2026-08-01

*One fix, and it is the kind you cannot see.*

- **Maven Central is now searched before Clojars, matching tools.deps.** The
  default list was Clojars-then-Central, with a comment claiming that *was*
  the tools.deps default. It is not: tools.deps returns "central, then
  clojars, then other repos" unconditionally
  (`clojure/tools/deps/util/maven.clj:161-165`).

  cljgo fetches from the first repository that answers, so a coordinate
  published to **both** resolved to a **different artifact** on cljgo than on
  the JVM — with no conflict, no diagnostic, and nothing in the project files
  to hint at it. For a dual-host `.cljc` library this is the worst possible
  failure: the promise is identical behaviour on both hosts, and this broke it
  *below the level anyone would think to look*. You would be debugging a
  behavioural difference in your own code while the cause was which jar got
  fetched.

  `(mvn-repo …)` still prepends, so a project can override. The order is now
  pinned by a test citing the tools.deps source, so it cannot drift back.
  (#187)
- **`build.cljgo` beats `deps.edn` from anywhere in the search.** ADR 0119's
  precedence was applied per directory, so `cljgo run /elsewhere/foo.cljc`
  could take `/elsewhere`'s `deps.edn` and return before reaching a
  `build.cljgo` in the working directory. Where a cljgo build file exists,
  `deps.edn` is now not consulted at all.

Found by spike `s79` while surveying what `deps.edn` `:deps` translation
would cost — reading `:deps` does not cause it, it only exposes it. The
consequence is fetch-time and needs the network to demonstrate, so this
shipped as a source-to-source argument plus a pinned invariant, not an
observed mis-resolution.

## [v0.8.7](https://github.com/muthuishere/cljgo/releases/tag/v0.8.7) — 2026-08-01

*The REPL could not see your project. Both halves of that, plus the design
change that stops it recurring.*

- **`cljgo repl` and `cljgo nrepl` resolved nothing from the project.**
  `(require 'myproj.core)` failed in a project whose own `cljgo run` and
  `cljgo build` resolved it — including in a freshly generated `cljgo new`
  project, which could not require its own namespace in its own REPL. A
  REPL-vs-run divergence, the class ADR 0007 calls unforgivable. `require`
  was never broken: it was searching an empty list, because neither entry
  point established the load path. The nREPL case is the worse one — that is
  the editor path, so it was invisible from a shell and landed on anyone
  using an IDE. (#185)
- **Resolution now happens once, before the subcommand dispatch** — five
  scattered call sites became one, so a new entry point cannot forget it.
  The shape is taken from reading the alternatives: JVM Clojure has no
  in-process bootstrap at all (the classpath is fixed by an external launcher
  before the JVM starts, and `clojure.main/repl` touches the classloader only
  to *widen* it, never to establish it); let-go installs one resolver before
  mode dispatch; Glojure drains its classpath into one global load path as
  the first statement of `main`. (ADR 0118)
- **`deps.edn`'s `:paths` is honoured when a project has no cljgo build
  file.** `.cljc` is the dual-host mechanism, so the projects cljgo most
  needs to work with declare their roots in Clojure's own file and have no
  reason to carry a second one. `build.cljgo` still wins where both exist.
  Narrow on purpose: `:paths` only, vector form only, parsed as data — not
  `:deps`, not aliases, which carry tools.deps semantics cljgo does not
  implement. (ADR 0119)
- **A conformance test was racy, not flaky.** `agent-lifecycle` failed about
  one full run in N on the interpreted leg and passed 12/12 when hunted in
  isolation. The cause was the test using an error handler as a
  synchronisation point for a state transition that happens *after* it —
  racy on both hosts. cljgo's ordering matches the JVM exactly
  (`Agent.java:130-142`); the first hypothesis, that this was a runtime
  divergence, was wrong.
- **Two pieces of pure waste deleted**, both independent of the above:
  `filepath.Dir("")` and `filepath.Dir("rel.clj")` are *both* `"."`, so the
  common shapes resolved the same directory twice (74 ms on the `cljgo new`
  layout), and `build.LoadPlan` ran twice per pass for a plan that was
  discarded. Measured: `cljgo run` in a dep-free project 202.7 ms → 89.4 ms.

Measured and recorded, not acted on: project resolution boots a whole
tree-walking interpreter (38 ms, 54 MB, 875k allocs) to read a build file,
and that constant does not scale with dependency count — a dep-free project
is the *worst* case. Spikes `s72` and `s78`.

## [v0.8.6](https://github.com/muthuishere/cljgo/releases/tag/v0.8.6) — 2026-08-01

*The AOT test path did not work for dual-host projects, and the docs
recommended an unsafe idiom. Both found by consumers.*

- **`cljgo test --compiled` could not run a `.cljc` suite.** `nsNameFor`
  derived a namespace symbol by dropping the file extension using its own
  hand-written list, which trimmed only `.cljg` and `.clj`. So
  `test/demo/core_test.cljc` became the namespace `demo.core-test.cljc` — the
  extension turned into a name segment — and the compiled leg required a name
  that cannot exist. It failed in the direction that looks fine: `cljgo test`
  green, `--compiled` unable to build, so **anyone treating `--compiled` as
  their AOT test path silently had no AOT coverage at all.** That is the
  REPL-vs-binary divergence ADR 0007 calls unforgivable, occurring in the
  harness meant to detect it. `.cljgo` was broken identically — found by the
  regression test, reported by nobody. `pkg/eval.SourceExts` is now the single
  list both the resolver and the test harness read. (#182)
- **`:exit-code` had an EOF race, and cljgo's own comment taught it.** The
  docs said `nil` is falsey so `(if ((:exit-code p)) …)` reads as "has it
  finished". It does not: `nil` means **not yet reaped**, and reaping happens
  strictly after the child's stdout reaches EOF. Draining `:out` then reading
  `:exit-code` reports `nil` for a process that has already exited — measured
  at 5 runs of 5 on `sh -c 'echo hi >&2; exit 3'`, correct on all 5 after
  250 ms. The framing had already propagated into a consumer library's
  docstring. The semantics were always right; the recommended *read* was not.
  **Use `:wait` after draining** — it returns as soon as the reaper has the
  status and cannot report `nil`. `:exit-code` is for polling a child you are
  not draining.
- **`G5026`** replaces `namespace X not found after loading its source`, which
  named the wrong condition: the file *was* found and evaluated, it just never
  defined the namespace (no `ns` form, or one disagreeing with its path).
  "Not found" sent readers hunting for a file sitting right where it should be.
- **Release notes no longer contradict the release.** `.goreleaser.yaml`'s
  Install footer is injected into every release body and claimed `cljgo build`
  needs a checkout of this repo and `CLJGO_SRC` — the exact opposite of what
  v0.8.5 fixed. The published v0.8.5 body is corrected in place with a visible
  note rather than silently overwritten.

## [v0.8.5](https://github.com/muthuishere/cljgo/releases/tag/v0.8.5) — 2026-08-01

*Two defects that made a released cljgo unusable, both found by consumers
while the gate stayed green.*

- **A `go install`ed cljgo refused to build anything.**
  `go install github.com/muthuishere/cljgo/cmd/cljgo@v0.8.4` produced a binary
  that failed on every project with *"this is a dev cljgo binary (version
  0.1.0-dev), so the generated go.mod needs a local runtime tree"*.
  `IsRelease()` consulted only the version goreleaser stamps via ldflags, which
  `go install` never sets — so the release-pin path was skipped and a binary
  whose whole promise is needing no source checkout demanded one.
  **`templates/web/Dockerfile` installs cljgo exactly that way, so the shipped
  web template could not build in Docker at any release.** The requested tag was
  never missing: Go records it in the binary, and `pkg/version` already read
  that field to print the provenance line — the line was right, the gate was
  wrong. (ADR 0116)
- **A JVM `build.clj` broke every `cljgo run` in the project.** cljgo accepted
  `build.clj` as one of *its own* build-file names. That was harmless while only
  `cljgo build` read the build file; v0.8.3 put it on the `cljgo run` path, so
  cljgo began **evaluating another tool's build script**, and its
  `(:require [clojure.tools.build.api])` is unresolvable here. `build.clj` is
  the tools.build convention and the only way a dual-host library publishes to
  Clojars — so this blocked exactly the projects cljgo exists to serve, across
  v0.8.3 and v0.8.4. cljgo now probes `build.cljgo` then `build.cljg` only.
  **Breaking** if you named a cljgo build file `build.clj`: rename it.
  (ADR 0117)
- **`:exit-code` on `cljg.process/spawn`** — the non-blocking counterpart of
  `:wait`, and the half of the #173 report that shipping only `:alive?` left
  open. It separates *"exited with status N"* from *"still running"* without
  blocking, which a closed stdout alone cannot express. `nil` while running, so
  `(if ((:exit-code p)) …)` reads as "has it finished".
- **`G5025`** — a build file with no `(defn build [b] …)` reported
  `compiler error at <build-driver>:1:38`, naming a file cljgo synthesizes and
  giving a column *into that nonexistent file*, while never naming the build
  file the user wrote. It now names the file, the missing form, and a fix.
- **Subprocess output is now asserted complete and prompt.** Two truncation
  bugs shipped this cycle because nothing checked either property. New tests
  drive output far past any pipe buffer through `cljg.io/exec` and
  `cljg.process/spawn` and assert exact byte counts. Measured limitation,
  recorded in the test file: reverting the fix, they **pass** on fast hardware —
  only with a slowed reader (what a slower machine amounts to) do they fail, at
  `167936` bytes against `208894` expected. A green local gate is necessary and
  never sufficient here.

## [v0.8.4](https://github.com/muthuishere/cljgo/releases/tag/v0.8.4) — 2026-07-31

*Eight consumer-reported defects, and a `clojure.core` that is now measured
against the JVM instead of assumed.*

Every item here came from a real project using cljgo, filed with a repro.

- **`clojure.core` no longer carries names JVM Clojure lacks.** The ADR 0014
  Result/Option family (`ok`, `err`, `just`, `none`, …) was interned straight
  into `clojure.core` and auto-referred everywhere, so a user who legally wrote
  `(defn ok [x] …)` got a shadow warning that **cannot happen on the JVM**.
  It now lives in `cljx.meta`, required deliberately. `try`/`catch` remains the
  idiom for handling failure, exactly as on the JVM. (ADR 0115)
- **Core parity is now a measured, enforced property**, not a claim. A ratchet
  test diffs cljgo's `clojure.core` against the **real** JVM 1.12.5 public var
  set in both directions and fails on any new divergence — and on any *closed*
  one, so improvements get locked in. Today: 691 cljgo publics against the
  JVM's 679, 59 cljgo-only, 47 missing. Most of the 59 are internal helpers
  that are simply missing `:private`.
  - It immediately caught `nan?`, a lowercase alias of the JVM's `NaN?` with
    both spellings live. Removed: teaching a spelling that does not port is
    exactly what the precedence principle forbids.
- **`cljg.io` errors read as Clojure**, with the raw Go text preserved in
  `ex-data` under `:go/error` so nothing is lost. No more doubled
  `op "path": syscall path: reason` shapes or syscall names the caller never
  invoked. (ADR 0114)
- **`cljg.io/exec`'s `:timeout-ms` now bounds the CALL, not just the process.**
  A command that forked a grandchild held the output pipes open, so a killed
  `sleep 5` still returned ~16× past its deadline. A timeout that does not
  bound the call is a broken promise; measured 5.02 s → 0.35 s against a
  300 ms deadline.
- **`cljg.process` gains `:alive?`** — a non-blocking liveness check. `:wait`
  was the only option, so "is the child alive?" was unanswerable without
  running your own reaper.
- **`cljg.io/real-path`** resolves symlinks and refuses cycles with `G5024`.
  `absolute` is purely lexical and could not guard either.
- **`clojure.core/load-file`** was interned with a `nil` root: it resolved,
  looked supported, and died on "cannot call nil". Implemented for the
  interpreter; compiled binaries now say plainly that they have no reader
  rather than failing obscurely.
- **A fresh clone tells you what is wrong.** `cljgo run` in a project whose
  dependencies were never resolved raised a bare "could not locate namespace";
  it now raises `G5023` naming the build file and telling you to run
  `cljgo build`.
- **`cljgo version` identifies the build.** Every unstamped binary reported
  `0.1.0-dev`, so a bug report could not be tied to a commit. Dev builds now
  carry their commit and dirty flag; released binaries still show the clean tag.

## [v0.8.3](https://github.com/muthuishere/cljgo/releases/tag/v0.8.3) — 2026-07-31

*Two build defects that shipped green-looking wrong answers, and a benchmark
claim that did not reproduce.*

- **A `test/` tree was invisible to `cljgo run` and `cljgo build`.** A namespace
  resolved only relative to the requiring file, so `test/app/core_test.cljg`
  requiring `app.core` looked for `test/app/core.cljg` and failed — and the
  error named the *namespace*, not the paths tried, so the cause was close to
  unguessable. The only workaround was moving the suite under `src/`, which
  dual-host `.cljc` projects cannot do.

  Projects now have **source roots**, defaulting to `src` and `test` (whichever
  exist), declarable with `(paths b ["src" "spec"])`. They are published even
  when a project has no dependencies — the old code bailed before setting any
  roots if there was no `build.lock.edn`. Roots append *after* the requiring
  file's own directory, and providers still outrank every root, so `clojure.*`
  can never be shadowed.
- **Two `(exe …)` artifacts in one `build.cljgo` corrupted the second binary**,
  whichever it was. cljgo's namespace registry is process-global, so the second
  build saw the first build's namespaces already interned, never loaded them,
  and emitted a program with those namespaces **missing** — their vars interned
  as hollow shells and unbound at runtime. The first binary was fine, so a
  project building an app plus a test runner shipped a **working app and a test
  suite that could not run**. Both declaration orders are now covered by tests
  that drive the real binary against real project trees.
- **The published bri web-framework benchmark did not reproduce, and the page
  now says so.** It claimed 78,126 req/s; re-run at v0.8.2 on the same machine
  class bri measures **31,404**. clj-httpkit re-measured within 6% of its own
  published figure in the same session, so the ~2.4× movement is ours. The
  claims of "comparable-or-better throughput" and "top tier with
  Rust/Deno/Bun/http-kit" are withdrawn; the structural wins (~42× smaller
  image, ~40× faster cold-start, an order of magnitude less memory) stand.
  See [benchmarks](/cljgo/reference/benchmarks/).
- **Performance, language-wide.** Hash-map reads no longer allocate (`find`
  was heap-allocating an entry per lookup, and the hot `get` path discarded
  the key immediately); keyword hashes are cached at intern time rather than
  recomputed on every map operation (**2.3×** on keyword map lookup); and
  bulk hash-map construction now builds through a transient like Clojure's own
  `PersistentHashMap.create` (**3.4×** faster with **5.7×** fewer allocations
  at 128 entries).
- Request-path work in bri — a pre-sized body read (256 KiB: 2× faster, 41%
  less memory, allocations now flat regardless of payload) and a needless
  vector construction removed from JSON encoding. Worth knowing: this made
  `requestMap` 2.3× faster and moved end-to-end throughput **0–2%**. The
  request path was never the bottleneck, and the benchmarks page says that
  too.
- The web benchmark suite is now in the repo at
  [`benchmark/web/`](https://github.com/muthuishere/cljgo/blob/main/benchmark/web/README.md)
  — runner, corpus and Dockerfiles — and the bri image builds cljgo from your
  checkout rather than a release. Promoting it revealed the old corpus no
  longer compiled at all.

## [v0.8.2](https://github.com/muthuishere/cljgo/releases/tag/v0.8.2) — 2026-07-31

*A silent test-runner failure, a lockfile that dead-ended, and portable date
patterns.*

- **`cljgo test` silently skipped every `.cljc` file.** It reported
  `Ran 0 tests containing 0 assertions` and **exited 0** — a green build that
  had run nothing. If your project keeps its tests in `.cljc` (the portable
  choice this project recommends), CI has been passing without testing
  anything. This is the reason to upgrade.
- **Editing a dependency version no longer dead-ends the build.** Bumping a
  version in `build.cljgo` used to stop with
  `note: run resolve with -update to re-pin` — naming a command that has never
  existed, leaving `rm build.lock.edn` as the only findable remedy, which also
  discards every unrelated pin. The lock now **refreshes itself**, and does so
  **minimally**: what you bumped moves, and nothing else does. (ADR 0112)
- **`cljgo build --locked`** (or `CLJGO_LOCKED=1`) makes a stale lock an error
  instead of a refresh — the `npm ci` / `cargo --locked` position, for the case
  where a merge takes `build.cljgo` from one branch and `build.lock.edn` from
  another. Refuses with `G5021`, naming what diverged, and writes nothing.
- **`cljg.date/format-pattern` and `parse-pattern`** take a **java.time
  pattern** — `"yyyy-MM-dd HH:mm:ss"` means the same thing here and on the JVM,
  so a `.cljc` library's `:clj` branch needs no translation at all. Verified
  against a 4,000-pattern JVM differential corpus: **0 divergences, 0 patterns
  accepted that the JVM rejects.** Tokens with no exact equivalent are refused
  by name (`G5022`), never approximated. The existing Go-layout `format` /
  `parse` are unchanged. (ADR 0113)
  - Locale is **ENGLISH**, and the docs say so explicitly — *not* `ROOT`, which
    collapses `MMMM` to `Jul` and `EEEE` to `Fri` and would diverge silently on
    exactly the tokens the advice protects.
- **15 error codes that existed but were undocumented** are now on the
  [diagnostics page](/cljgo/reference/diagnostics/), with a test that fails if
  the registry and the published table ever drift apart again.
- Emit-comment truncation could split a multi-byte rune, producing invalid
  UTF-8 in generated Go.
- **Benchmarks re-measured at this version.** The AOT head-to-head was re-run
  as one fresh session — cljgo and both competitors re-timed together. Glojure
  still wins 6 of 8 rows; cljgo still takes `tak` and `fib`; the binary is
  **byte-identical** to v0.8.1 (7,049,666 B), because this release's code is
  build-time only or lives in `pkg/bri`, which a plain AOT program does not
  link. cljgo still starts slower than Glojure (5.1 ms vs 3.9 ms). See the
  [benchmarks page](/cljgo/reference/benchmarks/) — and note the caution
  there about not diffing timings across sessions. The other tables on that
  page still predate v0.7.0 and continue to say so.

## [v0.8.1](https://github.com/muthuishere/cljgo/releases/tag/v0.8.1) — 2026-07-31

*Clojars consumption works, and there is now a test that proves it.*

- **Consuming pure-Clojure libraries from Clojars works end to end.** Declare
  `(dep b "group/artifact" {:mvn/version "V"})` in `build.cljgo`, build in
  **project mode** (no file argument), and the POM chain, transitive walk,
  jar extraction and load path all resolve. Proven by running
  `com.stuartsierra/dependency 1.0.0` as a compiled binary. See
  [Dependencies & publishing](/cljgo/guides/deps-publish/).
- **Libraries that genuinely need the JVM are refused per namespace**, with a
  coded and located diagnostic — `I4002` for real Java interop, `R1012` for a
  starved reader conditional — rather than half-loading.
- **A network integration test** (`CLJGO_CLOJARS_IT=1`) now checks both
  directions against the live repositories, so this class of defect cannot
  reach a release unnoticed again. It is skipped by default: a green build
  never depends on Clojars being up.
- Five defects found only by taking the resolver to real libraries: `defn`
  attr-map, `defrecord` namespace qualification, nested starved conditionals,
  doubled error notes, and lockfile write ordering.
- **Benchmarks re-measured at this version.** Glojure AOT still wins 6 of 8
  rows; startup regressed 4.7 → 6.6 ms and the binary grew to 7.0 MB. Both
  are on the [benchmarks page](/cljgo/reference/benchmarks/) rather than
  buried.

## [v0.8.0](https://github.com/muthuishere/cljgo/releases/tag/v0.8.0) — 2026-07-30

*Clojars, the Go standard library, and bytes/dates.*

- **Consume from Clojars** (ADR 0095). Coordinate → transitive `.pom` walk →
  jar → extraction → load path → `require`. Pure Go (`net/http` +
  `archive/zip` + `encoding/xml`) — no JVM, no `mvn`, no Aether. Honest
  scope, stated in the docs: **pure-Clojure libraries**. The reachable set is
  utility and algorithm libraries, not the Java-wrapping mainstream. Of seven
  sampled live: 2 fully consumable, 4 partial, 1 unusable.
  - Gating is **per namespace**, not per library — one jar routinely mixes
    both (hiccup is 8 pure + 2 Java), so a whole-library gate is wrong in
    both directions.
  - Reader conditionals classify on **reader output**, not a text scan, so
    they are immune to `#?(` inside strings and comments. A `.cljc` whose
    real body is `:clj`-only fails loud (`R1012`) instead of silently loading
    an empty namespace.
  - Unimplemented Maven features (`${property}`, `dependencyManagement`,
    parent, ranges, SNAPSHOT, profiles, classifiers) **name-error** rather
    than half-resolve.
- **`require-go` reaches the Go standard library** (ADR 0109).
- **Bytes, dates, base64, and two core parity bugs** (ADR 0110).
  `cljg.io`/`cljg.stream` byte-level I/O, `cljg.date` ISO format and parse,
  base64. `(str (random-uuid))` is the bare 36-character form again — it had
  been leaking the `#uuid` tag into every id on the wire, silently; `pr-str`
  keeps the tag, and both the split and the round-trip are frozen.
  `clojure.string/replace` and `replace-first` accept a function
  replacement, with the JVM's contract for what the fn receives.
- Fixes: a Windows-only file-handle leak in `cljg.stream` (POSIX unlinks open
  files, so two platforms hid it and only `windows-latest` caught it);
  anchored and zero-width regex parity in `str/replace` on both the fn and
  the `$`-template path; byte routes agree on sign and producers compose with
  consumers.

Every fix in this cycle was checked by an adversarial verifier that re-derived
the JVM oracle itself. That caught three defects the fixes introduced, all
closed before the tag.

## [v0.7.0](https://github.com/muthuishere/cljgo/releases/tag/v0.7.0) — 2026-07-28

*The extensions tier, and all eleven known issues closed.*

- **`cljx.*` — the extensions tier.** `cljx.core` is opt-in ergonomics
  (`add!`/`bump!`/`del!`/`dbg`) as transparent aliases whose docstrings name
  the `swap!` form they replace (ADR 0106). `cljx.test` is Bun-flavoured
  testing layered on `clojure.test`: mocks, spies, stubs, output capture by
  default (ADR 0105).
- **`cljg.*` wave 1** (ADR 0103): `cljg.http/serve`, `cljg.socket`,
  `cljg.net.dns`, `cljg.compress`, `cljg.security`, `cljg.system/sleep`. All
  pure Go, `CGO_ENABLED=0`, and **lazily registered** — a binary that never
  requires them pays zero bytes.
- **Performance.** ADR 0067's numeric inference gained a second op table
  (`quot`/`rem`/`bit-*`/`unchecked-*`/predicates). One untyped op used to
  demote an entire carrier graph, so closing the table re-enables whole
  regions: collatz 1956.6 → 32.3 ms, bitmix 663.6 → 41.0 ms, digit-sum
  146.0 → 13.9 ms, gcd 343.6 → 33.8 ms — for +192 bytes of binary. ADR 0064
  added cross-var direct calls (~1.15× on call-heavy code, +1.5% binary).
  ADR 0108's `--seal-core` is **opt-in only**: it measured ~1–2% on micros
  and nothing end-to-end, so it is not the default — it exists for the JVM's
  `:inline` semantics, not for speed.
- **All 11 known issues closed**: `clojure.core.match` map patterns across
  differing key sets; `for` `:when`/`:while`; fn metadata; `extend-protocol`
  on qualified class names; compiled test binaries exiting 0 on a red suite;
  `.getMessage` on host errors; build-vs-run diagnostic parity; `sleep`; a
  public `*out*` writer.

## [v0.6.0](https://github.com/muthuishere/cljgo/releases/tag/v0.6.0) — 2026-07-24

*Opt-in OpenTelemetry.*

[`bri.otel`](/cljgo/bri/otel/) (ADR 0074) adds distributed tracing as an
**opt-in** namespace: a server span per request (named by route pattern), W3C
`traceparent` adopt and inject, an OTLP exporter driven by the `OTEL_*` env
vars, `service.name` from config, request-id and subject as span attributes,
correlated with the existing Prometheus metrics and structured logs.

Zero cost when unused, and proven so: it is linked **only** when the app
requires `bri.otel`, and a plain `bri.http` binary carries zero OpenTelemetry
symbols while a `bri.otel` binary carries the SDK. Pure Go, `CGO_ENABLED=0`
static, dual-mode byte-identical.

## [v0.5.0](https://github.com/muthuishere/cljgo/releases/tag/v0.5.0) — 2026-07-24

*First-class bri — the one-person web framework, end to end.*

```bash
cljgo new --template web blog && cd blog
cljgo generate resource Note title:string body:text
cljgo test        # green: a working, authenticated, DB-backed CRUD
docker build .    # a ~15 MB static-binary image
```

- [`bri.db`](/cljgo/bri/db/) (ADR 0072): a pure-Go data layer — modernc
  SQLite (the zero-install default) plus pgx/Postgres, parametrized queries,
  transactions, migrations. AOT-compiled and interpreted, byte-identical,
  `CGO_ENABLED=0` static.
- [The resource generator](/cljgo/guides/generate/) (ADR 0073):
  `cljgo generate resource` scaffolds migration + model + handlers + routes +
  a green CRUD test.
- The conformance suite got ~5.3× faster (237 s → 45 s) via a batched
  compiled harness.

## [v0.4.0](https://github.com/muthuishere/cljgo/releases/tag/v0.4.0) — 2026-07-24

*bri goes to production.*

The flagship web framework AOT-compiles to a single static `CGO_ENABLED=0`
binary, deployable as a ~15 MB Docker image (ADR 0071), while the interpreter
path stays byte-identical.

Measured against the field (Docker, `oha`): compiled `bri.http` at ~78k req/s,
p99 1.4 ms, ~16 MB RSS, ~30 ms cold start — top tier alongside Rust/Deno/Bun,
ahead of Go `net/http`, Node, .NET, Spring Boot and FastAPI; and against JVM
Clojure web, ~55× smaller image, ~40–55× faster start, ~22–30× less memory.
Reproduce with `spikes/s45-bri-aot-docker/bench/run.sh`.

## [v0.3.1](https://github.com/muthuishere/cljgo/releases/tag/v0.3.1) — 2026-07-24

*REPL session resume from the command line* (ADR 0070).

`cljgo repl :resume <#|id>` (and a bare `cljgo repl <id>`) resumes a saved
session on boot — completing the CLI leg ADR 0016 promised but never shipped,
so the farewell line's command now actually works. With no id,
`:resume`/`:sessions` list a numbered, newest-first table; resume by the short
number. Sessions record their working directory and resume `cd`s back into it,
so `require`/`load` resolve as they did.

## [v0.3.0](https://github.com/muthuishere/cljgo/releases/tag/v0.3.0) — 2026-07-23

*Performance, fundamentals, and the web API tier.*

- **Perf campaign** (ADRs 0063–0067): emitted code went from ~35× to ~4.8× of
  handwritten Go; the AOT binary won every recursion and data-structure
  benchmark row outright and tied startup at 5.0 ms.
- **`clojure.core` complete**: 632/679 oracle vars (93%), with the other 47
  documented as JVM machinery; every satellite namespace at 100%.
- **bri's API-first security tier** (ADR 0069): JWT auth, composable guards,
  a Compojure-style router, CORS, metrics, audit, auto-ban — one blessed way,
  one static binary.
- `examples/web-api`: a runnable JWT-secured JSON API to copy.

## [v0.2.0](https://github.com/muthuishere/cljgo/releases/tag/v0.2.0) — 2026-07-16

*The adoption release: a downloaded binary plus the Go toolchain is the whole
story.*

- `cljgo build` works **from the binary alone** — release binaries pin the
  published runtime module and interop host facts resolve from the generated
  module. No checkout, no `CLJGO_SRC` (ADRs 0028/0033).
- **nREPL**: `cljgo nrepl`, so Calva and CIDER connect (ADR 0031).
- `format`/`printf` with Java's grammar, corpus-verified (ADR 0030).
- **BigDecimal rewritten** as unscaled `big.Int` + scale, Java's exact model —
  killing silent long-literal corruption (ADR 0032).
- **9.5× faster boot** (211 ms → 22 ms) and faster dynamic-var access
  everywhere, via goroutine-ID lookup in assembly (ADR 0034).
- clojure-test-suite: 217/242 (89.7%), up from 57/242.

## [v0.1.0](https://github.com/muthuishere/cljgo/releases/tag/v0.1.0) — 2026-07-16

*First public release.* Clojure hosted on Go: a compiler written in Go that
AOT-emits plain Go source, plus a tree-walk evaluator that is the REPL and the
macro engine.

- Zero-ceremony Go interop in both modes — the Go toolchain is the classpath.
- Real goroutines and channels for `(go …)`, with no CPS rewrite.
- clojure-test-suite: 102/242 files passing (42.1%).
- Verified on Linux, macOS and Windows.
