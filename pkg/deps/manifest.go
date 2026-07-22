package deps

// A dependency's declarative manifest (cljgo.manifest.edn), read as DATA at
// resolve time — never by evaluating its build fn (ADR 0052 decision 5). It is
// what makes transitivity and impurity recoverable without running foreign
// code. Emitted at publish time from the library's own build.cljgo (ADR 0054
// territory); here we only consume it.

import (
	"os"
	"path/filepath"
	"sort"
)

// manifest is the consumed subset of cljgo.manifest.edn.
type manifest struct {
	Paths    []string // the dep's source roots (load-path slot 3); default ["src"]
	Impure   *Impurity
	Children []Dep    // transitive deps with fetch coords (:deps)
	ReqNames []string // sorted names of every transitive dep (lock :requires)
}

// readManifest reads dir/cljgo.manifest.edn. A missing manifest is a pure
// dependency with a default ["src"] root — not an error.
func readManifest(dir string) (*manifest, error) {
	form, err := readEDNFile(filepath.Join(dir, "cljgo.manifest.edn"))
	if os.IsNotExist(err) {
		return &manifest{Paths: []string{"src"}}, nil
	}
	if err != nil {
		return nil, err
	}

	m := &manifest{Paths: ednStrs(ednGet(form, "paths"))}
	if len(m.Paths) == 0 {
		m.Paths = []string{"src"}
	}

	imp := &Impurity{}
	// go-requires (S32 manifest) / go-require (lenient alias)
	gr := ednGet(form, "go-requires")
	if gr == nil {
		gr = ednGet(form, "go-require")
	}
	for _, g := range ednSlice(gr) {
		imp.GoRequire = append(imp.GoRequire, GoReq{
			Path:    ednStr(ednGet(g, "path")),
			Version: ednStr(ednGet(g, "version")),
		})
	}
	// ffi entries -> library names
	for _, f := range ednSlice(ednGet(form, "ffi")) {
		if lib := ednStr(ednGet(f, "lib")); lib != "" {
			imp.FFI = append(imp.FFI, lib)
		} else if s, ok := f.(string); ok {
			imp.FFI = append(imp.FFI, s)
		}
	}
	// c-link entries -> pkg-config name (fallback: first lib)
	for _, c := range ednSlice(ednGet(form, "c-link")) {
		name := ednStr(ednGet(c, "pkg-config"))
		if name == "" {
			if libs := ednStrs(ednGet(c, "libs")); len(libs) > 0 {
				name = libs[0]
			}
		}
		if name == "" {
			if s, ok := c.(string); ok {
				name = s
			}
		}
		if name != "" {
			imp.CLink = append(imp.CLink, name)
		}
	}
	if !imp.empty() {
		m.Impure = imp
	}

	// Transitive children with fetch coordinates (:deps — S33 shape).
	names := map[string]bool{}
	for _, e := range ednSlice(ednGet(form, "deps")) {
		d := Dep{
			Name:   ednStr(ednGet(e, "name")),
			GitURL: ednStr(ednGet(e, "git")),
			GitRef: ednStr(ednGet(e, "ref")),
			Subdir: ednStr(ednGet(e, "subdir")),
			Path:   ednStr(ednGet(e, "path")),
		}
		if d.Path == "" && d.GitRef == "" {
			d.GitRef = "HEAD"
		}
		m.Children = append(m.Children, d)
		if d.Name != "" {
			names[d.Name] = true
		}
	}
	// cljgo-requires (S32 shape) — names only, no fetch coords.
	for _, e := range ednSlice(ednGet(form, "cljgo-requires")) {
		if n := ednStr(ednGet(e, "name")); n != "" {
			names[n] = true
		}
	}
	for n := range names {
		m.ReqNames = append(m.ReqNames, n)
	}
	sort.Strings(m.ReqNames)
	return m, nil
}
