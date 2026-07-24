// db_test.go — the behavior suite for bri.db (ADR 0072). Like the rest
// of bri these behaviors have NO JVM oracle (bri.db does not exist in
// Clojure 1.12.5), so they run against the real interpreter here rather
// than in conformance/tests. Every test uses a private in-memory SQLite
// database (the ADR 0072 test sandbox) — pure Go, CGO_ENABLED=0, no file,
// no server. The dual-mode (interpreted vs AOT-compiled) parity check
// lives in the parity harness (pkg/bri/dbparity_test.go).
package bri_test

import (
	"os"
	"path/filepath"
	"testing"
)

// dbPrelude opens a fresh in-memory SQLite db and creates a notes table.
const dbPrelude = `
(require '[bri.db :as db])
(def conn (db/connect {:driver :sqlite :database ":memory:"}))
(db/exec! conn "create table notes (id integer primary key autoincrement, title text not null, body text, created_at text)")
`

func writeMigration(t *testing.T, dir, name, sql string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(sql), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Scenario: connect + parametrized query round-trips a row.
func TestConnectInsertQuery(t *testing.T) {
	d := newDriver(t)
	eval(t, d, dbPrelude)
	eval(t, d, `(db/insert! conn :notes {:title "hello" :body "world"})`)
	title := evalString(t, d, `(:title (db/one conn "select title from notes where title = ?" "hello"))`)
	if title != "hello" {
		t.Fatalf("query title = %q, want \"hello\"", title)
	}
}

// Scenario: params bind, never interpolate — a SQL-injection payload is
// stored as a literal string, not executed.
func TestParamsAreNotInterpolated(t *testing.T) {
	d := newDriver(t)
	eval(t, d, dbPrelude)
	eval(t, d, `(db/insert! conn :notes {:title "a'; drop table notes; --"})`)
	// The table still exists and holds exactly the literal row.
	n := eval(t, d, `(:n (db/one conn "select count(*) as n from notes"))`)
	if n != int64(1) {
		t.Fatalf("row count = %v, want 1 (injection payload stored as data)", n)
	}
	got := evalString(t, d, `(:title (db/one conn "select title from notes"))`)
	if got != "a'; drop table notes; --" {
		t.Fatalf("stored title = %q, want the literal payload", got)
	}
}

// Scenario: snake_case columns become kebab-case keyword keys.
func TestSnakeToKebabKeys(t *testing.T) {
	d := newDriver(t)
	eval(t, d, dbPrelude)
	eval(t, d, `(db/insert! conn :notes {:title "t" :created-at "2026-07-24T00:00:00Z"})`)
	got := evalString(t, d, `(:created-at (db/one conn "select created_at from notes"))`)
	if got != "2026-07-24T00:00:00Z" {
		t.Fatalf("created-at = %q, want the kebab-keyed value", got)
	}
}

// Scenario: insert! returns the last insert id (SQLite).
func TestInsertReturnsLastId(t *testing.T) {
	d := newDriver(t)
	eval(t, d, dbPrelude)
	id := eval(t, d, `(:last-insert-id (db/insert! conn :notes {:title "one"}))`)
	if id != int64(1) {
		t.Fatalf("first :last-insert-id = %v, want 1", id)
	}
	id2 := eval(t, d, `(:last-insert-id (db/insert! conn :notes {:title "two"}))`)
	if id2 != int64(2) {
		t.Fatalf("second :last-insert-id = %v, want 2", id2)
	}
}

// Scenario: exec! reports rows-affected; update!/delete! flow through it.
func TestUpdateDeleteRowsAffected(t *testing.T) {
	d := newDriver(t)
	eval(t, d, dbPrelude)
	eval(t, d, `(db/insert! conn :notes {:title "old"})`)
	up := eval(t, d, `(:rows-affected (db/update! conn :notes {:title "new"} {:title "old"}))`)
	if up != int64(1) {
		t.Fatalf("update! rows-affected = %v, want 1", up)
	}
	del := eval(t, d, `(:rows-affected (db/delete! conn :notes {:title "new"}))`)
	if del != int64(1) {
		t.Fatalf("delete! rows-affected = %v, want 1", del)
	}
	n := eval(t, d, `(:n (db/one conn "select count(*) as n from notes"))`)
	if n != int64(0) {
		t.Fatalf("remaining rows = %v, want 0", n)
	}
}

// Scenario: one! throws :bri.db/not-found on no match.
func TestOneBangNotFound(t *testing.T) {
	d := newDriver(t)
	eval(t, d, dbPrelude)
	ok := eval(t, d, `(try (db/one! conn "select * from notes where id = ?" 999)
                          (catch Throwable e (= :bri.db/not-found (:bri.db/error (ex-data e)))))`)
	if ok != true {
		t.Fatalf("one! on no row did not throw :bri.db/not-found (got %v)", ok)
	}
}

// Scenario: a tx commits on normal return.
func TestTxCommits(t *testing.T) {
	d := newDriver(t)
	eval(t, d, dbPrelude)
	eval(t, d, `(db/tx conn (fn [t]
                              (db/insert! t :notes {:title "a"})
                              (db/insert! t :notes {:title "b"})))`)
	n := eval(t, d, `(:n (db/one conn "select count(*) as n from notes"))`)
	if n != int64(2) {
		t.Fatalf("after commit rows = %v, want 2", n)
	}
}

// Scenario: a tx rolls back on throw and re-raises.
func TestTxRollsBackOnThrow(t *testing.T) {
	d := newDriver(t)
	eval(t, d, dbPrelude)
	threw := eval(t, d, `(try
                           (db/tx conn (fn [t]
                                         (db/insert! t :notes {:title "doomed"})
                                         (throw (ex-info "boom" {}))))
                           false
                           (catch Throwable e true))`)
	if threw != true {
		t.Fatalf("tx body throw did not propagate")
	}
	n := eval(t, d, `(:n (db/one conn "select count(*) as n from notes"))`)
	if n != int64(0) {
		t.Fatalf("after rollback rows = %v, want 0", n)
	}
}

// Scenario: with-rollback always rolls back (the per-test sandbox).
func TestWithRollback(t *testing.T) {
	d := newDriver(t)
	eval(t, d, dbPrelude)
	eval(t, d, `(db/with-rollback conn (fn [t] (db/insert! t :notes {:title "temp"})))`)
	n := eval(t, d, `(:n (db/one conn "select count(*) as n from notes"))`)
	if n != int64(0) {
		t.Fatalf("with-rollback left %v rows, want 0", n)
	}
}

// Scenario: migrations apply, are idempotent, and status reports state.
func TestMigrationsApplyIdempotentStatus(t *testing.T) {
	dir := t.TempDir()
	writeMigration(t, dir, "20260724120000_create_widgets.sql",
		"create table widgets (id integer primary key, name text);")
	writeMigration(t, dir, "20260724130000_add_color.sql",
		"alter table widgets add column color text;")

	d := newDriver(t)
	eval(t, d, `(require '[bri.db :as db])
                (def conn (db/connect {:driver :sqlite :database ":memory:"}))`)

	// First run applies both.
	pendingBefore := eval(t, d, `(count (:pending (db/migrate-status conn "`+dir+`")))`)
	if pendingBefore != int64(2) {
		t.Fatalf("pending before migrate = %v, want 2", pendingBefore)
	}
	eval(t, d, `(db/migrate! conn "`+dir+`")`)
	// The schema is present: an insert into the migrated table succeeds.
	eval(t, d, `(db/insert! conn :widgets {:name "w" :color "red"})`)
	got := evalString(t, d, `(:color (db/one conn "select color from widgets"))`)
	if got != "red" {
		t.Fatalf("migrated column value = %q, want \"red\"", got)
	}
	// Idempotent: a second migrate applies nothing; status shows none pending.
	eval(t, d, `(db/migrate! conn "`+dir+`")`)
	appliedN := eval(t, d, `(count (:applied (db/migrate-status conn "`+dir+`")))`)
	pendingN := eval(t, d, `(count (:pending (db/migrate-status conn "`+dir+`")))`)
	if appliedN != int64(2) || pendingN != int64(0) {
		t.Fatalf("after migrate applied=%v pending=%v, want 2/0", appliedN, pendingN)
	}
}
