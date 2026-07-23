package lang

import (
	"fmt"
	"strings"
	"sync"
	"unique"
)

// Keyword represents a keyword. Syntactically, a keyword is a symbol
// that starts with a colon and evaluates to itself.
//
// cljgo S4 surgery: interning moved from go4.org/intern to the stdlib
// unique package (Go 1.23+). unique.Handle[string] is a comparable
// one-word struct; two keywords with the same full name hold the same
// handle, so Keyword values compare equal with plain Go == in O(1),
// across packages and goroutines. The hash field is derived purely from
// the interned string, so equal names always produce identical structs
// and == on Keyword stays valid.
type Keyword struct {
	// kw is an interned string handle. This guarantees that two keywords
	// with the same name share the same canonical underlying string.
	kw   unique.Handle[string]
	hash uint32
}

var (
	_ Hasher = Keyword{}

	keywordRegistry   = make(map[string]struct{})
	keywordRegistryMu sync.RWMutex
)

func NewKeyword(s string) Keyword {
	keywordRegistryMu.Lock()
	keywordRegistry[s] = struct{}{}
	keywordRegistryMu.Unlock()

	return Keyword{
		kw:   unique.Make(s),
		hash: Hash(s) ^ keywordHashMask,
	}
}

// FindKeyword returns the keyword named s ONLY when a keyword with that
// full name has already been interned (via NewKeyword), reporting whether
// it had been. It is clojure.core/find-keyword's substrate and must never
// intern: a miss leaves the registry untouched (oracle 1.12.5:
// (find-keyword "never-interned") => nil, and stays nil on repeat calls).
func FindKeyword(s string) (Keyword, bool) {
	keywordRegistryMu.RLock()
	_, ok := keywordRegistry[s]
	keywordRegistryMu.RUnlock()
	if !ok {
		return Keyword{}, false
	}
	return NewKeyword(s), true
}

// AllKeywords returns all keyword strings that have been interned.
func AllKeywords() []string {
	keywordRegistryMu.RLock()
	defer keywordRegistryMu.RUnlock()
	result := make([]string, 0, len(keywordRegistry))
	for k := range keywordRegistry {
		result = append(result, k)
	}
	return result
}

func InternKeywordSymbol(s *Symbol) Keyword {
	return NewKeyword(s.FullName())
}

func InternKeywordString(s string) Keyword {
	return NewKeyword(s)
}

func InternKeyword(ns, name interface{}) Keyword {
	return InternKeywordSymbol(InternSymbol(ns, name))
}

func (k Keyword) value() string {
	return k.kw.Value()
}

func (k Keyword) Namespace() any {
	// Return the namespace of the keyword, or nil if it doesn't have
	// one.
	// TODO: support both nil and empty string namespace as clojure does
	if i := strings.Index(k.value(), "/"); i != -1 {
		return k.value()[:i]
	}
	return nil
}

func (k Keyword) Name() string {
	// Return the name of the keyword, or the empty string if it
	// doesn't have one.
	if i := strings.Index(k.value(), "/"); i != -1 {
		return k.value()[i+1:]
	}
	return k.value()
}

func (k Keyword) Sym() *Symbol {
	return InternSymbol(k.Namespace(), k.Name())
}

func (k Keyword) String() string {
	return ":" + k.value()
}

func (k Keyword) Equals(v interface{}) bool {
	return k == v
}

func (k Keyword) Invoke(args ...interface{}) interface{} {
	if len(args) == 0 || len(args) > 2 {
		panic(fmt.Errorf("wrong number of args (%v) passed to: %v", len(args), k))
	}
	var defaultVal interface{} = nil
	if len(args) == 2 {
		defaultVal = args[1]
	}

	return GetDefault(args[0], k, defaultVal)
}

func (k Keyword) ApplyTo(args ISeq) interface{} {
	return k.Invoke(seqToSlice(args)...)
}

func (k Keyword) Hash() uint32 {
	return k.hash
}

// HashEq is clojure.lang.Keyword.hasheq — the value clojure.core's `hash`
// and hash-map/set bucketing use. Matches JVM 1.12.5 byte-for-byte:
// the underlying symbol's hasheq plus the golden-ratio constant.
func (k Keyword) HashEq() uint32 {
	ns := ""
	if n, ok := k.Namespace().(string); ok {
		ns = n
	}
	return symbolHashEq(ns, k.Name()) + 0x9e3779b9
}

func (k Keyword) Compare(other any) int {
	if otherKw, ok := other.(Keyword); ok {
		s := k.String()
		os := otherKw.String()
		if s == os {
			return 0
		}
		ns, ok := k.Namespace().(string)
		if !ok {
			if otherKw.Namespace() != nil {
				return -1
			}
		} else {
			ons, ok := otherKw.Namespace().(string)
			if !ok {
				return 1
			}
			nsc := strings.Compare(ns, ons)
			if nsc != 0 {
				return nsc
			}
		}
		return strings.Compare(k.Name(), otherKw.Name())
	}
	panic(NewIllegalArgumentError(fmt.Sprintf("Cannot compare Keyword with %T", other)))
}
