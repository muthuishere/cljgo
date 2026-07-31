package corelib_test

// Regression coverage for issue #167 (clojure.core/load-file resolves
// but is unbound): before this fix, load-file was interned with a bare
// nil root, so it resolved (some? (resolve ...)) => true but calling it
// died with the generic "cannot call nil" symptom. Now, in a binary
// that never constructs an eval.Evaluator -- exactly the AOT-compiled
// shape (ADR 0046) -- load-file is BOUND to a stub that names itself and
// the real constraint, matching eval/macroexpand/macroexpand-1.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/corelib"
	"github.com/muthuishere/cljgo/pkg/lang"
)

func TestLoadFileAOTStub(t *testing.T) {
	corelib.RegisterAll()

	v := lang.NSCore.FindInternedVar(lang.NewSymbol("load-file"))
	if v == nil {
		t.Fatal("clojure.core/load-file does not resolve")
	}
	fn, ok := v.Deref().(lang.IFn)
	if !ok {
		t.Fatalf("clojure.core/load-file root is not callable: %#v", v.Deref())
	}

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("(load-file x) in an interpreter-less binary did not panic")
		}
		msg := fmt.Sprint(r)
		if strings.Contains(msg, "cannot call nil") {
			t.Fatalf("regressed to the pre-fix symptom: %v", r)
		}
		if !strings.Contains(msg, "load-file") || !strings.Contains(msg, "AOT-compiled binary") {
			t.Fatalf("stub message %q does not name load-file and the AOT-compiled-binary constraint", msg)
		}
	}()
	fn.Invoke("x")
}
