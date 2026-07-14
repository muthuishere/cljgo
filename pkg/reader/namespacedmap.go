package reader

// Namespaced map literals (design/01-reader.md §Phase 2), a faithful
// port of clojure.lang.LispReader$NamespaceMapReader:
//
//	#:ns{:a 1 :b 2}    => {:ns/a 1 :ns/b 2}
//	#::{:a 1}          => {:current.ns/a 1}      (auto-resolve current ns)
//	#::alias{:a 1}     => {:target.ns/a 1}       (auto-resolve via alias)
//
// Per-key qualification rules (verified vs clojure 1.12.5):
//   - a bare keyword  :a       => :ns/a
//   - a bare symbol    x       => ns/x
//   - an already-qualified key (:foo/b, :bar/c) is left unchanged
//   - the "_" namespace strips the qualification (:_/d => :d, _/x => x)
//   - non-keyword / non-symbol keys (numbers, nil, strings) are unchanged
//
// The leading '#:' has already been consumed by readDispatch.

import (
	"errors"
	"fmt"

	"github.com/muthuishere/cljgo/pkg/lang"
)

func (r *Reader) readNamespacedMap(start Position) (any, error) {
	incomplete := func() error {
		return &Error{Pos: r.s.Pos(), Start: &start, Err: fmt.Errorf("%w namespaced map", ErrIncomplete)}
	}

	// A second ':' means the namespace is auto-resolved (#:: / #::alias).
	auto := false
	c, err := r.s.Read()
	if err != nil {
		return nil, incomplete()
	}
	if c == ':' {
		auto = true
	} else {
		r.s.Unread()
	}

	// Peek past whitespace: if the next form is the map itself, there is
	// no explicit namespace symbol (only valid for the auto form).
	r.skipWhitespace()
	pc, err := r.s.Read()
	if err != nil {
		return nil, incomplete()
	}
	r.s.Unread()

	var osym *lang.Symbol
	if pc != '{' {
		form, err := r.readForm()
		if err != nil {
			if errors.Is(err, ErrEOF) {
				return nil, incomplete()
			}
			return nil, err
		}
		s, ok := form.(*lang.Symbol)
		if !ok || s.HasNamespace() {
			return nil, r.errAt(start, "Namespaced map must specify a valid namespace: %s", lang.PrintString(form))
		}
		osym = s
	}

	// Resolve the namespace string.
	var ns string
	switch {
	case auto && osym == nil:
		if r.resolver == nil {
			return nil, r.errAt(start, "Namespaced map with auto-resolved namespace requires a resolver")
		}
		cur := r.resolver.CurrentNS()
		if cur == nil {
			return nil, r.errAt(start, "Namespaced map with auto-resolved namespace has no current namespace")
		}
		ns = cur.Name()
	case auto:
		if r.resolver == nil {
			return nil, r.errAt(start, "Namespaced map with auto-resolved namespace requires a resolver")
		}
		target := r.resolver.ResolveAlias(osym)
		if target == nil {
			return nil, r.errAt(start, "Unknown auto-resolved namespace alias: %s", osym.Name())
		}
		ns = target.Name()
	case osym != nil:
		ns = osym.Name()
	default:
		// #:{...} with no namespace and no auto marker.
		return nil, r.errAt(start, "Namespaced map must specify a namespace")
	}

	// The map itself.
	r.skipWhitespace()
	oc, err := r.s.Read()
	if err != nil {
		return nil, incomplete()
	}
	if oc != '{' {
		return nil, r.errAt(start, "Namespaced map must specify a map")
	}
	forms, err := r.readDelimited("namespaced map", '}', start)
	if err != nil {
		return nil, err
	}
	if len(forms)%2 != 0 {
		return nil, r.errAt(start, "Namespaced map literal must contain an even number of forms")
	}

	out := make([]any, len(forms))
	for i := 0; i < len(forms); i += 2 {
		out[i] = qualifyNamespacedKey(forms[i], ns)
		out[i+1] = forms[i+1]
	}
	// Duplicate detection runs on the qualified keys, matching Clojure
	// (#:foo{:a 1 :foo/a 2} => "Duplicate key: :foo/a").
	if err := r.checkDuplicates(start, out, 2); err != nil {
		return nil, err
	}
	return lang.NewMap(out...), nil
}

// qualifyNamespacedKey applies a namespaced map's namespace to one key,
// per LispReader$NamespaceMapReader's key rewrite.
func qualifyNamespacedKey(key any, ns string) any {
	switch k := key.(type) {
	case lang.Keyword:
		switch kns := k.Namespace().(type) {
		case nil:
			return lang.NewKeyword(ns + "/" + k.Name())
		case string:
			if kns == "_" {
				return lang.NewKeyword(k.Name())
			}
		}
		return k
	case *lang.Symbol:
		if !k.HasNamespace() {
			return lang.InternSymbol(ns, k.Name())
		}
		if k.Namespace() == "_" {
			return lang.NewSymbol(k.Name())
		}
		return k
	default:
		return key
	}
}
