package diag

import (
	"fmt"
	"sort"
	"strings"

	diagdocs "github.com/muthuishere/cljgo/docs/diagnostics"
)

// Band is a diagnostic code band per ADR 0015. Each band owns a
// numeric range so a code's origin is readable at a glance.
type Band byte

const (
	BandReader   Band = 'R' // R1xxx
	BandAnalyzer Band = 'A' // A2xxx
	BandEmitter  Band = 'E' // E3xxx
	BandInterop  Band = 'I' // I4xxx
	BandGeneral  Band = 'G' // G5xxx
)

// bandRange maps each band letter to the leading digit its codes use.
var bandRange = map[Band]byte{
	BandReader:   '1',
	BandAnalyzer: '2',
	BandEmitter:  '3',
	BandInterop:  '4',
	BandGeneral:  '5',
}

// Entry is one registered diagnostic code. The registry is the
// compile-time source of truth (design.md D2); the committed snapshot
// docs/diagnostics/registry.lock enforces append-only evolution and
// every code carries an explain page docs/diagnostics/<CODE>.md.
type Entry struct {
	Code  string // e.g. "R1001"
	Title string // short summary, locked in registry.lock
	Since string // milestone the code first shipped in
}

// Band returns the entry's band letter.
func (e Entry) Band() Band { return Band(e.Code[0]) }

// registry holds every diagnostic code ever registered, sorted by
// code. APPEND-ONLY: never remove, renumber, or retitle an entry —
// TestRegistryLock fails against docs/diagnostics/registry.lock if
// you do. To add a code: append here, regenerate the lock (see
// LockText), and write docs/diagnostics/<CODE>.md.
var registry = []Entry{
	// R1xxx — reader
	{Code: "R1001", Title: "unterminated form", Since: "M2"},
	{Code: "R1002", Title: "unmatched delimiter", Since: "M2"},
	{Code: "R1003", Title: "map literal with odd number of forms", Since: "M2"},
	{Code: "R1004", Title: "duplicate key in map or set literal", Since: "M2"},
	{Code: "R1005", Title: "invalid token", Since: "M2"},
	{Code: "R1006", Title: "invalid number literal", Since: "M2"},
	{Code: "R1007", Title: "invalid escape sequence in string", Since: "M2"},
	{Code: "R1008", Title: "invalid character literal", Since: "M2"},
	{Code: "R1009", Title: "invalid metadata", Since: "M2"},

	// A2xxx — analyzer
	{Code: "A2001", Title: "unable to resolve symbol", Since: "M2"},
	{Code: "A2002", Title: "recur outside tail position", Since: "M2"},
	{Code: "A2003", Title: "recur argument count mismatch", Since: "M2"},
	{Code: "A2004", Title: "wrong number of forms in special form", Since: "M2"},
	{Code: "A2005", Title: "def name is not a symbol", Since: "M2"},
	{Code: "A2006", Title: "malformed binding vector", Since: "M2"},
	{Code: "A2007", Title: "invalid binding form", Since: "M2"},
	{Code: "A2008", Title: "conflicting fn overloads", Since: "M2"},

	// G5xxx — general (runtime errors carry raise-site codes here, ADR 0048)
	{Code: "G5000", Title: "uncategorized compiler error", Since: "M2"},
	{Code: "G5001", Title: "value is not a number", Since: "M5"},
	{Code: "G5002", Title: "value is not a function", Since: "M5"},
	{Code: "G5003", Title: "value is not seqable", Since: "M5"},
	{Code: "G5004", Title: "index out of bounds", Since: "M5"},
	{Code: "G5005", Title: "value is not a collection", Since: "M5"},
}

// Lookup returns the registry entry for code.
func Lookup(code string) (Entry, bool) {
	for _, e := range registry {
		if e.Code == code {
			return e, true
		}
	}
	return Entry{}, false
}

// Codes returns all registered codes, sorted.
func Codes() []string {
	out := make([]string, len(registry))
	for i, e := range registry {
		out[i] = e.Code
	}
	sort.Strings(out)
	return out
}

// Entries returns a copy of the full registry.
func Entries() []Entry {
	out := make([]Entry, len(registry))
	copy(out, registry)
	return out
}

// ValidCode reports whether code is well-formed for its band:
// one band letter followed by four digits, the first digit being the
// band's assigned range (R1xxx, A2xxx, E3xxx, I4xxx, G5xxx).
func ValidCode(code string) bool {
	if len(code) != 5 {
		return false
	}
	lead, ok := bandRange[Band(code[0])]
	if !ok || code[1] != lead {
		return false
	}
	for i := 2; i < 5; i++ {
		if code[i] < '0' || code[i] > '9' {
			return false
		}
	}
	return true
}

// LockText renders the canonical registry.lock content: one
// "CODE<space>title" line per entry, sorted by code, trailing
// newline. When adding a code, write this text to
// docs/diagnostics/registry.lock (existing lines must never change).
func LockText() string {
	entries := Entries()
	sort.Slice(entries, func(i, j int) bool { return entries[i].Code < entries[j].Code })
	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "%s %s\n", e.Code, e.Title)
	}
	return b.String()
}

// LockFile reads the committed lock snapshot embedded from
// docs/diagnostics/registry.lock.
func LockFile() (string, error) {
	b, err := diagdocs.FS.ReadFile("registry.lock")
	if err != nil {
		return "", fmt.Errorf("diag: reading registry.lock: %w", err)
	}
	return string(b), nil
}

// Explain returns the long-form explain page for a registered code
// (docs/diagnostics/<CODE>.md, embedded at build time).
func Explain(code string) (string, error) {
	if _, ok := Lookup(code); !ok {
		return "", fmt.Errorf("diag: unknown error code %q", code)
	}
	b, err := diagdocs.FS.ReadFile(code + ".md")
	if err != nil {
		return "", fmt.Errorf("diag: no explain page for %s: %w", code, err)
	}
	return string(b), nil
}
