package repl

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// setHome points os.UserHomeDir() at dir for the duration of the test.
//
// UserHomeDir reads $HOME on unix but %USERPROFILE% on Windows, so setting
// HOME alone silently does nothing there — the test would then read the
// runner's REAL home and fail looking for a journal that was never written
// to the temp dir. Set both.
func setHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", dir)
	}
}

// runInteractive runs a driver with the ADR 0018 interactive affordances
// enabled via the injected seam (no real tty), Prompts off so assertions
// see only results and affordance output.
func runInteractive(t *testing.T, in string) (*Driver, string, string) {
	t.Helper()
	d, out, errOut := newSession(in)
	d.Interactive = true
	if err := d.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return d, out.String(), errOut.String()
}

func TestExitFallbackEndsSession(t *testing.T) {
	// `exit` at an interactive prompt ends the session with a farewell;
	// the form after it must never evaluate.
	_, out, errOut := runInteractive(t, "exit\n(+ 1 2)\n")
	if !strings.Contains(out, "Goodbye") {
		t.Fatalf("want a farewell, got out=%q", out)
	}
	if strings.Contains(out, "3") {
		t.Fatalf("form after exit must not evaluate, got out=%q", out)
	}
	if errOut != "" {
		t.Fatalf("exit must not error, got errOut=%q", errOut)
	}

	// (exit) — the zero-arg call form — works too.
	_, out2, _ := runInteractive(t, "(quit)\n")
	if !strings.Contains(out2, "Goodbye") {
		t.Fatalf("(quit) should end the session, got out=%q", out2)
	}
}

func TestExitInertWhenNotInteractive(t *testing.T) {
	// Piped (non-interactive) input keeps the historical semantics: bare
	// exit is an unresolved symbol, not an affordance.
	out, errOut := run(t, "exit\n")
	if strings.Contains(out, "Goodbye") {
		t.Fatalf("exit must be inert off a tty, got out=%q", out)
	}
	if !strings.Contains(errOut, "unable to resolve symbol: exit") {
		t.Fatalf("want unresolved-symbol error, got errOut=%q", errOut)
	}
}

func TestUserDefinedExitWins(t *testing.T) {
	// The precedence principle: a user-defined exit shadows the
	// affordance even in an interactive session.
	_, out, errOut := runInteractive(t, "(def exit 42)\nexit\n(+ 1 1)\n")
	if strings.Contains(out, "Goodbye") {
		t.Fatalf("user-defined exit must win, got farewell in out=%q", out)
	}
	lines := outLines(out)
	if lines[len(lines)-1] != "2" {
		t.Fatalf("session must continue past a user exit, out=%q", out)
	}
	if !strings.Contains(out, "42") {
		t.Fatalf("user exit should evaluate to its value, out=%q", out)
	}
	if errOut != "" {
		t.Fatalf("unexpected error output: %q", errOut)
	}
}

func TestHelp(t *testing.T) {
	_, out, errOut := runInteractive(t, "help\n(+ 1 1)\n")
	if !strings.Contains(out, "REPL affordances") {
		t.Fatalf("help should list affordances, got out=%q", out)
	}
	for _, want := range []string{"exit, quit", "*1 *2 *3", ":resume"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help missing %q, got out=%q", want, out)
		}
	}
	// help does not end the session.
	if !strings.Contains(out, "2") {
		t.Fatalf("session must continue after help, out=%q", out)
	}
	if errOut != "" {
		t.Fatalf("unexpected error output: %q", errOut)
	}
}

func TestDidYouMean(t *testing.T) {
	// A near-miss on an interned/referred name gets a suggestion; this
	// works regardless of interactivity (it rides the diagnostic).
	_, errOut := run(t, "pritnln\n")
	if !strings.Contains(errOut, "unable to resolve symbol: pritnln") {
		t.Fatalf("missing resolve error: %q", errOut)
	}
	if !strings.Contains(errOut, "did you mean") || !strings.Contains(errOut, "println") {
		t.Fatalf("want did-you-mean println, got %q", errOut)
	}
}

func TestJournalWritesSuccessfulForms(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	t.Setenv("CLJGO_SESSION", "1")

	d, _, _ := newSession("(+ 1 2)\n(nope)\n")
	if err := d.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	id := d.SessionID()
	if id == "" {
		t.Fatal("expected a session id when journaling is enabled")
	}
	path := filepath.Join(home, ".config", "cljgo", "sessions", id+".journal")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	j := string(data)
	if !strings.Contains(j, "(+ 1 2)") {
		t.Fatalf("successful form not journaled: %q", j)
	}
	// Failed forms are journaled as comments (ADR 0016 §5), never as a
	// replayable form.
	if !strings.Contains(j, "failed:") || !strings.Contains(j, ";; (nope)") {
		t.Fatalf("failed form not journaled as a comment: %q", j)
	}
}

func TestResumeRoundTrip(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	t.Setenv("CLJGO_SESSION", "1")

	// Session A defines a var and journals it.
	a, _, _ := newSession("(def x 99)\n")
	if err := a.Run(); err != nil {
		t.Fatalf("Run A: %v", err)
	}
	id := a.SessionID()
	if id == "" {
		t.Fatal("session A has no id")
	}

	// Session B is a fresh evaluator (newSession removes the user ns, so x
	// is gone); :resume must replay the journal to restore it before the
	// new form runs.
	b, out, errOut := newSession(":resume " + id + "\n(+ x 1)\n")
	if err := b.Run(); err != nil {
		t.Fatalf("Run B: %v", err)
	}
	if !strings.Contains(out.String(), "resumed session "+id) {
		t.Fatalf("no resume notice, out=%q", out.String())
	}
	if !strings.Contains(out.String(), "100") {
		t.Fatalf("replay did not restore x (want 100), out=%q errOut=%q", out.String(), errOut.String())
	}
}
