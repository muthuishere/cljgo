## ADDED Requirements

### Requirement: a moved declaration makes the lock stale, and the lock refreshes itself

The lock MUST record a hash of the **normalised declared dependency set** —
the sorted set of `(name, version-or-ref, kind)` triples `build.cljgo`
declares, and nothing else. Resolution MUST treat the lock as stale when that
hash differs from the manifest's, and MUST re-resolve and rewrite the lock
rather than erroring.

The hash MUST NOT cover the manifest's bytes. Reformatting `build.cljgo`,
adding a comment, or renaming an artifact declares nothing new and MUST NOT
trigger a re-resolve.

No diagnostic may name a command that does not exist.

#### Scenario: bumping a version re-pins without deleting the lock

- GIVEN a project locked at `medley/medley 1.4.0`
- WHEN `build.cljgo` is changed to `{:mvn/version "1.5.0"}` and the project builds
- THEN the build succeeds and `build.lock.edn` pins `1.5.0`
- AND the user is never told to run a subcommand that does not exist

#### Scenario: a cosmetic manifest edit does not re-resolve

- GIVEN a locked project whose lock hash matches
- WHEN a comment is added to `build.cljgo` and the project builds
- THEN the lock is unchanged and no coordinate is re-fetched

### Requirement: a re-resolve moves only what changed

When the declared set moves, resolution MUST keep the existing pin of every
coordinate whose declaration is unchanged, and MUST re-resolve only the
changed declarations and the coordinates reachable only from them.

This is a correctness requirement, not an optimisation: a full re-resolve on a
one-line version bump silently drifts every other transitive to whatever is
newest today, which defeats the lock's purpose.

#### Scenario: bumping one root leaves unrelated pins untouched

- GIVEN a lock pinning roots `a`, `b` and their disjoint transitives
- WHEN only `a`'s declared version changes and the project builds
- THEN `a` and its transitives are re-resolved
- AND every pin reachable only from `b` is byte-identical to before

### Requirement: frozen mode makes a stale lock an error

`cljgo build --locked`, and the environment variable `CLJGO_LOCKED=1`, MUST
make a stale lock a hard error naming the divergent coordinates, and MUST NOT
re-resolve or rewrite the lock.

Frozen is distinct from offline: a build MAY be online and still require the
lock to be the authority. A merge that takes `build.cljgo` from one branch and
`build.lock.edn` from another MUST fail under `--locked` rather than silently
resolving a graph nobody reviewed.

#### Scenario: CI refuses a lock that does not match the manifest

- GIVEN a project whose manifest declares `1.5.0` and whose lock pins `1.4.0`
- WHEN it is built with `--locked`
- THEN the build fails with a coded diagnostic naming both versions
- AND `build.lock.edn` is unchanged on disk

#### Scenario: frozen mode is satisfied by a matching lock

- GIVEN a project whose lock hash matches its manifest
- WHEN it is built with `--locked` while online
- THEN the build succeeds and performs no coordinate re-resolution
