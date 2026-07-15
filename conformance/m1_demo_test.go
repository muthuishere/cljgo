package conformance

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/emit"
)

// TestM1ExitDemo verifies the M1 exit demo (design/00 §6, eval v2) end
// to end through the REAL binary: build cmd/cljgo and pipe the macro
// demos into `cljgo repl` — a defmacro typed at the prompt must work on
// the very next form, and the core.clj macros (defn, when, ->) must be
// live in the user namespace.
func TestM1ExitDemo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	bin := filepath.Join(t.TempDir(), "cljgo"+emit.ExeSuffix)
	build := exec.Command("go", "build", "-o", bin, "github.com/muthuishere/cljgo/cmd/cljgo")
	build.Dir = ".." // module root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	const demo = `(defmacro unless [t e] (list 'if t nil e))
(unless false 42)
(defn f [x] (when (< x 5) (* x 2)))
(f 2)
(-> 5 (+ 3) (* 2))
`
	cmd := exec.Command(bin, "repl")
	cmd.Stdin = strings.NewReader(demo)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("cljgo repl: %v\nstderr: %s", err, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}

	// Stdin is a pipe, so no prompts: one pr-str line per form.
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	want := []string{
		"#=(var user/unless)", // defmacro returns its var
		"42",                  // the macro works on the NEXT form
		"#=(var user/f)",
		"4",  // (defn f [x] (when (< x 5) (* x 2))) → (f 2)
		"16", // (-> 5 (+ 3) (* 2))
	}
	if len(lines) != len(want) {
		t.Fatalf("got %d output lines %q, want %d", len(lines), lines, len(want))
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
}
