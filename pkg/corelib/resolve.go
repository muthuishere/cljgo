package corelib

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// ResolveVar resolves a symbol to a Var against the global namespace
// world — the body of the analyzer's var-resolution hook (design/03
// §3a), a free function since ADR 0043: the "current namespace" is the
// *ns* dynamic var, not evaluator state, so symbol resolution needs no
// interpreter (compiled protocol code calls it at load time via
// -type-key / -instance-of-name?).
func ResolveVar(sym *lang.Symbol) (*lang.Var, error) {
	if sym.HasNamespace() {
		nsSym := lang.NewSymbol(sym.Namespace())
		ns := currentNS().LookupAlias(nsSym)
		if ns == nil {
			ns = lang.FindNamespace(nsSym)
		}
		if ns == nil {
			return nil, fmt.Errorf("no such namespace: %s", sym.Namespace())
		}
		v := ns.FindInternedVar(lang.NewSymbol(sym.Name()))
		if v == nil {
			return nil, fmt.Errorf("no such var: %s", sym.FullName())
		}
		return v, nil
	}
	if m := currentNS().Mappings().ValAt(sym); m != nil {
		if v, ok := m.(*lang.Var); ok {
			return v, nil
		}
	}
	// Last resort (ADR 0036): well-known JVM class names (`String`,
	// `Object`, `clojure.lang.PersistentHashSet`, …) resolve to interned
	// opaque ClassRef values. Tried only after every normal lookup missed,
	// so user definitions always win; fail-closed outside the fixed table.
	if v := classRefVar(sym); v != nil {
		return v, nil
	}
	// Also last-resort (ADR 0039): a dotted symbol spelling the GENERATED
	// class name of one of OUR defprotocol/defrecord/deftype vars
	// (my.name_space.TheName, namespace munged - → _) resolves to that var.
	if v := typeClassVar(sym); v != nil {
		return v, nil
	}
	return nil, fmt.Errorf("unable to resolve symbol: %s in this context", sym.Name())
}

// nsResolver adapts the global namespace world to reader.Resolver
// (design/01 §3): syntax-quote symbol resolution and auto-resolved
// keywords read the CURRENT *ns* per call, so `x in ns user reads as
// user/x and `list in a core-referring ns reads as clojure.core/list,
// as on JVM Clojure. Types don't resolve until the host registry lands
// (design/05). Stateless — moved from pkg/eval with ADR 0043.
type nsResolver struct{}

var _ reader.Resolver = nsResolver{}

// NSResolver returns the reader.Resolver backed by the current
// namespace (*ns*). The REPL driver, file loads and read-string inject
// it into their readers.
func NSResolver() reader.Resolver { return nsResolver{} }

func (nsResolver) CurrentNS() *lang.Symbol {
	return currentNS().Name()
}

func (nsResolver) ResolveAlias(sym *lang.Symbol) *lang.Symbol {
	if ns := currentNS().LookupAlias(sym); ns != nil {
		return ns.Name()
	}
	if ns := lang.FindNamespace(sym); ns != nil {
		return ns.Name()
	}
	return nil
}

func (nsResolver) ResolveVar(sym *lang.Symbol) *lang.Symbol {
	if m := currentNS().Mappings().ValAt(sym); m != nil {
		if v, ok := m.(*lang.Var); ok {
			return lang.InternSymbol(v.Namespace().Name().Name(), v.Symbol().Name())
		}
	}
	return nil
}

func (nsResolver) ResolveType(sym *lang.Symbol) *lang.Symbol {
	return nil
}
