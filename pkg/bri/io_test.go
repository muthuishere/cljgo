// io_test.go — cljg.io filesystem behavior through the interpreter (ADR 0089).
// No JVM oracle; these drive the namespace against a real Go t.TempDir() so the
// stat/list/mkdir/delete/copy/move/glob/walk/path ops are exercised end to end
// (pure-Go, CI-safe). The alias is `cio` — vars are process-global, so it must
// not collide with another package's `io`/`os` test alias.
package bri_test

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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

// TestCljgIORealPath is the regression test for issue #172: `absolute` is
// purely lexical (filepath.Abs) and cannot resolve symlinks, so it cannot
// canonicalise a path or guard a directory walk against a symlink cycle.
// `real-path` (filepath.EvalSymlinks) must: (1) resolve a symlink to its real
// target, (2) throw on a path that does not exist rather than faking a
// value, and (3) throw on a symlink cycle (ELOOP) rather than hanging.
func TestCljgIORealPath(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[cljg.io :as cio])`)

	tmp := t.TempDir()
	real, err := filepath.EvalSymlinks(tmp) // macOS temp dirs are themselves a symlink (/tmp -> /private/tmp)
	if err != nil {
		t.Fatal(err)
	}

	// a symlink resolves to its real target, absolute and cleaned
	target := filepath.Join(real, "target.txt")
	if err := os.WriteFile(target, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(real, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	got := evalString(t, d, `(cio/real-path "`+filepath.ToSlash(link)+`")`)
	if filepath.ToSlash(got) != filepath.ToSlash(target) {
		t.Errorf("real-path %q = %q, want %q", link, got, target)
	}

	// a non-existent path throws (coded G5024), not a faked value
	msg := evalErr(t, d, `(cio/real-path "`+filepath.ToSlash(real)+`/does-not-exist")`)
	if !strings.Contains(msg, "real-path") {
		t.Errorf("real-path on a missing path error = %q, want it to mention real-path", msg)
	}

	// a symlink cycle throws (ELOOP) rather than hanging forever — this is
	// the case `absolute` cannot detect at all, since it never touches the
	// filesystem.
	looped := filepath.Join(real, "loop")
	if err := os.Symlink(looped, looped); err != nil { // a symlink pointing at itself
		t.Fatal(err)
	}
	done := make(chan string, 1)
	go func() { done <- evalErr(t, d, `(cio/real-path "`+filepath.ToSlash(looped)+`")`) }()
	select {
	case msg := <-done:
		if msg == "" {
			t.Errorf("real-path on a symlink cycle did not throw")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("real-path on a symlink cycle hung instead of throwing ELOOP")
	}
}

// TestHelperProcess is the standard Go cross-platform subprocess: re-invoked as
// the child of the exec tests below (guarded by GO_WANT_HELPER_PROCESS so it is
// inert as a normal test). It reads a mode after "--" and behaves as a tiny,
// portable command — echo/cat/toerr/sleep/printenv — so cljg.io's exec is
// exercised on macOS/Linux/Windows with no external binary dependency.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	i := 0
	for ; i < len(args); i++ {
		if args[i] == "--" {
			i++
			break
		}
	}
	rest := args[i:]
	mode := ""
	if len(rest) > 0 {
		mode = rest[0]
	}
	switch mode {
	case "echo":
		fmt.Fprint(os.Stdout, strings.Join(rest[1:], " "))
	case "toerr":
		fmt.Fprint(os.Stderr, strings.Join(rest[1:], " "))
		os.Exit(3)
	case "cat":
		io.Copy(os.Stdout, os.Stdin)
	case "upcaselines":
		// read stdin a line at a time, echo each UPPER-CASED and flushed
		// immediately (no EOF needed) — exercises cljg.process bidirectional
		// streaming through the live pipes (ADR 0101).
		sc := bufio.NewScanner(os.Stdin)
		w := bufio.NewWriter(os.Stdout)
		for sc.Scan() {
			w.WriteString(strings.ToUpper(sc.Text()))
			w.WriteByte('\n')
			w.Flush()
		}
	case "count":
		// emit N lines "line-1".."line-N" to stdout (for stream-reduce tests).
		n, _ := strconv.Atoi(rest[1])
		w := bufio.NewWriter(os.Stdout)
		for i := 1; i <= n; i++ {
			fmt.Fprintf(w, "line-%d\n", i)
		}
		w.Flush()
	case "sleep":
		ms, _ := strconv.Atoi(rest[1])
		time.Sleep(time.Duration(ms) * time.Millisecond)
	case "printenv":
		fmt.Fprint(os.Stdout, os.Getenv(rest[1]))
	}
	os.Exit(0)
}

func TestCljgIOExec(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[cljg.io :as cio])`)

	self := filepath.ToSlash(os.Args[0])
	// cmd builds a Clojure vector literal invoking this test binary as the helper.
	cmd := func(parts ...string) string {
		var b strings.Builder
		b.WriteString("[")
		for _, p := range append([]string{self, "-test.run=TestHelperProcess", "--"}, parts...) {
			fmt.Fprintf(&b, " %q", p)
		}
		b.WriteString("]")
		return b.String()
	}
	// every exec must carry the helper marker in its env
	base := `{:env {"GO_WANT_HELPER_PROCESS" "1"}}`

	// stdout capture + zero exit
	if got := evalString(t, d, `(:out (cio/exec `+cmd("echo", "hello", "world")+` `+base+`))`); got != "hello world" {
		t.Errorf("exec echo :out = %q, want %q", got, "hello world")
	}
	if got := evalString(t, d, `(str (:exit (cio/exec `+cmd("echo", "x")+` `+base+`)))`); got != "0" {
		t.Errorf("exec echo :exit = %q, want 0", got)
	}

	// non-zero exit is a NORMAL result (exec does not throw), stderr captured
	res := evalString(t, d, `(let [r (cio/exec `+cmd("toerr", "boom")+` `+base+`)] (str (:exit r) "|" (:err r)))`)
	if res != "3|boom" {
		t.Errorf("exec toerr = %q, want %q", res, "3|boom")
	}

	// stdin via :in
	catOpts := `{:env {"GO_WANT_HELPER_PROCESS" "1"} :in "piped-data"}`
	if got := evalString(t, d, `(:out (cio/exec `+cmd("cat")+` `+catOpts+`))`); got != "piped-data" {
		t.Errorf("exec cat :in = %q, want %q", got, "piped-data")
	}

	// :env is merged onto the current environment
	envOpts := `{:env {"GO_WANT_HELPER_PROCESS" "1" "CLJGIO_FOO" "bar"}}`
	if got := evalString(t, d, `(:out (cio/exec `+cmd("printenv", "CLJGIO_FOO")+` `+envOpts+`))`); got != "bar" {
		t.Errorf("exec printenv :env = %q, want %q", got, "bar")
	}

	// :timeout-ms kills a slow process
	toOpts := `{:env {"GO_WANT_HELPER_PROCESS" "1"} :timeout-ms 150}`
	to := evalString(t, d, `(let [r (cio/exec `+cmd("sleep", "3000")+` `+toOpts+`)] (str (:timed-out? r) "|" (:exit r)))`)
	if to != "true|-1" {
		t.Errorf("exec timeout = %q, want %q", to, "true|-1")
	}

	// sh! THROWS on non-zero exit
	if msg := evalErr(t, d, `(cio/sh! `+cmd("toerr", "nope")+` `+base+`)`); !strings.Contains(msg, "exit 3") {
		t.Errorf("sh! on non-zero exit error = %q, want it to mention exit 3", msg)
	}
	// sh! returns the result map on success
	if got := evalString(t, d, `(:out (cio/sh! `+cmd("echo", "ok")+` `+base+`))`); got != "ok" {
		t.Errorf("sh! success :out = %q, want ok", got)
	}

	// a missing binary THROWS (it never ran) — distinct from a non-zero exit
	if msg := evalErr(t, d, `(cio/exec ["cljgo-no-such-binary-xyzzy"] {})`); !strings.Contains(msg, "exec") {
		t.Errorf("missing binary error = %q, want it to mention exec", msg)
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
