package emit

import (
	"bufio"
	"bytes"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/muthuishere/cljgo/pkg/ast"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/version"
)

// runtimeModule is the module emitted code links against — the ONE
// runtime package rule (design/00 §4.2): generated code imports only
// pkg/lang (plus pkg/eval for the v0 bootstrap, see below).
const runtimeModule = "github.com/muthuishere/cljgo"

// Options configures program emission.
type Options struct {
	// ModuleName is the generated module's path. Default "cljgo.gen/main".
	ModuleName string
	// RuntimeDir is the cljgo source tree the generated go.mod `replace`s
	// the runtime to (the -runtime flag). Empty → SynthGoMod's ADR 0028
	// resolution: $CLJGO_SRC, else release binaries pin the published
	// module at their own version, else walk-up repo detection.
	RuntimeDir string
	// HostFactsDir overrides the module directory go/packages loads host
	// type facts from (design/05 §2). Empty → RuntimeDir (the default: only
	// stdlib + the runtime's own deps resolve). build.cljgo's `go-require`
	// (ADR 0021 B2) points this at the generated module dir — once its
	// synthesized go.mod requires + `go get` the third-party modules, the
	// emitter resolves their signatures with zero hand-written bindings.
	HostFactsDir string
	// PrintLastValue makes main() print pr-str of the last top-level
	// form's value — the conformance dual-harness contract (ADR 0007).
	PrintLastValue bool
	// UsesBri marks a program that requires a bri.* namespace (ADR 0071).
	// When set, the emitted main package blank-imports pkg/briaot — the
	// AOT-compiled bri framework — so the replayed (require 'bri.http)
	// resolves at runtime to real net/http + the framework fns with no
	// interpreter linked. WriteProgram sets it from Program.UsesBri, which
	// CompileProgram records when a bri lib provider fires during discovery.
	UsesBri bool
	// OptInBriPkgs are the pkg/briaot sub-packages (e.g. "briotel") of any
	// OPT-IN bri namespaces the program required (ADR 0074). These are
	// EXCLUDED from the always-linked umbrella pkg/briaot; the emitted main
	// package blank-imports each of them ADDITIVELY, so their heavy
	// dependency (the OpenTelemetry SDK for bri.otel) links ONLY when the
	// app requires the namespace. WriteProgram sets it from
	// Program.OptInBriPkgs, recorded when an opt-in bri provider fires
	// during discovery.
	OptInBriPkgs []string
	// EntrySrcFile is the entry namespace's logical source path. When set,
	// the emitted main package's Load() binds *file* to it (ADR 0053 dec 3),
	// so an AOT binary reports the same *file* semantics as the interpreter
	// rather than NO_SOURCE_FILE. WriteProgram sets it from the program's
	// entry path; direct EmitMain callers may leave it empty (no binding,
	// the pre-0049 behavior). A binary has no source tree at runtime, so
	// this is a logical path — semantics match, not on-disk byte-identity.
	EntrySrcFile string
}

// EmitMain compiles analyzed top-level forms into a complete
// main-package Go file: hoisted interns (sorted — deterministic output,
// design/04 §6), a guarded source-ordered Load(), and main() =
// bootstrap + Load() (+ -main invocation when the program defines one).
//
// Bootstrap (pragmatic v0, design/04 §7 non-goal fence adjusted by this
// change): macros were already expanded by the analyzer, but the
// compiled code still references clojure.core vars (builtins + core.clj
// fns), so main() calls rt.Boot(), which constructs the evaluator —
// interning the builtins and loading the embedded core.clj (~5 ms) —
// and snapshots the pristine builtins backing the guarded arithmetic
// intrinsics. AOT-compiling core.clj itself is M5 (design/04 v2).
//
// Returns gofmt-ed source; the raw pre-format text comes back too so a
// format failure — the syntax gate (ADR 0001) — is debuggable.
func EmitMain(forms []*ast.Node, opts Options) (formatted []byte, raw []byte, err error) {
	// ADR 0053 dec 3: the entry namespace's *file* binds to its logical
	// source path (matching the interpreter), not NO_SOURCE_FILE.
	return emitPackage(forms, opts, pkgSpec{pkgName: "main", isMain: true, srcFile: opts.EntrySrcFile})
}

// pkgSpec parameterizes one emitted Go package (ADR 0042 §1): the entry
// namespace emits as `package main` (isMain), each dependency namespace
// as its own registered package.
type pkgSpec struct {
	pkgName string // Go package name ("main" for the entry)
	isMain  bool   // emit main() + lastVal + -main dispatch

	// nsName + srcFile are set for dependency packages: nsName is the
	// namespace registered via rt.RegisterLib(nsName, Load); srcFile is
	// the source path Load() binds *file* to while it runs (mirroring
	// the interpreter's load frame).
	nsName  string
	srcFile string

	// bindNS names the namespace Load() binds *ns* to while it runs. Empty
	// → the requiring frame's *ns* (the ADR 0042 dependency shape: the
	// file's own (in-ns …) sets it). The AOT core compiler sets it, because
	// core's sources are loaded by the interpreter under an *ns* the loader
	// binds (core.clj has no in-ns of its own) — ADR 0046.
	bindNS string

	// depImports are the module-qualified import paths of this
	// namespace's file-backed requires, blank-imported so the linker
	// keeps (and init-registers) them (ADR 0042 §2).
	depImports []string

	// host carries preloaded interop facts shared across the module's
	// packages; nil → emitPackage pre-scans and loads its own.
	host *hostFacts
}

// emitPackage compiles analyzed top-level forms into one complete Go
// package file (the EmitMain shape, parameterized per ADR 0042).
func emitPackage(forms []*ast.Node, opts Options, spec pkgSpec) (formatted []byte, raw []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			if ee, ok := r.(*emitErr); ok {
				err = ee.err
				return
			}
			panic(r)
		}
	}()

	g := newGenerator()
	g.host = spec.host

	// Pre-scan for Go-interop references and batch-load their type facts
	// (ADR 0010, design/05 §2) BEFORE emission — a non-interop program
	// pays no go/packages cost. The load runs in this compiler process;
	// the emitted binary calls the resolved functions directly.
	if hostPaths := collectHostPaths(forms); g.host == nil && len(hostPaths) > 0 {
		// HostFactsDir is expected to be the generated module dir always
		// (ADR 0033): both Build (compile.go) and buildArtifact (build.go)
		// set it unconditionally, stdlib-only or not — go/packages resolves
		// stdlib fine with no go.mod yet. RuntimeDir/FindRuntimeDir() below
		// are reached only by callers that don't (an explicit -runtime/
		// CLJGO_SRC override, or the in-repo conformance harness calling
		// WriteModule directly).
		dir := opts.HostFactsDir
		if dir == "" {
			dir = opts.RuntimeDir
		}
		if dir == "" {
			if dir, err = FindRuntimeDir(); err != nil {
				return nil, nil, err
			}
		}
		if g.host, err = loadHostFacts(dir, hostPaths); err != nil {
			return nil, nil, err
		}
	}

	printLast := opts.PrintLastValue && spec.isMain
	for i, n := range forms {
		// Numeric emission (spike s42 / ADR 0067) starts from the boxed
		// default here: g.ni is emptyInfer. Typed regions open lazily and
		// guarded — genFn specializes int64-provable fn bodies behind an
		// `if !rt.CoreDirty()` entry, and genLoop dual-emits a numeric
		// loop met in unguarded context under the same flag.
		g.wf("// %s\n", provenance(n))
		rv := g.gen(n)
		if printLast && i == len(forms)-1 {
			if rv == "" {
				rv = "nil"
			}
			g.wf("lastVal = %s\n", rv)
		} else {
			g.discard(rv)
		}
	}

	mainVar := g.mainVar
	if !spec.isMain {
		mainVar = "" // a -main def'd in a dependency ns is just a var
	}

	var out bytes.Buffer
	fmt.Fprintf(&out, "// Code generated by cljgo build. DO NOT EDIT.\n\npackage %s\n\n", spec.pkgName)

	// Imports: lang only when referenced (a constants-only program may
	// not touch it); fmt/os per usage flags; the eval bootstrap always.
	body := g.buf.String()
	// Lifted typed funcs (g.funcs) sit outside g.buf; scan them too for the
	// lang./rt. import decisions (but they are emitted separately, not into
	// Load's body).
	scanText := body + strings.Join(g.funcs, "")
	var declText bytes.Buffer
	if len(g.decls) > 0 {
		decls := make([]hoistDecl, len(g.decls))
		copy(decls, g.decls)
		sort.Slice(decls, func(i, j int) bool { return decls[i].goName < decls[j].goName })
		declText.WriteString("var (\n")
		for _, d := range decls {
			fmt.Fprintf(&declText, "%s = %s\n", d.goName, d.init)
		}
		declText.WriteString(")\n\n")
	}
	// spec.isMain: main() always references lang now — the
	// *command-line-args* binding it emits below (fundamentals batch A1).
	usesLang := strings.Contains(scanText, "lang.") || strings.Contains(declText.String(), "lang.") ||
		printLast || mainVar != "" || spec.srcFile != "" || spec.isMain

	out.WriteString("import (\n")
	if g.usesFmt || printLast || spec.isMain {
		out.WriteString("\"fmt\"\n")
	}
	if g.usesMath {
		out.WriteString("\"math\"\n")
	}
	// isMain always imports os: the top-level recover() boundary (spike s28)
	// prints to os.Stderr and os.Exit(1)s on an uncaught runtime error.
	if mainVar != "" || spec.isMain {
		out.WriteString("\"os\"\n")
	}
	// runtime: the env-gated alloc report (spike s42) main() emits below.
	// Inert unless CLJGO_ALLOC_REPORT is set, so it never affects a user
	// or a conformance run — it exists to measure the boxing-elimination
	// win (ADR 0067). Only main packages reference it.
	if spec.isMain {
		out.WriteString("\"runtime\"\n")
	}
	// rt: the bootstrap (main), the RegisterLib init (dependency
	// packages), and the intrinsic/interop/exception helpers. A package
	// that reaches for none of them must not import it (Go rejects an
	// unused import) — pkg/coreaot's pure-Clojure packages are exactly
	// that case.
	if strings.Contains(scanText, "rt.") || strings.Contains(declText.String(), "rt.") ||
		spec.isMain || spec.nsName != "" {
		fmt.Fprintf(&out, "rt %q\n", runtimeModule+"/pkg/emit/rt")
	}
	if usesLang {
		fmt.Fprintf(&out, "lang %q\n", runtimeModule+"/pkg/lang")
	}
	// Regex literals reconstruct as &reader.Regex values (the reader's
	// own type is the one both modes carry).
	if g.usesReader {
		fmt.Fprintf(&out, "reader %q\n", runtimeModule+"/pkg/reader")
	}
	// Go-interop imports (ADR 0010): an explicit alias only when it differs
	// from the path's last segment; go/format tidies grouping.
	hostPaths := make([]string, 0, len(g.hostImports))
	for p := range g.hostImports {
		hostPaths = append(hostPaths, p)
	}
	sort.Strings(hostPaths)
	for _, p := range hostPaths {
		name := g.hostImports[p]
		if base := p[strings.LastIndex(p, "/")+1:]; base == name {
			fmt.Fprintf(&out, "%q\n", p)
		} else {
			fmt.Fprintf(&out, "%s %q\n", name, p)
		}
	}
	// The AOT core (ADR 0046): main blank-imports pkg/coreaot so the
	// linker keeps it and its init() hands Load to rt.Boot. This is what
	// makes clojure.core exist in the binary WITHOUT the interpreter.
	if spec.isMain {
		fmt.Fprintf(&out, "_ %q\n", runtimeModule+"/pkg/coreaot")
		// The AOT-compiled bri framework (ADR 0071): a bri app blank-imports
		// pkg/briaot so its init() registers a lib provider per bri namespace
		// — the replayed (require 'bri.http) then resolves to the compiled
		// framework + Go shims, no interpreter linked.
		if opts.UsesBri {
			fmt.Fprintf(&out, "_ %q\n", runtimeModule+"/pkg/briaot")
		}
		// ADR 0074: an OPT-IN bri namespace (bri.otel) is NOT in the umbrella
		// pkg/briaot. Blank-import its sub-package additively so its provider
		// registers and its isolated heavy dependency links — but ONLY here,
		// where the app actually required it.
		for _, pkg := range opts.OptInBriPkgs {
			fmt.Fprintf(&out, "_ %q\n", runtimeModule+"/pkg/briaot/"+pkg)
		}
	}
	// File-backed requires: blank imports keep the dependency packages
	// linked (and init-registered) — ADR 0042 §2.
	for _, p := range spec.depImports {
		fmt.Fprintf(&out, "_ %q\n", p)
	}
	out.WriteString(")\n\n")

	out.Write(declText.Bytes())
	// Lifted typed funcs (ADR 0067 rung 3): package-level `func nameL(…
	// int64) int64` with direct int64 recursion, emitted before Load.
	for _, fn := range g.funcs {
		out.WriteString(fn)
		out.WriteString("\n")
	}
	if printLast {
		out.WriteString("var lastVal any\n\n")
	}
	out.WriteString("var loaded = false\n\n")
	if spec.nsName != "" {
		// The replayed (require …) in the requiring namespace triggers
		// this Load at its source position via the provider registry.
		fmt.Fprintf(&out, "func init() { rt.RegisterLib(%q, Load) }\n\n", spec.nsName)
	}
	out.WriteString("// Load evaluates the namespace's top-level forms exactly once, in source order.\n")
	out.WriteString("func Load() {\nif loaded {\nreturn\n}\nloaded = true\n")
	if spec.srcFile != "" {
		// The interpreter's load frame (repl.Driver.EvalReader shape):
		// the file's in-ns is undone afterwards and *file* reads as this
		// source path while the namespace loads.
		curNS := "lang.VarCurrentNS.Deref()"
		if spec.bindNS != "" {
			curNS = fmt.Sprintf("lang.FindOrCreateNamespace(lang.NewSymbol(%q))", spec.bindNS)
		}
		fmt.Fprintf(&out, "lang.PushThreadBindings(lang.NewMap(lang.VarCurrentNS, %s, lang.VarFile, %q))\n", curNS, spec.srcFile)
		out.WriteString("defer lang.PopThreadBindings()\n")
	}
	out.WriteString(body)
	out.WriteString("}\n")
	if spec.isMain {
		out.WriteString("\nfunc main() {\n")
		// The error boundary (spike s28 P0): a runtime error must print the
		// same clean detailed line the REPL/run print — NEVER a raw Go panic
		// + goroutine stack trace. Recover, render through the one shared
		// renderer, exit non-zero.
		out.WriteString("defer func() {\nif r := recover(); r != nil {\n")
		out.WriteString("fmt.Fprintln(os.Stderr, \"error: \"+rt.RenderRecovered(r))\n")
		out.WriteString("os.Exit(1)\n}\n}()\n")
		out.WriteString("rt.Boot() // bootstrap: Go builtins + the AOT-compiled core (ADR 0046)\n")
		// *command-line-args* (fundamentals batch A1): bound from os.Args
		// before Load(), the clojure.main contract — nil root when none.
		// Same wiring as cmd/cljgo's runFile, for REPL-vs-binary parity.
		out.WriteString("if len(os.Args) > 1 {\ncla := make([]any, 0, len(os.Args)-1)\nfor _, a := range os.Args[1:] {\ncla = append(cla, a)\n}\nlang.VarCommandLineArgs.BindRoot(lang.NewList(cla...))\n}\n")
		out.WriteString("Load()\n")
		if mainVar != "" {
			out.WriteString("args := make([]any, 0, len(os.Args)-1)\nfor _, a := range os.Args[1:] {\nargs = append(args, a)\n}\n")
			fmt.Fprintf(&out, "_ = lang.Apply(%s.Get(), args)\n", mainVar)
		}
		// Env-gated alloc report (spike s42 / ADR 0067): total mallocs since
		// process start, for the boxing-elimination A/B. Inert by default.
		out.WriteString("if os.Getenv(\"CLJGO_ALLOC_REPORT\") != \"\" {\nvar ms runtime.MemStats\nruntime.ReadMemStats(&ms)\nfmt.Fprintf(os.Stderr, \"CLJGO_MALLOCS=%d\\n\", ms.Mallocs)\n}\n")
		if printLast {
			out.WriteString("fmt.Println(lang.PrintString(lastVal))\n")
		}
		out.WriteString("}\n")
	}

	raw = out.Bytes()
	formatted, err = format.Source(raw) // the syntax gate: parses or fails here
	return formatted, raw, err
}

// provenance renders a one-line comment of the original form (design/00
// §4.5: the emitter uses Node.Form for provenance; go build errors map
// back to it).
func provenance(n *ast.Node) string {
	s := lang.PrintString(n.Form)
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 90 {
		s = s[:90] + "…"
	}
	return s
}

// WriteModule emits the program and writes a self-contained generated
// Go module: main.go plus go.mod (created once, never overwritten —
// design/04 §2; the runtime resolves per SynthGoMod's ADR 0028 rules).
func WriteModule(dir string, forms []*ast.Node, opts Options) error {
	formatted, raw, err := EmitMain(forms, opts)
	if err != nil {
		if len(raw) > 0 {
			return fmt.Errorf("emit: %w\n--- unformatted source ---\n%s", err, raw)
		}
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), formatted, 0o644); err != nil {
		return err
	}

	return SynthGoMod(dir, opts.ModuleName, opts.RuntimeDir, nil)
}

// GoModRequire is a pinned third-party module requirement for the generated
// go.mod — the build-graph translation of a build.cljgo `go-require`
// (ADR 0021 B2).
type GoModRequire struct {
	Path    string
	Version string
}

// SynthGoMod writes the generated module's go.mod, plus any pinned
// third-party requires from the build graph. Written only if absent —
// user-owned once created (design/04 §2). moduleName defaults to
// "cljgo.gen/main".
//
// The runtime resolves by precedence (ADR 0028): explicit runtimeDir
// (the -runtime flag) > $CLJGO_SRC > release-pin > walk-up repo detection.
// A release binary (version.IsRelease()) with no override writes a bare
// `require github.com/muthuishere/cljgo v<Version>` — no replace — pinning
// the exact published module it was built from, so a downloaded binary +
// the Go toolchain is the whole `cljgo build` story. Everything else keeps
// the local `replace` (dev binaries in-repo, conformance harness, overrides).
func SynthGoMod(dir, moduleName, runtimeDir string, requires []GoModRequire) error {
	modPath := filepath.Join(dir, "go.mod")
	if _, err := os.Stat(modPath); err == nil {
		return nil // user-owned: never overwrite
	}
	if runtimeDir == "" {
		var err error
		switch {
		case os.Getenv("CLJGO_SRC") != "":
			// FindRuntimeDir honors CLJGO_SRC first — and validates it.
			if runtimeDir, err = FindRuntimeDir(); err != nil {
				return err
			}
		case version.IsRelease():
			// Release-pin: no replace; runtimeDir stays empty.
		default:
			if runtimeDir, err = FindRuntimeDir(); err != nil {
				return fmt.Errorf("this is a dev cljgo binary (version %s), so the generated go.mod needs a local runtime tree: %w", version.Version, err)
			}
		}
	}
	if moduleName == "" {
		moduleName = "cljgo.gen/main"
	}
	runtimeVersion := "v0.0.0" // placeholder; the replace below wins
	if runtimeDir == "" {
		runtimeVersion = "v" + version.Version
	}
	// ADR 0071: a replace-based (dev/override) build resolves the runtime
	// LOCALLY, but Go does not inherit a replaced module's own external
	// dependencies — the generated module must require + sum them itself.
	// Carry the runtime's external requires (e.g. golang.org/x/crypto, which
	// bri's argon2/bcrypt shims need) as indirect requires and copy the
	// runtime's go.sum, so a readonly `go build` links them offline. Unused
	// requires are harmless: the linker drops any module a hello-world binary
	// does not import (no binary bloat), and release-pin builds (no replace)
	// keep the tidy path untouched.
	var extRequires []GoModRequire
	if runtimeDir != "" {
		var err error
		if extRequires, err = runtimeExternalRequires(runtimeDir); err != nil {
			return err
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "module %s\n\ngo 1.26\n\n", moduleName)
	b.WriteString("require (\n")
	fmt.Fprintf(&b, "%s %s\n", runtimeModule, runtimeVersion)
	for _, r := range requires {
		fmt.Fprintf(&b, "%s %s\n", r.Path, r.Version)
	}
	for _, r := range extRequires {
		fmt.Fprintf(&b, "%s %s // indirect\n", r.Path, r.Version)
	}
	b.WriteString(")\n")
	if runtimeDir != "" {
		fmt.Fprintf(&b, "\nreplace %s => %s\n", runtimeModule, runtimeDir)
	}
	if err := os.WriteFile(modPath, []byte(b.String()), 0o644); err != nil {
		return err
	}
	// Copy the runtime's go.sum so the added external requires verify offline
	// under readonly build (skipped for release-pin, which tidies its own).
	if runtimeDir != "" {
		if sum, err := os.ReadFile(filepath.Join(runtimeDir, "go.sum")); err == nil {
			if werr := os.WriteFile(filepath.Join(dir, "go.sum"), sum, 0o644); werr != nil {
				return werr
			}
		}
	}
	return nil
}

// runtimeExternalRequires parses runtimeDir/go.mod and returns every require
// whose module path is NOT the runtime module itself — the third-party deps
// (golang.org/x/crypto, …) a replace-based generated module must re-declare
// because Go does not inherit a replaced module's dependency graph into the
// consuming module's go.mod. Line-based parse (no x/mod dependency): handles
// both the `require ( … )` block and single-line `require path version`.
func runtimeExternalRequires(runtimeDir string) ([]GoModRequire, error) {
	data, err := os.ReadFile(filepath.Join(runtimeDir, "go.mod"))
	if err != nil {
		return nil, fmt.Errorf("reading runtime go.mod for external deps: %w", err)
	}
	var out []GoModRequire
	inBlock := false
	for _, line := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "//") || t == "" {
			continue
		}
		switch {
		case strings.HasPrefix(t, "require ("):
			inBlock = true
			continue
		case inBlock && t == ")":
			inBlock = false
			continue
		case strings.HasPrefix(t, "require ") && !strings.Contains(t, "("):
			t = strings.TrimPrefix(t, "require ")
		case !inBlock:
			continue
		}
		// t is now "path version [// indirect]"; keep path + version.
		fields := strings.Fields(t)
		if len(fields) < 2 || fields[0] == runtimeModule {
			continue
		}
		out = append(out, GoModRequire{Path: fields[0], Version: fields[1]})
	}
	return out, nil
}

// ExeSuffix is ".exe" on Windows and "" everywhere else — the extension an
// executable must carry to be runnable on the host.
//
// `go build -o <name>` writes exactly <name>, adding nothing; Go only appends
// ".exe" when it picks the name itself. cljgo follows the same rule: an
// explicit -o is honored verbatim (the user's choice), while any name WE
// choose gets this suffix. Without it, `cljgo build hello.clj` on Windows
// produces a file the OS refuses to exec.
var ExeSuffix = func() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}()

// GoBuild runs `go build` on a generated module directory, producing
// outPath (made absolute so the child working directory doesn't matter).
// outPath is used verbatim — callers that choose the name themselves are
// responsible for appending ExeSuffix, mirroring `go build -o`.
// Build errors surface with the compiler's output attached.
//
// A release-pinned module (bare require, no replace — ADR 0028) first gets
// `go mod tidy` to record its go.sum entries; replace-based dev modules
// skip that step entirely (offline, and the conformance perf budgets stay
// unaffected).
func GoBuild(dir, outPath string) error {
	return GoBuildTarget(dir, outPath, "", "")
}

// GoBuildTarget is GoBuild with an explicit cross-compilation target: when
// goos/goarch are non-empty they are set as GOOS/GOARCH for the child `go
// build`, so one generated module links to any platform (ADR 0077). Empty
// goos/goarch means the host — GoBuild's behavior, byte-for-byte unchanged.
//
// CGO_ENABLED=0 is forced unconditionally: cljgo binaries are pure-Go static
// (ADR 0023), which is exactly what makes cross-compilation free (no C
// cross-toolchain). The -s -w -trimpath release stripping (ADR 0023) applies
// to every target.
func GoBuildTarget(dir, outPath, goos, goarch string) error {
	if err := ensureGoSum(dir); err != nil {
		return err
	}
	abs, err := filepath.Abs(outPath)
	if err != nil {
		return err
	}
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", abs, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if goos != "" {
		cmd.Env = append(cmd.Env, "GOOS="+goos)
	}
	if goarch != "" {
		cmd.Env = append(cmd.Env, "GOARCH="+goarch)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build: %w\n%s", err, out)
	}
	return nil
}

// ensureGoSum runs `go mod tidy` in a generated module whose go.mod requires
// the runtime by version with no replace (the ADR 0028 release-pin shape) and
// which has no go.sum yet — a bare require can't `go build` without go.sum
// entries. This is the network step for release binaries; an unpublished pin
// fails here with Go's own clear `unknown revision` diagnostic. Modules with
// a replace never reach tidy, so the dev path stays offline.
func ensureGoSum(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, "go.sum")); err == nil {
		return nil
	}
	mod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil || strings.Contains(string(mod), "\nreplace ") {
		return nil // no module here, or replace-based: go build decides
	}
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go mod tidy: %w\n%s", err, out)
	}
	return nil
}

// FindRuntimeDir locates the cljgo module source tree for the generated
// go.mod's replace directive: $CLJGO_SRC, then walking up from the
// working directory, then from the executable. Since ADR 0028 this is the
// dev/override path only — release binaries pin the published module and
// never call it from SynthGoMod.
func FindRuntimeDir() (string, error) {
	if env := os.Getenv("CLJGO_SRC"); env != "" {
		if isRuntimeDir(env) {
			return env, nil
		}
		return "", fmt.Errorf("CLJGO_SRC=%s does not contain the %s module", env, runtimeModule)
	}
	if wd, err := os.Getwd(); err == nil {
		if dir := walkUpForModule(wd); dir != "" {
			return dir, nil
		}
	}
	if exe, err := os.Executable(); err == nil {
		if dir := walkUpForModule(filepath.Dir(exe)); dir != "" {
			return dir, nil
		}
	}
	return "", fmt.Errorf("cannot locate the %s source tree (set CLJGO_SRC or run inside the repo)", runtimeModule)
}

func walkUpForModule(start string) string {
	dir := start
	for {
		if isRuntimeDir(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func isRuntimeDir(dir string) bool {
	f, err := os.Open(filepath.Join(dir, "go.mod"))
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")) == runtimeModule
		}
	}
	return false
}

// EmitBootPackage compiles one embedded boot source's analyzed forms
// into a Go package for pkg/coreaot (ADR 0046, AOT-core piece 3): an
// unregistered, guarded Load() that binds *ns* to nsName and *file* to
// srcFile — exactly the frame eval.loadBootSource pushes — and then runs
// the source's top-level forms in source order. pkg/coreaot's own Load()
// calls these in core.BootSources() order, so a compiled binary's
// namespace world is built by the same forms in the same order as the
// interpreter's, with no interpreter linked.
func EmitBootPackage(forms []*ast.Node, pkgName, nsName, srcFile string, opts Options) (formatted []byte, raw []byte, err error) {
	return emitPackage(forms, opts, pkgSpec{
		pkgName: pkgName,
		srcFile: srcFile,
		bindNS:  nsName,
	})
}
