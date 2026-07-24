// s47 — native TUI fundamentals: a minimal Elm-architecture runtime built
// from scratch (no Charm), to test whether bri.cli can own a small, fast,
// PORTABLE terminal UI that scales from a prompt up to an editor (opencode is
// itself Bubble Tea + the Elm loop, so "opencode-class" == owning this loop).
//
// The fundamentals, and ALL a TUI framework really is:
//   1. a terminal PRIMITIVE — raw mode + read keys + write ANSI (the only
//      platform-specific part; here golang.org/x/term, pure Go). In the real
//      thing this is a ~1-screen shim: cljgo → Go x/term, JVM Clojure → JLine.
//   2. an INPUT event stream — bytes → Key events (arrows, enter, ctrl, runes).
//   3. the ELM LOOP — Model + Update(model,msg)->model + View(model)->string.
//   4. a DIFF RENDERER — only repaint changed lines (flicker-free, cheap).
//   5. WIDGETS as plain (model,update,view) values composed into the app.
//
// Everything except (1) is portable logic — in the real bri.cli it is written
// in Clojure and runs on cljgo AND the JVM. This Go prototype proves the
// mechanics work pure-Go and measures the size.
package main

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// --- 1. terminal primitive (the only platform-specific surface) -------------

type tty struct{ fd int; old *term.State }

func openTTY() (*tty, error) {
	fd := int(os.Stdin.Fd())
	old, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	fmt.Fprint(os.Stdout, "\033[?1049h\033[?25l") // alt-screen, hide cursor
	return &tty{fd, old}, nil
}
func (t *tty) close() {
	fmt.Fprint(os.Stdout, "\033[?25h\033[?1049l") // show cursor, leave alt-screen
	term.Restore(t.fd, t.old)
}
func (t *tty) readKey() Key { // blocking; bytes -> Key
	var b [4]byte
	n, _ := os.Stdin.Read(b[:])
	return decodeKey(b[:n])
}

// --- 2. input events --------------------------------------------------------

type KeyType int

const (
	KeyRune KeyType = iota
	KeyEnter
	KeyEsc
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyBackspace
	KeyCtrlC
	KeyTab
)

type Key struct {
	Type KeyType
	Rune rune
}

func decodeKey(b []byte) Key {
	switch {
	case len(b) == 0:
		return Key{Type: KeyEsc}
	case len(b) == 3 && b[0] == 27 && b[1] == '[':
		switch b[2] {
		case 'A':
			return Key{Type: KeyUp}
		case 'B':
			return Key{Type: KeyDown}
		case 'C':
			return Key{Type: KeyRight}
		case 'D':
			return Key{Type: KeyLeft}
		}
	case len(b) == 1:
		switch b[0] {
		case 13, 10:
			return Key{Type: KeyEnter}
		case 27:
			return Key{Type: KeyEsc}
		case 3:
			return Key{Type: KeyCtrlC}
		case 9:
			return Key{Type: KeyTab}
		case 127, 8:
			return Key{Type: KeyBackspace}
		default:
			if b[0] >= 32 {
				return Key{Type: KeyRune, Rune: rune(b[0])}
			}
		}
	}
	return Key{Type: KeyEsc}
}

// --- 3. the Elm loop + 4. diff renderer -------------------------------------

// Model is any app/widget: initial view, a key handler that returns the next
// model + whether it's done, and a render to lines. This 3-fn shape is the
// whole contract — the same one Bubble Tea/Elm use.
type Model interface {
	Update(Key) (Model, bool) // next model, done?
	View() []string           // lines to paint
}

// run drives a Model through the Elm loop with a line-diff renderer.
func run(t *tty, m Model) Model {
	prev := []string{}
	paint := func(lines []string) {
		var sb strings.Builder
		for i, ln := range lines {
			if i < len(prev) && prev[i] == ln {
				continue // unchanged line — skip (the diff: cheap, flicker-free)
			}
			fmt.Fprintf(&sb, "\033[%d;1H\033[K%s", i+1, ln) // move, clear, write
		}
		for i := len(lines); i < len(prev); i++ {
			fmt.Fprintf(&sb, "\033[%d;1H\033[K", i+1) // clear removed trailing lines
		}
		os.Stdout.WriteString(sb.String())
		prev = lines
	}
	paint(m.View())
	for {
		k := t.readKey()
		next, done := m.Update(k)
		m = next
		paint(m.View())
		if done {
			return m
		}
	}
}

// --- 5. widgets built ON the fundamentals -----------------------------------

// selectModel: an arrow-key list (the bri.cli :enum widget). ~25 lines.
type selectModel struct {
	title string
	opts  []string
	cur   int
}

func (s selectModel) Update(k Key) (Model, bool) {
	switch k.Type {
	case KeyUp:
		if s.cur > 0 {
			s.cur--
		}
	case KeyDown:
		if s.cur < len(s.opts)-1 {
			s.cur++
		}
	case KeyEnter:
		return s, true
	case KeyCtrlC, KeyEsc:
		s.cur = -1
		return s, true
	}
	return s, false
}
func (s selectModel) View() []string {
	out := []string{"\033[1m" + s.title + "\033[0m"}
	for i, o := range s.opts {
		p := "  "
		if i == s.cur {
			p = "\033[36m> \033[0m" // cyan cursor
		}
		out = append(out, p+o)
	}
	return append(out, "", "\033[2m↑/↓ move · enter select · esc cancel\033[0m")
}

// editorModel: a minimal multi-line text buffer with a cursor — the proof that
// the SAME fundamentals reach an "editor like opencode". ~40 lines. Not a full
// editor, but every primitive (buffer, cursor, insert, delete, newline,
// arrow-nav) an opencode-class editor is built from.
type editorModel struct {
	title string
	lines []string
	cx    int // cursor col
	cy    int // cursor row
}

func (e editorModel) Update(k Key) (Model, bool) {
	if len(e.lines) == 0 {
		e.lines = []string{""}
	}
	line := []rune(e.lines[e.cy])
	switch k.Type {
	case KeyRune:
		line = append(line[:e.cx], append([]rune{k.Rune}, line[e.cx:]...)...)
		e.lines[e.cy] = string(line)
		e.cx++
	case KeyBackspace:
		if e.cx > 0 {
			line = append(line[:e.cx-1], line[e.cx:]...)
			e.lines[e.cy] = string(line)
			e.cx--
		} else if e.cy > 0 {
			prev := []rune(e.lines[e.cy-1])
			e.cx = len(prev)
			e.lines[e.cy-1] = string(prev) + string(line)
			e.lines = append(e.lines[:e.cy], e.lines[e.cy+1:]...)
			e.cy--
		}
	case KeyEnter:
		rest := string(line[e.cx:])
		e.lines[e.cy] = string(line[:e.cx])
		e.lines = append(e.lines[:e.cy+1], append([]string{rest}, e.lines[e.cy+1:]...)...)
		e.cy++
		e.cx = 0
	case KeyLeft:
		if e.cx > 0 {
			e.cx--
		}
	case KeyRight:
		if e.cx < len(line) {
			e.cx++
		}
	case KeyUp:
		if e.cy > 0 {
			e.cy--
			e.cx = min(e.cx, len([]rune(e.lines[e.cy])))
		}
	case KeyDown:
		if e.cy < len(e.lines)-1 {
			e.cy++
			e.cx = min(e.cx, len([]rune(e.lines[e.cy])))
		}
	case KeyEsc, KeyCtrlC:
		return e, true
	}
	return e, false
}
func (e editorModel) View() []string {
	out := []string{"\033[1m" + e.title + "\033[0m", ""}
	for y, ln := range e.lines {
		if y == e.cy { // render the cursor as a reverse-video cell
			r := []rune(ln)
			for len(r) <= e.cx {
				r = append(r, ' ')
			}
			ln = string(r[:e.cx]) + "\033[7m" + string(r[e.cx]) + "\033[0m" + string(r[e.cx+1:])
		}
		out = append(out, ln)
	}
	return append(out, "", "\033[2mtype · arrows move · esc done\033[0m")
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--measure" {
		_ = decodeKey
		_ = run
		var _ Model = selectModel{}
		var _ Model = editorModel{}
		fmt.Println("s47 runtime: Elm loop + diff renderer + select + editor linked")
		return
	}
	t, err := openTTY()
	if err != nil {
		fmt.Println("needs a TTY:", err)
		return
	}
	defer t.close()

	kind := run(t, selectModel{title: "Pick a template", opts: []string{"lib", "cli", "web"}})
	ed := run(t, editorModel{title: "Notes (proves an editor is the same loop):", lines: []string{""}})
	t.close()
	sm := kind.(selectModel)
	em := ed.(editorModel)
	if sm.cur >= 0 {
		fmt.Printf("template=%s\nnotes:\n%s\n", sm.opts[sm.cur], strings.Join(em.lines, "\n"))
	}
}
