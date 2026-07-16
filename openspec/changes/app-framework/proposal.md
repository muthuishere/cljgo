# app-framework

## Why

ADR 0041 (proposed; evidence: spike S20) mandates keel — the
batteries-included application framework, library style. S20
demonstrated the risky claims against the real runtime (live var
handlers on a running server, routes-as-data on stdlib ServeMux,
EDN+env config, goroutine workers with a persistence seam) and fixed
the positions: one blessed way per pillar, Ring contract, no ORM, Oban
model on Postgres, Result/`let?` as the error spine. This change turns
the ADR into shippable tiers. The golden-path app in
spikes/s20-app-framework/VERDICT.md is the acceptance test: it must
run interpreted AND compile to a static binary, unmodified.

## What Changes

- New library namespaces shipped with the toolchain: `keel.http`,
  `keel.config`, `keel.db`, `keel.jobs`, `keel.cache`, `keel.ai`
  (cljgo source under `core/keel/`, host adapters in a new
  `pkg/keel` where Go is genuinely needed).
- Interpreter stdlib seed registry grows the packages T1 needs
  (net/http, io, os, time, context) so the REPL story is real, not
  AOT-only.
- `cljgo migrate` subcommand (T2) driving SQL-file migrations.
- Conformance: golden-path app runs dual-harness; each tier adds
  pillar tests with frozen expects; perf budgets per ADR 0024
  (http overhead ≤ 2× native Go handler interpreted, measured seam).

## Tiers

- **T1** server + routes + middleware + config (the 15-minute demo).
- **T2** data layer: pgx pool, query/one/insert/tx, casts→Result,
  migrations.
- **T3** jobs (Postgres-backed, transactional enqueue, :memory dev
  backend) + cache (TTL + singleflight; Redis protocol impl).
- **T4** AI providers (generate, step-key models, fallbacks, log seam).

## Non-goals

- No HTML templating system (owner: not a focus).
- No lifecycle/DI framework, no ORM, no broker-backed queue, no
  annotation or scanning mechanism of any kind.
- Auth ships as a reference design chapter, not code beyond middleware
  helpers.
