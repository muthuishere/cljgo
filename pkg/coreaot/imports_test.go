package coreaot_test

import (
	"os/exec"
	"strings"
	"testing"
)

// TestNoInterpreterInCompiledBinary is AOT-core piece 3's exit proof
// (ADR 0046, executing ADR 0037 / ADR 0023 §2): the two packages an
// emitted binary bootstraps through — pkg/coreaot (the compiled core)
// and pkg/emit/rt (the bootstrap + intrinsics) — must be linkable with
// NO interpreter in their dependency closure. If either grows an edge
// back to pkg/eval, the Go linker keeps the whole tree-walker (plus the
// analyzer and the AST) in every compiled binary and the cutover is
// silently undone.
//
// It is deliberately all-or-nothing: ONE package reaching for pkg/eval
// is enough to relink everything, so this fails on the first edge rather
// than on a symbol-count threshold. Measured on the same hello-world at
// the time of the cutover (unstripped, `go tool nm | grep -c`):
// pkg/eval 155 → 0 symbols, pkg/analyzer 63 → 0, pkg/ast 14 → 0.
func TestNoInterpreterInCompiledBinary(t *testing.T) {
	forbidden := []string{
		"github.com/muthuishere/cljgo/pkg/eval",
		"github.com/muthuishere/cljgo/pkg/analyzer",
		"github.com/muthuishere/cljgo/pkg/ast",
		"github.com/muthuishere/cljgo/pkg/emit", // the emitter itself: build-time only
		"github.com/muthuishere/cljgo/pkg/repl",
	}
	for _, pkg := range []string{
		"github.com/muthuishere/cljgo/pkg/coreaot",
		"github.com/muthuishere/cljgo/pkg/emit/rt",
		// ADR 0071: the AOT-compiled bri framework a bri binary blank-imports
		// must stay interpreter-free for the same reason coreaot must — an
		// edge back to pkg/eval relinks the whole tree-walker into every bri
		// binary and undoes the cutover. pkg/bri (the shims) and pkg/briaot
		// (the compiled namespaces + providers) are on this path; pkg/briloader
		// (the interpreter loader) is deliberately NOT — it is the REPL half.
		"github.com/muthuishere/cljgo/pkg/briaot",
		"github.com/muthuishere/cljgo/pkg/bri",
	} {
		out, err := exec.Command("go", "list", "-deps", pkg).CombinedOutput()
		if err != nil {
			t.Fatalf("go list -deps %s failed: %v\n%s", pkg, err, out)
		}
		for _, dep := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			for _, f := range forbidden {
				// pkg/emit/rt is under pkg/emit: the prefix rule would
				// flag it against itself.
				if dep == "github.com/muthuishere/cljgo/pkg/emit/rt" {
					continue
				}
				if dep == f || strings.HasPrefix(dep, f+"/") {
					t.Errorf("%s links %s — an AOT-compiled binary would carry the interpreter again (ADR 0046)", pkg, dep)
				}
			}
		}
	}
}
