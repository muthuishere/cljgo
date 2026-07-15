// Command cljgo is the CLI entry point (design/00 §3):
//
//	cljgo repl              start a terminal REPL on stdin/stdout
//	cljgo run <file.clj>    read + evaluate a file
//	cljgo build <file.clj>  AOT-compile a file to a native binary (M2)
//	cljgo version           print the version string
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
	"github.com/muthuishere/cljgo/pkg/emit"
	"github.com/muthuishere/cljgo/pkg/repl"
)

const version = "cljgo 0.0.1-m2"

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
	case "run":
		if len(args) != 2 {
			fmt.Fprintln(os.Stderr, "usage: cljgo run <file.clj>")
			return 2
		}
		return runFile(args[1])
	case "build":
		return runBuild(args[1:])
	case "check":
		return runCheck(args[1:], os.Stdout, os.Stderr)
	case "explain":
		return runExplain(args[1:], os.Stdout, os.Stderr)
	case "version", "--version", "-v":
		fmt.Println(version)
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
		fmt.Println(version)
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
	d := repl.New(nil, os.Stdout, os.Stderr)
	if _, err := d.EvalReader(f, path); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
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
	runtimeDir := fs.String("runtime", "", "cljgo source tree for the generated go.mod replace (default: $CLJGO_SRC or auto-detect)")
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

// runProjectBuild loads ./build.cljgo, evaluates its build fn, and runs the
// requested step (empty → default). keepGen preserves the generated modules.
func runProjectBuild(step, runtimeDir string, keepGen bool) int {
	if _, err := os.Stat(build.BuildFileName); err != nil {
		fmt.Fprintf(os.Stderr, "cljgo build: no %s in the current directory\n", build.BuildFileName)
		return 1
	}
	plan, err := build.LoadPlan(build.BuildFileName)
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
func defaultBinaryName(src string) string {
	base := strings.TrimSuffix(filepath.Base(src), ".clj")
	if base == "core" {
		if dir := filepath.Base(filepath.Dir(src)); dir != "." && dir != string(filepath.Separator) {
			return dir
		}
	}
	return base
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func usage(w io.Writer) {
	fmt.Fprintf(w, `%s

usage:
  cljgo repl                       start a REPL
  cljgo run <file.clj>             evaluate a file
  cljgo build [-o out] <file.clj>  compile a file to a native binary
  cljgo check <file.clj> [--json]  analyze a file, report diagnostics (ADR 0015)
  cljgo explain <code> [--json]    show an error code's explain page
  cljgo version                    print the version
`, version)
}
