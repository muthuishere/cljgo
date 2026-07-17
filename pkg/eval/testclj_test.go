package eval_test

// Tests for the interpreted clojure.test slice (core/test.cljg + the
// pkg/eval builtins/loader that back it). Counts are frozen against real
// JVM Clojure 1.12.5 clojure.test (the oracle): a 4-deftest mix of
// {2 passing =, 1 failing =, 1 throwing, 1 testing-block pass} yields
//   {:test 4 :pass 3 :fail 1 :error 1 :type :summary}
// ("Ran 4 tests containing 5 assertions.").

import (
	"bytes"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/muthuishere/cljgo/pkg/corelib"
	"github.com/muthuishere/cljgo/pkg/eval"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// Namespaces are global (lang package state), so eval.New() reuses the
// same `user` ns across tests and deftests would accumulate. Each test
// runs in its own fresh, uniquely-named namespace for isolation.
var nsCounter atomic.Int64

// freshNS switches e into a brand-new empty namespace referring only
// clojure.core, and returns its name (mirrors a fresh `user`).
func freshNS(t *testing.T, e *eval.Evaluator) string {
	t.Helper()
	name := fmt.Sprintf("cljgo.test-scratch-%d", nsCounter.Add(1))
	evalSrc(t, e, "(clojure.core/in-ns '"+name+")")
	evalSrc(t, e, "(clojure.core/refer 'clojure.core)")
	return name
}

// evalSrc reads and evaluates every form in src (through the reader, with
// the evaluator's live-ns resolver), returning the last value.
func evalSrc(t *testing.T, e *eval.Evaluator, src string) any {
	t.Helper()
	r := reader.New(strings.NewReader(src), reader.WithResolver(e.ReaderResolver()))
	var res any
	for {
		form, err := r.ReadOne()
		if err != nil {
			if err.Error() == "EOF" || strings.Contains(err.Error(), "EOF") {
				return res
			}
			t.Fatalf("read(%q): %v", src, err)
		}
		res, err = e.EvalForm(form)
		if err != nil {
			t.Fatalf("eval(%s): %v", lang.PrintString(form), err)
		}
	}
}

func kw(name string) lang.Keyword { return lang.NewKeyword(name) }

// summaryCount pulls an int64 count out of the run-tests summary map.
func summaryCount(t *testing.T, summary any, key string) int64 {
	t.Helper()
	m, ok := summary.(lang.IPersistentMap)
	if !ok {
		t.Fatalf("summary is not a map: %s", lang.PrintString(summary))
	}
	v := lang.Get(m, kw(key))
	n, ok := v.(int64)
	if !ok {
		t.Fatalf("summary %s = %v (%T), want int64", key, v, v)
	}
	return n
}

// bootTest boots an evaluator, moves into a fresh scratch namespace, and
// refers clojure.test into it — the standard way user code brings in the
// test idioms.
func bootTest(t *testing.T) *eval.Evaluator {
	t.Helper()
	e := eval.New()
	freshNS(t, e)
	evalSrc(t, e, `(require 'clojure.test) (refer 'clojure.test)`)
	return e
}

func TestClojureTestResolvableViaRequire(t *testing.T) {
	e := eval.New()
	freshNS(t, e)
	// clojure.test must be loadable and its vars reachable qualified even
	// before any refer.
	evalSrc(t, e, `(require 'clojure.test)`)
	if v := lang.FindNamespace(lang.NewSymbol("clojure.test")).
		FindInternedVar(lang.NewSymbol("run-tests")); v == nil {
		t.Fatal("clojure.test/run-tests not interned after require")
	}
	// require must NOT auto-refer into user: bare `deftest` stays unresolved.
	if _, err := e.EvalForm(mustRead(t, e, `(deftest z)`)); err == nil {
		t.Fatal("require should not refer clojure.test names into user")
	}
}

func TestDeftestRegistersTestVar(t *testing.T) {
	e := bootTest(t)
	evalSrc(t, e, `(deftest my-test (is (= 1 1)))`)
	v := e.CurrentNS().FindInternedVar(lang.NewSymbol("my-test"))
	if v == nil {
		t.Fatal("deftest did not intern a var")
	}
	// The test body thunk lives on the var's :test metadata (clojure.test).
	testMeta := lang.Get(v.Meta(), kw("test"))
	if _, ok := testMeta.(lang.IFn); !ok {
		t.Fatalf(":test metadata is not a fn: %v (%T)", testMeta, testMeta)
	}
}

func TestRunTestsCountsMix(t *testing.T) {
	e := bootTest(t)
	var out bytes.Buffer
	old := corelib.Out
	corelib.Out = &out
	defer func() { corelib.Out = old }()

	evalSrc(t, e, `
		(deftest t-pass (is (= 1 1)) (is (= 2 2)))
		(deftest t-fail (is (= 1 2)))
		(deftest t-err  (is (= 1 (/ 1 0))))
		(deftest t-ctx  (testing "context" (is (= 3 3))))`)

	summary := evalSrc(t, e, `(run-tests)`)

	// Counts frozen against JVM clojure.test 1.12.5 (oracle).
	for _, tc := range []struct {
		key  string
		want int64
	}{
		{"test", 4},  // four deftests run
		{"pass", 3},  // 1+1 in t-pass, 1 in t-ctx
		{"fail", 1},  // (= 1 2)
		{"error", 1}, // (/ 1 0) throws
	} {
		if got := summaryCount(t, summary, tc.key); got != tc.want {
			t.Errorf(":%s = %d, want %d", tc.key, got, tc.want)
		}
	}

	// Summary shape includes :type :summary, exactly like clojure.test.
	if typ := lang.Get(summary.(lang.IPersistentMap), kw("type")); !lang.Equiv(typ, kw("summary")) {
		t.Errorf(":type = %v, want :summary", typ)
	}

	// The failure report must surface the failing form + expected/actual.
	s := out.String()
	for _, want := range []string{"FAIL", "expected:", "(= 1 2)", "actual:", "(not (= 1 2))"} {
		if !strings.Contains(s, want) {
			t.Errorf("failure report missing %q; got:\n%s", want, s)
		}
	}
	// And the summary banner line.
	if !strings.Contains(s, "Ran 4 tests containing 5 assertions.") {
		t.Errorf("missing summary banner; got:\n%s", s)
	}
}

func TestSuccessfulPredicate(t *testing.T) {
	e := bootTest(t)
	discardOut(t)
	evalSrc(t, e, `(deftest ok (is (= 1 1)))`)
	if got := evalSrc(t, e, `(successful? (run-tests))`); got != true {
		t.Errorf("successful? on an all-pass run = %v, want true", got)
	}
	evalSrc(t, e, `(deftest bad (is (= 1 2)))`)
	if got := evalSrc(t, e, `(successful? (run-tests))`); got != false {
		t.Errorf("successful? with a failure = %v, want false", got)
	}
}

func TestThrownSupported(t *testing.T) {
	e := bootTest(t)
	discardOut(t)
	// (is (thrown? Class body)) now uses real try/catch: it passes when the
	// body throws a matching exception and fails when the body does not
	// throw. Class matching is the evaluator's CatchMatches (Exception =
	// any thrown value, ExceptionInfo = an ex-info).
	evalSrc(t, e, `
		(deftest t-thrown
		  (is (thrown? Exception (throw (ex-info "boom" {}))))  ;; pass
		  (is (thrown? ExceptionInfo (throw (ex-info "b" {})))) ;; pass
		  (is (thrown? Exception 42)))                          ;; fail (no throw)`)
	summary := evalSrc(t, e, `(run-tests)`)
	for _, tc := range []struct {
		key  string
		want int64
	}{
		{"pass", 2},
		{"fail", 1},
		{"error", 0},
	} {
		if got := summaryCount(t, summary, tc.key); got != tc.want {
			t.Errorf("thrown?: :%s = %d, want %d", tc.key, got, tc.want)
		}
	}
}

func TestThrownWithMsgSupported(t *testing.T) {
	e := bootTest(t)
	discardOut(t)
	// (is (thrown-with-msg? Class #"re" body)) passes only when the body
	// throws a matching class AND the message matches the regex.
	evalSrc(t, e, `
		(deftest t-msg
		  (is (thrown-with-msg? Exception #"bo+m" (throw (ex-info "boom!" {})))) ;; pass
		  (is (thrown-with-msg? Exception #"xyz"  (throw (ex-info "boom!" {}))))) ;; fail`)
	summary := evalSrc(t, e, `(run-tests)`)
	if got := summaryCount(t, summary, "pass"); got != 1 {
		t.Errorf("thrown-with-msg?: :pass = %d, want 1", got)
	}
	if got := summaryCount(t, summary, "fail"); got != 1 {
		t.Errorf("thrown-with-msg?: :fail = %d, want 1", got)
	}
}

func TestAreExpandsToIsForms(t *testing.T) {
	e := bootTest(t)
	discardOut(t)
	// (are [argv] expr & rows) => a do of `is` forms, one per row of
	// (count argv) values.
	evalSrc(t, e, `(deftest t-are (are [x y] (= x y) 1 1 2 2 3 3))`)
	summary := evalSrc(t, e, `(run-tests)`)
	if got := summaryCount(t, summary, "pass"); got != 3 {
		t.Errorf("are: :pass = %d, want 3", got)
	}
	if got := summaryCount(t, summary, "fail"); got != 0 {
		t.Errorf("are: :fail = %d, want 0", got)
	}
}

// --- helpers ---------------------------------------------------------

func mustRead(t *testing.T, e *eval.Evaluator, src string) any {
	t.Helper()
	r := reader.New(strings.NewReader(src), reader.WithResolver(e.ReaderResolver()))
	form, err := r.ReadOne()
	if err != nil {
		t.Fatalf("read(%q): %v", src, err)
	}
	return form
}

func discardOut(t *testing.T) {
	t.Helper()
	old := corelib.Out
	corelib.Out = &bytes.Buffer{}
	t.Cleanup(func() { corelib.Out = old })
}
