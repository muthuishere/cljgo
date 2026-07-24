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
)

// stdinReader is a single shared buffered reader over os.Stdin: successive
// prompts must not each construct their own reader (that would drop bytes
// buffered past a line boundary).
var stdinReader = bufio.NewReader(os.Stdin)

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
