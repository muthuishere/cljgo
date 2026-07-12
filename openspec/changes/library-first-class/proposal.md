## Why

ADR 0013 (docs/adr/0013-library-first-class.md, status accepted) makes every
cljgo project a first-class library, Zig-style: one codebase builds as a
Clojure library, a go-gettable Go library, a C shared/static library, or an
executable. Emitted output is already a normal Go module (ADR 0001) — this
change turns that accident into a product feature, starting M2+ because the
munging scheme becomes a public contract the moment the emitter ships.

## What Changes

- `cljgo build --lib`: emits a tidy, stable, go-gettable Go package — exported
  fns get real Go signatures from type hints where available (boxed `any`
  otherwise) and doc comments from docstrings.
- Exported-surface annotation (settled in design): `^:export` metadata plus an
  optional project-file exports map.
- Munging-as-public-contract: the requirements list (deterministic, stable,
  documented, versioned) handed to the M2 emitter — coordination note: an
  emitter change `m2-emitter-v0` may exist in openspec/changes/; this change
  references it and never edits it.
- go.mod versioning discipline for emitted libs (module path source, runtime
  dependency pinning, generated-code header) settled in design.
- `cljgo build --c-shared` / `--c-archive` via Go's buildmodes: .so/.a plus a
  C header whose shape is settled in design.
- Clojure-library consumption (source via path/git deps, interpreted or AOT'd
  by the consumer) documented as the zeroth build mode.

## Non-goals

- No package registry, publishing service, or dependency resolver — go get
  and git are the distribution channel.
- No WASM in this change (ADR 0013 notes it as later; same pipeline).
- No hand-written Go wrapper generation beyond the mechanical export surface;
  no Go-side generics on exported signatures.
- No C++ / SWIG-style binding generation — the C header is plain C, callers
  bring their own FFI.
- No stability promises for non-exported (unannotated) vars — internals may
  re-munge freely between versions.

## Capabilities

### New Capabilities
- `library-builds`: the exported-surface annotation, `--lib` Go-library
  emission, emitted go.mod versioning rules, and C library output shape.

### Modified Capabilities
(none — openspec/specs/ has no existing capability this touches)

## Impact

- ADRs relied on: **0013** (implemented here), 0001 (emit Go source — the
  enabler), 0006 (Go 1.26 toolchain), 0004 (hinted signatures ladder), 0011
  (C-FFI context for the C ABI direction). Design authority: design/04 §1
  (namespace→package compilation model — owning section), design/04 §2
  ("go.mod generation and management"), design/04 §6 (emission mechanism +
  munging conventions).
- Code: pkg/emit (export surface, signatures, doc comments, //export
  wrappers), cmd/cljgo (--lib/--c-shared/--c-archive verbs, buildmode
  driving), docs (public munging contract page).
- Coordination: munging requirements are input to m2-emitter-v0 (if open);
  the compiled-test elimination of the testing-first-class change shares the
  same package layout.
