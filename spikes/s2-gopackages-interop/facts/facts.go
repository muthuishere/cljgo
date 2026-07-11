// Package facts loads Go package type facts via golang.org/x/tools/go/packages
// and resolves function signatures programmatically — the pkg/host AOT-side
// prototype for cljgo spike S2.
package facts

import (
	"fmt"
	"go/types"
	"time"

	"golang.org/x/tools/go/packages"
)

// Param is one parameter or result of a Go function.
type Param struct {
	Name string
	Type string // types.Type.String() — fully qualified
}

// Sig is the resolved fact set for one exported function: everything the
// emitter needs to generate a direct call with [v err] shaping.
type Sig struct {
	PkgPath  string
	PkgName  string
	FuncName string
	Params   []Param
	Results  []Param
	Variadic bool
	// TrailingError: last result is exactly the universe `error` type.
	TrailingError bool
	// TrailingBool: comma-ok shape — last result is `bool` and there are >=2 results.
	TrailingBool bool
}

func (s Sig) String() string {
	shape := "plain"
	if s.TrailingError {
		shape = "[v err]"
	} else if s.TrailingBool {
		shape = "[v ok]"
	}
	return fmt.Sprintf("%s.%s params=%v results=%v variadic=%v shape=%s",
		s.PkgPath, s.FuncName, s.Params, s.Results, s.Variadic, shape)
}

var universeError = types.Universe.Lookup("error").Type()

// Load loads packages for the given patterns with the given mode and returns
// them plus wall-clock duration. dir is the module dir go/packages runs in
// (determines the go.mod whose dependency graph resolves third-party paths).
func Load(dir string, mode packages.LoadMode, patterns ...string) ([]*packages.Package, time.Duration, error) {
	cfg := &packages.Config{Mode: mode, Dir: dir}
	t0 := time.Now()
	pkgs, err := packages.Load(cfg, patterns...)
	d := time.Since(t0)
	if err != nil {
		return nil, d, err
	}
	for _, p := range pkgs {
		for _, e := range p.Errors {
			return nil, d, fmt.Errorf("package %s: %s", p.PkgPath, e.Msg)
		}
	}
	return pkgs, d, nil
}

// ResolveFunc looks up an exported package-level function in loaded type facts.
func ResolveFunc(pkg *packages.Package, name string) (Sig, error) {
	if pkg.Types == nil {
		return Sig{}, fmt.Errorf("%s: no type info loaded (mode missing NeedTypes?)", pkg.PkgPath)
	}
	obj := pkg.Types.Scope().Lookup(name)
	if obj == nil {
		return Sig{}, fmt.Errorf("%s: no symbol %q", pkg.PkgPath, name)
	}
	fn, ok := obj.(*types.Func)
	if !ok {
		return Sig{}, fmt.Errorf("%s.%s is a %T, not a func", pkg.PkgPath, name, obj)
	}
	return sigFacts(pkg, fn), nil
}

func sigFacts(pkg *packages.Package, fn *types.Func) Sig {
	sig := fn.Type().(*types.Signature)
	s := Sig{
		PkgPath:  pkg.PkgPath,
		PkgName:  pkg.Types.Name(),
		FuncName: fn.Name(),
		Variadic: sig.Variadic(),
	}
	qual := func(p *types.Package) string { return p.Path() } // fully-qualified type strings
	for i := 0; i < sig.Params().Len(); i++ {
		v := sig.Params().At(i)
		s.Params = append(s.Params, Param{Name: v.Name(), Type: types.TypeString(v.Type(), qual)})
	}
	n := sig.Results().Len()
	for i := 0; i < n; i++ {
		v := sig.Results().At(i)
		s.Results = append(s.Results, Param{Name: v.Name(), Type: types.TypeString(v.Type(), qual)})
	}
	if n > 0 {
		last := sig.Results().At(n - 1).Type()
		// Trailing-error detection is BY TYPE, not by name (doc 05 §2).
		if types.Identical(last, universeError) {
			s.TrailingError = true
		} else if n >= 2 {
			if b, ok := last.(*types.Basic); ok && b.Kind() == types.Bool {
				s.TrailingBool = true
			}
		}
	}
	return s
}

// AllFuncs enumerates every exported package-level function (what a registry
// generator or completion engine would walk).
func AllFuncs(pkg *packages.Package) []Sig {
	var out []Sig
	scope := pkg.Types.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if fn, ok := obj.(*types.Func); ok && fn.Exported() {
			out = append(out, sigFacts(pkg, fn))
		}
	}
	return out
}
