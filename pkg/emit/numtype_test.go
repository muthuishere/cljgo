package emit

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/corelib"
	"github.com/muthuishere/cljgo/pkg/lang"
)

func s42build(t *testing.T, src string) (goSrc string, out string) {
	t.Helper()
	lang.RemoveNamespace(lang.NewSymbol("user"))
	oldOut := corelib.Out
	corelib.Out = io.Discard
	forms, err := CompileReader(strings.NewReader(src), "s42.clj")
	corelib.Out = oldOut
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	dir := t.TempDir()
	if err := WriteModule(dir, forms, Options{PrintLastValue: true}); err != nil {
		t.Fatalf("write: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	goSrc = string(b)
	bin := filepath.Join(dir, "s42"+ExeSuffix)
	if err := GoBuild(dir, bin); err != nil {
		t.Fatalf("build: %v\n---\n%s", err, goSrc)
	}
	o, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, o)
	}
	return goSrc, strings.TrimSpace(string(o))
}

func TestS42Scratch(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"loopsum", `(loop* [i 0 acc 0] (if (< i 1000) (recur (+ i 1) (+ acc i)) acc))`, "499500"},
		{"factorial", `(def fact (fn* fact [n] (if (< n 2) 1 (* n (fact (- n 1)))))) (fact 15)`, "1307674368000"},
		{"fib", `(def fib (fn* fib [n] (if (< n 2) n (+ (fib (- n 2)) (fib (- n 1)))))) (fib 20)`, "6765"},
		{"mixedvec", `(loop* [i 0 v []] (if (< i 3) (recur (inc i) (conj v (* i i))) v))`, "[0 1 4]"},
		{"floatstay", `(loop* [i 0 acc 0.0] (if (< i 3) (recur (inc i) (+ acc 1.5)) acc))`, "4.5"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src, out := s42build(t, c.src)
			if out != c.want {
				t.Fatalf("output = %q, want %q\n---GO---\n%s", out, c.want, src)
			}
			t.Logf("%s => %s", c.name, out)
			// surface whether unboxing fired
			if strings.Contains(src, "rt.IAdd") || strings.Contains(src, "rt.IMul") || strings.Contains(src, "int64 =") {
				t.Logf("  [unboxed int64 path present]")
			}
		})
	}
}
