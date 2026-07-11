// cachetest prototypes the pkg/host signature-cache design question:
// once we know a package's export-data file (one packages.Load with
// NeedExportFile), can we skip `go list` on subsequent loads and decode
// types straight from export data? Measures both halves.
package main

import (
	"encoding/json"
	"fmt"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/tools/go/gcexportdata"
	"golang.org/x/tools/go/packages"

	"cljgo.spikes/s2-gopackages-interop/facts"
)

func main() {
	dir, _ := os.Getwd()
	dir, _ = filepath.Abs(dir)
	targets := []string{"os", "net/http", "github.com/google/uuid", "github.com/gorilla/websocket"}

	// Phase 1: one go/packages call to learn export-file locations (the part
	// that costs a `go list` subprocess).
	pkgs, d, err := facts.Load(dir, packages.NeedName|packages.NeedExportFile, targets...)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load:", err)
		os.Exit(1)
	}
	fmt.Printf("phase1 go list (NeedName|NeedExportFile): %v\n", d)
	files := map[string]string{}
	for _, p := range pkgs {
		if p.ExportFile == "" {
			fmt.Fprintf(os.Stderr, "no export file for %s\n", p.PkgPath)
			os.Exit(1)
		}
		files[p.PkgPath] = p.ExportFile
	}

	// Phase 2: decode export data directly — NO go list, NO subprocess.
	fset := token.NewFileSet()
	imports := map[string]*types.Package{}
	t0 := time.Now()
	tpkgs := map[string]*types.Package{}
	for _, path := range targets {
		f, err := os.Open(files[path])
		if err != nil {
			panic(err)
		}
		r, err := gcexportdata.NewReader(f)
		if err != nil {
			panic(err)
		}
		tp, err := gcexportdata.Read(r, fset, imports, path)
		f.Close()
		if err != nil {
			panic(err)
		}
		tpkgs[path] = tp
	}
	d2 := time.Since(t0)
	fmt.Printf("phase2 gcexportdata decode of %d packages: %v\n", len(targets), d2)

	// Sanity: resolve the same signatures from the decoded data.
	sig := func(pkg, name string) {
		obj := tpkgs[pkg].Scope().Lookup(name)
		fn := obj.(*types.Func).Type().(*types.Signature)
		fmt.Printf("  %s.%s: %s\n", pkg, name, fn)
	}
	sig("github.com/google/uuid", "NewRandom")
	sig("os", "Open")
	sig("net/http", "Get")
	sig("github.com/gorilla/websocket", "NewClient")

	// Phase 3: sizes + a serialized-Sig round trip for comparison.
	for _, path := range targets {
		st, _ := os.Stat(files[path])
		fmt.Printf("  export data %-32s %7.1f KB\n", path, float64(st.Size())/1024)
	}
	pkgs2, _, err := facts.Load(dir, packages.NeedName|packages.NeedTypes, "github.com/google/uuid")
	if err != nil {
		panic(err)
	}
	sigs := facts.AllFuncs(pkgs2[0])
	blob, _ := json.Marshal(sigs)
	t0 = time.Now()
	var back []facts.Sig
	for i := 0; i < 100; i++ {
		json.Unmarshal(blob, &back)
	}
	fmt.Printf("phase3 JSON Sig cache: uuid = %d sigs, %d bytes, unmarshal %v/load\n",
		len(sigs), len(blob), time.Since(t0)/100)
}
