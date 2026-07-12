// Package conformance is the shared conformance suite (design/00 §6,
// design/03 §7d) in its M0, eval-only form: each tests/*.clj file runs
// through the same Read→Analyze→Eval path as the REPL and its last
// value's pr-str is compared against the file's `;; expect:` comment
// (or its error against `;; expect-error:`). The emitter harness joins
// at M2 and replays the same files.
package conformance

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/repl"
)

type expectation struct {
	value   string // exact pr-str of the last form's value
	errText string // substring of the error message
	isError bool
}

// parseExpectation scans src for the single `;; expect:` /
// `;; expect-error:` comment (README.md).
func parseExpectation(path, src string) (expectation, error) {
	var exp expectation
	found := 0
	sc := bufio.NewScanner(strings.NewReader(src))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, ";; expect-error:"):
			exp = expectation{errText: strings.TrimSpace(strings.TrimPrefix(line, ";; expect-error:")), isError: true}
			found++
		case strings.HasPrefix(line, ";; expect:"):
			exp = expectation{value: strings.TrimSpace(strings.TrimPrefix(line, ";; expect:"))}
			found++
		}
	}
	if found != 1 {
		return exp, fmt.Errorf("%s: want exactly one ;; expect(-error): comment, found %d", path, found)
	}
	return exp, nil
}

// evalFile runs one file through the eval harness in a fresh `user`
// namespace (namespaces are process-global, so it is removed first —
// files must not depend on each other).
func evalFile(path string) (any, error) {
	lang.RemoveNamespace(lang.NewSymbol("user"))
	d := repl.New(nil, io.Discard, io.Discard)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return d.EvalReader(f, path)
}

func TestConformanceEval(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("tests", "*.clj"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no conformance test files found under tests/")
	}
	for _, path := range files {
		name := strings.TrimSuffix(filepath.Base(path), ".clj")
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			exp, err := parseExpectation(path, string(src))
			if err != nil {
				t.Fatal(err)
			}
			last, err := evalFile(path)
			if exp.isError {
				if err == nil {
					t.Fatalf("want error containing %q, got value %s", exp.errText, lang.PrintString(last))
				}
				if !strings.Contains(err.Error(), exp.errText) {
					t.Fatalf("error %q does not contain %q", err.Error(), exp.errText)
				}
				return
			}
			if err != nil {
				t.Fatalf("eval: %v", err)
			}
			if got := lang.PrintString(last); got != exp.value {
				t.Fatalf("last value pr-str = %q, want %q", got, exp.value)
			}
		})
	}
}
