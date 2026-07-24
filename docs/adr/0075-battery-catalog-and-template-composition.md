# ADR 0075 — Opt-in battery catalog + template composition (`cljgo new --template web-api`, `--with otel,db,…`)
Date: 2026-07-24 · Status: proposed (roadmap; owner-directed "take care later") · depends on ADR 0074's per-namespace conditional linking

## Context
bri is now first-class: API tier (ADR 0069), AOT + Docker (ADR 0071), the
`bri.db` data layer (ADR 0072), the resource generator (ADR 0073), and opt-in
`bri.otel` tracing (ADR 0074). The "Bun of Clojure" ambition (ADR 0056) now
needs breadth: a **curated catalog of opt-in `bri.*` batteries** wrapping
best-in-class **pure-Go** libraries, and a way for **templates to compose a
chosen subset** so `cljgo new --template web-api` (or `--with otel,db,jobs`)
hands you a pre-wired stack instead of a bare app.

Two enablers already exist or are landing:
- ADR 0074 makes `pkg/briaot` linking **per-namespace conditional** — a battery
  links (and pulls its Go deps) ONLY when the app `require`s it, so a catalog of
  many batteries costs nothing until used. This is the load-bearing prerequisite;
  without it, adding libraries bloats every binary (the ADR 0072 SQLite tradeoff).
- ADR 0047 makes templates real, runnable source; ADR 0021 (`build.cljgo` /
  `go-require`) already links third-party Go with zero bindings.

The batteries direction (ADRs 0056–0062: sqlite, pgx, config, vault, io, i18n)
recorded intent but shipped little; this ADR is the umbrella that turns that into
a growing, composable catalog.

## Decision
1. **A curated opt-in `bri.*` battery catalog.** Each battery is a separate
   namespace, one blessed way per pillar (precedence principle — never shadow
   clojure.core), pure-Go (CGO_ENABLED=0 sacred), dual-mode (interpreted +
   AOT, byte-identical), and linked only when required (ADR 0074). The seed
   catalog (each its own future ADR → spec → dual-mode → gates, delivered
   incrementally):

   | namespace | pillar | candidate pure-Go backing |
   |---|---|---|
   | `bri.http` `bri.auth` `bri.audit` `bri.html` `bri.config` | **shipped** (0069/0071) | stdlib net/http, x/crypto |
   | `bri.db` | data | modernc.org/sqlite, jackc/pgx | *(shipped, 0072)* |
   | `bri.otel` | tracing | go.opentelemetry.io/otel | *(0074)* |
   | `bri.cache` | cache / KV | in-proc + redis (redis/go-redis) |
   | `bri.jobs` | background jobs / queue | river / asynq-style over pgx or redis |
   | `bri.mail` | email | stdlib net/smtp + provider APIs |
   | `bri.client` | outbound HTTP | stdlib net/http + retry/circuit |
   | `bri.validate` | request validation | pure-Go, bri-native spec DSL |
   | `bri.openapi` | API schema / docs | generate OpenAPI from routes+validate |
   | `bri.ws` | websockets / SSE | nhooyr/coder websocket (pure-Go) |
   | `bri.storage` | object storage | S3/GCS via pure-Go SDKs |
   | `bri.cron` | scheduling | robfig/cron |
   | `bri.i18n` `bri.io` `bri.vault` | i18n / streaming io / secrets | *(0060/0061/0062 seeds)* |

   The catalog is append-only and lives in a manifest (`docs/batteries.md` or a
   registry) so `cljgo` can list available batteries and their status.

2. **Template composition.** Two composable mechanisms:
   - **Named template variants** — `web` (current, minimal bri), `web-api`
     (API-first: bri.http API-defaults + bri.db + bri.validate + bri.otel
     pre-wired + a health/metrics/trace-ready `-main`), and room for more
     (`web-full`, `cli`, `lib`). A variant is real source (ADR 0047) that
     already requires + wires its batteries.
   - **`--with <battery,…>`** — additive selection layered onto a base
     template: `cljgo new --template web --with otel,db,jobs` requires each
     battery, adds its config keys to `conf.edn`, and wires its default
     middleware/setup into `main`. Each battery ships a small, declarative
     "wire-in" fragment the generator splices (same marker mechanism as the
     resource generator, ADR 0073).

   The resource generator (ADR 0073) already establishes the splice-at-markers
   pattern; battery wire-in reuses it.

3. **Constraints (binding on every catalog entry).** Pure-Go / CGO_ENABLED=0;
   opt-in with zero cost when unused (ADR 0074 conditional linking); one blessed
   way per pillar; dual-mode parity is a release blocker; secrets-are-env; each
   battery is independently AOT-verifiable and Docker-deployable.

## Consequences
`cljgo new --template web-api` becomes a genuine batteries-included start —
API + DB + validation + tracing + metrics, in a lean static binary that only
links what it wired. The catalog grows one ADR at a time without bloating
existing apps (conditional linking guarantees it). Risk: catalog sprawl and
"one blessed way" erosion if entries aren't curated — mitigated by requiring a
per-battery ADR (context/decision/blessed-choice) before it joins. This ADR is
a **roadmap**: it ratifies the *shape* (opt-in catalog + composed templates)
and the constraints; individual batteries and the `--with` mechanism are
delivered later, each on its own ADR/spec/gates, gated behind ADR 0074's
conditional-linking landing. Supersedes ADR 0056's umbrella framing by making
"batteries" concretely opt-in-and-composable rather than always-linked.
