// sigdump loads type facts for github.com/google/uuid, net/http, and os,
// and resolves function signatures programmatically (S2 step 2).
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/tools/go/packages"

	"cljgo.spikes/s2-gopackages-interop/facts"
)

func main() {
	dir, _ := os.Getwd()
	dir, _ = filepath.Abs(dir)

	mode := packages.NeedName | packages.NeedTypes
	pkgs, d, err := facts.Load(dir, mode, "github.com/google/uuid", "net/http", "os")
	if err != nil {
		fmt.Fprintln(os.Stderr, "load:", err)
		os.Exit(1)
	}
	fmt.Printf("loaded %d packages in %v (mode NeedName|NeedTypes)\n\n", len(pkgs), d)

	byPath := map[string]*packages.Package{}
	for _, p := range pkgs {
		byPath[p.PkgPath] = p
	}

	// Resolve specific signatures the generator will consume.
	targets := []struct{ pkg, fn string }{
		{"github.com/google/uuid", "NewRandom"}, // (UUID, error)
		{"github.com/google/uuid", "Parse"},     // (UUID, error)
		{"github.com/google/uuid", "New"},       // UUID (panics internally) — plain shape
		{"os", "Open"},                          // (*os.File, error)
		{"os", "LookupEnv"},                     // (string, bool) — comma-ok shape
		{"os", "Getenv"},                        // string — plain
		{"net/http", "Get"},                     // (*http.Response, error)
		{"net/http", "NewRequest"},              // (*http.Request, error)
		{"net/http", "Handle"},                  // no results
	}
	for _, t := range targets {
		s, err := facts.ResolveFunc(byPath[t.pkg], t.fn)
		if err != nil {
			fmt.Println("ERROR:", err)
			continue
		}
		fmt.Println(s)
	}

	// Prove we can enumerate whole packages (registry/completion path).
	for _, p := range []string{"github.com/google/uuid", "os", "net/http"} {
		all := facts.AllFuncs(byPath[p])
		nErr := 0
		for _, s := range all {
			if s.TrailingError {
				nErr++
			}
		}
		fmt.Printf("\n%s: %d exported funcs, %d with trailing error", p, len(all), nErr)
	}
	fmt.Println()
}
