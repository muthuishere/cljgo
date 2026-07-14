package emit

// hostfacts loads Go package type facts via golang.org/x/tools/go/packages
// and resolves package-member signatures for the AOT interop emitter
// (ADR 0010, design/05 §2). Ported from spike S2's facts package. The
// load runs in the cljgo COMPILER process, never in the emitted binary —
// the generated Go calls the resolved function directly and non-reflectively.
//
// One packages.Load resolves every host package the program references
// (batched — the whole point of the pre-scan in EmitMain); results are
// held on the generator for the duration of one emission. Signature facts
// (result count, trailing-error / comma-ok shape) drive the shaping in
// host.go entirely from types — never from hardcoded knowledge of the
// callee (the zero-binding property S2 proved).

import (
	"fmt"
	"go/types"
	"time"

	"golang.org/x/tools/go/packages"

	"github.com/muthuishere/cljgo/pkg/ast"
)

// hostSig is the resolved fact set for one exported package member: the
// param/result Go types the emitter coerces to and shapes from.
type hostSig struct {
	pkgPath  string
	pkgName  string
	funcName string
	params   []types.Type
	results  []types.Type
	variadic bool
	// trailingError: last result is exactly the universe `error` type.
	trailingError bool
	// trailingBool: comma-ok shape — >=2 results, last is `bool`.
	trailingBool bool
}

// hostFacts holds the loaded packages for one emission plus a memoized
// last-load duration (reported by the build for the perf budget).
type hostFacts struct {
	byPath map[string]*packages.Package
	dur    time.Duration
}

var universeError = types.Universe.Lookup("error").Type()

// loadHostFacts batch-loads every host package the program references and
// returns a resolver. dir is the module dir go/packages runs in (its
// go.mod's dependency graph resolves third-party import paths; stdlib
// resolves from GOROOT regardless).
func loadHostFacts(dir string, paths []string) (*hostFacts, error) {
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedTypes, Dir: dir}
	t0 := time.Now()
	pkgs, err := packages.Load(cfg, paths...)
	d := time.Since(t0)
	if err != nil {
		return nil, fmt.Errorf("go/packages load: %w", err)
	}
	byPath := map[string]*packages.Package{}
	for _, p := range pkgs {
		for _, e := range p.Errors {
			return nil, fmt.Errorf("go interop: package %s: %s", p.PkgPath, e.Msg)
		}
		byPath[p.PkgPath] = p
	}
	return &hostFacts{byPath: byPath, dur: d}, nil
}

// object resolves an exported package-level member (func / const / var)
// in the loaded type facts.
func (h *hostFacts) object(pkgPath, member string) (types.Object, *packages.Package, error) {
	p, ok := h.byPath[pkgPath]
	if !ok || p.Types == nil {
		return nil, nil, fmt.Errorf("go interop: package %q not loaded", pkgPath)
	}
	obj := p.Types.Scope().Lookup(member)
	if obj == nil || !obj.Exported() {
		return nil, nil, fmt.Errorf("go interop: %s has no exported member %q", pkgPath, member)
	}
	return obj, p, nil
}

// sig resolves a callee's signature facts for OpHostCall emission.
func (h *hostFacts) sig(pkgPath, member string) (hostSig, error) {
	obj, p, err := h.object(pkgPath, member)
	if err != nil {
		return hostSig{}, err
	}
	fn, ok := obj.(*types.Func)
	if !ok {
		return hostSig{}, fmt.Errorf("go interop: %s.%s is a %T, not a func", pkgPath, member, obj)
	}
	s := fn.Type().(*types.Signature)
	hs := hostSig{
		pkgPath:  p.PkgPath,
		pkgName:  p.Types.Name(),
		funcName: fn.Name(),
		variadic: s.Variadic(),
	}
	for i := 0; i < s.Params().Len(); i++ {
		hs.params = append(hs.params, s.Params().At(i).Type())
	}
	n := s.Results().Len()
	for i := 0; i < n; i++ {
		hs.results = append(hs.results, s.Results().At(i).Type())
	}
	if n > 0 {
		last := s.Results().At(n - 1).Type()
		// Trailing-error / comma-ok are detected BY TYPE (design/05 §2),
		// never by result name — identical to the interpreter's reflect path.
		if types.Identical(last, universeError) {
			hs.trailingError = true
		} else if n >= 2 {
			if b, ok := last.(*types.Basic); ok && b.Kind() == types.Bool {
				hs.trailingBool = true
			}
		}
	}
	return hs, nil
}

// collectHostPaths walks the analyzed forms and returns the distinct Go
// import paths referenced by OpHostRef / OpHostCall — the batch for
// loadHostFacts (empty ⇒ a non-interop program pays no go/packages cost).
func collectHostPaths(forms []*ast.Node) []string {
	set := map[string]bool{}
	for _, n := range forms {
		collectHostPathsNode(n, set)
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	return out
}

func collectHostPathsNode(n *ast.Node, set map[string]bool) {
	switch n.Op {
	case ast.OpHostRef:
		set[n.Sub.(*ast.HostRefNode).Pkg] = true
	case ast.OpHostCall:
		set[n.Sub.(*ast.HostCallNode).Pkg] = true
	}
	eachChild(n, func(c *ast.Node, _ bool) { collectHostPathsNode(c, set) })
}
