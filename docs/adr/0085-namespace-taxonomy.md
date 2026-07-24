# ADR 0085 — Namespace taxonomy: `clojure.*` (language) · `cljg.*` (native stdlib) · `bri.*` (app framework: core/cli/web)

Date: 2026-07-24 · Status: accepted (owner-directed: *"bri should have umbrellas —
bri.core.security, bri.core.ui — some is common, some is CLI"*; *"fs should go to
cljg.fs, no need bri there"*). Organizes ADRs 0069–0084 and every future primitive;
supersedes ADR 0083's flat primitive list.

## Context

bri grew a flat namespace set (`bri.http`, `bri.auth`, `bri.db`, `bri.config`,
`bri.audit`, `bri.otel`, `bri.cli`, `bri.cli.validate`) and a large planned
catalog (ADR 0083: services, cron, secrets, openapi, sys; the CLI primitives). Flat
names do not scale and blur two different things: **general mechanism** any program
wants (filesystem, subprocess, FFI, terminal) versus the **opinionated application
framework** (auth policy, config policy, the web/CLI app shapes). The owner drew the
line: `cljg.fs` — not `bri.fs` — because a filesystem is not a framework concern.

## Decision

### Three umbrellas, by role

1. **`clojure.*` — the language.** The JVM-compatible standard library cljgo
   already provides (`clojure.core`/`string`/`edn`/`set`/`walk`/`zip`/`test`/
   `core.async`…). Reserved for namespaces that exist in JVM Clojure; **never**
   shadowed or extended with cljgo-only code (precedence principle, project CLAUDE.md).

2. **`cljg.*` — cljgo's native extended standard library.** General, unopinionated
   **mechanism** — framework-agnostic, useful to *any* cljgo program (not just a bri
   app). This is the "Bun stdlib" tier. cljgo-specific (these do not exist in JVM
   Clojure), so they live under `cljg.*`, NOT `clojure.*`, to avoid polluting the
   JVM-compatible namespace. **Bundled by concern** (like Go's `io`/`net`/`os`), not
   a flat list — the everyday tiers plus an advanced **`cljg.native`** bundle for the
   heavy/niche system primitives:

   | bundle | leaves | what |
   |---|---|---|
   | **`cljg.io`** | `cljg.io.fs`, `cljg.io.process` | filesystem (read/write/atomic/temp/glob/copy/**watch**) · subprocess/pipeline |
   | **`cljg.net`** | `cljg.net.http`, `cljg.net.openapi` | outbound HTTP client (retry/timeout/circuit) · parse/emit OpenAPI |
   | **`cljg.term`** | `cljg.term` | terminal: color/ANSI/raw-mode/size/keys + the s47 render loop |
   | **`cljg.os`** | `cljg.os` | env · signals · **service** (install/start/stop) · **cron** · ipc/single-instance · **notify** · **clipboard** |
   | **`cljg.native`** | `cljg.native.ffi`, `cljg.native.simd`, `cljg.native.gpu` | *(advanced / "extra")* purego FFI to any `.so`/`.dylib`/`.dll` · SIMD kernels (Go-asm + fallback) · GPU compute |

   The everyday tiers (`cljg.io`/`cljg.net`/`cljg.term`/`cljg.os`) are what a normal
   script reaches for; **`cljg.native`** is the opt-in advanced bundle (the owner's
   "cljg.extra") — a program never links FFI/SIMD/GPU unless it asks. Each leaf is
   still opt-in linked; the bundle is naming, not a link unit. Sources: `cljg.term`
   ← s47; `cljg.os` ← ADR 0083; `cljg.native.*` ← ADR 0084; the rest new.

3. **`bri.*` — the application framework.** Opinionated **policy** for building
   apps, on top of `clojure.*` + `cljg.*`. Split by role:

   - **`bri.core.*`** — shared app concerns (any app shape uses them):
     - `bri.core.security` — auth/JWT/guards + secrets/keystore/login/vault *(= `bri.auth` + new)*
     - `bri.core.config` — layered config/profiles/schema + `.env` + i18n *(= `bri.config`)*
     - `bri.core.data` — data layer: sqlite/pg + migrations *(= `bri.db`)*
     - `bri.core.audit` — audit trail *(= `bri.audit`)*
     - `bri.core.telemetry` — OpenTelemetry tracing *(= `bri.otel`)*
   - **`bri.cli.*`** — the CLI app shape (on `cljg.term`/`cljg.os`/…):
     - `bri.cli` — command tree · args · run · help/dispatch *(shipped)*
     - `bri.cli.validate` — composable validators *(shipped)*
     - `bri.cli.prompt` · `bri.cli.completion` · `bri.cli.doc` · `bri.cli.update`
   - **`bri.web.*`** — the web app shape:
     - `bri.web.http` *(= `bri.http`)* · `bri.web.html` *(= `bri.html`)*

### The dividing rule (`cljg` vs `bri`)

**Mechanism → `cljg`; policy → `bri`.** `cljg.http` is a raw retrying client;
`bri.core.security`'s API-client is opinionated auth. `cljg.term` is raw terminal
I/O; `bri.cli.prompt` is a validator-integrated prompt. `cljg.os` installs *any*
binary as a service; a bri app wires *itself* into it. When unsure, ask "would a
non-bri cljgo program want this exact API?" — yes ⇒ `cljg`, no ⇒ `bri`.

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

- One coherent map for everything: language (`clojure`), stdlib (`cljg`), framework
  (`bri.core`/`cli`/`web`). A reader knows where a thing lives from its prefix, and
  "common vs CLI" is explicit in the name.
- `cljg.*` makes cljgo a real "batteries-included" *language* (fs/process/ffi/term/
  http), usable without bri at all — the Bun-stdlib half of the pitch — while
  `bri.*` stays the opinionated framework.
- The ADR 0083 primitive catalog re-homes: `bri.service`/`cron`/`notify` → `cljg.os`;
  `bri.sys` → `cljg.native.ffi`/`cljg.native.simd`/`cljg.native.gpu`; `bri.secrets` →
  `bri.core.security`; `bri.openapi` parse → `cljg.net.openapi`. Nothing orphaned.
- New primitives are born in the right umbrella from day one.

## Not chosen

- Flat `bri.*` for everything — blurs mechanism vs policy; doesn't scale.
- **`clj.*` as the stdlib prefix** — `clj` is literally the JVM Clojure CLI and
  reads as "Clojure", so it would collide/confuse. `cljg.*` is unambiguously
  cljgo's OWN runtime namespace and cannot be mistaken for Clojure (owner: *"it's
  our runtime, special — don't want to collide"*).
- Putting `cljg.*` under `clojure.*` — those namespaces don't exist in JVM Clojure;
  extending `clojure.*` with cljgo-only code violates the precedence principle.
- Grandfathering the flat names — permanent inconsistency (`bri.auth` beside
  `bri.core.security`); rejected in favor of renaming while the cost is low.
