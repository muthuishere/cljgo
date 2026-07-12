package eval

import (
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// nsResolver adapts the evaluator's live namespace world to
// reader.Resolver (design/01 §3): syntax-quote symbol resolution and
// auto-resolved keywords read the CURRENT *ns* per call, so `x in ns
// user reads as user/x and `list in a core-referring ns reads as
// clojure.core/list, as on JVM Clojure. Types don't resolve until the
// host registry lands (design/05).
type nsResolver struct {
	e *Evaluator
}

var _ reader.Resolver = (*nsResolver)(nil)

// ReaderResolver returns the reader.Resolver backed by this evaluator's
// current namespace. The REPL driver and file loads inject it into
// their readers.
func (e *Evaluator) ReaderResolver() reader.Resolver { return &nsResolver{e: e} }

func (r *nsResolver) CurrentNS() *lang.Symbol {
	return r.e.CurrentNS().Name()
}

func (r *nsResolver) ResolveAlias(sym *lang.Symbol) *lang.Symbol {
	if ns := r.e.CurrentNS().LookupAlias(sym); ns != nil {
		return ns.Name()
	}
	if ns := lang.FindNamespace(sym); ns != nil {
		return ns.Name()
	}
	return nil
}

func (r *nsResolver) ResolveVar(sym *lang.Symbol) *lang.Symbol {
	if m := r.e.CurrentNS().Mappings().ValAt(sym); m != nil {
		if v, ok := m.(*lang.Var); ok {
			return lang.InternSymbol(v.Namespace().Name().Name(), v.Symbol().Name())
		}
	}
	return nil
}

func (r *nsResolver) ResolveType(sym *lang.Symbol) *lang.Symbol {
	return nil
}
