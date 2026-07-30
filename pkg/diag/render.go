package diag

import (
	"fmt"
	"strings"
)

// Render turns a Diagnostic into the "lighter detailed" human line the
// owner asked for (spike s28 / ADR 0048): the message, enriched inline
// with expected-vs-found and a source locus when known, followed by
// `help:` lines (applicable fixes, then the explain pointer). It is the
// ONE renderer every context calls — REPL, `cljgo run`, compiled binaries
// via the emitted recover(), and (later) nREPL — which is how "read the
// same error everywhere" is achieved by construction.
//
// It carries NO source snippet and NO caret span: the owner rescoped s28
// away from Rust's full block to a single richer line. Every enrichment is
// optional and degrades gracefully, so a bare uncoded/unlocated error
// renders as just its message — nothing regresses.
//
// The returned string has no "error: " prefix and no trailing newline; the
// caller owns those (all three contexts already print `error: %s\n`).
func Render(d Diagnostic) string {
	d = Normalize(d)

	var b strings.Builder
	b.WriteString(d.Message)

	// expected-vs-found, inline. The arg COUNT is usually already in the
	// message ("wrong number of args (3)"), so Found is optional: with only
	// Expected we render "(expects 1: [x])"; with both, "(expects X, got Y)".
	switch {
	case d.Expected != "" && d.Found != "":
		fmt.Fprintf(&b, " (expects %s, got %s)", d.Expected, d.Found)
	case d.Expected != "":
		fmt.Fprintf(&b, " (expects %s)", d.Expected)
	case d.Found != "":
		fmt.Fprintf(&b, " (got %s)", d.Found)
	}

	// locus, when the diagnostic is positioned.
	if loc := d.Location; loc.Line > 0 {
		if loc.File != "" {
			fmt.Fprintf(&b, " at %s:%d:%d", loc.File, loc.Line, loc.Column)
		} else {
			fmt.Fprintf(&b, " at line %d:%d", loc.Line, loc.Column)
		}
	}

	// The wrapped diagnostics, innermost context first. A cause whose text
	// the wrapper already quoted carries no Message (Normalize drops it), so
	// nothing is said twice.
	for _, c := range d.Causes {
		if c.Message == "" {
			continue
		}
		fmt.Fprintf(&b, "\nnote: caused by: %s", c.Message)
		if c.Location.Line > 0 && c.Location.File != "" {
			fmt.Fprintf(&b, " at %s:%d:%d", c.Location.File, c.Location.Line, c.Location.Column)
		}
	}

	// help: applicable fixes (did-you-mean etc.), then the explain pointer.
	for _, f := range d.Fixes {
		if f.Title != "" {
			fmt.Fprintf(&b, "\nhelp: %s", f.Title)
		}
	}
	for _, r := range d.Related {
		if r.Message != "" {
			if r.Location.Line > 0 && r.Location.File != "" {
				fmt.Fprintf(&b, "\nnote: %s at %s:%d:%d", r.Message, r.Location.File, r.Location.Line, r.Location.Column)
			} else {
				fmt.Fprintf(&b, "\nnote: %s", r.Message)
			}
		}
	}
	// EXACTLY ONE explain pointer, even when several codes are in play. The
	// innermost registered code wins: the outer code names the CATEGORY the
	// failure falls into, the inner one names what actually went wrong, and
	// the inner page is the one that helps. The outer diagnostic's substance
	// is not lost — its message, notes and fixes are all still rendered above,
	// and the JSON envelope keeps every code in the chain.
	if code := explainCode(d); code != "" {
		fmt.Fprintf(&b, "\nhelp: run `cljgo explain %s`", code)
	}
	return b.String()
}

// explainCode picks the one code whose explain page a human should read:
// the innermost REGISTERED code in the cause chain, falling back outward.
func explainCode(d Diagnostic) string {
	for i := len(d.Causes) - 1; i >= 0; i-- {
		if c := explainCode(d.Causes[i]); c != "" {
			return c
		}
	}
	if d.ErrorCode != "" {
		if _, ok := Lookup(d.ErrorCode); ok {
			return d.ErrorCode
		}
	}
	return ""
}

// RenderError is the convenience one-liner every non-REPL caller uses: map
// an arbitrary error into a Diagnostic (picking up any Carrier-supplied
// span/name detail) and render it. The REPL adds did-you-mean on top before
// rendering, so it builds the Diagnostic itself.
func RenderError(err error) string {
	if err == nil {
		return ""
	}
	return Render(FromError(err))
}
