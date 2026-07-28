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
