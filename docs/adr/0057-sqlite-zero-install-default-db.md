# ADR 0057 — SQLite is the zero-install default database
Date: 2026-07-23 · Status: accepted (owner-directed, 2026-07-23; evidence: spike
S37) · **Supersedes the embedded-Postgres dev-database decision of ADR 0041
— both §2 (Tier 0, "provisions the embedded-Postgres dev database … zero
install, dev/prod parity") and §4 (Data pillar).** Paired with ADR 0058
(Postgres).

## Context

ADR 0041 chose **embedded Postgres** as the zero-install dev database
(provisioned by `cljgo dev` when `APP_DB_URL` is unset). Spike S25 left the
*provisioning* an owner-gated question: vendor a ~100 MB downloader vs require
Docker vs a SQLite fallback (then thought to "break prod parity"). The owner now
wants the Bun `bun:sqlite` model: a real, file-backed database with genuine zero
install, upgrading to Postgres in production. Spike S37 proved feasibility.

## Decision

1. **SQLite via pure-Go `modernc.org/sqlite`** (NOT cgo `mattn/go-sqlite3`) is
   the **zero-install default database**. `cljgo dev`/`new` use a file DB
   (`.dev/app.db`, WAL mode) when `APP_DB_URL` is unset; setting
   `APP_DB_URL=postgres://…` switches to pgx (ADR 0058) with **zero app-code
   change**.
2. **One `bri.db` API over both drivers** — `query`/`one`/`insert`/`tx`, plain
   maps out, snake_case↔kebab-case names doctrine (ADR 0041) — a *driver swap,
   not an API fork*. `bri.db` normalizes placeholders (`?`↔`$n`) and names, but
   **not SQL dialect**.
3. This **supersedes ADR 0041's embedded-Postgres dev default in both §2 and
   §4**, and **retracts 0041's "dev/prod parity" wording**: dev-on-SQLite is
   explicitly *not* byte-parity with prod-on-Postgres — 0041's guarantee is
   replaced by the documented dialect seam below plus a `--db=postgres` CI gate.
   Postgres stays the blessed production database (ADR 0058) and the parity
   *target* the CI gate enforces.

## Evidence (S37 — `CGO_ENABLED=0`, Apple Silicon, `modernc` v1.54, measured)

- **Static build: PASS.** `otool -L` shows only `libSystem`/`libresolv` (the
  cgo-free baseline) — no `libsqlite` dylib; `CGO_ENABLED=0` baked in the binary.
- **Size delta: +7.15 MB** (hello 2.49 → +sqlite 9.64 MB) — the whole engine
  compiled to Go, a flat one-time cost.
- **Throughput:** 484k PK reads/s, 13.7k autocommit inserts/s, **1.93M rows/s**
  bulk insert in one tx; startup+first query 1.70 ms.
- **Concurrency: WAL survived 8 writers × 8 readers, zero "database is locked".**

## Consequences

- **+7.15 MB per binary** — the one cost the owner signs off knowingly. Still
  far cheaper than vendoring a ~100 MB embedded Postgres (S25 option never
  proven).
- **The dialect seam is real:** dev-on-SQLite can pass while prod-on-Postgres
  fails on pg-only SQL (`RETURNING`, JSON/enum types, `ON CONFLICT`, native
  dates). Mitigations: docs name the seam explicitly, and a **`cljgo test
  --db=postgres` CI path** runs the suite against real Postgres before release
  (new gate, tracked in the bri-data OpenSpec change).
- SQLite also unlocks **embedded / edge single-binary apps** that never need a
  server DB — a Bun-parity capability Postgres-only could not offer.
- Not chosen: `mattn` cgo SQLite (breaks the static binary); embedded-Postgres
  downloader (size/complexity); SQLite-only (loses production write concurrency).

**Constraint-filter #4 commitment (ADR 0056):** `bri.db` over SQLite lands with
dual-harness conformance (the same `.clj` suite must pass interpreted AND
AOT-compiled — REPL-vs-binary parity is the release blocker) and a perf budget
(ADR 0024): a host-relative wall-clock gate on a representative
insert-then-query workload, calibrated with the implementation, not from S37's
isolated micro-numbers.
