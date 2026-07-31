package main

// Regression coverage for issue #168: a fresh clone of a project that
// declares dependencies, never built, gets a coded G5023 diagnostic out of
// `cljgo run` instead of a bare "could not locate namespace" failure from
// the require that follows. No network: the declared dependency is a local
// :path dep, so nothing needs a Maven double.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/build"
)

func writeRunLockFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestResolveRunDepsNoLockNamesTheCause is the failing-without-fix case:
// before this change, a project with declared deps and no build.lock.edn
// silently `continue`d in resolveRunDeps, leaving the caller to fail later
// on the require with an unnamed "could not locate namespace" error.
func TestResolveRunDepsNoLockNamesTheCause(t *testing.T) {
	localDir := t.TempDir()
	writeRunLockFile(t, filepath.Join(localDir, "cljgo.manifest.edn"), `{:paths ["src"]}`)
	writeRunLockFile(t, filepath.Join(localDir, "src", "koine", "fs.clj"), "(ns koine.fs)\n(defn f [] 1)\n")

	proj := t.TempDir()
	writeRunLockFile(t, filepath.Join(proj, "src", "demo", "app.cljc"),
		"(ns demo.app (:require [koine.fs]))\n")
	writeRunLockFile(t, filepath.Join(proj, build.BuildFileName), `(defn build [b]
  (dep b "koine-fs" {:path `+quoted(localDir)+`})
  (let [app (exe b {:name "app" :main "src/demo/app.cljc"})]
    (install b app)))
`)

	// Sanity: no lock committed, exactly the fresh-clone state.
	if _, err := os.Stat(filepath.Join(proj, "build.lock.edn")); err == nil {
		t.Fatal("test setup: build.lock.edn must not exist")
	}

	// resolveRunDeps looks for the build file in the source file's own
	// directory, then in the process's CWD (cmd/cljgo/main.go:215) — exactly
	// what a real `cljgo run src/demo/app.cljc` invoked from the project
	// root sees. Match that here.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(proj); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	err = resolveRunDeps(filepath.Join(proj, "src", "demo", "app.cljc"))
	if err == nil {
		t.Fatal("resolveRunDeps: expected a G5023 error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "G5023") {
		t.Fatalf("resolveRunDeps error does not carry G5023:\n%s", msg)
	}
	if !strings.Contains(msg, "build.lock.edn") {
		t.Fatalf("resolveRunDeps error does not name build.lock.edn:\n%s", msg)
	}
	if !strings.Contains(msg, "cljgo build") {
		t.Fatalf("resolveRunDeps error does not point at `cljgo build`:\n%s", msg)
	}
}

// TestResolveRunDepsNoDepsIsANoOp pins the non-regression: a project that
// declares NO dependencies has no build.lock.edn either, and that must stay
// a silent no-op (ADR 0052) — only the declares-deps-but-unbuilt case gets
// the new diagnostic.
func TestResolveRunDepsNoDepsIsANoOp(t *testing.T) {
	proj := t.TempDir()
	writeRunLockFile(t, filepath.Join(proj, "src", "demo", "app.cljc"), "(ns demo.app)\n")
	writeRunLockFile(t, filepath.Join(proj, build.BuildFileName), `(defn build [b]
  (let [app (exe b {:name "app" :main "src/demo/app.cljc"})]
    (install b app)))
`)
	if err := resolveRunDeps(filepath.Join(proj, "src", "demo", "app.cljc")); err != nil {
		t.Fatalf("resolveRunDeps on a no-deps project: %v", err)
	}
}

func quoted(s string) string {
	return `"` + s + `"`
}
