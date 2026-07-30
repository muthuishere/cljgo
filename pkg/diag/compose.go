package diag

// Composition: what a user reads when TWO diagnostics are in play.
//
// A raise site may wrap an existing, already-diagnosed failure to add
// context ("this namespace came from a maven dependency and failed to
// compile on cljgo — a read-time interop-free classification is not a
// promise that it compiles"). Both layers legitimately have notes, fixes
// and a registered code, and rendering them independently produced the
// defect this file fixes: the same `note:` printed twice, two competing
// `help: run cljgo explain …` pointers, and — when the wrapper embedded the
// inner diagnostic's RENDERED text with %v — the outer's "(expects …, got …)"
// spliced into the middle of the inner's help line.
//
// The fix is at the render layer, not the raise sites (ADR 0015): Normalize
// collapses a composed diagnostic into ONE coherent value —
//
//   - a single-line Message (an inner diagnostic's rendered `note:`/`help:`
//     lines embedded in the message are lifted back out into structure);
//   - Fixes and Related deduplicated across the whole chain, first mention
//     wins, order preserved;
//   - the chain's codes kept under Causes, so `--json` carries every code
//     even though the human rendering prints one explain pointer.
//
// Nothing is dropped for honesty's sake: a wrapper's own notes ("this is a
// gap in cljgo, not evidence that the library is JVM-only") survive dedup
// untouched — only exact repeats are removed.

import "strings"

// Normalize collapses a diagnostic and everything it wraps into one
// Diagnostic: single-line message, deduplicated fixes and notes hoisted to
// the top level, and the cause chain reduced to its identifying parts
// (code, message, location). It is idempotent, and a plain single-layer
// diagnostic passes through byte-identically — Render's output for every
// existing (uncomposed) diagnostic is unchanged.
func Normalize(d Diagnostic) Diagnostic {
	out := d
	out.Causes = nil

	// An inner diagnostic that was embedded as rendered TEXT (the `%v`-on-a-
	// DiagError pattern) is parsed back into structure, so its notes, fixes
	// and code take part in dedup instead of being opaque prose.
	head, fixes, notes, codes := splitRendered(d.Message)
	out.Message = head
	out.Fixes = append(append([]Fix(nil), d.Fixes...), fixes...)
	out.Related = append(append([]Related(nil), d.Related...), notes...)
	for _, code := range codes {
		// The quoted text is already in Message, so the cause carries no
		// message of its own — the renderer must not repeat it.
		c := Diagnostic{ErrorCode: code, Severity: SeverityError}
		setExplainURL(&c)
		out.Causes = append(out.Causes, c)
	}

	// Structurally wrapped causes: hoist their fixes/notes into the merged,
	// deduplicated sets and keep only their identity in the chain.
	for _, c := range d.Causes {
		n := Normalize(c)
		out.Fixes = append(out.Fixes, n.Fixes...)
		out.Related = append(out.Related, n.Related...)
		out.Causes = append(out.Causes, n.Causes...)
		n.Fixes, n.Related, n.Causes = nil, nil, nil
		out.Causes = append(out.Causes, n)
	}

	// The wrapper often quotes its cause; say it once.
	for i := range out.Causes {
		if m := out.Causes[i].Message; m != "" && strings.Contains(out.Message, m) {
			out.Causes[i].Message = ""
		}
	}

	// A wrapper rarely has a position of its own; the failure it wraps does.
	// Inherit it so the composed line still LOCATES the problem (the error
	// doctrine's second rule) instead of losing the locus to composition.
	if out.Location.Line == 0 {
		for _, c := range out.Causes {
			if c.Location.Line > 0 {
				out.Location = c.Location
				break
			}
		}
	}

	out.Fixes = dedupFixes(out.Fixes)
	out.Related = dedupRelated(out.Related)
	out.Causes = dedupCauses(out.Causes, out.ErrorCode)
	if len(out.Fixes) == 0 {
		out.Fixes = nil
	}
	if len(out.Related) == 0 {
		out.Related = nil
	}
	if len(out.Causes) == 0 {
		out.Causes = nil
	}
	return out
}

// splitRendered pulls an already-rendered diagnostic back apart: the first
// line is the message, and any following `help:` / `note:` lines become
// Fixes / Related again, with `help: run \`cljgo explain CODE\“ recognized
// as a code rather than a suggestion. Continuation lines that are neither
// (a message that genuinely spans lines) stay part of the message. A
// single-line message yields itself and nothing else, which is why every
// uncomposed diagnostic renders byte-identically.
func splitRendered(msg string) (head string, fixes []Fix, notes []Related, codes []string) {
	if !strings.Contains(msg, "\n") {
		return msg, nil, nil, nil
	}
	lines := strings.Split(msg, "\n")
	head = lines[0]
	for _, ln := range lines[1:] {
		switch {
		case strings.HasPrefix(ln, explainPrefix) && strings.HasSuffix(ln, "`"):
			code := strings.TrimSuffix(strings.TrimPrefix(ln, explainPrefix), "`")
			if _, ok := Lookup(code); ok {
				codes = append(codes, code)
				continue
			}
			fixes = append(fixes, Fix{Title: strings.TrimPrefix(ln, "help: ")})
		case strings.HasPrefix(ln, "help: "):
			fixes = append(fixes, Fix{Title: strings.TrimPrefix(ln, "help: ")})
		case strings.HasPrefix(ln, "note: "):
			notes = append(notes, Related{Message: strings.TrimPrefix(ln, "note: ")})
		default:
			head += "\n" + ln
		}
	}
	return head, fixes, notes, codes
}

const explainPrefix = "help: run `cljgo explain "

func dedupFixes(in []Fix) []Fix {
	seen := map[Fix]bool{}
	out := in[:0]
	for _, f := range in {
		if f.Title == "" || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

func dedupRelated(in []Related) []Related {
	seen := map[Related]bool{}
	out := in[:0]
	for _, r := range in {
		if r.Message == "" || seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	return out
}

// dedupCauses keeps one entry per code (the first, which carries the most
// detail) and drops a cause that merely repeats the outer code.
func dedupCauses(in []Diagnostic, outer string) []Diagnostic {
	seen := map[string]bool{}
	if outer != "" {
		seen[outer] = true
	}
	out := in[:0]
	for _, c := range in {
		// A code already accounted for higher up adds nothing UNLESS it also
		// carries a message the reader has not seen.
		if c.ErrorCode != "" && seen[c.ErrorCode] && c.Message == "" {
			continue
		}
		if c.ErrorCode != "" {
			seen[c.ErrorCode] = true
		}
		out = append(out, c)
	}
	return out
}
