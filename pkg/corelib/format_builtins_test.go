package corelib_test

// Unit tests for format/printf (ADR 0030) beyond the corpus-driven
// conformance/tests/format-*.clj files: printf's write path specifically,
// since the conformance harness only asserts the LAST form's pr-str value
// (README.md), not side-effecting stdout content.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/corelib"
	"github.com/muthuishere/cljgo/pkg/eval"
	"github.com/muthuishere/cljgo/pkg/reader"
)

func TestPrintfWritesThroughOut(t *testing.T) {
	e := eval.New()
	var buf bytes.Buffer
	old := corelib.Out
	corelib.Out = &buf
	defer func() { corelib.Out = old }()

	r := reader.New(strings.NewReader(`(printf "%s=%d" "x" 1)`), reader.WithResolver(e.ReaderResolver()))
	form, err := r.ReadOne()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got, err := e.EvalForm(form)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	if got != nil {
		t.Errorf("printf returned %v, want nil", got)
	}
	if buf.String() != "x=1" {
		t.Errorf("printf wrote %q, want %q", buf.String(), "x=1")
	}
}
