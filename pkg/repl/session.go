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
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
		// A fresh journal records the working directory as a header comment
		// (ADR 0070): `:sessions` shows which folder a session belongs to,
		// and :resume cds back into it so requires/loads resolve as they did.
		// A comment, so the reader skips it on replay.
		if fi, _ := f.Stat(); fi != nil && fi.Size() == 0 {
			if wd, err := os.Getwd(); err == nil {
				fmt.Fprintf(f, ";; cljgo session %s dir=%s\n\n", d.sessionID, wd)
			}
		}
		d.journalFile = f
	}
	return d.journalFile
}

// journalDir extracts the working directory recorded in a journal's header
// (ADR 0070), or "" for a journal written before headers existed.
func journalDir(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for i := 0; sc.Scan() && i < 5; i++ { // the header is line 1
		line := sc.Text()
		if idx := strings.Index(line, " dir="); strings.HasPrefix(line, ";;") && idx >= 0 {
			return strings.TrimSpace(line[idx+len(" dir="):])
		}
	}
	return ""
}

// prettyPath abbreviates $HOME to ~ for display.
func prettyPath(p string) string {
	if p == "" {
		return "(unknown)"
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if p == home {
			return "~"
		}
		if strings.HasPrefix(p, home+string(os.PathSeparator)) {
			return "~" + p[len(home):]
		}
	}
	return p
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
		switch len(fields) {
		case 1:
			d.listSessions() // no id → show the table (same as :sessions)
		case 2:
			d.resumeSession(fields[1]) // an id or a listing index
		default:
			d.reportError(errors.New("usage: :resume [<#-or-id>]"))
		}
		return true
	}
	return false
}

// ListSessions is the exported entry point for `cljgo repl :resume` /
// `:sessions` with no id — print the saved-session table and return.
func (d *Driver) ListSessions() { d.listSessions() }

// sessionIDs returns every saved session id ordered NEWEST-FIRST by
// last-active (file mtime) — the order the listing prints and the order
// `:resume <#>` indexes, so what you see is what you resume. mtime beats
// id-sorting because two sessions in the same second get random id
// suffixes; recency is the honest "newest".
func sessionIDs() []string {
	entries, _ := os.ReadDir(sessionsDir())
	type sess struct {
		id  string
		mod time.Time
	}
	var ss []sess
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".journal") {
			continue
		}
		var mod time.Time
		if info, err := e.Info(); err == nil {
			mod = info.ModTime()
		}
		ss = append(ss, sess{strings.TrimSuffix(e.Name(), ".journal"), mod})
	}
	sort.Slice(ss, func(i, j int) bool {
		if ss[i].mod.Equal(ss[j].mod) {
			return ss[i].id > ss[j].id // stable tiebreak: newer id first
		}
		return ss[i].mod.After(ss[j].mod)
	})
	ids := make([]string, len(ss))
	for i, s := range ss {
		ids[i] = s.id
	}
	return ids
}

// resolveSessionRef turns a user reference into a session id: a small
// all-digit ref (1..9999) is a 1-based index into the newest-first list the
// listing prints; anything else is treated as a literal id. Returns "" when
// an index is out of range (a literal id is returned as-is and validated by
// the caller opening its journal).
func resolveSessionRef(ref string) string {
	if n, err := strconv.Atoi(ref); err == nil && len(ref) <= 4 {
		ids := sessionIDs() // newest-first
		if n >= 1 && n <= len(ids) {
			return ids[n-1]
		}
		return ""
	}
	return ref
}

// listSessions prints a numbered, newest-first table — index, id, folder,
// last-active, form count — plus a resume hint. The index means you resume
// with `:resume 1` instead of copying a long id (ADR 0070 UX).
func (d *Driver) listSessions() {
	ids := sessionIDs()
	d.outMu.Lock()
	defer d.outMu.Unlock()
	if len(ids) == 0 {
		fmt.Fprintln(d.out, "no saved sessions yet — start a REPL and define something.")
		return
	}
	fmt.Fprintln(d.out, "sessions (newest first):")
	fmt.Fprintf(d.out, "  %-3s %-22s %-30s %-16s %s\n", "#", "id", "folder", "last active", "forms")
	for i, id := range ids {
		path := d.journalPath(id)
		lastActive := "?"
		if fi, err := os.Stat(path); err == nil {
			lastActive = fi.ModTime().Format("2006-01-02 15:04")
		}
		fmt.Fprintf(d.out, "  %-3d %-22s %-30s %-16s %d\n",
			i+1, id, prettyPath(journalDir(path)), lastActive, d.countJournalForms(path))
	}
	fmt.Fprintln(d.out, "resume with:  cljgo repl :resume <#>   (or the id)")
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
func (d *Driver) resumeSession(ref string) {
	id := resolveSessionRef(ref)
	if id == "" {
		d.reportError(fmt.Errorf("no session %q — run `cljgo repl :resume` to list them", ref))
		return
	}
	path := d.journalPath(id)
	data, err := os.ReadFile(path)
	if err != nil {
		d.reportError(fmt.Errorf("no session %s (%v)", id, err))
		return
	}
	// "Come back as it is" (ADR 0070): cd into the folder the session was
	// started in BEFORE replay, so requires/loads and relative paths resolve
	// exactly as they did. Done before reading forms; a missing folder is a
	// note, not a failure (the vars still replay).
	dir := journalDir(path)
	movedTo := ""
	if dir != "" {
		if err := os.Chdir(dir); err == nil {
			movedTo = dir
		} else {
			d.outMu.Lock()
			fmt.Fprintf(d.out, "note: session folder %s is gone (%v); resuming in place.\n", prettyPath(dir), err)
			d.outMu.Unlock()
		}
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
	if movedTo != "" {
		fmt.Fprintf(d.out, " — in %s", prettyPath(movedTo))
	}
	fmt.Fprintln(d.out)
	fmt.Fprintln(d.out, "note: running goroutines, open channels and native handles do not survive resume; re-run the forms that created them.")
	d.outMu.Unlock()
}
