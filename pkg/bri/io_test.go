// io_test.go — cljg.io filesystem behavior through the interpreter (ADR 0089).
// No JVM oracle; these drive the namespace against a real Go t.TempDir() so the
// stat/list/mkdir/delete/copy/move/glob/walk/path ops are exercised end to end
// (pure-Go, CI-safe). The alias is `cio` — vars are process-global, so it must
// not collide with another package's `io`/`os` test alias.
package bri_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCljgIO(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[cljg.io :as cio])`)

	tmp := t.TempDir()
	// forward slashes so the string embeds cleanly in Clojure source on any OS;
	// filepath ops normalize per platform anyway.
	base := filepath.ToSlash(tmp)

	// mkdirs creates a nested tree and is idempotent
	if got := evalString(t, d, `(str (cio/mkdirs "`+base+`/a/b/c"))`); !strings.HasSuffix(got, "/a/b/c") {
		t.Errorf("mkdirs returned %q", got)
	}
	if got := eval(t, d, `(cio/directory? "`+base+`/a/b/c")`); got != true {
		t.Errorf("directory? after mkdirs = %v, want true", got)
	}
	if got := eval(t, d, `(cio/exists? "`+base+`/nope")`); got != false {
		t.Errorf("exists? on absent path = %v, want false", got)
	}

	// spit (clojure.core, NOT shadowed) writes; cljg.io reads structure around it
	if err := os.WriteFile(filepath.Join(tmp, "a", "hello.txt"), []byte("hi there"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := eval(t, d, `(cio/file? "`+base+`/a/hello.txt")`); got != true {
		t.Errorf("file? = %v, want true", got)
	}
	if got := eval(t, d, `(cio/size "`+base+`/a/hello.txt")`); got != int64(8) {
		t.Errorf("size = %v, want 8", got)
	}
	if got := eval(t, d, `(some? (cio/modified "`+base+`/a/hello.txt"))`); got != true {
		t.Errorf("modified should be present for an existing file")
	}
	if got := eval(t, d, `(cio/size "`+base+`/a/missing")`); got != nil {
		t.Errorf("size of absent path = %v, want nil", got)
	}

	// list-files returns entry names, sorted
	if got := evalString(t, d, `(pr-str (cio/list-files "`+base+`/a"))`); !strings.Contains(got, `"hello.txt"`) || !strings.Contains(got, `"b"`) {
		t.Errorf("list-files = %s, want b + hello.txt", got)
	}

	// copy! then move!
	eval(t, d, `(cio/copy! "`+base+`/a/hello.txt" "`+base+`/a/copy.txt")`)
	if got := eval(t, d, `(cio/file? "`+base+`/a/copy.txt")`); got != true {
		t.Errorf("copy! did not create the destination")
	}
	eval(t, d, `(cio/move! "`+base+`/a/copy.txt" "`+base+`/a/moved.txt")`)
	if got := eval(t, d, `[(cio/exists? "`+base+`/a/copy.txt") (cio/file? "`+base+`/a/moved.txt")]`); got == nil {
		t.Fatal("move! eval failed")
	} else if s := evalString(t, d, `(pr-str [(cio/exists? "`+base+`/a/copy.txt") (cio/file? "`+base+`/a/moved.txt")])`); s != "[false true]" {
		t.Errorf("after move! got %s, want [false true]", s)
	}

	// glob + walk
	if got := evalString(t, d, `(pr-str (mapv cio/filename (cio/glob "`+base+`/a/*.txt")))`); !strings.Contains(got, `"hello.txt"`) || !strings.Contains(got, `"moved.txt"`) {
		t.Errorf("glob *.txt = %s", got)
	}
	if got := eval(t, d, `(some #(clojure.string/ends-with? % "hello.txt") (cio/walk "`+base+`/a"))`); got != true {
		t.Errorf("walk should reach the nested file")
	}

	// delete! a file, delete-tree! a populated dir
	eval(t, d, `(cio/delete! "`+base+`/a/moved.txt")`)
	if got := eval(t, d, `(cio/exists? "`+base+`/a/moved.txt")`); got != false {
		t.Errorf("delete! left the file")
	}
	eval(t, d, `(cio/delete! "`+base+`/a/never-existed")`) // absent is not an error
	eval(t, d, `(cio/delete-tree! "`+base+`/a")`)
	if got := eval(t, d, `(cio/exists? "`+base+`/a")`); got != false {
		t.Errorf("delete-tree! left the directory")
	}
}

func TestCljgIOPaths(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[cljg.io :as cio])`)

	if got := evalString(t, d, `(cio/path "a" "b" "c")`); got != filepath.Join("a", "b", "c") {
		t.Errorf("path join = %q", got)
	}
	if got := evalString(t, d, `(cio/parent "a/b/c.txt")`); got != filepath.FromSlash("a/b") {
		t.Errorf("parent = %q", got)
	}
	if got := evalString(t, d, `(cio/filename "a/b/c.txt")`); got != "c.txt" {
		t.Errorf("filename = %q", got)
	}
	if got := evalString(t, d, `(cio/extension "a/b/c.txt")`); got != ".txt" {
		t.Errorf("extension = %q", got)
	}
	if got := evalString(t, d, `(cio/extension "a/b/c")`); got != "" {
		t.Errorf("extension (none) = %q, want empty", got)
	}
	// absolute resolves against cwd
	if got := evalString(t, d, `(cio/absolute "x")`); !filepath.IsAbs(got) {
		t.Errorf("absolute did not return an absolute path: %q", got)
	}
	// home / cwd are non-empty absolute-ish
	if got := evalString(t, d, `(cio/home)`); got == "" {
		t.Errorf("home was empty")
	}
	if got := evalString(t, d, `(cio/cwd)`); got == "" {
		t.Errorf("cwd was empty")
	}
}

func TestCljgIOTemp(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[cljg.io :as cio])`)

	// temp-file with prefix+suffix is created and matches
	f := evalString(t, d, `(cio/temp-file "cljgtest-" ".log")`)
	if !strings.Contains(filepath.Base(f), "cljgtest-") || !strings.HasSuffix(f, ".log") {
		t.Errorf("temp-file name = %q", f)
	}
	if _, err := os.Stat(f); err != nil {
		t.Errorf("temp-file was not created: %v", err)
	}
	os.Remove(f)

	// temp-dir is a real directory
	dir := evalString(t, d, `(cio/temp-dir "cljgtest-")`)
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Errorf("temp-dir not created as a directory: %v", err)
	}
	os.RemoveAll(dir)
}
