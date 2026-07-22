package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/muthuishere/cljgo/pkg/build"
	"github.com/muthuishere/cljgo/pkg/publish"
)

// runPublish implements `cljgo publish <go|clojars>` (ADR 0050). It loads the
// project build file, finds the declared library artifact ((lib b …)), and
// emits the requested target:
//
//   - go       a go-gettable Go module (Go interop allowed)
//   - clojars  pure Clojure source (refused if any namespace uses Go interop)
//
// One build.cljgo, both ecosystems (ADR 0050 decision 2): the SAME lib
// declaration publishes to either target; the target is chosen here, not in the
// build file.
func runPublish(args []string) int {
	fs := flag.NewFlagSet("publish", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	out := fs.String("o", "", "output directory (default: ./publish/<target>)")
	name := fs.String("name", "", "which library artifact to publish (default: the sole lib)")
	module := fs.String("module", "", "override the library module path / coordinate")
	runtimeDir := fs.String("runtime", "", "cljgo source tree for the generated go.mod replace (publish go)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cljgo publish <go|clojars> [-o dir] [-name lib] [-module path] [-runtime dir]")
		fs.PrintDefaults()
	}
	if len(args) == 0 {
		fs.Usage()
		return 2
	}
	switch args[0] {
	case "help", "--help", "-h":
		fs.Usage()
		return 0
	}
	target := args[0]
	if target != "go" && target != "clojars" {
		fmt.Fprintf(os.Stderr, "cljgo publish: unknown target %q (want: go | clojars)\n", target)
		fs.Usage()
		return 2
	}
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}

	buildFile := build.FindBuildFile(".")
	if buildFile == "" {
		fmt.Fprintf(os.Stderr, "cljgo publish: no %s in the current directory\n", build.BuildFileName)
		return 1
	}
	plan, err := build.LoadPlan(buildFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	art, err := plan.LibArtifact(*name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	mainPath := art.Main
	if !filepath.IsAbs(mainPath) {
		mainPath = filepath.Join(plan.ProjectDir, mainPath)
	}
	outDir := *out
	if outDir == "" {
		outDir = filepath.Join(plan.ProjectDir, "publish", target)
	}
	mod := art.Module
	if *module != "" {
		mod = *module
	}

	opts := []publish.Opt{publish.WithModule(mod), publish.WithRuntimeDir(*runtimeDir)}

	switch target {
	case "clojars":
		if err := publish.PublishClojars(mainPath, outDir, opts...); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "cljgo publish clojars: wrote pure Clojure source tree to %s\n", outDir)
		return 0
	case "go":
		res, err := publish.PublishGo(mainPath, outDir, opts...)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "cljgo publish go: wrote go-gettable module %s (ns %s, %d exported) to %s\n",
			res.Module, res.EntryNS, len(res.Exports), outDir)
		return 0
	}
	return 0
}
