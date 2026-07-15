package build

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/muthuishere/cljgo/pkg/emit"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// examplesDir is the repo's examples/ tree, relative to this test's package
// dir (pkg/build). FindRuntimeDir (used by the emitter for the go.mod
// replace) walks up from cwd and finds the repo root, so no CLJGO_SRC needed.
func examplesDir(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("..", "..", "examples"))
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// resetUser clears the global `user` namespace so each LoadPlan starts clean
// (namespaces are process-global, as in pkg/emit's tests).
func resetUser() { lang.RemoveNamespace(lang.NewSymbol("user")) }

// TestLoadPlanHello checks the build graph read back from build-hello's
// build.cljgo: one exe artifact, install (default) + run steps.
func TestLoadPlanHello(t *testing.T) {
	resetUser()
	bf := filepath.Join(examplesDir(t), "build-hello", BuildFileName)
	plan, err := LoadPlan(bf)
	if err != nil {
		t.Fatalf("LoadPlan: %v", err)
	}
	if len(plan.Artifacts) != 1 {
		t.Fatalf("artifacts = %v, want 1", plan.Artifacts)
	}
	a := plan.Artifacts[0]
	if a.Name != "hello" || a.Main != "src/hello.cljg" || a.Kind != "exe" {
		t.Fatalf("artifact = %+v", a)
	}
	if plan.Default != "install" {
		t.Fatalf("default = %q, want install", plan.Default)
	}
	if len(plan.Steps) != 2 || plan.Steps[0].Type != "install" || plan.Steps[1].Type != "run" {
		t.Fatalf("steps = %+v", plan.Steps)
	}
	if len(plan.GoRequires) != 0 {
		t.Fatalf("go-requires = %v, want none", plan.GoRequires)
	}
}

// TestBuildHelloBinary is the B1 exit criterion: a hello-world built via
// build.cljgo produces a working binary — same output as the single-file
// path would (design/08 §1 B1).
func TestBuildHelloBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping go-build in -short mode")
	}
	resetUser()
	bf := filepath.Join(examplesDir(t), "build-hello", BuildFileName)
	plan, err := LoadPlan(bf)
	if err != nil {
		t.Fatalf("LoadPlan: %v", err)
	}
	bin := filepath.Join(t.TempDir(), "hello")
	if _, err := plan.buildArtifact(plan.Artifacts[0], bin, emit.Options{}, false); err != nil {
		t.Fatalf("buildArtifact: %v", err)
	}
	out, err := exec.Command(bin).Output()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "hello from a build.cljgo binary\n(fact 10) = 3628800\n"
	if string(out) != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}
