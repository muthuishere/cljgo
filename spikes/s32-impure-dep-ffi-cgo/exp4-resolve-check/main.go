// S32 exp4 — a RESOLVE-TIME impurity check.
//
// The claim under test (ADR 0052 decision 6, third bullet): impurity can be
// determined from a declarative manifest, before fetching or building, and
// reported better than the link-time / init-time failures exp2 and exp3
// measured.
//
// Deliberate properties:
//   - the manifest is read with cljgo's OWN reader (pkg/reader) — no new
//     parser, and it proves the shape is ordinary EDN;
//   - a dependency's (defn build [b] ...) is NEVER evaluated (ADR 0052 §5);
//   - the host probe for an :ffi soname is a real purego Dlopen, i.e. the
//     exact call the program would make at run time, executed now instead;
//   - the c-link probe deliberately does NOT shell out to a compiler: it
//     checks for pkg-config, which (measured on this host) is absent, which
//     is itself a finding about ADR 0021's `{:pkg-config "sqlite3"}` surface.
//
// Spike code. Never merges into pkg/ (ADR 0027).
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/ebitengine/purego"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// ---------- manifest model ----------

type ffiReq struct {
	Lib        string
	Soname     map[string]string
	MinVersion string
	Symbols    []string
}

type cReq struct {
	PkgConfig  string
	Libs       []string
	Headers    []string
	MinVersion string
}

type goReq struct{ Path, Version string }

type manifest struct {
	Name         string
	Version      string
	Capabilities []string
	CljgoReqs    []string
	GoReqs       []goReq
	FFI          []ffiReq
	CLink        []cReq
}

// ---------- EDN helpers over cljgo's reader ----------

func kw(m any, name string) any {
	l, ok := m.(lang.ILookup)
	if !ok {
		return nil
	}
	return l.ValAt(lang.NewKeyword(name))
}

func str(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func strs(v any) []string {
	if v == nil {
		return nil
	}
	var out []string
	for _, e := range lang.ToSlice(v) {
		out = append(out, strings.TrimPrefix(str(e), ":"))
	}
	sort.Strings(out)
	return out
}

func strMap(v any) map[string]string {
	out := map[string]string{}
	if v == nil {
		return out
	}
	for s := lang.Seq(v); s != nil; s = s.Next() {
		if me, ok := s.First().(*lang.MapEntry); ok {
			out[strings.TrimPrefix(str(me.Key()), ":")] = str(me.Val())
		}
	}
	return out
}

func parseManifest(path string) (*manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	form, err := reader.ReadString(string(b))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	m := &manifest{
		Name:         str(kw(form, "name")),
		Version:      str(kw(form, "version")),
		Capabilities: strs(kw(form, "capabilities")),
	}
	for _, d := range lang.ToSlice(kw(form, "cljgo-requires")) {
		m.CljgoReqs = append(m.CljgoReqs, str(kw(d, "name")))
	}
	for _, d := range lang.ToSlice(kw(form, "go-requires")) {
		m.GoReqs = append(m.GoReqs, goReq{Path: str(kw(d, "path")), Version: str(kw(d, "version"))})
	}
	for _, d := range lang.ToSlice(kw(form, "ffi")) {
		m.FFI = append(m.FFI, ffiReq{
			Lib:        str(kw(d, "lib")),
			Soname:     strMap(kw(d, "soname")),
			MinVersion: str(kw(d, "min-version")),
			Symbols:    strs(kw(d, "symbols")),
		})
	}
	for _, d := range lang.ToSlice(kw(form, "c-link")) {
		m.CLink = append(m.CLink, cReq{
			PkgConfig:  str(kw(d, "pkg-config")),
			Libs:       strs(kw(d, "libs")),
			Headers:    strs(kw(d, "headers")),
			MinVersion: str(kw(d, "min-version")),
		})
	}
	return m, nil
}

// ---------- the check ----------

type finding struct{ dep, kind, msg, fix string }

func main() {
	root, _ := os.Getwd()
	policyFile := "build.cljgo.policy.edn"
	if len(os.Args) > 1 {
		policyFile = os.Args[1]
	}
	policyForm, err := readEDN(filepath.Join(root, policyFile))
	if err != nil {
		fmt.Fprintln(os.Stderr, "policy:", err)
		os.Exit(2)
	}
	allow := map[string]bool{}
	for _, c := range strs(kw(policyForm, "allow")) {
		allow[c] = true
	}
	needCross := kw(policyForm, "require-cross-compile") == true
	roots := strs(kw(policyForm, "deps"))

	fmt.Printf("cljgo resolve: project %s, host %s/%s\n",
		str(kw(policyForm, "project")), runtime.GOOS, runtime.GOARCH)
	fmt.Printf("               policy: allow=%v require-cross-compile=%v\n\n", keys(allow), needCross)

	// Transitive walk over MANIFESTS ONLY. No build fn is evaluated.
	seen := map[string]bool{}
	var order []*manifest
	var walk func(name, via string)
	walk = func(name, via string) {
		if seen[name] {
			return
		}
		seen[name] = true
		m, err := parseManifest(filepath.Join(root, "deps", name, "cljgo.manifest.edn"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "  cannot read manifest for %s (required by %s): %v\n", name, via, err)
			os.Exit(2)
		}
		order = append(order, m)
		for _, d := range m.CljgoReqs {
			walk(d, name)
		}
	}
	for _, r := range roots {
		walk(r, "the project")
	}

	var findings []finding
	goMerge := map[string][]string{} // module path -> "version (dep)"

	for _, m := range order {
		label := m.Name + " " + m.Version
		if len(m.Capabilities) == 0 {
			fmt.Printf("  %-22s pure\n", label)
		} else {
			fmt.Printf("  %-22s IMPURE: %s\n", label, strings.Join(m.Capabilities, " "))
		}
		for _, g := range m.GoReqs {
			goMerge[g.Path] = append(goMerge[g.Path], g.Version+" ("+m.Name+")")
		}
		for _, c := range m.Capabilities {
			if !allow[c] {
				findings = append(findings, finding{m.Name, c,
					fmt.Sprintf("declares capability :%s, which this project does not allow", c),
					fmt.Sprintf("add :%s to the project's allowed capabilities, or drop the dependency", c)})
			}
		}
		// :cgo — cross-compilation and toolchain consequences, decided from
		// the manifest alone, no build attempted.
		for _, c := range m.CLink {
			if needCross {
				findings = append(findings, finding{m.Name, "cgo",
					fmt.Sprintf("c-link %q forces CGO_ENABLED=1; this project requires cross-compilation, which cgo against a third-party system library cannot do (S32 exp2: zig-cc supplies libc, not the library's headers)", c.PkgConfig),
					"drop the dependency, replace it with a pure-Go or ffi equivalent, or drop the cross-compile requirement and build on each target"})
			}
			if _, err := exec.LookPath("pkg-config"); err != nil {
				findings = append(findings, finding{m.Name, "cgo",
					fmt.Sprintf("needs pkg-config %q (>= %s) to locate its C library; pkg-config is not installed on this host", c.PkgConfig, c.MinVersion),
					"install pkg-config and the " + c.PkgConfig + " development package"})
			}
		}
		// :ffi — probe the host with the SAME dlopen the program would do.
		for _, f := range m.FFI {
			so, ok := f.Soname[runtime.GOOS]
			if !ok {
				findings = append(findings, finding{m.Name, "ffi",
					fmt.Sprintf("declares no soname for %s (has: %v)", runtime.GOOS, keys2(f.Soname)),
					"the dependency does not support this OS"})
				continue
			}
			h, err := purego.Dlopen(so, purego.RTLD_NOW|purego.RTLD_GLOBAL)
			if err != nil {
				findings = append(findings, finding{m.Name, "ffi",
					fmt.Sprintf("needs the system library %q (>= %s) at run time; it is not loadable on this host", so, f.MinVersion),
					"install " + f.Lib + " (" + so + ")"})
				continue
			}
			// Symbols too — exp3 showed RegisterLibFunc PANICS on a missing
			// symbol, from init(), so checking now is strictly better.
			for _, sym := range f.Symbols {
				if _, err := purego.Dlsym(h, sym); err != nil {
					findings = append(findings, finding{m.Name, "ffi",
						fmt.Sprintf("declares symbol %q in %s, which the installed copy does not export", sym, so),
						"upgrade " + f.Lib + " to >= " + f.MinVersion})
				}
			}
		}
	}

	// Go-module merge conflicts (ADR 0052 decision 6, first bullet).
	for path, vs := range goMerge {
		if len(uniq(vs)) > 1 {
			findings = append(findings, finding{"<graph>", "go-require",
				fmt.Sprintf("Go module %s is pinned at conflicting versions: %s", path, strings.Join(vs, ", ")),
				"pin one version explicitly in build.cljgo"})
		}
	}

	fmt.Println()
	if len(findings) == 0 {
		fmt.Println("resolve ok: dependency graph satisfies the project's purity policy.")
		return
	}
	fmt.Fprintf(os.Stderr, "error: cljgo resolve refused the dependency graph (%d problem(s)), before fetching or building anything.\n\n", len(findings))
	for _, f := range findings {
		fmt.Fprintf(os.Stderr, "  %s [:%s]\n    %s\n    fix: %s\n\n", f.dep, f.kind, f.msg, f.fix)
	}
	os.Exit(1)
}

func readEDN(path string) (any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return reader.ReadString(string(b))
}

func keys(m map[string]bool) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func keys2(m map[string]string) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func uniq(v []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range v {
		k := strings.Fields(s)[0]
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}
