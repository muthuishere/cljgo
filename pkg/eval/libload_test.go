package eval

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// ADR 0051: ResolveLibPath accepts .clj, .cljg, and .cljgo, most-specific-first
// (.cljgo > .cljg > .clj). This is the single shared resolver, so both legs
// inherit the order identically.
func TestResolveLibPathExtensionPrecedence(t *testing.T) {
	dir := t.TempDir()
	nsName := "extprectest"
	ns := lang.FindOrCreateNamespace(lang.NewSymbol(nsName))
	defer lang.RemoveNamespace(lang.NewSymbol(nsName))

	resolve := func(reqName string) string {
		lang.PushThreadBindings(lang.NewMap(
			lang.VarCurrentNS, ns,
			lang.VarFile, filepath.Join(dir, "entry.clj"),
		))
		defer lang.PopThreadBindings()
		return ResolveLibPath(lang.NewSymbol(reqName))
	}
	write := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte("(ns dep)"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rm := func(name string) {
		t.Helper()
		_ = os.Remove(filepath.Join(dir, name))
	}

	// .cljgo wins when all three coexist.
	write("dep.clj")
	write("dep.cljg")
	write("dep.cljgo")
	if got := resolve("dep"); got != filepath.Join(dir, "dep.cljgo") {
		t.Fatalf("all three present: want dep.cljgo, got %q", got)
	}

	// Falls back to .cljg, then .clj, as the specific ones are removed.
	rm("dep.cljgo")
	if got := resolve("dep"); got != filepath.Join(dir, "dep.cljg") {
		t.Fatalf("after removing .cljgo: want dep.cljg, got %q", got)
	}
	rm("dep.cljg")
	if got := resolve("dep"); got != filepath.Join(dir, "dep.clj") {
		t.Fatalf("after removing .cljg: want dep.clj, got %q", got)
	}

	// A lone .cljgo resolves (the newly-accepted extension).
	rm("dep.clj")
	write("solo.cljgo")
	if got := resolve("solo"); got != filepath.Join(dir, "solo.cljgo") {
		t.Fatalf("lone .cljgo: want solo.cljgo, got %q", got)
	}
}
