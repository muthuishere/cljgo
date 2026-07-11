// Spike S3: REPL deps self-rebuild UX.
//
// Fake REPL over stdin. `:add-dep <module>` does:
//   go get <module> -> generate zz_registry.go (go/types walk, funcs only)
//   -> go build -o s3repl.new . -> rename -> syscall.Exec into the new binary.
// `:call <alias>/<Func> [string args...]` invokes via the registry + reflect.
//
// Run from the spike directory (the project go.mod must be in cwd).
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// registry is populated by the generated zz_registry.go (init funcs).
var (
	registry = map[string]any{}      // "pkg/path.Name" -> func value
	aliases  = map[string]string{}   // "uuid" -> "github.com/google/uuid"
	binName  = "s3repl"
	depsFile = "deps.list"
)

func register(pkgPath, name string, v any) { registry[pkgPath+"."+name] = v }
func registerPkg(alias, pkgPath string)    { aliases[alias] = pkgPath }

func main() {
	rebooted := len(os.Args) > 1 && os.Args[1] == "-rebooted"
	if rebooted {
		fmt.Printf("rebuilt, %d symbols available\n", len(registry))
		if t0s := os.Getenv("S3_T0"); t0s != "" {
			if ns, err := strconv.ParseInt(t0s, 10, 64); err == nil {
				fmt.Printf(";; :add-dep cycle wall-clock: %.2fs\n",
					time.Since(time.Unix(0, ns)).Seconds())
			}
			os.Unsetenv("S3_T0")
		}
	} else {
		fmt.Printf("s3repl fake REPL (%d symbols). Commands: :add-dep <mod> | :call <alias>/<Func> [args] | :syms | :quit\n", len(registry))
	}

	for {
		fmt.Print("user=> ")
		line, ok := readLine()
		if !ok {
			fmt.Println()
			return
		}
		line = strings.TrimSpace(line)
		switch {
		case line == "":
		case line == ":quit":
			return
		case line == ":syms":
			for k := range registry {
				fmt.Println(" ", k)
			}
			fmt.Printf(";; %d symbols, %d packages\n", len(registry), len(aliases))
		case strings.HasPrefix(line, ":add-dep "):
			if err := addDep(strings.TrimSpace(strings.TrimPrefix(line, ":add-dep "))); err != nil {
				fmt.Println("error:", err)
			}
			// on success addDep never returns (exec)
		case strings.HasPrefix(line, ":call "):
			if err := call(strings.Fields(strings.TrimPrefix(line, ":call "))); err != nil {
				fmt.Println("error:", err)
			}
		default:
			fmt.Println(";; unknown input (this is a fake REPL):", line)
		}
	}
}

// readLine reads stdin one byte at a time. Deliberate: a buffered reader
// would slurp lines that belong to the post-exec process and lose them.
func readLine() (string, bool) {
	var b []byte
	buf := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				return string(b), true
			}
			b = append(b, buf[0])
		}
		if err != nil {
			return string(b), len(b) > 0
		}
	}
}

func addDep(module string) error {
	t0 := time.Now()
	if module == "" {
		return fmt.Errorf("usage: :add-dep <module>")
	}
	if _, err := os.Stat("go.mod"); err != nil {
		return fmt.Errorf("run from the project dir (go.mod not found): %w", err)
	}

	spec := module
	if !strings.Contains(spec, "@") {
		spec += "@latest"
	}
	t := time.Now()
	if out, err := exec.Command("go", "get", spec).CombinedOutput(); err != nil {
		return fmt.Errorf("go get: %w\n%s", err, out)
	}
	fmt.Printf(";; go get %s: %.2fs\n", spec, time.Since(t).Seconds())

	mods, err := appendDep(module)
	if err != nil {
		return err
	}

	t = time.Now()
	n, err := generateRegistry(mods, "zz_registry.go")
	if err != nil {
		return fmt.Errorf("genpkg: %w", err)
	}
	fmt.Printf(";; registry regen (%d syms, %d pkgs): %.2fs\n", n, len(mods), time.Since(t).Seconds())

	t = time.Now()
	if out, err := exec.Command("go", "build", "-o", binName+".new", ".").CombinedOutput(); err != nil {
		return fmt.Errorf("go build: %w\n%s", err, out)
	}
	fmt.Printf(";; go build: %.2fs\n", time.Since(t).Seconds())
	if err := os.Rename(binName+".new", binName); err != nil {
		return err
	}

	exe, err := filepath.Abs(binName)
	if err != nil {
		return err
	}
	env := append(os.Environ(), fmt.Sprintf("S3_T0=%d", t0.UnixNano()))
	fmt.Println(";; exec", exe)
	if err := syscall.Exec(exe, []string{exe, "-rebooted"}, env); err != nil {
		return fmt.Errorf("exec: %w", err)
	}
	panic("unreachable: exec replaces the process")
}

// appendDep records module in deps.list (dedup) and returns the full list,
// so regeneration always covers every dep added so far.
func appendDep(module string) ([]string, error) {
	mods := []string{}
	if b, err := os.ReadFile(depsFile); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if l = strings.TrimSpace(l); l != "" {
				mods = append(mods, l)
			}
		}
	}
	base := strings.SplitN(module, "@", 2)[0]
	for _, m := range mods {
		if m == base {
			return mods, nil
		}
	}
	mods = append(mods, base)
	return mods, os.WriteFile(depsFile, []byte(strings.Join(mods, "\n")+"\n"), 0o644)
}

func call(args []string) error {
	if len(args) < 1 || !strings.Contains(args[0], "/") {
		return fmt.Errorf("usage: :call <alias>/<Func> [string args]")
	}
	i := strings.LastIndex(args[0], "/")
	alias, name := args[0][:i], args[0][i+1:]
	pkg, ok := aliases[alias]
	if !ok {
		return fmt.Errorf("unknown package alias %q (did you :add-dep it?)", alias)
	}
	fn, ok := registry[pkg+"."+name]
	if !ok {
		return fmt.Errorf("no symbol %s.%s in registry", pkg, name)
	}
	fv := reflect.ValueOf(fn)
	ft := fv.Type()
	if ft.Kind() != reflect.Func {
		fmt.Printf("%v\n", fn) // const/var, just print
		return nil
	}
	rest := args[1:]
	if ft.NumIn() != len(rest) {
		return fmt.Errorf("%s takes %d args, got %d (spike supports string args only)", name, ft.NumIn(), len(rest))
	}
	in := make([]reflect.Value, len(rest))
	for j, a := range rest {
		if ft.In(j).Kind() != reflect.String {
			return fmt.Errorf("arg %d: spike only coerces string args (want %s)", j, ft.In(j))
		}
		in[j] = reflect.ValueOf(a).Convert(ft.In(j))
	}
	out := fv.Call(in)
	for _, v := range out {
		fmt.Printf("%v\n", v.Interface())
	}
	return nil
}
