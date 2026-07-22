# Spike S33 verdict — SHA pins identity, tree hash pins content; the lock needs both, plus transitivity and impurity

Closed 2026-07-22. Recommendation feeds **ADR 0048 decisions 1 and 3**.

**Exit criterion: MET, all five clauses.** Two clean-cache resolutions in
different project dirs against different cache roots produced
**byte-identical lockfiles** (`cmp` exit 0) and **byte-identical dependency
trees** (content hash per dep, `diff` clean); a 13-byte append inside a warm
cache entry was **caught on the next resolve** with the expected/got hashes
named; a force-moved tag changed nothing for a locked build (twice — empty
commit and content-changing commit); a locked warm build resolved with the
fixture remotes **physically renamed off disk**; 8 concurrent resolvers
against one cold cache all exited 0 with 3 entries and 0 temp leftovers.

Raw transcripts: `results/e1…e9`. Prototype: `prototype/` (self-contained Go
module, `go.mod` module `s28`; never merges into `pkg/`, ADR 0027 §5).
Fixture remotes are **local bare git repos** (`prototype/fixture.sh`) — the
core proof touches no network.

---

## 1. The headline finding: a git SHA is not a content hash, and you need both

This is the finding that most changes ADR 0048's decision 1, which currently
says "content-addressed … keyed by resolved identity (git SHA)". Those are
two different things and the spike separated them empirically.

**E3e** force-moved tag `v1.0.0` onto an *empty* commit. The SHA changed;
the tree hash did **not**:

```
< :git/sha  "c3820883df6041abb3055c7ff6ed3d9050c4c620"
> :git/sha  "a55c3a92c7df43be1e6e6477e098a641204bd10d"
   (tree/hash unchanged: sha256:bd710db3debb…)
```

**E3g** then force-moved it onto a commit that changed a source byte. Both
fields moved. So the two hashes answer two different questions, and a lock
carrying only one is blind to a real class of change:

| field | answers | catches |
|---|---|---|
| `:git/sha` | *which commit did we agree on* | tag moved, branch advanced, ref ambiguity |
| `:tree/hash` | *are these the bytes we agreed on* | cache/vendor tampering, a lying or compromised mirror, a rewritten object, a truncated fetch |

**Recommendation: key the cache directory by identity, verify it by
content.** The cache key must be computable *before* the fetch — you cannot
address by tree hash something you have not downloaded yet — so the
directory name is `sha256(url ‖ sha ‖ subdir)` (`cas.go:srcDir`) and the
tree hash is the *integrity check on read*. This is exactly Go's shape:
`$GOMODCACHE/cache/download/<module>/@v/<version>.zip` is keyed by module +
version (identity), and `.ziphash` / `go.sum` verify the bytes. The
duplication is not redundancy; it is what makes "the cache is immutable and
safely shared across projects" a checkable claim rather than an assertion.

Consequence for ADR 0048 §1's wording: **"content-addressed" is the wrong
term for the key and the right term for the verification.** Say so
explicitly, or implementers will build one and think they have both.

## 2. Cache layout — and an existing inconsistency the repo must settle

The prototype's `CacheRoot()` (`prototype/cas.go:17`):

```
$CLJGO_CACHE            explicit override (used here as the "other machine")
$XDG_CACHE_HOME/cljgo   ADR 0048 §1
~/.cache/cljgo          fallback
```

**Grepped, and reporting the conflict as instructed:** the only existing
user-level state in the tree is `pkg/repl/session.go:57`, which hardcodes
`filepath.Join(home, ".config", "cljgo", "sessions")` — **not XDG-aware,
and `.config` rather than `.cache`**. There is no other cache/config-dir
handling anywhere in `pkg/`, `cmd/`, or `core/`.

Recommendation: keep ADR 0048's `$XDG_CACHE_HOME/cljgo` for the dep cache
(it is genuinely regenerable cache data, so `.cache` is correct and
`.config` would be wrong), and land a **separate** trivial fix making
`sessionsDir()` honour `$XDG_CONFIG_HOME` so the two agree on the pattern.
Sessions belong in `.config` and deps in `.cache`; the bug is only the
missing XDG lookup. That is a one-function change in `pkg/repl` and should
not be smuggled into the dependency work — note it and file it.

Measured layout (E8):

```
<root>/dl/<sha256(url)>.git      bare mirror, one per remote (refetch source)
<root>/src/<sha256(url‖sha)>/    materialized tree, immutable, mode 0555
<root>/lock/<sha256(key)>.lock   flock file, one per cache entry
```

Three sub-findings:

- **`git archive`, not `git checkout`.** The tree is materialized by piping
  `git archive --format=tar <sha>` into `tar -x`. That writes no `.git`, no
  index, and no mtime-dependent state — a deterministic materialization.
  A worktree checkout is not deterministic enough to hash.
- **Entries are chmod'd 0555** (`markReadOnly`). Not security — integrity is
  the tree hash — but a tripwire that makes "I edited the cache to debug"
  loud. Practical cost, discovered the hard way: **`rm -rf` on the cache
  fails**, so the product needs a real `cljgo cache clean` verb rather than
  telling users to delete a directory.
- **Keeping the bare mirror separate from the materialized tree** is what
  makes a second version of the same dep cheap (incremental fetch) and is
  what let E3d re-fetch an old SHA after the tag had already moved.

## 3. Concurrency (ADR 0048 §1's explicit ask): flock + atomic rename, nothing more

**E5**: 8 resolvers, one cold cache, all exit 0, identical output, 3 src
entries, **0 leftover `.tmp-` dirs**, and a verification pass after the
storm is clean.

The mechanism is two mechanisms and no daemon:

1. `syscall.Flock(LOCK_EX)` on `<root>/lock/<key>.lock` around fetch —
   released by the OS on process death, so a crashed or killed resolver
   cannot wedge the cache (the failure mode of a hand-rolled lockfile).
2. Materialize into `src/.tmp-XXXX`, then `os.Rename` to the final name.
   Rename is atomic within a filesystem, so a reader never observes a
   partial tree. If the destination already exists (another process won),
   the temp is discarded — **entries are immutable, so both copies are the
   same bytes by construction**; there is no merge case.

The double-check inside the lock (`if stat(dst) ok { return nil }`) is what
makes the 8-way race collapse to one fetch.

## 4. Vendor escape hatch — it works, and it needs its own load-path slot

**E4d**: with the entire cache emptied, the remotes renamed off disk, and
`-offline`, a resolve backed only by `vendor/` succeeded, and the emitted
load path pointed into `vendor/`, not the cache:

```
$W/proj/vendor/acme-http/src
$W/proj/vendor/acme-crypt/src
$W/proj/local-lib/src
$W/proj/vendor/acme-util/src
```

**E4e**: tampering the vendored copy was caught by the *same* lock
`tree/hash` that guards the cache — one integrity mechanism, two storage
backends. This is the strongest argument for the schema: the hash is a
property of the dependency, not of where it happens to sit.

Interaction with **ADR 0048 decision 2** (S30's load path): vendoring does
**not** need a new slot in the resolution order. It resolves *within* slot 3
("resolved dependency roots, in lock order") — the resolver simply hands
S30 a different base directory for the same dep, in the same lock order.
Precedence inside slot 3 is `:path` dep → `vendor/<name>` → global cache
(`resolve.go:baseDir`). Decision 2's four-slot list stands unmodified; add
one sentence saying slot 3's *location* is chosen by the resolver and
vendoring is invisible above it.

## 5. `:path` deps — they belong in the lock, as a NAMED HOLE

Recommendation, and the prototype implements it: **record them, never hash
them.** A `:path` entry carries `:name`, `:local/path`, `:paths`,
`:requires`, and `:local/unlocked? true` — and deliberately no `:git/sha`
and no `:tree/hash`:

```edn
{:local/path "local-lib" :local/unlocked? true
 :name "local-lib" :paths ["src"] :pure? true :requires []}
```

Three reasons omitting them entirely is worse:

1. **The graph is incomplete without them.** A `:path` dep has its own
   transitive deps, and those *are* fetched and *are* reproducible. In the
   fixture, dropping `local-lib` from the lock would drop whatever it
   requires from the lock too. Transitivity has to flow through it.
2. **Load-path order is part of the resolution result** (decision 2, slot
   3, "in lock order"). A dep absent from the lock has no position, so
   shadowing behaviour would depend on `build.cljgo` scan order rather than
   on a reviewable artifact.
3. **Honesty is machine-readable.** `:local/unlocked? true` lets CI say
   "this project is not reproducible: 1 unlocked path dep" instead of
   silently believing a green lock. Omission would make an irreproducible
   project *look* fully locked — the worst of the three options.

The rule to write down: **the lock records every node in the graph; it
hashes only the nodes whose bytes it can pin.**

## 6. Offline — proven, and stronger than asked

**E4a**: warm cache + lock + `-offline`, with `$W/remotes` renamed to
`$W/remotes-GONE` (visible in the transcript's directory listing) — exit 0.

**E4b** is the more interesting one: the same resolve **without**
`-offline` also succeeded with the remotes gone. That proves the property
by construction rather than by flag — when every dep is locked and cached,
the resolver has no reason to call `git` at all. `resolveRef` (the only
remote-touching function) is reached only when a dep is unlocked or
`-update` was passed. `-offline` is therefore a *guard*, not a mode: it
converts a would-be network call into a legible error rather than enabling
an alternate path.

**E4c** confirms the error is actionable:
`offline: acme-http@724b2d1a606a is not in the cache (<path>)`.

Cost (E9, medians, 3 deps): cold **163 ms** (3 bare clones + archive +
verify), warm **3.5 ms**, offline **3.3 ms**, against a **3.3 ms** process
baseline. Warm resolution including full tree re-verification is
indistinguishable from process startup at this size — but that is 3 tiny
deps. **Full re-hashing on every read will not stay free**; before this
ships at ecosystem scale it wants Go's trick of a per-entry `.ok` stamp
written after verification, with the full hash recomputed on demand
(`cljgo cache verify`) rather than every build. Flagging it now, not
building it — a premature stamp is a correctness hole, and the spike's job
was to prove the check works.

## 7. Transitivity without executing anything (ADR 0048 decision 5)

Decision 5 forbids evaluating a dep's `(defn build [b] …)` at resolve time,
which means transitive requirements must be *data*. The prototype reads
`cljgo.manifest.edn` from the dep's tree — `:paths`, `:deps`, `:go-require`,
`:c-link`, `:ffi` — and never evaluates anything. It works: `acme-util` was
discovered purely through `acme-http`'s manifest and appears in the lock
with `:requires ["acme-util"]` recorded on the requirer.

This spike does **not** claim the manifest can be produced from ADR 0021's
code-first surface — that is **S32's question**, and S32 may conclude it
cannot. What S33 establishes is narrower and still useful: *given* a
data-shaped manifest, breadth-first resolution + a flat lock is sufficient,
and the lock alone reproduces the graph on a cold machine with no manifest
reads at all beyond the ones it re-verifies. If S32 fails, the manifest can
be emitted at publish time or — the fallback S32 should cost — reconstructed
into the lock by the *publisher's* build, so consumers still read data.

## 8. Version conflict (S31's, not this spike's)

The prototype refuses rather than picks:

```
dependency conflict: "x" required at ref "a" and "b"
  (S31 owns the policy; S33 refuses to pick)
```

Recorded here only to note the boundary held: nothing in the cache or lock
design presumes a resolution algorithm, so S31 can choose hard-error (ADR
0048 §4) or later MVS without touching any of this.

## 9. Deliberately NOT built

No registry, no index, no publishing, no semver ranges, no constraint
solver, no private/authenticated sources, no vulnerability scanning — per
ADR 0048's out-of-scope list. Also not built: cache GC/eviction, a
`.ok`-stamp fast path (§6), and cross-filesystem rename fallback (the temp
dir is created *inside* `<root>/src` precisely so rename stays atomic).

## 10. Honest limits of the evidence

- **"Different machines" was simulated** as different project directories
  with different cache roots on one host. That isolates every path the
  resolver actually varies on, but the two locks share an absolute
  `file://` URL from the shared fixture, so this proves *cache-root and
  project-dir independence*, not *hostname/OS independence*. Nothing in the
  lock schema carries host state, so the gap is small — but it is a gap.
- **Only local bare repos** were exercised (deliberate, per the brief). No
  https/ssh transport, no auth, no shallow-fetch path.
- **darwin/arm64 only.** `syscall.Flock` is unix; Windows needs
  `LockFileEx`. Not tested.
- **Small trees.** 3 deps, a handful of files. The §6 hashing-cost warning
  is a projection, not a measurement.

---

## Recommendation

**Ratify ADR 0048 decisions 1 and 3, with decision 1 reworded.** The
mechanism is small — flock, atomic rename, `git archive`, one merkle hash,
one EDN file — and every clause of the exit criterion held. Specifically:

### What decision 1 should say

> Fetched dependencies live in a **global cache** at `$XDG_CACHE_HOME/cljgo`
> (falling back to `~/.cache/cljgo`, overridable by `$CLJGO_CACHE`), laid
> out as `dl/<hash(url)>.git` (bare mirrors) and `src/<hash(url‖sha‖subdir)>`
> (immutable materialized trees, written by `git archive` so they are
> byte-deterministic, published 0555).
>
> Entries are **keyed by resolved identity** (git SHA — never a branch or
> tag) and **verified by content** (a merkle tree hash) on every read. Those
> are two distinct guarantees: the key makes the entry findable before it is
> fetched; the hash makes it trustworthy after. A lock carrying only the SHA
> cannot detect a modified cache entry or a lying mirror [S33 §1].
>
> Concurrency safety is `flock` on a per-entry lockfile plus publication by
> `os.Rename` from a temp dir inside the same filesystem; entries are
> immutable, so a lost race discards the loser's copy rather than merging
> [S33 §3, 8-way race clean].
>
> A project-local `vendor/<name>/` overrides the cache for the same dep,
> verified by the same lock hash. Vendoring is invisible to the load path:
> it changes which directory fills decision 2's slot 3, not the slot order
> [S33 §4].
>
> Cache removal needs a `cljgo cache clean` verb, because entries are
> read-only by design.

### What decision 3 should say

> `build.lock.edn` is EDN, adjacent to `build.cljgo`, committed, generated
> by `cljgo resolve` and never hand-edited. It contains, per dependency:
> `:name`, source identity (`:git/url` + `:git/ref` + `:git/sha`, or
> `:local/path` + `:local/unlocked? true`), `:tree/hash`, `:paths`,
> `:requires` (transitive dep names — the lock is where transitivity lives,
> since decision 5 forbids discovering it by execution), and either
> `:pure? true` or an `:impure` map carrying `:go-require` / `:c-link` /
> `:ffi`. Top level: `:lock/version` and `:build/hash`.
>
> `:git/ref` is recorded as **provenance, not identity** — the lock is
> authoritative on `:git/sha`, and a `build.cljgo` whose ref no longer
> matches the lock is a *divergence error* naming both, not a silent
> re-pin [S33 §1, E3f].
>
> `:path` deps appear in the lock as named holes: recorded for graph
> completeness and load-path order, never hashed, and flagged
> `:local/unlocked?` so a project's irreproducibility is machine-readable
> rather than invisible [S33 §5].
>
> Deps are emitted sorted by name and maps with sorted keys, so two machines
> produce byte-identical lockfiles [S33 §E1c, `cmp` exit 0].

### The proposed schema, verbatim from the prototype (E7)

```edn
;; GENERATED by `cljgo resolve` — commit this file, do not hand-edit.
{:build/hash "sha256:1972a07e…"        ; hash of build.cljgo; detects drift
 :deps [{:name       "acme-crypt"
         :git/url    "file://…/acme-crypt.git"
         :git/ref    "v1.0.0"          ; provenance only — tags move
         :git/sha    "750035182e3f…"   ; IDENTITY (immutable)
         :tree/hash  "sha256:e4a0eaf9…"; INTEGRITY (verified on every read)
         :paths      ["src"]           ; roots contributed to the load path
         :requires   []                ; transitive cljgo deps, by name
         :impure     {:c-link [{:pkg-config "libsodium"}]
                      :ffi    [{:lib "sodium"}]}}
        {:name "acme-http" :git/sha "724b2d1a…" :tree/hash "sha256:26b79cb6…"
         :requires ["acme-util"]
         :impure {:go-require [{:module "github.com/gorilla/websocket"
                                :version "v1.5.3"}]}
         :git/ref "v1.0.0" :git/url "…" :paths ["src"]}
        {:name "acme-util" :git/sha "c3820883…" :tree/hash "sha256:bd710db3…"
         :pure? true :requires [] :git/ref "v1.0.0" :git/url "…" :paths ["src"]}
        {:name "local-lib" :local/path "local-lib" :local/unlocked? true
         :pure? true :paths ["src"] :requires []}]
 :lock/version 1}
```

**Fields considered and rejected**, with reasons — the schema argument is as
much about what is absent:

| rejected | why |
|---|---|
| `:git/branch`, `:git/tag` as separate keys | one `:git/ref` recording what was *written* is enough; splitting invites treating a tag as identity, the exact bug §1 exists to prevent |
| fetch timestamp / resolved-at | breaks byte-identical locks across machines for zero benefit; provenance belongs in VCS history |
| per-file hashes | the merkle root already pins every byte; a file list bloats the lock and leaks tree shape into a diff for no added guarantee |
| download URL / mirror | a mirror is a *machine* fact, not a *project* fact; putting it in a committed lock makes builds depend on the resolving machine's network topology |
| `:size`, `:file-count` | weaker restatements of `:tree/hash` |
| a nested dep tree instead of a flat list | the graph is a DAG with shared nodes; nesting duplicates them and makes diffs unreadable. `:requires` edges on a flat, name-sorted list carry the same information and diff cleanly |
| resolved Go-module *versions* merged at lock time | this is decision 6's open question and S31's call; recording each dep's *declared* `go-require` keeps the lock honest without pre-deciding the merge |

### Blocking dependencies on other spikes

Decisions 1 and 3 are ratifiable now. Two things must **not** be written
into them yet:

- The **shape** of `:impure` is provisional. S33 proves the lock *can* carry
  impurity markers and that resolution can read them before any build step
  runs — which is precisely what decision 6's "detectability" question asks
  for, and the answer is **yes, impurity is knowable at resolve time from
  the lock alone**. The *policy* (refuse? warn? propagate?) is S32's.
- Whether `cljgo.manifest.edn` can exist at all is **S32's**. If it cannot,
  decisions 1 and 3 survive unchanged — only the *source* of `:requires`
  and `:impure` moves (to publish-time emission), not the lock schema.
