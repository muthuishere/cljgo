# ADR 0056 — cljgo is the Bun of Clojure: a curated batteries set
Date: 2026-07-23 · Status: accepted (owner-directed, 2026-07-23; evidence: spikes
S37–S41) · Umbrella over ADRs 0057–0062; extends the bri framework (ADR 0041).

## Context

Owner mandate (2026-07-22/23): make cljgo **Bun-style** — batteries-included,
fast, one static binary, zero ceremony. Bun's pull is that a single downloaded
binary *already has* SQLite, secrets, fast file I/O, a test runner — no
dependency archaeology before the first useful line. Clojure's historic
weakness is the opposite (ADR 0041): everything past the language is
assembly-required, and the JVM adds an install/classpath tax on top.

cljgo's substrate removes the obstacle: a Go host, a single static binary
(`CGO_ENABLED=0`, the identity in `.goreleaser.yaml`), `require-go` interop, and
the `bri` framework (ADR 0041) already ship the machinery. What was missing was
the *curated set itself* and the discipline that keeps it from becoming a
package zoo. Five validation spikes (S37–S41) built real, runnable prototypes
for the candidate batteries; all came back GREEN with measured evidence. Where a
spike proved a claim only in shape (the interpreted-mode shim, the production
plural dep), the backing ADR says so explicitly rather than laundering a
recommendation into evidence.

The **precedence principle** governs throughout: every battery is additive,
lives under `bri.*`, and never shadows, renames, or changes anything in
clojure.core or the reader.

## Decision

cljgo adopts a **curated batteries set**, each specified in its own ADR, each
validated by a spike:

| ADR | Battery | Namespace | Spike |
|-----|---------|-----------|-------|
| 0057 | SQLite — zero-install default DB | `bri.db` | S37 |
| 0058 | Postgres (pgx) — production data pillar | `bri.db` | S25 |
| 0059 | Unified layered + runtime config | `bri.config` | S38 |
| 0060 | Pluggable secrets / vault | `bri.vault` | S39 |
| 0061 | Streaming file & byte I/O | `bri.io` | S40 |
| 0062 | Internationalisation | `bri.i18n` | S41 |

**The constraint filter — a capability may be blessed as a battery only if it
passes ALL four:**

1. **Pure-Go, static.** It builds `CGO_ENABLED=0` into the single binary. A
   cgo-only backend disqualifies the naive choice; the battery finds the
   pure-Go path or does not ship. (S37 picked `modernc.org/sqlite` over cgo
   `mattn`; S39 *proved* `go-keyring` cgo-free on macOS; S41's prototype
   hand-rolled its plural engine and *recommends* binding pure-Go
   `golang.org/x/text/feature/plural` for CLDR breadth — chosen but un-exercised.)
2. **REPL-live via the bri lazy-shim model** (`pkg/bri`): loads on first
   `require`, no boot tax (ADR 0024), nothing scanned. **Proven end-to-end only
   for the pgx shim (S25);** for S37/S40/S41 the interpreted-mode reach was left
   shape-only, so this is a *design commitment* those batteries' ADRs carry into
   implementation, not measured evidence.
3. **`bri.*`-namespaced.** Batteries are always called namespace-qualified
   (`db/query`, `config/get`, `io/lines`), so reusing a clojure.core name as a
   *qualified* public var is not a shadow — it follows `clojure.string`'s own
   convention (`replace`/`reverse` via `(:refer-clojure :exclude …)`), and the
   *unqualified* core name keeps its meaning everywhere. A name meant to be used
   unqualified or referred renames instead (the ratified `just`/`none` rule).
   `io/byte-chunks` (S40) also renames off `clojure.core/bytes` for clarity.
4. **Dual-harness conformance + a perf budget** (ADR 0024), like every other
   cljgo feature. REPL-vs-binary divergence stays the unforgivable failure mode.
   **Each battery ADR (0057–0062) restates this commitment concretely** — the
   spikes measured feasibility, not the shipped budget; the budget lands *with*
   the implementation.

**Curation, not a zoo:** one blessed way per pillar (ADR 0041 §4). Alternatives
stay possible as documented escape hatches, never as equals.

## Consequences

- cljgo's identity sharpens from "Clojure that compiles to Go" to "the
  batteries-included Clojure you deploy as one static binary" — a
  product-defining surface, the demo and the doc.
- Every battery adds binary weight (S37: +7.15 MB for SQLite). Acceptable given
  the single-binary value proposition, but each ADR states and the owner signs
  off its cost.
- The interpreter's Go-native seed registry must grow per battery for the REPL
  story (an ADR 0041 consequence) — reinforcing interop as priority #1.
- **Sequencing (owner, 2026-07-23): the core per-element perf baseline comes
  first.** Emitted cljgo runs ~35× handwritten Go on compute
  (`CLJGO_PERF_RATIO_MAX`, `TestFactorialPerfBudget`); every battery inherits
  that per-element cost, so the core perf campaign (boxing/dispatch/deref, and
  the chunk-aware `map`/`filter` lever the S-series core audit found) is
  scheduled **before** batteries are implemented. These ADRs record the
  destination; the perf work clears the road to it. Batteries stay decisions,
  not in-flight work, until the baseline moves.
- Not chosen: shipping these as external libraries (defeats batteries-included);
  any cgo backend (breaks the static binary); `cljgo.*` namespaces (breaks the
  precedence principle).
