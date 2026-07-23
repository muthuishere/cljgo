package repl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mkSession runs a throwaway journaling session in dir that evaluates src,
// and returns its id. It exercises the real journal-writer header path
// (dir=…) so the folder-aware features have genuine journals to read.
func mkSession(t *testing.T, dir, src string) string {
	t.Helper()
	t.Setenv("CLJGO_SESSION", "1")
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	d, _, _ := newSession(src)
	if err := d.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	id := d.SessionID()
	if id == "" {
		t.Fatal("session got no id")
	}
	return id
}

// --- journalDir / prettyPath ------------------------------------------------

func TestJournalDirRecordsAndReadsFolder(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	work := t.TempDir()

	id := mkSession(t, work, "(def a 1)\n")
	got := journalDir(filepath.Join(sessionsDir(), id+".journal"))
	// macOS temp dirs are symlinked (/var → /private/var); compare resolved.
	if r1, _ := filepath.EvalSymlinks(got); r1 != mustEval(work) {
		t.Fatalf("journalDir = %q, want the session folder %q", got, work)
	}
}

func TestJournalDirEmptyForHeaderlessJournal(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.journal")
	if err := os.WriteFile(p, []byte(";; 2026 ns=user\n(def a 1)\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if d := journalDir(p); d != "" {
		t.Fatalf("journalDir of a headerless journal = %q, want empty", d)
	}
	if d := journalDir(filepath.Join(dir, "nope.journal")); d != "" {
		t.Fatalf("journalDir of a missing file = %q, want empty", d)
	}
}

func TestPrettyPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	cases := map[string]string{
		"":                        "(unknown)",
		home:                      "~",
		filepath.Join(home, "wk"): "~" + string(os.PathSeparator) + "wk",
		"/etc/x":                  "/etc/x",
	}
	for in, want := range cases {
		if got := prettyPath(in); got != want {
			t.Errorf("prettyPath(%q) = %q, want %q", in, got, want)
		}
	}
}

// --- resolveSessionRef ------------------------------------------------------

func TestResolveSessionRef(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	work := t.TempDir()

	// Three sessions, oldest → newest by id (ids sort by construction).
	a := mkSession(t, work, "(def a 1)\n")
	b := mkSession(t, work, "(def b 1)\n")
	c := mkSession(t, work, "(def c 1)\n")

	// Index is 1-based NEWEST-first: 1=c, 3=a.
	if got := resolveSessionRef("1"); got != c {
		t.Errorf("resolveSessionRef(1) = %q, want newest %q", got, c)
	}
	if got := resolveSessionRef("3"); got != a {
		t.Errorf("resolveSessionRef(3) = %q, want oldest %q", got, a)
	}
	// A literal id passes through untouched.
	if got := resolveSessionRef(b); got != b {
		t.Errorf("resolveSessionRef(id) = %q, want %q", got, b)
	}
	// Out-of-range index resolves to "" (caller errors).
	if got := resolveSessionRef("9"); got != "" {
		t.Errorf("resolveSessionRef(9) out of range = %q, want empty", got)
	}
	if got := resolveSessionRef("0"); got != "" {
		t.Errorf("resolveSessionRef(0) = %q, want empty", got)
	}
	// A long all-digit string is treated as a literal id, not an index.
	if got := resolveSessionRef("20260724"); got != "20260724" {
		t.Errorf("resolveSessionRef(long digits) = %q, want passthrough", got)
	}
}

// --- listSessions -----------------------------------------------------------

func TestListSessionsEmpty(t *testing.T) {
	setHome(t, t.TempDir())
	d, out, _ := newSession("")
	d.ListSessions()
	if !strings.Contains(out.String(), "no saved sessions") {
		t.Fatalf("empty listing = %q", out.String())
	}
}

func TestListSessionsNumberedNewestFirstWithFolder(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	work := t.TempDir()
	old := mkSession(t, work, "(def a 1)\n")
	recent := mkSession(t, work, "(def b 1)\n(def c 2)\n")

	d, out, _ := newSession("")
	d.ListSessions()
	s := out.String()
	for _, want := range []string{"sessions (newest first)", "#", "folder", "resume with", old, recent} {
		if !strings.Contains(s, want) {
			t.Errorf("listing missing %q\n%s", want, s)
		}
	}
	// Newest (recent) must be listed as #1 — appear before the older id.
	if strings.Index(s, recent) > strings.Index(s, old) {
		t.Errorf("newest session not first:\n%s", s)
	}
	// The session folder is shown (abbreviated), not "(unknown)".
	if !strings.Contains(s, prettyPath(mustEval(work))) && !strings.Contains(s, filepath.Base(work)) {
		t.Errorf("folder not shown in listing:\n%s", s)
	}
}

// --- resume cds back into the session folder --------------------------------

func TestResumeCdsBackIntoFolder(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	work := t.TempDir()
	elsewhere := t.TempDir()

	id := mkSession(t, work, "(def treasure 77)\n")

	// Resume from a DIFFERENT directory; resume must chdir back to work and
	// restore the var.
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	if err := os.Chdir(elsewhere); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLJGO_SESSION", "1")
	b, out, errOut := newSession("(+ treasure 1)\n")
	b.ResumeID = id
	if err := b.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "78") {
		t.Fatalf("var not restored (want 78): out=%q err=%q", out.String(), errOut.String())
	}
	nowCwd, _ := os.Getwd()
	if mustEval(nowCwd) != mustEval(work) {
		t.Fatalf("resume did not cd back: cwd=%q want=%q", nowCwd, work)
	}
	if !strings.Contains(out.String(), "resumed session "+id) {
		t.Fatalf("no resume notice: %q", out.String())
	}
}

func TestResumeMissingFolderStillReplays(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	gone := filepath.Join(t.TempDir(), "gone")
	if err := os.MkdirAll(gone, 0o755); err != nil {
		t.Fatal(err)
	}
	id := mkSession(t, gone, "(def kept 5)\n")
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CLJGO_SESSION", "1")
	b, out, _ := newSession("(* kept 2)\n")
	b.ResumeID = id
	if err := b.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "is gone") {
		t.Errorf("missing folder not noted: %q", out.String())
	}
	if !strings.Contains(out.String(), "10") {
		t.Errorf("var not replayed despite gone folder: %q", out.String())
	}
}

func TestResumeUnknownRefErrors(t *testing.T) {
	setHome(t, t.TempDir())
	t.Setenv("CLJGO_SESSION", "1")
	b, _, errOut := newSession("")
	b.ResumeID = "99" // no sessions exist → out-of-range index
	if err := b.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(errOut.String(), "no session") {
		t.Fatalf("expected a no-session error, got %q", errOut.String())
	}
}

// --- in-REPL :resume / :sessions commands -----------------------------------

func TestInReplResumeNoIdLists(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	work := t.TempDir()
	id := mkSession(t, work, "(def a 1)\n")

	t.Setenv("CLJGO_SESSION", "1")
	d, out, _ := newSession(":resume\n:sessions\n")
	d.Interactive = true
	if err := d.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Both :resume (no id) and :sessions print the table.
	if strings.Count(out.String(), "sessions (newest first)") < 2 {
		t.Fatalf(":resume and :sessions should both list:\n%s", out.String())
	}
	if !strings.Contains(out.String(), id) {
		t.Fatalf("listing missing the session id:\n%s", out.String())
	}
}

func TestInReplResumeByIndex(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	work := t.TempDir()
	mkSession(t, work, "(def gold 42)\n")

	t.Setenv("CLJGO_SESSION", "1")
	// A returning expression (result prints to the driver's out); println
	// side-effects go to *out*/stdout, not this buffer.
	d, out, errOut := newSession(":resume 1\n(+ gold 1)\n")
	d.Interactive = true
	if err := d.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "43") {
		t.Fatalf(":resume 1 did not restore the var (want 43): out=%q err=%q", out.String(), errOut.String())
	}
}

// sessionCommand is the whole-line dispatcher for :sessions / :resume; cover
// every branch directly (it is only called when journaling is on).
func TestSessionCommandBranches(t *testing.T) {
	setHome(t, t.TempDir())
	d, out, errOut := newSession("")
	cases := []struct {
		line     string
		consumed bool
	}{
		{"", false},                // no fields
		{"(+ 1 2)", false},         // not a session command
		{":sessions", true},        // lists
		{":sessions extra", false}, // arity wrong → flows to the reader
		{":resume", true},          // lists
		{":resume 1", true},        // resume by index (no sessions → errors, still consumed)
		{":resume a b c", true},    // too many → usage error, consumed
	}
	for _, c := range cases {
		if got := d.sessionCommand(c.line); got != c.consumed {
			t.Errorf("sessionCommand(%q) = %v, want %v", c.line, got, c.consumed)
		}
	}
	if !strings.Contains(errOut.String(), "usage: :resume") {
		t.Errorf(":resume a b c should print usage; errOut=%q", errOut.String())
	}
	_ = out
}

func TestResumeMalformedJournalReports(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	if err := os.MkdirAll(sessionsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	id := "20260724-120000-dead"
	// An unbalanced form makes the reader error (not EOF) mid-journal.
	body := ";; cljgo session " + id + " dir=/tmp\n\n(def ok 1)\n(def broken \n"
	if err := os.WriteFile(filepath.Join(sessionsDir(), id+".journal"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	d, out, errOut := newSession("")
	d.resumeSession(id)
	if !strings.Contains(errOut.String(), "journal "+id) {
		t.Errorf("malformed journal not reported: errOut=%q out=%q", errOut.String(), out.String())
	}
}

func TestResumeReportsFailedForms(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	if err := os.MkdirAll(sessionsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	id := "20260724-130000-beef"
	body := ";; cljgo session " + id + " dir=/tmp\n\n(def good 1)\n\n(this-var-does-not-exist)\n\n"
	if err := os.WriteFile(filepath.Join(sessionsDir(), id+".journal"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	d, out, _ := newSession("")
	d.resumeSession(id)
	if !strings.Contains(out.String(), "1 failed") {
		t.Errorf("failed-form count not surfaced: out=%q", out.String())
	}
}

// journalWriter disables journaling (with one warning) when the sessions dir
// cannot be created — here because its parent is a regular file.
func TestJournalDisabledWhenDirUnwritable(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	t.Setenv("CLJGO_SESSION", "1")
	// Make ~/.config/cljgo a FILE so MkdirAll(sessions) fails.
	cfg := filepath.Join(home, ".config", "cljgo")
	if err := os.MkdirAll(filepath.Dir(cfg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, _, errOut := newSession("(def a 1)\n")
	if err := d.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(errOut.String(), "journal disabled") {
		t.Errorf("expected a journal-disabled warning, got %q", errOut.String())
	}
	if d.journalOn {
		t.Error("journalOn should be false after the open failed")
	}
}

func mustEval(p string) string {
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		return p
	}
	return r
}
