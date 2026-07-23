// Numeric type inference for the emitter (spike s42 / ADR 0067).
//
// A conservative, bottom-up pass that proves an emitted local carries an
// unboxed Go `int64` — from integer literals, checked arithmetic on
// proven-int64 operands, numeric loop/recur carriers whose init and every
// recur value are int64, and (under param specialization) int64 fn
// parameters and self-recursive calls that return int64. Where the proof
// does not hold the pass reports ntUnknown and the emitter keeps the
// boxed `any` path unchanged. Correctness-first: any uncertainty stays
// boxed.
//
// The pass is the "primitive hints / intrinsics" rung of design/04 §5 and
// the boxing-elimination the ADR-0045 pprof decomposition pointed at
// (every emitted local `any`, so rt.Add2/Mul2 re-box each int64 via
// runtime.convT64 — ~23% CPU and all 12M allocs on (fact 15)x2M).
package emit

import (
	"os"

	"github.com/muthuishere/cljgo/pkg/ast"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// numInferEnabled gates the whole pass. Set CLJGO_NUMINFER_OFF=1 to force
// the boxed emission everywhere — the A/B lever for the spike-s42 / ADR
// 0067 measurement, and a kill switch if the pass is ever suspected.
var numInferEnabled = os.Getenv("CLJGO_NUMINFER_OFF") == ""

type numType uint8

const (
	// ntUnknown MUST be the zero value: a map lookup for an unseen node or
	// binding then yields "not proven int64", not a false positive. (An
	// earlier ordering made ntBottom the zero value, so an untyped param
	// read back as the meet-identity and wrongly typed a loop carrier whose
	// init was that param — a real miscompile caught regenerating core.)
	ntUnknown numType = iota
	ntInt64
	// ntBottom is a node that transfers control (recur/throw) and yields no
	// value; it is the identity element of the type meet so a loop's
	// non-recur arm alone determines the loop-value type. It is produced
	// ONLY by explicit OpRecur/OpThrow typing, never by a default lookup.
	ntBottom
)

// meet combines two branch types. int64 ⊓ int64 = int64; bottom is
// identity; anything with an unknown is unknown.
func meet(a, b numType) numType {
	if a == ntBottom {
		return b
	}
	if b == ntBottom {
		return a
	}
	if a == ntInt64 && b == ntInt64 {
		return ntInt64
	}
	return ntUnknown
}

// carrier records a loop/method recur carrier: its loopID and slot index.
type carrier struct {
	loopID string
	slot   int
}

// numInfer holds the result of one inference run over a body subtree.
type numInfer struct {
	bind map[*ast.BindingNode]numType // proven type of a binding site
	node map[*ast.Node]numType        // cached type of an expression node

	letBinds map[*ast.BindingNode]bool    // let* binding sites (init-typed)
	carriers map[*ast.BindingNode]carrier // loop/method recur carriers
	recurs   map[string][]*ast.RecurNode  // loopID -> recur nodes targeting it
	forced   map[*ast.BindingNode]bool    // bindings pinned to Unknown (captured carriers)

	// self identifies the fn being specialized so a self-recursive call
	// with all-int64 args can be typed int64 (greatest-fixpoint optimistic
	// assumption, validated after the run).
	selfBind   *ast.BindingNode // fn* self-name binding, or nil
	selfVar    *lang.Var        // def target var, or nil
	selfRetInt bool             // assume the self-fn returns int64
}

func (ni *numInfer) isInt64(n *ast.Node) bool          { return ni != nil && ni.node[n] == ntInt64 }
func (ni *numInfer) bindInt64(b *ast.BindingNode) bool { return ni != nil && ni.bind[b] == ntInt64 }

// goType is "int64" for a proven-int64 node, else "any".
func (ni *numInfer) goType(n *ast.Node) string {
	if ni.isInt64(n) {
		return "int64"
	}
	return "any"
}

// inferNumeric runs the fixpoint over root. seed pre-types binding sites
// (fn params under specialization); methodCarriers registers a fn method's
// params as recur carriers of methodLoopID (so recur values constrain the
// param types). selfBind/selfVar enable self-call typing.
func inferNumeric(root *ast.Node, seed map[*ast.BindingNode]numType, methodCarriers []*ast.BindingNode, methodLoopID string, selfBind *ast.BindingNode, selfVar *lang.Var) *numInfer {
	if !numInferEnabled {
		return emptyInfer()
	}
	ni := &numInfer{
		bind:       map[*ast.BindingNode]numType{},
		node:       map[*ast.Node]numType{},
		letBinds:   map[*ast.BindingNode]bool{},
		carriers:   map[*ast.BindingNode]carrier{},
		recurs:     map[string][]*ast.RecurNode{},
		forced:     map[*ast.BindingNode]bool{},
		selfBind:   selfBind,
		selfVar:    selfVar,
		selfRetInt: selfBind != nil || selfVar != nil,
	}
	ni.collect(root)
	for i, b := range methodCarriers {
		ni.carriers[b] = carrier{loopID: methodLoopID, slot: i}
	}
	// Seed params.
	for b, t := range seed {
		ni.bind[b] = t
	}
	// Optimistic start: every let binding and loop carrier is int64; the
	// fixpoint demotes those that fail. Captured carriers stay boxed — a
	// nested-fn closure captures the Go VARIABLE, and the S5 per-iteration
	// copy keeps that path on `any`.
	for b := range ni.letBinds {
		if !ni.forced[b] {
			ni.bind[b] = ntInt64
		}
	}
	for b := range ni.carriers {
		if _, seeded := seed[b]; !seeded && !ni.forced[b] {
			ni.bind[b] = ntInt64
		}
	}
	// Fixpoint: recompute node types, then demote failing bindings, until
	// stable. Movement is monotone int64 -> unknown, so it terminates.
	for iter := 0; iter < 128; iter++ {
		ni.node = map[*ast.Node]numType{}
		ni.typeNode(root)
		changed := false
		for b := range ni.bind {
			if ni.bind[b] != ntInt64 {
				continue
			}
			if nt := ni.bindTypeFromUses(b); nt != ntInt64 {
				ni.bind[b] = ntUnknown
				changed = true
			}
		}
		if ni.selfRetInt {
			if rt := ni.node[root]; rt != ntInt64 && rt != ntBottom {
				ni.selfRetInt = false
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	ni.node = map[*ast.Node]numType{}
	ni.typeNode(root)
	return ni
}

// bindTypeFromUses derives a binding's type from its definition sites.
func (ni *numInfer) bindTypeFromUses(b *ast.BindingNode) numType {
	if ni.forced[b] {
		return ntUnknown
	}
	if ni.letBinds[b] {
		if b.Init != nil {
			return ni.node[b.Init]
		}
		return ntUnknown
	}
	if c, ok := ni.carriers[b]; ok {
		t := ntInt64
		if b.Init != nil { // loop carriers have an init; params (BindArg) do not
			t = meet(t, ni.node[b.Init])
		}
		for _, r := range ni.recurs[c.loopID] {
			if c.slot < len(r.Exprs) {
				t = meet(t, ni.node[r.Exprs[c.slot]])
			}
		}
		return t
	}
	// A seeded param that is not a recur carrier: its int64-ness is
	// enforced by the runtime entry guard, so it holds.
	return ni.bind[b]
}

func (ni *numInfer) typeNode(n *ast.Node) numType {
	if n == nil {
		return ntUnknown
	}
	if t, ok := ni.node[n]; ok {
		return t
	}
	ni.node[n] = ntUnknown // cycle guard
	t := ni.computeNode(n)
	ni.node[n] = t
	return t
}

func (ni *numInfer) computeNode(n *ast.Node) numType {
	switch n.Op {
	case ast.OpConst:
		if _, ok := n.Sub.(*ast.ConstNode).Value.(int64); ok {
			return ntInt64
		}
		return ntUnknown
	case ast.OpLocal:
		return ni.bind[n.Sub.(*ast.LocalNode).Binding]
	case ast.OpDo:
		s := n.Sub.(*ast.DoNode)
		for _, st := range s.Statements {
			ni.typeNode(st)
		}
		return ni.typeNode(s.Ret)
	case ast.OpIf:
		s := n.Sub.(*ast.IfNode)
		ni.typeNode(s.Test)
		return meet(ni.typeNode(s.Then), ni.typeNode(s.Else))
	case ast.OpLet, ast.OpLoop:
		s := n.Sub.(*ast.LetNode)
		for _, bn := range s.Bindings {
			if b := bn.Sub.(*ast.BindingNode); b.Init != nil {
				ni.typeNode(b.Init)
			}
		}
		return ni.typeNode(s.Body)
	case ast.OpRecur:
		for _, e := range n.Sub.(*ast.RecurNode).Exprs {
			ni.typeNode(e)
		}
		return ntBottom
	case ast.OpThrow:
		ni.typeNode(n.Sub.(*ast.ThrowNode).Exception)
		return ntBottom
	case ast.OpInvoke:
		return ni.typeInvoke(n.Sub.(*ast.InvokeNode))
	case ast.OpFn:
		// A nested fn is an opaque function value here; its interior is a
		// separate inference scope (own params/self/loops), run when genFn
		// emits it. Do not descend — that would type inner nodes under the
		// wrong scope.
		return ntUnknown
	default:
		eachChild(n, func(c *ast.Node, _ bool) { ni.typeNode(c) })
		return ntUnknown
	}
}

// numericBuiltin2 are the pristine core ops yielding int64 from two int64
// operands (checked; overflow throws — never promotes).
var numericBuiltin2 = map[string]bool{"+": true, "-": true, "*": true}

// numericBuiltin1 are the 1-arg pristine core ops yielding int64.
var numericBuiltin1 = map[string]bool{"inc": true, "dec": true}

func (ni *numInfer) typeInvoke(s *ast.InvokeNode) numType {
	argt := make([]numType, len(s.Args))
	for i, a := range s.Args {
		argt[i] = ni.typeNode(a)
	}
	if s.Fn.Op == ast.OpVar {
		v := s.Fn.Sub.(*ast.VarNode).Var
		if v.Namespace() == lang.NSCore {
			name := v.Symbol().Name()
			if len(argt) == 2 && numericBuiltin2[name] && argt[0] == ntInt64 && argt[1] == ntInt64 {
				return ntInt64
			}
			if len(argt) == 1 && numericBuiltin1[name] && argt[0] == ntInt64 {
				return ntInt64
			}
		}
		if ni.selfVar != nil && v == ni.selfVar && ni.selfRetInt && allInt64(argt) {
			return ntInt64
		}
	} else if s.Fn.Op == ast.OpLocal {
		b := s.Fn.Sub.(*ast.LocalNode).Binding
		if ni.selfBind != nil && b == ni.selfBind && ni.selfRetInt && allInt64(argt) {
			return ntInt64
		}
	}
	ni.typeNode(s.Fn)
	return ntUnknown
}

func allInt64(ts []numType) bool {
	if len(ts) == 0 {
		return false
	}
	for _, t := range ts {
		if t != ntInt64 {
			return false
		}
	}
	return true
}

// collect records let* binding sites, loop* carriers and recur targets.
func (ni *numInfer) collect(n *ast.Node) {
	if n == nil {
		return
	}
	switch n.Op {
	case ast.OpLoop:
		s := n.Sub.(*ast.LetNode)
		captured := capturedLoopBindings(s.Bindings, s.Body)
		for i, bn := range s.Bindings {
			b := bn.Sub.(*ast.BindingNode)
			ni.carriers[b] = carrier{loopID: s.LoopID, slot: i}
			if captured[b] {
				ni.forced[b] = true
			}
		}
	case ast.OpLet:
		s := n.Sub.(*ast.LetNode)
		for _, bn := range s.Bindings {
			ni.letBinds[bn.Sub.(*ast.BindingNode)] = true
		}
	case ast.OpRecur:
		s := n.Sub.(*ast.RecurNode)
		ni.recurs[s.LoopID] = append(ni.recurs[s.LoopID], s)
	case ast.OpFn:
		return // nested fn: separate inference scope (see computeNode)
	}
	eachChild(n, func(c *ast.Node, _ bool) { ni.collect(c) })
}
