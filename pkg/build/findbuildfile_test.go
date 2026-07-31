package build

import (
	"os"
	"path/filepath"
	"testing"
)

// ADR 0055, narrowed by ADR 0117: `cljgo build` accepts build.cljgo /
// build.cljg, most-specific-first, and reports "" when none is present.
// `build.clj` is NOT accepted — it is the tools.build convention.
func TestFindBuildFile(t *testing.T) {
	write := func(dir, name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte("(defn build [b])"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("none present → empty", func(t *testing.T) {
		if got := FindBuildFile(t.TempDir()); got != "" {
			t.Fatalf("empty dir: want \"\", got %q", got)
		}
	})

	t.Run("each single name is found", func(t *testing.T) {
		for _, name := range BuildFileNames {
			dir := t.TempDir()
			write(dir, name)
			if got := FindBuildFile(dir); got != filepath.Join(dir, name) {
				t.Fatalf("%s: got %q", name, got)
			}
		}
	})

	t.Run("most-specific wins when several coexist", func(t *testing.T) {
		dir := t.TempDir()
		write(dir, "build.cljg")
		write(dir, "build.cljgo")
		if got := FindBuildFile(dir); got != filepath.Join(dir, "build.cljgo") {
			t.Fatalf("precedence: want build.cljgo, got %q", got)
		}
	})

	t.Run("cljg wins when it is the most specific present", func(t *testing.T) {
		dir := t.TempDir()
		write(dir, "build.cljg")
		if got := FindBuildFile(dir); got != filepath.Join(dir, "build.cljg") {
			t.Fatalf("precedence: want build.cljg, got %q", got)
		}
	})

	// ADR 0117 / issue #176: `build.clj` is the tools.build convention, and a
	// dual-host library must have one to publish to Clojars. Claiming it made
	// cljgo EVALUATE another tool's build script — koine's requires
	// `clojure.tools.build.api`, so every `cljgo run` in the project died with
	// "could not locate namespace clojure.tools.build.api". A lone build.clj
	// must therefore read as "no cljgo build file", not as ours.
	t.Run("a JVM tools.build build.clj is not a cljgo build file", func(t *testing.T) {
		dir := t.TempDir()
		write(dir, "build.clj")
		if got := FindBuildFile(dir); got != "" {
			t.Fatalf("build.clj must not be claimed: got %q", got)
		}
	})

	// And it must not win when a real cljgo build file sits beside it — the
	// exact dual-host layout: build.clj for the JVM, build.cljgo for cljgo.
	t.Run("build.clj never shadows a real cljgo build file", func(t *testing.T) {
		dir := t.TempDir()
		write(dir, "build.clj")
		write(dir, "build.cljgo")
		if got := FindBuildFile(dir); got != filepath.Join(dir, "build.cljgo") {
			t.Fatalf("dual-host layout: want build.cljgo, got %q", got)
		}
	})
}
