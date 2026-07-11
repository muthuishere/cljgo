// Package ast is a micro version of the pkg/ast contract from design/03+00:
// one uniform Node{Op, Sub} with typed per-op payload structs. Only the ops
// spike S1 needs. No Form field (no reader here, so no provenance to carry).
package ast

type Op uint8

const (
	OpConst Op = iota
	OpIf
	OpLet
	OpDo
	OpVarRef
	OpLocal
	OpFn
	OpInvoke
	OpLoop
	OpRecur
	OpDef
)

func (o Op) String() string {
	return [...]string{"const", "if", "let", "do", "var", "local", "fn", "invoke", "loop", "recur", "def"}[o]
}

// Node is the uniform node; Sub holds the per-op payload struct.
type Node struct {
	Op  Op
	Sub any
}

// Const: Value is nil | bool | int64 | float64 | string.
type Const struct{ Value any }

type If struct{ Test, Then, Else *Node } // Else may be nil

type Binding struct {
	Name string
	Init *Node
}

type Let struct {
	Bindings []Binding // sequential, like let*
	Body     *Node
}

type Do struct{ Forms []*Node }

// VarRef references a global var by (unmunged) Clojure name.
type VarRef struct{ Name string }

// Local references a lexical binding by Clojure name.
type Local struct{ Name string }

// Fn is single-arity only in this spike (multi-arity switch is the same
// technique repeated per case; S1 doesn't need it to validate flattening).
type Fn struct {
	Name   string // optional self-name; unused by the spike programs
	Params []string
	Body   *Node
}

type Invoke struct {
	Target *Node
	Args   []*Node
}

type Loop struct {
	Bindings []Binding
	Body     *Node
}

type Recur struct{ Args []*Node }

type Def struct {
	Name string
	Init *Node
}
