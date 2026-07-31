package eval_test

// Regression coverage for issue #167 (clojure.core/load-file resolves but
// is unbound). Semantic behavior lives in the conformance suite
// (conformance/tests/load-file-basic.clj, dual-oracled against Clojure
// 1.12.5); this file is the Go-level check that a MISSING file produces a
// real error rather than the old cannot-call-nil symptom, plus a direct
// interpreter-level exercise of loadFile.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/eval"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// TestLoadFileReadsAndEvaluates: load-file reads every top-level form in
// the given file and evaluates them in order, returning the LAST value,
// with defs landing in the CURRENT namespace (oracled against Clojure
// 1.12.5 -- see conformance/tests/load-file-basic.clj).
func TestLoadFileReadsAndEvaluates(t *testing.T) {
	e := eval.New()
	ns := freshNS(t, e)
	defer evalSrc(t, e, "(clojure.core/in-ns (quote user))")

	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.cljc")
	if err := os.WriteFile(path, []byte("(def lf-y 10)\n(+ lf-y 5)\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := evalSrc(t, e, fmt.Sprintf("(load-file %q)", path))
	if n, ok := got.(int64); !ok || n != 15 {
		t.Fatalf("(load-file ...) = %#v, want int64(15)", got)
	}

	defVal := evalSrc(t, e, "lf-y")
	if n, ok := defVal.(int64); !ok || n != 10 {
		t.Fatalf("lf-y after load-file = %#v, want int64(10) (defs land in ns %s)", defVal, ns)
	}
}

// TestLoadFileMissingFileErrors: before the fix, resolving
// clojure.core/load-file and then calling it died with the generic
// cannot-call-nil error -- a var that exists but a function that is not
// there (issue #167). Now a missing path fails with an error that
// actually names load-file and the path, never the nil-call symptom.
func TestLoadFileMissingFileErrors(t *testing.T) {
	e := eval.New()
	freshNS(t, e)
	defer evalSrc(t, e, "(clojure.core/in-ns (quote user))")

	path := filepath.Join(t.TempDir(), "does-not-exist.cljc")
	src := fmt.Sprintf("(load-file %q)", path)
	r := reader.New(strings.NewReader(src), reader.WithResolver(e.ReaderResolver()))
	form, err := r.ReadOne()
	if err != nil {
		t.Fatalf("read(%q): %v", src, err)
	}
	_, err = e.EvalForm(form)
	if err == nil {
		t.Fatalf("(load-file %q) succeeded, want an error naming the missing file", path)
	}
	if strings.Contains(err.Error(), "cannot call nil") {
		t.Fatalf("regressed to the pre-fix symptom: %v", err)
	}
	if !strings.Contains(err.Error(), "load-file") || !strings.Contains(err.Error(), path) {
		t.Fatalf("error %q does not name load-file and the missing path", err.Error())
	}
}
