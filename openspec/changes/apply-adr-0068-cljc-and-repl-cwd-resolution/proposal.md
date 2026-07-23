# apply-adr-0068-cljc-and-repl-cwd-resolution

## Why

A multi-host library targets cljgo through `.cljc` files with `#?(:cljgo …)`
reader conditionals (ADR 0036), but three gaps break the story in practice
(ADR 0068): `require` cannot load a `.cljc` namespace (the resolver probes
only `.cljgo`/`.cljg`/`.clj`), the interactive REPL cannot `require` ANY
cwd namespace (`*file*` is `NO_SOURCE_FILE`, so resolution bails before
dependency roots and `$CLJGO_PATH` are consulted), and the CLI's
source-file test disagrees with the resolver (`cljgo build demo.cljgo` /
`demo.cljc` fall through to build-step dispatch).

## What Changes

- `ResolveLibPath` probes four extensions, most-specific-first:
  `.cljgo` > `.cljg` > `.clj` > `.cljc` (ADR 0068 §1, refining ADR 0055).
- When `*file*` provides no requiring-file context (unset, `NO_SOURCE_FILE`,
  `NO_SOURCE_PATH`, `REPL`), the resolver roots at the process cwd instead
  of returning empty; dependency roots and `$CLJGO_PATH` still append after
  (ADR 0068 §2).
- `isSourceFile` in the CLI accepts all four extensions; `defaultBinaryName`
  strips whichever accepted extension is present (ADR 0068 §3).
- The build-file name set is unchanged (`build.cljgo`/`.cljg`/`.clj`).
- Dual-harness conformance test freezes `.cljc` require + reader-conditional
  selection.

## Capabilities

### New Capabilities
- `source-resolution`: how a namespace symbol resolves to a source file —
  the accepted extension set and precedence, the candidate roots (requiring
  file, cwd fallback, dependency roots, `$CLJGO_PATH`), and the CLI's
  recognition of source-file arguments.

### Modified Capabilities

(none — `host-resolution-parity` governs host references, not namespace
source lookup; no existing spec covers the extension set)

## Impact

- `pkg/eval/libload.go` — extension list + no-file cwd fallback (shared
  resolver: interpreter and emitter inherit identically, ADR 0053 invariant).
- `cmd/cljgo/main.go` — `isSourceFile`, `defaultBinaryName`.
- `conformance/tests/` — new `.cljc` fixture + test file.
- Purely additive: every name that resolved before resolves to the same file.
