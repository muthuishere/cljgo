// suite.go — `cljgo suite`: run the external jank clojure-test-suite against
// cljgo and emit a per-file pass/fail/error/skipped scoreboard (ADR 0022,
// design/08 Batch 0 / Track 3 T1). The suite is NOT vendored; its path is a
// flag/env (default ../clojure-test-suite), never a hardcoded absolute path.
//
// Method: each *.cljc test file is loaded into a FRESH evaluator (so state
// never leaks between files), form by form, catching per-form errors the way
// the REPL does; then clojure.test/run-all-tests tallies the file's assertions.
// Classification:
//   - skipped: the var under test is unimplemented, so the file's
//     when-var-exists gate elided the body — 0 tests, no load error.
//   - error:   a form failed to load (e.g. it references another
//     unimplemented var outside a gate) or an assertion threw (:error > 0).
//   - fail:    at least one assertion returned false (:fail > 0).
//   - pass:    tests ran with no failures or errors.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/muthuishere/cljgo/pkg/eval"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// helperFiles are suite namespaces that are NOT test files: the portability
// shim (cljgo pre-loads its own) and the number-range constants helper (loaded
// into each file's evaluator before the test file that requires it).
var helperFiles = map[string]bool{
	"portability.cljc":  true,
	"number_range.cljc": true,
}

type fileResult struct {
	File   string `json:"file"`
	Status string `json:"status"` // pass | fail | error | skipped
	Test   int64  `json:"test"`
	Pass   int64  `json:"pass"`
	Fail   int64  `json:"fail"`
	Error  int64  `json:"error"`
	Load   int    `json:"load_errors"`
}

func runSuite(args []string) int {
	fs := flag.NewFlagSet("suite", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dir := fs.String("dir", "", "clojure-test-suite root (default: $CLJGO_SUITE_DIR or ../clojure-test-suite)")
	jsonOut := fs.String("json", "", "write the JSON scoreboard to this path")
	ednOut := fs.String("edn", "", "write the EDN scoreboard to this path")
	verbose := fs.Bool("v", false, "print every file's status, not just the summary")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cljgo suite [--dir <path>] [--json <file>] [--edn <file>] [-v]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}

	root := *dir
	if root == "" {
		root = os.Getenv("CLJGO_SUITE_DIR")
	}
	if root == "" {
		root = filepath.Join("..", "clojure-test-suite")
	}
	testRoot := filepath.Join(root, "test")
	info, err := os.Stat(testRoot)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "cljgo suite: no test/ dir under %s (set --dir or $CLJGO_SUITE_DIR)\n", root)
		return 1
	}

	// numberRange is the shared constants helper; each file's evaluator loads
	// it best-effort before the test file (files that don't need it ignore it).
	numberRange := filepath.Join(testRoot, "clojure", "core_test", "number_range.cljc")

	var files []string
	err = filepath.Walk(testRoot, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() || filepath.Ext(path) != ".cljc" {
			return nil
		}
		if helperFiles[filepath.Base(path)] {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "cljgo suite: walking test tree:", err)
		return 1
	}
	sort.Strings(files)

	// Silence clojure.test's per-run "Ran N tests" chatter for the whole run.
	savedOut := eval.Out
	eval.Out = io.Discard
	defer func() { eval.Out = savedOut }()

	// One shared evaluator: cljgo's namespace registry is process-global, so a
	// fresh evaluator per file would NOT isolate tests (run-all-tests would see
	// every file's deftests). Instead we load number-range once, load each file,
	// and run only THAT file's namespace (run-tests <ns>) — unique per file, so
	// no cross-talk.
	e := eval.New()
	loadFile(e, numberRange) // shared constants helper; best-effort

	results := make([]fileResult, 0, len(files))
	for _, path := range files {
		rel, _ := filepath.Rel(testRoot, path)
		results = append(results, runOneFile(e, rel, path))
	}

	// Tally.
	var pass, fail, errc, skip int
	for _, r := range results {
		switch r.Status {
		case "pass":
			pass++
		case "fail":
			fail++
		case "error":
			errc++
		case "skipped":
			skip++
		}
	}
	total := len(results)
	tested := total - skip // files whose var cljgo resolves (non-skipped)

	if *verbose {
		for _, r := range results {
			fmt.Fprintf(os.Stdout, "%-8s %s\n", r.Status, r.File)
		}
		fmt.Fprintln(os.Stdout)
	}

	fmt.Fprintf(os.Stdout, "clojure-test-suite baseline (cljgo, interpreted)\n")
	fmt.Fprintf(os.Stdout, "  suite:      %s\n", root)
	fmt.Fprintf(os.Stdout, "  files:      %d total\n", total)
	fmt.Fprintf(os.Stdout, "  pass:       %d\n", pass)
	fmt.Fprintf(os.Stdout, "  fail:       %d\n", fail)
	fmt.Fprintf(os.Stdout, "  error:      %d\n", errc)
	fmt.Fprintf(os.Stdout, "  skipped:    %d  (var not implemented; when-var-exists elided)\n", skip)
	fmt.Fprintf(os.Stdout, "  vars resolved (non-skipped): %d/%d = %.1f%%\n",
		tested, total, pct(tested, total))
	fmt.Fprintf(os.Stdout, "  files passing: %d/%d = %.1f%% (of all)  %.1f%% (of non-skipped)\n",
		pass, total, pct(pass, total), pct(pass, tested))

	if *jsonOut != "" {
		if err := writeSuiteJSON(*jsonOut, results, root, total, pass, fail, errc, skip); err != nil {
			fmt.Fprintln(os.Stderr, "cljgo suite: writing JSON:", err)
			return 1
		}
	}
	if *ednOut != "" {
		if err := writeEDN(*ednOut, results, root, total, pass, fail, errc, skip); err != nil {
			fmt.Fprintln(os.Stderr, "cljgo suite: writing EDN:", err)
			return 1
		}
	}
	return 0
}

// runOneFile loads path into the shared evaluator and runs clojure.test for
// exactly that file's namespace, classifying the outcome.
func runOneFile(e *eval.Evaluator, rel, path string) fileResult {
	nsName, loadErrs := loadFile(e, path)

	res := fileResult{File: rel, Load: loadErrs}
	summary, err := runTests(e, nsName)
	if err != nil {
		// run-all-tests itself blew up — treat as a load/error file.
		res.Status = "error"
		return res
	}
	res.Test = igetInt(summary, "test")
	res.Pass = igetInt(summary, "pass")
	res.Fail = igetInt(summary, "fail")
	res.Error = igetInt(summary, "error")

	switch {
	case res.Test == 0 && loadErrs == 0:
		res.Status = "skipped"
	case loadErrs > 0 || res.Error > 0:
		res.Status = "error"
	case res.Fail > 0:
		res.Status = "fail"
	default:
		res.Status = "pass"
	}
	return res
}

// loadFile reads path form by form into e, binding *ns*/*file* like a real
// file load; per-form errors are counted and swallowed (the REPL evaluates
// each form independently), returning the number of forms that errored. A
// missing file returns 0 (best-effort helpers).
func loadFile(e *eval.Evaluator, path string) (nsName string, loadErrs int) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0
	}
	defer f.Close()

	lang.PushThreadBindings(lang.NewMap(
		lang.VarCurrentNS, e.CurrentNS(),
		lang.VarFile, path,
	))
	defer lang.PopThreadBindings()

	r := reader.New(bufio.NewReader(f), reader.WithFilename(path), reader.WithResolver(e.ReaderResolver()))
	for {
		form, err := r.ReadOne()
		if errors.Is(err, reader.ErrEOF) {
			// The file's (ns …) form moved *ns* to the test namespace; capture
			// it before the deferred pop reverts the thread binding.
			return e.CurrentNS().Name().Name(), loadErrs
		}
		if err != nil {
			// A reader error consumes the rest of the file (we can't resync),
			// so count it and stop.
			return e.CurrentNS().Name().Name(), loadErrs + 1
		}
		if _, err := e.EvalForm(form); err != nil {
			loadErrs++
		}
	}
}

// runTests evaluates (clojure.test/run-tests 'ns) in e and returns the summary
// map — only that namespace's deftests, so files don't accumulate (cljgo's
// namespace registry is process-global). An empty nsName runs nothing.
func runTests(e *eval.Evaluator, nsName string) (any, error) {
	if nsName == "" {
		return nil, fmt.Errorf("no namespace loaded")
	}
	form, err := reader.ReadString("(clojure.test/run-tests '"+nsName+")",
		reader.WithResolver(e.ReaderResolver()))
	if err != nil {
		return nil, err
	}
	return e.EvalForm(form)
}

// igetInt reads an int64 counter out of a clojure.test summary map.
func igetInt(m any, key string) int64 {
	v := lang.Get(m, lang.NewKeyword(key))
	if n, ok := v.(int64); ok {
		return n
	}
	return 0
}

func pct(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return 100 * float64(n) / float64(d)
}

func writeSuiteJSON(path string, results []fileResult, root string, total, pass, fail, errc, skip int) error {
	doc := map[string]any{
		// basename only — the scoreboard is a committed artifact (T2 ratchet),
		// so it must not embed a machine-specific absolute suite path.
		"suite": filepath.Base(root),
		"summary": map[string]any{
			"total": total, "pass": pass, "fail": fail, "error": errc, "skipped": skip,
			"tested":             total - skip,
			"pass_pct":           pct(pass, total),
			"pass_pct_of_tested": pct(pass, total-skip),
		},
		"files": results,
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// writeEDN emits the same scoreboard as EDN (keyword-keyed maps), so it reads
// straight back into a Clojure/cljgo REPL for the ratchet (T2).
func writeEDN(path string, results []fileResult, root string, total, pass, fail, errc, skip int) error {
	var b strings.Builder
	// basename only — committed artifact must stay machine-independent.
	fmt.Fprintf(&b, "{:suite %q\n", filepath.Base(root))
	fmt.Fprintf(&b, " :summary {:total %d :pass %d :fail %d :error %d :skipped %d :tested %d}\n",
		total, pass, fail, errc, skip, total-skip)
	b.WriteString(" :files\n [")
	for i, r := range results {
		if i > 0 {
			b.WriteString("\n  ")
		}
		fmt.Fprintf(&b, "{:file %q :status :%s :test %d :pass %d :fail %d :error %d :load-errors %d}",
			r.File, r.Status, r.Test, r.Pass, r.Fail, r.Error, r.Load)
	}
	b.WriteString("]}\n")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
