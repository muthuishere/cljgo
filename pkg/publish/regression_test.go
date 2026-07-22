package publish

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPublishClojarsRefusesBareRequireGo — a library whose only Go-interop is a
// bare (require-go …) with NO member access must be REFUSED from clojars.
// Regression for the purity-gate bypass: previously the classifier keyed only on
// OpHost* member nodes, so a bare require-go was mislabeled pure, copied verbatim
// into the JVM source tree (where it is not valid Clojure), and the manifest
// claimed :pure? true.
func TestPublishClojarsRefusesBareRequireGo(t *testing.T) {
	out := filepath.Join(t.TempDir(), "clj")
	entry := filepath.FromSlash("../emit/testdata/purity/barereq/barereq/core.clj")
	err := PublishClojars(entry, out, WithModule("github.com/you/bare"))
	if err == nil {
		t.Fatal("expected refusal for a bare require-go library, got nil")
	}
	if !strings.Contains(err.Error(), "require-go") && !strings.Contains(err.Error(), "Go interop") {
		t.Fatalf("refusal should name the Go-interop cause, got: %v", err)
	}
	// No partial source tree leaked on refusal.
	if _, statErr := os.Stat(filepath.Join(out, "src")); statErr == nil {
		t.Error("a src/ tree was written despite refusal — refuse-before-write violated")
	}
}

// TestPublishGoCreatesMissingOutDir — publish go for a Go-interop library must
// succeed even when the output directory does not pre-exist. Regression for the
// go/packages chdir-into-missing-dir crash that only reproduced off the test
// path (t.TempDir pre-creates the dir; the real CLI target does not).
func TestPublishGoCreatesMissingOutDir(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping go build in -short")
	}
	out := filepath.Join(t.TempDir(), "does", "not", "exist") // not pre-created
	entry := filepath.FromSlash("testdata/gointerop/gi/core.clj")
	if _, err := PublishGo(entry, out, WithModule("example.com/golib")); err != nil {
		t.Fatalf("PublishGo into a missing dir failed: %v", err)
	}
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = out
	if o, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build ./... failed: %v\n%s", err, o)
	}
}

// TestPublishGoExportNameCollidesWithLoad — a public defn named `load` munges to
// the Go identifier `Load`, which is the generated AOT loader the wrappers call.
// The emitted module must still compile. Regression for the silent duplicate-
// symbol failure where exportGoName deduped only against other exports, not
// against the package's generated identifiers.
func TestPublishGoExportNameCollidesWithLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping go build in -short")
	}
	out := filepath.Join(t.TempDir(), "ln")
	entry := filepath.FromSlash("testdata/loadname/ln/core.clj")
	if _, err := PublishGo(entry, out, WithModule("example.com/lnlib")); err != nil {
		t.Fatalf("PublishGo(load-name): %v", err)
	}
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = out
	if o, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build ./... failed (Load redeclared?): %v\n%s", err, o)
	}
}
