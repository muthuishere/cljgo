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
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/muthuishere/cljgo/pkg/build"
	"github.com/muthuishere/cljgo/pkg/deps"
	"github.com/muthuishere/cljgo/pkg/emit"
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

func run(args []string) int {
	if len(args) == 0 {
		usage(os.Stderr)
		return 2
	}
	switch args[0] {
	case "repl":
		return runREPL()
	case "nrepl":
		return runNREPL(args[1:])
	case "run":
		if len(args) != 2 {
			fmt.Fprintln(os.Stderr, "usage: cljgo run <file.clj>")
			return 2
		}
		return runFile(args[1])
	case "build":
		return runBuild(args[1:])
	case "publish":
		return runPublish(args[1:])
	case "cache":
		return runCache(args[1:])
	case "new":
		return runNew(args[1:])
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

func runREPL() int {
	d := repl.New(os.Stdin, os.Stdout, os.Stderr)
	d.Prompts = isTerminal(os.Stdin)
	d.Interactive = d.Prompts
	if d.Prompts {
		fmt.Println(banner())
	}
	if err := d.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

func runFile(path string) int {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer f.Close()
	// ADR 0048: if the project is locked (build.lock.edn present), resolve its
	// dependencies and publish their roots before evaluating, so a `cljgo run`
	// of a project with deps resolves them the same way `cljgo build` does.
	if err := resolveRunDeps(path); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	d := repl.New(nil, os.Stdout, os.Stderr)
	if _, err := d.EvalReader(f, path); err != nil {
		// Same renderer as the REPL (spike s28): named + located + expected/
		// found detail and did-you-mean, so `cljgo run` reads identically.
		fmt.Fprintf(os.Stderr, "error: %s\n", d.RenderError(err))
		return 1
	}
	return 0
}

// resolveRunDeps wires dependency resolution into the `cljgo run` bootstrap
// (ADR 0048 decision 2). It looks for a project build file next to the source
// file, then in the current directory; if that project is already locked
// (build.lock.edn present), it resolves the declared deps and publishes their
// roots for the interpreter load path. No lock means nothing has been resolved
// yet — `cljgo build` creates the lock — so run stays a no-op there.
func resolveRunDeps(file string) error {
	for _, dir := range []string{filepath.Dir(file), "."} {
		buildFile := build.FindBuildFile(dir)
		if buildFile == "" {
			continue
		}
		lockPath := filepath.Join(filepath.Dir(buildFile), "build.lock.edn")
		if _, err := os.Stat(lockPath); err != nil {
			continue
		}
		return build.ResolveProjectDeps(buildFile)
	}
	return nil
}

// runCache implements `cljgo cache <subcommand>` (ADR 0048 decision 1). The
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
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cljgo build [-o out] [-gen dir] [-runtime dir] [<file.clj> | <step>]")
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
		return runProjectBuild(step, *runtimeDir, *gen != "")
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
	genDir, err := emit.Build(src, outPath, *gen, emit.Options{RuntimeDir: *runtimeDir})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
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
func runProjectBuild(step, runtimeDir string, keepGen bool) int {
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
	if err := plan.Run(step, emit.Options{RuntimeDir: runtimeDir}, keepGen); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

// isSourceFile reports whether arg names a cljgo source file (the
// single-file build path) rather than a build step name.
func isSourceFile(arg string) bool {
	return strings.HasSuffix(arg, ".clj") || strings.HasSuffix(arg, ".cljg")
}

// defaultBinaryName derives the output name: the parent directory for a
// core.clj (examples/hello/core.clj → hello), else the file's base name.
//
// The name WE choose carries emit.ExeSuffix, so `cljgo build hello.clj` on
// Windows produces hello.exe rather than a file the OS refuses to run. An
// explicit -o is honored verbatim — same rule as `go build -o`.
func defaultBinaryName(src string) string {
	base := strings.TrimSuffix(filepath.Base(src), ".clj")
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
  cljgo publish <go|clojars>       publish the project library to Go or Clojars (ADR 0050)
  cljgo cache clean                remove the global dependency cache (ADR 0048)
  cljgo new [--template T] <name>  generate a project: T = lib (default) | cli | web | <path>
  cljgo dev                        run a bri app: server + nREPL + dev warnings
  cljgo test                       run the app's tests (test/ via clojure.test)
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
