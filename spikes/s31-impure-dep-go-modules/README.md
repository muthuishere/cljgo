# Spike S31 — Impure cljgo deps: how do a dependency's `go-require`s compose into ONE go.mod?

Opened 2026-07-21. Feeds **ADR 0048** (decisions 4/5/6). Follows S30.

## Context

ADR 0021 gave cljgo a Zig-style build: `build.cljgo` is code, and
`(go-require art "github.com/x/y" "v1.2.3")` is how a project pins a
third-party **Go** module. ADR 0028/0042 fixed the emission shape: the
generated program is **ONE Go module** (one `go.mod`, one package per
namespace inside it).

Those two facts collide the moment a cljgo *library* dependency is
**impure** — i.e. it carries its own `go-require`s. The owner's concern,
stated 2026-07-21: *"some dependencies may not be pure and how we can
handle all instead of doing all work and redoing."* This spike exists to
find the rework-forcing surprises **before** the `dep` verb is built, not
after.

Verified current state (all citations checked in this worktree, branch
`specs/toolkit`):

- `core/build.cljg:38` — `go-require`'s own docstring: *"the pin is
  accumulated module-wide (the emitted go.mod is one module)"*. It appends
  to `plan.GoRequires` and **ignores which artifact it was called on**.
- `pkg/build/build.go:242-246` — those requires reach
  `emit.SynthGoMod(genDir, moduleName, runtimeDir, reqs)`, then
  `goGet` + `goModTidy`.
- `pkg/emit/program.go:329-331` — `SynthGoMod` returns `nil` early when
  `go.mod` already exists: `// user-owned: never overwrite`
  (design/04 §2's "create on first build, never overwrite" rule).
- **There is no `dep` verb.** `core/build.cljg` has no `dep` fn; ADR 0021
  decision 3 promised one, nothing implements it. So cljgo→cljgo library
  dependencies do not exist yet, and every experiment below that involves
  *two* cljgo projects is a **simulation** — explicitly labelled as such.

## The one question

**When a cljgo library dependency is IMPURE — it carries its own
`go-require` third-party Go modules — how do those requires compose into
the consumer's single generated `go.mod`, and what breaks?**

## Exit criterion (written before any code, per ADR 0027)

The spike closes when all five are answered with **real toolchain output**
captured under `results/`:

1. **Flattening works at all.** A hand-merged require set (consumer C +
   simulated dep A) drives `emit.SynthGoMod` and produces a module that
   `go build`s and runs. PASS = a binary that executes and prints from a
   transitively-acquired third-party module.
2. **The conflict case is decided by measurement, not by opinion.** Two
   deps pin the SAME module path at DIFFERENT versions. Determine
   empirically (a) what `go` does when a single `go.mod` lists one path
   twice, and (b) what it does when the *module graph* disagrees. PASS =
   captured `go` output for both, and a stated recommendation among
   {hard error, MVS, defer to `go mod tidy`} justified by that output.
3. **Leakage.** Establish whether a dep's Go module is observable from the
   consumer's namespace/import surface, and what a diamond
   (C→A→X, C→B→X) does. PASS = a yes/no with a reproduction.
4. **The write-once guard's real constraint is established empirically.**
   (This clause was rewritten mid-spike: the brief asked me to confirm a
   cross-build staleness *bug*; experiment I disproved it, and the
   coordinator independently issued the same correction. The question is
   therefore not "how do we fix it" but "what does it constrain".)
   PASS = a falsifiable run showing whether an edited require set reaches
   `go.mod` on the project path, plus the stated constraint a
   dependency-aware `go.mod` must satisfy.
5. **Transitive discovery without executing dep build code** (draft ADR
   0048 decision 5 forbids evaluating a dependency's `build.cljgo` at
   resolve time). PASS = a prototyped concrete artifact shape (manifest
   and/or lock entry) with a working reader, and a demonstration that it
   reproduces the same merged require set the "evaluate the dep" path
   would have produced.

FAIL for the spike as a whole = we cannot state, at close, which of the
three conflict policies ADR 0048 should adopt.

## Method

Everything runs against the real `go1.26.3` toolchain in throwaway module
directories under `$TMPDIR`, driven by `driver.go` (package `main`, in
this directory, part of the repo module so the gates cover it). It imports
`pkg/emit` **read-only** — no file under `pkg/`, `cmd/`, `core/`, or
`templates/` is modified by this spike, per the spike rules.

Third-party modules used as guinea pigs are real, small, and already in
the local module cache where possible (`github.com/google/uuid`,
`github.com/mitchellh/go-homedir`, `golang.org/x/tools`) so the spike is
reproducible offline-ish.

`results/` holds raw captured output; `VERDICT.md` closes the spike.
