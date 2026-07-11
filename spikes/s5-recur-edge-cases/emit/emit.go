// Package emit is the S5 flattening emitter: S1's emitter extended with the
// recur/loop edge cases S1 did not cover:
//
//   - fn-level recur (no loop): the fn body becomes a labeled `for {}` whose
//     params are the recur carriers; recur = rebind params + continue. NOT
//     goto: a `goto` back over `tmpN := ...` declarations is illegal Go
//     ("jumps over variable declaration"), the for/continue form never is.
//   - closure capturing a loop local: Clojure closures capture the VALUE at
//     the iteration that created them (JVM closes over a copied field), but
//     Go closures capture the VARIABLE by reference — and our loop carriers
//     are `var x any` declared OUTSIDE the `for {}`, so Go 1.22's
//     per-iteration loop-var change does NOT apply to them. Fix: when a loop
//     local is referenced under an fn inside the loop body, reads inside the
//     body go through a fresh per-iteration copy (`xN_iter := xN` at the top
//     of the for body); recur still writes the OUTER carriers. Toggleable via
//     NoCaptureFix to demonstrate the divergence.
//
// Every gen* call writes statements to the buffer and returns the name of an
// r-value (temp var, local, or literal). NO IIFEs (design/04 §3). Output is
// gated through go/format.Source before it is handed back.
package emit

import (
	"bytes"
	"fmt"
	"go/format"
	"sort"
	"strconv"
	"strings"

	"cljgo-spike-s5/ast"
)

const runtimeImport = "cljgo-spike-s5/lang"

type recurFrame struct {
	label    string
	bindings []string // Go names of the loop locals, in order
	boundary bool     // fn boundary: recur may not cross it
}

type generator struct {
	buf    bytes.Buffer
	id     int               // monotonic counter: temps, locals, loop labels
	scopes []map[string]string // Clojure local name -> Go name
	recurs []*recurFrame
	vars   map[string]string // Clojure var name -> hoisted Go package-level name

	// NoCaptureFix disables the per-iteration copy for closure-captured loop
	// locals — the naive S1 emission — to demonstrate the divergence from
	// Clojure semantics (case 1).
	NoCaptureFix bool
}

func (g *generator) wf(f string, a ...any) { fmt.Fprintf(&g.buf, f, a...) }

func (g *generator) next() int { g.id++; return g.id }

func (g *generator) temp() string { return fmt.Sprintf("tmp%d", g.next()) }

// ---- munging ---------------------------------------------------------------

var mungeMap = map[rune]string{
	'-': "_", '<': "_LT_", '>': "_GT_", '=': "_EQ_", '+': "_PLUS_",
	'*': "_STAR_", '/': "_SLASH_", '!': "_BANG_", '?': "_QMARK_",
	'.': "_DOT_", '\'': "_SQUOTE_", '&': "_AMP_",
}

func munge(name string) string {
	var b strings.Builder
	for _, r := range name {
		if s, ok := mungeMap[r]; ok {
			b.WriteString(s)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ---- scopes ----------------------------------------------------------------

func (g *generator) pushScope() { g.scopes = append(g.scopes, map[string]string{}) }
func (g *generator) popScope()  { g.scopes = g.scopes[:len(g.scopes)-1] }

// bind allocates a fresh suffixed Go name so shadowing never collides
// (Glojure's varScope-stack technique).
func (g *generator) bind(name string) string {
	gn := munge(name) + strconv.Itoa(g.next())
	g.scopes[len(g.scopes)-1][name] = gn
	return gn
}

func (g *generator) lookup(name string) string {
	for i := len(g.scopes) - 1; i >= 0; i-- {
		if gn, ok := g.scopes[i][name]; ok {
			return gn
		}
	}
	panic("unresolved local: " + name)
}

// hoist registers a global var reference; rendered as a package-level
// `var v_x = lang.InternVar("x")` (idempotent interning makes this safe).
func (g *generator) hoist(cljName string) string {
	if gn, ok := g.vars[cljName]; ok {
		return gn
	}
	gn := "v_" + munge(cljName)
	g.vars[cljName] = gn
	return gn
}

// ---- literals ---------------------------------------------------------------

func literal(v any) string {
	switch x := v.(type) {
	case nil:
		return "nil"
	case bool:
		return strconv.FormatBool(x)
	case int64:
		return fmt.Sprintf("int64(%d)", x)
	case float64:
		return fmt.Sprintf("float64(%s)", strconv.FormatFloat(x, 'g', -1, 64))
	case string:
		return strconv.Quote(x)
	}
	panic(fmt.Sprintf("unsupported const type %T", v))
}

// discard emits a use for an r-value we are throwing away, so `tmpN := ...`
// never trips Go's unused-variable error. "" (recur produced no value) and
// the untyped-nil literal are skipped (`_ = nil` is illegal Go).
func (g *generator) discard(rv string) {
	if rv == "" || rv == "nil" {
		return
	}
	g.wf("_ = %s\n", rv)
}

// recursDirectly reports whether the node recurs to THIS frame — descent
// stops at fn and loop boundaries, which capture their own recurs. Needed
// because Go rejects unused labels, so a loop only gets a label when some
// recur will `continue` to it.
func recursDirectly(n *ast.Node) bool {
	if n == nil {
		return false
	}
	switch n.Op {
	case ast.OpRecur:
		return true
	case ast.OpFn, ast.OpLoop:
		return false
	case ast.OpIf:
		s := n.Sub.(*ast.If)
		return recursDirectly(s.Test) || recursDirectly(s.Then) || recursDirectly(s.Else)
	case ast.OpLet:
		s := n.Sub.(*ast.Let)
		for _, b := range s.Bindings {
			if recursDirectly(b.Init) {
				return true
			}
		}
		return recursDirectly(s.Body)
	case ast.OpDo:
		for _, f := range n.Sub.(*ast.Do).Forms {
			if recursDirectly(f) {
				return true
			}
		}
		return false
	case ast.OpInvoke:
		s := n.Sub.(*ast.Invoke)
		if recursDirectly(s.Target) {
			return true
		}
		for _, a := range s.Args {
			if recursDirectly(a) {
				return true
			}
		}
		return false
	case ast.OpDef:
		return recursDirectly(n.Sub.(*ast.Def).Init)
	}
	return false
}

// capturedIdx reports which of the recur-carrier names (loop locals or
// fn params) are referenced from INSIDE an fn nested in body — i.e. closed
// over. Those need per-iteration value copies, because Go closures capture
// variables by reference while Clojure closures capture the value at the
// iteration that created them. Shadowing (fn params, let/loop bindings of
// the same name) is respected: a shadowed reference is not a capture.
//
// In the real compiler this is an ANALYZER annotation on the loop node
// (each local already knows its binding site); this walk is a spike shortcut.
func capturedIdx(body *ast.Node, names []string) []int {
	target := map[string]bool{}
	for _, n := range names {
		target[n] = true
	}
	hit := map[string]bool{}

	var walk func(n *ast.Node, inFn bool, shadow map[string]bool)
	shadowPlus := func(shadow map[string]bool, more ...string) map[string]bool {
		out := make(map[string]bool, len(shadow)+len(more))
		for k := range shadow {
			out[k] = true
		}
		for _, m := range more {
			out[m] = true
		}
		return out
	}
	walk = func(n *ast.Node, inFn bool, shadow map[string]bool) {
		if n == nil {
			return
		}
		switch n.Op {
		case ast.OpLocal:
			name := n.Sub.(*ast.Local).Name
			if inFn && target[name] && !shadow[name] {
				hit[name] = true
			}
		case ast.OpFn:
			s := n.Sub.(*ast.Fn)
			walk(s.Body, true, shadowPlus(shadow, s.Params...))
		case ast.OpIf:
			s := n.Sub.(*ast.If)
			walk(s.Test, inFn, shadow)
			walk(s.Then, inFn, shadow)
			walk(s.Else, inFn, shadow)
		case ast.OpLet:
			s := n.Sub.(*ast.Let)
			sh := shadow
			for _, b := range s.Bindings { // sequential: init sees prior shadows
				walk(b.Init, inFn, sh)
				sh = shadowPlus(sh, b.Name)
			}
			walk(s.Body, inFn, sh)
		case ast.OpLoop:
			s := n.Sub.(*ast.Loop)
			sh := shadow
			for _, b := range s.Bindings {
				walk(b.Init, inFn, sh)
				sh = shadowPlus(sh, b.Name)
			}
			walk(s.Body, inFn, sh)
		case ast.OpDo:
			for _, f := range n.Sub.(*ast.Do).Forms {
				walk(f, inFn, shadow)
			}
		case ast.OpInvoke:
			s := n.Sub.(*ast.Invoke)
			walk(s.Target, inFn, shadow)
			for _, a := range s.Args {
				walk(a, inFn, shadow)
			}
		case ast.OpRecur:
			for _, a := range n.Sub.(*ast.Recur).Args {
				walk(a, inFn, shadow)
			}
		case ast.OpDef:
			walk(n.Sub.(*ast.Def).Init, inFn, shadow)
		}
	}
	walk(body, false, map[string]bool{})

	var idx []int
	for i, n := range names {
		if hit[n] {
			idx = append(idx, i)
		}
	}
	return idx
}

// emitIterCopies gives closure-captured recur carriers fresh per-iteration
// value copies at the top of the for body: body READS resolve to the copy
// (scope rebind), recur WRITES still hit the outer carrier (recurFrame keeps
// the outer names). Caller must pushScope() before / popScope() after body.
func (g *generator) emitIterCopies(captured []int, cljNames, carriers []string) {
	for _, ci := range captured {
		fresh := g.bind(cljNames[ci]) // rebinds the clj name for body reads
		g.wf("%s := %s\n_ = %s\n", fresh, carriers[ci], fresh)
	}
}

// loopCaptured returns the indexes of loop bindings that are closed over by
// an fn in the loop BODY or in a LATER binding's init. Both positions matter:
// a body closure must see per-iteration values (fresh copy in the for body),
// and an init-position closure must see the INITIAL value forever (so the
// carrier the recur reassigns has to be a separate variable from the binding
// var the init closure captured). Verified against real Clojure:
// (loop [i 0 f (fn [] i)] ...) prints 0, never the rebound i.
func loopCaptured(bindings []ast.Binding, body *ast.Node) []int {
	names := make([]string, len(bindings))
	for i, b := range bindings {
		names[i] = b.Name
	}
	hit := map[int]bool{}
	for _, ci := range capturedIdx(body, names) {
		hit[ci] = true
	}
	for j := range bindings { // init j may close over bindings 0..j-1
		if j == 0 {
			continue
		}
		for _, ci := range capturedIdx(bindings[j].Init, names[:j]) {
			hit[ci] = true
		}
	}
	var idx []int
	for i := range names {
		if hit[i] {
			idx = append(idx, i)
		}
	}
	return idx
}

// ---- the flattener -----------------------------------------------------------

// gen writes the statements for n and returns the r-value name, or "" when
// the node transferred control (recur) and produced no value.
func (g *generator) gen(n *ast.Node) string {
	switch n.Op {

	case ast.OpConst:
		return literal(n.Sub.(*ast.Const).Value)

	case ast.OpVarRef:
		return g.hoist(n.Sub.(*ast.VarRef).Name) + ".Get()"

	case ast.OpLocal:
		return g.lookup(n.Sub.(*ast.Local).Name)

	case ast.OpDef:
		s := n.Sub.(*ast.Def)
		gv := g.hoist(s.Name)
		rv := g.gen(s.Init)
		g.wf("%s.BindRoot(%s)\n", gv, rv)
		return "nil"

	case ast.OpDo:
		s := n.Sub.(*ast.Do)
		if len(s.Forms) == 0 {
			return "nil"
		}
		for _, f := range s.Forms[:len(s.Forms)-1] {
			g.discard(g.gen(f))
		}
		return g.gen(s.Forms[len(s.Forms)-1])

	case ast.OpIf:
		s := n.Sub.(*ast.If)
		cond := g.gen(s.Test)
		t := g.temp()
		g.wf("var %s any\n_ = %s\n", t, t) // _ = : both branches may recur
		g.wf("if lang.IsTruthy(%s) {\n", cond)
		if rv := g.gen(s.Then); rv != "" {
			g.wf("%s = %s\n", t, rv)
		}
		g.wf("} else {\n")
		if s.Else != nil {
			if rv := g.gen(s.Else); rv != "" {
				g.wf("%s = %s\n", t, rv)
			}
		} else {
			g.wf("%s = nil\n", t)
		}
		g.wf("}\n")
		return t

	case ast.OpLet:
		s := n.Sub.(*ast.Let)
		t := g.temp()
		g.wf("var %s any\n_ = %s\n", t, t)
		g.wf("{\n") // new lexical block; suffixed names make shadowing free anyway
		g.pushScope()
		for _, b := range s.Bindings {
			rv := g.gen(b.Init) // init sees earlier bindings (sequential let*)
			gn := g.bind(b.Name)
			g.wf("var %s any = %s\n_ = %s\n", gn, rv, gn)
		}
		if rv := g.gen(s.Body); rv != "" {
			g.wf("%s = %s\n", t, rv)
		}
		g.popScope()
		g.wf("}\n")
		return t

	case ast.OpFn:
		s := n.Sub.(*ast.Fn)
		t := g.temp()
		// Go closures capture enclosing locals by reference: no env struct,
		// no lifting. The func literal is emitted inline; body statements
		// flatten INSIDE it, so no IIFE-at-use-site ever appears.
		g.wf("%s := lang.Fn(func(args ...any) any {\n", t)
		g.pushScope()
		g.wf("lang.CheckArity(args, %d)\n", len(s.Params))
		gnames := make([]string, len(s.Params))
		for i, p := range s.Params {
			gp := g.bind(p)
			g.wf("%s := args[%d]\n_ = %s\n", gp, i, gp)
			gnames[i] = gp
		}
		if recursDirectly(s.Body) {
			// fn-level recur (no loop): the params ARE the recur carriers.
			// Mechanism: labeled `for {}` + rebind-params + continue. A
			// backward `goto` (doc 04's suggestion) is ALSO legal Go — the
			// "jumps over variable declaration" error applies only to
			// FORWARD jumps (verified in this spike) — but for/continue is
			// used anyway: it is the same machinery as loop emission (one
			// recurFrame shape) and continue-from-nested-blocks needs no
			// block-structure reasoning. Non-recur paths `return` directly,
			// so the for never breaks and Go's termination analysis accepts
			// the function without a trailing return.
			label := fmt.Sprintf("fnloop%d", g.next())
			g.recurs = append(g.recurs, &recurFrame{label: label, bindings: gnames})
			var captured []int
			if !g.NoCaptureFix {
				captured = capturedIdx(s.Body, s.Params)
			}
			g.wf("%s:\n", label)
			g.wf("for {\n")
			g.pushScope()
			g.emitIterCopies(captured, s.Params, gnames)
			rv := g.gen(s.Body)
			if rv != "" {
				g.wf("return %s\n", rv)
			}
			g.popScope()
			g.recurs = g.recurs[:len(g.recurs)-1]
			g.wf("}\n") // for{} with no break: terminating statement
		} else {
			g.recurs = append(g.recurs, &recurFrame{boundary: true})
			rv := g.gen(s.Body)
			if rv == "" {
				g.wf("panic(\"unreachable\")\n")
			} else {
				g.wf("return %s\n", rv)
			}
			g.recurs = g.recurs[:len(g.recurs)-1]
		}
		g.popScope()
		g.wf("})\n")
		return t

	case ast.OpInvoke:
		s := n.Sub.(*ast.Invoke)
		frv := g.gen(s.Target) // fn position evaluates first
		argRvs := make([]string, len(s.Args))
		for i, a := range s.Args {
			argRvs[i] = g.gen(a)
		}
		t := g.temp()
		switch len(argRvs) {
		case 0:
			g.wf("%s := lang.Apply0(%s)\n", t, frv)
		case 1:
			g.wf("%s := lang.Apply1(%s, %s)\n", t, frv, argRvs[0])
		case 2:
			g.wf("%s := lang.Apply2(%s, %s, %s)\n", t, frv, argRvs[0], argRvs[1])
		default:
			g.wf("%s := lang.Apply(%s, []any{%s})\n", t, frv, strings.Join(argRvs, ", "))
		}
		return t

	case ast.OpLoop:
		s := n.Sub.(*ast.Loop)
		t := g.temp()
		g.wf("var %s any\n_ = %s\n", t, t)
		g.wf("{\n")
		g.pushScope()
		cljNames := make([]string, len(s.Bindings))
		bnames := make([]string, len(s.Bindings)) // binding vars (never reassigned)
		for i, b := range s.Bindings {
			rv := g.gen(b.Init)
			gn := g.bind(b.Name)
			g.wf("var %s any = %s\n_ = %s\n", gn, rv, gn)
			cljNames[i] = b.Name
			bnames[i] = gn
		}
		recurs := recursDirectly(s.Body)
		var captured []int
		if recurs && !g.NoCaptureFix {
			captured = loopCaptured(s.Bindings, s.Body)
		}
		// Carriers are what recur reassigns. For captured bindings the carrier
		// must be a SEPARATE variable from the binding var: an init-position
		// closure captured the binding var and must keep seeing the initial
		// value (Clojure semantics); only the carrier gets rebound.
		carriers := make([]string, len(s.Bindings))
		copy(carriers, bnames)
		for _, ci := range captured {
			cn := fmt.Sprintf("%s_c%d", munge(cljNames[ci]), g.next())
			g.wf("var %s any = %s\n_ = %s\n", cn, bnames[ci], cn)
			carriers[ci] = cn
		}
		label := fmt.Sprintf("loop%d", g.next())
		if recurs {
			// Labeled for: recur inside a NESTED loop's enclosing blocks must
			// continue the right for-statement, and Go rejects unused labels,
			// so the label appears only when a recur targets it.
			g.wf("%s:\n", label)
		}
		g.wf("for {\n")
		if recurs {
			g.recurs = append(g.recurs, &recurFrame{label: label, bindings: carriers})
		}
		g.pushScope()
		g.emitIterCopies(captured, cljNames, carriers)
		if rv := g.gen(s.Body); rv != "" {
			g.wf("%s = %s\n", t, rv)
		}
		g.popScope()
		if recurs {
			g.recurs = g.recurs[:len(g.recurs)-1]
			g.wf("break %s\n", label)
		} else {
			g.wf("break\n")
		}
		g.wf("}\n")
		g.popScope()
		g.wf("}\n")
		return t

	case ast.OpRecur:
		s := n.Sub.(*ast.Recur)
		if len(g.recurs) == 0 {
			panic("recur outside loop")
		}
		fr := g.recurs[len(g.recurs)-1] // capture BEFORE generating args:
		if fr.boundary {                // args may contain nested loops that
			panic("recur crosses fn boundary") // push/pop their own frames
		}
		if len(s.Args) != len(fr.bindings) {
			panic(fmt.Sprintf("recur arity %d != loop arity %d", len(s.Args), len(fr.bindings)))
		}
		// Simultaneous rebinding: evaluate ALL new values into temps first,
		// then assign — a later init must not see an earlier rebind.
		temps := make([]string, len(s.Args))
		for i, a := range s.Args {
			rv := g.gen(a)
			tt := g.temp()
			g.wf("var %s any = %s\n", tt, rv)
			temps[i] = tt
		}
		for i, b := range fr.bindings {
			g.wf("%s = %s\n", b, temps[i])
		}
		g.wf("continue %s\n", fr.label)
		return "" // no r-value: the branch that recurs assigns nothing
	}
	panic("unhandled op: " + n.Op.String())
}

// ---- assembly ----------------------------------------------------------------

// EmitMain compiles top-level forms into a complete main-package Go file:
// hoisted var interns, a guarded Load() with the forms in source order, and
// main() = lang.Init() + Load() (design/04 §1). Returns gofmt-ed source; the
// raw (pre-format) text comes back too so a format failure is debuggable.
func EmitMain(forms []*ast.Node) (formatted []byte, raw []byte, err error) {
	return EmitMainOpt(forms, false)
}

// EmitMainOpt is EmitMain with the capture fix toggle (noCaptureFix=true
// reproduces S1's naive loop emission, for the case-1 divergence demo).
func EmitMainOpt(forms []*ast.Node, noCaptureFix bool) (formatted []byte, raw []byte, err error) {
	g := &generator{
		scopes:       []map[string]string{{}},
		vars:         map[string]string{},
		NoCaptureFix: noCaptureFix,
	}
	for _, f := range forms {
		g.discard(g.gen(f))
	}

	var out bytes.Buffer
	out.WriteString("// Code generated by cljgo spike S5. DO NOT EDIT.\n")
	out.WriteString("package main\n\n")
	fmt.Fprintf(&out, "import lang %q\n\n", runtimeImport)

	if len(g.vars) > 0 {
		names := make([]string, 0, len(g.vars))
		for clj := range g.vars {
			names = append(names, clj)
		}
		sort.Strings(names) // deterministic output (design/04 §6)
		out.WriteString("var (\n")
		for _, clj := range names {
			fmt.Fprintf(&out, "%s = lang.InternVar(%q)\n", g.vars[clj], clj)
		}
		out.WriteString(")\n\n")
	}

	out.WriteString("var loaded = false\n\n")
	out.WriteString("// Load evaluates the top-level forms once, in source order.\nfunc Load() {\nif loaded {\nreturn\n}\nloaded = true\n")
	out.Write(g.buf.Bytes())
	out.WriteString("}\n\nfunc main() {\nlang.Init()\nLoad()\n}\n")

	raw = out.Bytes()
	formatted, err = format.Source(raw) // the syntax gate: parses or fails here
	return formatted, raw, err
}
