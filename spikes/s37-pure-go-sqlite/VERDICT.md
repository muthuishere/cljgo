# S37 VERDICT — pure-Go SQLite as cljgo's zero-install default DB

Status: CLOSED. Recommendation: **BLESS SQLite (`modernc.org/sqlite`) as
the zero-install default database — CONDITIONAL on the owner accepting a
~7 MB binary-size cost and a documented dev(sqlite)→prod(pg) dialect
seam. This reverses ADR 0041's embedded-Postgres-dev choice in favour of
the Bun `bun:sqlite` model. Recommend YES.**

Measured on Apple Silicon (arm64), Go 1.26.3, `modernc.org/sqlite`
v1.54.0, `CGO_ENABLED=0`. Reproduce: `./run.sh`. Numbers are a single
warm run — treat as order-of-magnitude, not benchmark-suite precision.

## Evidence per exit criterion

| # | criterion | result |
|---|---|---|
| 1 | static build, no libsqlite | **PASS** — `CGO_ENABLED=0` baked into the binary (`go version -m`); `otool -L` deps are only `libSystem.B.dylib` + `libresolv.9.dylib` (the macOS system libs every cgo-free Go binary links). **No libsqlite dylib.** |
| 2 | binary-size delta | hello **2.49 MB** → hello+sqlite **9.64 MB** = **+7.15 MB** the battery adds |
| 3 | latency + throughput | startup+first query **1.70 ms**; single-row insert **13,662/sec** (73 µs/op, autocommit); single-row read (PK, prepared) **484,411/sec** (2.06 µs/op); bulk insert in one tx **1,929,469 rows/sec** |
| 4 | one API over two drivers | **PASS** — the same four verbs (`query`/`one`/`insert`/`tx`, plain maps, snake↔kebab names, tx rollback) ran for real over a **file DB** and a **`:memory:` DB**; `pgxStub` satisfies the identical Go `DB` interface (compile-time), so prod is a driver swap |
| 5 | single-binary deploy / WAL | **PASS** — `journal_mode=wal` engaged; 8 writers×2000 + 8 readers×2000 over ONE file: **16,000 writes + 16,000 reads, ZERO `database is locked` errors**, ~18,354 writes/sec under contention. DB file lives next to the binary (`.dev/app.db` + `-wal`/`-shm`) |

### Criterion 1 — the static-build proof, verbatim

```
$ go version -m ./s37probe | grep -i cgo
	build	CGO_ENABLED=0
$ otool -L ./s37probe
	/usr/lib/libSystem.B.dylib
	/usr/lib/libresolv.9.dylib          # <- no libsqlite; system libs only
```

`mattn/go-sqlite3` (the popular driver) is DISQUALIFIED — it needs cgo,
which breaks cljgo's single-static-binary identity. `modernc.org/sqlite`
is the only cgo-free option and it passes cleanly: it is a pure-Go
machine-translation of SQLite's C amalgamation, so the whole engine
links into the Go binary with nothing dynamic.

## Positions

**Bless `modernc.org/sqlite`.** It is the ONLY driver compatible with
`CGO_ENABLED=0`, and it works: real WAL, real transactions, real
prepared statements, correct rollback, and enough throughput
(13.6k autocommit inserts/sec, 484k point reads/sec, 1.9M rows/sec bulk)
for any dev workload and most small production ones. Zero install: no
Docker, no download, no `zonky`-style 100 MB embedded-Postgres fetch —
the engine is already in the binary the user already has.

**This is the right reversal of ADR 0041.** S25's owner call #1 laid out
three options for the zero-install dev DB: (a) vendor embedded Postgres
(~100 MB first-run download, real parity), (b) require Docker (not
zero-install), (c) SQLite fallback (zero-install, imperfect parity). S25
recommended (a) but never proved it. This spike proves (c) is cheap and
real. The Bun precedent is decisive: `bun:sqlite` shipping in the runtime
is why "just start writing" works with no setup — that is exactly the
cljgo `cljgo new && cljgo dev` promise. The parity cost is real but
bounded (below) and the zero-install win is total.

**The size cost is the honest tax: +7.15 MB.** That is the whole SQLite
engine compiled to Go. For a language runtime that is acceptable (Bun
itself is ~90 MB); for cljgo it roughly triples a trivial binary but is
a flat one-time cost, not per-app. It is the single number the owner
must sign off on. Mitigation if it ever bites: gate sqlite behind the
same lazy `bri.*` shim as http/config so a binary that never `(require
'bri.db)` could in principle be built without it — but Go links by
import, not by runtime require, so realizing that saving needs a build
tag, not just laziness. Flagged, not solved.

**The blessed `bri.db` API is driver-agnostic by construction.** One
Go interface (`Query`/`One`/`Insert`/`Tx`), two impls (sqlite default,
pgx prod), selected from `APP_DB_URL` with **zero branches in app code**
(`shapes/bri-db-sqlite-default.cljg`). This is the same verb set S25
blessed for pgx — so S37 does not add an API, it adds a *driver* under
the API S25 already chose. The names doctrine (snake_case column ↔
kebab-case keyword) round-trips identically on both.

## The one real caveat — the dev/prod dialect seam

"Dev on SQLite, prod on Postgres" is NOT free parity. What bri.db can
paper over, and what it cannot:

- **Placeholders** — `?` (sqlite) vs `$1` (pg): bri.db normalises. App
  authors write `?` everywhere; the pgx adapter rewrites. Solved.
- **Names doctrine** — identical both sides. Solved.
- **SQL dialect** — `RETURNING`, JSON/array/enum types, `ON CONFLICT`
  spelling, `AUTOINCREMENT` vs `SERIAL`/`IDENTITY`, timestamp handling
  (sqlite has no native date type; we stored `created_at` as int
  millis) — these DIVERGE and bri.db does NOT hide them. An app that
  leans on Postgres-only SQL will pass its sqlite dev tests and fail in
  prod. This is the honest failure mode of the Bun model and must be
  documented as the seam, with a `cljgo test --db=postgres` path
  recommended before release so CI catches divergence. This is the
  price of the reversal and the owner should accept it knowingly.

## What I did NOT prove

- **Interpreted-mode reach** — same answer as S25/T1: bri.db is a Go
  shim interned lazily into `bri.db`, so the interpreter reaches sqlite
  the way it reaches http (`pkg/bri` pattern). Not wired here — shape
  only.
- **Migrations on sqlite** — S25 proved the migration engine on pg; I
  did not re-run it against sqlite (DDL dialect differs slightly:
  `INTEGER PRIMARY KEY` vs `BIGSERIAL`). Low risk, unmeasured.
- **Linux static build** — proved on macOS arm64 only. `modernc.org/sqlite`
  is pure Go so Linux `CGO_ENABLED=0` should yield a fully-static ELF
  (even more self-contained than macOS, which always links libSystem),
  but I did not run `ldd` on a Linux artifact. Verify in CI.
- **Concurrency ceiling** — 8+8 goroutines over one file was clean, but
  SQLite is single-writer by design; a write-heavy multi-tenant server
  will serialise writers. Fine for dev and light prod; the prod-upgrade
  path to pg exists precisely for when it isn't.
- **Absolute throughput vs pgx** — different machines/transports; not
  compared head-to-head. The point is sqlite is *sufficient*, not
  faster.
- **`:memory:` + connection pool** — `:memory:` is per-connection in
  SQLite; a multi-conn pool over `:memory:` sees separate DBs. Tests
  must pin one connection (or use `file::memory:?cache=shared`). Noted,
  not wired.

## Owner-gated call

**Replace ADR 0041's embedded-Postgres dev default with a SQLite
zero-install default (`modernc.org/sqlite`), Bun-style, prod upgrading
to pgx via `APP_DB_URL`. Recommend YES**, conditional on the owner
accepting:

1. **+7.15 MB** on every cljgo binary (the SQLite engine, always linked).
2. **A documented dev/prod dialect seam** — bri.db hides placeholders and
   names but not SQL dialect; ship a `cljgo test --db=postgres` CI path
   so parity gaps surface before release.

If both are acceptable (they align with the "zero-install, just start
writing" mandate and the existing prod-Postgres upgrade path), this is
a clean win over vendoring a 100 MB embedded-Postgres downloader. A new
ADR should supersede the relevant part of 0041 and record the seam
doctrine.
