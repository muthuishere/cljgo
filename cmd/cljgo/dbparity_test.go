// dbparity_test.go — the cljg.data.cast dual-mode parity gate (ADR 0072 dec 8).
// testdata/dbparity.cljg drives the whole blessed data surface (connect,
// insert!, tx commit + rollback, update!, delete!, query, one) over an
// in-memory SQLite database and prints a deterministic transcript. This
// test runs it BOTH interpreted (`cljgo run`) and AOT-compiled (`cljgo
// build`) and asserts byte-identical output. A REPL↔binary divergence is
// the release blocker (CLAUDE.md); any diff here fails the build. It is
// also the proof a cljg.data.cast app links CGO_ENABLED=0 (modernc SQLite is pure
// Go) — the whole point of ADR 0057.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/muthuishere/cljgo/pkg/emit"
)

func TestBriDBParity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	bin := buildCljgo(t)
	src, err := filepath.Abs(filepath.Join("testdata", "dbparity.cljg"))
	if err != nil {
		t.Fatal(err)
	}

	// Interpreted: `cljgo run` evaluates the file's top-level forms.
	interp, err := exec.Command(bin, "run", src).Output()
	if err != nil {
		t.Fatalf("cljgo run: %v", err)
	}

	// Compiled: `cljgo build` → a static binary whose func main runs the
	// same top-level forms.
	out := filepath.Join(t.TempDir(), "dbparity"+emit.ExeSuffix)
	build := exec.Command(bin, "build", "-o", out, src)
	build.Env = append(os.Environ(), "CLJGO_SRC="+repoRoot(t))
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("cljgo build: %v\n%s", err, b)
	}
	compiled, err := exec.Command(out).Output()
	if err != nil {
		t.Fatalf("running the compiled cljg.data.cast binary: %v", err)
	}

	if string(interp) != string(compiled) {
		t.Fatalf("cljg.data.cast REPL↔binary divergence (release blocker):\n--- interpreted ---\n%s\n--- compiled ---\n%s",
			interp, compiled)
	}

	// And the transcript is the expected one (so a matching-but-wrong pair
	// can't pass silently).
	want := "row 2 beta 9\nrow 3 gamma 1\none beta\ncount 2\n" +
		"cast delta 4 admin? false\ncast-err expected an integer\n" +
		"params-err cljg.data.cast/query: param 1 is a vector — SQL params are varargs, " +
		"not a collection (expects [db sql & params], found a vector passed as one param); " +
		"spread it with (apply query db sql params)\n" +
		"params-apply beta\n" +
		"row-insert-err cljg.data.cast/insert!: column :label of the row map is a vector — " +
		"a column value must be a scalar SQL param " +
		"(expects a string, number, boolean, keyword or nil, found a vector)\n" +
		"row-update-err cljg.data.cast/update!: column :label of the set map is a map — " +
		"a column value must be a scalar SQL param " +
		"(expects a string, number, boolean, keyword or nil, found a map)\n" +
		"row-delete-err cljg.data.cast/delete!: column :label of the where map is a set — " +
		"a column value must be a scalar SQL param " +
		"(expects a string, number, boolean, keyword or nil, found a set)\n" +
		"row-ok 6\nrow-gone 0\n"
	if string(compiled) != want {
		t.Fatalf("cljg.data.cast parity transcript =\n%q\nwant\n%q", compiled, want)
	}
}
