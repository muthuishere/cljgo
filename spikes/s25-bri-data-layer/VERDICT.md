# S25 VERDICT — bri's data layer

Status: CLOSED. Recommendation: **ADR 0041 T2's sketch survives contact
with a real Postgres — pgx + inline-SQL-strings + plain-map rows is the
right blessed form. Ship it. Two owner calls below.**

Measured on Postgres 17.10 (Docker, localhost), pgx v5.10.0 vs
database/sql + lib/pq v1.10.9, Go 1.26.3, Apple Silicon. Numbers are
localhost round-trips (network-dominated); treat ratios, not absolutes,
as the signal. Reproduce: `./run.sh`.

## What the probe measured

| criterion | result |
|---|---|
| 1 — driver, 1-row read | pgx **70.5µs/op** vs database/sql **153.9µs/op** — pgx **2.18×** faster |
| 1 — driver, 100-row read | pgx (CollectRows→struct) **91.6µs/op** vs database/sql (manual scan) **87.1µs/op** — **0.95×**, a wash |
| 2 — names doctrine | `full_name` ↔ `:full-name`, `email_address` ↔ `:email-address`, both directions, only case renamed — PASS |
| 2 — names cost | row→Clojure-map marshalling adds **27.3%** over a raw struct scan (146.7µs vs 115.2µs for 100 rows) |
| 3 — REPL liveness | query held in `#'user-sql`, re-`def`'d live on a running pgx pool, next call ran the NEW SQL — no reconnect — PASS |
| 6 — migrations | 2 UTC-stamped SQL files, ledger table, re-run applied 0 — idempotent, additive-only — PASS (engine is ~15 LOC) |

## Positions per question

**Driver: pgx, confirmed by number.** pgx's native protocol is 2.18×
faster than database/sql on the single-row read that dominates web
handlers, and never loses. The 100-row wash is an artifact of pgx's
reflection-based `CollectRows` vs a hand-written scan loop — bri's
marshaller (which is hand-written, criterion 2) does not pay that
reflection tax, so real bri 100-row reads stay on pgx's fast side.
ADR 0041's "pgx behind bri.db, database/sql as the hatch" stands.

**Blessed API shape: inline SQL strings (shape A).** Three shapes were
written as real code (`shapes/`):
- **A, inline SQL** — SQL is a string in the handler's own var. On cljgo
  specifically this is the shape that is *already live*: re-`def` the
  handler, the new SQL runs (criterion 3, measured). Fewest concepts.
- **B, HugSQL `.sql` files** — nice for big query catalogs, but
  `def-queries` interns the vars ONCE at load; editing the `.sql` does
  not re-run it, so liveness needs a re-eval/file-watcher. A legitimate
  T2 *hatch* library, not the blessing.
- **C, Datomic-ish entity/pull** — is an ORM by another name (the schema
  map is the mapping layer 0041 rules out), owns a large pull→SQL
  compiler cljgo would maintain, and hides the SQL lever. Not chosen;
  buildable as a downstream library.

The precedence principle applies cleanly: SQL strings keep the
Postgres-fluent user's lever, and the liveness advantage is unique to
cljgo — so inline SQL is not just "simplest", it is the shape that best
uses cljgo's unfair advantage.

**Names doctrine costs 27%, and that is acceptable.** Turning rows into
idiomatic kebab-keyword maps is measurably not free, but it is a
per-row constant an app can opt out of (a `:raw` read returning vectors)
for a hot path. The 27% buys every downstream form being ordinary
Clojure map access — the right default.

**Interpreted-mode reach: a Go shim, NOT self-rebuild.** The liveness
proof (criterion 3) *is* the answer: pgx was captured in a Go closure
and interned as a native fn into the namespace — the exact model
`pkg/bri` already uses to ship net/http lazily. So an interpreted bri
app reaches pgx because bri compiles pgx into the cljgo binary as a
shim, and the re-`def` cost is just an evaluator eval (microseconds), no
`go get`/recompile. design/05's `syscall.Exec` self-rebuild stays for
*arbitrary user* `require-go` deps; the framework's own driver does not
need it. This closes S20's honesty note for data the same way T1 closed
it for http.

**No-ORM claim, concretely.** The blessed CRUD path is: a `bri.db`
require, a cast schema (one map), and SQL strings. Concept count vs
Spring Boot JPA for the same "create + read a user":
- JPA: `@Entity`/`@Table`/`@Id`/`@GeneratedValue`/`@Column` annotations,
  an entity class, a `JpaRepository<User,Long>` interface, an
  `EntityManager`/persistence-context lifecycle, `application.properties`
  datasource + dialect + ddl-auto, `@Transactional` — roughly 6–8
  framework concepts before the first row.
- bri: `(cast/schema …)` + `(db/insert! pool :users attrs)` +
  `(db/one! pool "SELECT …" id)` — 3 fns, zero classes, zero
  annotations, zero lifecycle object. The mapping is one doctrine
  (snake↔kebab), not a per-field declaration.

## Owner calls (options + recommendation, not a silent pick)

1. **Embedded dev Postgres provisioning.** ADR 0041 promises `cljgo dev`
   provisions embedded Postgres when `APP_DB_URL` is unset ("zero
   install"). This spike used a Docker Postgres and did NOT prove the
   embedded path. Options: (a) vendor a `zonky`-style embedded-postgres
   binary downloader (real parity, ~100MB download on first run);
   (b) require Docker (simpler, but not "zero install"); (c) ship an
   SQLite dev fallback (zero install, breaks prod parity — violates the
   0041 parity mandate). **Recommend (a)**; it is the only option that
   keeps 0041's parity promise, but it is a real build/vendoring cost
   the owner should greenlight before T2 commits to it.

2. **Ecto-Sandbox test transactions.** 0041 promises per-test rollback
   fixtures "same var, no with-redefs". Not prototyped here. The pgx
   pool would need a test mode that hands every checkout the same
   pinned, rolled-back transaction. This is achievable but is real
   `bri.db` machinery — flagging it as un-costed, not blocking.

## What I did NOT prove

- **Embedded Postgres provisioning** — used Docker; the "zero install"
  UX is unverified (owner call 1).
- **Per-test transaction rollback fixtures** — design only.
- **Prepared-statement caching / pool-under-concurrency** — the driver
  numbers are single-threaded warm loops; I did not measure pgx's
  statement cache or behavior under a saturated pool.
- **Absolute latency** — all reads are localhost; a real network would
  shift the 1-row ratio (pgx's protocol edge widens or narrows with RTT).
- **`db/tx`, `cast`, `insert!` as running code** — the three API shapes
  are illustrative real Clojure against a `bri.db` that does not exist;
  only the raw pgx path, the marshaller, migrations, and liveness ran.
- **NOTIFY/LISTEN, JSON/array/enum column round-tripping through the
  names doctrine** — only scalar columns (bigint/text/timestamptz) were
  marshalled.
