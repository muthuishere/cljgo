package corelib_test

import (
	"os/exec"
	"strings"
	"testing"
)

// TestNoInterpreterInDependencyClosure is AOT-core piece 2's exit proof
// (ADR 0043): pkg/corelib must be linkable WITHOUT the tree-walk
// interpreter, so a compiled core.clj (piece 3) can register the Go
// builtins while emitted binaries drop pkg/eval entirely (ADR 0023 §2).
// The check runs on the non-test package — this external test file may
// import pkg/eval-adjacent helpers freely without weakening it.
func TestNoInterpreterInDependencyClosure(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps",
		"github.com/muthuishere/cljgo/pkg/corelib").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps failed: %v\n%s", err, out)
	}
	forbidden := []string{
		"github.com/muthuishere/cljgo/pkg/eval",
		"github.com/muthuishere/cljgo/pkg/analyzer",
		"github.com/muthuishere/cljgo/pkg/ast",
		"github.com/muthuishere/cljgo/pkg/emit",
	}
	for _, dep := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		for _, f := range forbidden {
			if dep == f || strings.HasPrefix(dep, f+"/") {
				t.Errorf("pkg/corelib dependency closure contains %s — the interpreter leaked back in", dep)
			}
		}
	}
}
