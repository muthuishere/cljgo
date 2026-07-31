package corelib_test

// (deref ref timeout-ms timeout-val) — the 3-arity clojure.core form.
//
// It is the only portable way to put a DEADLINE on a blocking read. Without
// it, a caller waiting on a future or promise that never delivers has no way
// back, and ordinary Clojure fails with "wrong number of args (3) passed to:
// deref". future and Promise already implemented IBlockingDeref; only the
// builtin's entry point was missing. Reported by koine, 2026-07-31, where a
// subprocess reader needed a bounded wait so a hung child could not hang the
// program.

import (
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/eval"
	"github.com/muthuishere/cljgo/pkg/reader"
)

func evalStr(t *testing.T, e *eval.Evaluator, src string) any {
	t.Helper()
	r := reader.New(strings.NewReader(src), reader.WithResolver(e.ReaderResolver()))
	var last any
	for {
		form, err := r.ReadOne()
		if err != nil {
			break
		}
		if form == nil {
			break
		}
		v, err := e.EvalForm(form)
		if err != nil {
			t.Fatalf("eval %s: %v", src, err)
		}
		last = v
	}
	return last
}

func TestDerefWithTimeoutGivesUp(t *testing.T) {
	e := eval.New()
	// a promise nobody ever delivers: the 1-arity would block forever
	got := evalStr(t, e, `(let [p (promise)] (deref p 50 :timed-out))`)
	if got != evalStr(t, e, `:timed-out`) {
		t.Fatalf("deref on an undelivered promise = %v, want :timed-out", got)
	}
}

func TestDerefWithTimeoutReturnsTheValueWhenItArrives(t *testing.T) {
	e := eval.New()
	got := evalStr(t, e, `(let [p (promise)] (deliver p 42) (deref p 1000 :timed-out))`)
	if n, ok := got.(int64); !ok || n != 42 {
		t.Fatalf("deref on a delivered promise = %v (%T), want 42", got, got)
	}
}

func TestDerefWithTimeoutOnAFuture(t *testing.T) {
	e := eval.New()
	got := evalStr(t, e, `(deref (future 7) 2000 :timed-out)`)
	if n, ok := got.(int64); !ok || n != 7 {
		t.Fatalf("deref on a future = %v (%T), want 7", got, got)
	}
}

func TestDerefOneArityStillWorks(t *testing.T) {
	e := eval.New()
	got := evalStr(t, e, `(deref (atom 3))`)
	if n, ok := got.(int64); !ok || n != 3 {
		t.Fatalf("deref on an atom = %v (%T), want 3", got, got)
	}
}
