// Package emit is the Go source emitter (design/04, design/00 §4, ADR
// 0001): it consumes the analyzer's AST — never re-analyzing, never
// holding private special-form knowledge (ADR 0002) — and writes plain
// Go statements into one generated main package, gated through
// go/format.Source.
//
// Emission technique is the S1/S5-validated flattener: every gen(node)
// call writes statements to the buffer and returns the name of an
// r-value (temp var, local, or literal); "" means the node transferred
// control (recur) and produced no value. NO IIFEs (design/04 §3).
// Compound expressions declare `var tmpN any` up front and assign it in
// branches; `_ = x` follows every declaration so Go's unused-variable
// check never fires on conditionally-used values.
//
// Calling convention (ADR 0004): vars deref per call via one atomic
// load (`v.Get()`), never inlined — REPL/compile liveness parity.
// Single-fixed-arity fns (≤ 4 params) emit as lang.FnFunc0..4 so
// call sites through lang.Apply0..4 dispatch with zero []any
// allocation (the S6-winning fixed-arity shape); multi-arity, variadic
// and >4-arity fns fall back to variadic lang.FnFunc with a
// `switch len(args)` dispatch.
package emit

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/muthuishere/cljgo/pkg/ast"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// emitErr carries an unsupported-construct failure out of the recursive
// generator; EmitMain recovers it into an error. Anything else panicking
// through is a bug and propagates.
type emitErr struct{ err error }

// recurFrame is a live recur target: the loop label and the Go names of
// the carriers (the variables recur reassigns), in binding order.
// Frames are keyed by the analyzer's LoopID — unique gensyms, so no
// boundary bookkeeping is needed (the analyzer already rejected any
// recur that would cross an fn or try boundary).
type recurFrame struct {
	label    string
	carriers []string
}

// hoistDecl is one package-level intern, rendered sorted by name for
// deterministic output (design/04 §6).
type hoistDecl struct {
	goName string
	init   string
}

type generator struct {
	buf bytes.Buffer
	id  int // monotonic: temps, locals, loop labels

	// locals maps binding sites to current Go names. The analyzer
	// resolved every OpLocal to its *BindingNode, so pointer identity
	// replaces S5's name/shadow bookkeeping entirely.
	locals map[*ast.BindingNode]string
	frames map[string]*recurFrame // LoopID → active frame

	vars    map[*lang.Var]string // hoisted var interns
	dynVars map[*lang.Var]bool   // emitted with .SetDynamic()
	kws     map[lang.Keyword]string
	syms    map[string]string // quoted symbol full-name → Go name
	taken   map[string]bool   // global-identifier dedup (munge is lossy)
	decls   []hoistDecl

	usesFmt    bool
	usesMath   bool
	usesReader bool
	mainVar    string // hoisted Go name of a def'd -main, if any

	host        *hostFacts        // loaded go/packages type facts (nil = no interop)
	hostImports map[string]string // import path → package name, for interop
}

func newGenerator() *generator {
	return &generator{
		locals:  map[*ast.BindingNode]string{},
		frames:  map[string]*recurFrame{},
		vars:    map[*lang.Var]string{},
		dynVars: map[*lang.Var]bool{},
		kws:     map[lang.Keyword]string{},
		syms:    map[string]string{},
		taken:   map[string]bool{},

		hostImports: map[string]string{},
	}
}

// addHostImport records an interop import (path → package name) and
// returns the local package name used at the call site. Idempotent.
func (g *generator) addHostImport(path, name string) string {
	g.hostImports[path] = name
	return name
}

func (g *generator) wf(format string, a ...any) { fmt.Fprintf(&g.buf, format, a...) }

func (g *generator) next() int { g.id++; return g.id }

func (g *generator) temp() string { return fmt.Sprintf("tmp%d", g.next()) }

func (g *generator) failf(format string, a ...any) string {
	panic(&emitErr{fmt.Errorf(format, a...)})
}

// bindLocal allocates a fresh suffixed Go name for a binding site and
// maps the binding to it. Suffixes come from the one monotonic counter,
// so shadowing and sibling loops can never collide (S1-proven).
func (g *generator) bindLocal(b *ast.BindingNode) string {
	gn := munge(b.Name.Name()) + strconv.Itoa(g.next())
	g.locals[b] = gn
	return gn
}

// uniqueGlobal dedups a munged package-level identifier (munge is not
// injective — MUNGING.md).
func (g *generator) uniqueGlobal(base string) string {
	gn := base
	for i := 2; g.taken[gn]; i++ {
		gn = fmt.Sprintf("%s_%d", base, i)
	}
	g.taken[gn] = true
	return gn
}

// hoistVar interns a Var reference as a package-level
// `var v_… = lang.InternVarName(…)` (idempotent, order-free — design/00
// §4.4 rationale applies to vars exactly as to keywords). Vars whose
// compile-time metadata carries :dynamic are re-marked dynamic in the
// emitted binary so `binding`/`set!` keep working.
func (g *generator) hoistVar(v *lang.Var) string {
	if gn, ok := g.vars[v]; ok {
		return gn
	}
	ns := v.Namespace().Name().String()
	name := v.Symbol().Name()
	gn := g.uniqueGlobal("v_" + munge(ns) + "_" + munge(name))
	init := fmt.Sprintf("lang.InternVarName(lang.NewSymbol(%q), lang.NewSymbol(%q))", ns, name)
	if lang.IsTruthy(lang.Get(v.Meta(), lang.KWDynamic)) {
		init += ".SetDynamic()"
		g.dynVars[v] = true
	}
	g.vars[v] = gn
	g.decls = append(g.decls, hoistDecl{gn, init})
	return gn
}

// hoistRegex interns a #"…" literal as a package-level
// `var re_N = &reader.Regex{Pattern: "…"}`. Deliberately NOT deduped:
// real Clojure's Pattern has no .equals, so two separately-read
// #"same text" literals are NOT `=` (pkg/reader/dispatch.go's oracle),
// and one literal read once IS the same object on every evaluation of
// its form — which is exactly what one package-level var per literal
// site gives (design/00 §4.4's interning rationale, inverted).
func (g *generator) hoistRegex(re *reader.Regex) string {
	g.usesReader = true
	gn := g.uniqueGlobal(fmt.Sprintf("re_%d", g.next()))
	g.decls = append(g.decls, hoistDecl{gn, fmt.Sprintf("&reader.Regex{Pattern: %s}", strconv.Quote(re.Pattern))})
	return gn
}

func (g *generator) hoistKeyword(k lang.Keyword) string {
	if gn, ok := g.kws[k]; ok {
		return gn
	}
	full := strings.TrimPrefix(k.String(), ":")
	gn := g.uniqueGlobal("kw_" + munge(full))
	g.kws[k] = gn
	g.decls = append(g.decls, hoistDecl{gn, fmt.Sprintf("lang.InternKeywordString(%q)", full)})
	return gn
}

func (g *generator) hoistSymbol(s *lang.Symbol) string {
	full := s.FullName()
	if gn, ok := g.syms[full]; ok {
		return gn
	}
	gn := g.uniqueGlobal("sym_" + munge(full))
	g.syms[full] = gn
	g.decls = append(g.decls, hoistDecl{gn, fmt.Sprintf("lang.NewSymbol(%q)", full)})
	return gn
}

// discard emits a use for an r-value we are throwing away. "" (control
// transfer) and the untyped-nil literal are skipped (`_ = nil` is
// illegal Go) — the S1 rules.
func (g *generator) discard(rv string) {
	if rv == "" || rv == "nil" {
		return
	}
	g.wf("_ = %s\n", rv)
}

// ---- constants ------------------------------------------------------------

// formMeta extracts reader metadata (e.g. `^:foo {}`) off an analyzed
// literal form, or nil if there is none. Mirrors pkg/eval's withFormMeta:
// vector/map/set literals rebuild a fresh runtime collection from their
// evaluated items, so without re-attaching this the reader's WithMeta call
// (reader.go readMeta) would be silently dropped by the compiled binary —
// exactly the REPL/binary divergence ADR 0002/0007 forbids.
func formMeta(form any) lang.IPersistentMap {
	im, ok := form.(lang.IMeta)
	if !ok {
		return nil
	}
	return im.Meta()
}

// constExpr renders a compile-time constant (OpConst / OpQuote payload)
// as a Go expression. Keywords and symbols hoist to package-level
// interns (design/00 §4.4); collections construct inline via the pure
// lang constructors (idempotent, deterministic per source order).
func (g *generator) constExpr(v any) string {
	switch x := v.(type) {
	case nil:
		return "nil"
	case bool:
		return strconv.FormatBool(x)
	case int64:
		return fmt.Sprintf("int64(%d)", x)
	case float64:
		// Non-finite and negative-zero doubles have no Go literal
		// spelling that round-trips (`-0` is constant +0): construct
		// them via math. Everything else round-trips through the
		// shortest FormatFloat representation.
		switch {
		case math.IsInf(x, 1):
			g.usesMath = true
			return "math.Inf(1)"
		case math.IsInf(x, -1):
			g.usesMath = true
			return "math.Inf(-1)"
		case math.IsNaN(x):
			g.usesMath = true
			return "math.NaN()"
		case x == 0 && math.Signbit(x):
			g.usesMath = true
			return "math.Copysign(0, -1)"
		}
		return fmt.Sprintf("float64(%s)", strconv.FormatFloat(x, 'g', -1, 64))
	case string:
		return strconv.Quote(x)
	case *lang.BigInt:
		// Numeric-tower literals (1N, 3/2, 1.5M) reconstruct from the exact
		// string cljgo printed, so the compiled constant equals the one the
		// reader built (dual-mode, ADR 0002 / design/08 §5 Batch 2).
		return fmt.Sprintf("lang.MustBigInt(%s)", strconv.Quote(x.String()))
	case *lang.Ratio:
		return fmt.Sprintf("lang.MustRatio(%s)", strconv.Quote(x.String()))
	case *lang.BigDecimal:
		return fmt.Sprintf("lang.MustBigDecimal(%s)", strconv.Quote(x.String()))
	case lang.Char:
		return fmt.Sprintf("lang.Char(%s)", strconv.QuoteRune(rune(x)))
	case *reader.Regex:
		return g.hoistRegex(x)
	case lang.Keyword:
		return g.hoistKeyword(x)
	case *lang.Symbol:
		return g.hoistSymbol(x)
	case lang.IPersistentVector:
		items := make([]string, 0, x.Count())
		for i := 0; i < x.Count(); i++ {
			items = append(items, g.constExpr(x.Nth(i)))
		}
		return "lang.NewVector(" + strings.Join(items, ", ") + ")"
	case lang.IPersistentMap:
		var kvs []string
		for s := lang.Seq(x); s != nil; s = s.Next() {
			e := s.First().(lang.IMapEntry)
			kvs = append(kvs, g.constExpr(e.Key()), g.constExpr(e.Val()))
		}
		return "lang.NewMap(" + strings.Join(kvs, ", ") + ")"
	case lang.IPersistentSet:
		var items []string
		for s := lang.Seq(x); s != nil; s = s.Next() {
			items = append(items, g.constExpr(s.First()))
		}
		return "lang.NewSet(" + strings.Join(items, ", ") + ")"
	case lang.ISeq:
		var items []string
		for s := lang.Seq(x); s != nil; s = s.Next() {
			items = append(items, g.constExpr(s.First()))
		}
		return "lang.NewList(" + strings.Join(items, ", ") + ")"
	}
	return g.failf("emit: unsupported constant type %T (%s)", v, lang.PrintString(v))
}

// ---- recur / capture analysis ----------------------------------------------

// eachChild invokes visit on every direct child node. enterFn reports
// whether the child sits inside a nested fn* relative to n's context
// (used by the capture walk).
func eachChild(n *ast.Node, visit func(child *ast.Node, entersFn bool)) {
	switch n.Op {
	case ast.OpConst, ast.OpQuote, ast.OpVar, ast.OpTheVar, ast.OpLocal, ast.OpHostRef:
	case ast.OpHostCall:
		for _, c := range n.Sub.(*ast.HostCallNode).Args {
			visit(c, false)
		}
	case ast.OpHostMethod:
		s := n.Sub.(*ast.HostMethodNode)
		visit(s.Recv, false)
		for _, c := range s.Args {
			visit(c, false)
		}
	case ast.OpHostField:
		visit(n.Sub.(*ast.HostFieldNode).Recv, false)
	case ast.OpHostNew:
		if f := n.Sub.(*ast.HostNewNode).Fields; f != nil {
			visit(f, false)
		}
	case ast.OpVector:
		for _, c := range n.Sub.(*ast.VectorNode).Items {
			visit(c, false)
		}
	case ast.OpMap:
		s := n.Sub.(*ast.MapNode)
		for i := range s.Keys {
			visit(s.Keys[i], false)
			visit(s.Vals[i], false)
		}
	case ast.OpSet:
		for _, c := range n.Sub.(*ast.SetNode).Items {
			visit(c, false)
		}
	case ast.OpDo:
		s := n.Sub.(*ast.DoNode)
		for _, c := range s.Statements {
			visit(c, false)
		}
		if s.Ret != nil {
			visit(s.Ret, false)
		}
	case ast.OpIf:
		s := n.Sub.(*ast.IfNode)
		visit(s.Test, false)
		visit(s.Then, false)
		visit(s.Else, false)
	case ast.OpDef:
		s := n.Sub.(*ast.DefNode)
		if s.Init != nil {
			visit(s.Init, false)
		}
		if s.Meta != nil {
			visit(s.Meta, false)
		}
	case ast.OpLet, ast.OpLoop:
		s := n.Sub.(*ast.LetNode)
		for _, bn := range s.Bindings {
			if init := bn.Sub.(*ast.BindingNode).Init; init != nil {
				visit(init, false)
			}
		}
		visit(s.Body, false)
	case ast.OpFn:
		for _, mn := range n.Sub.(*ast.FnNode).Methods {
			visit(mn.Sub.(*ast.FnMethodNode).Body, true)
		}
	case ast.OpFnMethod:
		visit(n.Sub.(*ast.FnMethodNode).Body, false)
	case ast.OpInvoke:
		s := n.Sub.(*ast.InvokeNode)
		visit(s.Fn, false)
		for _, c := range s.Args {
			visit(c, false)
		}
	case ast.OpRecur:
		for _, c := range n.Sub.(*ast.RecurNode).Exprs {
			visit(c, false)
		}
	case ast.OpSetBang:
		s := n.Sub.(*ast.SetBangNode)
		visit(s.Target, false)
		visit(s.Val, false)
	case ast.OpDynBind:
		s := n.Sub.(*ast.DynBindNode)
		for i := range s.Vars {
			visit(s.Vars[i], false)
			visit(s.Vals[i], false)
		}
		visit(s.Body, false)
	case ast.OpBinding:
		if init := n.Sub.(*ast.BindingNode).Init; init != nil {
			visit(init, false)
		}
	case ast.OpThrow:
		visit(n.Sub.(*ast.ThrowNode).Exception, false)
	case ast.OpTry:
		s := n.Sub.(*ast.TryNode)
		visit(s.Body, false)
		for _, c := range s.Catches {
			visit(c, false)
		}
		if s.Finally != nil {
			visit(s.Finally, false)
		}
	case ast.OpCatch:
		// The catch binding has no init; only the body has children.
		visit(n.Sub.(*ast.CatchNode).Body, false)
	default:
		panic(&emitErr{fmt.Errorf("emit: walk: unhandled op %v", n.Op)})
	}
}

// recursTo reports whether any recur targeting loopID appears under n.
// LoopIDs are unique analyzer gensyms and the analyzer already rejected
// out-of-frame recurs, so a plain exhaustive walk is exact.
func recursTo(n *ast.Node, loopID string) bool {
	if n.Op == ast.OpRecur && n.Sub.(*ast.RecurNode).LoopID == loopID {
		return true
	}
	found := false
	eachChild(n, func(c *ast.Node, _ bool) {
		if !found && recursTo(c, loopID) {
			found = true
		}
	})
	return found
}

// markCaptured records the targets referenced under a nested fn* within
// n (inFn tracks fn nesting from the walk root). Clojure closures
// capture the VALUE at creation; Go closures capture the VARIABLE — a
// captured recur carrier therefore needs the S5 per-iteration-copy fix.
// TODO(analyzer): this should become a `captured` annotation on the
// binding, set during analysis (S5 RESULTS rule 1); the emitter-side
// walk re-traverses loop bodies.
func markCaptured(n *ast.Node, inFn bool, targets map[*ast.BindingNode]bool, hit map[*ast.BindingNode]bool) {
	if n.Op == ast.OpLocal {
		if b := n.Sub.(*ast.LocalNode).Binding; inFn && targets[b] {
			hit[b] = true
		}
		return
	}
	eachChild(n, func(c *ast.Node, entersFn bool) {
		markCaptured(c, inFn || entersFn, targets, hit)
	})
}

// capturedLoopBindings returns the loop bindings closed over by an fn*
// in the loop BODY or in a LATER binding's init (both S5 case-1 forms).
func capturedLoopBindings(bindings []*ast.Node, body *ast.Node) map[*ast.BindingNode]bool {
	targets := map[*ast.BindingNode]bool{}
	hit := map[*ast.BindingNode]bool{}
	for j, bn := range bindings {
		b := bn.Sub.(*ast.BindingNode)
		if j > 0 && b.Init != nil {
			// init j may close over bindings 0..j-1 (targets so far).
			markCaptured(b.Init, false, targets, hit)
		}
		targets[b] = true
	}
	markCaptured(body, false, targets, hit)
	return hit
}

// capturedParams returns the fn-method params closed over by a nested
// fn* in the method body (S5 case 5c: params are recur carriers too).
func capturedParams(params []*ast.Node, body *ast.Node) map[*ast.BindingNode]bool {
	targets := map[*ast.BindingNode]bool{}
	for _, pn := range params {
		targets[pn.Sub.(*ast.BindingNode)] = true
	}
	hit := map[*ast.BindingNode]bool{}
	markCaptured(body, false, targets, hit)
	return hit
}

// ---- the flattener ----------------------------------------------------------

// gen writes the statements for n and returns the r-value name, or ""
// when the node transferred control (recur) and produced no value.
func (g *generator) gen(n *ast.Node) string {
	switch n.Op {

	case ast.OpConst:
		return g.constExpr(n.Sub.(*ast.ConstNode).Value)

	case ast.OpQuote:
		return g.constExpr(n.Sub.(*ast.QuoteNode).Value)

	case ast.OpVar:
		// Per-call deref, one atomic load (ADR 0004). Materialized into
		// a temp so evaluation order vs. later sibling expressions is
		// exactly Clojure's left-to-right (a sibling side effect that
		// re-defs the var must not be visible to this reference).
		gn := g.hoistVar(n.Sub.(*ast.VarNode).Var)
		t := g.temp()
		g.wf("%s := %s.Get()\n", t, gn)
		return t

	case ast.OpTheVar:
		return g.hoistVar(n.Sub.(*ast.TheVarNode).Var)

	case ast.OpLocal:
		b := n.Sub.(*ast.LocalNode).Binding
		gn, ok := g.locals[b]
		if !ok {
			return g.failf("emit: internal: unresolved local %s", n.Sub.(*ast.LocalNode).Name.Name())
		}
		return gn

	case ast.OpVector:
		items := n.Sub.(*ast.VectorNode).Items
		rvs := make([]string, len(items))
		for i, c := range items {
			rvs[i] = g.gen(c)
		}
		t := g.temp()
		g.wf("%s := lang.NewVector(%s)\n", t, strings.Join(rvs, ", "))
		if m := formMeta(n.Form); m != nil {
			g.wf("%s = %s.WithMeta(%s).(*lang.Vector)\n", t, t, g.constExpr(m))
		}
		return t

	case ast.OpMap:
		s := n.Sub.(*ast.MapNode)
		kvs := make([]string, 0, 2*len(s.Keys))
		for i := range s.Keys {
			kvs = append(kvs, g.gen(s.Keys[i]), g.gen(s.Vals[i]))
		}
		t := g.temp()
		g.wf("%s := lang.NewMap(%s)\n", t, strings.Join(kvs, ", "))
		if m := formMeta(n.Form); m != nil {
			g.wf("%s = %s.(lang.IObj).WithMeta(%s).(lang.IPersistentMap)\n", t, t, g.constExpr(m))
		}
		return t

	case ast.OpSet:
		items := n.Sub.(*ast.SetNode).Items
		rvs := make([]string, len(items))
		for i, c := range items {
			rvs[i] = g.gen(c)
		}
		t := g.temp()
		g.wf("%s := lang.NewSet(%s)\n", t, strings.Join(rvs, ", "))
		if m := formMeta(n.Form); m != nil {
			g.wf("%s = %s.WithMeta(%s).(*lang.Set)\n", t, t, g.constExpr(m))
		}
		return t

	case ast.OpDo:
		s := n.Sub.(*ast.DoNode)
		for _, stmt := range s.Statements {
			g.discard(g.gen(stmt))
		}
		return g.gen(s.Ret)

	case ast.OpIf:
		s := n.Sub.(*ast.IfNode)
		var condStmt string
		if boolRv, ok := g.genTestIntrinsic(s.Test); ok {
			condStmt = fmt.Sprintf("if %s {\n", boolRv)
		} else {
			condStmt = fmt.Sprintf("if lang.IsTruthy(%s) {\n", g.gen(s.Test))
		}
		t := g.temp()
		g.wf("var %s any\n_ = %s\n", t, t) // both branches may recur
		g.wf("%s", condStmt)
		thenRv := g.gen(s.Then)
		if thenRv != "" {
			g.wf("%s = %s\n", t, thenRv)
		}
		g.wf("} else {\n")
		elseRv := g.gen(s.Else)
		if elseRv != "" {
			g.wf("%s = %s\n", t, elseRv)
		}
		g.wf("}\n")
		if thenRv == "" && elseRv == "" {
			// Both branches transferred control (recur/throw): the if
			// produces no value and nothing after it is reachable. Say so
			// ("" = control transfer) instead of handing back a temp the
			// caller would assign from unreachable code — `go vet`'s
			// unreachable check is a gate, and pkg/coreaot's generated
			// packages are vetted like any other package in this repo.
			return ""
		}
		return t

	case ast.OpDef:
		s := n.Sub.(*ast.DefNode)
		gv := g.hoistVar(s.Var)
		if s.Name.Name() == "-main" {
			g.mainVar = gv
		}
		if s.Meta != nil {
			// Re-apply the def's constant metadata (parseDef put the same
			// map on the compile-time var): :private/:declared/:doc and
			// reader position must read back identically from (meta #'v)
			// in a compiled binary (dual-mode meta, ADR 0002/0007).
			// s.Meta is always an OpQuote of an IPersistentMap, so gen
			// yields a constExpr whose Go type is lang.IPersistentMap.
			mrv := g.gen(s.Meta)
			g.wf("%s.SetMeta(%s)\n", gv, mrv)
		}
		if s.Init != nil {
			rv := g.gen(s.Init)
			g.wf("%s.BindRoot(%s)\n", gv, rv)
		}
		return gv // def's value is the Var itself

	case ast.OpLet:
		s := n.Sub.(*ast.LetNode)
		t := g.temp()
		g.wf("var %s any\n_ = %s\n", t, t)
		g.wf("{\n")
		for _, bn := range s.Bindings {
			b := bn.Sub.(*ast.BindingNode)
			rv := g.gen(b.Init) // sequential: init sees earlier bindings
			gn := g.bindLocal(b)
			g.wf("var %s any = %s\n_ = %s\n", gn, rv, gn)
		}
		bodyRv := g.gen(s.Body)
		if bodyRv != "" {
			g.wf("%s = %s\n", t, bodyRv)
		}
		g.wf("}\n")
		if bodyRv == "" {
			return "" // the body transferred control; propagate (see OpIf)
		}
		return t

	case ast.OpLoop:
		return g.genLoop(n.Sub.(*ast.LetNode))

	case ast.OpFn:
		return g.genFn(n.Sub.(*ast.FnNode))

	case ast.OpInvoke:
		s := n.Sub.(*ast.InvokeNode)
		if t, ok := g.genIntrinsic(s); ok {
			return t
		}
		frv := g.gen(s.Fn) // fn position evaluates first
		rvs := make([]string, len(s.Args))
		for i, a := range s.Args {
			rvs[i] = g.gen(a)
		}
		t := g.temp()
		if len(rvs) <= 4 {
			// Fixed-arity fast path: Apply0..4 dispatch FnFunc0..4
			// directly — no []any allocation (ADR 0004 / S6).
			g.wf("%s := lang.Apply%d(%s)\n", t, len(rvs),
				strings.Join(append([]string{frv}, rvs...), ", "))
		} else {
			g.wf("%s := lang.Apply(%s, []any{%s})\n", t, frv, strings.Join(rvs, ", "))
		}
		return t

	case ast.OpRecur:
		s := n.Sub.(*ast.RecurNode)
		fr := g.frames[s.LoopID]
		if fr == nil {
			return g.failf("emit: internal: recur to unknown frame %s", s.LoopID)
		}
		// Simultaneous rebinding: ALL new values into temps first, then
		// assign the carriers — a later carrier must not see an earlier
		// rebind (S5 case 4).
		temps := make([]string, len(s.Exprs))
		for i, ex := range s.Exprs {
			rv := g.gen(ex)
			tt := g.temp()
			g.wf("var %s any = %s\n", tt, rv)
			temps[i] = tt
		}
		for i, c := range fr.carriers {
			g.wf("%s = %s\n", c, temps[i])
		}
		g.wf("continue %s\n", fr.label)
		return ""

	case ast.OpSetBang:
		s := n.Sub.(*ast.SetBangNode)
		// A Go field target (`(set! (.-Field recv) v)`) assigns via the
		// shared rt.FieldSet → eval.GoFieldSet — the SAME path the
		// interpreter takes, byte-identical (ADR 0010, design/05 §1).
		if s.Target.Op == ast.OpHostField {
			f := s.Target.Sub.(*ast.HostFieldNode)
			recvT := g.temp()
			g.wf("var %s any = %s\n", recvT, g.gen(f.Recv))
			valT := g.temp()
			g.wf("var %s any = %s\n", valT, g.gen(s.Val))
			t := g.temp()
			g.wf("var %s any = rt.FieldSet(%s, %q, %s)\n", t, recvT, f.Field, valT)
			return t
		}
		gv := g.hoistVar(s.Target.Sub.(*ast.VarNode).Var)
		rv := g.gen(s.Val)
		t := g.temp()
		g.wf("%s := %s.Set(%s)\n", t, gv, rv)
		return t

	case ast.OpDynBind:
		s := n.Sub.(*ast.DynBindNode)
		// Vals evaluate before any binding is pushed (parallel binding
		// semantics, as the evaluator). Emission is flat push/…/pop —
		// no closure (the no-IIFE invariant): v0 has no catch, so a
		// panic unwinding past the pop terminates the process anyway;
		// v1's try/catch brings the real finally mechanism.
		kvs := make([]string, 0, 2*len(s.Vars))
		for i := range s.Vars {
			gv := g.hoistVar(s.Vars[i].Sub.(*ast.VarNode).Var)
			rv := g.gen(s.Vals[i])
			kvs = append(kvs, gv, rv)
		}
		g.wf("lang.PushThreadBindings(lang.NewMap(%s))\n", strings.Join(kvs, ", "))
		rv := g.gen(s.Body) // recur across binding is an analysis error
		t := g.temp()
		g.wf("var %s any = %s\n", t, rv)
		g.wf("lang.PopThreadBindings()\n")
		return t

	case ast.OpHostRef, ast.OpHostCall, ast.OpHostMethod, ast.OpHostField, ast.OpHostNew:
		// Go interop (ADR 0010, M3-v0/M3.1/M3.2). AOT direct-call emission —
		// go/packages signature resolution, real imports, [v err]/!
		// shaping, go.mod pinning — lands in host.go (ports spike S2).
		// Method calls (OpHostMethod), field access (OpHostField) and struct
		// construction (OpHostNew) emit reflective rt.CallMethod / rt.FieldGet
		// / rt.MakeStruct / rt.NewStruct — the SAME shared eval fns the
		// interpreter uses, byte-identical by construction.
		return g.genHost(n)

	case ast.OpThrow:
		// throw = panic; the value transferred control and yields no
		// r-value (like recur, "" tells callers not to assign). rt.Throw
		// wraps a non-error so the catch-all classes still catch it — the
		// SAME eval.Throw the interpreter uses, byte-identical.
		s := n.Sub.(*ast.ThrowNode)
		rv := g.gen(s.Exception)
		g.wf("panic(rt.Throw(%s))\n", rv)
		return ""

	case ast.OpTry:
		return g.genTry(n.Sub.(*ast.TryNode))

	case ast.OpBinding, ast.OpFnMethod, ast.OpCatch:
		return g.failf("emit: internal: op %v is not directly emittable", n.Op)

	default:
		return g.failf("emit: unhandled op %v (new op must land in evaluator AND emitter together, design/00 §2)", n.Op)
	}
}

// intrinsic2 maps 2-argument clojure.core arithmetic builtins to their
// pkg/emit/rt guarded helpers. Each helper still derefs the var per
// call and falls back through the current value on redefinition (ADR
// 0004 liveness); pristine builtins open-code the int64 fast path —
// without this, every arithmetic op pays the variadic nativeFn's []any
// allocation and the M2 factorial budget (design/00 §1.4) is blown by
// an order of magnitude (measured: 168× raw; see perf_test.go).
var intrinsic2 = map[string]string{
	"+": "Add2", "-": "Sub2", "*": "Mul2", "/": "Div2",
	"<": "LT2", ">": "GT2", "=": "EQ2",
}

// testIntrinsics are the comparison builtins with unboxed bool variants
// for `if` tests (no interface boxing, no IsTruthy round-trip).
var testIntrinsics = map[string]string{"<": "LTBool", ">": "GTBool", "=": "EQBool"}

// genTestIntrinsic emits an unboxed comparison for an if-test that is a
// 2-arg call of a core comparison builtin, returning a bool-typed
// r-value. The guard inside the rt helper preserves redefinition
// semantics (a redefined comparison's value goes through IsTruthy —
// exactly what the generic emission would do).
func (g *generator) genTestIntrinsic(test *ast.Node) (string, bool) {
	if test.Op != ast.OpInvoke {
		return "", false
	}
	s := test.Sub.(*ast.InvokeNode)
	if len(s.Args) != 2 || s.Fn.Op != ast.OpVar {
		return "", false
	}
	v := s.Fn.Sub.(*ast.VarNode).Var
	if v.Namespace() != lang.NSCore {
		return "", false
	}
	helper, ok := testIntrinsics[v.Symbol().Name()]
	if !ok {
		return "", false
	}
	gv := g.hoistVar(v)
	a := g.gen(s.Args[0])
	b := g.gen(s.Args[1])
	t := g.temp()
	g.wf("%s := rt.%s(%s, %s, %s)\n", t, helper, gv, a, b)
	return t, true
}

// genIntrinsic emits a guarded arithmetic intrinsic call when the
// invoke is a 2-arg call of a clojure.core arithmetic builtin var.
func (g *generator) genIntrinsic(s *ast.InvokeNode) (string, bool) {
	if len(s.Args) != 2 || s.Fn.Op != ast.OpVar {
		return "", false
	}
	v := s.Fn.Sub.(*ast.VarNode).Var
	if v.Namespace() != lang.NSCore {
		return "", false
	}
	helper, ok := intrinsic2[v.Symbol().Name()]
	if !ok {
		return "", false
	}
	gv := g.hoistVar(v)
	a := g.gen(s.Args[0])
	b := g.gen(s.Args[1])
	t := g.temp()
	g.wf("%s := rt.%s(%s, %s, %s)\n", t, helper, gv, a, b)
	return t, true
}

// genLoop emits loop* — S5's proven shape: binding vars (never
// reassigned), separate carriers for captured bindings, labeled `for {}`
// (label only when a recur targets it), per-iteration copies at the top
// of the body for captured carriers, break out with the result.
func (g *generator) genLoop(s *ast.LetNode) string {
	t := g.temp()
	g.wf("var %s any\n_ = %s\n", t, t)
	g.wf("{\n")

	bnodes := make([]*ast.BindingNode, len(s.Bindings))
	bnames := make([]string, len(s.Bindings))
	for i, bn := range s.Bindings {
		b := bn.Sub.(*ast.BindingNode)
		rv := g.gen(b.Init)
		gn := g.bindLocal(b)
		g.wf("var %s any = %s\n_ = %s\n", gn, rv, gn)
		bnodes[i] = b
		bnames[i] = gn
	}

	recurs := recursTo(s.Body, s.LoopID)
	var captured map[*ast.BindingNode]bool
	if recurs {
		captured = capturedLoopBindings(s.Bindings, s.Body)
	}

	// Carriers are what recur reassigns. A captured binding's carrier
	// must be a SEPARATE variable from the binding var: an init-position
	// closure captured the binding var and must keep seeing the initial
	// value (S5 case 1b); only the carrier is rebound.
	carriers := make([]string, len(bnames))
	copy(carriers, bnames)
	for i, b := range bnodes {
		if captured[b] {
			cn := fmt.Sprintf("%s_c%d", munge(b.Name.Name()), g.next())
			g.wf("var %s any = %s\n_ = %s\n", cn, bnames[i], cn)
			carriers[i] = cn
		}
	}

	label := fmt.Sprintf("loop%d", g.next())
	if recurs {
		g.wf("%s:\n", label) // Go rejects unused labels: only when recurred to
		g.frames[s.LoopID] = &recurFrame{label: label, carriers: carriers}
	}
	g.wf("for {\n")
	// Per-iteration copies: body READS of a captured binding resolve to
	// a fresh copy (closures made this iteration capture this
	// iteration's value); recur WRITES still hit the outer carrier.
	for i, b := range bnodes {
		if captured[b] {
			fresh := g.bindLocal(b)
			g.wf("%s := %s\n_ = %s\n", fresh, carriers[i], fresh)
		}
	}
	if rv := g.gen(s.Body); rv != "" {
		g.wf("%s = %s\n", t, rv)
	}
	if recurs {
		delete(g.frames, s.LoopID)
		g.wf("break %s\n", label)
	} else {
		g.wf("break\n")
	}
	g.wf("}\n")
	g.wf("}\n")
	return t
}

// genTry emits (try body* catch* finally?). This is the ONE sanctioned
// IIFE for control flow (design/04 §3): try genuinely needs a func
// boundary for Go's defer/recover. The result var is declared OUTSIDE the
// closure and assigned by the body (normal path) or a catch body (recover
// path); a finally runs via a deferred closure with its value discarded.
// Defers are LIFO, so the catch-defer (emitted last) recovers first and
// the finally-defer (emitted first) runs after — finally on every path,
// including an uncaught throw unwinding through the closure. recur cannot
// cross a try (analysis-blocked), so no `continue` ever needs to escape
// the closure. rt.Throw / rt.Recover / rt.CatchMatches are the SAME
// eval.* functions the interpreter uses, so both modes are byte-identical.
func (g *generator) genTry(s *ast.TryNode) string {
	t := g.temp()
	g.wf("var %s any\n_ = %s\n", t, t)
	g.wf("func() {\n")

	if s.Finally != nil {
		g.wf("defer func() {\n")
		g.discard(g.gen(s.Finally)) // finally value discarded
		g.wf("}()\n")
	}

	if len(s.Catches) > 0 {
		g.wf("defer func() {\n")
		g.wf("if r := recover(); r != nil {\n")
		g.wf("thrown := rt.Recover(r)\n")
		for _, cn := range s.Catches {
			c := cn.Sub.(*ast.CatchNode)
			g.wf("if rt.CatchMatches(%q, thrown) {\n", c.ClassName)
			b := c.Binding.Sub.(*ast.BindingNode)
			gn := g.bindLocal(b)
			g.wf("var %s any = thrown\n_ = %s\n", gn, gn)
			if rv := g.gen(c.Body); rv != "" {
				g.wf("%s = %s\n", t, rv)
			}
			g.wf("return\n")
			g.wf("}\n")
		}
		g.wf("panic(r)\n") // no catch matched: re-throw (finally still runs)
		g.wf("}\n")
		g.wf("}()\n")
	}

	if rv := g.gen(s.Body); rv != "" {
		g.wf("%s = %s\n", t, rv)
	}
	g.wf("}()\n")
	return t
}

// genFn emits an fn* value.
//
//   - single fixed-arity method with ≤ 4 params → lang.FnFuncN closure
//     (params are real Go params) — the ADR 0004 default shape.
//   - otherwise → variadic lang.FnFunc with `switch len(args)` dispatch,
//     variadic method as default with floor check (design/04 §4,
//     Glojure's scheme, RestFn semantics).
//
// A self-name binds via a pre-declared Go var captured by the closure
// and assigned right after construction — calls can only happen later.
func (g *generator) genFn(fn *ast.FnNode) string {
	name := "fn"
	selfGo := ""
	if fn.Local != nil {
		b := fn.Local.Sub.(*ast.BindingNode)
		name = b.Name.Name()
		selfGo = g.bindLocal(b)
		g.wf("var %s any\n_ = %s\n", selfGo, selfGo)
	}

	t := g.temp()
	if m := singleFixedMethod(fn); m != nil {
		gnames := make([]string, len(m.Params))
		for i, pn := range m.Params {
			gnames[i] = g.bindLocal(pn.Sub.(*ast.BindingNode))
		}
		params := ""
		if len(gnames) > 0 {
			params = strings.Join(gnames, ", ") + " any"
		}
		g.wf("%s := lang.FnFunc%d(func(%s) any {\n", t, m.FixedArity, params)
		g.genMethodBody(m, gnames)
		g.wf("})\n")
	} else {
		g.usesFmt = true
		g.wf("%s := lang.FnFunc(func(args ...any) any {\n", t)
		g.wf("switch len(args) {\n")
		var variadic *ast.FnMethodNode
		methods := make([]*ast.FnMethodNode, 0, len(fn.Methods))
		for _, mn := range fn.Methods {
			m := mn.Sub.(*ast.FnMethodNode)
			if m.IsVariadic {
				variadic = m
				continue
			}
			methods = append(methods, m)
		}
		sort.Slice(methods, func(i, j int) bool { return methods[i].FixedArity < methods[j].FixedArity })
		for _, m := range methods {
			g.wf("case %d:\n", m.FixedArity)
			g.genArgsBinding(m)
		}
		g.wf("default:\n")
		if variadic != nil {
			g.wf("if len(args) < %d {\n", variadic.FixedArity)
			g.wf("panic(fmt.Errorf(\"wrong number of args (%%d) passed to: %%s\", len(args), %q))\n", name)
			g.wf("}\n")
			g.genArgsBinding(variadic)
		} else {
			g.wf("panic(fmt.Errorf(\"wrong number of args (%%d) passed to: %%s\", len(args), %q))\n", name)
		}
		g.wf("}\n")
		g.wf("})\n")
	}

	if selfGo != "" {
		g.wf("%s = %s\n", selfGo, t)
	}
	return t
}

// singleFixedMethod returns the sole method when the fn qualifies for
// the fixed-arity FnFuncN representation, else nil.
func singleFixedMethod(fn *ast.FnNode) *ast.FnMethodNode {
	if len(fn.Methods) != 1 || fn.IsVariadic {
		return nil
	}
	m := fn.Methods[0].Sub.(*ast.FnMethodNode)
	if m.IsVariadic || m.FixedArity > 4 {
		return nil
	}
	return m
}

// genArgsBinding binds a method's params from the variadic `args` slice
// (fixed prefix by index; rest packs to a list, nil when empty — exactly
// evalFn) and emits the method body.
func (g *generator) genArgsBinding(m *ast.FnMethodNode) {
	gnames := make([]string, len(m.Params))
	for i, pn := range m.Params {
		b := pn.Sub.(*ast.BindingNode)
		gp := g.bindLocal(b)
		gnames[i] = gp
		if b.IsVariadic {
			g.wf("var %s any\n", gp)
			g.wf("if len(args) > %d {\n%s = lang.NewList(args[%d:]...)\n}\n", m.FixedArity, gp, m.FixedArity)
			g.wf("_ = %s\n", gp)
		} else {
			g.wf("%s := args[%d]\n_ = %s\n", gp, i, gp)
		}
	}
	g.genMethodBody(m, gnames)
}

// genMethodBody emits a method body whose params (gnames) are the recur
// carriers. With recur: labeled `for {}` + continue (S5 rule 3 — no
// goto, no trailing return: `for{}` with no break is a terminating
// statement), per-iteration copies for closure-captured params (case
// 5c). Without: plain `return rv`.
func (g *generator) genMethodBody(m *ast.FnMethodNode, gnames []string) {
	if !recursTo(m.Body, m.LoopID) {
		rv := g.gen(m.Body)
		if rv == "" {
			// The body transferred control on every path (e.g. a fn whose
			// whole body is a throw, like with-redefs wrapping a throwing
			// body): the emitted panic is a Go terminating statement, so
			// the method needs no return.
			return
		}
		g.wf("return %s\n", rv)
		return
	}
	captured := capturedParams(m.Params, m.Body)
	label := fmt.Sprintf("fnloop%d", g.next())
	g.frames[m.LoopID] = &recurFrame{label: label, carriers: gnames}
	g.wf("%s:\n", label)
	g.wf("for {\n")
	for i, pn := range m.Params {
		b := pn.Sub.(*ast.BindingNode)
		if captured[b] {
			fresh := g.bindLocal(b)
			g.wf("%s := %s\n_ = %s\n", fresh, gnames[i], fresh)
		}
	}
	rv := g.gen(m.Body)
	if rv != "" {
		g.wf("return %s\n", rv)
	}
	delete(g.frames, m.LoopID)
	g.wf("}\n")
}
