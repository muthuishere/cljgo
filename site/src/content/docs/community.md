---
title: Discuss & contribute
description: Comment on any docs page via GitHub Discussions, file issues, and find the ADRs, design docs, and conformance discipline behind cljgo.
---

## Comment on any page

Every page on this site has a public comment box at the bottom, powered by
[giscus](https://giscus.app) and backed by GitHub Discussions on
`muthuishere/cljgo`. Sign in with your GitHub account and comment — each page
maps to its own discussion thread, and everything is visible to everyone at
[github.com/muthuishere/cljgo/discussions](https://github.com/muthuishere/cljgo/discussions).
Questions, corrections, and "this didn't work on my machine" reports are all
welcome there.

## File an issue

Bugs and feature requests go to
[github.com/muthuishere/cljgo/issues](https://github.com/muthuishere/cljgo/issues).
For a bug, the ideal report is a minimal `.clj` file plus the output of both
legs — `cljgo run file.clj` and the compiled binary from
`cljgo build file.clj` — since any divergence between them is treated as a
release blocker. `cljgo check file.clj --json` output helps too: errors carry
registered diagnostic codes (see
[error codes & diagnostics](/cljgo/reference/diagnostics/)).

## Where the decisions live

cljgo is decision-logged. If you want to know *why* something is the way it
is, the sources in the repo are, in authority order:

- **`docs/adr/`** — the ADR decision log. Every non-trivial decision is a
  numbered ADR; decisions are superseded by newer ADRs, never edited away.
- **`design/00-architecture.md`** — cross-component contracts and the M0–M5
  roadmap; `design/01–07` cover component internals (reader, data structures,
  analyzer/eval, emitter, interop, concurrency), and `design/08` is the
  Zig-model build/comptime direction.
- **`openspec/`** — active change proposals.

The [architecture page](/cljgo/reference/architecture/) is the guided tour.

## The conformance discipline

The bar for correctness is not "looks right" — it is JVM Clojure 1.12.5, the
semantic oracle. Every semantic behavior gets a file in `conformance/tests/`
with frozen expected output, verified against real JVM Clojure via the
`clojure` CLI and cited in a comment. The same files run twice on every
commit — once through the tree-walk evaluator and once AOT-compiled — and the
outputs must match byte-for-byte. A REPL↔binary divergence is the one
unforgivable failure mode.

If you contribute a semantic change, that is the shape it takes: a
conformance file, oracle-cited, passing on both harnesses. The repo gates
(`go build ./... && go vet ./... && gofmt -l … && go test ./...`) run before
every commit.

## Start somewhere

- New here? [Why cljgo](/cljgo/why/), then [install](/cljgo/install/) and the
  [quickstart](/cljgo/quickstart/).
- Checking compatibility claims? [Compatibility](/cljgo/reference/compatibility/)
  and [benchmarks](/cljgo/reference/benchmarks/) — every number is
  reproducible from the repo.
