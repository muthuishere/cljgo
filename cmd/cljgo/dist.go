package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/muthuishere/cljgo/pkg/build"
	"github.com/muthuishere/cljgo/pkg/emit"
)

// target is one GOOS/GOARCH cross-compilation pair.
type target struct{ os, arch string }

func (t target) String() string { return t.os + "/" + t.arch }

// slug is the artifact-name suffix for this target: "darwin-arm64".
func (t target) slug() string { return t.os + "-" + t.arch }

// defaultMatrix is the five mainstream desktop/server targets `cljgo dist`
// builds with no flags (ADR 0077): Apple Silicon + Intel Mac, x86-64 + ARM
// Linux, and Windows — effectively every real end user.
var defaultMatrix = []target{
	{"darwin", "arm64"}, {"darwin", "amd64"},
	{"linux", "amd64"}, {"linux", "arm64"},
	{"windows", "amd64"},
}

// runDist cross-compiles a cljgo program to a matrix of platforms in one
// command (ADR 0077). Input resolution mirrors `cljgo build`: a .clj/.cljc/
// .cljg positional is the single-file fast path; a bare invocation uses the
// project build.cljgo install artifact. Unlike build, dist's default is the
// matrix (host-only is already `cljgo build`).
func runDist(args []string) int {
	fs := flag.NewFlagSet("dist", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	outDir := fs.String("o", "dist", "output directory for the built binaries + checksums")
	targetList := fs.String("target", "", "comma-separated GOOS/GOARCH pairs (e.g. linux/amd64,windows/amd64); default: the mainstream matrix")
	all := fs.Bool("all", false, "build every GOOS/GOARCH `go tool dist list` reports (the long tail, opt-in)")
	runtimeDir := fs.String("runtime", "", "cljgo source tree for the generated go.mod replace (default: $CLJGO_SRC)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cljgo dist [-o dir] [--target os/arch,...] [--all] [<file.clj>]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()

	targets, err := resolveTargets(*targetList, *all)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	// Prepare the target-independent Go module ONCE (ADR 0077), then link it
	// per target. Single-file vs project mirrors `cljgo build`.
	opts := emit.Options{RuntimeDir: *runtimeDir}
	var genDir, name string
	if len(rest) == 1 && isSourceFile(rest[0]) {
		src := rest[0]
		name = strings.TrimSuffix(filepath.Base(defaultBinaryName(src)), emit.ExeSuffix)
		genDir, err = emit.PrepareModule(src, "", opts)
	} else if len(rest) == 0 {
		genDir, name, err = prepareProjectDist(opts)
	} else {
		fs.Usage()
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer os.RemoveAll(genDir)

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	// Link once per target and hash each artifact.
	type result struct {
		t    target
		path string
		size int64
		sum  string
	}
	var results []result
	for _, t := range targets {
		bin := name + "_" + t.slug()
		if t.os == "windows" {
			bin += ".exe"
		}
		out := filepath.Join(*outDir, bin)
		fmt.Fprintf(os.Stderr, "cljgo dist: building %-16s -> %s\n", t, out)
		if err := emit.GoBuildTarget(genDir, out, t.os, t.arch); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s: %v\n", t, err)
			return 1
		}
		sum, size, err := sha256File(out)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		results = append(results, result{t, out, size, sum})
	}

	// checksums.txt in `sha256sum -c` format (<hex>␣␣<basename>).
	var cs strings.Builder
	for _, r := range results {
		fmt.Fprintf(&cs, "%s  %s\n", r.sum, filepath.Base(r.path))
	}
	if err := os.WriteFile(filepath.Join(*outDir, "checksums.txt"), []byte(cs.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	// Summary table.
	fmt.Fprintf(os.Stderr, "\ncljgo dist: %d binaries in %s/\n", len(results), *outDir)
	for _, r := range results {
		fmt.Fprintf(os.Stderr, "  %-22s %6.1f MB  %s\n", r.t, float64(r.size)/(1024*1024), filepath.Base(r.path))
	}
	return 0
}

// prepareProjectDist loads build.cljgo and prepares its install artifact's
// module once (ADR 0077), returning the ready-to-link genDir + artifact name.
func prepareProjectDist(opts emit.Options) (genDir, name string, err error) {
	buildFile := build.FindBuildFile(".")
	if buildFile == "" {
		return "", "", fmt.Errorf("no %s in the current directory (and no <file.clj> given)", build.BuildFileName)
	}
	plan, err := build.LoadPlan(buildFile)
	if err != nil {
		return "", "", err
	}
	return plan.DistInstall(opts)
}

// resolveTargets turns the --target / --all flags into the target list: --all
// enumerates `go tool dist list`; an explicit --target is validated against it
// (a typo is a named error, not a cryptic go-build failure); neither yields the
// mainstream default matrix.
func resolveTargets(list string, all bool) ([]target, error) {
	if all {
		return goToolDistList()
	}
	if list == "" {
		return defaultMatrix, nil
	}
	valid, err := goToolDistList()
	if err != nil {
		return nil, err
	}
	validSet := map[string]bool{}
	for _, t := range valid {
		validSet[t.String()] = true
	}
	var out []target
	for _, tok := range strings.Split(list, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		os, arch, ok := strings.Cut(tok, "/")
		if !ok || os == "" || arch == "" {
			return nil, fmt.Errorf("bad target %q: want GOOS/GOARCH (e.g. linux/amd64)", tok)
		}
		if !validSet[tok] {
			return nil, fmt.Errorf("unsupported target %q: not in `go tool dist list`", tok)
		}
		out = append(out, target{os, arch})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("--target was empty")
	}
	return out, nil
}

// goToolDistList returns every GOOS/GOARCH pair the toolchain supports.
func goToolDistList() ([]target, error) {
	out, err := exec.Command("go", "tool", "dist", "list").Output()
	if err != nil {
		return nil, fmt.Errorf("go tool dist list: %w", err)
	}
	var ts []target
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if os, arch, ok := strings.Cut(strings.TrimSpace(line), "/"); ok {
			ts = append(ts, target{os, arch})
		}
	}
	sort.Slice(ts, func(i, j int) bool { return ts[i].String() < ts[j].String() })
	return ts, nil
}

// sha256File returns the hex sha256 and byte size of a file.
func sha256File(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), n, nil
}
