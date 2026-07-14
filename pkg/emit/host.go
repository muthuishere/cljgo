package emit

import "github.com/muthuishere/cljgo/pkg/ast"

// genHost emits Go interop nodes (OpHostRef / OpHostCall) in AOT mode
// (ADR 0010, design/05 §4, spike S2). M3-v0 target: resolve the callee's
// signature from go/packages type facts, add the real `import`, emit a
// direct non-reflective call, and shape the result per the shared table
// ([v err] vector, `!` → unwrap-or-panic, nil normalization) — identical
// semantics to the interpreter (dual-harness). This is the contract stub;
// the AOT path is filled in by the M3-v0 interop work, and until then a
// host form is a clean build error rather than a panic-free miscompile.
func (g *generator) genHost(n *ast.Node) string {
	switch n.Op {
	case ast.OpHostRef:
		r := n.Sub.(*ast.HostRefNode)
		return g.failf("AOT go interop not yet implemented: %s.%s", r.Pkg, r.Member)
	case ast.OpHostCall:
		c := n.Sub.(*ast.HostCallNode)
		return g.failf("AOT go interop not yet implemented: (%s.%s ...)", c.Pkg, c.Member)
	default:
		return g.failf("genHost: unexpected op %v", n.Op)
	}
}
