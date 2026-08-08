# ADR 0102 — Fundamentals move to `cljg.*`; `bri.*` is framework + template only

Date: 2026-07-27 · Status: **accepted (implemented 2026-08-08 — the four namespaces live at
`core/cljg/{cache,jobs,secrets,data_cast}.cljg`, and this ADR's own grep-gate condition is now met
by `pkg/briaot/stale_namespace_test.go`. The second condition below — freezing the moved
namespaces' conformance under the new names — is still OUTSTANDING: no `conformance/tests/` file
exercises `cljg.jobs`, `cljg.cache` or `cljg.secrets`, so the move is gated against stale *names*
but not against behavioural regression.)** — was proposed (owner-directed, 2026-07-27: *"fundamentals
will be in cljg while bri is just framework and template stuff"*). Refines
**ADR 0085** (namespace taxonomy) by sharpening the `cljg.*` / `bri.*` boundary and
relocating four recently-shipped namespaces that landed on the wrong side of it.

## Context

ADR 0085 drew the line: `cljg.*` = general **mechanism** any program wants;
`bri.*` = opinionated **application-framework policy**. But four fundamentals
shipped under `bri.core.*` before that line was this sharp:

- `bri.core.cache` (ADR 0093 — `Cache` protocol + local TTL impl)
- `bri.core.jobs` (ADR 0094 — `Queue` protocol + local core.async worker pool)
- `bri.core.secrets` (ADR 0086 — env/keychain secret store)
- `bri.core.data` (cast/validation input gate)

Each is **framework-agnostic mechanism**: a plain CLI or a batch script wants an
in-process cache, a job queue, a secret store, an input validator — none of these
assume a web app. By the owner's test (*"would a non-web CLI program use it?"*)
they are `cljg`, not `bri`.

## Decision

### 1. The boundary test (sharpens ADR 0085)

A namespace is **`cljg.*`** if a non-web, non-framework program would reach for it
(mechanism); it is **`bri.*`** if it encodes an opinion about *how you build an
app* (policy, wiring, templates).

| stays `bri.*` (framework/policy) | moves to `cljg.*` (mechanism) |
|---|---|
| `bri.web.*` (routing, server, html) · `bri.auth` (auth policy) · `bri.config` (app config policy) · `bri.openapi` · `bri.audit` · `bri.cli` (app-CLI shape) | `bri.core.cache` → **`cljg.cache`** · `bri.core.jobs` → **`cljg.jobs`** · `bri.core.secrets` → **`cljg.secrets`** · `bri.core.data` → **`cljg.data.cast`** (or `cljg.validate`) |

`clojure.*` is untouched (faithful JVM-compatible libs only).

### 2. Migration — rename, not rewrite

The four namespaces move under `cljg.*` with **identical protocols and public
APIs** — only the namespace symbol changes. The `Cache`/`Queue` protocols, the
local impls, the secret providers, the cast gate: all keep their shapes (ADRs
0093/0094/0086 stand; this ADR only re-homes them). Each keeps its lazy +
opt-in-linked wiring (ADR 0096 mandate 5) — they already ride the `genbri`/`briaot`
machinery, so the move is a Specs()/registration rename plus the `.cljg` source
relocation and a twin regen.

### 3. Compatibility

cljgo is pre-1.0 and these namespaces are days old and not externally consumed, so
**no deprecation shim is required** — a clean rename is acceptable (owner call).
If any in-tree caller (templates, examples, bri.web) requires the old name, it is
updated in the same change. A grep-gate confirms no stale `bri.core.{cache,jobs,
secrets,data}` reference survives (the double-registration/orphan discipline from
the agent-team playbook).

## Consequences

- **`bri.*` becomes coherent**: it is *only* the app framework + templates — the
  Bun/Rails layer — with every general mechanism underneath it in `cljg.*` or
  `clojure.*`. This is exactly the mental model the owner asked for.
- **`cljg.*` becomes the full "batteries" stdlib**: io/net/os/system/process/date
  (ADR 0101) **plus** cache/jobs/secrets/validate — the framework-agnostic
  fundamentals a plain program uses, all lazy + opt-in.
- **Churn, bounded**: four recently-shipped namespaces rename; protocols/impls
  unchanged; in-tree callers updated in one pass; regenerate the `briaot` twins.
- **Sequencing**: land this before much external code depends on the `bri.core.*`
  names. It is independent of ADR 0096 (contrib libs are `clojure.*`) and can ship
  in parallel.

## Process

No spike — this is a rename of proven code, not a new capability. Implementation
via `/opsx:propose`: relocate the four `.cljg` sources, rename the Specs()
registrations, update in-tree callers, regenerate `briaot`, add a grep-gate for
stale names, and freeze the moved namespaces' existing conformance under the new
names. Gates green in both harnesses, no exceptions.
