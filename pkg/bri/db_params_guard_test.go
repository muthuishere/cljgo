// db_params_guard_test.go — the cljg.data.cast params-shape guard
// (docs/known-issues-2026-07-28.md §9, diagnostic G5008). Params are varargs;
// bundling them into a collection used to reach database/sql and die as
// `converting argument $1 type: unsupported type lang.Vector, a struct`,
// which named neither the fn nor the shape it wanted. It now fails at the API
// boundary. Like the rest of bri this has NO JVM oracle (cljg.data.cast does
// not exist in Clojure 1.12.5); the dual-mode (interpreted vs AOT-compiled)
// parity freeze lives in cmd/cljgo/testdata/dbparity.cljg.
package bri_test

import (
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/diag"
)

func TestDBParamsAsCollectionNamesTheVerbAndShape(t *testing.T) {
	d := newDriver(t)
	eval(t, d, dbPrelude)

	// Each verb names ITSELF, not the private -db-query/-db-exec shim.
	for _, tc := range []struct{ code, verb string }{
		{`(db/exec! conn "insert into notes (title) values (?)" ["x"])`, "exec!"},
		{`(db/query conn "select title from notes where title = ?" ["x"])`, "query"},
		{`(db/one conn "select title from notes where title = ?" ["x"])`, "one"},
		{`(db/one! conn "select title from notes where title = ?" ["x"])`, "one!"},
	} {
		msg := evalString(t, d, `(try `+tc.code+` (catch Throwable e (ex-message e)))`)
		want := "cljg.data.cast/" + tc.verb + ": param 1 is a vector — SQL params are varargs, " +
			"not a collection (expects [db sql & params], found a vector passed as one param); " +
			"spread it with (apply " + tc.verb + " db sql params)"
		if msg != want {
			t.Fatalf("%s\n got %q\nwant %q", tc.code, msg, want)
		}
	}

	// A map/set/seq param is rejected the same way, named by its own kind.
	for _, tc := range []struct{ param, kind string }{
		{`{:a 1}`, "map"},
		{`#{1}`, "set"},
		{`(map inc [1])`, "sequence"},
		{`(list 1)`, "sequence"},
	} {
		msg := evalString(t, d,
			`(try (db/query conn "select title from notes where title = ?" `+tc.param+
				`) (catch Throwable e (ex-message e)))`)
		if !strings.Contains(msg, "param 1 is a "+tc.kind+" —") {
			t.Fatalf("param %s: got %q, want it named a %s", tc.param, msg, tc.kind)
		}
	}

	// `apply` is the documented escape hatch and still works.
	eval(t, d, `(apply db/exec! conn "insert into notes (title) values (?)" ["ok"])`)
	if got := evalString(t, d, `(:title (db/one conn "select title from notes where title = ?" "ok"))`); got != "ok" {
		t.Fatalf("apply-spread params did not round-trip: %q", got)
	}

	// Scalars keep working: strings, numbers, keywords and nil are legal.
	eval(t, d, `(db/exec! conn "insert into notes (title, body) values (?, ?)" :kw nil)`)
	if got := evalString(t, d, `(:title (db/one conn "select title from notes where title = ?" "kw"))`); got != "kw" {
		t.Fatalf("keyword param did not round-trip: %q", got)
	}
}

// The row-map verbs (insert!/update!/delete!) take MAPS, not varargs: a
// collection sitting in one of those maps is a column-value mistake, not the
// varargs misuse above. Until 2026-07-28 all three were misdiagnosed as
// `cljg.data.cast/exec!: param 1 is a vector … (apply exec! db sql params)` —
// a verb the user never called and a fix that fixes nothing. Each verb now
// names ITSELF and the offending column, and offers no Fix at all.
func TestRowMapCollectionValueIsItsOwnDiagnosis(t *testing.T) {
	d := newDriver(t)
	eval(t, d, dbPrelude)

	for _, tc := range []struct{ code, want string }{
		{`(db/insert! conn :notes {:title ["x" "y"]})`,
			"cljg.data.cast/insert!: column :title of the row map is a vector — " +
				"a column value must be a scalar SQL param " +
				"(expects a string, number, boolean, keyword or nil, found a vector)"},
		{`(db/update! conn :notes {:title {:k 1}} {:body "z"})`,
			"cljg.data.cast/update!: column :title of the set map is a map — " +
				"a column value must be a scalar SQL param " +
				"(expects a string, number, boolean, keyword or nil, found a map)"},
		{`(db/update! conn :notes {:title "t"} {:body #{1}})`,
			"cljg.data.cast/update!: column :body of the where map is a set — " +
				"a column value must be a scalar SQL param " +
				"(expects a string, number, boolean, keyword or nil, found a set)"},
		{`(db/delete! conn :notes {:title #{1}})`,
			"cljg.data.cast/delete!: column :title of the where map is a set — " +
				"a column value must be a scalar SQL param " +
				"(expects a string, number, boolean, keyword or nil, found a set)"},
	} {
		msg := evalString(t, d, `(try `+tc.code+` (catch Throwable e (ex-message e)))`)
		if msg != tc.want {
			t.Fatalf("%s\n got %q\nwant %q", tc.code, msg, tc.want)
		}
		// The wrong advice must be gone: no `exec!`, no `apply` suggestion.
		if strings.Contains(msg, "apply") || strings.Contains(msg, "exec!") || strings.Contains(msg, "varargs") {
			t.Fatalf("%s: still carries the varargs framing: %q", tc.code, msg)
		}
	}

	// It carries its own registered code with an explain page.
	_, err := d.EvalString(`(db/insert! conn :notes {:title ["x"]})`, "bri_test")
	if err == nil {
		t.Fatal("want an error from a collection column value")
	}
	dg := diag.FromError(err)
	if dg.ErrorCode != "G5009" {
		t.Fatalf("diagnostic code = %q, want G5009 (err %v)", dg.ErrorCode, err)
	}
	if _, err := diag.Explain("G5009"); err != nil {
		t.Fatalf("G5009 has no explain page: %v", err)
	}
	if got := diag.Render(dg); !strings.HasSuffix(got, "help: run `cljgo explain G5009`") {
		t.Fatalf("rendered line = %q, want the explain pointer", got)
	}

	// Scalar column values keep round-tripping through all three verbs.
	eval(t, d, `(db/insert! conn :notes {:title "omega" :body "b"})`)
	eval(t, d, `(db/update! conn :notes {:body "b2"} {:title "omega"})`)
	if got := evalString(t, d, `(:body (db/one conn "select body from notes where title = ?" "omega"))`); got != "b2" {
		t.Fatalf("update! round-trip = %q, want b2", got)
	}
	eval(t, d, `(db/delete! conn :notes {:title "omega"})`)
	if got := evalString(t, d, `(str (:n (db/one conn "select count(*) as n from notes where title = ?" "omega")))`); got != "0" {
		t.Fatalf("delete! left %q rows, want 0", got)
	}
}

// The guard carries the registered G5008 code, so the rendered line ends in
// the `help: run cljgo explain G5008` pointer the doctrine asks for.
func TestDBParamsGuardCarriesG5008(t *testing.T) {
	d := newDriver(t)
	eval(t, d, dbPrelude)
	_, err := d.EvalString(`(db/query conn "select title from notes where title = ?" [1])`, "bri_test")
	if err == nil {
		t.Fatal("want an error from a collection param")
	}
	dg := diag.FromError(err)
	if dg.ErrorCode != "G5008" {
		t.Fatalf("diagnostic code = %q, want G5008 (err %v)", dg.ErrorCode, err)
	}
	if _, err := diag.Explain("G5008"); err != nil {
		t.Fatalf("G5008 has no explain page: %v", err)
	}
	if got := diag.Render(dg); !strings.HasSuffix(got, "help: run `cljgo explain G5008`") {
		t.Fatalf("rendered line = %q, want the explain pointer", got)
	}
}
