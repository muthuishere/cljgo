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

// directFn records a local binding whose value is a statically-known,
// single-fixed-arity fn* (≤ 4 params, non-variadic) reachable through a
// TYPED Go handle (a lang.FnFuncN-typed variable or temp holding that
// exact closure). A call to such a binding of matching arity can bypass
// lang.ApplyN's type-switch dispatch and invoke the closure value
// directly (ADR 0064). goName is the typed handle; arity its fixed arity.
type directFn struct {
	goName string
	arity  int
}

type generator struct {
	buf bytes.Buffer
	id  int // monotonic: temps, locals, loop labels

	// locals maps binding sites to current Go names. The analyzer
	// resolved every OpLocal to its *BindingNode, so pointer identity
	// replaces S5's name/shadow bookkeeping entirely.
	locals map[*ast.BindingNode]string
	frames map[string]*recurFrame // LoopID → active frame

	// directFns maps a local fn binding (a named fn's self-name, or a
	// let-bound single-fixed-arity fn) to its typed Go handle, so a call
	// of matching arity emits a direct closure invocation instead of
	// lang.ApplyN (ADR 0064). Keyed by binding pointer identity, so
	// shadowing never mis-resolves.
	directFns map[*ast.BindingNode]directFn

	vars    map[*lang.Var]string // hoisted var interns
	dynVars map[*lang.Var]bool   // emitted with .SetDynamic()
	kws     map[lang.Keyword]string
	syms    map[string]string // quoted symbol full-name → Go name
	taken   map[string]bool   // global-identifier dedup (munge is lossy)
	decls   []hoistDecl

	usesFmt    bool
	usesMath   bool
	usesReader bool
	mainVar    string    // hoisted Go name of a def'd -main, if any
	defName    string    // qualified name of the Var an fn is being def'd into,
	defVar     *lang.Var // the Var an fn is being def'd into (ADR 0067 self-call typing)
	// so an anonymous (defn f …)'s fn* names its arity error user/f (ADR 0048)

	host        *hostFacts        // loaded go/packages type facts (nil = no interop)
	hostImports map[string]string // import path → package name, for interop

	// funcs holds package-level typed Go function declarations lifted from
	// self-recursive int64 fns (ADR 0067 rung 3): `func factL(n int64)
	// int64 {…}` with DIRECT int64 recursion, so the recursive return never
	// crosses the `any` FnFunc boundary and never re-boxes. Emitted after
	// the var block.
	funcs []string
	// selfDirect, when set, tells genSelfCallInt to emit a direct call to
	// the lifted typed func (goName) for a self-recursive call, instead of
	// the boxed Apply+MustInt64 dispatch.
	selfDirect *selfFn

	// ni is the ACTIVE numeric type-inference scope (spike s42 / ADR 0067):
	// gen consults it for every node/binding to decide int64 vs boxed. It is
	// swapped (save/restore) per region — a dual-body specialized fn emits
	// its typed and boxed halves under two different scopes. emptyInfer()
	// (types nothing) is the default, so everything outside a guarded
	// region stays boxed.
	ni *numInfer
	// guarded is true while emitting inside an `if !rt.CoreDirty()` typed
	// region (a specialized fn fast path, a lifted typed func, or the typed
	// arm of a dual-emitted loop). Unboxed emission only ever happens with
	// guarded set — that is the ADR 0066/0067 redefinition contract — and a
	// loop met while unguarded starts its own guarded dual emission.
	guarded bool
	// boxedForced is true in the else-arm of a dual-emitted loop: the dirty
	// flag is sticky (never cleared), so nested loops there must not emit
	// their own dead `if !rt.CoreDirty()` arms.
	boxedForced bool
}

func newGenerator() *generator {
	return &generator{
		locals:    map[*ast.BindingNode]string{},
		frames:    map[string]*recurFrame{},
		directFns: map[*ast.BindingNode]directFn{},
		vars:      map[*lang.Var]string{},
		dynVars:   map[*lang.Var]bool{},
		kws:       map[lang.Keyword]string{},
		syms:      map[string]string{},
		taken:     map[string]bool{},

		hostImports: map[string]string{},
		ni:          emptyInfer(),
	}
}

// bindGoType is the Go type of the local the emitter declares for binding
// b: "int64" when inference proved it int64 (ADR 0067), else "any".
func (g *generator) bindGoType(b *ast.BindingNode) string {
	if g.ni.bindInt64(b) {
		return "int64"
	}
	return "any"
}

// emptyInfer types nothing: the active g.ni when no numeric scope is in
// effect, so isInt64/bindInt64 return false and every path stays boxed.
func emptyInfer() *numInfer {
	return &numInfer{bind: map[*ast.BindingNode]numType{}, node: map[*ast.Node]numType{}}
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
// emitted binary so `binding`/`set!` keep working; :private is replayed
// the same way so ns-publics/ns-interns agree across modes.
func (g *generator) hoistVar(v *lang.Var) string {
	if gn, ok := g.vars[v]; ok {
		return gn
	}
	ns := v.Namespace().Name().String()
	name := v.Symbol().Name()
	gn := g.uniqueGlobal("v_" + munge(ns) + "_" + munge(name))
	init := fmt.Sprintf("lang.InternVarName(lang.NewSymbol(%q), lang.NewSymbol(%q))", ns, name)
	// Class-ref vars (ADR 0036) are interned lazily BY the interpreter's
	// resolveVar fallback, value included; re-interning by name alone
	// would leave them unbound in the binary (REPL/binary divergence,
	// ADR 0002/0007). rt.ClassRefVar re-runs that fallback at runtime.
	if ns == "cljgo.classes" {
		init = fmt.Sprintf("rt.ClassRefVar(%q)", name)
	}
	if lang.IsTruthy(lang.Get(v.Meta(), lang.KWDynamic)) {
		init += ".SetDynamic()"
		g.dynVars[v] = true
	}
	// ^:private is replayed the same way (fundamentals audit 2026-07):
	// def meta is applied at ANALYSIS time (analyzer parseDef), so the
	// compile-process var carries it but the binary's re-interned var
	// would not — and ns-publics/ns-interns would diverge between modes.
	if lang.IsTruthy(lang.Get(v.Meta(), lang.KWPrivate)) {
		init += ".SetPrivate()"
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
	case reader.Inst:
		// #inst constants reconstruct from the literal timestamp text the
		// compile-time reader already validated (reader.Inst round-trips
		// verbatim — pkg/reader/tagged.go), so the compiled value equals
		// the interpreter's, epoch millis included.
		g.usesReader = true
		return fmt.Sprintf("reader.MustInst(%q)", x.Value())
	case *reader.UUID:
		// #uuid constants reconstruct the same way (reader.MustUUID over
		// the canonical lowercase text the compile-time reader validated),
		// so compiled and interpreted #uuid values are `=` and print
		// identically (tail wave, 2026-07-23; the MustInst pattern).
		g.usesReader = true
		return fmt.Sprintf("reader.MustUUID(%q)", x.Value())
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
		g.wf("var %s %s\n_ = %s\n", t, g.ni.goType(n), t) // both branches may recur
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
			// Thread the Var's qualified name onto an fn* init so its arity
			// error names the fn (user/f) like the interpreter, not "fn"
			// (ADR 0048): (defn f …) builds an anonymous fn* whose own self-
			// name is "fn". Consumed and cleared by genFn.
			if s.Init.Op == ast.OpFn && s.Var != nil {
				g.defName = s.Var.ToSymbol().String()
				g.defVar = s.Var
			}
			rv := g.gen(s.Init)
			g.defName = ""
			g.defVar = nil
			g.wf("%s.BindRoot(%s)\n", gv, rv)
		}
		return gv // def's value is the Var itself

	case ast.OpLet:
		s := n.Sub.(*ast.LetNode)
		t := g.temp()
		g.wf("var %s %s\n_ = %s\n", t, g.ni.goType(n), t)
		g.wf("{\n")
		for _, bn := range s.Bindings {
			b := bn.Sub.(*ast.BindingNode)
			rv := g.gen(b.Init) // sequential: init sees earlier bindings
			gn := g.bindLocal(b)
			// ADR 0067: an int64-proven binding declares as a raw Go int64;
			// an fn-valued binding is never int64, so this composes with the
			// ADR 0064 direct-call registration below.
			g.wf("var %s %s = %s\n_ = %s\n", gn, g.bindGoType(b), rv, gn)
			// A let-bound single-fixed-arity fn* can be called directly
			// through its typed temp (ADR 0064). let bindings are
			// immutable, so the temp holds this exact closure for the whole
			// block (and any nested closure that captures it); rv is the
			// *lang.NamedFnN temp genFn returned. Registered after this
			// binding so later inits and the body resolve it, but not this
			// init (a self-recursive call there is the fn's own self-name,
			// handled inside genFn).
			if b.Init.Op == ast.OpFn {
				if m := singleFixedMethod(b.Init.Sub.(*ast.FnNode)); m != nil {
					// rv is the *lang.NamedFnN temp genFn returned; its F
					// field is the raw FnFuncN closure the direct call
					// invokes (ADR 0048 named wrapper + ADR 0064).
					g.directFns[b] = directFn{goName: rv + ".F", arity: m.FixedArity}
				}
			}
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
		return g.genLoop(n)

	case ast.OpFn:
		return g.genFn(n.Sub.(*ast.FnNode))

	case ast.OpInvoke:
		s := n.Sub.(*ast.InvokeNode)
		if t, ok := g.genSelfCallInt(n, s); ok {
			return t
		}
		if t, ok := g.genIntrinsic(n, s); ok {
			return t
		}
		// Direct-call fast path (ADR 0064): the callee is a local binding
		// known to hold a specific single-fixed-arity closure (a named
		// fn's self-name, or a let-bound fn) AND the call arity matches.
		// Invoke the typed closure handle directly, skipping ApplyN's
		// type-switch dispatch. The fn position is a side-effect-free
		// local read, so evaluating the args first is order-preserving.
		// Any mismatch (wrong arity, non-local, reassigned carrier) simply
		// isn't registered and falls through to the ApplyN path below,
		// which keeps exact arity-error semantics.
		if s.Fn.Op == ast.OpLocal {
			if df, ok := g.directFns[s.Fn.Sub.(*ast.LocalNode).Binding]; ok && df.arity == len(s.Args) {
				rvs := make([]string, len(s.Args))
				for i, a := range s.Args {
					rvs[i] = g.gen(a)
				}
				t := g.temp()
				g.wf("%s := %s(%s)\n", t, df.goName, strings.Join(rvs, ", "))
				return t
			}
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
			g.wf("var %s %s = %s\n", tt, g.ni.goType(ex), rv)
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
// pkg/emit/rt guarded helpers. Each helper checks the sealed-core dirty
// flag once (ADR 0066 / spike s43) and only derefs the var + falls back
// through the current value when a redefinition has tripped it (ADR
// 0004 liveness, preserved via the escape hatch); pristine builtins
// open-code the int64 fast path —
// without this, every arithmetic op pays the variadic nativeFn's []any
// allocation and the M2 factorial budget (design/00 §1.4) is blown by
// an order of magnitude (measured: 168× raw; see perf_test.go).
var intrinsic2 = map[string]string{
	"+": "Add2", "-": "Sub2", "*": "Mul2", "/": "Div2",
	"<": "LT2", ">": "GT2", "=": "EQ2", "<=": "LE2", ">=": "GE2",
}

// testIntrinsics are the comparison builtins with unboxed bool variants
// for `if` tests (no interface boxing, no IsTruthy round-trip).
var testIntrinsics = map[string]string{
	"<": "LTBool", ">": "GTBool", "=": "EQBool", "<=": "LEBool", ">=": "GEBool",
}

// intUnboxCmp maps a proven-int64 comparison to a raw Go operator (ADR
// 0067): when the inference pass proved both operands int64, the compare
// is a single Go instruction — no var deref, no boxing. For two int64,
// raw `<`/`>`/`==` is byte-identical to LT/GT/Equiv.
var intUnboxCmp = map[string]string{"<": "<", ">": ">", "=": "==", "<=": "<=", ">=": ">="}

// intUnboxArith2/1 map proven-int64 arithmetic to the checked rt helpers
// on raw Go int64s (ADR 0067). These do NOT deref the operator var — the
// design/04 §5 rung-4 primitive-intrinsic contract — but they reproduce
// the tower's overflow tests exactly, so the "integer overflow" throw
// stays byte-identical. `/` is never here (ratio semantics live in the
// tower).
var intUnboxArith2 = map[string]string{"+": "IAdd", "-": "ISub", "*": "IMul"}
var intUnboxArith1 = map[string]string{"inc": "IInc", "dec": "IDec"}

// genTestIntrinsic emits an unboxed comparison for an if-test that is a
// 2-arg call of a core comparison builtin, returning a bool-typed
// r-value. When both operands were proven int64 (ADR 0067) it emits a raw
// Go comparison; otherwise the rt helper's guard preserves redefinition
// semantics (a redefined comparison goes through IsTruthy).
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
	name := v.Symbol().Name()
	if op, ok := intUnboxCmp[name]; ok && g.ni.isInt64(s.Args[0]) && g.ni.isInt64(s.Args[1]) {
		a := g.gen(s.Args[0])
		b := g.gen(s.Args[1])
		t := g.temp()
		g.wf("%s := %s %s %s\n", t, a, op, b)
		return t, true
	}
	helper, ok := testIntrinsics[name]
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

// genIntrinsic emits an arithmetic intrinsic. When inference proved the
// call yields int64 from int64 operands (ADR 0067) it emits raw checked
// int64 arithmetic (rt.IAdd/ISub/IMul/IInc/IDec) that keeps the value
// unboxed; otherwise it falls back to the guarded boxed helper (rt.Add2…)
// on a 2-arg core call.
func (g *generator) genIntrinsic(n *ast.Node, s *ast.InvokeNode) (string, bool) {
	if s.Fn.Op != ast.OpVar {
		return "", false
	}
	v := s.Fn.Sub.(*ast.VarNode).Var
	if v.Namespace() != lang.NSCore {
		return "", false
	}
	name := v.Symbol().Name()

	if g.ni.isInt64(n) {
		if helper, ok := intUnboxArith2[name]; ok && len(s.Args) == 2 {
			a := g.gen(s.Args[0])
			b := g.gen(s.Args[1])
			t := g.temp()
			g.wf("var %s int64 = rt.%s(%s, %s)\n", t, helper, a, b)
			return t, true
		}
		if helper, ok := intUnboxArith1[name]; ok && len(s.Args) == 1 {
			a := g.gen(s.Args[0])
			t := g.temp()
			g.wf("var %s int64 = rt.%s(%s)\n", t, helper, a)
			return t, true
		}
	}

	helper, ok := intrinsic2[name]
	if !ok || len(s.Args) != 2 {
		return "", false
	}
	gv := g.hoistVar(v)
	a := g.gen(s.Args[0])
	b := g.gen(s.Args[1])
	t := g.temp()
	g.wf("%s := rt.%s(%s, %s, %s)\n", t, helper, gv, a, b)
	return t, true
}

// genSelfCallInt emits a self-recursive call whose result inference proved
// int64 (ADR 0067 param specialization) as an unboxed int64: the boxed
// Apply dispatches into the same fn (whose int64-guarded body returns
// int64), and rt.MustInt64 re-types the result. The single arg box at the
// call and the single result unbox are the accepted boundary cost; the
// arithmetic on either side stays raw int64.
func (g *generator) genSelfCallInt(n *ast.Node, s *ast.InvokeNode) (string, bool) {
	if !g.ni.isInt64(n) {
		return "", false
	}
	// Only a call (not a core arithmetic op) reaches here as int64 via the
	// self-return rule; core ops are handled by genIntrinsic.
	if s.Fn.Op == ast.OpVar {
		if v := s.Fn.Sub.(*ast.VarNode).Var; v.Namespace() == lang.NSCore {
			return "", false // an arithmetic/core call — genIntrinsic owns it
		}
	}
	// Rung 3: inside a lifted typed func, a self-recursive call is a DIRECT
	// int64 call — no Apply, no boxing of the arg, no MustInt64 of the
	// result. This is what makes fact/fib's recursive returns alloc-free.
	if g.selfDirect != nil && g.callsSelf(s) && len(s.Args) == g.selfDirect.arity {
		rvs := make([]string, len(s.Args))
		for i, a := range s.Args {
			rvs[i] = g.gen(a)
		}
		t := g.temp()
		g.wf("var %s int64 = %s(%s)\n", t, g.selfDirect.goName, strings.Join(rvs, ", "))
		return t, true
	}
	frv := g.gen(s.Fn)
	rvs := make([]string, len(s.Args))
	for i, a := range s.Args {
		rvs[i] = g.gen(a)
	}
	t := g.temp()
	if len(rvs) <= 4 {
		g.wf("var %s int64 = rt.MustInt64(lang.Apply%d(%s))\n", t, len(rvs),
			strings.Join(append([]string{frv}, rvs...), ", "))
	} else {
		g.wf("var %s int64 = rt.MustInt64(lang.Apply(%s, []any{%s}))\n", t, frv, strings.Join(rvs, ", "))
	}
	return t, true
}

// callsSelf reports whether invoke s targets the current lifted typed func
// (by fn* self-name binding or by def target var).
func (g *generator) callsSelf(s *ast.InvokeNode) bool {
	sd := g.selfDirect
	if sd == nil {
		return false
	}
	if s.Fn.Op == ast.OpLocal {
		return sd.bind != nil && s.Fn.Sub.(*ast.LocalNode).Binding == sd.bind
	}
	if s.Fn.Op == ast.OpVar {
		return sd.vr != nil && s.Fn.Sub.(*ast.VarNode).Var == sd.vr
	}
	return false
}

// genLoop emits loop*. A numeric loop met in UNGUARDED context (no
// enclosing `!rt.CoreDirty()` typed region) opens its own: the loop is
// dual-emitted — a typed arm with int64 carriers behind the dirty check,
// and a fully boxed arm honoring redefined core arithmetic (ADR 0066/
// 0067). Inside an already-guarded region, or when nothing numeric was
// proven, it emits once under the active inference scope.
func (g *generator) genLoop(n *ast.Node) string {
	if !g.guarded && !g.boxedForced && numInferEnabled {
		if ni := inferNumeric(n, nil, nil, "", nil, nil); loopWin(ni, n) {
			return g.genLoopDual(n, ni)
		}
	}
	return g.genLoopEmit(n)
}

// loopWin reports whether the loop-scope inference proved at least one of
// THIS loop's carriers int64 and the typed emission would actually differ
// (some arithmetic call unboxes) — the bar for paying dual emission.
func loopWin(ni *numInfer, n *ast.Node) bool {
	s := n.Sub.(*ast.LetNode)
	anyCarrier := false
	for _, bn := range s.Bindings {
		if ni.bindInt64(bn.Sub.(*ast.BindingNode)) {
			anyCarrier = true
			break
		}
	}
	return anyCarrier && ni.hasArithWin()
}

// genLoopDual wraps the two arms: `if !rt.CoreDirty() { typed } else
// { boxed }`, both assigning one shared any temp (the loop value boxes at
// this boundary either way — it flows on into boxed context). boxedForced
// marks the else-arm so nested loops skip their own (dead — the flag is
// sticky) dirty checks.
func (g *generator) genLoopDual(n *ast.Node, ni *numInfer) string {
	t := g.temp()
	g.wf("var %s any\n_ = %s\n", t, t)
	g.wf("if !rt.CoreDirty() {\n")
	save, saveG := g.ni, g.guarded
	g.ni, g.guarded = ni, true
	if rv := g.genLoopEmit(n); rv != "" {
		g.wf("%s = %s\n", t, rv)
	}
	g.ni, g.guarded = save, saveG
	g.wf("} else {\n")
	saveF := g.boxedForced
	g.boxedForced = true
	if rv := g.genLoopEmit(n); rv != "" {
		g.wf("%s = %s\n", t, rv)
	}
	g.boxedForced = saveF
	g.wf("}\n")
	return t
}

// genLoopEmit is the S5-proven loop* emission: binding vars (never
// reassigned), separate carriers for captured bindings, labeled `for {}`
// (label only when a recur targets it), per-iteration copies at the top
// of the body for captured carriers, break out with the result.
func (g *generator) genLoopEmit(n *ast.Node) string {
	s := n.Sub.(*ast.LetNode)
	t := g.temp()
	g.wf("var %s %s\n_ = %s\n", t, g.ni.goType(n), t)
	g.wf("{\n")

	bnodes := make([]*ast.BindingNode, len(s.Bindings))
	bnames := make([]string, len(s.Bindings))
	for i, bn := range s.Bindings {
		b := bn.Sub.(*ast.BindingNode)
		rv := g.gen(b.Init)
		gn := g.bindLocal(b)
		g.wf("var %s %s = %s\n_ = %s\n", gn, g.bindGoType(b), rv, gn)
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
	// The display name for an arity error: the Var this fn is being def'd
	// into (user/f, matching the interpreter) wins over the fn*'s own self-
	// name; an anonymous fn with neither stays "fn". Consume g.defName here
	// so a nested fn literal in the body does not inherit it (ADR 0048).
	displayName := "fn"
	defName := g.defName
	defVar := g.defVar
	g.defName = ""
	g.defVar = nil

	name := "fn"
	selfGo := ""
	selfTyped := "" // typed FnFuncN handle for direct self-calls (ADR 0064)
	var selfBind *ast.BindingNode
	if fn.Local != nil {
		b := fn.Local.Sub.(*ast.BindingNode)
		selfBind = b
		name = b.Name.Name()
		displayName = name
		selfGo = g.bindLocal(b)
		g.wf("var %s any\n_ = %s\n", selfGo, selfGo)
		// When the fn is a single fixed-arity method, also pre-declare a
		// typed handle the closure captures, so its own self-recursive
		// calls bypass lang.ApplyN (ADR 0064). Assigned alongside selfGo
		// after construction; direct calls can only fire once it is set.
		if m := singleFixedMethod(fn); m != nil {
			selfTyped = selfGo + "d"
			g.wf("var %s lang.FnFunc%d\n_ = %s\n", selfTyped, m.FixedArity, selfTyped)
			g.directFns[b] = directFn{goName: selfTyped, arity: m.FixedArity}
		}
	}
	if defName != "" {
		displayName = defName
	}

	t := g.temp()
	typedT := "" // raw FnFuncN closure temp (the ADR 0064 direct-call handle)
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
		// Numeric parameter specialization (ADR 0067): if the body is
		// int64-provable with every param assumed int64, emit an int64
		// fast path behind `if !rt.CoreDirty()` (ADR 0066) + entry
		// type-assertions, then the boxed body as the fallback for
		// non-int64 (float/BigInt) callers and redefined core arithmetic.
		if spec, lift := g.specializeInt(fn, m, defVar); spec != nil {
			if lift {
				// Rung 3: lift to a package-level typed func with direct
				// int64 recursion; the guard just delegates to it. Named
				// from the def target (user/add1 → fnL_user_add1) when the
				// fn* has no self-name, so the generated source is legible.
				fnL := g.emitTypedFunc(m, spec, displayName, liftSelfBind(fn), defVar)
				g.emitLiftGuard(m, gnames, fnL)
			} else {
				// Inline typed body (self-calls stay boxed — spec was
				// re-derived without self-typing; wins on per-op cost).
				g.emitTypedGuard(m, gnames, spec)
			}
		}
		g.emitBoxedMethod(m, gnames)
		g.wf("})\n")
		// Wrap the raw closure with its display name + expects label so an
		// arity mismatch (which bypasses the direct-call path and lands in
		// Invoke) panics the same NAMED ArityError the interpreter raises —
		// "passed to: user/f", never an unnamed count-only message
		// (ADR 0048; REPL-vs-binary parity). Matching-arity calls stay
		// direct: lang.Apply0..4 dispatch *NamedFnN through F, and the
		// ADR 0064 typed handle keeps holding the raw FnFuncN closure.
		typedT = t
		named := g.temp()
		g.wf("%s := &lang.NamedFn%d{Name: %q, Expects: %q, F: %s}\n",
			named, m.FixedArity, displayName, fnArityLabel(fn), typedT)
		t = named
	} else {
		expects := fnArityLabel(fn)
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
			g.wf("panic(lang.NewArityError(len(args), %q, %q))\n", displayName, expects)
			g.wf("}\n")
			g.genArgsBinding(variadic)
		} else {
			g.wf("panic(lang.NewArityError(len(args), %q, %q))\n", displayName, expects)
		}
		g.wf("}\n")
		g.wf("})\n")
	}

	if selfGo != "" {
		g.wf("%s = %s\n", selfGo, t)
	}
	if selfTyped != "" {
		// typedT is the raw lang.FnFuncN-typed temp; assign the same
		// closure into the typed handle the body captured for direct
		// self-calls.
		g.wf("%s = %s\n", selfTyped, typedT)
	}
	if selfBind != nil {
		// The self-name is only in scope within this fn's own body; drop
		// the direct-call registration so no later binding-pointer reuse
		// (arena/GC) could alias it. Keyed by pointer, this is belt-and-
		// suspenders — but cheap and unambiguous.
		delete(g.directFns, selfBind)
	}
	return t
}

// fnArityLabel renders a fn's accepted arities as the "expects" label the
// arity-error line shows — "1: [x]", "1: [x] or 2: [x y]", with a variadic
// method as "N+: [a & more]". It mirrors pkg/eval.arityExpects byte-for-byte
// so the compiled arity error reads identically to the interpreted one
// (ADR 0048).
func fnArityLabel(fn *ast.FnNode) string {
	parts := make([]string, 0, len(fn.Methods))
	for _, mn := range fn.Methods {
		m := mn.Sub.(*ast.FnMethodNode)
		names := make([]string, 0, len(m.Params))
		for _, pn := range m.Params {
			names = append(names, pn.Sub.(*ast.BindingNode).Name.Name())
		}
		label := fmt.Sprintf("%d: [%s]", m.FixedArity, strings.Join(names, " "))
		if m.IsVariadic {
			label = fmt.Sprintf("%d+: [%s & more]", m.FixedArity, strings.Join(names, " "))
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, " or ")
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
	g.emitBoxedMethod(m, gnames)
}

// emitBoxedMethod emits a method body on `any` params, fully boxed
// (emptyInfer): the fallback arm every caller can rely on — non-int64
// args, redefined core arithmetic (rt.CoreDirty), variadic/multi-arity
// fns. It (re)maps the params to gnames so a preceding typed guard's
// rebinding does not leak, then swaps the active inference for the
// duration. A numeric loop inside still gets its own guarded dual
// emission via genLoop (g.guarded is false here).
func (g *generator) emitBoxedMethod(m *ast.FnMethodNode, gnames []string) {
	for i, pn := range m.Params {
		g.locals[pn.Sub.(*ast.BindingNode)] = gnames[i]
	}
	save, saveG := g.ni, g.guarded
	g.ni, g.guarded = emptyInfer(), false
	g.genMethodBody(m, gnames)
	g.ni, g.guarded = save, saveG
}

// liftSelfBind extracts the fn* self-name binding, or nil.
func liftSelfBind(fn *ast.FnNode) *ast.BindingNode {
	if fn.Local == nil {
		return nil
	}
	return fn.Local.Sub.(*ast.BindingNode)
}

// specializeInt attempts int64 parameter specialization for a single
// fixed-arity method (ADR 0067). It seeds every param int64, registers
// them as recur carriers of the method loop, and runs the fixpoint —
// twice if need be:
//
//	pass 1 WITH self-call typing; if the proof holds AND the body is
//	liftable (canLift: no free locals, no nested fn, every self-reference
//	an int64-proven arity-matching call), return (spec, lift=true) — the
//	rung-3 typed-func path, where self-calls compile to direct int64 calls.
//
//	otherwise pass 2 WITHOUT self-call typing; if the body still proves
//	int64, return (spec, lift=false) — the inline typed body, where any
//	self-call stays fully boxed (it flows through the untyped path, so no
//	int64 re-typing of a boxed result is ever needed).
//
// The proof is: body returns int64 and every param stayed int64, and the
// typed emission would actually differ (hasArithWin). Params captured by
// a nested closure, and variadic params, are never specialized. Returns
// (nil, false) when nothing holds — the boxed body alone is emitted.
func (g *generator) specializeInt(fn *ast.FnNode, m *ast.FnMethodNode, defVar *lang.Var) (*numInfer, bool) {
	captured := capturedParams(m.Params, m.Body)
	seed := map[*ast.BindingNode]numType{}
	carriers := make([]*ast.BindingNode, len(m.Params))
	for i, pn := range m.Params {
		b := pn.Sub.(*ast.BindingNode)
		if b.IsVariadic || captured[b] {
			return nil, false
		}
		seed[b] = ntInt64
		carriers[i] = b
	}
	selfBind := liftSelfBind(fn)
	proves := func(spec *numInfer) bool {
		if spec.node[m.Body] != ntInt64 {
			return false
		}
		for _, b := range carriers {
			if spec.bind[b] != ntInt64 {
				return false
			}
		}
		return spec.hasArithWin()
	}
	spec := inferNumeric(m.Body, seed, carriers, m.LoopID, selfBind, defVar)
	if proves(spec) && canLift(m, selfBind, defVar, spec) {
		return spec, true
	}
	// Pass 2: no self typing — self-calls become ntUnknown and stay boxed,
	// so the inline typed body never re-types a boxed self-call result.
	spec = inferNumeric(m.Body, seed, carriers, m.LoopID, nil, nil)
	if proves(spec) {
		return spec, false
	}
	return nil, false
}

// emitTypedGuard emits the int64 fast path of a specialized method: the
// `!rt.CoreDirty()` region entry (ADR 0066 — redefined core arithmetic
// falls through to the boxed body, which honors it per call), one nested
// `if pI, ok := p.(int64); ok {` per param binding a fresh int64 local,
// then the method body under the specialized inference (so its arithmetic
// emits raw checked int64 ops), then the closing braces. On a dirty flag
// or a non-int64 arg the guard falls through to the boxed body below.
func (g *generator) emitTypedGuard(m *ast.FnMethodNode, outer []string, spec *numInfer) {
	save, saveG := g.ni, g.guarded
	g.ni, g.guarded = spec, true
	g.wf("if !rt.CoreDirty() {\n")
	inner := make([]string, len(m.Params))
	for i, pn := range m.Params {
		gn := g.bindLocal(pn.Sub.(*ast.BindingNode))
		g.wf("if %s, ok := %s.(int64); ok {\n_ = %s\n", gn, outer[i], gn)
		inner[i] = gn
	}
	g.genMethodBody(m, inner)
	for range m.Params {
		g.wf("}\n")
	}
	g.wf("}\n")
	g.ni, g.guarded = save, saveG
}

// selfFn identifies a lifted typed function so a self-recursive call
// inside it emits a direct int64 call rather than the boxed dispatch.
type selfFn struct {
	goName string
	bind   *ast.BindingNode // fn* self-name binding, or nil
	vr     *lang.Var        // def target var, or nil
	arity  int
}

// canLift reports whether a specialized method can be lifted to a
// package-level typed func (ADR 0067 rung 3):
//
//  1. the body references no free locals (nothing lexically outside
//     {params, self-name}) and contains no nested fn — both would need a
//     closure, which a package func is not; and
//  2. EVERY reference to the fn* self-name is the callee of an
//     int64-proven, arity-matching self-call (which emitTypedFunc turns
//     into a direct int64 call). Any other self-reference — value
//     position, wrong arity, non-int64 args — would emit the closure-
//     scoped selfGo/selfTyped handles, which do not exist at package
//     level. Such bodies keep the inline typed body / boxed path.
//
// Self-reference through the def'd VAR (an fn* with no self-name calling
// itself by name) is exempt from rule 2's scope concern — the hoisted var
// is package-level — but a non-int64/wrong-arity var self-call inside the
// lifted body is still fine: it emits v.Get() + ApplyN, both package-safe.
// fact/fib qualify; a capturing closure does not.
func canLift(m *ast.FnMethodNode, selfBind *ast.BindingNode, defVar *lang.Var, spec *numInfer) bool {
	bound := map[*ast.BindingNode]bool{}
	for _, pn := range m.Params {
		bound[pn.Sub.(*ast.BindingNode)] = true
	}
	if selfBind != nil {
		bound[selfBind] = true
	}
	// Collect every binding introduced anywhere in the body — pointers are
	// unique per site, so a free var (introduced OUTSIDE this body) is
	// exactly one whose binding is absent from this set.
	collectBindings(m.Body, bound)
	if hasFreeOrNestedFn(m.Body, bound) {
		return false
	}
	if selfBind != nil && !selfRefsAllDirect(m.Body, selfBind, spec, len(m.Params)) {
		return false
	}
	return true
}

// selfRefsAllDirect verifies rule 2 of canLift: every OpLocal reference to
// the self-name binding is the Fn of an OpInvoke that inference proved
// int64 with matching arity — exactly the calls genSelfCallInt emits as
// direct typed calls inside the lifted func.
func selfRefsAllDirect(root *ast.Node, selfBind *ast.BindingNode, spec *numInfer, arity int) bool {
	ok := true
	var walk func(n *ast.Node)
	walk = func(n *ast.Node) {
		if !ok {
			return
		}
		if n.Op == ast.OpInvoke {
			s := n.Sub.(*ast.InvokeNode)
			if s.Fn.Op == ast.OpLocal && s.Fn.Sub.(*ast.LocalNode).Binding == selfBind {
				if !spec.isInt64(n) || len(s.Args) != arity {
					ok = false
					return
				}
				for _, a := range s.Args { // the Fn ref itself is sanctioned
					walk(a)
				}
				return
			}
		}
		if n.Op == ast.OpLocal && n.Sub.(*ast.LocalNode).Binding == selfBind {
			ok = false // bare self-reference in value position
			return
		}
		eachChild(n, func(c *ast.Node, _ bool) { walk(c) })
	}
	walk(root)
	return ok
}

func collectBindings(n *ast.Node, bound map[*ast.BindingNode]bool) {
	switch n.Op {
	case ast.OpLet, ast.OpLoop:
		for _, bn := range n.Sub.(*ast.LetNode).Bindings {
			bound[bn.Sub.(*ast.BindingNode)] = true
		}
	case ast.OpFn:
		s := n.Sub.(*ast.FnNode)
		if s.Local != nil {
			bound[s.Local.Sub.(*ast.BindingNode)] = true
		}
		for _, mn := range s.Methods {
			for _, pn := range mn.Sub.(*ast.FnMethodNode).Params {
				bound[pn.Sub.(*ast.BindingNode)] = true
			}
		}
	case ast.OpCatch:
		bound[n.Sub.(*ast.CatchNode).Binding.Sub.(*ast.BindingNode)] = true
	}
	eachChild(n, func(c *ast.Node, _ bool) { collectBindings(c, bound) })
}

func hasFreeOrNestedFn(n *ast.Node, bound map[*ast.BindingNode]bool) bool {
	if n.Op == ast.OpFn {
		return true // a nested fn needs a closure — do not lift
	}
	if n.Op == ast.OpLocal {
		if b := n.Sub.(*ast.LocalNode).Binding; !bound[b] {
			return true // free variable
		}
	}
	found := false
	eachChild(n, func(c *ast.Node, _ bool) {
		if !found && hasFreeOrNestedFn(c, bound) {
			found = true
		}
	})
	return found
}

// emitTypedFunc lifts the specialized method to a package-level
// `func <name>L(p0 int64, …) int64 { body }` with DIRECT int64 recursion
// (ADR 0067 rung 3). The body is emitted into a side buffer under the
// specialized inference with selfDirect set, so self-calls become direct
// `<name>L(…)` calls that return int64 — the recursive value never boxes.
// Returns the func's Go name.
func (g *generator) emitTypedFunc(m *ast.FnMethodNode, spec *numInfer, name string, selfBind *ast.BindingNode, defVar *lang.Var) string {
	goName := g.uniqueGlobal("fnL_" + munge(name))

	saveBuf := g.buf
	saveNi, saveG := g.ni, g.guarded
	saveSelf := g.selfDirect
	g.buf = bytes.Buffer{}
	g.ni, g.guarded = spec, true
	g.selfDirect = &selfFn{goName: goName, bind: selfBind, vr: defVar, arity: len(m.Params)}
	// The ADR 0064 typed self-handle (selfGo+"d") is a closure-scope local;
	// it must never be referenced from this package-level func. canLift
	// already guarantees every self-call goes through genSelfCallInt's
	// direct path, but drop the registration for the duration anyway.
	var savedDF directFn
	var hadDF bool
	if selfBind != nil {
		savedDF, hadDF = g.directFns[selfBind]
		delete(g.directFns, selfBind)
	}

	inner := make([]string, len(m.Params))
	decls := make([]string, len(m.Params))
	for i, pn := range m.Params {
		gn := g.bindLocal(pn.Sub.(*ast.BindingNode))
		inner[i] = gn
		decls[i] = gn + " int64"
	}
	sig := ""
	if len(decls) > 0 {
		sig = strings.Join(decls, ", ")
	}
	g.wf("func %s(%s) int64 {\n", goName, sig)
	// Every param is a real Go param; silence unused (a fn may ignore one).
	for _, gn := range inner {
		g.wf("_ = %s\n", gn)
	}
	g.genMethodBody(m, inner)
	g.wf("}\n")

	g.funcs = append(g.funcs, g.buf.String())
	g.buf = saveBuf
	g.ni, g.guarded = saveNi, saveG
	g.selfDirect = saveSelf
	if hadDF {
		g.directFns[selfBind] = savedDF
	}
	return goName
}

// emitLiftGuard emits the fast-path guard that delegates to a lifted typed
// func: `if !rt.CoreDirty() { if pI, ok := p.(int64); ok { return
// <name>L(pI…) } }`. The dirty check is the ADR 0066 region entry — a
// redefined core arithmetic op falls through to the boxed body, which
// honors it per call; a non-int64 arg falls through the same way.
func (g *generator) emitLiftGuard(m *ast.FnMethodNode, outer []string, fnL string) {
	g.wf("if !rt.CoreDirty() {\n")
	inner := make([]string, len(m.Params))
	for i := range m.Params {
		gn := fmt.Sprintf("a%d_%d", i, g.next())
		g.wf("if %s, ok := %s.(int64); ok {\n", gn, outer[i])
		inner[i] = gn
	}
	g.wf("return %s(%s)\n", fnL, strings.Join(inner, ", "))
	for range m.Params {
		g.wf("}\n")
	}
	g.wf("}\n")
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
