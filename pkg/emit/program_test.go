package emit

// Provenance comments: the emitter writes each top-level form as a `// (…)`
// comment above its generated Go, truncated so a long form stays one line.

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/muthuishere/cljgo/pkg/ast"
)

// A multi-byte character straddling the 90-byte truncation point used to be cut
// in half, which made the generated Go invalid UTF-8: go/format then rejected
// the whole file with "illegal UTF-8 encoding", pointing at a line in generated
// source the author never wrote. Found building koine.host, whose `tier`
// docstring happened to put an em dash right there — the namespace ran fine
// interpreted and would not compile.
func TestProvenanceTruncatesOnRuneBoundary(t *testing.T) {
	for pad := 80; pad <= 95; pad++ {
		form := strings.Repeat("x", pad) + "—tail of a docstring that keeps going"
		got := provenance(&ast.Node{Form: form})
		if !utf8.ValidString(got) {
			t.Fatalf("pad=%d: provenance produced invalid UTF-8: %q", pad, got)
		}
	}
}

// Multi-byte characters well inside the limit must survive untouched.
func TestProvenanceKeepsShortFormsIntact(t *testing.T) {
	form := "(def x \"café ☃\")"
	if got := provenance(&ast.Node{Form: form}); !strings.Contains(got, "café ☃") {
		t.Fatalf("provenance mangled a short form: %q", got)
	}
}
