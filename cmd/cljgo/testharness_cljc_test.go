// testharness_cljc_test.go — `cljgo test` and `cljgo test --compiled` must
// accept the SAME suite. Issue #182: they did not, for `.cljc`.
//
// nsNameFor derives a namespace symbol from a file path by dropping the
// extension, and carried its own hand-written list that trimmed only
// ".cljg" and ".clj". A `.cljc` file therefore produced the namespace
// `demo.core-test.cljc` — the extension became a name segment. The
// interpreted leg never noticed, because it loads the file directly; the
// compiled leg emits a require for that name and dies with "could not
// locate namespace …cljc".
//
// The consequence is worse than a broken flag. `.cljc` is the dual-host
// extension, so this hit exactly the projects that target JVM Clojure AND
// cljgo — and it failed in the direction that LOOKS fine: `cljgo test`
// green, `cljgo test --compiled` unable to build. Anyone treating
// --compiled as their AOT test path silently had no AOT coverage at all,
// which is the REPL-vs-binary divergence ADR 0007 calls unforgivable.
//
// Reported by the toolnexus Clojure port against the v0.8.5 release
// archive, with a public repro in their spikes/s24-test-harness. They
// ranked it LOW for themselves — they cover AOT via `cljgo build` plus
// running the binary — and filed it anyway, because the people it silently
// breaks are the ones who would never find out.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestNsNameForDropsEverySourceExtension pins the unit: every extension the
// resolver accepts must be dropped here, or the two disagree about what a
// file's namespace is called.
func TestNsNameForDropsEverySourceExtension(t *testing.T) {
	root := filepath.FromSlash("/proj/test")
	cases := map[string]string{
		"demo/core_test.cljc":  "demo.core-test",
		"demo/core_test.clj":   "demo.core-test",
		"demo/core_test.cljg":  "demo.core-test",
		"demo/core_test.cljgo": "demo.core-test",
	}
	for rel, want := range cases {
		t.Run(rel, func(t *testing.T) {
			got := nsNameFor(filepath.Join(root, filepath.FromSlash(rel)), root)
			if got != want {
				t.Errorf("nsNameFor(%s) = %q, want %q", rel, got, want)
			}
		})
	}
}

// TestCompiledTestRunsACljcSuite is the end-to-end case, run through the
// real binary: a .cljc suite must pass on BOTH legs. Asserting only the
// compiled leg would miss a fix that broke the interpreted one.
func TestCompiledTestRunsACljcSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	bin := buildCljgo(t)
	proj := t.TempDir()
	writeCljcProject(t, proj, map[string]string{
		"src/demo/core.cljc": "(ns demo.core)\n(defn add [a b] (+ a b))\n",
		"test/demo/core_test.cljc": "(ns demo.core-test\n" +
			"  (:require [clojure.test :refer [deftest is]]\n" +
			"            [demo.core :as c]))\n" +
			"(deftest adds (is (= 3 (c/add 1 2))))\n",
		"build.cljgo": "(defn build [b])\n",
	})

	for _, leg := range []struct {
		name string
		args []string
	}{
		{"interpreted", []string{"test"}},
		{"compiled", []string{"test", "--compiled"}},
	} {
		t.Run(leg.name, func(t *testing.T) {
			cmd := exec.Command(bin, leg.args...)
			cmd.Dir = proj
			cmd.Env = append(os.Environ(), "CLJGO_SRC="+repoRoot(t))
			raw, err := cmd.CombinedOutput()
			out := string(raw)
			code := 0
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else if err != nil {
				t.Fatalf("running cljgo %v: %v", leg.args, err)
			}
			// Name the specific failure so a regression is unmistakable.
			if strings.Contains(out, ".cljc (no registered provider") ||
				strings.Contains(out, "core-test.cljc") {
				t.Fatalf("%s leg kept the .cljc extension in the namespace:\n%s", leg.name, out)
			}
			if code != 0 {
				t.Fatalf("%s leg: exit %d\n%s", leg.name, code, out)
			}
			if !strings.Contains(out, "0 failures, 0 errors") {
				t.Fatalf("%s leg did not report a clean run:\n%s", leg.name, out)
			}
		})
	}
}

// writeCljcProject materialises a project tree from slash-separated
// relative paths.
func writeCljcProject(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
