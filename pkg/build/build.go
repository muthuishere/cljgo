// Package build is the Zig-style build system (ADR 0021, design/08 §1).
// `cljgo build` with no file arg loads ./build.cljgo, evaluates its
// (defn build [b] …) through the interpreter against the embedded
// cljgo.build library (core/build.cljg), reads back a plain-data build
// plan (the step DAG), and executes the requested step.
//
// The plan crosses the Go↔cljgo boundary as ordinary persistent data — an
// atom of maps/vectors — so no host object leaks; LoadPlan derefs it once
// and reads it with lang.Get / lang.ToSlice. Step execution (emit the
// artifact, synthesize go.mod, `go get` third-party modules, `go build`)
// lives here in Go, reusing the pkg/emit machinery `cljgo build <file>`
// already uses.
package build

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/muthuishere/cljgo/pkg/deps"
	"github.com/muthuishere/cljgo/pkg/emit"
	"github.com/muthuishere/cljgo/pkg/eval"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// BuildFileName is the canonical project-root build description name (ADR
// 0021) — what `cljgo new` emits and what error messages name. `cljgo build`
// accepts any of BuildFileNames (ADR 0051).
const BuildFileName = "build.cljgo"

// BuildFileNames are the accepted build-description names, most-specific-first
// (ADR 0051): cljgo-native `.cljgo`/`.cljg` before the portable `.clj`.
var BuildFileNames = []string{"build.cljgo", "build.cljg", "build.clj"}

// FindBuildFile returns the first accepted build file present in dir (ADR 0051
// precedence), or "" if none exists.
func FindBuildFile(dir string) string {
	for _, name := range BuildFileNames {
		p := filepath.Join(dir, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return ""
}

// Artifact is one buildable output declared by (exe b …) (lib/kinds are
// later milestones). Main is the entry .cljg path, relative to the
// build.cljgo directory.
type Artifact struct {
	Name string
	Main string
	Kind string
	// Module is a library artifact's Go module path / Clojure coordinate
	// (ADR 0050 `lib`), used by `cljgo publish`. Empty for exe artifacts.
	Module string
}

// GoRequire is a pinned third-party Go module from (go-require art …) —
// this replaces deps.edn (ADR 0021 B2).
type GoRequire struct {
	Path    string
	Version string
}

// Step is a node in the executed DAG: an install or run of an artifact.
type Step struct {
	Type string // "install" | "run"
	Name string // artifact name
}

// Plan is the finalized build graph read back from the builder atom.
type Plan struct {
	ProjectDir string
	Artifacts  []Artifact
	GoRequires []GoRequire
	// Deps are the declared (dep …) dependencies (ADR 0048), resolved before
	// compilation; their source roots feed the interpreter load path (slot 3)
	// and their Go-requires merge into the consumer go.mod.
	Deps           []deps.Dep
	AcceptVersions map[string]string // (accept-version …): module -> version
	AllowCaps      map[string]bool   // (allow-capability …): acknowledged capabilities
	Steps          []Step
	Default        string // default step type when `cljgo build` gets no step arg
}

// LoadPlan evaluates buildFile's (defn build [b] …) through a fresh
// interpreter and returns the resulting plan. The build description gets
// the full language (comptime context, ADR 0021 decision 4).
func LoadPlan(buildFile string) (*Plan, error) {
	absDir, err := filepath.Abs(filepath.Dir(buildFile))
	if err != nil {
		return nil, err
	}
	ev := eval.New()

	// Refer the embedded cljgo.build publics (exe/install/run/go-require/…)
	// into the current (user) ns so build.cljgo calls them unqualified.
	if _, err := evalString(ev, "(clojure.core/require '[cljgo.build :refer :all])"); err != nil {
		return nil, fmt.Errorf("boot cljgo.build: %w", err)
	}

	// Load build.cljgo (defines `build`, plus any helper defs) form by form,
	// exactly as a file load — *ns*/*file* bound for the duration.
	if err := loadFileForms(ev, buildFile); err != nil {
		return nil, err
	}

	// Construct a fresh builder, run the user's build fn against it, hand
	// the atom back. `build` is resolved from the ns build.cljgo defined it in.
	res, err := evalString(ev, "(let [b (cljgo.build/make-builder)] (build b) b)")
	if err != nil {
		return nil, fmt.Errorf("evaluating build fn: %w", err)
	}
	atom, ok := res.(*lang.Atom)
	if !ok {
		return nil, fmt.Errorf("internal: builder is %T, not an atom", res)
	}
	plan, err := planFromValue(atom.Deref())
	if err != nil {
		return nil, err
	}
	plan.ProjectDir = absDir
	return plan, nil
}

// planFromValue reads the plain-data build plan (an IPersistentMap of
// vectors of maps) into the Go Plan.
func planFromValue(v any) (*Plan, error) {
	m, ok := v.(lang.IPersistentMap)
	if !ok {
		return nil, fmt.Errorf("internal: build plan is %T, not a map", v)
	}
	p := &Plan{Default: str(lang.Get(m, kw("default")))}
	for _, a := range lang.ToSlice(lang.Get(m, kw("artifacts"))) {
		p.Artifacts = append(p.Artifacts, Artifact{
			Name:   str(lang.Get(a, kw("name"))),
			Main:   str(lang.Get(a, kw("main"))),
			Kind:   str(lang.Get(a, kw("kind"))),
			Module: str(lang.Get(a, kw("module"))),
		})
	}
	for _, r := range lang.ToSlice(lang.Get(m, kw("go-requires"))) {
		p.GoRequires = append(p.GoRequires, GoRequire{
			Path:    str(lang.Get(r, kw("path"))),
			Version: str(lang.Get(r, kw("version"))),
		})
	}
	// (dep name {:git … :ref … :subdir … | :path …}) — ADR 0048.
	for _, d := range lang.ToSlice(lang.Get(m, kw("deps"))) {
		p.Deps = append(p.Deps, deps.Dep{
			Name:   str(lang.Get(d, kw("name"))),
			GitURL: str(lang.Get(d, kw("git"))),
			GitRef: str(lang.Get(d, kw("ref"))),
			Subdir: str(lang.Get(d, kw("subdir"))),
			Path:   str(lang.Get(d, kw("path"))),
		})
	}
	// (accept-version module version) — a cljgo map, iterated as MapEntries.
	for _, e := range lang.ToSlice(lang.Get(m, kw("accept-versions"))) {
		me, ok := e.(*lang.MapEntry)
		if !ok {
			continue
		}
		if p.AcceptVersions == nil {
			p.AcceptVersions = map[string]string{}
		}
		p.AcceptVersions[str(me.Key())] = str(me.Val())
	}
	// (allow-capability :ffi|:cgo|:go-require) — a vector of keywords.
	for _, c := range lang.ToSlice(lang.Get(m, kw("allow-caps"))) {
		if k, ok := c.(lang.Keyword); ok {
			if p.AllowCaps == nil {
				p.AllowCaps = map[string]bool{}
			}
			p.AllowCaps[k.Name()] = true
		}
	}
	for _, s := range lang.ToSlice(lang.Get(m, kw("steps"))) {
		p.Steps = append(p.Steps, Step{
			Type: str(lang.Get(s, kw("type"))),
			Name: str(lang.Get(s, kw("name"))),
		})
	}
	return p, nil
}

// LibArtifact returns the library artifact (ADR 0050 `lib`) to publish: the one
// named, or — when name is "" — the sole lib artifact. It errors when there is
// no lib, or when name is "" and more than one lib is declared (ambiguous).
func (p *Plan) LibArtifact(name string) (Artifact, error) {
	var libs []Artifact
	for _, a := range p.Artifacts {
		if a.Kind == "lib" {
			libs = append(libs, a)
		}
	}
	if len(libs) == 0 {
		return Artifact{}, fmt.Errorf("no library artifact in %s — declare one with (lib b {:name … :main … :module …}) to publish", BuildFileName)
	}
	if name == "" {
		if len(libs) == 1 {
			return libs[0], nil
		}
		names := make([]string, len(libs))
		for i, a := range libs {
			names[i] = a.Name
		}
		return Artifact{}, fmt.Errorf("%s declares %d library artifacts (%s); name which to publish", BuildFileName, len(libs), strings.Join(names, ", "))
	}
	for _, a := range libs {
		if a.Name == name {
			return a, nil
		}
	}
	return Artifact{}, fmt.Errorf("no library artifact named %q in %s", name, BuildFileName)
}

// artifact returns the named artifact, or an error naming the miss.
func (p *Plan) artifact(name string) (Artifact, error) {
	for _, a := range p.Artifacts {
		if a.Name == name {
			return a, nil
		}
	}
	return Artifact{}, fmt.Errorf("no artifact named %q in %s", name, BuildFileName)
}

// Run executes the build. stepArg is the requested step ("" → the default
// install step, mirroring `zig build`; "run" → build+exec, `zig build run`).
// keepGen keeps the generated module dirs (else they're removed on success).
func (p *Plan) Run(stepArg string, opts emit.Options, keepGen bool) error {
	want := stepArg
	if want == "" {
		want = p.Default
	}
	if want == "" {
		want = "install"
	}

	// A plan that declares nothing is not a broken plan — it is a
	// LIBRARY (the `lib` template's build.cljgo, ADR 0047): there is no
	// binary to install, and the namespace is consumed by requiring it.
	// Say so instead of failing with "no install step", which reads as a
	// typo in the build file.
	if len(p.Artifacts) == 0 && len(p.Steps) == 0 {
		fmt.Fprintf(os.Stderr, "cljgo build: nothing to build — %s declares no artifacts.\n"+
			"A library has no binary: it is consumed by requiring its namespace, and `cljgo test` is its check.\n",
			BuildFileName)
		return nil
	}

	ran := false
	for _, s := range p.Steps {
		if s.Type != want {
			continue
		}
		art, err := p.artifact(s.Name)
		if err != nil {
			return err
		}
		switch s.Type {
		case "install":
			// The artifact name comes from build.cljgo without an extension;
			// Windows will not execute a file that lacks .exe, so the suffix
			// is ours to add — same rule as the single-file path (an explicit
			// -o is still honored verbatim).
			out := filepath.Join(p.ProjectDir, art.Name+emit.ExeSuffix)
			if _, err := p.buildArtifact(art, out, opts, keepGen); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "cljgo build: installed %s\n", out)
		case "run":
			out, err := os.CreateTemp("", "cljgo-run-"+art.Name+"-*"+emit.ExeSuffix)
			if err != nil {
				return err
			}
			out.Close()
			os.Remove(out.Name())
			if _, err := p.buildArtifact(art, out.Name(), opts, keepGen); err != nil {
				return err
			}
			defer os.Remove(out.Name())
			return runBinary(out.Name())
		default:
			return fmt.Errorf("unknown step type %q", s.Type)
		}
		ran = true
	}
	if !ran {
		return fmt.Errorf("no %q step in %s", want, BuildFileName)
	}
	return nil
}

// buildArtifact drives the emit pipeline for one artifact: compile the main
// .cljg, synthesize the module (go.mod with any go-require pins + `go get`
// them), emit main.go with host facts resolved against that module, `go
// build`. Returns the generated module dir.
func (p *Plan) buildArtifact(art Artifact, outPath string, opts emit.Options, keepGen bool) (string, error) {
	// ADR 0048: resolve declared dependencies BEFORE compiling. This publishes
	// their source roots (deps.SetResolvedRoots) so the emitter's interpreter
	// discovery pass — which evaluates require forms — finds dep namespaces
	// (load-path slot 3), and merges their Go-requires with the consumer's own,
	// hard-erroring on a version conflict here rather than letting `go mod
	// tidy`'s silent MVS be the arbiter.
	goReqs := p.GoRequires
	if len(p.Deps) > 0 {
		merged, err := p.resolveDeps()
		if err != nil {
			return "", err
		}
		goReqs = merged
	} else if len(p.GoRequires) > 0 {
		// No deps, but the consumer's own go-requires must still be
		// conflict-checked: a self-conflict (same module pinned twice at two
		// versions in one build.cljgo) must hard-error, not be silently
		// MVS-collapsed by `go mod tidy` (ADR 0048 decision 4).
		merged, err := mergeOwnGoRequires(p.GoRequires, p.AcceptVersions)
		if err != nil {
			return "", err
		}
		goReqs = merged
	}

	mainPath := art.Main
	if !filepath.IsAbs(mainPath) {
		mainPath = filepath.Join(p.ProjectDir, mainPath)
	}
	prog, err := emit.CompileProgram(mainPath)
	if err != nil {
		return "", err
	}

	genDir, err := os.MkdirTemp("", "cljgo-build-*")
	if err != nil {
		return "", err
	}
	// On any error below genDir is returned un-removed for debugging (as the
	// single-file path keeps -gen); a clean build removes it unless keepGen.

	// ADR 0033: host facts always resolve against the generated module,
	// never FindRuntimeDir()'s repo walk-up — stdlib resolves fine with no
	// go.mod yet (spike S17), so this is set unconditionally, not just
	// when go-require is in play.
	opts.HostFactsDir = genDir

	// Third-party Go modules (ADR 0021 B2): synthesize go.mod with the pins
	// and `go get` them so go/packages can resolve their type facts before
	// WriteModule's fact load runs.
	if len(goReqs) > 0 {
		reqs := make([]emit.GoModRequire, len(goReqs))
		for i, r := range goReqs {
			reqs[i] = emit.GoModRequire{Path: r.Path, Version: r.Version}
		}
		if err := emit.SynthGoMod(genDir, opts.ModuleName, opts.RuntimeDir, reqs); err != nil {
			return genDir, err
		}
		if err := goGet(genDir, goReqs); err != nil {
			return genDir, err
		}
	}

	// WriteProgram emits main.go (plus one package per file-backed
	// required namespace — ADR 0042) and writes go.mod only if absent —
	// the synthesized one above is preserved.
	if err := emit.WriteProgram(genDir, prog, opts); err != nil {
		return genDir, err
	}

	// With third-party imports present, tidy the go.sum for transitive deps
	// now that main.go references them (a no-op for pure-Go programs). The
	// require SET is already merged and conflict-checked above, so tidy only
	// fills go.sum — it is never the version arbiter (ADR 0048 decision 4).
	if len(goReqs) > 0 {
		if err := goModTidy(genDir); err != nil {
			return genDir, err
		}
	}

	if err := emit.GoBuild(genDir, outPath); err != nil {
		return genDir, err
	}
	if !keepGen {
		os.RemoveAll(genDir)
	}
	return genDir, nil
}

// resolveDeps resolves the plan's declared (dep …) dependencies (ADR 0048),
// publishes their source roots for the interpreter load path (slot 3), and
// returns the merged, conflict-checked Go-require set (the consumer's own
// go-requires + every resolved dependency's) for the generated go.mod. It
// loads build.lock.edn adjacent to build.cljgo; when the lock is absent it
// resolves fresh and writes it (first-resolution generates the lock). A
// version conflict hard-errors here, before the go.mod write.
func (p *Plan) resolveDeps() ([]GoRequire, error) {
	lockPath := filepath.Join(p.ProjectDir, "build.lock.edn")
	lock, err := deps.LoadLock(lockPath)
	if err != nil {
		return nil, err
	}
	update := lock == nil // absent lock → generate it
	opts := deps.ResolveOptions{
		ProjectDir:     p.ProjectDir,
		Lock:           lock,
		Update:         update,
		AllowCaps:      p.AllowCaps,
		AcceptVersions: p.AcceptVersions,
		VendorDir:      filepath.Join(p.ProjectDir, "vendor"),
	}
	resolved, err := deps.Resolve(p.Deps, opts)
	if err != nil {
		return nil, err
	}
	// Slot 3: publish resolved roots so both legs' interpreter loads see them.
	deps.SetResolvedRoots(resolved.Roots)

	// Merge the consumer's own go-requires with the resolved dependency set,
	// hard-erroring on a duplicate module at two versions (naming both) unless
	// an (accept-version …) override pins one — never delegating to go mod
	// tidy's silent MVS (decision 4).
	own := make([]deps.GoReq, len(p.GoRequires))
	for i, r := range p.GoRequires {
		own[i] = deps.GoReq{Path: r.Path, Version: r.Version}
	}
	mergedReq, err := deps.MergeGoRequires([][]deps.GoReq{own, resolved.GoRequires}, p.AcceptVersions)
	if err != nil {
		return nil, err
	}
	merged := make([]GoRequire, len(mergedReq))
	for i, g := range mergedReq {
		merged[i] = GoRequire{Path: g.Path, Version: g.Version}
	}

	if update {
		if err := deps.WriteLock(lockPath, resolved.Lock); err != nil {
			return nil, err
		}
	}
	return merged, nil
}

// mergeOwnGoRequires conflict-checks a plan's own go-requires (no dependencies
// involved) through the same layer as resolveDeps, so a self-conflict hard-errors
// rather than reaching `go mod tidy`'s silent MVS (ADR 0048 decision 4).
func mergeOwnGoRequires(reqs []GoRequire, accept map[string]string) ([]GoRequire, error) {
	own := make([]deps.GoReq, len(reqs))
	for i, r := range reqs {
		own[i] = deps.GoReq{Path: r.Path, Version: r.Version}
	}
	mergedReq, err := deps.MergeGoRequires([][]deps.GoReq{own}, accept)
	if err != nil {
		return nil, err
	}
	merged := make([]GoRequire, len(mergedReq))
	for i, g := range mergedReq {
		merged[i] = GoRequire{Path: g.Path, Version: g.Version}
	}
	return merged, nil
}

// ResolveProjectDeps loads buildFile and, if it declares (dep …) dependencies,
// resolves them and publishes their source roots for the interpreter load path
// (ADR 0048 slot 3). It is used by the `cljgo run` bootstrap so a project with
// locked dependencies resolves them the same way `cljgo build` does — one
// resolver, one order, no REPL-vs-binary divergence. It never evaluates a
// dependency's build fn (decision 5); it reads the lock and manifests as data.
func ResolveProjectDeps(buildFile string) error {
	plan, err := LoadPlan(buildFile)
	if err != nil {
		return err
	}
	if len(plan.Deps) == 0 {
		return nil
	}
	_, err = plan.resolveDeps()
	return err
}

// goGet fetches each pinned module into the module cache and records it in
// go.sum (`go get path@version`). This is the network step; when offline it
// fails with go's own diagnostic (the caller surfaces it).
func goGet(dir string, reqs []GoRequire) error {
	for _, r := range reqs {
		spec := r.Path
		if r.Version != "" {
			spec = r.Path + "@" + r.Version
		}
		cmd := exec.Command("go", "get", spec)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("go get %s: %w\n%s", spec, err, out)
		}
	}
	return nil
}

func goModTidy(dir string) error {
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go mod tidy: %w\n%s", err, out)
	}
	return nil
}

func runBinary(path string) error {
	cmd := exec.Command(path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// --- interpreter helpers ----------------------------------------------------

// evalString reads and evaluates a single form from src through ev.
func evalString(ev *eval.Evaluator, src string) (any, error) {
	rd := reader.New(strings.NewReader(src),
		reader.WithFilename("<build-driver>"),
		reader.WithResolver(ev.ReaderResolver()))
	form, err := rd.ReadOne()
	if err != nil {
		return nil, err
	}
	return ev.EvalForm(form)
}

// loadFileForms reads and evaluates every top-level form of buildFile with
// *ns*/*file* bound, as a REPL file load (so a helper def before `build` is
// visible to it).
func loadFileForms(ev *eval.Evaluator, buildFile string) error {
	f, err := os.Open(buildFile)
	if err != nil {
		return err
	}
	defer f.Close()

	lang.PushThreadBindings(lang.NewMap(
		lang.VarCurrentNS, ev.CurrentNS(),
		lang.VarFile, buildFile,
	))
	defer lang.PopThreadBindings()

	rd := reader.New(bufio.NewReader(f),
		reader.WithFilename(buildFile),
		reader.WithResolver(ev.ReaderResolver()))
	for {
		form, err := rd.ReadOne()
		if errors.Is(err, reader.ErrEOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := ev.EvalForm(form); err != nil {
			return err
		}
	}
}

func kw(name string) lang.Keyword { return lang.NewKeyword(name) }

// str coerces a plan value to a string ("" for nil/non-strings).
func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
