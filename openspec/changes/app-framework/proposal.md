# app-framework

## Why

ADR 0041 (proposed; evidence: spike S20) mandates keel — the
batteries-included application framework, library style. S20
demonstrated the risky claims against the real runtime (live var
handlers on a running server, routes-as-data on stdlib ServeMux,
EDN+env config, goroutine workers with a persistence seam) and three
adversarial DHH-persona review rounds fixed the positions: a tier-0
generator carries the conventions (its output IS the golden page),
the beginner surface is `!` forms + a documented error table,
default-on security middleware, casts on day one, no I/O at namespace
load, one blessed way per pillar, Ring contract, no ORM, Oban model
on Postgres (:memory backend per ADR 0040), live vars uniformly
(http AND jobs), keel.ai as an independently versioned satellite. The
golden-path app in spikes/s20-app-framework/VERDICT.md is the
acceptance test: it must run interpreted AND compile to a static
binary, unmodified.

## What Changes

- New library namespaces shipped with the toolchain: `keel.http`,
  `keel.html`, `keel.config`, `keel.db`, `keel.jobs`, `keel.cache`,
  `keel.ai` (cljgo source under `core/keel/`, host adapters in a new
  `pkg/keel` where Go is genuinely needed).
- `cmd/cljgo` grows subcommands: `new` (scaffold, incl. `--with-auth`
  copy-in), `dev` (migrate + serve + nREPL), `migrate`, `config`
  (resolved-map explain).
- Interpreter stdlib seed registry grows the packages T1 needs
  (net/http, io, os, time, context) so the REPL story is real, not
  AOT-only.
- Guides ship as gated deliverables per tier: 15-minute tutorial,
  per-pillar guides, auth chapter, production checklist.
- Conformance: golden-path app runs dual-harness; each tier adds
  pillar tests with frozen expects; perf budgets per ADR 0024
  (interpreted http overhead ≤ 2× native Go handler, measured seam).

## Tiers

- **T0** `cljgo new` / `cljgo dev` — the 15-minute magic (blessed
  layout, config schema, first migration, first test, rendered page).
- **T1** server + html + routes + middleware + config + sessions/CSRF.
- **T2** data layer: pgx pool, query/one/insert/tx, casts→Result,
  migrations.
- **T3** jobs (Postgres-backed, transactional enqueue, live var
  handlers, :memory dev backend) + cache (TTL + singleflight; Redis
  protocol impl).
- **T4** AI providers (generate, step-key models, fallbacks, log
  seam).

## Non-goals

- No template DSL or asset pipeline (keel.html is a fn over data;
  owner: templating is not a focus).
- No lifecycle/DI framework, no ORM, no broker-backed queue, no
  annotation or scanning mechanism, no ambient global handles.
- Auth beyond sessions/CSRF ships as generated code you own
  (`--with-auth`) plus its guide — not a framework module.
