package build

import (
	"os"
	"path/filepath"
	"testing"
)

// ADR 0051: `cljgo build` accepts build.cljgo / build.cljg / build.clj,
// most-specific-first, and reports "" when none is present.
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
		write(dir, "build.clj")
		write(dir, "build.cljg")
		write(dir, "build.cljgo")
		if got := FindBuildFile(dir); got != filepath.Join(dir, "build.cljgo") {
			t.Fatalf("precedence: want build.cljgo, got %q", got)
		}
	})

	t.Run("cljg beats clj", func(t *testing.T) {
		dir := t.TempDir()
		write(dir, "build.clj")
		write(dir, "build.cljg")
		if got := FindBuildFile(dir); got != filepath.Join(dir, "build.cljg") {
			t.Fatalf("precedence: want build.cljg, got %q", got)
		}
	})
}
