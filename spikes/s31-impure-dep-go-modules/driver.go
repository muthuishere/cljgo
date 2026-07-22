// Spike S31 driver — exercises pkg/emit.SynthGoMod with MERGED require sets,
// simulating what a `dep` verb would hand it once cljgo libraries can depend
// on cljgo libraries. Read-only against pkg/; nothing here merges.
//
//	go run ./spikes/s31-impure-dep-go-modules -case flatten
//	go run ./spikes/s31-impure-dep-go-modules -case overwrite
//	go run ./spikes/s31-impure-dep-go-modules -case conflict
//	go run ./spikes/s31-impure-dep-go-modules -case manifest
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/muthuishere/cljgo/pkg/emit"
)

func main() {
	var kase string
	flag.StringVar(&kase, "case", "flatten", "flatten|overwrite|conflict|manifest")
	flag.Parse()

	var err error
	switch kase {
	case "flatten":
		err = caseFlatten()
	case "overwrite":
		err = caseOverwrite()
	case "conflict":
		err = caseConflict()
	case "manifest":
		err = caseManifest()
	default:
		err = fmt.Errorf("unknown case %q", kase)
	}
	if err != nil {
		fmt.Println("ERROR:", err)
		os.Exit(1)
	}
}

// ---------------------------------------------------------------------------
// Case 1 — flattening. Consumer C has no go-requires of its own; simulated dep
// A carries `github.com/google/uuid`. The merged set must reach go.mod and the
// resulting module must build and run.
// ---------------------------------------------------------------------------

func caseFlatten() error {
	banner("CASE 1 — FLATTEN: consumer C (pure) + simulated dep A (impure: google/uuid)")

	dir, err := os.MkdirTemp("", "s26-flatten-*")
	if err != nil {
		return err
	}
	fmt.Println("gen dir:", dir)

	// What a `dep` verb would compute: C's own requires (none) ∪ A's requires.
	consumerReqs := []emit.GoModRequire{}
	depAReqs := []emit.GoModRequire{{Path: "github.com/google/uuid", Version: "v1.6.0"}}
	merged := mergeNaive(append(consumerReqs, depAReqs...))
	fmt.Println("merged require set:", show(merged))

	if err := emit.SynthGoMod(dir, "cljgo.gen/main", "", merged); err != nil {
		return err
	}
	dump(filepath.Join(dir, "go.mod"), "go.mod as SynthGoMod wrote it")

	// Emitted main.go standing in for what the consumer's namespace compiles to:
	// it uses the dep's Go module, which the consumer never declared itself.
	main := `package main

import (
	"fmt"

	"github.com/google/uuid"
)

func main() {
	fmt.Println("consumer C ran; uuid from dep A's go-require:", uuid.NewSHA1(uuid.NameSpaceDNS, []byte("cljgo")).String())
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(main), 0o644); err != nil {
		return err
	}
	run(dir, "go", "mod", "tidy")
	run(dir, "go", "build", "-o", "app", ".")
	run(dir, "./app")
	dump(filepath.Join(dir, "go.mod"), "go.mod after tidy")
	return nil
}

// ---------------------------------------------------------------------------
// Case 2 — the never-overwrite bug (pkg/emit/program.go:329). Build once with
// require set R1, then "the dep changed" -> build again with R2 into the SAME
// dir. Does R2 reach go.mod?
// ---------------------------------------------------------------------------

func caseOverwrite() error {
	banner("CASE 2 — NEVER-OVERWRITE: re-resolution after a dep's requires change")

	dir, err := os.MkdirTemp("", "s26-overwrite-*")
	if err != nil {
		return err
	}
	fmt.Println("gen dir:", dir)

	r1 := []emit.GoModRequire{{Path: "github.com/google/uuid", Version: "v1.6.0"}}
	fmt.Println("\n>> build #1, require set:", show(r1))
	if err := emit.SynthGoMod(dir, "cljgo.gen/main", "", r1); err != nil {
		return err
	}
	dump(filepath.Join(dir, "go.mod"), "go.mod after build #1")

	// The dependency was upgraded / a second dep was added: a DIFFERENT set.
	r2 := []emit.GoModRequire{
		{Path: "github.com/google/uuid", Version: "v1.6.0"},
		{Path: "github.com/google/go-cmp", Version: "v0.7.0"},
	}
	fmt.Println("\n>> build #2, require set:", show(r2))
	err = emit.SynthGoMod(dir, "cljgo.gen/main", "", r2)
	fmt.Println("SynthGoMod returned:", err)
	dump(filepath.Join(dir, "go.mod"), "go.mod after build #2")

	after, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
	if strings.Contains(string(after), "go-cmp") {
		fmt.Println("\nRESULT: the new require REACHED go.mod — no bug.")
	} else {
		fmt.Println("\nRESULT: the new require DID NOT reach go.mod — SILENT NO-OP CONFIRMED.")
		fmt.Println("        SynthGoMod returned nil, so pkg/build proceeds as if it had written.")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Case 3 — THE CONFLICT. Dep A and dep B go-require the SAME path at DIFFERENT
// versions. What does today's code path produce, end to end?
// ---------------------------------------------------------------------------

func caseConflict() error {
	banner("CASE 3 — CONFLICT: dep A wants go-cmp v0.6.0, dep B wants go-cmp v0.7.0")

	dir, err := os.MkdirTemp("", "s26-conflict-*")
	if err != nil {
		return err
	}
	fmt.Println("gen dir:", dir)

	// Exactly what core/build.cljg's go-require accumulation does today:
	// append, module-wide, no dedupe, no version reconciliation.
	depA := []emit.GoModRequire{{Path: "github.com/google/go-cmp", Version: "v0.6.0"}}
	depB := []emit.GoModRequire{{Path: "github.com/google/go-cmp", Version: "v0.7.0"}}
	merged := mergeNaive(append(depA, depB...))
	fmt.Println("merged (naive append, today's semantics):", show(merged))

	if err := emit.SynthGoMod(dir, "cljgo.gen/main", "", merged); err != nil {
		return err
	}
	dump(filepath.Join(dir, "go.mod"), "go.mod as SynthGoMod wrote it (NOTE the duplicate)")

	main := `package main

import (
	"fmt"

	"github.com/google/go-cmp/cmp"
)

func main() { fmt.Println("consumer ran; go-cmp linked, Diff(1,1) empty =", cmp.Diff(1, 1) == "") }
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(main), 0o644); err != nil {
		return err
	}
	fmt.Println("\n-- pkg/build then runs goGet + goModTidy + go build --")
	run(dir, "go", "mod", "tidy")
	dump(filepath.Join(dir, "go.mod"), "go.mod after `go mod tidy`")
	run(dir, "go", "build", "-o", "app", ".")
	run(dir, "./app")
	run(dir, "go", "list", "-m", "github.com/google/go-cmp")
	fmt.Println("\n-- who asked for what? the flattened module cannot say: --")
	run(dir, "go", "mod", "graph")
	return nil
}

// ---------------------------------------------------------------------------
// Case 4 — the crux. Discover a dep's go-requires WITHOUT evaluating its
// build.cljgo. Prototype: every published cljgo lib carries a generated
// `cljgo-manifest.edn` next to its source; resolve reads manifests only.
// ---------------------------------------------------------------------------

func caseManifest() error {
	banner("CASE 4 — TRANSITIVE DISCOVERY WITHOUT RUNNING DEP BUILD CODE")

	root, err := os.MkdirTemp("", "s26-manifest-*")
	if err != nil {
		return err
	}
	fmt.Println("fixture root:", root)

	// S31 does NOT invent a manifest format. Spike S33 already prototyped the
	// lockfile (`build.lock.edn`) with a per-dep `:impure {:go-require [...]}`
	// block — see spikes/s33-dep-fetch-cache-lock/prototype/resolve.go:148,181.
	// That block IS the answer to decision 5's "where do a dep's go-requires
	// come from": the lock, written at resolve time by the CONSUMER's fetch,
	// from the dep's declarative surface. Nothing here runs a dep's build fn.
	writeLock(root, `{:lock/version 1
 :build/hash "…"
 :deps [{:name "libhttp"
         :git/sha "a1b2c3"
         :paths ["src"]
         :impure {:go-require [{:path "github.com/google/go-cmp" :version "v0.6.0"}]}}
        {:name "libuuid"
         :git/sha "d4e5f6"
         :paths ["src"]
         :impure {:go-require [{:path "github.com/google/uuid" :version "v1.6.0"}
                               {:path "github.com/google/go-cmp" :version "v0.7.0"}]}}]}`)

	fmt.Println("\n-- resolve: read build.lock.edn (S33 schema); never evaluate a build fn --")
	all, err := readLock(filepath.Join(root, "build.lock.edn"))
	if err != nil {
		return err
	}
	for _, r := range all {
		fmt.Printf("   %-8s -> %s %s\n", r.From, r.Path, r.Version)
	}

	fmt.Println("\n-- policy A: naive append (today) --")
	fmt.Println("  ", show(mergeNaive(strip(all))))
	fmt.Println("   => duplicate path in go.mod; `go mod tidy` silently picks the higher.")

	fmt.Println("\n-- policy B: MVS in cljgo (pick higher, record provenance) --")
	mvs, notes := mergeMVS(all)
	fmt.Println("  ", show(mvs))
	for _, n := range notes {
		fmt.Println("   note:", n)
	}

	fmt.Println("\n-- policy C: hard error on any disagreement --")
	if _, err := mergeStrict(all); err != nil {
		fmt.Println("   cljgo build would fail with:")
		fmt.Println("  ", strings.ReplaceAll(err.Error(), "\n", "\n   "))
	}
	return nil
}

// --- merge policies --------------------------------------------------------

type provReq struct {
	emit.GoModRequire
	From string
}

func strip(in []provReq) []emit.GoModRequire {
	out := make([]emit.GoModRequire, len(in))
	for i, r := range in {
		out[i] = r.GoModRequire
	}
	return out
}

// mergeNaive is today's behavior: plain concatenation, duplicates and all.
func mergeNaive(in []emit.GoModRequire) []emit.GoModRequire { return in }

// mergeMVS keeps the highest version per path, Go-style, and reports what it
// overrode.
func mergeMVS(in []provReq) ([]emit.GoModRequire, []string) {
	best := map[string]provReq{}
	var notes []string
	for _, r := range in {
		cur, ok := best[r.Path]
		if !ok {
			best[r.Path] = r
			continue
		}
		lo, hi := cur, r
		if semverLess(r.Version, cur.Version) {
			lo, hi = r, cur
		}
		best[r.Path] = hi
		notes = append(notes, fmt.Sprintf("%s: %s (from %s) upgraded over %s (from %s)",
			hi.Path, hi.Version, hi.From, lo.Version, lo.From))
	}
	var out []emit.GoModRequire
	for _, r := range best {
		out = append(out, r.GoModRequire)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, notes
}

// mergeStrict fails on any path pinned at two versions.
func mergeStrict(in []provReq) ([]emit.GoModRequire, error) {
	seen := map[string]provReq{}
	for _, r := range in {
		if cur, ok := seen[r.Path]; ok && cur.Version != r.Version {
			return nil, fmt.Errorf(
				"conflicting go-require for %s\n  %s pins %s\n  %s pins %s\nresolve it in build.cljgo with an explicit (go-require ...) override",
				r.Path, cur.From, cur.Version, r.From, r.Version)
		}
		seen[r.Path] = r
	}
	return strip(in), nil
}

// semverLess is a crude vX.Y.Z comparison — enough for the spike.
func semverLess(a, b string) bool {
	pa, pb := parts(a), parts(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return false
}

func parts(v string) [3]int {
	var out [3]int
	f := strings.SplitN(strings.TrimPrefix(v, "v"), ".", 3)
	for i := 0; i < len(f) && i < 3; i++ {
		n := 0
		fmt.Sscanf(f[i], "%d", &n)
		out[i] = n
	}
	return out
}

// --- manifest fixture ------------------------------------------------------

func writeLock(root, body string) {
	os.MkdirAll(root, 0o755)
	os.WriteFile(filepath.Join(root, "build.lock.edn"), []byte(body+"\n"), 0o644)
}

// readLock extracts each dep's :impure :go-require entries by scanning —
// deliberately NOT by evaluating anything. A real implementation reads it with
// pkg/reader (pure data, no eval), exactly as S33's prototype does. The point
// is the shape and the provenance, not the parser.
func readLock(path string) ([]provReq, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []provReq
	// Split per dep entry so each require keeps the name of the dep that asked
	// for it — provenance the flattened go.mod cannot reconstruct.
	entries := strings.Split(string(b), ":name")
	for _, e := range entries[1:] {
		name := between(e, `"`, `"`)
		for _, chunk := range strings.Split(e, "{:path")[1:] {
			if !strings.Contains(chunk, ":version") {
				continue
			}
			p := between(chunk, `"`, `"`)
			v := between(chunk[strings.Index(chunk, ":version"):], `"`, `"`)
			if p != "" && v != "" {
				out = append(out, provReq{emit.GoModRequire{Path: p, Version: v}, name})
			}
		}
	}
	return out, nil
}

func between(s, a, b string) string {
	i := strings.Index(s, a)
	if i < 0 {
		return ""
	}
	s = s[i+len(a):]
	j := strings.Index(s, b)
	if j < 0 {
		return ""
	}
	return s[:j]
}

// --- helpers ---------------------------------------------------------------

func banner(s string) { fmt.Printf("\n=== %s ===\n\n", s) }

func show(rs []emit.GoModRequire) string {
	var b []string
	for _, r := range rs {
		b = append(b, r.Path+"@"+r.Version)
	}
	if len(b) == 0 {
		return "(empty)"
	}
	return strings.Join(b, ", ")
}

func dump(path, label string) {
	b, err := os.ReadFile(path)
	fmt.Printf("\n--- %s ---\n", label)
	if err != nil {
		fmt.Println("(missing:", err, ")")
		return
	}
	fmt.Print(string(b))
	fmt.Println("---")
}

func run(dir string, args ...string) {
	fmt.Printf("\n$ %s\n", strings.Join(args, " "))
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		fmt.Print(string(out))
	}
	fmt.Println("exit err:", err)
}
