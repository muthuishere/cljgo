// dbparity_test.go — the bri.db dual-mode parity gate (ADR 0072 dec 8).
// testdata/dbparity.cljg drives the whole blessed data surface (connect,
// insert!, tx commit + rollback, update!, delete!, query, one) over an
// in-memory SQLite database and prints a deterministic transcript. This
// test runs it BOTH interpreted (`cljgo run`) and AOT-compiled (`cljgo
// build`) and asserts byte-identical output. A REPL↔binary divergence is
// the release blocker (CLAUDE.md); any diff here fails the build. It is
// also the proof a bri.db app links CGO_ENABLED=0 (modernc SQLite is pure
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
		t.Fatalf("running the compiled bri.db binary: %v", err)
	}

	if string(interp) != string(compiled) {
		t.Fatalf("bri.db REPL↔binary divergence (release blocker):\n--- interpreted ---\n%s\n--- compiled ---\n%s",
			interp, compiled)
	}

	// And the transcript is the expected one (so a matching-but-wrong pair
	// can't pass silently).
	want := "row 2 beta 9\nrow 3 gamma 1\none beta\ncount 2\n"
	if string(compiled) != want {
		t.Fatalf("bri.db parity transcript =\n%q\nwant\n%q", compiled, want)
	}
}
