# app-framework

## Why

ADR 0041 (proposed; evidence: spike S20) mandates keel — the
batteries-included application framework, library style. S20
demonstrated the risky claims against the real runtime (live var
handlers on a running server, routes-as-data on stdlib ServeMux,
EDN+env config, goroutine workers with a persistence seam) and three
adversarial DHH-persona review rounds fixed the positions: a tier-0
generator carries the conventions (its output IS the golden page),
the beginner surface is `!` forms + a documented error table
(Result crosses http only through the visible `http/render` bridge),
default-on security middleware (inspectable-data defaults; CSRF with
`html/form` tokens, sessionless-JSON posture), casts on day one, no
I/O at namespace load (workers start in `-main`, shutdown wired via
`:drain`), a zero-install embedded-Postgres dev database, a names
doctrine, single-binary deployment (embedded `public/` +
`migrations/`, `./myapp migrate && ./myapp`), one blessed way per
pillar, Ring contract, no ORM, Oban model on Postgres with the
`:memory` backend (ADR 0040) tests-only, live vars uniformly (http
AND jobs). The golden-path app in spikes/s20-app-framework/VERDICT.md
is the acceptance test: it must run interpreted AND compile to a
static binary, unmodified, and satisfy this spec.

## What Changes

- New library namespaces shipped with the toolchain: `keel.http`,
  `keel.html`, `keel.config`, `keel.db`, `keel.jobs`, `keel.cache`
  (cljgo source under `core/keel/`, host adapters in a new
  `pkg/keel` where Go is genuinely needed). `keel.ai` is OUT of this
  change — an independently versioned satellite specced separately
  after T1 boots a generated app.
- `cmd/cljgo` grows subcommands: `new` (scaffold, incl. `--with-auth`
  copy-in), `dev` (serve + nREPL; + embedded dev db and migrations
  from T2), `migrate`, `config` (resolved-map explain), `routes`
  (effective middleware/route stack).
- Interpreter stdlib seed registry grows the packages T1 needs
  (net/http, io, os, time, context) so the REPL story is real, not
  AOT-only — and nothing below T1 proceeds until a generated app
  boots.
- Guides ship as gated deliverables per tier: 15-minute tutorial,
  per-pillar guides, names doctrine, deployment guide, auth chapter,
  production checklist.
- Conformance: golden-path app runs dual-harness; each tier adds
  pillar tests with frozen expects; perf budgets per ADR 0024
  (interpreted http overhead ≤ 2× native Go handler, measured seam).

## Tiers (topologically sorted — every generated verb has a same-tier implementation)

- **T0** `cljgo new` / `cljgo dev` — the 15-minute magic (blessed
  layout, styled page, first test; no db verbs).
- **T1** server + html (+form/CSRF) + routes + middleware defaults +
  config + app-test client.
- **T2** data layer: pgx pool, query/one/insert/tx, casts→Result,
  names doctrine, migrations, embedded-Postgres dev database,
  single-binary deployment, test sandbox.
- **T3** jobs (Postgres-backed, transactional validated enqueue,
  live var handlers, drain wiring; :memory backend tests-only) +
  cache (TTL + singleflight; Redis protocol impl).

## Non-goals

- No template DSL or asset pipeline (keel.html is a fn over data;
  `html/form` is the deliberate outer boundary; owner: templating is
  not a focus).
- No lifecycle/DI framework, no ORM, no broker-backed queue, no
  annotation or scanning mechanism, no ambient registries (including
  shutdown).
- Auth beyond sessions/CSRF ships as generated code you own
  (`--with-auth`) plus its guide — not a framework module.
- `keel.ai` (deferred satellite change; positions fixed in ADR 0041).
