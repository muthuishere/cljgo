package main

// S33 prototype resolver. Stands in for `cljgo resolve` / the resolution
// phase of `cljgo build`. Throwaway (ADR 0027 §5).
//
//	s28 resolve  -project DIR [-update] [-offline] [-vendor DIR]
//	s28 treehash DIR
//	s28 loadpath -project DIR

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: s28 <resolve|treehash|loadpath> ...")
		os.Exit(2)
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	project := fs.String("project", ".", "project dir containing build.cljgo")
	update := fs.Bool("update", false, "re-resolve refs against remotes and rewrite the lock")
	offline := fs.Bool("offline", false, "never contact a remote; lock + cache only")
	vendor := fs.String("vendor", "", "project-local vendor dir (overrides the cache)")
	quiet := fs.Bool("quiet", false, "suppress the trace")
	only := fs.String("name", "", "entry: which dep")

	switch cmd {
	case "treehash":
		if len(os.Args) < 3 {
			die(fmt.Errorf("treehash needs a directory"))
		}
		h, err := TreeHash(os.Args[2])
		die(err)
		fmt.Println(h)
		return
	case "resolve", "loadpath", "entry":
		die(fs.Parse(os.Args[2:]))
	default:
		die(fmt.Errorf("unknown command %q", cmd))
	}

	proj, err := filepath.Abs(*project)
	die(err)
	vdir := *vendor
	if vdir == "" {
		if d := filepath.Join(proj, "vendor"); dirExists(d) {
			vdir = d
		}
	}
	r := &Resolver{Root: CacheRoot(), Project: proj, Offline: *offline, Update: *update, Vendor: vdir}
	die(r.LoadLock())

	buildFile := filepath.Join(proj, "build.cljgo")
	roots, err := ScanBuildFile(buildFile)
	die(err)
	buildHash, err := HashFile(buildFile)
	die(err)

	deps, err := r.Resolve(roots)
	if err != nil {
		if !*quiet {
			printTrace(r)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if cmd == "entry" {
		for _, d := range deps {
			if d.Name == *only {
				fmt.Println(r.baseDir(d))
				return
			}
		}
		die(fmt.Errorf("no dep named %q", *only))
	}

	if cmd == "loadpath" {
		for _, p := range r.LoadPathRoots(deps) {
			fmt.Println(p)
		}
		return
	}

	if !*quiet {
		printTrace(r)
	}
	if *update || len(r.locked) == 0 {
		die(r.WriteLock(deps, buildHash))
		fmt.Printf("wrote %s (%d deps)\n", r.lockPath(), len(deps))
	} else {
		fmt.Printf("ok: %d deps resolved from build.lock.edn\n", len(deps))
	}
	fmt.Printf("cache: %s\n", r.Root)
}

func printTrace(r *Resolver) {
	for _, l := range r.Trace {
		fmt.Fprintln(os.Stderr, "  "+l)
	}
}

func die(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
