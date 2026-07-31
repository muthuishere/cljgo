package repl

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// captureStderr redirects the REAL process os.Stderr for the duration of
// fn and returns everything written to it. Needed because
// Namespace.checkReplacement (pkg/lang/namespace.go) prints the shadow
// WARNING straight to os.Stderr, ignoring any Driver-injected errOut.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	fn()

	w.Close()
	os.Stderr = orig
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

// TestNoShadowWarningForOkErr is the regression test for issue #171:
// clojure.core exposed ok/err (ADR 0014 Result/Option constructors) with
// NO JVM Clojure counterpart, so legitimate user code defining
// (defn ok ...) / (defn err ...) tripped a shadow WARNING on cljgo that
// could never fire on the JVM -- a precedence-principle violation
// (CLAUDE.md: cljgo may not put new names into clojure.core where the
// JVM has none). ok/err now live in cljx.meta (ADR 0115), so a fresh
// namespace never sees them unless it explicitly requires+refers
// cljx.meta, exactly matching the JVM (which never had them at all).
//
// checkReplacement (pkg/lang/namespace.go) writes the shadow warning
// straight to the process os.Stderr, bypassing this Driver own errOut
// writer entirely -- so this test captures the REAL os.Stderr for the
// duration of the eval, the only way to observe it.
func TestNoShadowWarningForOkErr(t *testing.T) {
	lang.RemoveNamespace(lang.NewSymbol("app.tool171"))
	defer lang.RemoveNamespace(lang.NewSymbol("app.tool171"))

	stderrOut := captureStderr(t, func() {
		src := `(ns app.tool171)
(defn ok [x] {:output x :isError false})
(defn err [x] {:output x :isError true})
(ok 5)
`
		if _, errOut := run(t, src); errOut != "" {
			t.Fatalf("unexpected driver errOut: %q", errOut)
		}
	})
	if strings.Contains(stderrOut, "WARNING") {
		t.Fatalf("defining ok/err in a fresh namespace warned (should not, ok/err are not in clojure.core): %q", stderrOut)
	}
}

// TestOkErrNotInClojureCore: (resolve (quote clojure.core/ok)) must be
// nil, matching the real clojure CLI 1.12.5 oracle exactly (neither ok
// nor err exist there) -- the direct fix for issue #171.
func TestOkErrNotInClojureCore(t *testing.T) {
	lang.RemoveNamespace(lang.NewSymbol("user"))
	d := New(strings.NewReader(""), &strings.Builder{}, &strings.Builder{})
	src := `[(resolve (quote clojure.core/ok)) (resolve (quote clojure.core/err))]`
	v, err := d.EvalReader(strings.NewReader(src), "probe171.clj")
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	if got := lang.PrintString(v); got != "[nil nil]" {
		t.Fatalf("[(resolve clojure.core/ok) (resolve clojure.core/err)] = %s, want [nil nil] (oracle: clojure 1.12.5 has neither)", got)
	}
}
