// bespoke — the from-scratch pure-Go candidate for bri.cli's interactive
// backend (spike s46): stdin + golang.org/x/term (pure Go) + ANSI, the
// minimum that could serve the unified model's widgets. This is a HONEST
// minimal implementation — enough to judge the effort and quality gap vs the
// Charm stack, including a real arrow-key select (the part that needs raw
// mode + escape-sequence parsing + redraw).
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

// promptText reads a line, re-asking while validate fails (:string param).
func promptText(title string, validate func(string) error) (string, error) {
	r := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprintf(os.Stderr, "%s ", title)
		line, err := r.ReadString('\n')
		if err != nil {
			return "", err
		}
		v := strings.TrimRight(line, "\r\n")
		if validate == nil {
			return v, nil
		}
		if verr := validate(v); verr != nil {
			fmt.Fprintf(os.Stderr, "  %v\n", verr)
			continue
		}
		return v, nil
	}
}

// promptPassword reads without echo (:secret param).
func promptPassword(title string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s ", title)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	return string(b), err
}

// promptConfirm is a y/n (:bool param).
func promptConfirm(title string) (bool, error) {
	v, err := promptText(title+" [y/N]", nil)
	if err != nil {
		return false, err
	}
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "y" || v == "yes", nil
}

// promptSelect is the hard one: an arrow-key menu. It needs raw mode, manual
// escape-sequence parsing, and ANSI redraw — everything a TUI framework gives
// for free. This is the minimum viable version (no filtering, no paging, no
// theming). (:enum/:one-of param.)
func promptSelect(title string, opts []string) (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		// Non-TTY fallback: numbered prompt (scriptable).
		for i, o := range opts {
			fmt.Fprintf(os.Stderr, "  %d) %s\n", i+1, o)
		}
		v, err := promptText(title+" [1-"+fmt.Sprint(len(opts))+"]", nil)
		if err != nil {
			return "", err
		}
		var n int
		fmt.Sscanf(v, "%d", &n)
		if n < 1 || n > len(opts) {
			return "", fmt.Errorf("out of range")
		}
		return opts[n-1], nil
	}
	old, err := term.MakeRaw(fd)
	if err != nil {
		return "", err
	}
	defer term.Restore(fd, old)

	cur := 0
	draw := func() {
		fmt.Fprintf(os.Stderr, "%s\r\n", title)
		for i, o := range opts {
			cursor := "  "
			if i == cur {
				cursor = "> "
			}
			fmt.Fprintf(os.Stderr, "%s%s\r\n", cursor, o)
		}
	}
	up := func() { fmt.Fprintf(os.Stderr, "\033[%dA", len(opts)+1) }
	draw()
	buf := make([]byte, 3)
	for {
		n, _ := os.Stdin.Read(buf)
		switch {
		case n == 1 && (buf[0] == '\r' || buf[0] == '\n'):
			return opts[cur], nil
		case n == 1 && (buf[0] == 3 || buf[0] == 'q'): // ctrl-c / q
			return "", fmt.Errorf("cancelled")
		case n == 3 && buf[0] == 27 && buf[1] == '[':
			switch buf[2] {
			case 'A': // up
				if cur > 0 {
					cur--
				}
			case 'B': // down
				if cur < len(opts)-1 {
					cur++
				}
			}
		case n == 1 && buf[0] == 'k':
			if cur > 0 {
				cur--
			}
		case n == 1 && buf[0] == 'j':
			if cur < len(opts)-1 {
				cur++
			}
		}
		up()
		draw()
	}
}

// spinner prints frames until stop() (ADR 0078 §4). Minimal; no TTY-awareness
// beyond the caller's choice.
func spinner(title string) (stop func()) {
	done := make(chan struct{})
	go func() {
		frames := []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}
		i := 0
		for {
			select {
			case <-done:
				fmt.Fprintf(os.Stderr, "\r\033[K")
				return
			default:
				fmt.Fprintf(os.Stderr, "\r%c %s", frames[i%len(frames)], title)
				i++
				time.Sleep(80 * time.Millisecond)
			}
		}
	}()
	return func() { close(done) }
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--measure" {
		_ = promptText
		_ = promptPassword
		_ = promptConfirm
		_ = promptSelect
		_ = spinner
		fmt.Println("bespoke: all widgets linked (measure mode)")
		return
	}
	name, _ := promptText("Project name?", func(s string) error {
		if len(s) < 2 {
			return fmt.Errorf("must be at least 2 characters")
		}
		return nil
	})
	kind, _ := promptSelect("Template?", []string{"lib", "cli", "web"})
	tok, _ := promptPassword("API token?")
	yes, _ := promptConfirm("Proceed?")
	stop := spinner("Building…")
	time.Sleep(300 * time.Millisecond)
	stop()
	fmt.Printf("name=%s kind=%s toklen=%d yes=%v\n", name, kind, len(tok), yes)
}
