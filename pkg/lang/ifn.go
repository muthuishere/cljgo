package lang

// FnFunc is a wrapped Go function that implements the IFn interface.
type FnFunc func(args ...any) any

var (
	_ IFn = FnFunc(nil)
	_ IFn = FnFunc0(nil)
	_ IFn = FnFunc1(nil)
	_ IFn = FnFunc2(nil)
	_ IFn = FnFunc3(nil)
	_ IFn = FnFunc4(nil)
)

func NewFnFunc(fn func(args ...any) any) FnFunc {
	return FnFunc(fn)
}

func (f FnFunc) Invoke(args ...any) any {
	return f(args...)
}

func (f FnFunc) ApplyTo(args ISeq) any {
	return f(seqToSlice(args)...)
}

func (f FnFunc) Meta() IPersistentMap {
	return nil
}

func (f FnFunc) WithMeta(meta IPersistentMap) any {
	// no-op
	return f
}

// FnFunc0 is a zero-argument function implementing IFn with no []any allocation.
type FnFunc0 func() any

func NewFnFunc0(fn func() any) FnFunc0 { return FnFunc0(fn) }

func (f FnFunc0) Invoke(args ...any) any {
	if len(args) != 0 {
		panic(&ArityError{Actual: len(args), Expected: "0"})
	}
	return f()
}

func (f FnFunc0) ApplyTo(args ISeq) any {
	return f()
}

func (f FnFunc0) Meta() IPersistentMap          { return nil }
func (f FnFunc0) WithMeta(_ IPersistentMap) any { return f }

// FnFunc1 is a one-argument function implementing IFn with no []any allocation.
type FnFunc1 func(any) any

func NewFnFunc1(fn func(any) any) FnFunc1 { return FnFunc1(fn) }

func (f FnFunc1) Invoke(args ...any) any {
	if len(args) != 1 {
		panic(&ArityError{Actual: len(args), Expected: "1"})
	}
	return f(args[0])
}

func (f FnFunc1) ApplyTo(args ISeq) any {
	return f.Invoke(seqToSlice(args)...)
}

func (f FnFunc1) Meta() IPersistentMap          { return nil }
func (f FnFunc1) WithMeta(_ IPersistentMap) any { return f }

// FnFunc2 is a two-argument function implementing IFn with no []any allocation.
type FnFunc2 func(any, any) any

func NewFnFunc2(fn func(any, any) any) FnFunc2 { return FnFunc2(fn) }

func (f FnFunc2) Invoke(args ...any) any {
	if len(args) != 2 {
		panic(&ArityError{Actual: len(args), Expected: "2"})
	}
	return f(args[0], args[1])
}

func (f FnFunc2) ApplyTo(args ISeq) any {
	return f.Invoke(seqToSlice(args)...)
}

func (f FnFunc2) Meta() IPersistentMap          { return nil }
func (f FnFunc2) WithMeta(_ IPersistentMap) any { return f }

// FnFunc3 is a three-argument function implementing IFn with no []any allocation.
type FnFunc3 func(any, any, any) any

func NewFnFunc3(fn func(any, any, any) any) FnFunc3 { return FnFunc3(fn) }

func (f FnFunc3) Invoke(args ...any) any {
	if len(args) != 3 {
		panic(&ArityError{Actual: len(args), Expected: "3"})
	}
	return f(args[0], args[1], args[2])
}

func (f FnFunc3) ApplyTo(args ISeq) any {
	return f.Invoke(seqToSlice(args)...)
}

func (f FnFunc3) Meta() IPersistentMap          { return nil }
func (f FnFunc3) WithMeta(_ IPersistentMap) any { return f }

// FnFunc4 is a four-argument function implementing IFn with no []any allocation.
type FnFunc4 func(any, any, any, any) any

func NewFnFunc4(fn func(any, any, any, any) any) FnFunc4 { return FnFunc4(fn) }

func (f FnFunc4) Invoke(args ...any) any {
	if len(args) != 4 {
		panic(&ArityError{Actual: len(args), Expected: "4"})
	}
	return f(args[0], args[1], args[2], args[3])
}

func (f FnFunc4) ApplyTo(args ISeq) any {
	return f.Invoke(seqToSlice(args)...)
}

func (f FnFunc4) Meta() IPersistentMap          { return nil }
func (f FnFunc4) WithMeta(_ IPersistentMap) any { return f }

// NamedFn0..NamedFn4 wrap the corresponding FnFuncN closure with the fn's
// display name ("user/f") and expects label ("1: [x]") so an arity
// mismatch panics the NAMED ArityError the interpreter's evalFn raises —
// a compiled binary must read identically (ADR 0048; the error-message
// doctrine forbids "passed to: fn"). The emitter binds these as the fn
// VALUE while keeping the raw FnFuncN closure in the typed direct-call
// handle (ADR 0064), and lang.Apply0..4 dispatch the matching-arity call
// straight through F — only the error path pays for the name.

var (
	_ IFn = (*NamedFn0)(nil)
	_ IFn = (*NamedFn1)(nil)
	_ IFn = (*NamedFn2)(nil)
	_ IFn = (*NamedFn3)(nil)
	_ IFn = (*NamedFn4)(nil)
)

// NamedFn0 is a zero-argument closure carrying its arity-error name.
type NamedFn0 struct {
	Name    string
	Expects string
	F       FnFunc0
}

func (f *NamedFn0) Invoke(args ...any) any {
	if len(args) != 0 {
		panic(&ArityError{Actual: len(args), Name: f.Name, Expected: f.Expects})
	}
	return f.F()
}

func (f *NamedFn0) ApplyTo(args ISeq) any         { return f.Invoke(seqToSlice(args)...) }
func (f *NamedFn0) String() string                { return "#object[" + f.Name + "]" }
func (f *NamedFn0) Meta() IPersistentMap          { return nil }
func (f *NamedFn0) WithMeta(_ IPersistentMap) any { return f }

// NamedFn1 is a one-argument closure carrying its arity-error name.
type NamedFn1 struct {
	Name    string
	Expects string
	F       FnFunc1
}

func (f *NamedFn1) Invoke(args ...any) any {
	if len(args) != 1 {
		panic(&ArityError{Actual: len(args), Name: f.Name, Expected: f.Expects})
	}
	return f.F(args[0])
}

func (f *NamedFn1) ApplyTo(args ISeq) any         { return f.Invoke(seqToSlice(args)...) }
func (f *NamedFn1) String() string                { return "#object[" + f.Name + "]" }
func (f *NamedFn1) Meta() IPersistentMap          { return nil }
func (f *NamedFn1) WithMeta(_ IPersistentMap) any { return f }

// NamedFn2 is a two-argument closure carrying its arity-error name.
type NamedFn2 struct {
	Name    string
	Expects string
	F       FnFunc2
}

func (f *NamedFn2) Invoke(args ...any) any {
	if len(args) != 2 {
		panic(&ArityError{Actual: len(args), Name: f.Name, Expected: f.Expects})
	}
	return f.F(args[0], args[1])
}

// Invoke2 is the IFn2 reduce-seam fast path (no []any allocation).
func (f *NamedFn2) Invoke2(a, b any) any { return f.F(a, b) }

func (f *NamedFn2) ApplyTo(args ISeq) any         { return f.Invoke(seqToSlice(args)...) }
func (f *NamedFn2) String() string                { return "#object[" + f.Name + "]" }
func (f *NamedFn2) Meta() IPersistentMap          { return nil }
func (f *NamedFn2) WithMeta(_ IPersistentMap) any { return f }

// NamedFn3 is a three-argument closure carrying its arity-error name.
type NamedFn3 struct {
	Name    string
	Expects string
	F       FnFunc3
}

func (f *NamedFn3) Invoke(args ...any) any {
	if len(args) != 3 {
		panic(&ArityError{Actual: len(args), Name: f.Name, Expected: f.Expects})
	}
	return f.F(args[0], args[1], args[2])
}

func (f *NamedFn3) ApplyTo(args ISeq) any         { return f.Invoke(seqToSlice(args)...) }
func (f *NamedFn3) String() string                { return "#object[" + f.Name + "]" }
func (f *NamedFn3) Meta() IPersistentMap          { return nil }
func (f *NamedFn3) WithMeta(_ IPersistentMap) any { return f }

// NamedFn4 is a four-argument closure carrying its arity-error name.
type NamedFn4 struct {
	Name    string
	Expects string
	F       FnFunc4
}

func (f *NamedFn4) Invoke(args ...any) any {
	if len(args) != 4 {
		panic(&ArityError{Actual: len(args), Name: f.Name, Expected: f.Expects})
	}
	return f.F(args[0], args[1], args[2], args[3])
}

func (f *NamedFn4) ApplyTo(args ISeq) any         { return f.Invoke(seqToSlice(args)...) }
func (f *NamedFn4) String() string                { return "#object[" + f.Name + "]" }
func (f *NamedFn4) Meta() IPersistentMap          { return nil }
func (f *NamedFn4) WithMeta(_ IPersistentMap) any { return f }
