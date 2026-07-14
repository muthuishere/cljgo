package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/diag"
)

// checkGolden runs `cljgo check <file> --json` and returns stdout, the
// exit code, and stderr.
func runVerb(t *testing.T, verb string, args ...string) (stdout string, code int, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	switch verb {
	case "check":
		code = runCheck(args, &out, &errb)
	case "explain":
		code = runExplain(args, &out, &errb)
	default:
		t.Fatalf("unknown verb %q", verb)
	}
	return out.String(), code, errb.String()
}

func TestCheckUnresolvedSymbolJSON(t *testing.T) {
	const want = `{
  "schema": "cljgo-diag/1",
  "diagnostics": [
    {
      "error_code": "A2001",
      "severity": "error",
      "message": "unable to resolve symbol: pie in this context",
      "location": {
        "file": "testdata/unresolved.clj",
        "line": 1,
        "column": 19
      },
      "explain_url": "docs/diagnostics/A2001.md"
    }
  ]
}
`
	got, code, _ := runVerb(t, "check", "testdata/unresolved.clj", "--json")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if got != want {
		t.Fatalf("check --json JSON mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestCheckCleanFileJSON(t *testing.T) {
	got, code, _ := runVerb(t, "check", "testdata/clean.clj", "--json")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got != "{\"ok\":true}\n" {
		t.Fatalf("clean check --json = %q, want %q", got, "{\"ok\":true}\n")
	}
}

func TestCheckCleanFileHumanSilent(t *testing.T) {
	got, code, errb := runVerb(t, "check", "testdata/clean.clj")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got != "" || errb != "" {
		t.Fatalf("clean check (human) should be silent: stdout=%q stderr=%q", got, errb)
	}
}

func TestCheckHumanPositioned(t *testing.T) {
	_, code, errb := runVerb(t, "check", "testdata/unresolved.clj")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	const want = "error: testdata/unresolved.clj:1:19: unable to resolve symbol: pie in this context [A2001]\n"
	if errb != want {
		t.Fatalf("human check stderr = %q, want %q", errb, want)
	}
}

// TestCheckClassification exercises the error->code mapping across the
// common bands so a regression in classify() is caught.
func TestCheckClassification(t *testing.T) {
	cases := []struct {
		file string
		code string
	}{
		{"testdata/unresolved.clj", "A2001"},
		{"testdata/def-nonsymbol.clj", "A2005"},
		{"testdata/odd-binding.clj", "A2006"},
		{"testdata/unterminated.clj", "R1001"},
	}
	for _, c := range cases {
		diags := CheckSource(mustRead(t, c.file), c.file)
		if len(diags) != 1 {
			t.Fatalf("%s: got %d diagnostics, want 1", c.file, len(diags))
		}
		if diags[0].ErrorCode != c.code {
			t.Fatalf("%s: error_code = %q, want %q", c.file, diags[0].ErrorCode, c.code)
		}
	}
}

func TestExplainKnownCodeJSON(t *testing.T) {
	got, code, _ := runVerb(t, "explain", "A2001", "--json")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	// The verb must emit exactly the structured registry entry + doc.
	page, err := diag.ExplainStructured("A2001")
	if err != nil {
		t.Fatal(err)
	}
	wantBytes, _ := json.MarshalIndent(page, "", "  ")
	want := string(wantBytes) + "\n"
	if got != want {
		t.Fatalf("explain --json mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	// Sanity on the payload shape.
	var round diag.ExplainPage
	if err := json.Unmarshal([]byte(got), &round); err != nil {
		t.Fatalf("explain --json is not valid JSON: %v", err)
	}
	if round.Code != "A2001" || round.Band != "analyzer" ||
		round.ExplainURL != "docs/diagnostics/A2001.md" ||
		!strings.HasPrefix(round.Doc, "# A2001") {
		t.Fatalf("explain --json fields wrong: %+v", round)
	}
}

func TestExplainHumanCaseInsensitive(t *testing.T) {
	got, code, _ := runVerb(t, "explain", "a2001")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.HasPrefix(got, "# A2001") {
		t.Fatalf("explain (human) should print the page, got: %q", got[:min(40, len(got))])
	}
}

func TestExplainUnknownCode(t *testing.T) {
	_, code, errb := runVerb(t, "explain", "Z9999", "--json")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errb, "unknown error code") {
		t.Fatalf("explain unknown stderr = %q", errb)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
