// cli.go — bri.cli's interactive half: the thin per-host terminal primitive
// behind the unified parameter model (ADR 0078 §the-other-half, s47 VERDICT
// step 1). A parameter is declared once; when a value is not supplied on the
// command line and stdin is a TTY, bri.cli PROMPTS for it — running the SAME
// validators as the flag path. This file is that primitive and nothing more:
//
//	-tty?        is stdin an interactive terminal
//	-read-input  print a prompt to stderr, read one line (cooked, echoed)
//	-read-secret print a prompt, read one line with echo OFF (passwords)
//
// All the prompt POLICY (which param prompts, re-prompt-on-invalid, :env
// precedence) lives in the portable Clojure half (bri/cli.cljg) so it runs
// byte-identically interpreted and AOT-compiled; only these three terminal
// operations are host-specific. Built on golang.org/x/term — pure Go, so
// CGO_ENABLED=0 and `cljgo dist` cross-compile hold (ADR 0077/0084). The
// `-getenv` shim is shared with bri.web.http (http.go).
package bri

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// stdinReader is a single shared buffered reader over os.Stdin: successive
// prompts must not each construct their own reader (that would drop bytes
// buffered past a line boundary).
var stdinReader = bufio.NewReader(os.Stdin)

// rawState holds the saved terminal state between -raw-enter and -raw-exit.
// A widget session brackets its interactive loop with these; the diff
// renderer + key decoding + widget logic above it are all portable Clojure.
var rawState *term.State

// installCLIShims interns bri.cli's private terminal primitives.
func installCLIShims(def func(name string, fn func(args ...any) any)) {
	def("-getenv", getenvShim)
	def("-tty?", func(args ...any) any { return term.IsTerminal(int(os.Stdin.Fd())) })
	def("-read-input", func(args ...any) any {
		return readInput(asString(one("-read-input", args)))
	})
	def("-read-secret", func(args ...any) any {
		return readSecret(asString(one("-read-secret", args)))
	})

	// --- raw-mode primitives for the Elm-loop widgets (s47) ---
	// -raw-enter: raw mode + alt-screen + hide cursor (returns nil).
	def("-raw-enter", func(args ...any) any {
		st, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			panic(fmt.Errorf("cannot enter raw mode: %w", err))
		}
		rawState = st
		fmt.Fprint(os.Stdout, "\033[?1049h\033[?25l")
		return nil
	})
	// -raw-exit: show cursor, leave alt-screen, restore cooked mode.
	def("-raw-exit", func(args ...any) any {
		fmt.Fprint(os.Stdout, "\033[?25h\033[?1049l")
		if rawState != nil {
			term.Restore(int(os.Stdin.Fd()), rawState)
			rawState = nil
		}
		return nil
	})
	// -read-key: block for one keypress, returning the raw bytes as a vector
	// of ints (0..255). Decoding bytes -> key event is portable Clojure
	// (decode-key), so the JVM host reuses it unchanged (s47 VERDICT).
	def("-read-key", func(args ...any) any {
		var b [8]byte
		n, _ := os.Stdin.Read(b[:])
		out := make([]any, n)
		for i := 0; i < n; i++ {
			out[i] = int64(b[i])
		}
		return lang.NewVectorOwning(out)
	})
	// -raw-write: write a string to stdout with NO added newline (the ANSI
	// paint stream). Raw mode is on, so \n alone would not return the cursor.
	def("-raw-write", func(args ...any) any {
		fmt.Fprint(os.Stdout, asString(one("-raw-write", args)))
		return nil
	})
}

// readInput writes the prompt to stderr (so stdout stays clean for piped
// output) and reads one line from stdin, trimming the trailing newline. EOF
// with no bytes yields "" — the caller treats empty as "no value given".
func readInput(prompt string) string {
	fmt.Fprint(os.Stderr, prompt)
	line, err := stdinReader.ReadString('\n')
	if err != nil && line == "" {
		return ""
	}
	return strings.TrimRight(line, "\r\n")
}

// readSecret reads one line with terminal echo disabled (passwords/API keys),
// then emits a newline to stderr since the user's Enter was not echoed. If
// stdin is not a real terminal (e.g. a pipe) it falls back to a plain line
// read so scripted input still works.
func readSecret(prompt string) string {
	fmt.Fprint(os.Stderr, prompt)
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return readInputRaw()
	}
	b, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return ""
	}
	return string(b)
}

// readInputRaw reads a line from the shared reader without printing a prompt
// (readSecret already printed one) — the non-TTY password fallback.
func readInputRaw() string {
	line, err := stdinReader.ReadString('\n')
	if err != nil && line == "" {
		return ""
	}
	return strings.TrimRight(line, "\r\n")
}
