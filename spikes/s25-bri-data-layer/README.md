# S25 — bri's data layer (ADR 0041 T2)

ADR 0041 sketches T2 in two sentences: "no ORM. pgx behind `bri.db`
(query/one/insert/update/delete/tx; plain maps out; SQL strings are THE
blessed form), a names doctrine, Ecto-style casts returning Result,
SQL-file migrations." That is a *position*, asserted, never measured.
This spike tests it.

Owner's bar (ADR 0041 context): **Spring Boot capability, Rails/Elixir
ergonomics, library style — you call it, it never calls you.**

## The one question

What does a Clojure-first data layer on Go actually look like — and is
"pgx + SQL strings + plain maps" the right blessed form, or does a
HugSQL-style SQL-file mapping or a Datomic-ish entity API buy enough to
justify itself?

## Exit criteria (written before any code)

1. **Driver choice measured, not assumed.** pgx v5 (native protocol)
   vs `database/sql`+stdlib wrapper, same Postgres, same query: ns/op
   and allocs/op for a single-row read and a 100-row read. ADR 0041's
   "pgx behind bri.db" either survives a number or changes.
2. **The names doctrine costed.** snake_case column → kebab-case
   keyword maps, both directions, round-tripped; the marshalling
   overhead measured against a raw positional scan. If turning rows
   into idiomatic Clojure maps costs more than the query, that is a
   finding.
3. **REPL-liveness demonstrated against a real database.** A query
   held in a cljgo VAR, re-`def`ed through the real evaluator while a
   pool is live, and the NEXT call runs the NEW SQL — no reconnect, no
   restart. This is the claim bri sells; it must be shown, not asserted.
4. **Interpreted-mode honesty.** pgx is not in the interpreter's seed
   registry. Establish, with a measurement, what an interpreted app
   actually pays to reach it (design/05 self-rebuild, or a Go shim like
   `pkg/bri`), and say which one T2 should ship.
5. **Three API shapes written as REAL code** against one schema —
   (a) inline SQL strings, (b) HugSQL-style `.sql` files with named
   params, (c) a Datomic-ish entity/pull API — compared on: lines for
   the golden-page CRUD, what stays live at the REPL, what the error
   surface looks like when the SQL is wrong.
6. **Migrations exercised end to end**: SQL files, UTC-timestamped,
   ledger table, idempotent re-run, additive-only doctrine. Smallest
   implementation that does it, LOC reported.
7. **The no-ORM claim made concrete**: the same feature counted in
   concepts against Spring Boot JPA (annotations, classes, config
   files, lifecycle objects) — so "no ORM ceremony" is a number.
8. **VERDICT.md** takes a position per question, states explicitly
   what was NOT proven, and routes genuine owner calls to the owner
   with options + a recommendation rather than a silent pick.

## Non-goals

Not building `bri.db`. Not touching `pkg/` or `core/`. No conformance
tests, no OpenSpec deltas — those come after an ADR, per ADR 0027.
Probe code is throwaway and never merges into `pkg/`.

## Layout

- `probe/` — standalone Go module (own `go.mod`, replace-directive to
  the repo root; excluded from root `./...` by construction) carrying
  the pgx/database-sql benchmarks, the names-doctrine marshaller, the
  migration runner, and the embedded-evaluator liveness proof.
- `shapes/` — the three candidate API shapes as real Clojure.
- `run.sh` — brings up Postgres in Docker, runs everything, prints
  PASS/FAIL per criterion.
- `VERDICT.md` — the positions + the measured evidence.
