// bri.go — the project CLI surface (ADR 0041, ADR 0047, openspec
// app-framework T0/T1):
//
//	cljgo new <name>   generate a project from a template (default: lib)
//	cljgo dev          run a bri app: server + nREPL attached + the banner
//	cljgo test         load src/, run every test under test/
//	cljgo config       print the resolved config map, layer per key
//	cljgo routes       print the routes and the effective middleware stack
//
// `cljgo new` belongs to the LANGUAGE and knows only about TEMPLATES
// (ADR 0047): it walks a template FS, renames the app, and writes. It
// has no idea what bri is — `web` is one of three built-ins, and the
// default is `lib`. `dev`/`config`/`routes` below ARE bri-shaped and
// say so.
//
// `cljgo new` is a generator, not a container: it writes plain files
// the user owns; nothing scans them (a bri app calls bri, visibly).
// The generated sources live in templates/ as REAL FILES (see that
// package's doc) — never as string literals here.
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/nrepl"
	"github.com/muthuishere/cljgo/pkg/repl"
	"github.com/muthuishere/cljgo/templates"
)

const appMain = "src/app/main.cljg"

// --- cljgo new ---------------------------------------------------------------

func runNew(args []string) int {
	flags := flag.NewFlagSet("new", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	template := flags.String("template", templates.DefaultTemplate,
		"built-in template name ("+templates.BuiltinNames()+"), or a path to a template directory")
	flags.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cljgo new [--template <name|path>] <name>")
		fmt.Fprintln(os.Stderr, "\nbuilt-in templates:")
		for _, b := range templates.Builtins {
			fmt.Fprintf(os.Stderr, "  %-4s %s\n", b.Name, b.Summary)
		}
		fmt.Fprintln(os.Stderr, "\n--template also takes a path to a local template directory.")
		fmt.Fprintln(os.Stderr)
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return 2
	}
	name := flags.Arg(0)
	if !validAppName(name) {
		fmt.Fprintf(os.Stderr, "cljgo new: %q is not a valid app name (lowercase letters, digits, - _)\n", name)
		return 2
	}
	if entries, err := os.ReadDir(name); err == nil && len(entries) > 0 {
		fmt.Fprintf(os.Stderr, "cljgo new: directory %s already exists and is not empty\n", name)
		return 1
	}

	src, err := templateFS(*template)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cljgo new:", err)
		return 2
	}
	files, err := renderTemplate(src, name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cljgo new:", err)
		return 1
	}
	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "cljgo new: template %s is empty\n", *template)
		return 1
	}

	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths) // map iteration is random; the listing is not

	for _, p := range paths {
		full := filepath.Join(name, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "cljgo new:", err)
			return 1
		}
		if err := os.WriteFile(full, []byte(files[p]), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "cljgo new:", err)
			return 1
		}
	}

	fmt.Printf("created %s/\n", name)
	for _, p := range paths {
		fmt.Printf("  %s\n", p)
	}
	fmt.Printf("\nnext:\n  cd %s\n", name)
	// The "next" commands are TEMPLATE metadata (they live beside the
	// template), not knowledge this command holds. A --template path is
	// somebody else's directory: only the generic step is honest there.
	next := []string{"cljgo test    # the generated test"}
	if b, ok := templates.LookupBuiltin(*template); ok {
		next = b.Next
	}
	for _, line := range next {
		fmt.Printf("  %s\n", line)
	}
	return 0
}

// templateFS resolves --template: a built-in name (embedded, the
// zero-install default) or a local directory path. Git URLs are not
// supported yet — fetching one needs machinery we do not have, and a
// half-done fetch is worse than an honest error (follow-up: openspec
// app-framework task 0.3).
func templateFS(name string) (fs.FS, error) {
	if strings.Contains(name, "://") || strings.HasPrefix(name, "git@") {
		return nil, fmt.Errorf("--template does not take a git URL yet — clone it and pass the path")
	}
	if !strings.ContainsAny(name, `/\.`) {
		unknown := fmt.Errorf("no built-in template %q (built-in: %s — or pass a path to a template directory)",
			name, templates.BuiltinNames())
		if _, ok := templates.LookupBuiltin(name); !ok {
			return nil, unknown
		}
		sub, err := fs.Sub(templates.FS, name)
		if err != nil {
			return nil, unknown
		}
		if _, err := fs.Stat(sub, "."); err != nil {
			return nil, unknown
		}
		return sub, nil
	}
	info, err := os.Stat(name)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("--template %s: not a directory", name)
	}
	return os.DirFS(name), nil
}

// renderTemplate reads every file of a template FS and renames the app:
// templates.DefaultName → the requested name, in file CONTENTS and in
// PATH names. That one substitution is the whole mechanism — no
// template language, nothing to escape, and the template files stay
// runnable source in place (which is what lets CI run them).
func renderTemplate(src fs.FS, name string) (map[string]string, error) {
	files := map[string]string{}
	err := fs.WalkDir(src, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, err := fs.ReadFile(src, path)
		if err != nil {
			return err
		}
		out := strings.ReplaceAll(path, templates.DefaultName, name)
		files[out] = strings.ReplaceAll(string(body), templates.DefaultName, name)
		return nil
	})
	return files, err
}

func validAppName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

// --- cljgo dev ------------------------------------------------------------------

// runDev is the T0 dev loop (task 0.2): the app served through the
// interpreter with an nREPL attached — the REPL is the reload story
// (nothing is watched; re-def the var). BRI_DEV=1 turns on bri's
// loud dev behaviors (plain-fn route warnings, bare-Result messages).
// Ctrl-C/SIGTERM = graceful drain (bri.http/serve owns shutdown).
func runDev(args []string) int {
	fs := flag.NewFlagSet("dev", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	nreplPort := fs.Int("nrepl-port", 0, "nREPL port (0 = an ephemeral port)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cljgo dev [--nrepl-port N] (run from the app directory)")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if _, err := os.Stat(appMain); err != nil {
		fmt.Fprintf(os.Stderr, "cljgo dev: no %s here — run from an app directory (create one with `cljgo new <name>`)\n", appMain)
		return 1
	}

	os.Setenv("BRI_DEV", "1")
	if os.Getenv("APP_PROFILE") == "" {
		os.Setenv("APP_PROFILE", "dev")
	}

	// nREPL first: editors connect while the server runs. Same process,
	// same global namespace/var registry — a re-def through this port
	// changes the NEXT request on the live server (the S20 claim).
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *nreplPort))
	if err != nil {
		fmt.Fprintln(os.Stderr, "cljgo dev:", err)
		return 1
	}
	defer ln.Close()
	actual := ln.Addr().(*net.TCPAddr).Port
	if err := os.WriteFile(portFileName, []byte(strconv.Itoa(actual)), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not write %s: %v\n", portFileName, err)
	} else {
		defer os.Remove(portFileName)
	}
	go nrepl.NewServer().Serve(ln)

	appName := filepath.Base(mustGetwd())
	fmt.Printf(`bri dev — %s
  profile : %s
  nREPL   : nrepl://127.0.0.1:%d (.nrepl-port written)
  reload  : re-(def) a handler var at the REPL — routes hold #'vars

`, appName, os.Getenv("APP_PROFILE"), actual)

	// ADR 0048: resolve declared dependencies (if the project is locked) and
	// publish their roots before loading any namespace, so `cljgo dev` resolves
	// deps the same way `cljgo run`/`build` do — one resolver, no divergence.
	if err := resolveRunDeps(appMain); err != nil {
		fmt.Fprintln(os.Stderr, "cljgo dev:", err)
		return 1
	}
	d := repl.New(nil, os.Stdout, os.Stderr)
	if code := evalAppFile(d, appMain); code != 0 {
		return code
	}
	if _, err := d.EvalString("(app.main/-main)", "cljgo-dev"); err != nil {
		fmt.Fprintln(os.Stderr, "cljgo dev:", err)
		return 1
	}
	return 0
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "app"
	}
	return wd
}

// evalAppFile loads one source file through the driver (same load
// frame as `cljgo run`).
func evalAppFile(d *repl.Driver, path string) int {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer f.Close()
	if _, err := d.EvalReader(f, path); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

// --- cljgo test -------------------------------------------------------------------

// runTest loads every namespace under src/ (skipping ones a require
// already pulled in), then every *_test file under test/, then runs
// clojure.test over all of it. APP_PROFILE defaults to test — the
// no-I/O-at-load contract is what makes requiring app.main safe here.
func runTest(args []string) int {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cljgo test (run from the app directory)")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if _, err := os.Stat("test"); err != nil {
		fmt.Fprintln(os.Stderr, "cljgo test: no test/ directory here")
		return 1
	}
	if os.Getenv("APP_PROFILE") == "" {
		os.Setenv("APP_PROFILE", "test")
	}

	// ADR 0048: resolve declared dependencies (if locked) and publish their
	// roots before loading src/ and test/, so `cljgo test` sees dep namespaces
	// the same way the other entry points do.
	if err := resolveRunDeps("."); err != nil {
		fmt.Fprintln(os.Stderr, "cljgo test:", err)
		return 1
	}
	d := repl.New(nil, os.Stdout, os.Stderr)
	for _, root := range []string{"src", "test"} {
		files, err := sourceFiles(root)
		if err != nil {
			fmt.Fprintln(os.Stderr, "cljgo test:", err)
			return 1
		}
		for _, path := range files {
			if nsAlreadyLoaded(path, root) {
				continue
			}
			if code := evalAppFile(d, path); code != 0 {
				return code
			}
		}
	}
	res, err := d.EvalString("(clojure.test/successful? (clojure.test/run-all-tests))", "cljgo-test")
	if err != nil {
		fmt.Fprintln(os.Stderr, "cljgo test:", err)
		return 1
	}
	if res != true {
		return 1
	}
	return 0
}

// sourceFiles lists .clj/.cljg files under root, sorted.
func sourceFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && (strings.HasSuffix(path, ".clj") || strings.HasSuffix(path, ".cljg")) {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

// nsAlreadyLoaded reports whether path's conventional namespace (its
// path relative to root, _ → -, / → .) already exists — a require from
// an earlier file loaded it; evaluating the file again would re-run it.
func nsAlreadyLoaded(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	stem := strings.TrimSuffix(strings.TrimSuffix(rel, ".cljg"), ".clj")
	nsName := strings.ReplaceAll(strings.ReplaceAll(filepath.ToSlash(stem), "/", "."), "_", "-")
	return lang.FindNamespace(lang.NewSymbol(nsName)) != nil
}

// --- cljgo config -----------------------------------------------------------------

// runConfig prints the resolved config map with each key's winning
// layer (task 1.6 — "2 a.m. debugging").
func runConfig(args []string) int {
	if _, err := os.Stat("conf.edn"); err != nil {
		fmt.Fprintln(os.Stderr, "cljgo config: no conf.edn here — run from an app directory")
		return 1
	}
	d := repl.New(nil, os.Stdout, os.Stderr)
	out, err := d.EvalString("(do (require 'bri.config) (bri.config/explain))", "cljgo-config")
	if err != nil {
		fmt.Fprintln(os.Stderr, "cljgo config:", err)
		return 1
	}
	s, _ := out.(string)
	fmt.Print(s)
	return 0
}

// --- cljgo routes -----------------------------------------------------------------

// runRoutes prints every route and the effective middleware stack
// (task 1.3 — the defaults are inspectable DATA).
func runRoutes(args []string) int {
	if _, err := os.Stat(appMain); err != nil {
		fmt.Fprintf(os.Stderr, "cljgo routes: no %s here — run from an app directory\n", appMain)
		return 1
	}
	d := repl.New(nil, os.Stdout, os.Stderr)
	if code := evalAppFile(d, appMain); code != 0 {
		return code
	}
	out, err := d.EvalString("(bri.http/describe app.main/routes {})", "cljgo-routes")
	if err != nil {
		fmt.Fprintln(os.Stderr, "cljgo routes:", err)
		return 1
	}
	s, _ := out.(string)
	fmt.Print(s)
	return 0
}
