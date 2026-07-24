# ADR 0085 — Namespace taxonomy: `clojure.*` (language) · `clj.*` (native stdlib) · `bri.*` (app framework: core/cli/web)

Date: 2026-07-24 · Status: accepted (owner-directed: *"bri should have umbrellas —
bri.core.security, bri.core.ui — some is common, some is CLI"*; *"fs should go to
clj.fs, no need bri there"*). Organizes ADRs 0069–0084 and every future primitive;
supersedes ADR 0083's flat primitive list.

## Context

bri grew a flat namespace set (`bri.http`, `bri.auth`, `bri.db`, `bri.config`,
`bri.audit`, `bri.otel`, `bri.cli`, `bri.cli.validate`) and a large planned
catalog (ADR 0083: services, cron, secrets, openapi, sys; the CLI primitives). Flat
names do not scale and blur two different things: **general mechanism** any program
wants (filesystem, subprocess, FFI, terminal) versus the **opinionated application
framework** (auth policy, config policy, the web/CLI app shapes). The owner drew the
line: `clj.fs` — not `bri.fs` — because a filesystem is not a framework concern.

## Decision

### Three umbrellas, by role

1. **`clojure.*` — the language.** The JVM-compatible standard library cljgo
   already provides (`clojure.core`/`string`/`edn`/`set`/`walk`/`zip`/`test`/
   `core.async`…). Reserved for namespaces that exist in JVM Clojure; **never**
   shadowed or extended with cljgo-only code (precedence principle, project CLAUDE.md).

2. **`clj.*` — cljgo's native extended standard library.** General, unopinionated
   **mechanism** — framework-agnostic, useful to *any* cljgo program (not just a bri
   app). This is the "Bun stdlib" tier. cljgo-specific (these do not exist in JVM
   Clojure), so they live under `clj.*`, NOT `clojure.*`, to avoid polluting the
   JVM-compatible namespace. **Bundled by concern** (like Go's `io`/`net`/`os`), not
   a flat list — the everyday tiers plus an advanced **`clj.native`** bundle for the
   heavy/niche system primitives:

   | bundle | leaves | what |
   |---|---|---|
   | **`clj.io`** | `clj.io.fs`, `clj.io.process` | filesystem (read/write/atomic/temp/glob/copy/**watch**) · subprocess/pipeline |
   | **`clj.net`** | `clj.net.http`, `clj.net.openapi` | outbound HTTP client (retry/timeout/circuit) · parse/emit OpenAPI |
   | **`clj.term`** | `clj.term` | terminal: color/ANSI/raw-mode/size/keys + the s47 render loop |
   | **`clj.os`** | `clj.os` | env · signals · **service** (install/start/stop) · **cron** · ipc/single-instance · **notify** · **clipboard** |
   | **`clj.native`** | `clj.native.ffi`, `clj.native.simd`, `clj.native.gpu` | *(advanced / "extra")* purego FFI to any `.so`/`.dylib`/`.dll` · SIMD kernels (Go-asm + fallback) · GPU compute |

   The everyday tiers (`clj.io`/`clj.net`/`clj.term`/`clj.os`) are what a normal
   script reaches for; **`clj.native`** is the opt-in advanced bundle (the owner's
   "clj.extra") — a program never links FFI/SIMD/GPU unless it asks. Each leaf is
   still opt-in linked; the bundle is naming, not a link unit. Sources: `clj.term`
   ← s47; `clj.os` ← ADR 0083; `clj.native.*` ← ADR 0084; the rest new.

3. **`bri.*` — the application framework.** Opinionated **policy** for building
   apps, on top of `clojure.*` + `clj.*`. Split by role:

   - **`bri.core.*`** — shared app concerns (any app shape uses them):
     - `bri.core.security` — auth/JWT/guards + secrets/keystore/login/vault *(= `bri.auth` + new)*
     - `bri.core.config` — layered config/profiles/schema + `.env` + i18n *(= `bri.config`)*
     - `bri.core.data` — data layer: sqlite/pg + migrations *(= `bri.db`)*
     - `bri.core.audit` — audit trail *(= `bri.audit`)*
     - `bri.core.telemetry` — OpenTelemetry tracing *(= `bri.otel`)*
   - **`bri.cli.*`** — the CLI app shape (on `clj.term`/`clj.os`/…):
     - `bri.cli` — command tree · args · run · help/dispatch *(shipped)*
     - `bri.cli.validate` — composable validators *(shipped)*
     - `bri.cli.prompt` · `bri.cli.completion` · `bri.cli.doc` · `bri.cli.update`
   - **`bri.web.*`** — the web app shape:
     - `bri.web.http` *(= `bri.http`)* · `bri.web.html` *(= `bri.html`)*

### The dividing rule (`clj` vs `bri`)

**Mechanism → `clj`; policy → `bri`.** `clj.http` is a raw retrying client;
`bri.core.security`'s API-client is opinionated auth. `clj.term` is raw terminal
I/O; `bri.cli.prompt` is a validator-integrated prompt. `clj.os` installs *any*
binary as a service; a bri app wires *itself* into it. When unsure, ask "would a
non-bri cljgo program want this exact API?" — yes ⇒ `clj`, no ⇒ `bri`.

### Linking + shape unchanged

Every **leaf** is still opt-in linked (ADR 0074/0076) — the umbrella is
organization, not a link unit; a heavy-dep leaf (`bri.core.data`'s SQLite) stays
isolated exactly as today. `clojure.*` stays always-linked stdlib.

## Migration (rename the shipped flat names — do it now, while early)

The project is early ("moving fast", few external users), so renaming now is far
cheaper than after the catalog lands. A follow-up apply PR does the mechanical move:

| today | → | new |
|---|---|---|
| `bri.http` | → | `bri.web.http` |
| `bri.html` | → | `bri.web.html` |
| `bri.auth` | → | `bri.core.security` |
| `bri.config` | → | `bri.core.config` |
| `bri.db` | → | `bri.core.data` |
| `bri.audit` | → | `bri.core.audit` |
| `bri.otel` | → | `bri.core.telemetry` |
| `bri.cli`, `bri.cli.validate` | → | unchanged |

Each rename touches the `bri.Specs()` entry, the `core/bri/*.cljg` file (the
in-ns/refer header, [[cljgo-bri-namespace-convention]]), genbri output, templates,
`docs/`, the site, and conformance/examples — mechanical, gate-verified. The
byte-stable conformance outputs that mention a namespace update in lockstep.

## Consequences

- One coherent map for everything: language (`clojure`), stdlib (`clj`), framework
  (`bri.core`/`cli`/`web`). A reader knows where a thing lives from its prefix, and
  "common vs CLI" is explicit in the name.
- `clj.*` makes cljgo a real "batteries-included" *language* (fs/process/ffi/term/
  http), usable without bri at all — the Bun-stdlib half of the pitch — while
  `bri.*` stays the opinionated framework.
- The ADR 0083 primitive catalog re-homes: `bri.service`/`cron`/`notify` → `clj.os`;
  `bri.sys` → `clj.native.ffi`/`clj.native.simd`/`clj.native.gpu`; `bri.secrets` →
  `bri.core.security`; `bri.openapi` parse → `clj.net.openapi`. Nothing orphaned.
- New primitives are born in the right umbrella from day one.

## Not chosen

- Flat `bri.*` for everything — blurs mechanism vs policy; doesn't scale.
- Putting `clj.*` under `clojure.*` — those namespaces don't exist in JVM Clojure;
  extending `clojure.*` with cljgo-only code violates the precedence principle.
- Grandfathering the flat names — permanent inconsistency (`bri.auth` beside
  `bri.core.security`); rejected in favor of renaming while the cost is low.
