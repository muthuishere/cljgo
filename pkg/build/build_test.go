package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// TestLoadPlanGoRequire checks that (go-require …) records a pinned module
// requirement on the plan (B2 graph-side, no network).
func TestLoadPlanGoRequire(t *testing.T) {
	resetUser()
	bf := filepath.Join(examplesDir(t), "build-websocket", BuildFileName)
	plan, err := LoadPlan(bf)
	if err != nil {
		t.Fatalf("LoadPlan: %v", err)
	}
	if len(plan.GoRequires) != 1 {
		t.Fatalf("go-requires = %v, want 1", plan.GoRequires)
	}
	gr := plan.GoRequires[0]
	if gr.Path != "github.com/gorilla/websocket" || gr.Version != "v1.5.3" {
		t.Fatalf("go-require = %+v", gr)
	}
}

// TestBuildWebsocketBinary is the B2 exit criterion: a program using a real
// third-party Go module (gorilla/websocket) builds from build.cljgo with
// ZERO hand-written bindings and zero hand-written go.mod. Requires network
// (go get); skipped with a clear note when the fetch is unavailable.
func TestBuildWebsocketBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping go-build in -short mode")
	}
	resetUser()
	bf := filepath.Join(examplesDir(t), "build-websocket", BuildFileName)
	plan, err := LoadPlan(bf)
	if err != nil {
		t.Fatalf("LoadPlan: %v", err)
	}
	bin := filepath.Join(t.TempDir(), "wsclient")
	if _, err := plan.buildArtifact(plan.Artifacts[0], bin, emit.Options{}, false); err != nil {
		// Distinguish an offline fetch failure (skip) from a real regression.
		if isNetworkErr(err) {
			t.Skipf("network unavailable for `go get` — go.mod synthesis + go-get "+
				"invocation are exercised by TestLoadPlanGoRequire/TestSynthGoMod; "+
				"full link skipped: %v", err)
		}
		t.Fatalf("buildArtifact: %v", err)
	}
	out, err := exec.Command(bin).Output()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got := string(out)
	// The const (1000) and the linked FormatCloseMessage call both prove the
	// real module linked — no bindings, no hand-written go.mod.
	if !strings.Contains(got, "close-normal code: 1000") ||
		!strings.Contains(got, "non-nil close frame") {
		t.Fatalf("output = %q", got)
	}
}

// TestSynthGoModOffline exercises go.mod synthesis with a third-party require
// deterministically, no network — the honest fallback the B2 caveat names.
func TestSynthGoModOffline(t *testing.T) {
	dir := t.TempDir()
	reqs := []emit.GoModRequire{{Path: "github.com/gorilla/websocket", Version: "v1.5.3"}}
	rt, err := emit.FindRuntimeDir()
	if err != nil {
		t.Fatalf("FindRuntimeDir: %v", err)
	}
	if err := emit.SynthGoMod(dir, "cljgo.gen/main", rt, reqs); err != nil {
		t.Fatalf("SynthGoMod: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{
		"module cljgo.gen/main",
		"github.com/gorilla/websocket v1.5.3",
		"replace github.com/muthuishere/cljgo => " + rt,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("go.mod missing %q:\n%s", want, got)
		}
	}
}

// isNetworkErr heuristically detects a `go get` failure caused by no network
// (offline CI), so the B2 test skips rather than fails.
func isNetworkErr(err error) bool {
	s := err.Error()
	for _, m := range []string{
		"dial tcp", "no such host", "network is unreachable",
		"timeout", "connection refused", "i/o timeout",
		"TLS handshake", "lookup ", "unrecognized import path",
	} {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}
