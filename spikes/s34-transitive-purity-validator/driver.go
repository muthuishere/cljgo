// Spike S34 driver — ONE transitive walk that classifies every reachable
// namespace of a cljgo library by purity, via PLUGGABLE taint predicates,
// and derives BOTH a whole-library publish gate and a per-namespace use gate
// from the same classification map.
//
// Read-only against pkg/: it calls emit.CompileProgram (the existing
// transitive-require traversal, ADR 0042) and walks the captured analyzed
// forms. Nothing here modifies pkg/. The child-walker below is COPIED and
// adapted from pkg/emit/emit.go:eachChild (unexported there) — S34 owns its
// own copy per the spike rules.
//
//	go run ./spikes/s34-transitive-purity-validator            # all fixtures
//	go run ./spikes/s34-transitive-purity-validator -fixture mixed
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/muthuishere/cljgo/pkg/ast"
	"github.com/muthuishere/cljgo/pkg/emit"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// ---- classification result ------------------------------------------------

// Taint classes. "pure" is the absence of any taint finding.
const (
	ClassPure      = "pure"
	ClassGoInterop = "go-interop"
	ClassJava      = "java"
	ClassFFI       = "ffi" // designed-for slot; no predicate ships one today
)

// Finding is one reason a namespace is non-pure, with position for the
// hard-error message a real gate would raise (item 5).
type Finding struct {
	Class  string
	NS     string
	Path   string
	Line   int
	Detail string
}

func (f Finding) where() string {
	if f.Line > 0 {
		return fmt.Sprintf("%s:%d", f.Path, f.Line)
	}
	return f.Path
}

// Predicate examines ONE captured namespace and returns the taint findings
// it can prove. Predicates are independent and composable: the walk runs
// every predicate over every namespace exactly once. This is the pluggable
// design the coordinator asked for — go-interop, java, ffi are separate
// predicates over the SAME walk, not hardcoded branches.
type Predicate func(ns *emit.CompiledNS) []Finding

// ---- the go-interop predicate (the concrete exemplar) ---------------------

// hostOps are the analyzed AST markers of Go host interop (design/05,
// ADR 0010): the require-go alias members (OpHostRef/OpHostCall carry the Go
// import Pkg) and the receiver-based dot-forms. Same node set
// pkg/emit/hostfacts.go:collectHostPaths keys on.
func goInteropPredicate(ns *emit.CompiledNS) []Finding {
	var out []Finding
	name := nsLabel(ns)
	for _, form := range ns.Forms {
		walk(form, func(n *ast.Node) {
			var detail string
			switch n.Op {
			case ast.OpHostRef:
				detail = "require-go member " + n.Sub.(*ast.HostRefNode).Pkg + "/" + n.Sub.(*ast.HostRefNode).Member
			case ast.OpHostCall:
				detail = "require-go call " + n.Sub.(*ast.HostCallNode).Pkg + "/" + n.Sub.(*ast.HostCallNode).Member
			case ast.OpHostMethod:
				detail = "Go dot-method ." + n.Sub.(*ast.HostMethodNode).Method
			case ast.OpHostField:
				detail = "Go dot-field .-" + n.Sub.(*ast.HostFieldNode).Field
			case ast.OpHostNew:
				detail = "Go new/struct ctor"
			default:
				return
			}
			line := nodeLine(n)
			if line == 0 {
				line = nodeLine(form)
			}
			out = append(out, Finding{Class: ClassGoInterop, NS: name, Path: ns.Path, Line: line, Detail: detail})
		})
	}
	return out
}

// ---- the java predicate (ABSTRACT — S35 owns the real primitive) ----------

// javaStubPredicate stands in for S35's uses-java?(ns). S34 does not decide
// what a Java form IS; it only proves the walk surfaces a Java finding as a
// DISTINCT third class when an external predicate reports one. The stub flags
// namespaces named in usesJava.
func javaStubPredicate(usesJava map[string]bool) Predicate {
	return func(ns *emit.CompiledNS) []Finding {
		name := nsLabel(ns)
		if !usesJava[name] {
			return nil
		}
		return []Finding{{Class: ClassJava, NS: name, Path: ns.Path, Line: 1,
			Detail: "flagged by external uses-java? predicate (S35)"}}
	}
}

// ffiPredicate is the future slot (ADR 0011/0044/S32). No ffi/deflib AST op
// exists in pkg/ today, so it proves nothing and reports nothing — it exists
// to show the walk composes N predicates, not exactly two.
func ffiPredicate(ns *emit.CompiledNS) []Finding { return nil }

// ---- the ONE walk ---------------------------------------------------------

// nsClass is the per-namespace verdict.
type nsClass struct {
	NS       string
	Class    string // ClassPure or the first non-pure class found
	Findings []Finding
}

// Classification is the whole result of one walk: a map derived ONCE, from
// which both gates read.
type Classification struct {
	Order []string // namespaces, dependency-first (entry last)
	ByNS  map[string]*nsClass
}

// classify runs emit.CompileProgram (the existing transitive traversal) ONCE,
// then runs every predicate over every captured namespace ONCE, building the
// per-namespace class map. Both gates below read this single map.
func classify(entrySrc string, preds []Predicate) (*Classification, error) {
	prog, err := emit.CompileProgram(entrySrc)
	if err != nil {
		// A namespace that fails to ANALYZE (e.g. an unresolved java.* static
		// call) surfaces here with file:line — itself a hard-error signal.
		return nil, err
	}
	// CompileProgram leaves the entry's Name "" (it is the srcPath, not a
	// required ns). Recover the declared ns so the entry is a first-class,
	// named member of the classification like every dep.
	if prog.Entry.Name == "" {
		prog.Entry.Name = readNSName(entrySrc)
	}
	c := &Classification{ByNS: map[string]*nsClass{}}
	// Deps are dependency-first; entry last. One pass, every ns.
	all := append([]*emit.CompiledNS{}, prog.Deps...)
	all = append(all, prog.Entry)
	for _, ns := range all {
		name := nsLabel(ns)
		nc := &nsClass{NS: name, Class: ClassPure}
		for _, p := range preds {
			for _, f := range p(ns) {
				nc.Findings = append(nc.Findings, f)
				if nc.Class == ClassPure {
					nc.Class = f.Class
				}
			}
		}
		c.Order = append(c.Order, name)
		c.ByNS[name] = nc
	}
	return c, nil
}

// wholeLibGate — the `publish clojars` gate. Refuses if ANY reachable
// namespace is non-pure. Derived purely from the classification map.
func (c *Classification) wholeLibGate() (ok bool, reasons []Finding) {
	ok = true
	for _, name := range c.Order {
		if nc := c.ByNS[name]; nc.Class != ClassPure {
			ok = false
			reasons = append(reasons, nc.Findings...)
		}
	}
	return
}

// perNSGate — the plain `use <ns>` gate. Only the named namespace's own class
// matters (its transitive deps are gated when THEY are used). Same map.
func (c *Classification) perNSGate(ns string) (ok bool, reasons []Finding) {
	nc, present := c.ByNS[ns]
	if !present {
		return false, []Finding{{Detail: "namespace not reachable: " + ns}}
	}
	return nc.Class == ClassPure, nc.Findings
}

// ---- helpers --------------------------------------------------------------

// readNSName extracts the declared namespace from a source file's leading
// (ns X …) form, textually — enough to label the entry.
func readNSName(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "<entry:" + filepath.Base(path) + ">"
	}
	s := string(b)
	i := strings.Index(s, "(ns ")
	if i < 0 {
		return "<entry:" + filepath.Base(path) + ">"
	}
	rest := s[i+4:]
	j := strings.IndexAny(rest, " \t\r\n()")
	if j < 0 {
		j = len(rest)
	}
	return strings.TrimSpace(rest[:j])
}

func nsLabel(ns *emit.CompiledNS) string {
	if ns.Name == "" {
		return "<entry:" + filepath.Base(ns.Path) + ">"
	}
	return ns.Name
}

func nodeLine(n *ast.Node) int {
	im, ok := n.Form.(lang.IMeta)
	if !ok || im.Meta() == nil {
		return 0
	}
	if l, ok := lang.AsInt(lang.Get(im.Meta(), lang.KWLine)); ok {
		return l
	}
	return 0
}

// walk visits n and every descendant. COPIED/adapted from
// pkg/emit/emit.go:eachChild (unexported) — the exhaustive child enumeration
// per Op. S34 keeps its own copy per the spike read-only rule.
func walk(n *ast.Node, visit func(*ast.Node)) {
	if n == nil {
		return
	}
	visit(n)
	eachChildS29(n, func(c *ast.Node) { walk(c, visit) })
}

func eachChildS29(n *ast.Node, f func(*ast.Node)) {
	switch n.Op {
	case ast.OpConst, ast.OpQuote, ast.OpVar, ast.OpTheVar, ast.OpLocal, ast.OpHostRef:
	case ast.OpHostCall:
		for _, c := range n.Sub.(*ast.HostCallNode).Args {
			f(c)
		}
	case ast.OpHostMethod:
		s := n.Sub.(*ast.HostMethodNode)
		f(s.Recv)
		for _, c := range s.Args {
			f(c)
		}
	case ast.OpHostField:
		f(n.Sub.(*ast.HostFieldNode).Recv)
	case ast.OpHostNew:
		if fl := n.Sub.(*ast.HostNewNode).Fields; fl != nil {
			f(fl)
		}
	case ast.OpVector:
		for _, c := range n.Sub.(*ast.VectorNode).Items {
			f(c)
		}
	case ast.OpMap:
		s := n.Sub.(*ast.MapNode)
		for i := range s.Keys {
			f(s.Keys[i])
			f(s.Vals[i])
		}
	case ast.OpSet:
		for _, c := range n.Sub.(*ast.SetNode).Items {
			f(c)
		}
	case ast.OpDo:
		s := n.Sub.(*ast.DoNode)
		for _, c := range s.Statements {
			f(c)
		}
		if s.Ret != nil {
			f(s.Ret)
		}
	case ast.OpIf:
		s := n.Sub.(*ast.IfNode)
		f(s.Test)
		f(s.Then)
		f(s.Else)
	case ast.OpDef:
		s := n.Sub.(*ast.DefNode)
		if s.Init != nil {
			f(s.Init)
		}
		if s.Meta != nil {
			f(s.Meta)
		}
	case ast.OpLet, ast.OpLoop:
		s := n.Sub.(*ast.LetNode)
		for _, bn := range s.Bindings {
			if init := bn.Sub.(*ast.BindingNode).Init; init != nil {
				f(init)
			}
		}
		f(s.Body)
	case ast.OpFn:
		for _, mn := range n.Sub.(*ast.FnNode).Methods {
			f(mn.Sub.(*ast.FnMethodNode).Body)
		}
	case ast.OpFnMethod:
		f(n.Sub.(*ast.FnMethodNode).Body)
	case ast.OpInvoke:
		s := n.Sub.(*ast.InvokeNode)
		f(s.Fn)
		for _, c := range s.Args {
			f(c)
		}
	case ast.OpRecur:
		for _, c := range n.Sub.(*ast.RecurNode).Exprs {
			f(c)
		}
	case ast.OpSetBang:
		s := n.Sub.(*ast.SetBangNode)
		f(s.Target)
		f(s.Val)
	case ast.OpDynBind:
		s := n.Sub.(*ast.DynBindNode)
		for i := range s.Vars {
			f(s.Vars[i])
			f(s.Vals[i])
		}
		f(s.Body)
	case ast.OpBinding:
		if init := n.Sub.(*ast.BindingNode).Init; init != nil {
			f(init)
		}
	case ast.OpThrow:
		f(n.Sub.(*ast.ThrowNode).Exception)
	case ast.OpTry:
		s := n.Sub.(*ast.TryNode)
		f(s.Body)
		for _, cn := range s.Catches {
			f(cn)
		}
		if s.Finally != nil {
			f(s.Finally)
		}
	case ast.OpCatch:
		f(n.Sub.(*ast.CatchNode).Body)
	}
}

// ---- fixture harness ------------------------------------------------------

type fixture struct {
	name     string
	entry    string          // entry .clj relative to fixtures/<name>/src/
	usesJava map[string]bool // stub S35 predicate input
	// expectations for the exit criterion
	wantWhole bool              // whole-lib clojars gate expected pass?
	perNS     map[string]bool   // ns -> expected per-ns use gate pass?
	wantClass map[string]string // ns -> expected class
}

func main() {
	only := flag.String("fixture", "", "run one fixture by name")
	flag.Parse()

	base := filepath.Join("spikes", "s34-transitive-purity-validator", "fixtures")

	fixtures := []fixture{
		{
			name: "pure", entry: "pure/core.clj",
			wantWhole: true,
			perNS:     map[string]bool{"pure.core": true, "pure.util": true},
			wantClass: map[string]string{"pure.util": ClassPure, "pure.core": ClassPure},
		},
		{
			name: "go-buried", entry: "gob/core.clj",
			wantWhole: false,
			perNS:     map[string]bool{"gob.core": true, "gob.mid": true, "gob.leaf": false},
			wantClass: map[string]string{"gob.leaf": ClassGoInterop, "gob.mid": ClassPure, "gob.core": ClassPure},
		},
		{
			name: "mixed", entry: "mix/core.clj",
			wantWhole: false,
			perNS:     map[string]bool{"mix.pureside": true, "mix.goside": false},
			wantClass: map[string]string{"mix.pureside": ClassPure, "mix.goside": ClassGoInterop},
		},
		{
			name: "java-abstract", entry: "jav/core.clj",
			usesJava:  map[string]bool{"jav.leaf": true},
			wantWhole: false,
			perNS:     map[string]bool{"jav.core": true, "jav.mid": true, "jav.leaf": false},
			wantClass: map[string]string{"jav.leaf": ClassJava, "jav.mid": ClassPure, "jav.core": ClassPure},
		},
	}

	fail := 0
	for _, fx := range fixtures {
		if *only != "" && fx.name != *only {
			continue
		}
		if !runFixture(base, fx) {
			fail++
		}
	}
	if fail > 0 {
		fmt.Printf("\n%d fixture(s) did NOT meet expectation\n", fail)
		os.Exit(1)
	}
	fmt.Println("\nAll fixtures met the exit criterion.")
}

func runFixture(base string, fx fixture) bool {
	fmt.Printf("========== fixture: %s ==========\n", fx.name)
	entry := filepath.Join(base, fx.name, "src", fx.entry)

	preds := []Predicate{goInteropPredicate, javaStubPredicate(fx.usesJava), ffiPredicate}

	c, err := classify(entry, preds)
	if err != nil {
		// Report the analysis-time hard error verbatim (this is the
		// hard-error-not-nil path for unanalyzable forms, item 5).
		fmt.Printf("  classify ERROR (hard-error at analysis, file:line): %v\n", err)
		return false
	}

	fmt.Println("  ONE walk, per-namespace class map (dependency-first):")
	ok := true
	for _, name := range c.Order {
		nc := c.ByNS[name]
		mark := ""
		if want, has := fx.wantClass[name]; has && want != nc.Class {
			mark = fmt.Sprintf("  <-- WANT %s", want)
			ok = false
		}
		fmt.Printf("    %-22s %s%s\n", name, nc.Class, mark)
		for _, f := range nc.Findings {
			fmt.Printf("        · %s  [%s]\n", f.Detail, f.where())
		}
	}

	whole, reasons := c.wholeLibGate()
	fmt.Printf("  [publish clojars] whole-lib gate: %s (want %s)\n",
		passStr(whole), passStr(fx.wantWhole))
	if whole != fx.wantWhole {
		ok = false
	}
	if !whole {
		for _, r := range reasons {
			fmt.Printf("        refuse: %s uses %s (%s) at %s\n", r.NS, r.Class, r.Detail, r.where())
		}
	}

	fmt.Println("  [use <ns>] per-namespace gate (same map):")
	var names []string
	for n := range fx.perNS {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		g, rs := c.perNSGate(n)
		wm := ""
		if g != fx.perNS[n] {
			wm = fmt.Sprintf("  <-- WANT %s", passStr(fx.perNS[n]))
			ok = false
		}
		detail := ""
		if !g && len(rs) > 0 {
			detail = " (" + rs[0].Detail + " at " + rs[0].where() + ")"
		}
		fmt.Printf("    use %-22s %s%s%s\n", n, passStr(g), detail, wm)
	}

	// Prove both gates derive from the SAME map: whole-lib == AND of per-ns
	// over reachable namespaces.
	andOfPerNS := true
	for _, name := range c.Order {
		g, _ := c.perNSGate(name)
		andOfPerNS = andOfPerNS && g
	}
	if andOfPerNS != whole {
		fmt.Printf("  INCONSISTENT: whole-lib(%v) != AND-of-per-ns(%v)\n", whole, andOfPerNS)
		ok = false
	} else {
		fmt.Printf("  consistency: whole-lib == AND(per-ns over all reachable) == %v\n", whole)
	}

	fmt.Printf("  RESULT: %s\n\n", map[bool]string{true: "MET", false: "NOT MET"}[ok])
	return ok
}

func passStr(b bool) string {
	if b {
		return "PASS"
	}
	return "FAIL"
}

var _ = strings.TrimSpace
