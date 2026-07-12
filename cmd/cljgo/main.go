// Command cljgo is the CLI entry point (design/00 §3):
//
//	cljgo repl            start a terminal REPL on stdin/stdout
//	cljgo run <file.clj>  read + evaluate a file
//	cljgo version         print the version string
//
// Both subcommands front the pkg/repl driver — one Read→Analyze→Eval
// path, per design/03 §7d.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/muthuishere/cljgo/pkg/repl"
)

const version = "cljgo 0.0.1-m0"

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

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func usage(w io.Writer) {
	fmt.Fprintf(w, `%s

usage:
  cljgo repl            start a REPL
  cljgo run <file.clj>  evaluate a file
  cljgo version         print the version
`, version)
}
