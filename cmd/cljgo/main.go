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

// runBuild is `cljgo build <file.clj> [-o out] [-gen dir] [-runtime dir]`
// (flags accepted before or after the file). The generated module is a
// temp dir removed after a successful build unless -gen keeps it
// somewhere visible.
func runBuild(args []string) int {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	out := fs.String("o", "", "output binary path (default: derived from the source file)")
	gen := fs.String("gen", "", "directory for the generated Go module (default: a temp dir, removed on success)")
	runtimeDir := fs.String("runtime", "", "cljgo source tree for the generated go.mod replace (default: $CLJGO_SRC or auto-detect)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cljgo build [-o out] [-gen dir] [-runtime dir] <file.clj>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) >= 1 && len(rest) > 1 { // flags after the file: re-parse the tail
		if err := fs.Parse(rest[1:]); err != nil {
			return 2
		}
		rest = rest[:1]
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
  cljgo version                    print the version
`, version)
}
