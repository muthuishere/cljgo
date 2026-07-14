// Session journals (ADR 0016): every successful top-level form is
// appended — with its namespace context and a timestamp — to
// ~/.config/cljgo/sessions/<id>.journal, plain readable Clojure.
// Failed forms are journaled as comments (visible history, never
// replayed). :resume <id> replays a journal through the normal
// read→analyze→eval path and continues journaling to that id.
// Journaling is off when stdin is not a tty unless CLJGO_SESSION=1,
// so scripts, pipes and tests stay clean.
package repl

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/muthuishere/cljgo/pkg/reader"
)

// sessionEnabled decides whether this session journals: CLJGO_SESSION=1
// forces on, CLJGO_SESSION=0 forces off, otherwise on only when input
// is an interactive terminal.
func sessionEnabled(in io.Reader) bool {
	switch os.Getenv("CLJGO_SESSION") {
	case "1":
		return true
	case "0":
		return false
	}
	f, ok := in.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// newSessionID is short and sortable: yyyymmdd-hhmmss-rand4.
func newSessionID() string {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		b = [2]byte{byte(time.Now().UnixNano()), byte(time.Now().UnixNano() >> 8)}
	}
	return time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(b[:])
}

func sessionsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".config", "cljgo", "sessions")
}

func (d *Driver) journalPath(id string) string {
	return filepath.Join(sessionsDir(), id+".journal")
}

// SessionID is this session's journal id ("" when journaling is off or
// Run has not started). Frontends may print it at startup.
func (d *Driver) SessionID() string { return d.sessionID }

// journalWriter lazily opens the append-only journal (creating the
// sessions dir on first use), so sessions that never evaluate anything
// leave no file behind. A failure to open disables journaling for the
// session with one warning.
func (d *Driver) journalWriter() io.Writer {
	if !d.journalOn {
		return nil
	}
	if d.journalFile == nil {
		if err := os.MkdirAll(sessionsDir(), 0o755); err != nil {
			d.journalOn = false
			d.reportError(fmt.Errorf("session journal disabled: %v", err))
			return nil
		}
		f, err := os.OpenFile(d.journalPath(d.sessionID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			d.journalOn = false
			d.reportError(fmt.Errorf("session journal disabled: %v", err))
			return nil
		}
		d.journalFile = f
	}
	return d.journalFile
}

// journalSuccess appends one successful top-level form. Written BEFORE
// the result prints (ADR 0016 §3): a crash loses at most the in-flight
// form. ns is the namespace the form was evaluated in.
func (d *Driver) journalSuccess(ns, src string) {
	src = strings.TrimSpace(src)
	if src == "" {
		return
	}
	w := d.journalWriter()
	if w == nil {
		return
	}
	fmt.Fprintf(w, ";; %s ns=%s\n%s\n\n", time.Now().Format(time.RFC3339), ns, src)
}

// journalFailure appends a failed form as comments (ADR 0016 §5):
// visible history, never replayed.
func (d *Driver) journalFailure(ns, src string, evalErr error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return
	}
	w := d.journalWriter()
	if w == nil {
		return
	}
	msg := strings.SplitN(evalErr.Error(), "\n", 2)[0]
	fmt.Fprintf(w, ";; %s ns=%s failed: %s\n", time.Now().Format(time.RFC3339), ns, msg)
	for _, line := range strings.Split(src, "\n") {
		fmt.Fprintf(w, ";; %s\n", line)
	}
	fmt.Fprintln(w)
}

// closeJournal flushes the session's journal at the end of Run.
func (d *Driver) closeJournal() {
	if d.journalFile != nil {
		d.journalFile.Close()
		d.journalFile = nil
	}
}

// sessionCommand handles the in-session commands :sessions and
// :resume <id> when they are a whole line at an empty prompt. Returns
// true when the line was consumed as a command. Only reached when
// journaling is enabled — otherwise the line flows to the reader and
// keeps its ordinary keyword semantics.
func (d *Driver) sessionCommand(line string) bool {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return false
	}
	switch fields[0] {
	case ":sessions":
		if len(fields) != 1 {
			return false
		}
		d.listSessions()
		return true
	case ":resume":
		if len(fields) != 2 {
			d.reportError(errors.New("usage: :resume <id>"))
			return true
		}
		d.resumeSession(fields[1])
		return true
	}
	return false
}

// listSessions prints id, last-active and form count for every saved
// journal, oldest first (ids are sortable by construction).
func (d *Driver) listSessions() {
	entries, err := os.ReadDir(sessionsDir())
	var ids []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".journal") {
			ids = append(ids, strings.TrimSuffix(e.Name(), ".journal"))
		}
	}
	if err != nil || len(ids) == 0 {
		d.outMu.Lock()
		fmt.Fprintln(d.out, "no saved sessions")
		d.outMu.Unlock()
		return
	}
	sort.Strings(ids)
	d.outMu.Lock()
	defer d.outMu.Unlock()
	for _, id := range ids {
		path := d.journalPath(id)
		lastActive := "?"
		if fi, err := os.Stat(path); err == nil {
			lastActive = fi.ModTime().Format("2006-01-02 15:04")
		}
		fmt.Fprintf(d.out, "%s  last-active %s  %d forms\n", id, lastActive, d.countJournalForms(path))
	}
}

// countJournalForms counts the replayable forms in a journal by reading
// it — comments (failed forms) are skipped by the reader, so the count
// is exact. Journals are small; this is cheap.
func (d *Driver) countJournalForms(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	rd := reader.New(strings.NewReader(string(data)), reader.WithFilename(path),
		reader.WithResolver(d.ev.ReaderResolver()))
	n := 0
	for {
		if _, err := rd.ReadOne(); err != nil {
			return n
		}
		n++
	}
}

// resumeSession replays journal <id> through the normal eval path
// (restoring vars, namespaces and macros — replay IS the state, ADR
// 0016), prints the honesty notice of ADR 0016 §4, then switches this
// session's journaling to that id. Replay results are not printed and
// replayed forms are not re-journaled. Runs on Run's goroutine, under
// the session frame, so in-ns during replay moves the prompt as usual.
func (d *Driver) resumeSession(id string) {
	path := d.journalPath(id)
	data, err := os.ReadFile(path)
	if err != nil {
		d.reportError(fmt.Errorf("no session %s (%v)", id, err))
		return
	}
	rd := reader.New(strings.NewReader(string(data)), reader.WithFilename(path),
		reader.WithResolver(d.ev.ReaderResolver()))
	replayed, failed := 0, 0
	for {
		form, err := rd.ReadOne()
		if errors.Is(err, reader.ErrEOF) {
			break
		}
		if err != nil {
			d.reportError(fmt.Errorf("journal %s: %v", id, err))
			break
		}
		if _, err := d.ev.EvalForm(form); err != nil {
			failed++
			d.reportError(err)
			continue
		}
		replayed++
	}
	// Continue journaling to the resumed id: new forms append to it.
	d.closeJournal()
	d.sessionID = id
	d.journalOn = true
	d.outMu.Lock()
	fmt.Fprintf(d.out, "resumed session %s: %d forms replayed", id, replayed)
	if failed > 0 {
		fmt.Fprintf(d.out, " (%d failed)", failed)
	}
	fmt.Fprintln(d.out)
	fmt.Fprintln(d.out, "note: running goroutines, open channels and native handles do not survive resume; re-run the forms that created them.")
	d.outMu.Unlock()
}
