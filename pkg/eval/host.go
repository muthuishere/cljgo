package eval

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/ast"
)

// evalHost evaluates Go interop nodes (OpHostRef / OpHostCall) in the
// interpreter (ADR 0010, design/05 §1–§2). M3-v0 target: a reflect-backed
// seed registry (fmt/strings/strconv/math), require-go alias resolution,
// and the shared shaping table ([v err] vectors, `!` throw, nil
// normalization). This is the contract stub — the interpreted path is
// filled in by the M3-v0 interop work; until then host forms error
// cleanly rather than panicking.
func (e *Evaluator) evalHost(n *ast.Node) (any, error) {
	switch n.Op {
	case ast.OpHostRef:
		r := n.Sub.(*ast.HostRefNode)
		return nil, fmt.Errorf("go interop not yet implemented: %s.%s", r.Pkg, r.Member)
	case ast.OpHostCall:
		c := n.Sub.(*ast.HostCallNode)
		return nil, fmt.Errorf("go interop not yet implemented: (%s.%s ...)", c.Pkg, c.Member)
	default:
		return nil, fmt.Errorf("evalHost: unexpected op %v", n.Op)
	}
}
