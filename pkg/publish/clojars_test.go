package publish

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPublishClojarsPureLibrary — a wholly pure library publishes: a src/ tree
// with every namespace's source, a deps.edn git-coordinate stub, and a pure
// cljgo.manifest.edn.
func TestPublishClojarsPureLibrary(t *testing.T) {
	out := t.TempDir()
	entry := filepath.FromSlash("testdata/pure/pure/core.clj")
	if err := PublishClojars(entry, out, WithModule("github.com/you/greet")); err != nil {
		t.Fatalf("PublishClojars(pure): %v", err)
	}

	// Source tree: both namespaces placed by ns → path.
	for _, rel := range []string{"src/pure/core.clj", "src/pure/util.clj"} {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(rel))); err != nil {
			t.Errorf("expected published source %s: %v", rel, err)
		}
	}

	// deps.edn present, git-coordinate, carries the module.
	deps, err := os.ReadFile(filepath.Join(out, "deps.edn"))
	if err != nil {
		t.Fatalf("deps.edn: %v", err)
	}
	if !strings.Contains(string(deps), ":paths [\"src\"]") {
		t.Errorf("deps.edn missing :paths [\"src\"]:\n%s", deps)
	}
	if !strings.Contains(string(deps), "github.com/you/greet") {
		t.Errorf("deps.edn missing module coordinate:\n%s", deps)
	}

	// Pure manifest present.
	man, err := os.ReadFile(filepath.Join(out, "cljgo.manifest.edn"))
	if err != nil {
		t.Fatalf("cljgo.manifest.edn: %v", err)
	}
	if !strings.Contains(string(man), ":pure? true") {
		t.Errorf("manifest not declared pure:\n%s", man)
	}
}

// TestPublishClojarsRefusesGoInterop — a library with a require-go buried in a
// dependency is refused, naming the offending file:line.
func TestPublishClojarsRefusesGoInterop(t *testing.T) {
	out := t.TempDir()
	entry := filepath.FromSlash("testdata/gointerop/gi/core.clj")
	err := PublishClojars(entry, out)
	if err == nil {
		t.Fatalf("PublishClojars(go-interop) should have been refused")
	}
	msg := err.Error()
	if !strings.Contains(msg, "gi.leaf") {
		t.Errorf("error should name the offending namespace gi.leaf: %s", msg)
	}
	if !strings.Contains(filepath.ToSlash(msg), "gi/leaf.clj:3") {
		t.Errorf("error should cite gi/leaf.clj:3 (the sc/Itoa call): %s", msg)
	}
	if !strings.Contains(msg, "cannot run on the JVM") {
		t.Errorf("error should explain the JVM incompatibility: %s", msg)
	}
	// It refused before writing a source tree.
	if _, err := os.Stat(filepath.Join(out, "deps.edn")); err == nil {
		t.Errorf("a refused publish must not write deps.edn")
	}
}

// TestPublishClojarsJavaFlavoredButGoFree — a library using Java-flavored but
// cljgo-analyzable, Go-interop-free surfaces (instance?/catch/class-ref) still
// publishes: the clojars gate is uses-go-interop?, not uses-java? (ADR 0054
// decision 3/4). Java runs on the JVM, so it does not disqualify.
func TestPublishClojarsJavaFlavoredButGoFree(t *testing.T) {
	out := t.TempDir()
	entry := filepath.FromSlash("testdata/javafree/jf/core.clj")
	if err := PublishClojars(entry, out); err != nil {
		t.Fatalf("Java-flavored (go-free) library should publish: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, filepath.FromSlash("src/jf/core.clj"))); err != nil {
		t.Errorf("expected published source src/jf/core.clj: %v", err)
	}
}
