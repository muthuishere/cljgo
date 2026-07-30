package diag

import (
	"encoding/json"
	"strings"
	"testing"
)

// The composed-diagnostic defect, reproduced from a real project-mode build
// against hiccup/hiccup 1.0.5: a G5020 wrapper ("this maven namespace failed
// to compile") wrapping an ALREADY fully-rendered I4002 printed the same
// `note:` twice, two `help: run cljgo explain …` pointers, and spliced the
// outer's "(expects …, got …)" into the middle of the inner's help line.
//
// Both composition shapes are covered: STRUCTURAL (the wrapper carries the
// inner Diagnostic in Causes) and TEXTUAL (the older `%v`-on-a-DiagError
// pattern, where the inner diagnostic's rendered text lands in the outer's
// message).
func TestRenderComposedDiagnostic(t *testing.T) {
	inner := Diagnostic{
		ErrorCode: "I4002",
		Severity:  SeverityError,
		Message:   "namespace hiccup.compiler requires Java interop and cannot load on cljgo — (ns … (:import …)) — JVM-only ns clause",
		Location:  Location{File: "hiccup/compiler.clj", Line: 4, Column: 1},
		Fixes:     []Fix{{Title: "6 other namespaces are usable in hiccup/hiccup 1.0.5"}},
		Related:   []Related{{Message: "it came from the maven dependency hiccup/hiccup 1.0.5"}},
	}
	outerNotes := []Related{
		// The SAME note the inner already carries — this is the duplicate.
		{Message: "it came from the maven dependency hiccup/hiccup 1.0.5"},
		{Message: "the resolve report's interop-free count is a read-time measurement"},
		{Message: "this is a gap in cljgo, not evidence that the library is JVM-only"},
	}
	outerFixes := []Fix{{Title: "report it with the namespace and the error above"}}

	structural := Diagnostic{
		ErrorCode: "G5020",
		Severity:  SeverityError,
		Message:   "namespace hiccup.core came from a maven dependency and failed to compile on cljgo — " + inner.Message,
		Fixes:     outerFixes,
		Related:   outerNotes,
		Causes:    []Diagnostic{inner},
	}
	textual := Diagnostic{
		ErrorCode: "G5020",
		Severity:  SeverityError,
		// What `fmt.Sprintf("… %v", innerDiagError)` produces: the inner
		// diagnostic's whole RENDERED block inside the outer message.
		Message:  "namespace hiccup.core came from a maven dependency and failed to compile on cljgo — " + Render(inner),
		Expected: "interop-free at resolve",
		Found:    "it does not compile on cljgo",
		Fixes:    outerFixes,
		Related:  outerNotes,
	}

	for _, tc := range []struct {
		name string
		d    Diagnostic
	}{
		{"structural", structural},
		{"textual", textual},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Render(tc.d)
			lines := strings.Split(got, "\n")

			// ONE error line: everything after the first line is a note/help.
			for _, ln := range lines[1:] {
				if !strings.HasPrefix(ln, "note: ") && !strings.HasPrefix(ln, "help: ") {
					t.Errorf("stray line in a composed render: %q\n---\n%s", ln, got)
				}
			}

			// No duplicated note.
			seen := map[string]bool{}
			for _, ln := range lines {
				if !strings.HasPrefix(ln, "note: ") {
					continue
				}
				if seen[ln] {
					t.Errorf("duplicate note %q in:\n%s", ln, got)
				}
				seen[ln] = true
			}
			if n := strings.Count(got, "note: it came from the maven dependency hiccup/hiccup 1.0.5"); n != 1 {
				t.Errorf("the maven-origin note appears %d times, want 1:\n%s", n, got)
			}

			// EXACTLY ONE explain pointer, and it is the INNER, more specific
			// code — the page that explains what actually went wrong.
			if n := strings.Count(got, "help: run `cljgo explain "); n != 1 {
				t.Errorf("%d explain pointers, want exactly 1:\n%s", n, got)
			}
			if !strings.Contains(got, "help: run `cljgo explain I4002`") {
				t.Errorf("explain pointer is not the inner code I4002:\n%s", got)
			}
			if !strings.HasSuffix(got, "help: run `cljgo explain I4002`") {
				t.Errorf("the explain pointer must be the last line:\n%s", got)
			}

			// The G5020 point survives dedup — this is honesty, not noise.
			for _, must := range []string{
				"note: the resolve report's interop-free count is a read-time measurement",
				"note: this is a gap in cljgo, not evidence that the library is JVM-only",
				"help: report it with the namespace and the error above",
			} {
				if !strings.Contains(got, must) {
					t.Errorf("composition lost %q:\n%s", must, got)
				}
			}

			// No expected/found fragment spliced into a help line.
			for _, ln := range lines[1:] {
				if strings.Contains(ln, "(expects ") {
					t.Errorf("expected/found spliced into a trailing line: %q\n%s", ln, got)
				}
			}

			// The --json envelope keeps BOTH codes as structured data.
			b, err := json.Marshal(NewEnvelope([]Diagnostic{tc.d}))
			if err != nil {
				t.Fatal(err)
			}
			var env struct {
				Diagnostics []struct {
					ErrorCode string `json:"error_code"`
					Causes    []struct {
						ErrorCode string `json:"error_code"`
					} `json:"causes"`
				} `json:"diagnostics"`
			}
			if err := json.Unmarshal(b, &env); err != nil {
				t.Fatal(err)
			}
			d0 := env.Diagnostics[0]
			if d0.ErrorCode != "G5020" {
				t.Errorf("envelope lost the outer code: %q", d0.ErrorCode)
			}
			if len(d0.Causes) != 1 || d0.Causes[0].ErrorCode != "I4002" {
				t.Errorf("envelope does not carry the inner code I4002: %s", b)
			}
		})
	}
}

// Normalize must be idempotent and must leave an uncomposed diagnostic
// byte-identical — every existing error line is frozen text.
func TestNormalizeIdempotentAndInert(t *testing.T) {
	d := Diagnostic{
		ErrorCode: "A2001",
		Severity:  SeverityError,
		Message:   "unable to resolve symbol: pritnln in this context",
		Location:  Location{File: "demo.clj", Line: 1, Column: 1},
		Fixes:     []Fix{{Title: "did you mean println?", Replacement: "println"}},
	}
	if got, want := Render(Normalize(d)), Render(d); got != want {
		t.Fatalf("Normalize is not idempotent under Render:\n got %q\nwant %q", got, want)
	}
	n := Normalize(d)
	if n.Message != d.Message || n.ErrorCode != d.ErrorCode || len(n.Causes) != 0 {
		t.Fatalf("Normalize disturbed an uncomposed diagnostic: %+v", n)
	}
}
