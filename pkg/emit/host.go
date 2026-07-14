package emit

// host.go emits Go interop nodes (OpHostRef / OpHostCall) in AOT mode
// (ADR 0010, design/05 §2, spike S2). The callee's signature is resolved
// from go/packages type facts (hostfacts.go) in the COMPILER process; the
// generated Go performs a direct, non-reflective call and shapes the
// result per the shared table — [v err] vectors, `!` unwrap-or-panic, nil
// normalization, int→int64 / float→float64 widening — BYTE-IDENTICAL to
// the interpreter's reflect path (pkg/eval/host.go). Divergence between
// the two harnesses is the unforgivable failure mode (design/00 §1.4).

import (
	"fmt"
	"go/types"
	"strings"

	"github.com/muthuishere/cljgo/pkg/ast"
)

func (g *generator) genHost(n *ast.Node) string {
	switch n.Op {
	case ast.OpHostRef:
		return g.genHostRef(n.Sub.(*ast.HostRefNode))
	case ast.OpHostCall:
		return g.genHostCall(n.Sub.(*ast.HostCallNode))
	case ast.OpHostMethod:
		return g.genHostMethod(n.Sub.(*ast.HostMethodNode))
	default:
		return g.failf("genHost: unexpected op %v", n.Op)
	}
}

// genHostMethod emits a Clojure dot-form method call `(.Method recv arg...)`
// (design/05 §1, ADR 0010). The receiver's static type is unknown in M3.1,
// so the call is reflective: rt.CallMethod(recv, "Method", throw, args...)
// delegates to the SAME eval.CallGoMethod the interpreter uses, so no
// go/packages resolution is needed and the two harnesses are byte-identical
// by construction. Receiver and args are forced through `any` temps so the
// reflective boundary always sees interface values.
func (g *generator) genHostMethod(m *ast.HostMethodNode) string {
	rv := g.gen(m.Recv)
	recvT := g.temp()
	g.wf("var %s any = %s\n", recvT, rv)

	argTemps := make([]string, len(m.Args))
	for i, an := range m.Args {
		av := g.gen(an)
		at := g.temp()
		g.wf("var %s any = %s\n", at, av)
		argTemps[i] = at
	}

	res := g.temp()
	call := fmt.Sprintf("rt.CallMethod(%s, %q, %t", recvT, m.Method, m.Throw)
	if len(argTemps) > 0 {
		call += ", " + strings.Join(argTemps, ", ")
	}
	call += ")"
	g.wf("var %s any = %s\n", res, call)
	return res
}

// genHostRef emits a Go member used in value position (design/05 §1).
// v0 supports an exported const/var (e.g. math/Pi); a function-as-value
// needs a boxed IFn adapter — deferred to M3.1.
func (g *generator) genHostRef(r *ast.HostRefNode) string {
	if g.host == nil {
		return g.failf("emit: internal: host facts not loaded for %s.%s", r.Pkg, r.Member)
	}
	obj, p, err := g.host.object(r.Pkg, r.Member)
	if err != nil {
		return g.failf("%v", err)
	}
	if _, isFunc := obj.(*types.Func); isFunc {
		return g.failf("emit: fn-as-value in AOT is M3.1 (%s.%s)", r.Pkg, r.Member)
	}
	pkgName := g.addHostImport(p.PkgPath, p.Types.Name())
	sel := pkgName + "." + r.Member
	t := g.temp()
	g.wf("var %s any = %s\n", t, g.widen(sel, obj.Type()))
	return t
}

// genHostCall emits a direct call and shapes its results (design/05 §2).
func (g *generator) genHostCall(c *ast.HostCallNode) string {
	if g.host == nil {
		return g.failf("emit: internal: host facts not loaded for %s.%s", c.Pkg, c.Member)
	}
	sig, err := g.host.sig(c.Pkg, c.Member)
	if err != nil {
		return g.failf("%v", err)
	}
	if len(sig.params) != len(c.Args) && !sig.variadic {
		return g.failf("emit: wrong number of args (%d) passed to: %s.%s",
			len(c.Args), c.Pkg, c.Member)
	}
	if sig.variadic && len(c.Args) < len(sig.params)-1 {
		return g.failf("emit: wrong number of args (%d) passed to: %s.%s",
			len(c.Args), c.Pkg, c.Member)
	}
	pkgName := g.addHostImport(sig.pkgPath, sig.pkgName)

	// Each arg r-value is forced through an `any` temp so the coercion
	// type-assertion always has an interface source (const args emit as
	// concretely-typed Go literals, not `any`).
	coerced := make([]string, len(c.Args))
	for i, an := range c.Args {
		rv := g.gen(an)
		at := g.temp()
		g.wf("var %s any = %s\n", at, rv)
		coerced[i] = g.coerce(at, sig.paramType(i))
	}
	call := fmt.Sprintf("%s.%s(%s)", pkgName, sig.funcName, strings.Join(coerced, ", "))

	n := len(sig.results)
	if n == 0 {
		g.wf("%s\n", call)
		return "nil"
	}

	// Bind every result so shaping can widen/normalize each independently.
	ts := make([]string, n)
	for i := range ts {
		ts[i] = g.temp()
	}
	g.wf("%s := %s\n", strings.Join(ts, ", "), call)
	last := ts[n-1]

	res := g.temp()

	switch {
	case sig.trailingError:
		vals := ts[:n-1]
		if c.Throw {
			// `!`: non-nil trailing error → panic (recovered like any
			// exception); else unwrap the value(s).
			g.wf("if %s != nil {\npanic(rt.GoError(%s))\n}\n", last, last)
			g.wf("var %s any = %s\n", res, g.shapeValues(vals, sig.results))
		} else if len(vals) == 0 {
			// Only-error result: the error-or-nil directly, NOT a vector.
			g.wf("var %s any = rt.NormErr(%s)\n", res, last)
		} else {
			parts := g.widenAll(vals, sig.results)
			parts = append(parts, fmt.Sprintf("rt.NormErr(%s)", last))
			g.wf("var %s any = lang.NewVector(%s)\n", res, strings.Join(parts, ", "))
		}

	case sig.trailingBool:
		vals := ts[:n-1]
		if c.Throw {
			g.usesFmt = true
			g.wf("if !%s {\npanic(rt.GoError(fmt.Errorf(%q)))\n}\n", last,
				fmt.Sprintf("%s.%s returned false", c.Pkg, c.Member))
			g.wf("var %s any = %s\n", res, g.shapeValues(vals, sig.results))
		} else {
			parts := g.widenAll(vals, sig.results)
			parts = append(parts, last) // bool boxed as-is
			g.wf("var %s any = lang.NewVector(%s)\n", res, strings.Join(parts, ", "))
		}

	case n == 1:
		g.wf("var %s any = %s\n", res, g.widen(ts[0], sig.results[0]))

	default: // multiple non-error/non-bool results → a vector
		parts := g.widenAll(ts, sig.results)
		g.wf("var %s any = lang.NewVector(%s)\n", res, strings.Join(parts, ", "))
	}
	return res
}

// shapeValues shapes the value portion (results minus a trailing
// error/bool) for the `!` path: 0 → nil, 1 → the widened value, ≥2 → a
// vector of widened values. rts is the FULL result-type slice; only the
// leading len(vals) entries are consumed.
func (g *generator) shapeValues(vals []string, rts []types.Type) string {
	switch len(vals) {
	case 0:
		return "nil"
	case 1:
		return g.widen(vals[0], rts[0])
	default:
		return "lang.NewVector(" + strings.Join(g.widenAll(vals, rts), ", ") + ")"
	}
}

func (g *generator) widenAll(srcs []string, rts []types.Type) []string {
	out := make([]string, len(srcs))
	for i, s := range srcs {
		out[i] = g.widen(s, rts[i])
	}
	return out
}

// widen renders a Go result value into its Clojure-normalized form
// (design/05 §2, mirroring pkg/eval/host.go's normalizeResult): Go
// integer/uint kinds → int64, float32/float64 → float64, nilable kinds →
// rt.NilNorm (typed-nil → Clojure nil); everything else boxes unchanged.
func (g *generator) widen(src string, t types.Type) string {
	if b, ok := t.Underlying().(*types.Basic); ok {
		switch info := b.Info(); {
		case info&types.IsInteger != 0:
			return fmt.Sprintf("int64(%s)", src)
		case info&types.IsFloat != 0:
			return fmt.Sprintf("float64(%s)", src)
		}
		return src // bool / string / complex box as-is
	}
	switch t.Underlying().(type) {
	case *types.Pointer, *types.Interface, *types.Map, *types.Slice, *types.Chan, *types.Signature:
		return fmt.Sprintf("rt.NilNorm(%s)", src)
	}
	return src
}

// coerce renders an `any`-typed source as the Go parameter type (Clojure →
// Go), mirroring pkg/eval/host.go's coerceArg: cljgo integers are int64 →
// convert to the exact Go integer type; floats accept int64|float64 via
// rt.ToFloat64; string/bool assert directly.
func (g *generator) coerce(src string, t types.Type) string {
	if b, ok := t.Underlying().(*types.Basic); ok {
		switch info := b.Info(); {
		case info&types.IsString != 0:
			return fmt.Sprintf("%s.(string)", src)
		case info&types.IsBoolean != 0:
			return fmt.Sprintf("%s.(bool)", src)
		case info&types.IsInteger != 0:
			return fmt.Sprintf("%s(%s.(int64))", g.goType(t), src)
		case info&types.IsFloat != 0:
			return fmt.Sprintf("%s(rt.ToFloat64(%s))", g.goType(t), src)
		}
	}
	// Fallback: a direct assertion to the concrete Go type.
	return fmt.Sprintf("%s.(%s)", src, g.goType(t))
}

// goType renders a Go type as source, registering any package qualifier as
// an import (so named external types resolve). The v0 seed surface uses
// only builtins, where the qualifier is never invoked.
func (g *generator) goType(t types.Type) string {
	qual := func(p *types.Package) string { return g.addHostImport(p.Path(), p.Name()) }
	return types.TypeString(t, qual)
}

// paramType returns the Go type of the i-th argument, accounting for a
// variadic tail (each trailing arg takes the slice element type).
func (s hostSig) paramType(i int) types.Type {
	if s.variadic && i >= len(s.params)-1 {
		return s.params[len(s.params)-1].(*types.Slice).Elem()
	}
	return s.params[i]
}
