// Package conformance is the shared conformance suite (design/00 §6,
// design/03 §7d), dual-harness since M2 (ADR 0007): each tests/*.clj
// file runs through the Read→Analyze→Eval path (this file) AND through
// the Go emitter as a compiled binary (compiled_test.go) with
// byte-identical output required; ORACLE=1 (oracle_test.go) re-audits
// the frozen expectations against the real `clojure` CLI. Per-file
// directives: `;; harness: eval — reason` (eval-only waiver) and
// `;; oracle: skip — reason`.
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

// directives are the per-file harness waivers (README.md).
type directives struct {
	evalOnly   string // reason after ";; harness: eval", "" = dual
	oracleSkip string // reason after ";; oracle: skip", "" = audited
}

func parseDirectives(src string) directives {
	var d directives
	sc := bufio.NewScanner(strings.NewReader(src))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if rest, ok := strings.CutPrefix(line, ";; harness: eval"); ok {
			d.evalOnly = strings.TrimSpace(strings.TrimLeft(rest, " —-"))
			if d.evalOnly == "" {
				d.evalOnly = "marked eval-only"
			}
		}
		if rest, ok := strings.CutPrefix(line, ";; oracle: skip"); ok {
			d.oracleSkip = strings.TrimSpace(strings.TrimLeft(rest, " —-"))
			if d.oracleSkip == "" {
				d.oracleSkip = "marked oracle-skip"
			}
		}
	}
	return d
}

// namespaceSnapshot / removeNewNamespaces bracket one harness run: the
// namespace registry is process-global, and a run may load file-backed
// namespaces (require, ADR 0042) whose survival would let the NEXT run
// skip loading them — cross-talk between harnesses, not semantics.
func namespaceSnapshot() map[string]bool {
	snap := map[string]bool{}
	for s := lang.AllNamespaces(); s != nil; s = s.Next() {
		snap[s.First().(*lang.Namespace).Name().String()] = true
	}
	return snap
}

func removeNewNamespaces(snap map[string]bool) {
	for s := lang.AllNamespaces(); s != nil; s = s.Next() {
		name := s.First().(*lang.Namespace).Name()
		if !snap[name.String()] {
			lang.RemoveNamespace(name)
		}
	}
}

// evalFile runs one file through the eval harness in a fresh `user`
// namespace (namespaces are process-global, so it is removed first —
// files must not depend on each other).
func evalFile(path string) (any, error) {
	snap := namespaceSnapshot()
	defer removeNewNamespaces(snap)
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
