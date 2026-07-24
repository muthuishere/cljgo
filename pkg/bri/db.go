// db.go — the pure-Go host shims for bri.db (ADR 0072, realizing ADR
// 0041 §4 Data / ADR 0057 SQLite-default + ADR 0058 Postgres-via-pgx).
//
// The Clojure half is core/bri/db.cljg (ns bri.db); this file interns
// the private `-db-*` primitives it leans on, driving database/sql over
// two PURE-GO drivers so a compiled bri app still links CGO_ENABLED=0:
//
//   - modernc.org/sqlite (registered as driver "sqlite") — the
//     zero-install default, NOT cgo mattn/go-sqlite3 (ADR 0057);
//   - github.com/jackc/pgx/v5/stdlib (driver "pgx") — production
//     Postgres (ADR 0058), also pure Go.
//
// Like the rest of pkg/bri this file must NOT import pkg/eval — it links
// into an AOT binary. Handles (*sql.DB / *sql.Tx) are held opaquely by
// the Clojure layer inside its {:bri.db/handle …} map and handed back to
// these shims; both DB and Tx satisfy one `querier`, so one -db-query /
// -db-exec serves connections and transactions alike.
package bri

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // driver "pgx"
	"github.com/muthuishere/cljgo/pkg/lang"
	_ "modernc.org/sqlite" // driver "sqlite"
)

// querier is the common surface of *sql.DB and *sql.Tx that bri.db uses;
// a db handle and a tx handle drive the identical read/write verbs.
type querier interface {
	Query(query string, args ...any) (*sql.Rows, error)
	Exec(query string, args ...any) (sql.Result, error)
}

// dbHandle wraps a pool; txHandle wraps an in-flight transaction. driver
// ("sqlite" | "pgx") selects placeholder style. Both are opaque to the
// Clojure layer, which stores them under :bri.db/handle and passes them
// straight back.
type dbHandle struct {
	db     *sql.DB
	driver string
}

type txHandle struct {
	tx     *sql.Tx
	driver string
}

// handleOf resolves the opaque handle argument to its querier + driver.
func handleOf(v any) (querier, string) {
	switch h := v.(type) {
	case *dbHandle:
		return h.db, h.driver
	case *txHandle:
		return h.tx, h.driver
	}
	panic(fmt.Errorf("bri.db: not a db/tx handle: %s", lang.PrintString(v)))
}

// installDBShims interns bri.db's private Go primitives (ADR 0072). It is
// referenced by pkg/bri.Specs() and by pkg/briaot's generated loader, so
// these run identically interpreted and compiled.
func installDBShims(def func(name string, fn func(args ...any) any)) {
	def("-db-open", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: -db-open", len(args)))
		}
		return dbOpen(asString(args[0]), asString(args[1]))
	})
	def("-db-close", func(args ...any) any {
		if h, ok := one("-db-close", args).(*dbHandle); ok {
			_ = h.db.Close()
		}
		return nil
	})
	def("-db-query", func(args ...any) any {
		if len(args) != 3 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: -db-query", len(args)))
		}
		q, driver := handleOf(args[0])
		return dbQuery(q, driver, asString(args[1]), args[2])
	})
	def("-db-exec", func(args ...any) any {
		if len(args) != 3 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: -db-exec", len(args)))
		}
		q, driver := handleOf(args[0])
		return dbExec(q, driver, asString(args[1]), args[2])
	})
	def("-db-begin", func(args ...any) any {
		h, ok := one("-db-begin", args).(*dbHandle)
		if !ok {
			panic(fmt.Errorf("bri.db: -db-begin needs a connection handle (transactions do not nest into new transactions)"))
		}
		tx, err := h.db.Begin()
		if err != nil {
			panic(fmt.Errorf("bri.db: begin: %w", err))
		}
		return &txHandle{tx: tx, driver: h.driver}
	})
	def("-db-commit", func(args ...any) any {
		if h, ok := one("-db-commit", args).(*txHandle); ok {
			if err := h.tx.Commit(); err != nil {
				panic(fmt.Errorf("bri.db: commit: %w", err))
			}
		}
		return nil
	})
	def("-db-rollback", func(args ...any) any {
		if h, ok := one("-db-rollback", args).(*txHandle); ok {
			_ = h.tx.Rollback()
		}
		return nil
	})
	def("-db-migration-files", func(args ...any) any {
		return migrationFiles(asString(one("-db-migration-files", args)))
	})
	def("-db-now", func(args ...any) any { return time.Now().UTC().Format(time.RFC3339Nano) })
	def("-getenv", getenvShim)
}

// dbOpen resolves a driver name + DSN into a live pool. SQLite gets WAL +
// busy-timeout for the concurrent-writer story (ADR 0057 evidence);
// ":memory:" stays a private in-memory database (the test sandbox).
func dbOpen(driver, dsn string) any {
	sqlDriver := driver
	switch driver {
	case "sqlite":
		sqlDriver = "sqlite"
		if dsn != ":memory:" && !strings.Contains(dsn, ":memory:") {
			// Zero-install means the default `.dev/app.db` (ADR 0057) just
			// works: create the parent directory if the path names one, so a
			// fresh checkout need not mkdir before the first connect.
			if parent := filepath.Dir(dsn); parent != "." && parent != "" {
				_ = os.MkdirAll(parent, 0o755)
			}
			if !strings.Contains(dsn, "?") {
				dsn += "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
			}
		}
	case "pgx", "postgres":
		sqlDriver = "pgx"
	default:
		panic(fmt.Errorf("bri.db: unknown driver %q (want :sqlite or :postgres)", driver))
	}
	db, err := sql.Open(sqlDriver, dsn)
	if err != nil {
		panic(fmt.Errorf("bri.db: open %s: %w", driver, err))
	}
	// An in-memory SQLite database lives inside ONE connection: a pool that
	// opens a second connection would see a fresh, empty database. Cap it at
	// one so the handle is a stable sandbox (writes persist, isolated per
	// connect) — exactly the ADR 0072 per-test model.
	if driver == "sqlite" && strings.Contains(dsn, ":memory:") {
		db.SetMaxOpenConns(1)
	}
	if err := db.Ping(); err != nil {
		panic(fmt.Errorf("bri.db: cannot reach the database (%s): %w", driver, err))
	}
	return &dbHandle{db: db, driver: driver}
}

// dbQuery runs a parametrized SELECT and returns a Clojure vector of maps
// (snake_case columns → kebab-case keyword keys).
func dbQuery(q querier, driver, query string, paramsColl any) any {
	rows, err := q.Query(rewritePlaceholders(query, driver), driverArgs(paramsColl)...)
	if err != nil {
		panic(fmt.Errorf("bri.db: query: %w", err))
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		panic(fmt.Errorf("bri.db: columns: %w", err))
	}
	keys := make([]lang.Keyword, len(cols))
	for i, c := range cols {
		keys[i] = lang.NewKeyword(snakeToKebab(c))
	}
	var out []any
	for rows.Next() {
		cells := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range cells {
			ptrs[i] = &cells[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			panic(fmt.Errorf("bri.db: scan: %w", err))
		}
		kvs := make([]any, 0, len(cols)*2)
		for i, cell := range cells {
			kvs = append(kvs, keys[i], goToClojure(cell))
		}
		out = append(out, lang.NewMap(kvs...))
	}
	if err := rows.Err(); err != nil {
		panic(fmt.Errorf("bri.db: rows: %w", err))
	}
	return lang.NewVectorOwning(out)
}

// dbExec runs a parametrized write and returns {:rows-affected n
// :last-insert-id id} (last-insert-id is nil where the driver has none).
func dbExec(q querier, driver, query string, paramsColl any) any {
	res, err := q.Exec(rewritePlaceholders(query, driver), driverArgs(paramsColl)...)
	if err != nil {
		panic(fmt.Errorf("bri.db: exec: %w", err))
	}
	var affected any
	if n, err := res.RowsAffected(); err == nil {
		affected = n
	}
	var lastID any
	if id, err := res.LastInsertId(); err == nil {
		lastID = id
	}
	return lang.NewMap(
		lang.NewKeyword("rows-affected"), affected,
		lang.NewKeyword("last-insert-id"), lastID,
	)
}

// migrationFiles reads dir and returns a Clojure vector of
// {:version :name :sql} maps sorted ascending by filename (the UTC
// timestamp prefix orders lexically). A missing dir yields an empty
// vector (nothing to migrate).
func migrationFiles(dir string) any {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return lang.NewVectorOwning(nil)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	out := make([]any, 0, len(names))
	for _, name := range names {
		sqlBytes, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			panic(fmt.Errorf("bri.db: reading migration %s: %w", name, err))
		}
		version := strings.TrimSuffix(name, ".sql")
		if i := strings.Index(version, "_"); i > 0 {
			version = version[:i]
		}
		out = append(out, lang.NewMap(
			lang.NewKeyword("version"), version,
			lang.NewKeyword("name"), name,
			lang.NewKeyword("sql"), string(sqlBytes),
		))
	}
	return lang.NewVectorOwning(out)
}

// driverArgs converts a Clojure params collection (a vector) into
// database/sql args. Keywords pass as their name; the tagged/plain
// scalars pass straight through.
func driverArgs(coll any) []any {
	var args []any
	for s := lang.Seq(coll); s != nil; s = lang.Next(s) {
		args = append(args, clojureToDriver(lang.First(s)))
	}
	return args
}

func clojureToDriver(v any) any {
	switch t := v.(type) {
	case nil, bool, string, int64, int, float64:
		return t
	case lang.Keyword:
		return keywordName(t)
	default:
		return v
	}
}

// goToClojure maps a scanned SQL cell to cljgo data: []byte→string,
// time.Time→RFC3339 string (JVM-free + identical across modes), the rest
// (int64/float64/bool/string/nil) straight through.
func goToClojure(v any) any {
	switch t := v.(type) {
	case []byte:
		return string(t)
	case time.Time:
		return t.UTC().Format(time.RFC3339Nano)
	default:
		return v
	}
}

// rewritePlaceholders turns `?` placeholders into `$1,$2,…` for Postgres
// (pgx), leaving SQLite's `?` untouched. Quote-aware: a `?` inside a
// single-quoted string literal is not a placeholder. SQL dialect is NOT
// rewritten — only the placeholder token (ADR 0057 seam).
func rewritePlaceholders(query, driver string) string {
	if driver != "pgx" && driver != "postgres" {
		return query
	}
	var b strings.Builder
	inStr := false
	n := 0
	for i := 0; i < len(query); i++ {
		c := query[i]
		switch {
		case c == '\'':
			inStr = !inStr
			b.WriteByte(c)
		case c == '?' && !inStr:
			n++
			b.WriteByte('$')
			b.WriteString(fmt.Sprintf("%d", n))
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

func snakeToKebab(s string) string { return strings.ReplaceAll(s, "_", "-") }
