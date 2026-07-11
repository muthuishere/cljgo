// S1 driver: hand-constructs ASTs for four programs, emits Go source via the
// flattening emitter, writes each into its own generated module under gen/,
// runs `go build`, executes the binary, verifies output, and measures
// build latency / binary size / startup time.
//
// Run from this directory:  go run .
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"cljgo-spike-s1/ast"
	"cljgo-spike-s1/emit"
)

// ---- AST construction helpers ----------------------------------------------

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

// ---- test programs -----------------------------------------------------------

type program struct {
	name     string
	clj      string // for the provenance comment / RESULTS
	forms    []*ast.Node
	expected string
}

// oracle for the wrapped int64 factorial (Go signed overflow wraps, defined
// behavior; lang's * uses the same int64 multiply, so this is exact).
func wrappedFact(n int64) int64 {
	acc := int64(1)
	for i := int64(1); i <= n; i++ {
		acc *= i
	}
	return acc
}

func programs() []program {
	// (a) def + fn* + if + self-recursion through the Var
	fact := def("fact", fn([]string{"n"},
		iff(call("<", lo("n"), i64(2)),
			i64(1),
			call("*", lo("n"), inv(vr("fact"), call("-", lo("n"), i64(1)))))))

	// (b) let with nested if-as-expression in a binding init + do in body
	letIf := call("println",
		let([]ast.Binding{
			b("a", i64(1)),
			b("b", iff(call("<", lo("a"), i64(2)),
				call("+", lo("a"), i64(10)),
				call("+", lo("a"), i64(20)))),
		},
			do(call("println", c("in-let")),
				call("*", lo("a"), lo("b")))))

	// (c1) iterative loop/recur factorial, n=100000, constant stack
	loopFact := call("println",
		loop([]ast.Binding{b("i", i64(1)), b("acc", i64(1))},
			iff(call(">", lo("i"), i64(100000)),
				lo("acc"),
				recur(call("+", lo("i"), i64(1)),
					call("*", lo("acc"), lo("i"))))))

	// (c1b) sum 1..100000 — the wrapped factorial is 0 (2^64 | 100000!), so
	// this adds a non-trivial 100k-iteration value check: 5000050000.
	loopSum := call("println",
		loop([]ast.Binding{b("i", i64(1)), b("acc", i64(0))},
			iff(call(">", lo("i"), i64(100000)),
				lo("acc"),
				recur(call("+", lo("i"), i64(1)),
					call("+", lo("acc"), lo("i"))))))

	// (c2) nested loops: the INNER loop sits in non-tail position (inside the
	// outer recur's args), so its `continue`/`break` must be label-qualified
	// relative to the outer for-statement.
	inner := loop([]ast.Binding{b("j", i64(0)), b("s", i64(0))},
		iff(call("<", lo("j"), i64(10)),
			recur(call("+", lo("j"), i64(1)), call("+", lo("s"), i64(1))),
			lo("s")))
	nested := call("println",
		loop([]ast.Binding{b("i", i64(0)), b("total", i64(0))},
			iff(call("<", lo("i"), i64(100)),
				recur(call("+", lo("i"), i64(1)),
					call("+", lo("total"), inner)),
				lo("total"))))

	// (d) closure over a let local
	makeAdder := def("make-adder", fn([]string{"n"},
		let([]ast.Binding{b("base", call("+", lo("n"), i64(100)))},
			fn([]string{"x"}, call("+", lo("x"), lo("base"))))))

	return []program{
		{
			name: "fact-recursive",
			clj:  `(def fact (fn* [n] (if (< n 2) 1 (* n (fact (- n 1)))))) (println (fact 10))`,
			forms: []*ast.Node{
				fact,
				call("println", inv(vr("fact"), i64(10))),
			},
			expected: "3628800\n",
		},
		{
			name:     "let-nested-if",
			clj:      `(println (let [a 1 b (if (< a 2) (+ a 10) (+ a 20))] (do (println "in-let") (* a b))))`,
			forms:    []*ast.Node{letIf},
			expected: "in-let\n11\n",
		},
		{
			name: "loop-recur-100k",
			clj:  `(println (loop [i 1 acc 1] (if (> i 100000) acc (recur (+ i 1) (* acc i))))) + nested loop`,
			forms: []*ast.Node{
				loopFact,
				loopSum,
				nested,
			},
			expected: fmt.Sprintf("%d\n%d\n%d\n", wrappedFact(100000), 5000050000, 1000),
		},
		{
			name: "closure-capture",
			clj:  `(def make-adder (fn* [n] (let [base (+ n 100)] (fn* [x] (+ x base))))) (def add5 (make-adder 5)) ...`,
			forms: []*ast.Node{
				makeAdder,
				def("add5", inv(vr("make-adder"), i64(5))),
				call("println", inv(vr("add5"), i64(1))),
				call("println", inv(vr("add5"), i64(2))),
			},
			expected: "106\n107\n",
		},
	}
}

// ---- harness -------------------------------------------------------------------

func must(err error, ctx string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL %s: %v\n", ctx, err)
		os.Exit(1)
	}
}

func runGo(dir, gocache string, args ...string) (time.Duration, string, error) {
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOCACHE="+gocache, "GOFLAGS=-mod=mod")
	start := time.Now()
	out, err := cmd.CombinedOutput()
	return time.Since(start), string(out), err
}

func writeGenModule(spikeRoot, dir string, src []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	gomod := fmt.Sprintf(
		"module s1gen/%s\n\ngo 1.26\n\nrequire cljgo-spike-s1 v0.0.0\n\nreplace cljgo-spike-s1 => %s\n",
		filepath.Base(dir), spikeRoot)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "main.go"), src, 0o644)
}

func measureStartup(bin string, runs int) (min, mean time.Duration, err error) {
	var total time.Duration
	min = time.Hour
	for i := 0; i < runs; i++ {
		cmd := exec.Command(bin)
		cmd.Stdout = nil
		start := time.Now()
		if e := cmd.Run(); e != nil {
			return 0, 0, e
		}
		d := time.Since(start)
		total += d
		if d < min {
			min = d
		}
	}
	return min, total / time.Duration(runs), nil
}

func main() {
	spikeRoot, err := os.Getwd()
	must(err, "getwd")
	genRoot := filepath.Join(spikeRoot, "gen")

	gocache, err := os.MkdirTemp("", "s1-gocache-")
	must(err, "mktemp gocache")
	defer os.RemoveAll(gocache)
	fmt.Printf("== S1 emitter slice ==\nspike root: %s\nfresh GOCACHE: %s\n\n", spikeRoot, gocache)

	allOK := true
	for i, p := range programs() {
		fmt.Printf("--- program %s ---\n", p.name)
		formatted, raw, err := emit.EmitMain(p.forms)
		if err != nil {
			// format.Source failed: dump raw for debugging, fail the program.
			bad := filepath.Join(genRoot, p.name, "main.go.bad")
			_ = os.MkdirAll(filepath.Dir(bad), 0o755)
			_ = os.WriteFile(bad, raw, 0o644)
			fmt.Printf("FORMAT GATE FAILED: %v (raw at %s)\n", err, bad)
			allOK = false
			continue
		}
		dir := filepath.Join(genRoot, p.name)
		must(writeGenModule(spikeRoot, dir, formatted), "write gen module")

		// cold build: first program pays for compiling lang + stdlib deps
		// into the fresh cache; later programs show the shared-cache cost.
		coldDur, out, err := runGo(dir, gocache, "build", "-o", p.name, ".")
		if err != nil {
			fmt.Printf("BUILD FAILED (%v):\n%s\n", err, out)
			allOK = false
			continue
		}
		label := "warm-cache"
		if i == 0 {
			label = "cold-cache"
		}
		fmt.Printf("go build (%s): %v\n", label, coldDur.Round(time.Millisecond))

		// warm build: touch the source, rebuild with the now-populated cache.
		must(os.Chtimes(filepath.Join(dir, "main.go"), time.Now(), time.Now()), "touch")
		warmDur, out, err := runGo(dir, gocache, "build", "-o", p.name, ".")
		if err != nil {
			fmt.Printf("WARM BUILD FAILED (%v):\n%s\n", err, out)
			allOK = false
			continue
		}
		fmt.Printf("go build (rebuild after touch): %v\n", warmDur.Round(time.Millisecond))

		bin := filepath.Join(dir, p.name)
		st, err := os.Stat(bin)
		must(err, "stat binary")
		fmt.Printf("binary size: %.2f MB\n", float64(st.Size())/(1024*1024))

		// correctness
		cmd := exec.Command(bin)
		got, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("RUN FAILED (%v):\n%s\n", err, got)
			allOK = false
			continue
		}
		if string(got) == p.expected {
			fmt.Printf("output OK: %q\n", strings.TrimRight(string(got), "\n"))
		} else {
			fmt.Printf("OUTPUT MISMATCH:\n  want %q\n  got  %q\n", p.expected, string(got))
			allOK = false
			continue
		}

		minD, meanD, err := measureStartup(bin, 10)
		must(err, "startup measurement")
		fmt.Printf("startup+run wall (10 runs): min %v, mean %v\n\n", minD.Round(time.Microsecond), meanD.Round(time.Microsecond))
	}

	if allOK {
		fmt.Println("== ALL PROGRAMS PASSED ==")
	} else {
		fmt.Println("== FAILURES ==")
		os.Exit(1)
	}
}
