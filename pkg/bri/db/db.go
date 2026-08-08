// Package db is the ISOLATED Go half of cljg.data.cast — the pure-Go SQLite/pgx
// host shims for the opt-in data layer (ADR 0072, realizing ADR 0041 §4
// Data / ADR 0057 SQLite-default + ADR 0058 Postgres-via-pgx). It is a
// SEPARATE package from pkg/bri on purpose (ADR 0076): the SQLite + pgx
// drivers are a heavy dependency (~7 MB) that must NOT link into a bri
// binary that never touches a database. pkg/bri never imports this
// package; only pkg/briloader (the interpreter / REPL path, which already
// links the whole interpreter) and the generated pkg/briaot/cljgdatacast
// sub-package (blank-imported into a user binary ONLY when the app requires
// cljg.data.cast) do — so the linker keeps the drivers exactly when, and only when,
// an app uses the database.
//
// The Clojure half is core/cljg/data_cast.cljg (ns cljg.data.cast); this file interns the
// private `-db-*` primitives it leans on, driving database/sql over two
// PURE-GO drivers so a compiled bri app still links CGO_ENABLED=0:
//
//   - modernc.org/sqlite (registered as driver "sqlite") — the
//     zero-install default, NOT cgo mattn/go-sqlite3 (ADR 0057);
//   - github.com/jackc/pgx/v5/stdlib (driver "pgx") — production
//     Postgres (ADR 0058), also pure Go.
//
// Like the rest of bri's Go half this package must NOT import pkg/eval — it
// links into an AOT binary. It registers its shim installer with pkg/bri
// from init() (RegisterInstaller), so bri.InstallShimsInto resolves cljg.data.cast's
// private vars exactly like every other namespace once this package is
// linked. Handles (*sql.DB / *sql.Tx) are held opaquely by the Clojure layer
// inside its {:cljg.data.cast/handle …} map and handed back to these shims; both DB
// and Tx satisfy one `querier`, so one -db-query / -db-exec serves
// connections and transactions alike.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // driver "pgx"
	"github.com/muthuishere/cljgo/pkg/bri"
	"github.com/muthuishere/cljgo/pkg/lang"
	_ "modernc.org/sqlite" // driver "sqlite"
)

// init wires cljg.data.cast's shim installer into pkg/bri's registry. It runs only
// when this package is linked (i.e. the app requires cljg.data.cast, or the
// interpreter is running), so a non-db AOT binary never carries the SQLite +
// pgx drivers (ADR 0076).
func init() { bri.RegisterInstaller("cljg.data.cast", installDBShims) }

// querier is the common surface of *sql.DB and *sql.Tx that cljg.data.cast uses;
// a db handle and a tx handle drive the identical read/write verbs.
type querier interface {
	Query(query string, args ...any) (*sql.Rows, error)
	Exec(query string, args ...any) (sql.Result, error)
}

// dbHandle wraps a pool; txHandle wraps an in-flight transaction. driver
// ("sqlite" | "pgx") selects placeholder style. Both are opaque to the
// Clojure layer, which stores them under :cljg.data.cast/handle and passes them
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
	panic(fmt.Errorf("cljg.data.cast: not a db/tx handle: %s", lang.PrintString(v)))
}

// installDBShims interns cljg.data.cast's private Go primitives (ADR 0072). It is
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
	// The optional 4th arg is the PUBLIC cljg.data.cast fn the call came from
	// (query/one/one!/exec!/insert!/update!/delete!). It exists purely so the
	// params guard in driverArgs can name the fn the user actually wrote
	// instead of the private shim; omitting it keeps the historical 3-arg
	// contract. The optional 5th arg is a vector of per-param LABELS — the
	// row-map verbs (insert!/update!/delete!) build their params themselves
	// out of a map, so a bad value there is a column value, not a varargs
	// param, and the label ("column :a of the row map") is what the guard
	// names. No labels ⇒ the params came straight from the user as varargs.
	def("-db-query", func(args ...any) any {
		if len(args) < 3 || len(args) > 5 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: -db-query (expects 3: [handle sql params], 4: [handle sql params verb] or 5: [handle sql params verb labels])", len(args)))
		}
		q, driver := handleOf(args[0])
		return dbQuery(q, driver, asString(args[1]), args[2], siteOf(args, "query"))
	})
	def("-db-exec", func(args ...any) any {
		if len(args) < 3 || len(args) > 5 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: -db-exec (expects 3: [handle sql params], 4: [handle sql params verb] or 5: [handle sql params verb labels])", len(args)))
		}
		q, driver := handleOf(args[0])
		return dbExec(q, driver, asString(args[1]), args[2], siteOf(args, "exec!"))
	})
	def("-db-begin", func(args ...any) any {
		h, ok := one("-db-begin", args).(*dbHandle)
		if !ok {
			panic(fmt.Errorf("cljg.data.cast: -db-begin needs a connection handle (transactions do not nest into new transactions)"))
		}
		tx, err := h.db.Begin()
		if err != nil {
			panic(fmt.Errorf("cljg.data.cast: begin: %w", err))
		}
		return &txHandle{tx: tx, driver: h.driver}
	})
	def("-db-commit", func(args ...any) any {
		if h, ok := one("-db-commit", args).(*txHandle); ok {
			if err := h.tx.Commit(); err != nil {
				panic(fmt.Errorf("cljg.data.cast: commit: %w", err))
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

// paramSite describes WHERE the params of one -db-query/-db-exec call came
// from, so a bad param is diagnosed as the call the USER wrote. verb is the
// public cljg.data.cast fn; labels (when present, one per param) name the
// row-map slot each param was taken from — the row-map verbs assemble the
// params themselves, so "you passed a collection as a varargs param" is
// simply false on that path.
type paramSite struct {
	verb   string
	labels []string
}

// label returns the row-map label of the i-th (1-based) param, or "" when the
// params were the caller's own varargs.
func (s paramSite) label(i int) string {
	if i-1 < 0 || i-1 >= len(s.labels) {
		return ""
	}
	return s.labels[i-1]
}

// siteOf reads the optional trailing verb + labels arguments of
// -db-query/-db-exec, falling back to def when the caller used the 3-arg form.
func siteOf(args []any, def string) paramSite {
	site := paramSite{verb: def}
	if len(args) >= 4 {
		if s, ok := args[3].(string); ok && s != "" {
			site.verb = s
		}
	}
	if len(args) >= 5 {
		for s := lang.Seq(args[4]); s != nil; s = lang.Next(s) {
			l, _ := lang.First(s).(string)
			site.labels = append(site.labels, l)
		}
	}
	return site
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
		panic(fmt.Errorf("cljg.data.cast: unknown driver %q (want :sqlite or :postgres)", driver))
	}
	db, err := sql.Open(sqlDriver, dsn)
	if err != nil {
		panic(fmt.Errorf("cljg.data.cast: open %s: %w", driver, err))
	}
	// An in-memory SQLite database lives inside ONE connection: a pool that
	// opens a second connection would see a fresh, empty database. Cap it at
	// one so the handle is a stable sandbox (writes persist, isolated per
	// connect) — exactly the ADR 0072 per-test model.
	if driver == "sqlite" && strings.Contains(dsn, ":memory:") {
		db.SetMaxOpenConns(1)
	}
	if err := db.Ping(); err != nil {
		panic(fmt.Errorf("cljg.data.cast: cannot reach the database (%s): %w", driver, err))
	}
	return &dbHandle{db: db, driver: driver}
}

// dbQuery runs a parametrized SELECT and returns a Clojure vector of maps
// (snake_case columns → kebab-case keyword keys).
func dbQuery(q querier, driver, query string, paramsColl any, site paramSite) any {
	rows, err := q.Query(rewritePlaceholders(query, driver), driverArgs(site, paramsColl)...)
	if err != nil {
		panic(fmt.Errorf("cljg.data.cast: query: %w", err))
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		panic(fmt.Errorf("cljg.data.cast: columns: %w", err))
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
			panic(fmt.Errorf("cljg.data.cast: scan: %w", err))
		}
		kvs := make([]any, 0, len(cols)*2)
		for i, cell := range cells {
			kvs = append(kvs, keys[i], goToClojure(cell))
		}
		out = append(out, lang.NewMap(kvs...))
	}
	if err := rows.Err(); err != nil {
		panic(fmt.Errorf("cljg.data.cast: rows: %w", err))
	}
	return lang.NewVectorOwning(out)
}

// dbExec runs a parametrized write and returns {:rows-affected n
// :last-insert-id id} (last-insert-id is nil where the driver has none).
func dbExec(q querier, driver, query string, paramsColl any, site paramSite) any {
	res, err := q.Exec(rewritePlaceholders(query, driver), driverArgs(site, paramsColl)...)
	if err != nil {
		panic(fmt.Errorf("cljg.data.cast: exec: %w", err))
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
			panic(fmt.Errorf("cljg.data.cast: reading migration %s: %w", name, err))
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
//
// site is the cljg.data.cast call the params came from, so the guard below can
// name the fn the USER wrote and diagnose the shape THAT fn takes. A SQL param
// is always a scalar, but a collection reaching here means one of two very
// different mistakes:
//
//   - varargs framing (query/one/one!/exec!, no labels): the caller bundled the
//     params into one collection instead of passing them as the varargs the API
//     takes — the natural first guess, and until 2026-07-28 it died opaquely
//     inside database/sql as `converting argument $1 type: unsupported type
//     lang.Vector, a struct` (docs/known-issues-2026-07-28.md §9). G5008, and
//     `apply` is the fix.
//   - row-map framing (insert!/update!/delete!, labelled): the caller passed a
//     legal call shape and put a collection INSIDE the row/set/where map. There
//     is no varargs param to spread and `apply` fixes nothing, so this is its
//     own diagnosis (G5009), naming the column and carrying no Fix — an absent
//     Fix beats a wrong one.
func driverArgs(site paramSite, coll any) []any {
	var args []any
	i := 0
	for s := lang.Seq(coll); s != nil; s = lang.Next(s) {
		i++
		v := lang.First(s)
		if kind := collKind(v); kind != "" {
			if label := site.label(i); label != "" {
				panic(&lang.CodedError{
					Code: "G5009",
					Msg: fmt.Sprintf("cljg.data.cast/%s: %s is a %s — a column value must be a scalar SQL param "+
						"(expects a string, number, boolean, keyword or nil, found a %s)",
						site.verb, label, kind, kind),
				})
			}
			panic(&lang.CodedError{
				Code: "G5008",
				Msg: fmt.Sprintf("cljg.data.cast/%s: param %d is a %s — SQL params are varargs, not a collection "+
					"(expects [db sql & params], found a %s passed as one param); "+
					"spread it with (apply %s db sql params)", site.verb, i, kind, kind, site.verb),
			})
		}
		args = append(args, clojureToDriver(v))
	}
	return args
}

// collKind names v's collection kind for the params guard, or "" when v is a
// legal scalar param. Strings are Seqable on the JVM but are the single most
// common param type, so only real Clojure collections are rejected.
func collKind(v any) string {
	switch v.(type) {
	case string, []byte, nil:
		return ""
	case lang.IPersistentVector:
		return "vector"
	case lang.IPersistentMap:
		return "map"
	case lang.IPersistentSet:
		return "set"
	case lang.ISeq:
		return "sequence"
	case lang.IPersistentCollection:
		return "collection"
	default:
		return ""
	}
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

// --- shared shim helpers -----------------------------------------------------
// Duplicated (not exported from pkg/bri) so this isolated package stays
// self-contained and pkg/bri keeps zero edges to the drivers — the same
// choice pkg/bri/otel made (ADR 0076).

// one asserts a single argument and returns it (a 1-arity shim guard).
func one(name string, args []any) any {
	if len(args) != 1 {
		panic(fmt.Errorf("wrong number of args (%d) passed to: %s", len(args), name))
	}
	return args[0]
}

// asString unwraps a Clojure string argument.
func asString(v any) string {
	s, ok := v.(string)
	if !ok {
		panic(fmt.Errorf("expected a string, got: %s", lang.PrintString(v)))
	}
	return s
}

// keywordName is a keyword's name without the leading colon.
func keywordName(k lang.Keyword) string { return strings.TrimPrefix(k.String(), ":") }

// getenvShim backs cljg.data.cast's (-getenv name) — os.LookupEnv, nil when unset.
func getenvShim(args ...any) any {
	v, ok := os.LookupEnv(asString(one("-getenv", args)))
	if !ok {
		return nil
	}
	return v
}
