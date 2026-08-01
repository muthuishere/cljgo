// Command cljgo is the CLI entry point (design/00 §3):
//
//	cljgo repl              start a terminal REPL on stdin/stdout
//	cljgo nrepl [--port N]  start an nREPL server for editors (ADR 0031)
//	cljgo run <file.clj>    read + evaluate a file
//	cljgo build <file.clj>  AOT-compile a file to a native binary (M2)
//	cljgo version           print the version string (also --version/-version)
//
// repl/run front the pkg/repl driver — one Read→Analyze→Eval path, per
// design/03 §7d; build fronts pkg/emit, which consumes the same
// analyzer's AST (ADR 0002).
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/muthuishere/cljgo/pkg/build"
	"github.com/muthuishere/cljgo/pkg/deps"
	"github.com/muthuishere/cljgo/pkg/diag"
	"github.com/muthuishere/cljgo/pkg/emit"
	"github.com/muthuishere/cljgo/pkg/emit/rt"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/repl"
	"github.com/muthuishere/cljgo/pkg/version"
)

// banner is the REPL greeting and `cljgo version` body:
//
//	cljgo 0.1.0 (Go 1.26.3, Clojure 1.12.5)
//
// All three numbers matter to a bug reporter: ours, the host's, the
// language's. pkg/version owns them so this can never drift from
// (cljgo-version) at the prompt.
func banner() string { return "cljgo " + version.Full() }

// versionLine mirrors the Clojure CLI's phrasing, verified against the real
// `clojure --version` ("Clojure CLI version 1.12.5.1645").
func versionLine() string { return "cljgo CLI version " + version.Full() }

func main() {
	os.Exit(run(os.Args[1:]))
}

// commandsThatSkipProjectResolution are the subcommands that must NOT resolve
// the surrounding project before running. Everything else does, once, in run()
// below.
//
// The list is deliberately a DENY-list, and the direction matters more than
// the contents: a new subcommand added tomorrow resolves by default, so the
// cost of forgetting this list is a few milliseconds of stat calls. Under an
// allow-list, forgetting means a silently broken command — which is exactly
// what happened twice (issue #185: `cljgo repl` and `cljgo nrepl` both booted
// evaluators with no project source roots and no dependency roots).
//
//   - new: creates a project; resolving the directory it is called FROM is
//     both useless and able to fail on an unrelated neighbouring project.
//   - version/help/explain: pure output, no evaluation, and must stay usable
//     inside a broken project — `cljgo explain <CODE>` is what you reach for
//     when the project does not build.
var commandsThatSkipProjectResolution = map[string]bool{
	"new": true, "version": true, "--version": true, "-v": true,
	"-version": true, "help": true, "--help": true, "-h": true,
	"explain": true,
	// build/dist/publish CREATE the lock this resolution reads. Resolving
	// first makes `cljgo build` in an unbuilt project print "no
	// build.lock.edn" and then succeed anyway, having just written it —
	// an error message contradicted two lines later by the same command
	// (measured in spike s78).
	"build": true, "dist": true, "publish": true,
	// Commands that never evaluate project code. Measured in s72: resolution
	// costs a fixed ~78 ms (locked) to ~153 ms (dep-free) and 111-221 MB,
	// against a ~10 ms baseline — 8.7x to 16.6x startup, for work whose
	// result they never read. `cljgo cache help` also printed a spurious
	// "no build.lock.edn" before doing its unrelated job.
	"cache": true, "config": true, "generate": true, "g": true,
	"routes": true, "migrate": true,
}

// commandsThatTolerateResolutionFailure keep running when project resolution
// fails; everything else treats it as fatal.
//
// Two lists is worse than one and it is still the honest encoding, because
// the policy genuinely differs. For `run`, `test`, `dev` a failure means the
// program cannot work, and continuing would surface it later as a bare
// "could not locate namespace" — the exact unnamed failure G5023 exists to
// replace (#168). For a REPL, the prompt is still the tool you would use to
// investigate, so refusing to start removes the only thing that could help.
var commandsThatTolerateResolutionFailure = map[string]bool{
	"repl": true, "nrepl": true,
}

// resolveProjectForCommand performs the ONE project resolution for this
// process, before the command dispatch below. Anchor it on `run`'s script
// path, because resolveRunDeps looks next to the source file as well as in
// the working directory — `cljgo run /elsewhere/foo.clj` must still find
// /elsewhere's project.
//
// Resolution failures are reported and NOT fatal here; commands that cannot
// proceed without it fail on their own terms afterwards (`cljgo run` still
// exits 1 via its own call). A REPL, by contrast, keeps its prompt — see
// runREPL.
func resolveProjectForCommand(args []string) (exit int, stop bool) {
	if len(args) == 0 || commandsThatSkipProjectResolution[args[0]] {
		return 0, false
	}
	anchor := ""
	if args[0] == "run" && len(args) > 1 {
		anchor = args[1]
	}
	if err := resolveRunDeps(anchor); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		if !commandsThatTolerateResolutionFailure[args[0]] {
			return 1, true
		}
	}
	return 0, false
}

func run(args []string) int {
	if len(args) == 0 {
		usage(os.Stderr)
		return 2
	}
	// ONE bootstrap for every command that evaluates, before dispatch — the
	// shape let-go and Glojure both use, and the reason neither can have a
	// REPL that forgets what its run path does. let-go builds one NSResolver
	// and installs it with rt.SetNSLoader before any mode branch
	// (lg.go:440-442); Glojure drains GLJ_CLASSPATH into one global load path
	// as the first statement of gljmain.Main, ahead of REPL / -e / --nrepl /
	// file (gljmain.go:58-62).
	//
	// cljgo used to resolve at each call site, so each new entry point could
	// forget independently — and two of eight did (#185). Hoisting it here
	// removes four scattered calls in favour of one, which is fewer moving
	// parts, not more: this change would be worth making even if it fixed
	// nothing.
	if code, stop := resolveProjectForCommand(args); stop {
		return code
	}
	switch args[0] {
	case "repl":
		return runREPL(args[1:])
	case "nrepl":
		return runNREPL(args[1:])
	case "run":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: cljgo run <file.clj> [args...]")
			return 2
		}
		return runFile(args[1], args[2:])
	case "build":
		return runBuild(args[1:])
	case "dist":
		return runDist(args[1:])
	case "publish":
		return runPublish(args[1:])
	case "cache":
		return runCache(args[1:])
	case "new":
		return runNew(args[1:])
	case "generate", "g":
		return runGenerate(args[1:])
	case "migrate":
		return runMigrate(args[1:])
	case "dev":
		return runDev(args[1:])
	case "test":
		return runTest(args[1:])
	case "config":
		return runConfig(args[1:])
	case "routes":
		return runRoutes(args[1:])
	case "suite":
		return runSuite(args[1:])
	case "check":
		return runCheck(args[1:], os.Stdout, os.Stderr)
	case "explain":
		return runExplain(args[1:], os.Stdout, os.Stderr)
	// Clojure's CLI splits these two by STREAM, not content: `clojure
	// --version` prints to stdout, `clojure -version` prints to stderr
	// (verified against the real 1.12.5 CLI). Mirrored here so muscle memory
	// and scripts carry over. `cljgo version` is our own subcommand form.
	case "version", "--version", "-v":
		fmt.Fprintln(os.Stdout, versionLine())
		return 0
	case "-version":
		fmt.Fprintln(os.Stderr, versionLine())
		return 0
	case "help", "--help", "-h":
		usage(os.Stdout)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "cljgo: unknown command %q\n", args[0])
		usage(os.Stderr)
		return 2
	}
}

// replAction is the parsed intent of `cljgo repl`'s arguments: resume a
// session (resumeID set), just list the saved sessions (list), or start a
// plain REPL (both zero). Pure so it is unit-testable without a stdin loop.
type replAction struct {
	resumeID string // a session id or listing index to resume on boot
	list     bool   // print the sessions table and exit
}

// parseReplArgs maps `cljgo repl [args]` to an action. exit >= 0 means print
// the usage error and return that code; exit == -1 means proceed.
func parseReplArgs(args []string) (act replAction, exit int) {
	switch {
	case len(args) == 0:
		return replAction{}, -1
	case len(args) == 1 && (args[0] == ":resume" || args[0] == ":sessions"):
		return replAction{list: true}, -1
	case len(args) == 2 && args[0] == ":resume":
		return replAction{resumeID: args[1]}, -1
	case len(args) == 1 && args[0] != ":resume":
		return replAction{resumeID: args[0]}, -1
	default:
		return replAction{}, 2
	}
}

func runREPL(args []string) int {
	// `cljgo repl :resume <id-or-#>` (or a bare `cljgo repl <id-or-#>`)
	// resumes a saved session on boot; `:resume` / `:sessions` with no id
	// lists the saved sessions and exits. See parseReplArgs.
	act, exit := parseReplArgs(args)
	if exit >= 0 {
		fmt.Fprintln(os.Stderr, "usage: cljgo repl [:resume [<#-or-id>]]")
		return exit
	}
	// Resolve the project the SAME way `cljgo run` does (main.go's run path
	// calls this too). Without it the REPL booted with no project source
	// roots and no dependency roots, so `(require 'myproj.core)` failed in a
	// project whose own `cljgo run` and `cljgo build` resolve it fine —
	// including in a freshly generated `cljgo new` project, which could not
	// require its own namespace in its own REPL. That is a REPL-vs-run
	// divergence, the class ADR 0007 calls unforgivable (issue #185).
	//
	// Reported as an error and CONTINUED, where `run` exits 1. The reason is
	// not laxity: for `run`, unresolved deps mean the program cannot execute,
	// so there is nothing to proceed to. A REPL still has a usable prompt,
	// and the diagnostic is on screen where the user can act on it — refusing
	// to start would take away the one tool for investigating the problem it
	// is reporting. Resolution behavior itself is identical; only what
	// happens after a failure differs, because the two commands have
	// different things left to do.
	d := repl.New(os.Stdin, os.Stdout, os.Stderr)
	d.Prompts = isTerminal(os.Stdin)
	d.Interactive = d.Prompts
	if act.list {
		d.ListSessions()
		return 0
	}
	d.ResumeID = act.resumeID
	if d.Prompts {
		fmt.Println(banner())
	}
	if err := d.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

func runFile(path string, cliArgs []string) int {
	// *command-line-args*: everything after the file, or nil when none —
	// the clojure.main contract (fundamentals batch A1). Bound as the root
	// so the whole evaluation (and any goroutines) sees it; the emitted
	// func main() does the same from os.Args[1:] for REPL-vs-binary parity.
	if len(cliArgs) > 0 {
		vals := make([]any, len(cliArgs))
		for i, a := range cliArgs {
			vals[i] = a
		}
		lang.VarCommandLineArgs.BindRoot(lang.NewList(vals...))
	}
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer f.Close()
	// ADR 0052: if the project is locked (build.lock.edn present), resolve its
	// dependencies and publish their roots before evaluating, so a `cljgo run`
	// of a project with deps resolves them the same way `cljgo build` does.
	d := repl.New(nil, os.Stdout, os.Stderr)
	if _, err := d.EvalReader(f, path); err != nil {
		// Same renderer as the REPL (spike s28): named + located + expected/
		// found detail and did-you-mean, so `cljgo run` reads identically.
		fmt.Fprintf(os.Stderr, "error: %s\n", d.RenderError(err))
		return 1
	}
	// A file that ran clojure.test assertions and had failures exits non-zero,
	// exactly as the binary compiled from that same file now does (ADR 0105
	// task 2.1). Without this the interpreted and compiled legs would disagree
	// on the exit code — the divergence bar applies to exit status too.
	if rt.TestsFailed() {
		return 1
	}
	return 0
}

// resolveRunDeps wires dependency resolution into the `cljgo run` bootstrap
// (ADR 0052 decision 2). It looks for a project build file next to the source
// file, then in the current directory; if that project is already locked
// (build.lock.edn present), it resolves the declared deps and publishes their
// roots for the interpreter load path. No lock means nothing has been resolved
// yet — `cljgo build` creates the lock — so run stays a no-op there.
func resolveRunDeps(file string) error {
	// Dedupe: filepath.Dir("") and filepath.Dir("rel.clj") are BOTH ".", so
	// the two-entry list resolved the same directory twice — the whole
	// pipeline, including booting an interpreter to evaluate build.cljgo.
	// Measured at 74 ms of pure waste on the layout `cljgo new` emits
	// (spike s78: `cljgo run tiny.clj` 196.9 ms vs an absolute path 123.0 ms),
	// which is larger than the entire hoist costs.
	dirs := []string{filepath.Dir(file)}
	if dirs[0] != "." {
		dirs = append(dirs, ".")
	}
	for _, dir := range dirs {
		buildFile := build.FindBuildFile(dir)
		if buildFile == "" {
			// No cljgo build file — but a dual-host project may still declare
			// its source roots in Clojure's own deps.edn (ADR 0119). Honour
			// `:paths` and nothing else. `.cljc` is THE dual-host mechanism,
			// so these projects are not a corner case; they are the ones cljgo
			// most needs to work with, and they have no reason to carry a
			// second project file.
			if roots := deps.DepsEDNPaths(dir); len(roots) > 0 {
				addSourceRoots(roots)
				return nil
			}
			continue
		}
		// The project's SOURCE ROOTS, always — this is not conditional on
		// there being dependencies. A project with no deps has no
		// build.lock.edn, and the old early-`continue` below meant it got no
		// roots at all: `test/app/core_test.cljg` requiring `app.core` looked
		// only under test/ and failed, naming the namespace rather than the
		// paths tried. See build.DefaultSourceRoots.
		// LoadPlan ONCE. It used to run here and again in the no-lock
		// branch below, and each run boots a fresh tree-walking interpreter
		// to read the build file — 39.1 ms and 54 MB per boot, of which
		// evaluating the user's actual build form is under 2 ms (s72). Two
		// boots for a project with deps, and the dep-free default template
		// paid four. Deleting the duplicate is simplification, not
		// optimisation: the second plan was thrown away.
		plan, planErr := build.LoadPlan(buildFile)
		if planErr != nil {
			return planErr
		}
		if err := addProjectSourceRootsFromPlan(buildFile, plan); err != nil {
			return err
		}
		lockPath := filepath.Join(filepath.Dir(buildFile), "build.lock.edn")
		if _, err := os.Stat(lockPath); err != nil {
			// No lock: a project with no declared dependencies is fine as-is
			// (nothing was ever meant to be resolved). A project that DOES
			// declare dependencies but was never built is the fresh-clone
			// trap (#168) — the require that follows fails on a namespace
			// that plainly exists in `deps`, naming the wrong problem. Name
			// the real one here instead.
			if len(plan.Deps) > 0 {
				return build.ErrNoLock(buildFile, len(plan.Deps))
			}
			continue
		}
		return build.ResolveProjectDeps(buildFile)
	}
	return nil
}

// addProjectSourceRoots appends the project's source roots to the resolved
// roots. APPEND, never replace: dependency roots that a previous bootstrap
// installed stay, and the requiring file's own directory still outranks all
// of them (eval.ResolveLibPath). Duplicates are skipped so repeated calls in
// one process cannot grow the list without bound.
func addProjectSourceRootsFromPlan(buildFile string, plan *build.Plan) error {
	addSourceRoots(plan.SourceRoots(filepath.Dir(buildFile)))
	return nil
}

// addSourceRoots appends roots to the resolved set. APPEND, never replace:
// dependency roots installed by an earlier bootstrap stay, and the requiring
// file's own directory still outranks all of them (eval.ResolveLibPath).
// Duplicates are skipped so repeated calls in one process cannot grow the
// list without bound. Shared by the build.cljgo and deps.edn paths so the two
// cannot drift in how they publish.
func addSourceRoots(add []string) {
	roots := deps.ResolvedRoots()
	have := make(map[string]bool, len(roots))
	for _, r := range roots {
		have[r] = true
	}
	for _, r := range add {
		if !have[r] {
			roots = append(roots, r)
			have[r] = true
		}
	}
	deps.SetResolvedRoots(roots)
}

// runCache implements `cljgo cache <subcommand>` (ADR 0052 decision 1). The
// global dependency cache holds immutable 0555 source trees, so a plain
// `rm -rf` cannot remove them cleanly — `cljgo cache clean` is required.
func runCache(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: cljgo cache clean")
		return 2
	}
	switch args[0] {
	case "clean":
		if err := deps.CacheClean(); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		fmt.Fprintln(os.Stderr, "cljgo cache: cleaned")
		return 0
	case "help", "--help", "-h":
		fmt.Fprintln(os.Stdout, "usage: cljgo cache clean   remove the global dependency cache ($CLJGO_CACHE / $XDG_CACHE_HOME/cljgo / ~/.cache/cljgo)")
		return 0
	default:
		fmt.Fprintf(os.Stderr, "cljgo cache: unknown subcommand %q\nusage: cljgo cache clean\n", args[0])
		return 2
	}
}

// runBuild fronts two modes (ADR 0021):
//   - single-file fast path: `cljgo build <file.clj> [-o out] [-gen dir]
//     [-runtime dir]` (ADR 0001), unchanged.
//   - project path: `cljgo build [step]` with NO source file loads
//     ./build.cljgo, evaluates its (build b) fn, and runs the requested
//     step (default: install). `cljgo build run` mirrors `zig build run`.
//
// The two are told apart by the positional arg: a `.clj`/`.cljg` file →
// single-file; a bare word (or nothing) → project mode.
func runBuild(args []string) int {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	out := fs.String("o", "", "output binary path (default: derived from the source file)")
	gen := fs.String("gen", "", "directory for the generated Go module (single-file: keep it here; project: any value keeps the temp dirs)")
	runtimeDir := fs.String("runtime", "", "cljgo source tree for the generated go.mod replace (default: $CLJGO_SRC; release binaries pin the published module, dev binaries auto-detect the repo)")
	locked := fs.Bool("locked", false, "frozen: a build.lock.edn that does not match build.cljgo is an ERROR, not a refresh, and the lock is never rewritten. For CI and merges — an ordinary build re-pins what moved. Not the same as offline: you can be online and still want the lock to be the authority (ADR 0112). Also settable with CLJGO_LOCKED=1")
	sealCore := fs.Bool("seal-core", false, "hard-inline core arithmetic: a with-redefs/def/alter-var-root of + - * / < > = <= >= is then NOT seen at those call sites (JVM :inline semantics). Measured gain over the default guard: ~0-2% — opt in only if you want the JVM's inlining semantics (ADR 0108)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cljgo build [-o out] [-gen dir] [-runtime dir] [--locked] [--seal-core] [<file.clj> | <step>]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) >= 1 && len(rest) > 1 { // flags after the positional: re-parse the tail
		if err := fs.Parse(rest[1:]); err != nil {
			return 2
		}
		rest = rest[:1]
	}

	// Project mode: no positional, or a positional that is not a source file
	// (a step name like `run`). Anything ending .clj/.cljg is the single-file
	// fast path.
	if len(rest) == 0 || !isSourceFile(rest[0]) {
		step := ""
		if len(rest) == 1 {
			step = rest[0]
		}
		return runProjectBuild(step, *runtimeDir, *gen != "", *sealCore, *locked || os.Getenv("CLJGO_LOCKED") == "1")
	}

	if len(rest) != 1 {
		fs.Usage()
		return 2
	}
	src := rest[0]

	outPath := *out
	if outPath == "" {
		outPath = defaultBinaryName(src)
	}
	genDir, err := emit.Build(src, outPath, *gen, emit.Options{RuntimeDir: *runtimeDir, SealCore: *sealCore})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", buildErrText(err))
		return 1
	}
	if *gen == "" && genDir != "" {
		os.RemoveAll(genDir)
	}
	return 0
}

// runProjectBuild loads the project build file (build.cljgo/.cljg/.clj — ADR
// 0051, most-specific-first), evaluates its build fn, and runs the requested
// step (empty → default). keepGen preserves the generated modules.
func runProjectBuild(step, runtimeDir string, keepGen, sealCore, locked bool) int {
	buildFile := build.FindBuildFile(".")
	if buildFile == "" {
		fmt.Fprintf(os.Stderr, "cljgo build: no %s in the current directory\n", build.BuildFileName)
		return 1
	}
	plan, err := build.LoadPlan(buildFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	plan.Frozen = locked
	if err := plan.Run(step, emit.Options{RuntimeDir: runtimeDir, SealCore: sealCore}, keepGen); err != nil {
		fmt.Fprintln(os.Stderr, "error:", buildErrText(err))
		return 1
	}
	return 0
}

// buildErrText renders a build failure the way `cljgo run` renders the same
// failure. A build error that came from the user's Clojure source
// (emit.CompileError) goes through the ONE shared renderer — so the named
// fn, the expected-vs-found arity, the source locus and the `help:` explain
// pointer all survive the build phase instead of being flattened to the bare
// message (docs/known-issues-2026-07-28.md §8). Infrastructure failures (a
// missing file, a `go build` link error) keep their plain text, exactly as
// `cljgo run` prints its os.Open failure plainly.
func buildErrText(err error) string {
	var ce *emit.CompileError
	if errors.As(err, &ce) {
		return diag.RenderError(err)
	}
	return err.Error()
}

// isSourceFile reports whether arg names a cljgo source file (the
// single-file build path) rather than a build step name.
func isSourceFile(arg string) bool {
	switch filepath.Ext(arg) {
	case ".clj", ".cljc", ".cljg", ".cljgo":
		return true
	}
	return false
}

// defaultBinaryName derives the output name: the parent directory for a
// core.clj (examples/hello/core.clj → hello), else the file's base name.
//
// The name WE choose carries emit.ExeSuffix, so `cljgo build hello.clj` on
// Windows produces hello.exe rather than a file the OS refuses to run. An
// explicit -o is honored verbatim — same rule as `go build -o`.
func defaultBinaryName(src string) string {
	base := filepath.Base(src)
	if isSourceFile(base) {
		base = strings.TrimSuffix(base, filepath.Ext(base))
	}
	if base == "core" {
		if dir := filepath.Base(filepath.Dir(src)); dir != "." && dir != string(filepath.Separator) {
			return dir + emit.ExeSuffix
		}
	}
	return base + emit.ExeSuffix
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func usage(w io.Writer) {
	fmt.Fprintf(w, `%s

usage:
  cljgo repl                       start a REPL
  cljgo nrepl [--port N]           start an nREPL server for editors (writes .nrepl-port; ADR 0031)
  cljgo run <file.clj>             evaluate a file
  cljgo build [-o out] <file.clj>  compile a file to a native binary
  cljgo dist [--target os/arch,...] [--all] [<file.clj>]  cross-compile to every platform + checksums (ADR 0077)
  cljgo publish <go|clojars>       publish the project library to Go or Clojars (ADR 0054)
  cljgo cache clean                remove the global dependency cache (ADR 0052)
  cljgo new [--template T] <name>  generate a project: T = lib (default) | cli | web | <path>
  cljgo generate resource <Name> <field:type>...  scaffold a CRUD resource into a bri app (ADR 0073)
  cljgo migrate [up|status|new <name>]  apply/inspect/create DB migrations (ADR 0072)
  cljgo dev                        run a bri app: server + nREPL + dev warnings
  cljgo test [--compiled|--both]   run the app's tests (test/ via clojure.test); --both diffs interpreted vs AOT
  cljgo config                     print resolved config, winning layer per key
  cljgo routes                     print routes + the effective middleware stack
  cljgo suite [--dir <path>]       run the jank clojure-test-suite, print a scoreboard (ADR 0022)
  cljgo check <file.clj> [--json]  analyze a file, report diagnostics (ADR 0015)
  cljgo explain <code> [--json]    show an error code's explain page
  cljgo version                    print the version to stdout
  cljgo --version                  print the version to stdout
  cljgo -version                   print the version to stderr
`, banner())
}
