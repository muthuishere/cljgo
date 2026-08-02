# Design — apply-adr-0068-cljc-and-repl-cwd-resolution

## Context

`ResolveLibPath` (`pkg/eval/libload.go`) is the single shared resolver for
both execution legs (ADR 0053 invariant). It early-returned `""` whenever
`*file*` was unset — the interactive REPL's normal state — and probed only
`.cljgo`/`.cljg`/`.clj` (ADR 0055). The CLI's `isSourceFile` accepted only
`.clj`/`.cljg`, and `defaultBinaryName` stripped only `.clj`. ADR 0068
ratifies the fixes.

## Goals / Non-Goals

**Goals:**
- `(require 'lib)` finds `lib.cljc` in every context: REPL, `cljgo run`,
  AOT build.
- The REPL resolves cwd namespaces (and gains dependency roots +
  `$CLJGO_PATH`) when `*file*` has no value.
- `cljgo build demo.cljc` / `demo.cljgo` dispatch as source files and name
  the binary correctly.

**Non-Goals:**
- No change to the build-file name set (`build.cljgo`/`.cljg`/`.clj`).
- No shadowing diagnostic (ADR 0055 §3 deferral stands).
- No `:clj` feature answering (ADR 0036 stands).

## Decisions

1. **Extension order `.cljgo` > `.cljg` > `.clj` > `.cljc`** — `.cljc` is
   the portable multi-host fallback, ranked last exactly as the JVM ranks
   `.clj` over `.cljc`. One line in the shared resolver; both legs inherit.
2. **No-file contexts root at `.`** — the branch keys off `*file*` being
   `""`/`NO_SOURCE_FILE`/`NO_SOURCE_PATH`/`REPL`, and *replaces only the
   requiring-file roots*; dependency roots and `$CLJGO_PATH` append after,
   order unchanged. This is the moral equivalent of the JVM's
   cwd-on-classpath (`:paths ["."]`).
3. **CLI recognition via `filepath.Ext` switch** — `isSourceFile` matches
   the four-extension set; `defaultBinaryName` strips the extension only
   when `isSourceFile` accepts it, so `build hello.cljg` → `hello`, and a
   non-source arg is untouched.
4. **Conformance is dual-harness** — a `.cljc` fixture with
   `#?(:clj … :cljs … :cljgo … :default …)` branches is required by a
   `.clj` test whose `;; expect:` freezes the `:cljgo` selections, so REPL
   and compiled binaries cannot diverge on this path.

## Risks / Trade-offs

- Cwd-rooting the REPL could surprise a user with a stray `lib.clj` in cwd
  shadowing nothing-it-shadowed-before — acceptable: providers still
  outrank all roots (`clojure.*` unshadowable), and the JVM behaves the
  same with cwd on the classpath.
- A name existing as both `x.clj` and `x.cljc` now resolves to `x.clj`
  (most-specific-wins, silent) — consistent with ADR 0055 §3.
