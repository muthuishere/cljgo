package conformance

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestM0ExitDemo verifies the M0 exit demo (design/00 §6) end to end
// through the REAL binary: build cmd/cljgo, pipe the fact definition,
// (fact 10), and a re-def sequence into `cljgo repl`, and assert
// 3628800 plus the re-def being visible to a previously captured
// reference.
func TestM0ExitDemo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	bin := filepath.Join(t.TempDir(), "cljgo")
	build := exec.Command("go", "build", "-o", bin, "github.com/muthuishere/cljgo/cmd/cljgo")
	build.Dir = ".." // module root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	const demo = `(def fact (fn* fact [n] (if (< n 2) 1 (* n (fact (- n 1))))))
(fact 10)
(def fact-via-var (fn* [n] (fact n)))
(fact-via-var 10)
(def fact (fn* [n] :redefined))
(fact-via-var 10)
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
		"#=(var user/fact)",
		"3628800",
		"#=(var user/fact-via-var)",
		"3628800",
		"#=(var user/fact)",
		":redefined", // the captured reference sees the re-defed fact
	}
	if len(lines) != len(want) {
		t.Fatalf("got %d output lines %q, want %d", len(lines), lines, len(want))
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i+1, lines[i], want[i])
		}
	}
}

// TestM0RunFile drives `cljgo run` over a temp file end to end.
func TestM0RunFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	bin := filepath.Join(t.TempDir(), "cljgo")
	build := exec.Command("go", "build", "-o", bin, "github.com/muthuishere/cljgo/cmd/cljgo")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	src := filepath.Join(t.TempDir(), "fact.clj")
	prog := `(def fact (fn* fact [n] (if (< n 2) 1 (* n (fact (- n 1))))))
(println (fact 10))
`
	if err := os.WriteFile(src, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(bin, "run", src).Output()
	if err != nil {
		t.Fatalf("cljgo run: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "3628800" {
		t.Fatalf("output = %q, want 3628800", got)
	}
}
