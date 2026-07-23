// S37 probe — pure-Go SQLite (modernc.org/sqlite) as cljgo's zero-install
// default DB. Standalone throwaway module; never imported by pkg/.
//
// Run modes (argv[1]):
//
//	bench    — startup+first-query latency, insert/read/sec, bulk tx rows/sec
//	adapter  — the bri.db-shaped adapter over sqlite: file DB + :memory:,
//	           plain-map rows, snake_case<->kebab-case names doctrine
//	wal       — WAL + concurrency probe (N goroutines writing/reading one file)
//	all      — every mode in sequence
//
// The size-delta binaries live in ./sizetest (built by run.sh).
package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

func main() {
	mode := "all"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	switch mode {
	case "bench":
		bench()
	case "adapter":
		adapter()
	case "wal":
		walProbe()
	case "all":
		bench()
		fmt.Println()
		adapter()
		fmt.Println()
		walProbe()
	default:
		fmt.Fprintf(os.Stderr, "unknown mode %q\n", mode)
		os.Exit(2)
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// open returns a *sql.DB for the given DSN with pragmas a server workload
// wants: WAL journal (concurrent readers + one writer), busy_timeout so a
// contended writer waits instead of erroring, foreign_keys on.
func open(dsn string) *sql.DB {
	// modernc registers itself as driver name "sqlite".
	full := dsn + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)"
	db, err := sql.Open("sqlite", full)
	must(err)
	// One writer connection avoids "database is locked" on a single file
	// while still allowing WAL concurrent reads; a real bri would size this.
	return db
}

// ---------------------------------------------------------------------------
// criterion 3 — latency + throughput
// ---------------------------------------------------------------------------

func bench() {
	fmt.Println("== criterion 3: latency + throughput (modernc.org/sqlite, file DB, WAL) ==")

	tmp, err := os.MkdirTemp("", "s37bench")
	must(err)
	defer os.RemoveAll(tmp)
	dbfile := tmp + "/bench.db"

	// --- startup + first-query latency (cold process already paid; this is
	//     sql.Open + first real round-trip through the driver) ---
	t0 := time.Now()
	db := open(dbfile)
	var one int
	must(db.QueryRow("SELECT 1").Scan(&one))
	startup := time.Since(t0)
	fmt.Printf("startup + first query (Open+SELECT 1): %v\n", startup)

	_, err = db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY,
		full_name TEXT NOT NULL,
		email_address TEXT NOT NULL,
		created_at INTEGER NOT NULL)`)
	must(err)

	// --- single-row insert/sec (prepared) ---
	insStmt, err := db.Prepare(`INSERT INTO users (full_name, email_address, created_at) VALUES (?, ?, ?)`)
	must(err)
	const nIns = 20000
	t0 = time.Now()
	for i := 0; i < nIns; i++ {
		_, err := insStmt.Exec(fmt.Sprintf("User %d", i), fmt.Sprintf("u%d@example.com", i), time.Now().UnixMilli())
		must(err)
	}
	insDur := time.Since(t0)
	fmt.Printf("single-row insert (prepared, autocommit): %d rows in %v = %.0f inserts/sec (%v/op)\n",
		nIns, insDur, float64(nIns)/insDur.Seconds(), insDur/nIns)

	// --- single-row read/sec (prepared, point lookup on PK) ---
	selStmt, err := db.Prepare(`SELECT id, full_name, email_address, created_at FROM users WHERE id = ?`)
	must(err)
	const nRead = 50000
	t0 = time.Now()
	for i := 0; i < nRead; i++ {
		id := (i % nIns) + 1
		var uid, created int64
		var name, email string
		must(selStmt.QueryRow(id).Scan(&uid, &name, &email, &created))
	}
	readDur := time.Since(t0)
	fmt.Printf("single-row read  (prepared, PK lookup):   %d rows in %v = %.0f reads/sec (%v/op)\n",
		nRead, readDur, float64(nRead)/readDur.Seconds(), readDur/nRead)

	// --- bulk insert inside ONE transaction (rows/sec) ---
	_, err = db.Exec(`CREATE TABLE bulk (id INTEGER PRIMARY KEY, v TEXT NOT NULL)`)
	must(err)
	const nBulk = 200000
	t0 = time.Now()
	tx, err := db.Begin()
	must(err)
	bstmt, err := tx.Prepare(`INSERT INTO bulk (v) VALUES (?)`)
	must(err)
	for i := 0; i < nBulk; i++ {
		_, err := bstmt.Exec(fmt.Sprintf("row-%d", i))
		must(err)
	}
	must(tx.Commit())
	bulkDur := time.Since(t0)
	fmt.Printf("bulk insert (single tx):                  %d rows in %v = %.0f rows/sec\n",
		nBulk, bulkDur, float64(nBulk)/bulkDur.Seconds())

	// report the on-disk size so the deploy story (criterion 5) has a number
	if fi, err := os.Stat(dbfile); err == nil {
		fmt.Printf("db file on disk after bench: %.1f MB (%s + -wal/-shm alongside)\n",
			float64(fi.Size())/1e6, dbfile)
	}
	must(db.Close())
}

// ---------------------------------------------------------------------------
// criterion 4 — one bri.db-shaped API over sqlite (file + :memory:)
// ---------------------------------------------------------------------------

// DB is the tiny driver-agnostic surface the bri.db verbs sit on. The SAME
// interface is what a pgx-backed prod DB would satisfy — S25 proved the pgx
// side; here we run the sqlite side for real. bri.db (query/one/insert/tx)
// is a thin Clojure layer over exactly these four Go methods.
type DB interface {
	// Query returns every row as a plain map with kebab-case keyword-style
	// string keys (the names doctrine). Positional params.
	Query(sqlText string, args ...any) ([]map[string]any, error)
	// One returns the first row or nil.
	One(sqlText string, args ...any) (map[string]any, error)
	// Insert takes a table + kebab-keyed attrs, snake-cases the columns,
	// runs the insert, returns the new row id.
	Insert(table string, attrs map[string]any) (int64, error)
	// Tx runs fn inside a transaction; rollback on error.
	Tx(fn func(DB) error) error
}

// sqliteDB adapts *sql.DB to the bri.db surface. A pgxDB would adapt a
// *pgxpool.Pool the same way (stubbed here — see pgxStub below).
type sqliteDB struct {
	db *sql.DB
	// tx is non-nil inside Tx(); when set, all ops run on it.
	tx *sql.Tx
}

func (s *sqliteDB) exec() interface {
	Query(string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
	Exec(string, ...any) (sql.Result, error)
} {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

// snakeToKebab: full_name -> full-name (column name -> map key).
func snakeToKebab(s string) string { return strings.ReplaceAll(s, "_", "-") }

// kebabToSnake: full-name -> full_name (map key -> column name).
func kebabToSnake(s string) string { return strings.ReplaceAll(s, "-", "_") }

func (s *sqliteDB) Query(sqlText string, args ...any) ([]map[string]any, error) {
	rows, err := s.exec().Query(sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for rows.Next() {
		cells := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range cells {
			ptrs[i] = &cells[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			v := cells[i]
			// []byte -> string is the one coercion a Clojure caller expects
			// (sqlite hands TEXT back as []byte through database/sql).
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			m[snakeToKebab(c)] = v
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *sqliteDB) One(sqlText string, args ...any) (map[string]any, error) {
	rows, err := s.Query(sqlText, args...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

func (s *sqliteDB) Insert(table string, attrs map[string]any) (int64, error) {
	cols := make([]string, 0, len(attrs))
	ph := make([]string, 0, len(attrs))
	vals := make([]any, 0, len(attrs))
	// deterministic column order isn't required for correctness; keep simple.
	for k, v := range attrs {
		cols = append(cols, kebabToSnake(k))
		ph = append(ph, "?")
		vals = append(vals, v)
	}
	q := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		kebabToSnake(table), strings.Join(cols, ", "), strings.Join(ph, ", "))
	res, err := s.exec().Exec(q, vals...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *sqliteDB) Tx(fn func(DB) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	inner := &sqliteDB{db: s.db, tx: tx}
	if err := fn(inner); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func newSqliteDB(dsn string) *sqliteDB {
	db := open(dsn)
	return &sqliteDB{db: db}
}

func adapter() {
	fmt.Println("== criterion 4: one bri.db-shaped API over sqlite (file + :memory:) ==")

	for _, tc := range []struct {
		label string
		dsn   string
	}{
		{"file DB (.dev/app.db)", mustTempDB()},
		{":memory: DB", ":memory:"},
	} {
		fmt.Printf("\n-- %s (dsn=%s) --\n", tc.label, tc.dsn)
		var d DB = newSqliteDB(tc.dsn)

		// schema — snake_case columns, as a real migration would write them
		_, err := d.(*sqliteDB).db.Exec(`CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			full_name TEXT NOT NULL,
			email_address TEXT NOT NULL,
			created_at INTEGER NOT NULL)`)
		must(err)

		// insert! with kebab-keyed attrs -> snake columns
		id, err := d.Insert("users", map[string]any{
			"full-name":     "Ada Lovelace",
			"email-address": "ada@example.com",
			"created-at":    time.Now().UnixMilli(),
		})
		must(err)
		fmt.Printf("insert users {:full-name ...} -> id %d\n", id)

		// one -> plain map, kebab keys back out
		row, err := d.One("SELECT id, full_name, email_address FROM users WHERE id = ?", id)
		must(err)
		fmt.Printf("one  -> %#v\n", row)
		if _, ok := row["full-name"]; !ok {
			panic("names doctrine failed: expected :full-name key")
		}

		// tx — two inserts commit together
		err = d.Tx(func(t DB) error {
			if _, e := t.Insert("users", map[string]any{"full-name": "Grace Hopper", "email-address": "grace@example.com", "created-at": time.Now().UnixMilli()}); e != nil {
				return e
			}
			_, e := t.Insert("users", map[string]any{"full-name": "Katherine Johnson", "email-address": "kj@example.com", "created-at": time.Now().UnixMilli()})
			return e
		})
		must(err)

		// query -> vector of maps
		all, err := d.Query("SELECT id, full_name FROM users ORDER BY id")
		must(err)
		fmt.Printf("query -> %d rows: ", len(all))
		var names []string
		for _, r := range all {
			names = append(names, fmt.Sprintf("%v", r["full-name"]))
		}
		fmt.Println(strings.Join(names, ", "))

		// tx rollback proof
		before, _ := d.Query("SELECT count(*) AS n FROM users")
		_ = d.Tx(func(t DB) error {
			_, _ = t.Insert("users", map[string]any{"full-name": "Rollback Me", "email-address": "x@example.com", "created-at": time.Now().UnixMilli()})
			return fmt.Errorf("boom") // force rollback
		})
		after, _ := d.Query("SELECT count(*) AS n FROM users")
		fmt.Printf("tx rollback: count before=%v after=%v (must be equal)\n", before[0]["n"], after[0]["n"])

		d.(*sqliteDB).db.Close()
	}

	// show the pgx side is the SAME interface (stub — S25 proved pgx for real)
	var _ DB = (*pgxStub)(nil)
	fmt.Println("\npgxStub satisfies the identical DB interface -> prod upgrade is a driver swap, same bri.db verbs.")
}

func mustTempDB() string {
	tmp, err := os.MkdirTemp("", "s37adapter")
	must(err)
	return tmp + "/app.db"
}

// pgxStub shows the prod driver is the same shape (compile-time proof only;
// S25 already benchmarked real pgx). bri.db picks the impl from APP_DB_URL:
// sqlite:// or file path -> sqliteDB (default), postgres:// -> pgxDB.
type pgxStub struct{}

func (*pgxStub) Query(string, ...any) ([]map[string]any, error) { panic("stub: use S25 pgx") }
func (*pgxStub) One(string, ...any) (map[string]any, error)     { panic("stub: use S25 pgx") }
func (*pgxStub) Insert(string, map[string]any) (int64, error)   { panic("stub: use S25 pgx") }
func (*pgxStub) Tx(func(DB) error) error                        { panic("stub: use S25 pgx") }

// ---------------------------------------------------------------------------
// criterion 5 — WAL + concurrency (server workload) probe
// ---------------------------------------------------------------------------

func walProbe() {
	fmt.Println("== criterion 5: WAL + concurrency probe (single file, N goroutines) ==")

	tmp, err := os.MkdirTemp("", "s37wal")
	must(err)
	defer os.RemoveAll(tmp)
	dbfile := tmp + "/server.db"

	db := open(dbfile)
	defer db.Close()

	// confirm WAL actually engaged
	var jmode string
	must(db.QueryRow("PRAGMA journal_mode").Scan(&jmode))
	fmt.Printf("journal_mode = %s (want: wal)\n", jmode)

	_, err = db.Exec(`CREATE TABLE events (id INTEGER PRIMARY KEY, who INTEGER, ts INTEGER)`)
	must(err)

	const writers = 8
	const readers = 8
	const perWriter = 2000

	var wErrs, rErrs int64
	var wrote, read int64
	start := time.Now()
	var wg sync.WaitGroup

	// writers — contend on the single file; busy_timeout should absorb locks
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(who int) {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				if _, e := db.Exec(`INSERT INTO events (who, ts) VALUES (?, ?)`, who, time.Now().UnixMilli()); e != nil {
					atomic.AddInt64(&wErrs, 1)
				} else {
					atomic.AddInt64(&wrote, 1)
				}
			}
		}(w)
	}
	// readers — WAL lets these run concurrently with the writer
	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				var n int
				if e := db.QueryRow(`SELECT count(*) FROM events`).Scan(&n); e != nil {
					atomic.AddInt64(&rErrs, 1)
				} else {
					atomic.AddInt64(&read, 1)
				}
			}
		}()
	}
	wg.Wait()
	dur := time.Since(start)

	fmt.Printf("%d writers x %d + %d readers x %d over ONE file in %v\n", writers, perWriter, readers, perWriter, dur)
	fmt.Printf("writes ok=%d err=%d | reads ok=%d err=%d\n", wrote, wErrs, read, rErrs)
	fmt.Printf("effective write throughput under contention: %.0f writes/sec\n", float64(wrote)/dur.Seconds())
	if wErrs == 0 && rErrs == 0 {
		fmt.Println("RESULT: no 'database is locked' errors — busy_timeout+WAL absorbed contention.")
	} else {
		fmt.Printf("RESULT: %d write + %d read errors under contention (locking gotcha to document).\n", wErrs, rErrs)
	}
}
