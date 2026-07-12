// Package ast defines the AST produced by the analyzer (design/03 §1,
// design/00 §4.1): one uniform *Node with an integer Op tag and a typed
// per-op payload struct in Sub. The analyzer is the only writer; the
// evaluator and the Go emitter are read-only consumers dispatching on
// Node.Op. Passes that need annotations use side tables keyed by *Node,
// never mutation.
//
// v0 op vocabulary (design/03 §8, milestone v0): Const, collection
// literals, Var, Local, Do, If, Def, Let, Binding, Fn, FnMethod, Invoke,
// Quote. v1 (milestone M1) adds Loop, Recur, TheVar, SetBang and DynBind
// (the `binding` form). Later phases (letfn*, throw/try/catch, host
// interop) add their ops to this enum and to both consumers together.
package ast

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// Op tags a Node with its analyzed form kind. Integer dispatch (a flat
// switch) beats a type switch and keeps both consumers trivially auditable
// against the specials list (design/03 §1).
type Op uint8

const (
	OpConst Op = iota + 1
	OpVector
	OpMap
	OpSet
	OpVar
	OpLocal
	OpDo
	OpIf
	OpDef
	OpLet
	OpBinding
	OpFn
	OpFnMethod
	OpInvoke
	OpQuote
	OpLoop
	OpRecur
	OpTheVar
	OpSetBang
	OpDynBind
)

var opNames = map[Op]string{
	OpConst:    "const",
	OpVector:   "vector",
	OpMap:      "map",
	OpSet:      "set",
	OpVar:      "var",
	OpLocal:    "local",
	OpDo:       "do",
	OpIf:       "if",
	OpDef:      "def",
	OpLet:      "let",
	OpBinding:  "binding",
	OpFn:       "fn",
	OpFnMethod: "fn-method",
	OpInvoke:   "invoke",
	OpQuote:    "quote",
	OpLoop:     "loop",
	OpRecur:    "recur",
	OpTheVar:   "the-var",
	OpSetBang:  "set!",
	OpDynBind:  "dyn-bind",
}

func (op Op) String() string {
	if s, ok := opNames[op]; ok {
		return s
	}
	return fmt.Sprintf("Op(%d)", uint8(op))
}

// Node is the uniform AST node. Form carries the original form; source
// position (:file/:line/:column) rides on its metadata (design/00 §4.5).
// Sub points at the Op-specific payload struct.
type Node struct {
	Op   Op
	Form any
	Sub  any

	IsLiteral    bool // constant-foldable
	IsAssignable bool // set! target (unused in v0; no v0 op is assignable)
}

// BindKind classifies an OpBinding's introduction site.
type BindKind uint8

const (
	BindLet  BindKind = iota + 1 // let* binding
	BindArg                      // fn method parameter
	BindFn                       // fn* self-name
	BindLoop                     // loop* binding (a recur target)
)

func (k BindKind) String() string {
	switch k {
	case BindLet:
		return "let"
	case BindArg:
		return "arg"
	case BindFn:
		return "fn"
	case BindLoop:
		return "loop"
	}
	return fmt.Sprintf("BindKind(%d)", uint8(k))
}

// ConstNode is the payload of OpConst.
type ConstNode struct {
	Value any
}

// VectorNode is the payload of OpVector: a vector literal with analyzed
// children.
type VectorNode struct {
	Items []*Node
}

// MapNode is the payload of OpMap. Keys[i] pairs with Vals[i].
type MapNode struct {
	Keys []*Node
	Vals []*Node
}

// SetNode is the payload of OpSet.
type SetNode struct {
	Items []*Node
}

// VarNode is the payload of OpVar: a symbol resolved to a Var. The node
// holds the Var pointer, never its value — deref happens per use at
// runtime so re-def stays live (design/00 §4.2).
type VarNode struct {
	Var *lang.Var
}

// LocalNode is the payload of OpLocal: a symbol resolved to a lexical
// binding established earlier in analysis.
type LocalNode struct {
	Name    *lang.Symbol
	Binding *BindingNode
}

// DoNode is the payload of OpDo.
type DoNode struct {
	Statements []*Node
	Ret        *Node
}

// IfNode is the payload of OpIf. Else is never nil: a missing else branch
// analyzes to a const-nil node.
type IfNode struct {
	Test *Node
	Then *Node
	Else *Node
}

// DefNode is the payload of OpDef. Var is interned at analysis time
// (design/03 §2) so forward references and self-recursion resolve.
// Init and Meta may be nil.
type DefNode struct {
	Name *lang.Symbol
	Var  *lang.Var
	Init *Node
	Meta *Node
}

// LetNode is the payload of OpLet and OpLoop. Bindings are OpBinding
// nodes, in order (let* and loop* bindings are both sequential). LoopID
// is "" for let* (not a recur target); loop* sets it and its OpRecur
// nodes carry the same id (design/03 §5).
type LetNode struct {
	Bindings []*Node
	Body     *Node
	LoopID   string
}

// BindingNode is the payload of OpBinding. Init is nil for BindArg/BindFn
// bindings (their values arrive at call time).
type BindingNode struct {
	Name       *lang.Symbol
	Init       *Node
	Kind       BindKind
	ArgID      int
	IsVariadic bool
}

// FnNode is the payload of OpFn. Local, if non-nil, is the OpBinding for
// the optional self-name, visible only inside the fn's own bodies.
type FnNode struct {
	Methods       []*Node // OpFnMethod
	IsVariadic    bool
	MaxFixedArity int
	Local         *Node // OpBinding (BindFn) or nil
}

// FnMethodNode is the payload of OpFnMethod. Params are OpBinding nodes
// (BindArg); for a variadic method the last param is the rest param and
// FixedArity counts only the fixed prefix. Each method is its own recur
// target (LoopID), used from v1 on.
type FnMethodNode struct {
	Params     []*Node // OpBinding
	FixedArity int
	IsVariadic bool
	Body       *Node
	LoopID     string
}

// InvokeNode is the payload of OpInvoke.
type InvokeNode struct {
	Fn   *Node
	Args []*Node
}

// QuoteNode is the payload of OpQuote: the datum is unanalyzed.
type QuoteNode struct {
	Value any
}

// RecurNode is the payload of OpRecur. LoopID names the owning loop* or
// fn-method frame; the evaluator's recur signal and the emitter's labeled
// `continue` both match on it (design/03 §5).
type RecurNode struct {
	Exprs  []*Node
	LoopID string
}

// TheVarNode is the payload of OpTheVar: (var sym) resolved to an
// existing Var. Evaluates to the Var object itself, not its value.
type TheVarNode struct {
	Var *lang.Var
}

// SetBangNode is the payload of OpSetBang. Target is an assignable node —
// in v1 only an OpVar (the dynamic/thread-binding check is the
// evaluator's, per Clojure); host fields join in a later phase.
type SetBangNode struct {
	Target *Node
	Val    *Node
}

// DynBindNode is the payload of OpDynBind, the `binding` form. Vars[i]
// (an OpVar node) is bound to the value of Vals[i] for the dynamic
// extent of Body via push/popThreadBindings. Vals are evaluated before
// any binding is pushed (bindings are made "in parallel", as in
// Clojure's binding macro).
type DynBindNode struct {
	Vars []*Node // OpVar
	Vals []*Node
	Body *Node
}
