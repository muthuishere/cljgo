package reader

// Generalized tagged literals (design/01-reader.md §Phase 2). The
// built-in tags #uuid and #inst are wired here, alongside the existing
// cljgo Result/Option tags (ADR 0014). Unknown tags fall through to a
// *data-readers*-style extension point (lang.VarDataReaders): if that
// var maps the tag symbol to a callable, it is invoked on the read
// form. This keeps the reader dumb about domain types while letting a
// program register its own readers.

import (
	"regexp"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// UUID is the value of a #uuid "..." literal. Clojure's java.util.UUID
// lowercases its canonical string form, so we store it lowercased and
// print it back as #uuid "<lower>" (oracle 1.12.5:
// (pr-str #uuid "550E8400-...") => #uuid "550e8400-...").
type UUID struct{ s string }

func (u UUID) String() string { return `#uuid "` + u.s + `"` }

// Value returns the canonical (lowercase) UUID string.
func (u UUID) Value() string { return u.s }

// Inst is the value of an #inst "..." literal. v0 preserves the literal
// timestamp text verbatim (cljgo has no Date type yet); it prints back
// as #inst "<text>". Full RFC3339 normalization to a canonical instant
// is deferred until a host time type lands.
type Inst struct{ s string }

func (i Inst) String() string { return `#inst "` + i.s + `"` }

// Value returns the instant's literal text.
func (i Inst) Value() string { return i.s }

// uuidRe matches the canonical 8-4-4-4-12 hex UUID shape.
var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// dataReaderFor consults lang.*data-readers* for a registered reader
// function bound to tag, returning it (and true) when present.
func dataReaderFor(tag *lang.Symbol) (lang.IFn, bool) {
	v := lang.VarDataReaders
	if v == nil || !v.IsBound() {
		return nil, false
	}
	m, ok := v.Deref().(lang.IPersistentMap)
	if !ok {
		return nil, false
	}
	fn, ok := m.ValAt(tag).(lang.IFn)
	if !ok {
		return nil, false
	}
	return fn, true
}
