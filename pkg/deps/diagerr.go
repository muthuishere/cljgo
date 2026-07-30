package deps

// Coded dependency-resolution errors (CLAUDE.md error doctrine): name the
// thing, locate it, expected-vs-found, a registered code from the append-only
// registry, suggestions as Fixes rather than prose. Every error on the Maven
// consume path is one of these — no bare fmt.Errorf, and no raw Go panic,
// including on the network path where a DNS failure, a TLS error, a 500 and a
// truncated zip each map to a named diagnostic.

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/diag"
)

// DiagError is an error carrying a fully-formed Diagnostic computed at the
// raise site. It implements diag.Carrier, so diag.RenderError picks up the
// code, the locus and the fixes in every context — the REPL, `cljgo run`, a
// compiled binary, and the --json envelope — without parsing prose.
type DiagError struct{ D diag.Diagnostic }

func (e *DiagError) Error() string { return diag.Render(e.D) }

// Diagnostic implements diag.Carrier.
func (e *DiagError) Diagnostic() (diag.Diagnostic, bool) { return e.D, true }

// Code returns the diagnostic's registered error code.
func (e *DiagError) Code() string { return e.D.ErrorCode }

// codedf builds a DiagError with a registered code and a formatted message.
func codedf(code, format string, args ...any) *DiagError {
	return &DiagError{D: diag.Diagnostic{
		ErrorCode: code,
		Severity:  diag.SeverityError,
		Message:   fmt.Sprintf(format, args...),
	}}
}

// withFix appends a `help:` suggestion.
func (e *DiagError) withFix(title string) *DiagError {
	e.D.Fixes = append(e.D.Fixes, diag.Fix{Title: title})
	return e
}

// withExpectedFound sets the expected-vs-found pair.
func (e *DiagError) withExpectedFound(expected, found string) *DiagError {
	e.D.Expected, e.D.Found = expected, found
	return e
}

// at sets the source locus.
func (e *DiagError) at(file string, line, col int) *DiagError {
	e.D.Location = diag.Location{File: file, Line: line, Column: col}
	return e
}

// note appends a `note:` related line.
func (e *DiagError) note(msg string) *DiagError {
	e.D.Related = append(e.D.Related, diag.Related{Message: msg})
	return e
}

// ErrCode reports the registered code an error carries, or "".
func ErrCode(err error) string {
	if de, ok := err.(*DiagError); ok {
		return de.Code()
	}
	return ""
}
