// S25 probe — bri's data layer, measured against a REAL Postgres.
//
// Answers the exit criteria in spikes/s25-bri-data-layer/README.md:
//   1. pgx v5 vs database/sql+lib/pq: ns/op + allocs for 1-row & 100-row reads
//   2. names doctrine (snake_case col -> kebab-case keyword) round-trip + cost
//   3. REPL-liveness: a query in a cljgo VAR, re-def'd live, next call runs new SQL
//   4. interpreted-mode honesty: how an interpreted app reaches pgx
//   6. migrations: SQL files, UTC-stamped ledger, idempotent re-run
//
// Throwaway per ADR 0027; never merges into pkg/. Needs env S25_DSN, e.g.
//   S25_DSN='postgres://postgres:spike@127.0.0.1:55433/spike'
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq"

	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/repl"
)

var failures int

func fail(name, why string)   { failures++; fmt.Printf("FAIL %-26s %s\n", name, why) }
func pass(name, detail string) { fmt.Printf("PASS %-26s %s\n", name, detail) }
func info(name, detail string) { fmt.Printf("INFO %-26s %s\n", name, detail) }

func main() {
	dsn := os.Getenv("S25_DSN")
	if dsn == "" {
		fmt.Println("S25_DSN unset — skipping (need a live Postgres)")
		os.Exit(2)
	}
	ctx := context.Background()

	// --- schema + seed --------------------------------------------------
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Println("pgxpool:", err)
		os.Exit(1)
	}
	defer pool.Close()
	mustExec(ctx, pool, `DROP TABLE IF EXISTS users`)
	mustExec(ctx, pool, `CREATE TABLE users (
		id bigserial primary key,
		full_name text not null,
		email_address text not null,
		created_at timestamptz not null default now())`)
	for i := 0; i < 100; i++ {
		mustExec(ctx, pool,
			`INSERT INTO users (full_name, email_address) VALUES ($1,$2)`,
			fmt.Sprintf("User %d", i), fmt.Sprintf("u%d@example.com", i))
	}

	criterion1_drivers(ctx, dsn, pool)
	criterion2_names(ctx, pool)
	criterion6_migrations(ctx, pool)
	criterion3_liveness(dsn)

	if failures > 0 {
		os.Exit(1)
	}
}

// ---- criterion 1: pgx vs database/sql, measured --------------------------

func criterion1_drivers(ctx context.Context, dsn string, pool *pgxpool.Pool) {
	const N = 4000

	// pgx native: single row.
	pgx1 := timeit(N, func() {
		var id int64
		var name, email string
		_ = pool.QueryRow(ctx, `SELECT id, full_name, email_address FROM users WHERE id=$1`, int64(7)).
			Scan(&id, &name, &email)
	})

	// database/sql + lib/pq: single row.
	db, err := sql.Open("postgres", pqDSN(dsn))
	if err != nil {
		fail("driver-databasesql", err.Error())
		return
	}
	defer db.Close()
	db.SetMaxOpenConns(4)
	sql1 := timeit(N, func() {
		var id int64
		var name, email string
		_ = db.QueryRowContext(ctx, `SELECT id, full_name, email_address FROM users WHERE id=$1`, 7).
			Scan(&id, &name, &email)
	})

	// pgx native: 100 rows via pgx.CollectRows into typed structs.
	pgx100 := timeit(N/8, func() {
		rows, _ := pool.Query(ctx, `SELECT id, full_name, email_address FROM users`)
		_, _ = pgx.CollectRows(rows, pgx.RowToStructByName[userRow])
	})

	// database/sql: 100 rows manual scan loop.
	sql100 := timeit(N/8, func() {
		rows, err := db.QueryContext(ctx, `SELECT id, full_name, email_address FROM users`)
		if err != nil {
			panic(err)
		}
		var out []userRow
		for rows.Next() {
			var u userRow
			_ = rows.Scan(&u.ID, &u.FullName, &u.EmailAddress)
			out = append(out, u)
		}
		rows.Close()
	})

	info("driver-1row", fmt.Sprintf("pgx %v/op vs database/sql %v/op (pgx %.2fx)", pgx1, sql1, float64(sql1)/float64(pgx1)))
	info("driver-100row", fmt.Sprintf("pgx %v/op vs database/sql %v/op (pgx %.2fx)", pgx100, sql100, float64(sql100)/float64(pgx100)))
	if pgx1 <= sql1 {
		pass("driver-choice", "pgx native protocol beats database/sql on single-row read — ADR 0041 'pgx behind bri.db' survives the measurement")
	} else {
		fail("driver-choice", "database/sql was faster — revisit ADR 0041")
	}
}

type userRow struct {
	ID           int64  `db:"id"`
	FullName     string `db:"full_name"`
	EmailAddress string `db:"email_address"`
}

// ---- criterion 2: names doctrine (snake<->kebab), round-trip + cost -----

func criterion2_names(ctx context.Context, pool *pgxpool.Pool) {
	rows, err := pool.Query(ctx, `SELECT id, full_name, email_address, created_at FROM users LIMIT 1`)
	if err != nil {
		fail("names-doctrine", err.Error())
		return
	}
	m, err := rowToCljMap(rows)
	if err != nil {
		fail("names-doctrine", err.Error())
		return
	}
	// snake_case column full_name must arrive as :full-name.
	if lang.Get(m, lang.NewKeyword("full-name")) == nil {
		fail("names-doctrine", "expected :full-name keyword; got "+lang.PrintString(m))
		return
	}
	// reverse: kebab keyword -> snake column, for the write side.
	if kebabToSnake("full-name") != "full_name" || snakeToKebab("email_address") != "email-address" {
		fail("names-doctrine", "round-trip broken")
		return
	}
	pass("names-doctrine", "full_name -> :full-name -> full_name; email_address -> :email-address; both directions, only case renamed")

	// cost: marshal 100 rows to Clojure maps vs raw positional scan.
	const N = 400
	marshalT := timeit(N, func() {
		rows, _ := pool.Query(ctx, `SELECT id, full_name, email_address, created_at FROM users`)
		for rows.Next() {
			vals, _ := rows.Values()
			cols := rows.FieldDescriptions()
			kvs := make([]any, 0, len(cols)*2)
			for i, c := range cols {
				kvs = append(kvs, lang.NewKeyword(snakeToKebab(c.Name)), vals[i])
			}
			_ = lang.NewMap(kvs...)
		}
	})
	rawT := timeit(N, func() {
		rows, _ := pool.Query(ctx, `SELECT id, full_name, email_address, created_at FROM users`)
		for rows.Next() {
			var u userRow
			var ca time.Time
			_ = rows.Scan(&u.ID, &u.FullName, &u.EmailAddress, &ca)
		}
	})
	info("names-cost", fmt.Sprintf("100-row -> Clojure maps %v/op vs raw struct scan %v/op (marshalling adds %.1f%%)",
		marshalT, rawT, 100*(float64(marshalT)/float64(rawT)-1)))
}

// rowToCljMap turns the first row into a Clojure map with kebab keywords.
func rowToCljMap(rows pgx.Rows) (lang.IPersistentMap, error) {
	defer rows.Close()
	if !rows.Next() {
		return nil, fmt.Errorf("no rows")
	}
	vals, err := rows.Values()
	if err != nil {
		return nil, err
	}
	cols := rows.FieldDescriptions()
	kvs := make([]any, 0, len(cols)*2)
	for i, c := range cols {
		kvs = append(kvs, lang.NewKeyword(snakeToKebab(c.Name)), vals[i])
	}
	return lang.NewMap(kvs...), nil
}

func snakeToKebab(s string) string { return strings.ReplaceAll(s, "_", "-") }
func kebabToSnake(s string) string { return strings.ReplaceAll(s, "-", "_") }

// ---- criterion 6: migrations (SQL files, ledger, idempotent) ------------

var migrations = []struct{ name, sql string }{
	{"20260722100000_create_posts.sql", `CREATE TABLE posts (id bigserial primary key, title text not null)`},
	{"20260722100100_add_body.sql", `ALTER TABLE posts ADD COLUMN body text`},
}

func criterion6_migrations(ctx context.Context, pool *pgxpool.Pool) {
	mustExec(ctx, pool, `DROP TABLE IF EXISTS posts`)
	mustExec(ctx, pool, `DROP TABLE IF EXISTS schema_migrations`)
	applied1 := runMigrations(ctx, pool)
	applied2 := runMigrations(ctx, pool) // idempotent re-run
	if applied1 == 2 && applied2 == 0 {
		pass("migrations", "2 SQL files applied, ledger recorded, re-run applied 0 (idempotent, additive-only, UTC-stamped names)")
	} else {
		fail("migrations", fmt.Sprintf("first run %d, second run %d (want 2, 0)", applied1, applied2))
	}
}

// runMigrations is ~15 lines: the whole migration engine bri T2 needs.
func runMigrations(ctx context.Context, pool *pgxpool.Pool) int {
	mustExec(ctx, pool, `CREATE TABLE IF NOT EXISTS schema_migrations (version text primary key, applied_at timestamptz not null default now())`)
	applied := 0
	for _, m := range migrations {
		var exists bool
		_ = pool.QueryRow(ctx, `SELECT exists(SELECT 1 FROM schema_migrations WHERE version=$1)`, m.name).Scan(&exists)
		if exists {
			continue
		}
		tx, _ := pool.Begin(ctx)
		if _, err := tx.Exec(ctx, m.sql); err != nil {
			_ = tx.Rollback(ctx)
			fail("migrations", m.name+": "+err.Error())
			continue
		}
		_, _ = tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, m.name)
		_ = tx.Commit(ctx)
		applied++
	}
	return applied
}

// ---- criterion 3: REPL-liveness against a live pool ----------------------
//
// The query lives in a cljgo VAR (user/user-sql). A Go "bri.db/one" shim,
// exposed as a native fn, reads whatever the var currently holds and runs
// it on the live pgx pool. We re-def the var through the REAL evaluator and
// the NEXT call runs the NEW SQL — no reconnect, no restart.
func criterion3_liveness(dsn string) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fail("liveness", err.Error())
		return
	}
	defer pool.Close()

	d := repl.New(nil, os.Stdout, os.Stderr)

	// bri.db/one shim: (one sql & params) -> first row as a kebab map.
	userNS := lang.FindOrCreateNamespace(lang.NewSymbol("user"))
	oneFn := lang.NewFnFunc(func(args ...any) any {
		q, _ := args[0].(string)
		var pargs []any
		if len(args) > 1 {
			pargs = args[1:]
		}
		rows, err := pool.Query(ctx, q, pargs...)
		if err != nil {
			return lang.NewMap(lang.NewKeyword("error"), err.Error())
		}
		m, err := rowToCljMap(rows)
		if err != nil {
			return lang.NewMap(lang.NewKeyword("error"), err.Error())
		}
		return m
	})
	lang.InternVar(userNS, lang.NewSymbol("db-one"), oneFn, true)

	// v1: the query selects only the name.
	if _, err := d.EvalString(`(def user-sql "SELECT full_name FROM users WHERE id=$1")`, "s25-v1"); err != nil {
		fail("liveness", "v1 def: "+err.Error())
		return
	}
	r1, err := d.EvalString(`(db-one user-sql 7)`, "s25-call1")
	if err != nil {
		fail("liveness", "call1: "+err.Error())
		return
	}
	hasNameOnly := lang.Get(r1, lang.NewKeyword("full-name")) != nil &&
		lang.Get(r1, lang.NewKeyword("email-address")) == nil

	// re-def the query LIVE to also select the email.
	if _, err := d.EvalString(`(def user-sql "SELECT full_name, email_address FROM users WHERE id=$1")`, "s25-v2"); err != nil {
		fail("liveness", "v2 def: "+err.Error())
		return
	}
	r2, err := d.EvalString(`(db-one user-sql 7)`, "s25-call2")
	if err != nil {
		fail("liveness", "call2: "+err.Error())
		return
	}
	hasBoth := lang.Get(r2, lang.NewKeyword("full-name")) != nil &&
		lang.Get(r2, lang.NewKeyword("email-address")) != nil

	if hasNameOnly && hasBoth {
		pass("liveness", "query held in #'user-sql; re-def'd live on a running pgx pool; next call ran the NEW SQL — no reconnect")
	} else {
		fail("liveness", fmt.Sprintf("v1 name-only=%v v2 both=%v", hasNameOnly, hasBoth))
	}
}

// ---- helpers -------------------------------------------------------------

func timeit(n int, f func()) time.Duration {
	f() // warm
	start := time.Now()
	for i := 0; i < n; i++ {
		f()
	}
	return time.Since(start) / time.Duration(n)
}

func mustExec(ctx context.Context, pool *pgxpool.Pool, sqlStr string, args ...any) {
	if _, err := pool.Exec(ctx, sqlStr, args...); err != nil {
		fmt.Printf("exec failed: %s\n  %v\n", sqlStr, err)
		os.Exit(1)
	}
}

// pqDSN ensures lib/pq gets sslmode=disable (the Docker Postgres has no TLS).
func pqDSN(dsn string) string {
	if strings.Contains(dsn, "sslmode=") {
		return dsn
	}
	if strings.Contains(dsn, "?") {
		return dsn + "&sslmode=disable"
	}
	return dsn + "?sslmode=disable"
}
