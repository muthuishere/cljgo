// Package diag implements the structured-diagnostics data model of
// ADR 0015 (docs/adr/0015-structured-diagnostics-introspection.md) as
// settled in openspec/changes/structured-diagnostics/design.md D1/D2:
// the Diagnostic value, the append-only banded error-code registry,
// Explain pages, and a best-effort adapter from today's error values.
//
// This package is the foundation slice only: no CLI wiring, no
// renderers beyond JSON marshaling. Human-readable error output is
// produced elsewhere and remains byte-for-byte unchanged.
package diag

// SchemaVersion tags every envelope; additive-only changes within /1.
const SchemaVersion = "cljgo-diag/1"

// Severity of a diagnostic (design.md D1: error | warning | note).
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityNote    Severity = "note"
)

// Location is a source span. Line and Column are 1-based rune
// positions (reader convention); EndLine/EndColumn are exclusive and
// zero when the end of the span is unknown.
type Location struct {
	File      string `json:"file"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	EndLine   int    `json:"end_line,omitempty"`
	EndColumn int    `json:"end_column,omitempty"`
}

// ByteRange is a half-open range of UTF-8 byte offsets into the exact
// source bytes the compiler read, making fixes machine-applicable
// without column arithmetic (design.md D1).
type ByteRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// Fix is a machine-applicable remedy: replacing ByteRange in the
// checked source bytes with Replacement applies the fix.
type Fix struct {
	Title       string    `json:"title"`
	Replacement string    `json:"replacement"`
	ByteRange   ByteRange `json:"byte_range"`
}

// Related is a secondary note pointing at another position, e.g.
// "unterminated form starts here" or "recur target is this loop".
type Related struct {
	Message  string   `json:"message"`
	Location Location `json:"location"`
}

// Diagnostic is the structured value behind every compiler error,
// warning, or note. Field names marshal snake_case per ADR 0015 / D1.
type Diagnostic struct {
	// ErrorCode is a stable code from the banded registry
	// (R1xxx reader, A2xxx analyzer, E3xxx emitter, I4xxx interop,
	// G5xxx general). Codes are append-only and documented.
	ErrorCode string   `json:"error_code"`
	Severity  Severity `json:"severity"`
	Message   string   `json:"message"`
	Location  Location `json:"location"`

	// Expected/Found are set where the diagnostic has a clear
	// expected-vs-actual shape.
	Expected string `json:"expected,omitempty"`
	Found    string `json:"found,omitempty"`

	Fixes   []Fix     `json:"fixes,omitempty"`
	Related []Related `json:"related,omitempty"`

	// ExplainURL points at the code's explain page
	// (docs/diagnostics/<CODE>.md) when the code is registered.
	ExplainURL string `json:"explain_url,omitempty"`

	// ID is a per-run handle for explain/suggest_fix (design.md D1);
	// unset until a check run assigns one.
	ID string `json:"id,omitempty"`
}

// Envelope is the versioned JSON container for a run's diagnostics
// (design.md D1): {"schema": "cljgo-diag/1", "diagnostics": [...]}.
type Envelope struct {
	Schema      string       `json:"schema"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// NewEnvelope wraps diagnostics in a versioned envelope. A nil slice
// marshals as an empty array, never null.
func NewEnvelope(diags []Diagnostic) Envelope {
	if diags == nil {
		diags = []Diagnostic{}
	}
	return Envelope{Schema: SchemaVersion, Diagnostics: diags}
}
