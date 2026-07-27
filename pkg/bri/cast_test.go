// cast_test.go — the cljg.data.cast cast/cast! input-gate suite (ADR 0072 /
// app-framework task 2.2). No JVM oracle. cast validates + coerces untrusted
// input against a schema, DROPS undeclared keys (mass assignment off the path),
// and returns ok/err; cast! throws. Pure — needs no DB connection, so it loads
// without the (opt-in) db shims. Alias `dd` (vars are process-global).
package bri_test

import "testing"

func TestBriCoreDataCast(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[cljg.data.cast :as dd])`)
	eval(t, d, `(def schema {:title :string :views :int :done :bool})`)

	// good input: strings coerced to their declared types
	eval(t, d, `(def clean (unwrap (dd/cast {:title "Hi" :views "5" :done "true" :admin "1"} schema)))`)
	if got := evalString(t, d, `(:title clean)`); got != "Hi" {
		t.Errorf("cast :title = %q, want Hi", got)
	}
	if got := evalString(t, d, `(str (:views clean))`); got != "5" {
		t.Errorf("cast :views = %q, want 5 (coerced from string)", got)
	}
	if got := eval(t, d, `(:done clean)`); got != true {
		t.Errorf("cast :done = %v, want true (coerced from \"true\")", got)
	}
	// mass assignment: the undeclared :admin key is dropped, never reachable
	if got := eval(t, d, `(contains? clean :admin)`); got != false {
		t.Errorf("cast kept undeclared :admin (%v) — mass-assignment hole", got)
	}

	// bad input → (err {field message})
	if got := eval(t, d, `(err? (dd/cast {:views "notnum"} schema))`); got != true {
		t.Errorf("cast of bad input is not err (%v)", got)
	}

	// cast! returns the clean row on success, throws with :errors on failure
	if got := evalString(t, d, `(str (:views (dd/cast! {:views 3} schema)))`); got != "3" {
		t.Errorf("cast! good :views = %q, want 3", got)
	}
	if got := evalString(t, d, `(:views (:errors (try (dd/cast! {:views "x"} schema)
	                                                    (catch Throwable e (ex-data e)))))`); got != "expected an integer" {
		t.Errorf("cast! error message = %q, want \"expected an integer\"", got)
	}
}
