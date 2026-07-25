// jobs_test.go — the bri.core.jobs behavior suite (ADR 0094). No JVM oracle (a
// cljgo fundamental). Covers the worker pool draining every submitted job, error
// capture from a throwing handler, and — crucially — that a USER backend
// implementing the `Queue` protocol works through the same public fns (the
// "interface for all" contract). Alias `jq` (vars are process-global).
package bri_test

import "testing"

func TestBriCoreJobs(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[bri.core.jobs :as jq])`)

	// a worker pool runs every submitted job; drain blocks until all settle; a
	// throwing handler is captured (not fatal) and the good jobs still all run
	eval(t, d, `(def done (atom []))`)
	eval(t, d, `(def q (jq/local {:add  (fn [{:keys [x]}] (swap! done conj x))
	                              :boom (fn [_] (throw (ex-info "nope" {})))}
	                             {:workers 3}))`)
	eval(t, d, `(doseq [i (range 50)] (jq/submit q :add {:x i}))`)
	eval(t, d, `(jq/submit q :boom {})`)
	eval(t, d, `(jq/drain q)`)
	if got := evalString(t, d, `(str (count @done))`); got != "50" {
		t.Errorf("ran %s jobs, want 50 (drain must wait for all)", got)
	}
	if got := evalString(t, d, `(str (reduce + @done))`); got != "1225" {
		t.Errorf("job sum = %s, want 1225 (0..49)", got)
	}
	if got := evalString(t, d, `(str (count (jq/errors q)))`); got != "1" {
		t.Errorf("captured errors = %s, want 1 (the throwing handler)", got)
	}
	eval(t, d, `(jq/stop q)`)

	// the interface: a USER backend implementing Queue works via the same fns
	eval(t, d, `(def n (atom 0))
	            (def uq (reify jq/Queue
	                      (-submit [_ t p] (swap! n inc))
	                      (-drain [_] nil) (-stop [_] nil) (-errors [_] [])))`)
	eval(t, d, `(jq/submit uq :x {}) (jq/submit uq :y {})`)
	if got := evalString(t, d, `(str @n)`); got != "2" {
		t.Errorf("user Queue impl submits = %s, want 2 (dispatched through submit)", got)
	}
}
