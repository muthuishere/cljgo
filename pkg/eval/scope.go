package eval

// Scope is the runtime lexical environment: a parent-linked frame
// (design/03 §3c). The evaluator is its only consumer — the emitter maps
// locals to Go variables instead. Closure capture = an *evalFn holding the
// *Scope live at fn* evaluation time.
type Scope struct {
	parent *Scope
	vals   map[string]any
}

// NewScope returns an empty root scope.
func NewScope() *Scope {
	return &Scope{vals: map[string]any{}}
}

// Push returns a child scope.
func (s *Scope) Push() *Scope {
	return &Scope{parent: s, vals: map[string]any{}}
}

// Define binds name in this frame.
func (s *Scope) Define(name string, v any) {
	s.vals[name] = v
}

// Lookup walks up the chain.
func (s *Scope) Lookup(name string) (any, bool) {
	for cur := s; cur != nil; cur = cur.parent {
		if v, ok := cur.vals[name]; ok {
			return v, true
		}
	}
	return nil, false
}
