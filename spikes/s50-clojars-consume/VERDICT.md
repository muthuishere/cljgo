# s50 VERDICT — MET (with a scope the ADR must state honestly)

**Date:** 2026-07-27 · **Spike:** s50-clojars-consume · **ADR:** 0095 decision 1

## Question

Can a minimal **pure-Go, zero-dependency** client resolve a real Clojars/Maven
coordinate's transitive `.pom` graph, download the jar, and extract usable
`.clj` source onto a load path — and how thin is the pure subset?

## Outcome: MET

**Pure-Go resolution + extraction works** (kill-condition #1 not triggered), and
**the pure subset is real but narrow and predictably shaped** (kill-condition #2
partially triggered, survives with a tightened, honestly-documented scope).

### Evidence (live network, 7-library sample, `results.txt`)

- Every coordinate resolved and every jar's source extracted with the Go
  standard library alone — `net/http` + `archive/zip` + `encoding/xml` +
  `regexp`, no `go.sum`, no `mvn`, no JVM. Deepest graph resolved: `clj-http`,
  **11 transitive coordinates** across both repos.
- Purity, per-namespace: `tools.cli` 1/1 pure, `medley` fully pure (its
  `java.util` is fenced in a `#?(:clj …)` branch cljgo skips), `hiccup` 8 pure +
  2 Java, `core.match` 5+5, `cheshire` 2+8, `clj-http` 1+9, `data.json` 0+1.
- **2 fully consumable · 4 partially · 1 unusable.**

### What this tightens in ADR 0095

1. **Build the resolver** — it is cheap, pure-Go, and needs no JVM/Aether. The
   two Maven features it does *not* implement (`${property}` interpolation,
   `dependencyManagement` version supply) are outside the "slice real pure
   Clojure libraries need" (ADR 0095 dec 4) — name-error them, don't half-support.
2. **Target is "pure-Clojure libraries," stated honestly.** The reachable set is
   *utility/algorithm* libraries (arg parsing, data helpers, hiccup templating,
   spec-style pure code), **not** the Java-wrapping mainstream (HTTP clients,
   Jackson-backed JSON). The consume guide must say so plainly — same honesty bar
   as the competitive-claims rule. Do not imply "consume the Clojure ecosystem";
   say "consume pure-Clojure libraries; everything else fails loud, per-namespace."
3. **Per-namespace loud-fail (ADR 0054 dec 4) is confirmed correct**, not merely
   assumed: one jar routinely mixes pure and Java namespaces (hiccup 8+2). A
   whole-library gate is wrong in both directions; per-namespace is the only
   honest granularity.
4. **Reader-conditional handling is load-bearing and must be precise at apply
   time.** The cheap positional heuristic here already moved medley PARTIAL→FULLY;
   the production gate needs balanced-paren scanning of `#?(…)` and must confirm
   cljgo supplies the branch it reads (`:cljgo`/`:default`), because a `.cljc`
   whose real body is `:clj`-only gives cljgo nothing loadable. This is the one
   piece worth a focused test corpus during implementation.

## Residual unknowns (carried to apply, not blockers)

- Precise reader-conditional branch resolution for `.cljc` (above).
- `${property}` / `dependencyManagement` version resolution *if* a wanted pure
  lib needs it transitively — deferred until a real case demands it.
- Deploy round-trip is **s51's** question, not this spike's.

**Per ADR 0027 §2 this spike is closed.** ADR 0095 decision 1 stands, with the
scope language above folded into its consume section and the `deps-publish` guide.
