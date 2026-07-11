// Package emit is the S1 flattening emitter: AST -> Go source TEXT.
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

	"cljgo-spike-s1/ast"
)

const runtimeImport = "cljgo-spike-s1/lang"

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
		g.recurs = append(g.recurs, &recurFrame{boundary: true})
		g.wf("lang.CheckArity(args, %d)\n", len(s.Params))
		for i, p := range s.Params {
			gp := g.bind(p)
			g.wf("%s := args[%d]\n_ = %s\n", gp, i, gp)
		}
		rv := g.gen(s.Body)
		if rv == "" {
			g.wf("panic(\"unreachable\")\n")
		} else {
			g.wf("return %s\n", rv)
		}
		g.recurs = g.recurs[:len(g.recurs)-1]
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
		gnames := make([]string, len(s.Bindings))
		for i, b := range s.Bindings {
			rv := g.gen(b.Init)
			gn := g.bind(b.Name)
			g.wf("var %s any = %s\n_ = %s\n", gn, rv, gn)
			gnames[i] = gn
		}
		recurs := recursDirectly(s.Body)
		label := fmt.Sprintf("loop%d", g.next())
		if recurs {
			// Labeled for: recur inside a NESTED loop's enclosing blocks must
			// continue the right for-statement, and Go rejects unused labels,
			// so the label appears only when a recur targets it.
			g.wf("%s:\n", label)
		}
		g.wf("for {\n")
		if recurs {
			g.recurs = append(g.recurs, &recurFrame{label: label, bindings: gnames})
		}
		if rv := g.gen(s.Body); rv != "" {
			g.wf("%s = %s\n", t, rv)
		}
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
	g := &generator{
		scopes: []map[string]string{{}},
		vars:   map[string]string{},
	}
	for _, f := range forms {
		g.discard(g.gen(f))
	}

	var out bytes.Buffer
	out.WriteString("// Code generated by cljgo spike S1. DO NOT EDIT.\n")
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
