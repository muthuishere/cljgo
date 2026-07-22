// purity.go — the transitive Go-interop taint classifier (ADR 0054 dec 3).
//
// ONE pass, no new walk: it iterates the namespaces CompileProgram already
// captured (Entry + Deps, ADR 0042's transitive-require traversal) and walks
// each namespace's analyzed forms with the canonical emit.eachChild child
// enumerator, switching on the five OpHost* ops. A pluggable Predicate slot
// lets N taint classes compose over the same walk (S34 proved this); the
// shipping predicate is GoInteropPredicate. From the resulting per-namespace
// taint map both gates read: the whole-library `publish clojars` gate is an OR
// over all namespaces, the per-namespace `use <ns>` gate is a lookup, and
// WholeLibPure == AND(NamespacePure over every reachable namespace).
package emit

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/muthuishere/cljgo/pkg/ast"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// Taint is one namespace's first taint finding, with the position a real gate
// cites in its hard-error message. A namespace with no taint is clean and is
// simply absent from the classification map.
type Taint struct {
	Class  string // "go-interop" (extensible via new predicates)
	NS     string // namespace name ("" entry recovered textually to its real name)
	Path   string // source file
	Line   int    // first offending form's line
	Detail string // e.g. "require-go member access (strings.ToUpper)"
}

// Taint classes. The classifier is class-extensible; "go-interop" is the only
// class a shipping predicate produces today (ffi/deflib have no AST op yet).
const (
	TaintGoInterop = "go-interop"
)

// Predicate examines ONE captured namespace and returns the FIRST taint it can
// prove, or nil if the namespace is clean of that predicate's class. Predicates
// are independent and composable: Classify runs every predicate over every
// namespace, recording the first taint any predicate reports (predicate order,
// then form order). This is the pluggable slot S34 proved N predicates compose
// over one walk.
type Predicate func(ns *CompiledNS) *Taint

// GoInteropPredicate flags a namespace that uses any of the five Go host-interop
// ops — the same node set pkg/emit/hostfacts.go keys on. It returns the first
// offending form's file:line so a caller can cite it. nil means clean.
func GoInteropPredicate(ns *CompiledNS) *Taint {
	name := nsRealName(ns)
	for _, form := range ns.Forms {
		if t := firstHostTaint(form); t != nil {
			line := t.Line
			if line == 0 {
				line = nodeLine(form)
			}
			return &Taint{
				Class:  TaintGoInterop,
				NS:     name,
				Path:   ns.Path,
				Line:   line,
				Detail: t.Detail,
			}
		}
	}
	return nil
}

// firstHostTaint returns the first host-interop node in n's subtree
// (pre-order), as a bare Taint carrying only Line+Detail, or nil.
func firstHostTaint(n *ast.Node) *Taint {
	if n == nil {
		return nil
	}
	if detail, ok := hostOpDetail(n); ok {
		return &Taint{Line: nodeLine(n), Detail: detail}
	}
	var found *Taint
	eachChild(n, func(c *ast.Node, _ bool) {
		if found != nil {
			return
		}
		found = firstHostTaint(c)
	})
	return found
}

// hostOpDetail reports whether n is one of the five Go host-interop ops and, if
// so, a human-readable detail string naming the offending surface.
func hostOpDetail(n *ast.Node) (string, bool) {
	switch n.Op {
	case ast.OpHostRef:
		s := n.Sub.(*ast.HostRefNode)
		return "require-go member access (" + s.Pkg + "." + s.Member + ")", true
	case ast.OpHostCall:
		s := n.Sub.(*ast.HostCallNode)
		return "require-go call (" + s.Pkg + "." + s.Member + ")", true
	case ast.OpHostMethod:
		s := n.Sub.(*ast.HostMethodNode)
		return "Go dot-method call (." + s.Method + ")", true
	case ast.OpHostField:
		s := n.Sub.(*ast.HostFieldNode)
		return "Go dot-field access (.-" + s.Field + ")", true
	case ast.OpHostNew:
		return "Go struct construction", true
	}
	return "", false
}

// Classify runs the given predicates over every namespace of p (Entry+Deps) in
// dependency-first order, returning a per-namespace taint map keyed by the
// namespace's REAL name — the entry's "" is recovered textually to its declared
// ns. A namespace absent from the map is clean. For each namespace the first
// taint any predicate reports (predicate order, then form order) is recorded.
func Classify(p *Program, preds ...Predicate) map[string]*Taint {
	out := map[string]*Taint{}
	if p == nil {
		return out
	}
	all := make([]*CompiledNS, 0, len(p.Deps)+1)
	all = append(all, p.Deps...) // dependency-first
	if p.Entry != nil {
		all = append(all, p.Entry) // entry last
	}
	for _, ns := range all {
		name := nsRealName(ns)
		for _, pred := range preds {
			if t := pred(ns); t != nil {
				t.NS = name
				out[name] = t
				break
			}
		}
	}
	return out
}

// RequireGoPredicate flags a namespace that DECLARES a Go dependency via
// (require-go …), even when no member is ever dereferenced. The five OpHost*
// ops fire only on member ACCESS; a bare `(require-go '[strconv :as sc])` with
// no `sc/…` use produces no host-op node, yet the form itself is not valid
// Clojure and cannot load on the JVM. ADR 0054 dec 2/3 name require-go/ffi
// itself as the disqualifying surface — so this predicate closes that gap. It
// is the second composed predicate over the one walk (the slot S34 reserved).
func RequireGoPredicate(ns *CompiledNS) *Taint {
	name := nsRealName(ns)
	for _, form := range ns.Forms {
		if line, ok := firstRequireGo(form); ok {
			return &Taint{
				Class:  TaintGoInterop,
				NS:     name,
				Path:   ns.Path,
				Line:   line,
				Detail: "require-go declaration (a Go dependency; not loadable on the JVM)",
			}
		}
	}
	return nil
}

// firstRequireGo returns the line of the first (require-go …) invocation in n's
// subtree (pre-order), detecting an OpInvoke whose callee is the require-go var.
func firstRequireGo(n *ast.Node) (int, bool) {
	if n == nil {
		return 0, false
	}
	if n.Op == ast.OpInvoke {
		if iv, ok := n.Sub.(*ast.InvokeNode); ok && iv.Fn != nil && iv.Fn.Op == ast.OpVar {
			if vn, ok := iv.Fn.Sub.(*ast.VarNode); ok && vn.Var != nil &&
				vn.Var.Symbol() != nil && vn.Var.Symbol().Name() == "require-go" {
				return nodeLine(n), true
			}
		}
	}
	var line int
	var found bool
	eachChild(n, func(c *ast.Node, _ bool) {
		if found {
			return
		}
		if l, ok := firstRequireGo(c); ok {
			line, found = l, true
		}
	})
	return line, found
}

// ClassifyGoInterop is Classify with the shipping Go-interop predicates: host-op
// member access AND a bare require-go declaration (both are Go-interop taint).
func ClassifyGoInterop(p *Program) map[string]*Taint {
	return Classify(p, GoInteropPredicate, RequireGoPredicate)
}

// WholeLibPure reports whether NO namespace is tainted. When tainted it also
// returns the first offender in deterministic (namespace-sorted) order, so a
// caller can cite a stable file:line. This is the `publish clojars` gate: an OR
// over the classification map.
func WholeLibPure(m map[string]*Taint) (bool, *Taint) {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if t := m[n]; t != nil {
			return false, t
		}
	}
	return true, nil
}

// NamespacePure reports whether the named namespace is untainted. This is the
// `use <ns>` gate: a lookup into the same map. A namespace absent from the map
// is clean (true).
func NamespacePure(m map[string]*Taint, ns string) bool {
	return m[ns] == nil
}

// ---- helpers --------------------------------------------------------------

// nsRealName returns a namespace's addressable name. CompileProgram leaves the
// entry's Name "" (it is a source path, not a required ns); recover its declared
// (ns X …) name textually so the entry is a first-class, named member.
func nsRealName(ns *CompiledNS) string {
	if ns == nil {
		return ""
	}
	if ns.Name != "" {
		return ns.Name
	}
	return readNSName(ns.Path)
}

// readNSName extracts the declared namespace from a source file's leading
// (ns X …) form, textually — enough to label an entry whose Name is "".
func readNSName(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "<entry:" + filepath.Base(path) + ">"
	}
	s := string(b)
	i := strings.Index(s, "(ns ")
	if i < 0 {
		return "<entry:" + filepath.Base(path) + ">"
	}
	rest := s[i+4:]
	j := strings.IndexAny(rest, " \t\r\n()")
	if j < 0 {
		j = len(rest)
	}
	if name := strings.TrimSpace(rest[:j]); name != "" {
		return name
	}
	return "<entry:" + filepath.Base(path) + ">"
}

// nodeLine reads the :line meta a node's original form carries, or 0.
func nodeLine(n *ast.Node) int {
	im, ok := n.Form.(lang.IMeta)
	if !ok || im.Meta() == nil {
		return 0
	}
	if l, ok := lang.AsInt(lang.Get(im.Meta(), lang.KWLine)); ok {
		return l
	}
	return 0
}
