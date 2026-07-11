// bench measures packages.Load wall-clock for one pattern under a named
// LoadMode set. Repeats -n times IN-PROCESS to expose whether go/packages
// caches anything within a process (it does not — each Load shells out to
// `go list`). Cold vs warm build-cache is driven from outside via GOCACHE.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/tools/go/packages"

	"cljgo.spikes/s2-gopackages-interop/facts"
)

var modes = map[string]packages.LoadMode{
	// what the emitter actually needs: names + export-data types
	"types": packages.NeedName | packages.NeedTypes,
	// + full dependency package graph objects
	"types-deps": packages.NeedName | packages.NeedTypes | packages.NeedImports | packages.NeedDeps,
	// full source typecheck (what NOT to do): parse + typecheck from source
	"syntax": packages.NeedName | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports,
	// metadata only, no types — baseline for the `go list` overhead itself
	"name": packages.NeedName,
	// just the export-data file path (defer decoding to us)
	"exportfile": packages.NeedName | packages.NeedExportFile,
}

func main() {
	mode := flag.String("mode", "types", "one of: types, types-deps, syntax, name, exportfile")
	n := flag.Int("n", 1, "in-process repetitions")
	flag.Parse()
	patterns := flag.Args()
	if len(patterns) == 0 {
		fmt.Fprintln(os.Stderr, "usage: bench -mode types -n 2 <pattern>")
		os.Exit(2)
	}
	m, ok := modes[*mode]
	if !ok {
		fmt.Fprintln(os.Stderr, "unknown mode", *mode)
		os.Exit(2)
	}
	dir, _ := os.Getwd()
	dir, _ = filepath.Abs(dir)

	for i := 0; i < *n; i++ {
		pkgs, d, err := facts.Load(dir, m, patterns...)
		if err != nil {
			fmt.Fprintln(os.Stderr, "load:", err)
			os.Exit(1)
		}
		typed := 0
		for _, p := range pkgs {
			if p.Types != nil {
				typed++
			}
		}
		fmt.Printf("%v mode=%-10s call=%d pkgs=%d typed=%d %8.1fms\n",
			patterns, *mode, i+1, len(pkgs), typed, float64(d.Microseconds())/1000)
	}
}
