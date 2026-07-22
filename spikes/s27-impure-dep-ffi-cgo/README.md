# Spike S27 — What does an FFI/cgo-carrying dependency do to its consumer's build?

Opened 2026-07-22. Feeds **ADR 0048 decision 6** (dependency purity —
currently UNRESOLVED) and touches **decision 5** (transitive requirements
recoverable as data). Prior art: **ADR 0011** (purego primary door, cgo
ecosystem door), **ADR 0044** (`proposed` — purego dependency placement),
**ADR 0021** (`build.cljgo`, `c-link`, `CGO_ENABLED=1`), **ADR 0023**
(binary size is first-class). Closed spikes **S7** and **S21** are the
frozen evidence base for purego itself; this spike does NOT re-prove
purego marshaling — it asks what *consuming* such a library costs.

## Context — the verified state of the ground

Much of ADR 0048's decision-6 question is **prospective**, and this README
says so up front so the VERDICT's claims can be read at the right weight.

Verified in-tree at the time of opening:

- **`ffi/` does not exist.** No `ffi/deflib`, no purego anywhere in `pkg/`.
  ADR 0044 is `proposed`, not accepted; `github.com/ebitengine/purego`
  appears in no `go.mod` in this repo.
- **`c-link` does not exist.** `core/build.cljg` defines exactly
  `make-builder`, `exe`, `go-require`, `install`, `run`, `option`,
  `host-target`. There is no `lib`, no `c-link`, no `ffi`, no `dep`.
- **`CGO_ENABLED` is never set by cljgo.** `grep -rn CGO_ENABLED pkg cmd`
  returns nothing; `pkg/emit/program.go:406`'s `GoBuild` runs
  `go build -trimpath -ldflags=-s -w` with the **inherited** environment.
  ADR 0021 decision 5 is therefore entirely unimplemented.
- **There is no dependency mechanism at all.** No `dep` verb, no load
  path (`pkg/eval/libload.go:65` resolves only relative to `*file*`), so
  "a dependency" is not yet a thing cljgo can have.

Consequently every claim in the VERDICT is labelled **MEASURED** (real
command output, reproducible from this directory) or **PROJECTED** (a
consequence derived from measured host/toolchain behavior plus the
written text of an ADR). No claim is labelled from memory.

## The one question

**When a cljgo library dependency is impure in the hardest way — it uses
purego FFI, or cgo/`c-link` against a system C library — what does
consuming it do to the consumer's build, and can that impurity be
detected at RESOLVE time rather than at link time or run time?**

## Exit criterion (written before any code, per ADR 0027)

The spike closes **yes** iff all four hold, each backed by captured
command output:

1. **The purego conditional-inclusion rule is decided by evidence.** A
   consumer whose *own* source has no FFI, but which depends on a library
   that does, is built through the real mechanism cljgo has today
   (`build.cljgo` → `plan.GoRequires` → `emit.SynthGoMod` → `go build`),
   and the resulting `go.mod` is inspected. Either purego appears (ADR
   0044 decision 2's rule fires transitively) or it does not (the rule has
   a hole). Whichever it is, it is shown with the generated file, not
   argued.
2. **The four cgo consequences carry numbers, not adjectives.** For one
   real program linking a real system C library, all of:
   (a) cross-compile result under `CGO_ENABLED=1` (exact toolchain error,
   and whether zig-cc rescues it), (b) the exact link-time diagnostic on a
   machine without the library, (c) binary size delta in bytes vs the
   pure-Go build of the same program, (d) `otool -L`/`file` output showing
   whether the static-binary property survives.
3. **A resolve-time check exists and beats the linker.** A runnable
   prototype reads a proposed manifest for a dependency, determines
   impurity *without fetching or building it*, and fails with a
   diagnostic naming the dependency, the impurity kind, and the missing
   host requirement. Success is measured as: the check's message is
   emitted, and the raw linker/dlopen error it replaces is captured
   side-by-side for comparison.
4. **The dlopen failure mode is observed, not assumed.** A real purego
   `Dlopen` of (i) an absent library and (ii) a wrong-OS library name is
   executed, and the verdict states whether it returns an error or panics
   — the input ADR 0015 structured diagnostics needs.

Anything less closes the spike **no** for that sub-question, and ADR 0048
decision 6 must be written narrower.

## Additionally to be investigated and reported

- Does `SynthGoMod`'s write-once guard (`pkg/emit/program.go:329`)
  interact with impure deps in a way worse than the already-known
  "editing `go-require` does nothing" defect?
- Is `pkg-config` — which ADR 0021's `(c-link art {:pkg-config "sqlite3"})`
  surface assumes — actually present on a normal developer machine?
- Does `CGO_ENABLED=1` change binary size / linkage even for a program
  that links **no** C library (i.e. is the flag itself costly, separate
  from the C dep)?
- What must a manifest carry to be useful: pkg-config name, dlopen'd
  soname per OS, minimum version, and a declared capability set?

## Method

Self-contained throwaway module(s) in this directory (ADR 0027: spike
code never merges into `pkg/`). `pkg/`, `cmd/`, `core/`, `templates/` are
not modified; no closed spike and no `refs/` is touched. Every number in
`VERDICT.md` is reproducible by the commands recorded beside it.

## Results

See `VERDICT.md`.
