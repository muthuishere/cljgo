// Command prototype mirrors pkg/emit/hostfacts.go's loadHostFacts exactly
// (same packages.Config shape, same signature-fact extraction) but is a
// STANDALONE program — no dependency on the cljgo module — so it can be
// pointed at any directory and prove (or disprove) that go/packages can
// resolve host facts with no cljgo source tree anywhere on disk.
//
// Usage: prototype <dir> <fn> <pkgpath> <member> [<pkgpath> <member> ...]
//
//	dir     - the directory go/packages loads in (cfg.Dir)
//	the remaining args are (pkgPath, member) pairs to resolve and print
//
// Exits non-zero with the raw go/packages error on failure — the whole
// point is to see the REAL error a downloaded-binary user would hit.
package main

import (
	"fmt"
	"go/types"
	"os"
	"time"

	"golang.org/x/tools/go/packages"
)

func main() {
	if len(os.Args) < 4 || len(os.Args)%2 != 0 {
		fmt.Fprintln(os.Stderr, "usage: prototype <dir> <pkgpath> <member> [<pkgpath> <member> ...]")
		os.Exit(2)
	}
	dir := os.Args[1]
	pairs := os.Args[2:]

	paths := make([]string, 0, len(pairs)/2)
	seen := map[string]bool{}
	for i := 0; i < len(pairs); i += 2 {
		p := pairs[i]
		if !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}

	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedTypes, Dir: dir}
	t0 := time.Now()
	pkgs, err := packages.Load(cfg, paths...)
	elapsed := time.Since(t0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "go/packages load FAILED after %s: %v\n", elapsed, err)
		os.Exit(1)
	}

	byPath := map[string]*packages.Package{}
	hadErr := false
	for _, p := range pkgs {
		for _, e := range p.Errors {
			fmt.Fprintf(os.Stderr, "package %s error: %s\n", p.PkgPath, e.Msg)
			hadErr = true
		}
		byPath[p.PkgPath] = p
	}
	if hadErr {
		fmt.Fprintf(os.Stderr, "go/packages load completed with package errors after %s\n", elapsed)
		os.Exit(1)
	}

	fmt.Printf("go/packages load OK in %s (dir=%s)\n", elapsed, dir)

	universeError := types.Universe.Lookup("error").Type()

	for i := 0; i < len(pairs); i += 2 {
		pkgPath, member := pairs[i], pairs[i+1]
		p, ok := byPath[pkgPath]
		if !ok || p.Types == nil {
			fmt.Printf("  %s.%s: NOT LOADED\n", pkgPath, member)
			os.Exit(1)
		}
		obj := p.Types.Scope().Lookup(member)
		if obj == nil {
			fmt.Printf("  %s.%s: NOT FOUND\n", pkgPath, member)
			os.Exit(1)
		}
		fn, ok := obj.(*types.Func)
		if !ok {
			fmt.Printf("  %s.%s: %s (not a func)\n", pkgPath, member, obj)
			continue
		}
		s := fn.Type().(*types.Signature)
		trailingErr, trailingBool := false, false
		if n := s.Results().Len(); n > 0 {
			last := s.Results().At(n - 1).Type()
			if types.Identical(last, universeError) {
				trailingErr = true
			} else if n >= 2 {
				if b, ok := last.(*types.Basic); ok && b.Kind() == types.Bool {
					trailingBool = true
				}
			}
		}
		fmt.Printf("  %s.%s%s  [variadic=%v trailingError=%v trailingBool=%v]\n",
			pkgPath, member, s.String(), s.Variadic(), trailingErr, trailingBool)
	}
}
