// match_test.go — clojure.core.match behavior through the interpreter (ADR
// 0097). The 24 frozen behaviors are captured from real org.clojure/core.match
// 1.1.1 on the JVM (clojure -Sdeps '{:deps {org.clojure/core.match
// {:mvn/version "1.1.1"}}}'), the semantic oracle; each `want` below is that
// library's pr-str output, byte-identical. The compiler under test is the Go
// Maranget decision-tree primitive (-match-compile, match.go) behind the thin
// core/match.cljg macros.
package bri_test

import (
	"testing"
)

func TestCoreMatchOracle(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[clojure.core.match :refer [match matchv matchm match-let]])`)

	cases := []struct {
		name string
		expr string
		want string
	}{
		{"01-literal-two-col", `(match [1 2] [1 2] :a [_ _] :b)`, ":a"},
		{"02-wildcard-fallthrough", `(match [1 3] [1 2] :a [_ _] :b)`, ":b"},
		{"03-binding-capture", `(match [5] [x] x)`, "5"},
		{"04-literal-second-clause", `(match [2] [1] :one [2] :two :else :other)`, ":two"},
		{"05-nested-vector-bind", `(match [[1 2]] [[1 b]] b)`, "2"},
		{"06-map-pattern", `(match [{:a 1 :b 2}] [{:a a :b b}] (+ a b))`, "3"},
		{"07-or-miss", `(match [4] [(:or 1 2 3)] :low :else :other)`, ":other"},
		{"08-or-hit", `(match [2] [(:or 1 2 3)] :low :else :other)`, ":low"},
		{"09-guard-even", `(match [4] [(x :guard even?)] :even [_] :odd)`, ":even"},
		{"10-vector-rest", `(match [[1 2 3 4]] [[1 & r]] r)`, "[2 3 4]"},
		{"11-as-binding", `(match [[1 2]] [([1 2] :as v)] v)`, "[1 2]"},
		{"12-matchv", `(matchv ::objects [1 2] [1 2] :yes :else :no)`, ":yes"},
		{"13-match-let", `(match-let [x 1 y 2] [1 2] :a :else :b)`, ":a"},
		{"14-boolean-literal", `(match [true] [true] :t [false] :f)`, ":t"},
		{"15-nil-literal", `(match [nil] [nil] :nil [_] :some)`, ":nil"},
		{"16-string-literal", `(match ["hi"] ["hi"] :hi [_] :other)`, ":hi"},
		{"17-keyword-literal", `(match [:k] [:k] :key [_] :other)`, ":key"},
		{"18-two-col-second", `(match [3 4] [1 2] :a [3 4] :b [_ _] :c)`, ":b"},
		{"19-deep-nested", `(match [[1 [2 3]]] [[1 [a b]]] (+ a b))`, "5"},
		{"20-or-in-column", `(match [1 2] [(:or 1 2) 2] :a :else :b)`, ":a"},
		{"21-or-then-guard", `(match [6] [(:or 1 2)] :low [(x :guard (fn [n] (> n 5)))] :big :else :other)`, ":big"},
		{"22-guard-only", `(match [10] [(x :guard even?)] :evenx [_] :other)`, ":evenx"},
		{"23-vector-literal-tail", `(let [v [1 2 3]] (match [v] [[_ _ 3]] :three [_] :no))`, ":three"},
		{"24-empty-vector", `(match [[]] [[]] :empty [_] :ne)`, ":empty"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := evalString(t, d, `(pr-str `+tc.expr+`)`)
			if got != tc.want {
				t.Errorf("%s\n  expr: %s\n  got:  %s\n  want: %s", tc.name, tc.expr, got, tc.want)
			}
		})
	}
}

// TestCoreMatchNoClause confirms a total-failure match throws rather than
// returning a bogus value (the catch-all is required for exhaustiveness).
func TestCoreMatchNoClause(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[clojure.core.match :refer [match]])`)
	if s := evalString(t, d, `(pr-str (try (match [7] [1] :a [2] :b) (catch Throwable _ :threw)))`); s != ":threw" {
		t.Errorf("no-clause match did not throw, got %s", s)
	}
}
