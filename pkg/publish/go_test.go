package publish

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPublishGoPureLibraryBuilds — a pure library `publish go` emits a
// go-gettable module that `go build ./...`s, with exported wrappers for the
// public defns (and no wrapper for the ^:private one or the entry point).
func TestPublishGoPureLibraryBuilds(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping go build in -short")
	}
	out := t.TempDir()
	entry := filepath.FromSlash("testdata/pure/pure/core.clj")
	res, err := PublishGo(entry, out, WithModule("example.com/greetlib"))
	if err != nil {
		t.Fatalf("PublishGo(pure): %v", err)
	}
	if res.EntryNS != "pure.core" {
		t.Errorf("entry ns = %q, want pure.core", res.EntryNS)
	}

	// Exported surface: greet (fn) + answer (value); no private helper, no -main.
	got := map[string]bool{}
	for _, e := range res.Exports {
		got[e.CljName] = e.IsFn
	}
	if isFn, ok := got["greet"]; !ok || !isFn {
		t.Errorf("greet should be an exported fn wrapper; exports=%+v", res.Exports)
	}
	if isFn, ok := got["answer"]; !ok || isFn {
		t.Errorf("answer should be an exported value getter; exports=%+v", res.Exports)
	}

	// go.mod carries the library module path.
	mod, err := os.ReadFile(filepath.Join(out, "go.mod"))
	if err != nil {
		t.Fatalf("go.mod: %v", err)
	}
	if !strings.Contains(string(mod), "module example.com/greetlib") {
		t.Errorf("go.mod missing library module path:\n%s", mod)
	}

	// The entry package's wrappers.go exists and exports Greet/Answer.
	wrap, err := os.ReadFile(filepath.Join(out, filepath.FromSlash("pure/core/wrappers.go")))
	if err != nil {
		t.Fatalf("wrappers.go: %v", err)
	}
	for _, name := range []string{"func Greet(args ...any) any", "func Answer() any"} {
		if !strings.Contains(string(wrap), name) {
			t.Errorf("wrappers.go missing %q:\n%s", name, wrap)
		}
	}

	// The whole module compiles.
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = out
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build ./... on emitted module failed: %v\n%s", err, out)
	}
}

// TestPublishGoGoInteropLibraryBuilds — a library that uses stdlib Go interop
// (require-go strconv) publishes to go (Go interop is allowed on the go target)
// and the emitted module compiles.
func TestPublishGoGoInteropLibraryBuilds(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping go build in -short")
	}
	out := t.TempDir()
	entry := filepath.FromSlash("testdata/gointerop/gi/core.clj")
	res, err := PublishGo(entry, out, WithModule("example.com/golib"))
	if err != nil {
		t.Fatalf("PublishGo(go-interop): %v", err)
	}
	if res.EntryNS != "gi.core" {
		t.Errorf("entry ns = %q, want gi.core", res.EntryNS)
	}
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = out
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build ./... on go-interop module failed: %v\n%s", err, out)
	}
}
