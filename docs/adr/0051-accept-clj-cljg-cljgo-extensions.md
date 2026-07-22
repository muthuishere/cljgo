# ADR 0051 — Source and build files accept `.clj`, `.cljg`, and `.cljgo`

Date: 2026-07-22 · Status: accepted (owner-directed, 2026-07-22) · Refines the
resolver contract of **ADR 0042** / **ADR 0036** and the single-name build-file
assumption of **ADR 0021**.

## Context

Two separate extension conventions had grown up independently:

- The **source resolver** (`ResolveLibPath`, `pkg/eval/libload.go`) tried
  `.clj` then `.cljg`.
- The **build file** was a single fixed name, `build.cljgo`
  (`pkg/build/build.go:31`, ADR 0021).

Owner decision (2026-07-22): rather than force one extension or rename the
build file, **accept all three — `.clj`, `.cljg`, `.cljgo` — everywhere**, for
both source namespaces and the build file. `.cljg`/`.cljgo` are cljgo-native
source; `.clj` is the portable extension JVM Clojure also reads, which matters
for `publish clojars` (ADR 0050). Being permissive costs nothing and removes a
naming decision users would otherwise trip on.

This supersedes an in-flight idea to *drop* `.cljgo` and rename the build file
to `build.cljg`; that rename is not done — `build.cljgo` stays valid and
remains the canonical default.

## Decision

1. **Source resolution accepts all three**, precedence **most-specific-first:
   `.cljgo` > `.cljg` > `.clj`.** `.cljg`/`.cljgo` (cljgo-native) win over `.clj`
   (portable fallback) — mirroring Clojure's own host-extension pattern (a JVM
   host prefers `.clj` over `.cljc`, ClojureScript prefers `.cljs`). This
   reverses the prior `.clj`-first order. Because `ResolveLibPath` is the
   single shared resolver (S30), both execution legs inherit this identically —
   dual-mode parity by construction (ADR 0049 invariant), no second resolver.

2. **The build file is probed, not fixed:** `cljgo build` looks for
   `build.cljgo`, then `build.cljg`, then `build.clj` (same precedence),
   accepting the first present. `build.cljgo` remains the name `cljgo new`
   emits and the canonical default in error messages.

3. **Ambiguity is most-specific-wins, silently** — consistent with the
   load-path first-wins of the ADR 0048 / S30 design. A shadowing diagnostic
   may be added later; it is not a semantic change.

## Consequences

- **Additive for source, additive for the build file** — every file that
  resolved before still resolves; `.cljgo` source and `build.cljg`/`build.clj`
  now also resolve. The one behavior change is precedence when a name exists in
  two extensions at once (now most-specific wins, was `.clj`).
- **`.clj` acceptance aligns the ecosystem-bridge story** (ADR 0050): the same
  library's `.clj` files are what JVM Clojure reads, while cljgo prefers its
  native `.cljg`/`.cljgo` — the `.cljc`-style split, for free.
- **Templates are unchanged** — `build.cljgo` + `.cljg` sources keep working;
  no forced churn, closed spikes and historical ADRs keep their `build.cljgo`
  references as accurate records.
- Extends ADR 0042's resolver contract (the extension set is now three) and
  ADR 0021's build-file rule (name set, not a single name).
