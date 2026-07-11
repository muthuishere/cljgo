// S5 driver: recur/loop emission edge cases S1 did not cover.
//
// For each case: hand-construct the post-macro-expansion AST, emit Go via the
// (extended) flattening emitter, `go build`, run — then run the SAME program
// (using the real macros where the AST hand-builds their expansion) through
// the real Clojure CLI and diff the outputs. Clojure is the oracle.
//
// Extra steps beyond the per-case table:
//   - case 1 is ALSO emitted with the capture fix disabled (S1's naive loop
//     emission) to demonstrate the Go-closures-capture-by-reference
//     divergence from Clojure's value-capture semantics.
//   - case 5's recur-across-try is a Clojure-only check: it must FAIL to
//     compile ("Cannot recur across try") — the analyzer owns that rejection,
//     the emitter never sees such an AST.
//
// Run from this directory:  go run .
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"cljgo-spike-s5/ast"
	"cljgo-spike-s5/emit"
)

// ---- AST construction helpers (same as S1) ---------------------------------

func c(v any) *ast.Node        { return &ast.Node{Op: ast.OpConst, Sub: &ast.Const{Value: v}} }
func i64(n int64) *ast.Node    { return c(n) }
func vr(name string) *ast.Node { return &ast.Node{Op: ast.OpVarRef, Sub: &ast.VarRef{Name: name}} }
func lo(name string) *ast.Node { return &ast.Node{Op: ast.OpLocal, Sub: &ast.Local{Name: name}} }

func inv(target *ast.Node, args ...*ast.Node) *ast.Node {
	return &ast.Node{Op: ast.OpInvoke, Sub: &ast.Invoke{Target: target, Args: args}}
}
func call(varName string, args ...*ast.Node) *ast.Node { return inv(vr(varName), args...) }

func iff(test, then, els *ast.Node) *ast.Node {
	return &ast.Node{Op: ast.OpIf, Sub: &ast.If{Test: test, Then: then, Else: els}}
}
func let(bindings []ast.Binding, body *ast.Node) *ast.Node {
	return &ast.Node{Op: ast.OpLet, Sub: &ast.Let{Bindings: bindings, Body: body}}
}
func loop(bindings []ast.Binding, body *ast.Node) *ast.Node {
	return &ast.Node{Op: ast.OpLoop, Sub: &ast.Loop{Bindings: bindings, Body: body}}
}
func recur(args ...*ast.Node) *ast.Node {
	return &ast.Node{Op: ast.OpRecur, Sub: &ast.Recur{Args: args}}
}
func def(name string, init *ast.Node) *ast.Node {
	return &ast.Node{Op: ast.OpDef, Sub: &ast.Def{Name: name, Init: init}}
}
func fn(params []string, body *ast.Node) *ast.Node {
	return &ast.Node{Op: ast.OpFn, Sub: &ast.Fn{Params: params, Body: body}}
}
func do(forms ...*ast.Node) *ast.Node {
	return &ast.Node{Op: ast.OpDo, Sub: &ast.Do{Forms: forms}}
}
func b(name string, init *ast.Node) ast.Binding { return ast.Binding{Name: name, Init: init} }

// ---- test programs ----------------------------------------------------------

type program struct {
	name     string
	clj      string // real-Clojure source (real macros where AST is hand-expanded)
	forms    []*ast.Node
	expected string // what BOTH sides should print
}

func programs() []program {
	// ---- case 1: closure capturing a LOOP local across iterations ----------
	// Each collected (fn [] i) must see ITS iteration's i (Clojure closures
	// capture the value; verified against the CLI: prints "0 1 2").
	case1Loop := loop(
		[]ast.Binding{b("i", i64(0)), b("fs", call("vector"))},
		iff(call("<", lo("i"), i64(3)),
			recur(call("+", lo("i"), i64(1)),
				call("conj", lo("fs"), fn(nil, lo("i")))),
			lo("fs")))
	case1 := let([]ast.Binding{b("fs", case1Loop)},
		call("println",
			inv(call("nth", lo("fs"), i64(0))),
			inv(call("nth", lo("fs"), i64(1))),
			inv(call("nth", lo("fs"), i64(2)))))

	// ---- case 1b: closure in loop-binding INIT position and in recur args -----
	// Init-position closure captures the INITIAL value and never sees any
	// rebinding (real Clojure prints 0); a closure created in the recur args
	// sees the iteration that created it (real Clojure prints 2, the last
	// iteration that recurred). Needs the binding-var/carrier split: the init
	// closure holds the binding var, recur reassigns only the carrier.
	case1bInit := loop(
		[]ast.Binding{b("i", i64(0)), b("f", fn(nil, lo("i")))},
		iff(call("<", lo("i"), i64(3)),
			recur(call("+", lo("i"), i64(1)), lo("f")),
			call("println", inv(lo("f")))))
	case1bArg := loop(
		[]ast.Binding{b("i", i64(0)), b("f", fn(nil, lo("i")))},
		iff(call("<", lo("i"), i64(3)),
			recur(call("+", lo("i"), i64(1)), fn(nil, lo("i"))),
			call("println", inv(lo("f")))))

	// ---- case 2: shadowing ---------------------------------------------------
	// loop local x shadows outer let x; let x inside the loop body shadows the
	// loop x; recur must rebind the LOOP x (not the let shadow), and the outer
	// x must be untouched after the loop.
	case2 := let([]ast.Binding{b("x", i64(5))},
		do(
			call("println",
				loop([]ast.Binding{b("x", i64(0))},
					iff(call("<", lo("x"), i64(3)),
						let([]ast.Binding{b("x", call("+", lo("x"), i64(100)))},
							recur(call("-", lo("x"), i64(99)))),
						lo("x")))),
			call("println", lo("x"))))

	// ---- case 3: recur under macro-expansion shapes ---------------------------
	// (a) `when` => (if test (do body...) nil): recur in tail of do under if.
	whenShape := call("println",
		loop([]ast.Binding{b("i", i64(0)), b("acc", i64(0))},
			iff(call(">=", lo("i"), i64(10)),
				lo("acc"),
				iff(call("<", lo("i"), i64(100)), // (when (< i 100) (recur ...))
					do(recur(call("+", lo("i"), i64(1)), call("+", lo("acc"), lo("i")))),
					nil))))
	// (b) `(and true (recur ...))` => (let [t true] (if t (recur ...) t)).
	andShape := call("println",
		loop([]ast.Binding{b("i", i64(0)), b("acc", i64(0))},
			iff(call("<", lo("i"), i64(10)),
				let([]ast.Binding{b("and__t", c(true))},
					iff(lo("and__t"),
						recur(call("+", lo("i"), i64(1)), call("+", lo("acc"), lo("i"))),
						lo("and__t"))),
				lo("acc"))))
	// (c) `(or false (recur ...))` => (let [t false] (if t t (recur ...))).
	orShape := call("println",
		loop([]ast.Binding{b("i", i64(0))},
			iff(call(">=", lo("i"), i64(5)),
				lo("i"),
				let([]ast.Binding{b("or__t", c(false))},
					iff(lo("or__t"), lo("or__t"),
						recur(call("+", lo("i"), i64(1))))))))

	// ---- case 4: simultaneous rebinding (swap) --------------------------------
	// (recur (+ n 1) b a): sequential assignment would collapse a and b to the
	// same value after the first iteration; temps must make it a true swap.
	case4 := loop([]ast.Binding{b("n", i64(0)), b("a", i64(1)), b("bb", i64(2))},
		iff(call("<", lo("n"), i64(5)),
			recur(call("+", lo("n"), i64(1)), lo("bb"), lo("a")),
			call("println", lo("a"), lo("bb"))))

	// ---- case 5: fn-level recur (no loop) --------------------------------------
	factIter := def("fact-iter", fn([]string{"n", "acc"},
		iff(call("<", lo("n"), i64(1)),
			lo("acc"),
			recur(call("-", lo("n"), i64(1)), call("*", lo("acc"), lo("n"))))))
	countUp := def("count-up", fn([]string{"i", "acc"}, // 100k iterations, constant stack
		iff(call(">", lo("i"), i64(100000)),
			lo("acc"),
			recur(call("+", lo("i"), i64(1)), call("+", lo("acc"), lo("i"))))))
	sumTo := def("sum-to", fn([]string{"n"}, // recur inside loop inside fn targets the LOOP
		loop([]ast.Binding{b("i", i64(0)), b("acc", i64(0))},
			iff(call(">", lo("i"), lo("n")),
				lo("acc"),
				recur(call("+", lo("i"), i64(1)), call("+", lo("acc"), lo("i")))))))

	// ---- case 5c: closure capturing a self-recurring FN's param ----------------
	// fn params are recur carriers too; a closure created in the body must see
	// ITS iteration's param value, same as loop locals.
	collect := def("collect", fn([]string{"i", "fs"},
		iff(call("<", lo("i"), i64(3)),
			recur(call("+", lo("i"), i64(1)),
				call("conj", lo("fs"), fn(nil, lo("i")))),
			lo("fs"))))
	case5c := let([]ast.Binding{b("fs", inv(vr("collect"), i64(0), call("vector")))},
		call("println",
			inv(call("nth", lo("fs"), i64(0))),
			inv(call("nth", lo("fs"), i64(1))),
			inv(call("nth", lo("fs"), i64(2)))))

	// ---- case 6: loop as expression / two loops in one expression --------------
	twoLoops := call("println",
		call("+",
			loop([]ast.Binding{b("i", i64(0))},
				iff(call("<", lo("i"), i64(5)), recur(call("+", lo("i"), i64(1))), lo("i"))),
			loop([]ast.Binding{b("j", i64(0))},
				iff(call("<", lo("j"), i64(7)), recur(call("+", lo("j"), i64(1))), lo("j")))))
	loopArg := call("println",
		call("*",
			loop([]ast.Binding{b("k", i64(1))},
				iff(call("<", lo("k"), i64(4)), recur(call("+", lo("k"), i64(1))), lo("k"))),
			i64(10)))

	return []program{
		{
			name: "case1-closure-over-loop-local",
			clj: `(let [fs (loop [i 0 fs (vector)]
           (if (< i 3)
             (recur (+ i 1) (conj fs (fn [] i)))
             fs))]
  (println ((nth fs 0)) ((nth fs 1)) ((nth fs 2))))`,
			forms:    []*ast.Node{case1},
			expected: "0 1 2\n",
		},
		{
			name: "case1b-closure-init-and-recur-arg",
			clj: `(loop [i 0 f (fn [] i)]
  (if (< i 3)
    (recur (+ i 1) f)
    (println (f))))
(loop [i 0 f (fn [] i)]
  (if (< i 3)
    (recur (+ i 1) (fn [] i))
    (println (f))))`,
			forms:    []*ast.Node{case1bInit, case1bArg},
			expected: "0\n2\n",
		},
		{
			name: "case2-shadowing",
			clj: `(let [x 5]
  (println (loop [x 0]
             (if (< x 3)
               (let [x (+ x 100)]
                 (recur (- x 99)))
               x)))
  (println x))`,
			forms:    []*ast.Node{case2},
			expected: "3\n5\n",
		},
		{
			name: "case3-macro-shapes",
			clj: `(println (loop [i 0 acc 0]
           (if (>= i 10)
             acc
             (when (< i 100)
               (recur (+ i 1) (+ acc i))))))
(println (loop [i 0 acc 0]
           (if (< i 10)
             (and true (recur (+ i 1) (+ acc i)))
             acc)))
(println (loop [i 0]
           (if (>= i 5)
             i
             (or false (recur (+ i 1))))))`,
			forms:    []*ast.Node{whenShape, andShape, orShape},
			expected: "45\n45\n5\n",
		},
		{
			name: "case4-simultaneous-swap",
			clj: `(loop [n 0 a 1 bb 2]
  (if (< n 5)
    (recur (+ n 1) bb a)
    (println a bb)))`,
			forms:    []*ast.Node{case4},
			expected: "2 1\n",
		},
		{
			name: "case5-fn-level-recur",
			clj: `(def fact-iter (fn* [n acc] (if (< n 1) acc (recur (- n 1) (* acc n)))))
(println (fact-iter 10 1))
(def count-up (fn* [i acc] (if (> i 100000) acc (recur (+ i 1) (+ acc i)))))
(println (count-up 1 0))
(def sum-to (fn* [n] (loop [i 0 acc 0] (if (> i n) acc (recur (+ i 1) (+ acc i))))))
(println (sum-to 10))`,
			forms: []*ast.Node{
				factIter, call("println", inv(vr("fact-iter"), i64(10), i64(1))),
				countUp, call("println", inv(vr("count-up"), i64(1), i64(0))),
				sumTo, call("println", inv(vr("sum-to"), i64(10))),
			},
			expected: "3628800\n5000050000\n55\n",
		},
		{
			name: "case5c-closure-over-fn-param",
			clj: `(def collect (fn* [i fs]
  (if (< i 3)
    (recur (+ i 1) (conj fs (fn [] i)))
    fs)))
(let [fs (collect 0 (vector))]
  (println ((nth fs 0)) ((nth fs 1)) ((nth fs 2))))`,
			forms:    []*ast.Node{collect, case5c},
			expected: "0 1 2\n",
		},
		{
			name: "case6-loop-as-expression",
			clj: `(println (+ (loop [i 0] (if (< i 5) (recur (+ i 1)) i))
            (loop [j 0] (if (< j 7) (recur (+ j 1)) j))))
(println (* (loop [k 1] (if (< k 4) (recur (+ k 1)) k)) 10))`,
			forms:    []*ast.Node{twoLoops, loopArg},
			expected: "12\n40\n",
		},
	}
}

// recur across try: MUST be rejected by the real Clojure compiler; in cljgo
// the ANALYZER owns this rejection (the emitter never sees such an AST).
const recurAcrossTry = `(loop [i 0] (try (recur (+ i 1)) (catch Exception e :caught)))`

// ---- harness ------------------------------------------------------------------

func must(err error, ctx string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL %s: %v\n", ctx, err)
		os.Exit(1)
	}
}

func writeGenModule(spikeRoot, dir string, src []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	gomod := fmt.Sprintf(
		"module s5gen/%s\n\ngo 1.26\n\nrequire cljgo-spike-s5 v0.0.0\n\nreplace cljgo-spike-s5 => %s\n",
		filepath.Base(dir), spikeRoot)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "main.go"), src, 0o644)
}

// emitBuildRun emits forms (with/without the capture fix), builds, runs, and
// returns stdout of the emitted binary.
func emitBuildRun(spikeRoot, genRoot, name string, forms []*ast.Node, noCaptureFix bool) (string, error) {
	formatted, raw, err := emit.EmitMainOpt(forms, noCaptureFix)
	if err != nil {
		bad := filepath.Join(genRoot, name, "main.go.bad")
		_ = os.MkdirAll(filepath.Dir(bad), 0o755)
		_ = os.WriteFile(bad, raw, 0o644)
		return "", fmt.Errorf("format gate failed: %v (raw at %s)", err, bad)
	}
	dir := filepath.Join(genRoot, name)
	if err := writeGenModule(spikeRoot, dir, formatted); err != nil {
		return "", err
	}
	build := exec.Command("go", "build", "-o", name, ".")
	build.Dir = dir
	build.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	if out, err := build.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build failed: %v\n%s", err, out)
	}
	run := exec.Command(filepath.Join(dir, name))
	out, err := run.Output()
	if err != nil {
		return "", fmt.Errorf("run failed: %v", err)
	}
	return string(out), nil
}

// runClojure writes src to <genRoot>/clj/<name>.clj and runs it via the real
// Clojure CLI (script mode: does NOT echo top-level form values, unlike -e).
func runClojure(genRoot, name, src string) (stdout, stderr string, err error) {
	dir := filepath.Join(genRoot, "clj")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	path := filepath.Join(dir, name+".clj")
	if err := os.WriteFile(path, []byte(src+"\n"), 0o644); err != nil {
		return "", "", err
	}
	cmd := exec.Command("clojure", "-M", path)
	cmd.Dir = dir
	var out, serr strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &serr
	err = cmd.Run()
	return out.String(), serr.String(), err
}

func main() {
	spikeRoot, err := os.Getwd()
	must(err, "getwd")
	genRoot := filepath.Join(spikeRoot, "gen")
	fmt.Printf("== S5 recur/loop edge cases ==\nspike root: %s\n\n", spikeRoot)

	allOK := true
	for _, p := range programs() {
		fmt.Printf("--- %s ---\n", p.name)
		gotGo, err := emitBuildRun(spikeRoot, genRoot, p.name, p.forms, false)
		if err != nil {
			fmt.Printf("EMIT/BUILD/RUN FAILED: %v\n\n", err)
			allOK = false
			continue
		}
		gotClj, cljErr, err := runClojure(genRoot, p.name, p.clj)
		if err != nil {
			fmt.Printf("CLOJURE ORACLE FAILED: %v\n%s\n\n", err, cljErr)
			allOK = false
			continue
		}
		switch {
		case gotGo != gotClj:
			fmt.Printf("DIVERGENCE:\n  clojure %q\n  emitted %q\n\n", gotClj, gotGo)
			allOK = false
		case gotGo != p.expected:
			fmt.Printf("BOTH MATCH BUT DIFFER FROM EXPECTED (bad oracle program?):\n  both %q, expected %q\n\n", gotGo, p.expected)
			allOK = false
		default:
			fmt.Printf("OK: emitted == clojure == expected %q\n\n", strings.TrimRight(gotGo, "\n"))
		}
	}

	// ---- case 1 divergence demo: capture fix OFF ------------------------------
	fmt.Println("--- case1 with capture fix DISABLED (S1's naive loop emission) ---")
	p1 := programs()[0]
	gotNaive, err := emitBuildRun(spikeRoot, genRoot, p1.name+"-nofix", p1.forms, true)
	if err != nil {
		fmt.Printf("EMIT/BUILD/RUN FAILED: %v\n", err)
		allOK = false
	} else if gotNaive == p1.expected {
		fmt.Printf("UNEXPECTED: naive emission matches Clojure (%q) — divergence hypothesis wrong\n\n", gotNaive)
		allOK = false
	} else {
		fmt.Printf("CONFIRMED DIVERGENCE (fix is load-bearing): naive %q vs clojure %q\n\n",
			strings.TrimRight(gotNaive, "\n"), strings.TrimRight(p1.expected, "\n"))
	}

	// ---- case 5b: recur across try must be REJECTED by Clojure ----------------
	fmt.Println("--- case5 recur-across-try (Clojure must reject; analyzer owns this) ---")
	_, cljErr, err := runClojure(genRoot, "case5-recur-across-try", recurAcrossTry)
	if err == nil {
		fmt.Println("UNEXPECTED: Clojure ACCEPTED recur across try")
		allOK = false
	} else if strings.Contains(cljErr, "Cannot recur across try") {
		fmt.Printf("OK: rejected at compile time: %q\n\n", firstLine(cljErr)+" / Cannot recur across try")
	} else {
		fmt.Printf("rejected, but with unexpected message:\n%s\n\n", cljErr)
		allOK = false
	}

	if allOK {
		fmt.Println("== ALL CASES PASSED ==")
	} else {
		fmt.Println("== FAILURES ==")
		os.Exit(1)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
