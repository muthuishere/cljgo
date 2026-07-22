# ADR 0048 — Dependency resolution: load path, lock, and the purity question

Date: 2026-07-22 · Status: **proposed** (evidence: spikes S25–S28, all
closed) · Ratifies the dependency clauses of **ADR 0021** (`build.cljgo`,
itself still `proposed`), on the rails of **ADR 0042** (multi-namespace
emission) and **ADR 0013** (every project is a library).

## Context

**This ADR was deliberately drafted out of order, and said so.** ADR 0027
mandates spike → close → ADR → spec → apply. This ADR was first written as a
*draft* — a set of concrete, falsifiable claims put down BEFORE the evidence
existed, so that spikes S25–S28 had something specific to kill rather than an
open-ended "investigate dependencies" brief. All four spikes have now closed;
this revision folds their verdicts in and moves the ADR to `proposed`.

The spikes did their job — they killed real claims, some of them mine:

- The drafted phrase "content-addressed … keyed by git SHA" (decision 1)
  **conflated two guarantees**; S28 force-moved a tag onto an empty commit and
  the SHA moved while the tree hash did not. Key by identity, verify by
  content — corrected below.
- Decision 4's *reasoning* was **contradicted by the shipping implementation**:
  S26 and S27 both measured `go mod tidy` silently applying MVS to a duplicate
  require (exit 0, higher version wins). The hard error survives as a goal but
  must be actively built, not assumed.
- A drafted "`SynthGoMod` staleness bug" was **false** and is retracted below,
  visibly.
- And the spikes surfaced a pre-existing **silent REPL-vs-binary divergence**
  on third-party `go-require` — the failure mode CLAUDE.md calls unforgivable —
  which now gates implementing this ADR (new §6a). None of the four spikes was
  sent to find it; two found it independently.

Falsified claims are corrected in place, not deleted — the record shows what
was believed, what the evidence said, and why it changed.

### What ADR 0021 settled, and what it did not

ADR 0021 killed `deps.edn` on an explicit owner decision (2026-07-15):
dependencies are declared as *code* in `build.cljgo`, not in a static
manifest. Its decision 3 gives cljgo-package deps the surface
`(dep b "name" {:git … | :path …})`.

That settled the **surface**. It settled none of the **mechanics**: where
fetched source lives, how a namespace finds it, what pins it, how versions
are selected, and what happens when a dependency is not self-contained.
Those are this ADR's subject.

### Current state, verified 2026-07-22 at 1e98cd6

**The Go-module lane works — on the project path only.** The chain is
`build.cljgo` `go-require` → `plan.GoRequires` → `pkg/build/build.go:241`
→ `emit.SynthGoMod(genDir, …, reqs)`. The mechanism is subtler than it
looks: `pkg/build` writes `go.mod` *with* the requires **before**
`WriteModule` runs, and `SynthGoMod`'s early return when `go.mod` already
exists (`pkg/emit/program.go:329`, `// user-owned: never overwrite`) is what
stops the later call from clobbering it. The single-file path
(`pkg/emit/program.go:305`, `pkg/emit/module.go:192`) passes `nil` requires
and relies on that same guard.

So the "never overwrite" rule is **load-bearing**, not incidental: it is what
sequences the two writes *within a single build*, letting the requires-bearing
`go.mod` survive `WriteProgram`'s `nil` call.

It is **not** a staleness defect across builds. `buildArtifact` allocates
`genDir` fresh per artifact via `os.MkdirTemp` (`pkg/build/build.go:225`) and
removes it on success unless `-keep-gen` (`:272`), so `go.mod` never
pre-exists on the project path and the guard never fires there. Editing
`go-require` in `build.cljgo` and rebuilding **does** take effect. (An earlier
draft of this ADR asserted the opposite; it was wrong, and the claim is
recorded here corrected rather than deleted.) The guard's user-owned rationale
applies to the single-file `-gen` directory, which is kept — and that path
carries no `go-require` today.

**The cljgo-library lane does not exist.** `ResolveLibPath`
(`pkg/eval/libload.go:65`) resolves a namespace symbol *only* relative to
the requiring file: `dir(*file*)`, plus that directory with the requiring
namespace's own directory suffix stripped, trying `.clj` then `.cljg`. There
is no load path, no `$CLJGO_PATH`, no dependency root, no cache.

`(dep b …)` is therefore not "a verb we have not written yet". The verb is
the easy part. **A fetched dependency has nowhere to be put and no way to
announce itself to the resolver.** The resolver is the work.

`design/00-architecture.md:141` has flagged this by name since M0 — *"Not
yet written: a doc 06 (project layout / deps / load-path conventions)"* —
with doc 03 §7a already citing a design-06 load path that does not exist.

### The one thing that is already de-risked

`pkg/emit/module.go` discovers namespaces by **evaluating require forms
through the interpreter** and walking file-backed requires transitively. It
has no independent resolver — `ResolveLibPath` has exactly one non-test
caller (`pkg/eval/libload.go:39`).

One load path therefore serves both legs. Dual-mode parity — the failure
mode CLAUDE.md calls unforgivable — comes **free by construction** here,
rather than being a second implementation kept in sync by discipline. This
is the single strongest argument for growing `ResolveLibPath` rather than
introducing a parallel dependency-aware loader.

## Decision

### 1. Dependency cache: global, keyed by identity, verified by content [S28 — MET]

Fetched dependencies live in a **global cache** (`$XDG_CACHE_HOME/cljgo/`,
falling back to `~/.cache/cljgo/`) under two subdirectories: `dl/` holding
bare git mirrors, and `src/` holding **immutable `0555` source trees
materialized by `git archive`** (not `git checkout` — a checkout is not
deterministic enough to hash reproducibly).

S28 falsified the draft's single phrase "content-addressed … keyed by git
SHA": it **conflates two distinct guarantees**, and a git SHA alone provides
neither reliably. Force-moving a tag onto an empty commit moved the SHA but
not the tree hash; onto a content-changing commit, both moved. So:

- **Key by identity** — `sha256(url‖sha‖subdir)`, which must be computable
  *before* the fetch, to locate the cache slot.
- **Verify by content** — a merkle **tree hash**, recomputed on *every* read,
  to detect a tampered entry or a lying mirror. A lock carrying only the SHA
  can see neither.

This is Go's own shape (`@v/<version>.zip` + `.ziphash`/`go.sum`). Concurrency
is `flock` + atomic rename with immutable entries: a losing racer discards its
work, there is no merge. Because entries are read-only, a **`cljgo cache clean`
verb is required** — a user cannot `rm -rf` a `0555` tree cleanly.

A project-local **`vendor/<name>/`** overrides the cache when present, under
the *same* lock hash, and needs **no new load-path slot** — it merely varies
the base directory inside decision 2's slot 3. It covers air-gapped and
audited builds without making the common path pay.

*S28 showed (MET):* byte-identical lockfiles across two cache roots and
project dirs (`cmp` exit 0); a 13-byte cache tamper caught with expected/got
hashes; a force-moved tag changing nothing for a locked build; a warm locked
build resolving with the remotes renamed off disk; 8 concurrent resolvers on
one cold cache → 8× exit 0, 3 entries, 0 temp leftovers. Cost (3 deps): cold
163 ms, warm 3.5 ms, offline 3.3 ms, against a 3.3 ms process baseline. Honest
limit: "different machines" was simulated as different cache roots + project
dirs on one host; local git transports only; darwin/arm64 only, `flock` needs
a Windows equivalent — reproducibility across machines is *evidenced*, not
*proven*.

### 2. Load path: append to relative roots, providers outrank all [S25 — MET]

S25 met its exit criterion: an **8-line addition** to `ResolveLibPath` (the
codebase's only resolver) made a library living entirely outside the
consumer's tree resolve **byte-identically** under both legs (`cmp` exit 0),
with the AOT module containing a package for each dep namespace. `pkg/emit`
was never touched — the central de-risk claim holds (see §"already
de-risked"). The resolution order, first match wins:

1. the requiring file's own roots (today's behavior, **appended to, never
   replaced** — S25 proved this is *load-bearing for correctness*, not style:
   a decoy `a.util` planted in the consumer's own root did **not** win, because
   `evalLibFile` rebinds `*file*` to the dep's path so every file resolves from
   its *own* root by construction. The predicted `*file*` trap does not exist —
   ADR 0042 already solved it — but "append, never replace" is what keeps it
   solved),
2. the project's declared source roots,
3. resolved dependency roots, in lock order (this is also the slot a
   `vendor/<name>/` override varies — decision 1),
4. embedded/registered namespaces (`pkg/corelib`'s provider registry).

**Provider/present namespaces outrank all roots** — `loadLib` consults the
registry first, then namespace existence, then `ResolveLibPath`. A root
carrying `clojure/string.clj` is therefore ignored: **`clojure.*` cannot be
shadowed.** This is a *deliberate divergence* from JVM Clojure, which S25
oracled against the real `clojure` CLI and which *does* permit hijacking
(`HIJACKED clojure.string` printed). We choose the safer rule; it is recorded
here, not inherited by accident.

**Ambiguity among roots is silent first-wins — and exactly matches the
classpath** (S25 oracled both orderings against the CLI; both tools agree).
Keep the semantics; add a *shadowing diagnostic*, but no semantic change.

Roots are declared in `build.cljgo`. A `$CLJGO_PATH`-style env override is
**fine for `run`, a footgun for `build`** — build bakes foreign source into
the binary, so an env-supplied root makes the same command produce a different
binary per machine, invisible in the repo. Env roots are therefore **barred
from feeding a build artifact**. No `pkg/emit` change is required.

*S25 showed (MET):* baseline — both legs failed with the *same* error and
`cljgo build` printed the entry's `println` while failing, direct proof the
emitter evaluates requires through the interpreter; then the 8-line change made
both legs succeed identically. Full proposed text in the spike's `VERDICT.md`
§7.

### 3. A lockfile is required [S28 — MET]

`build.lock.edn`, EDN, adjacent to `build.cljgo`, committed. S28 prototyped
and validated the schema. Per dependency:

- `:name`
- `:git/url`, `:git/ref` (**provenance** — a moving human label),
  `:git/sha` (**identity** — what actually pins)
- `:tree/hash` (the merkle content hash decision 1 verifies against)
- `:paths` (the dep's source roots, for load-path slot 3)
- `:requires` (transitive dependency **names** — the lock is *where
  transitivity lives*, since decision 5 forbids executing a dep's build fn)
- `:pure? true`, **or** `:impure {:go-require […] :c-link […] :ffi […]}`
  (decision 6 reads impurity from here at resolve time)

Top level: `:lock/version`, `:build/hash`. Dependencies name-sorted, map keys
sorted → **byte-identical across machines**.

**`:path` (local) deps stay in the lock as *named holes*** — recorded with
`:local/unlocked? true`, never hashed. Omitting them would drop their
transitive deps from the graph, lose their load-path position, and make an
irreproducible project *look* fully locked. The lock records that a hole
exists; it does not pretend to pin it.

The **lock is authoritative on `:git/sha`.** A `build.cljgo` ref that disagrees
is a divergence **error naming both**, never a silent re-pin.

*Rationale:* git coordinates without a pinned SHA are not reproducible — a tag
moves, a branch always moves — and a SHA without a content hash cannot detect a
tampered cache or a lying mirror (decision 1). Since `build.cljgo` is
executable code (ADR 0021), the lock is also the only artifact that can be
*read* rather than *run*; see decision 5.

*S28 showed (MET):* identical resolution on a clean (simulated) machine and a
legible expected/got failure on divergence.

### 4. Version selection: explicit pins, hard error on conflict — but it must be BUILT [S26/S27 — MET, reasoning corrected]

The decision survives; the draft's *reasoning* did not. It assumed a version
conflict would surface naturally. It does not.

S26 and S27 **both measured** `go mod tidy` on a duplicate require: **exit 0**,
silently collapses to the higher version, order-independent — i.e. Go applies
MVS with no diagnostic. And cljgo runs `tidy` whenever `GoRequires` is
non-empty (`pkg/build/build.go:262`), so through the real pipeline dep A pinning
`go-cmp v0.6.0` and dep B pinning `v0.7.0` produced exit 0, no diagnostic,
`v0.7.0` linked. **The shipping implementation currently contradicts this
decision.**

So a duplicate require is *not* a Go error, and the hard error must be
**actively implemented**: cljgo must detect the conflict and **merge in its own
layer BEFORE the `go.mod` write**. It cannot delegate to `go mod tidy` without
inheriting MVS through the back door. Flattening also **destroys provenance** —
a real Go module graph keeps `depa → go-cmp@v0.6.0`, but cljgo's single
flattened module shows only `cljgo.gen/main → v0.7.0`, so `go mod why` can never
name the requirer. This is exactly why decision 3 makes `:requires` provenance
**mandatory**: without it, the conflict error message is unimplementable.

The error names both requirers and both versions. Still not MVS, not
nearest-wins, no solver at the cljgo layer. **S26's caveat:** a hard error needs
a **consumer-side override** (an explicit "I accept version X for this module")
or it will be unusable the first time two real deps disagree. MVS stays open as
its own later ADR.

*Rationale:* honest, tiny, forecloses nothing — a project that builds under
explicit pins still builds under MVS later. Resolution algorithms are where
package managers go to die, and cljgo has no ecosystem yet to demand one.

*S26/S27 showed (MET):* the silent-MVS behavior above, measured through the real
pipeline; the no-solver stance is implementable only as a pre-`go.mod`-write
merge, which decision 3's provenance makes possible.

### 5. Transitive dependencies come from the lock, never from executing a dep's build fn [S26/S27]

Resolution reads the lock and a dependency's **declarative manifest
surface** only. It never evaluates a dependency's `(defn build [b] …)`.

*Rationale:* ADR 0021 decision 4 made build description Turing-complete and
a comptime context — correct and powerful for *your own* build, and
unacceptable as a *resolution* input, where it means running arbitrary code
from every transitive dependency merely to discover what the graph is. The
consumer's own `build.cljgo` still runs, because the consumer chose it.

This forces a real constraint: a dependency's requirements must be
expressible as **data**, extractable without evaluation. **`build.lock.edn`
is that source** — decision 3's `:requires` and `:impure` fields carry the
whole transitive requirement set as data. S26 built the reader against S28's
exact schema and **recovered every transitive require with provenance,
evaluating nothing** — adopted by reference rather than forking the resolver.

Scope limit S26 was explicit about: this validates the **consumption** side
only. Whether a dependency can *produce* that manifest from ADR 0021's
code-first surface — emitted at publish time, or restricted to a
statically-readable subset — is **S27's** territory, and remains the open edge
that may yet amend ADR 0021 rather than be smuggled past it.

*S26/S27 showed (MET):* transitive requirements are recoverable from the lock
as data with provenance, without evaluating any build fn.

### 6. Dependency purity — RESOLVED: capability sets, default deny [S26/S27 — MET]

A cljgo dependency need not be pure Clojure. Via ADR 0021 it may carry
`go-require` Go modules; via ADR 0011/0044 it may carry `c-link` cgo
dependencies or purego `ffi` declarations. The consumer emits **one Go
module** (`SynthGoMod`, one `go.mod`), so impurity is not contained — it
propagates into the consumer's build. The draft left this UNRESOLVED; S26 and
S27 resolved it.

**Policy: capability sets, explicit opt-in, default deny** (S27's option (c)
enforced via (b)). A consumer must acknowledge a dependency's impurity for it
to resolve; unacknowledged impurity is refused, not warned.

**`:ffi` and `:cgo` are separate switches, deliberately.** S27 measured the
asymmetry: `:ffi` (purego) costs **~120 KB** (+2.4% on a 5 MB binary) and stays
fully portable; `:cgo` (`c-link`) costs **cross-compilation entirely**. One
switch would let cgo in through a door opened for ffi. Two hard clauses follow:

- **`:cgo` is *refused*, not warned,** when the project declares cross-targets
  (`:target`). S27 measured zig-cc cross-compiling cgo-against-libc fine but
  **failing on cgo-against-sqlite3** (`'sqlite3.h' file not found`) — zig
  supplies a toolchain and libc, not a sysroot for third-party libraries, so
  the escape hatch does not cover the case `c-link` is *for* (see also
  consequences: ADR 0011).
- A dependency's `:go-requires` **merge at the cljgo layer**, explicitly not
  via `go mod tidy` (decision 4), so conflicts surface instead of silently
  MVS-resolving.

**Detectability: YES.** Impurity is readable at **resolve time from the lock
alone** (decision 3's `:impure` field, S28). S27 shipped a runnable prototype
that parses a static `cljgo.manifest.edn` with cljgo's own `pkg/reader`,
evaluates **no** build fn, probes the host with real `purego.Dlopen`/`Dlsym`,
and **refuses bad graphs before fetching** — catching a missing library,
missing symbol, missing pkg-config, cgo-vs-cross-compile, and a Go-module
version conflict, each with a named fix.

**ADR 0044 has a hole, measured by S27.** `p.GoRequires` is populated
*exclusively* from the consumer's own `build.cljgo` (`pkg/build/build.go:241`),
and decision 5 forbids evaluating a dependency's build fn — so **no path exists
today** for a dependency's FFI requirement to reach the consumer's `go.mod`.
S27 built exactly this (pure-Clojure consumer, FFI in the *dependency*
namespace) and the build failed. ADR 0044 decision 2's conditional-inclusion
rule was written for a *program's own source*; but **libraries, not
applications, are what carry FFI.** ADR 0044 needs amendment (see
consequences).

### 6a. BLOCKER: third-party `go-require` silently diverges REPL from binary [S26/S27]

This is the most important output of the spike round, and it **gates
implementation of this ADR.** S26 and S27 *independently* measured the same
defect, with different fixtures:

```
$ cljgo run src/main.cljg   →  uuid: nil              # S26
$ ./consumerapp             →  uuid: 3d91365f-…       # S26
$ cljgo run src/main.cljg   →  RTLD_NOW=              # S27
$ ./consumer                →  RTLD_NOW=2             # S27
```

Both legs **exit 0**. It also corrupts a boolean (interpreted `false`, binary
`true`). The interpreter cannot reach an **unlinked** third-party Go package
and **returns `nil`/`""` instead of erroring**. Stdlib `require-go` is
consistent in both modes — the divergence is specific to *third-party*, i.e.
*impure*, modules. It fires during **every `cljgo build`**, because the emitter
evaluates require forms through the interpreter, and it reproduces against the
repo's own `examples/build-websocket`, so it **predates this work and is live
on `main` today**. Dependencies do not introduce it; they multiply it, because
a consumer inherits an impure dep's `require-go` without ever typing one.

**S26's conclusion, adopted:** *impurity is not adoptable until the interpreter
either links project Go deps or hard-errors, because `nil` is the worst of the
three options.* This is the failure mode CLAUDE.md calls unforgivable
(REPL-vs-binary divergence), and it must be fixed before decisions 1–6 ship a
`(dep …)` that can pull an impure dependency. It is tracked as its own ADR
(consequences), not folded into the resolver work.

## Consequences

- **Reproducibility becomes a property, not an aspiration.** Lock + content
  addressing means a clean machine builds byte-identically. That is a
  testable claim, and S28 is where it gets tested.
- **`SynthGoMod`'s write-once rule is a constraint to respect, not a bug to
  fix.** The guard is load-bearing for the go-require path's two-phase write
  (`pkg/build/build.go:241`), and the fresh-temp-`genDir` discipline is what
  keeps re-resolution correct today. A dependency-aware `go.mod` — merging a
  dep's requires (decision 6) — must preserve both properties: write once,
  fully-formed, into a directory that did not previously exist. If any future
  design needs to *rewrite* an existing `go.mod`, it must first distinguish a
  generated one from a user-edited one, which is a real design question with a
  migration, not a one-liner.
- **Design doc 06 gets written.** The load path in decision 2 is the content
  `design/00-architecture.md:141` has been promising since M0; doc 03 §7a's
  dangling citation resolves with it.
- **ADR 0021 gets ratified or amended, not left dangling.** It is still
  `proposed`; implementing its dependency clauses settles them in practice.
  Decision 5's constraint may force an amendment to its code-first surface.
- **Both execution legs are covered by one resolver**, so every conformance
  case for dependency loading runs in the dual harness (ADR 0007) with no
  new machinery.
- **A separate ADR is owed for two dual-mode divergences the spikes exposed,
  both live on `main` and neither caused by this work:** (a) the third-party
  `go-require` `nil` divergence (§6a, S26/S27); and (b) S25's finding that an
  entry namespace's `*file*` reads `NO_SOURCE_FILE` in an AOT binary but the
  real path under the interpreter — which also makes entry-namespace `require`
  unresolvable inside a binary, masked today only by the provider registry.
  Both are ADR 0002/0007-class. They block, and outrank, the resolver work.
- **ADR 0044 needs amendment** for the library-carries-FFI hole (§6): its
  conditional-inclusion rule reasons about a program's own source, but FFI
  arrives through dependencies, for which no inclusion path exists.
- **ADR 0023's binary-size framing needs a note:** for cgo, **linkage, not
  bytes, is the metric.** S27 measured the sqlite-linking darwin binary as
  **13,600 bytes *smaller*** while strictly less portable (a dynamic
  `ld-linux` dependency). Size can move the wrong way relative to portability.
- **ADR 0021's `{:pkg-config "sqlite3"}` needs a raw `:libs`/`:headers`
  alternative.** `pkg-config` is not installed on the dev Mac while `cc`,
  `zig`, and `libsqlite3` all are — a pkg-config-only surface is unbuildable
  on a machine that can otherwise link the library.
- **ADR 0011 decision 3's zig-cc escape hatch is narrower than claimed:** true
  for cgo-against-libc, false for cgo-against-a-third-party-library (§6, S27).
- **`pkg/repl/session.go:57` hardcodes `~/.config/cljgo/sessions`** with no XDG
  lookup — the only user-level state handling in the tree. Deps correctly land
  in `.cache`; recommend a separate one-function fix so the two agree, rather
  than folding it here.

**Out of scope**, deliberately, each its own later decision: a package
registry or index; publishing/distribution (ADR 0013's `--lib` /
`--c-shared` producer side is a separate, larger piece); semver ranges and
any constraint solver; private/authenticated dependency sources; and
dependency vulnerability scanning.

## Spikes

| spike | question | validates | outcome |
|---|---|---|---|
| **S25** | Can `ResolveLibPath` grow a load path that serves interpreter and emitter identically, without regressing relative resolution? | 1, 2 | **MET** — 8-line change, byte-identical both legs, `pkg/emit` untouched. Killed the predicted `*file*` trap (ADR 0042 already solves it); found `clojure.*` can't be shadowed (deliberate divergence) + a live entry-`*file*` AOT divergence |
| **S26** | What does version conflict look like on a realistic transitive graph, and does the no-solver stance hold against Go-module MVS? | 4, 5, 6 | **MET** — `go mod tidy` silently MVS-resolves duplicates (contradicts drafted d4 reasoning); hard error must be built pre-write; falsified the drafted `SynthGoMod` staleness bug; co-found §6a |
| **S27** | Is a dependency's requirement set recoverable as data without evaluating its build fn — and is impurity detectable at resolve time? | 5, 6 | **MET** — detectable YES (runnable prototype); ffi/cgo must split; ADR 0044 hole; zig-cc doesn't rescue cgo-3rd-party; co-found §6a |
| **S28** | Does lock + content-addressed cache produce reproducible cold-cache builds, safely, under concurrency? | 1, 3 | **MET** — key-by-identity / verify-by-content (SHA≠content hash); full lock schema; concurrency-safe; cross-machine evidenced not proven |

Each closed with a `VERDICT.md` per ADR 0027 §2; this ADR is now `proposed`.

**Before `apply`:** §6a (and the entry-`*file*` divergence) must land as their
own ADR + fix first — they gate a `(dep …)` that can pull an impure dependency.
Then `/opsx:propose` turns decisions 1–6 into OpenSpec deltas.
