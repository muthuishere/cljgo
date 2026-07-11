// Package harness is the golden-file diff harness for reader conformance.
//
// It loads input->golden pairs produced by gen_golden.clj + cmd/mkgolden and
// runs them against an injected ReadString — the candidate reader. Today the
// candidate is a stub; once pkg/reader exists, its CI wires in:
//
//	rs := func(src string) (string, error) {
//	    form, err := reader.ReadString(src)   // includes syntax-quote expansion
//	    if err != nil { return "", err }
//	    return lang.PrStr(form), nil          // Clojure-compatible printer
//	}
//	report := harness.Run(cases, rs)
//
// Both golden and candidate outputs pass through normalize.Gensyms before
// comparison, so per-run gensym counters never cause false diffs.
package harness

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/muthuishere/cljgo/spikes/s8-syntax-quote-harness/normalize"
)

// Case is one conformance case from the golden file.
type Case struct {
	Input   string // reader input, e.g. "`(a ~b)"
	Golden  string // normalized pr-str of the JVM expansion; "" when WantErr
	WantErr bool   // JVM Clojure's reader threw on this input
	ErrKind string // JVM exception simple class name (informational only)
}

// ReadString is the injection point for the candidate reader: read one form
// from src (performing syntax-quote expansion) and print it pr-str-style.
// Return an error for inputs the reader must reject.
type ReadString func(src string) (string, error)

// Result is the outcome of one case.
type Result struct {
	Case Case
	Pass bool
	Got  string // normalized candidate output, or "error: ..." on candidate error
}

// Report aggregates a run.
type Report struct {
	Results []Result
	Passed  int
	Failed  int
}

// LoadGolden parses a golden file: blank-line-separated records of
//
//	IN: <input>
//	OK: <expansion>      (or ERR: <ExceptionSimpleName>)
//
// Lines starting with ";;" are comments.
func LoadGolden(path string) ([]Case, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseGolden(f)
}

// ParseGolden reads golden records from r. See LoadGolden.
func ParseGolden(r io.Reader) ([]Case, error) {
	var cases []Case
	var cur *Case
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, ";;"):
			// comment
		case strings.TrimSpace(line) == "":
			if cur != nil {
				return nil, fmt.Errorf("golden line %d: record for %q has IN: but no OK:/ERR:", lineNo, cur.Input)
			}
		case strings.HasPrefix(line, "IN: "):
			if cur != nil {
				return nil, fmt.Errorf("golden line %d: record for %q has IN: but no OK:/ERR:", lineNo, cur.Input)
			}
			cur = &Case{Input: line[len("IN: "):]}
		case strings.HasPrefix(line, "OK: "):
			if cur == nil {
				return nil, fmt.Errorf("golden line %d: OK: without preceding IN:", lineNo)
			}
			cur.Golden = normalize.Gensyms(line[len("OK: "):])
			cases = append(cases, *cur)
			cur = nil
		case strings.HasPrefix(line, "ERR: "):
			if cur == nil {
				return nil, fmt.Errorf("golden line %d: ERR: without preceding IN:", lineNo)
			}
			cur.WantErr = true
			cur.ErrKind = line[len("ERR: "):]
			cases = append(cases, *cur)
			cur = nil
		default:
			return nil, fmt.Errorf("golden line %d: unrecognized line %q", lineNo, line)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if cur != nil {
		return nil, fmt.Errorf("golden file truncated: record for %q has IN: but no OK:/ERR:", cur.Input)
	}
	return cases, nil
}

// Run executes every case against rs, normalizing both sides before compare.
func Run(cases []Case, rs ReadString) Report {
	rep := Report{}
	for _, c := range cases {
		got, err := rs(c.Input)
		res := Result{Case: c}
		switch {
		case c.WantErr:
			res.Pass = err != nil
			if err != nil {
				res.Got = "error: " + err.Error()
			} else {
				res.Got = normalize.Gensyms(got)
			}
		case err != nil:
			res.Pass = false
			res.Got = "error: " + err.Error()
		default:
			res.Got = normalize.Gensyms(got)
			res.Pass = res.Got == c.Golden
		}
		if res.Pass {
			rep.Passed++
		} else {
			rep.Failed++
		}
		rep.Results = append(rep.Results, res)
	}
	return rep
}

// Print writes a human-readable pass/fail report to w and returns Failed.
func (rep Report) Print(w io.Writer, verbose bool) int {
	for i, r := range rep.Results {
		if r.Pass && !verbose {
			continue
		}
		status := "PASS"
		if !r.Pass {
			status = "FAIL"
		}
		fmt.Fprintf(w, "%s case %02d  input:  %s\n", status, i+1, r.Case.Input)
		if !r.Pass {
			want := r.Case.Golden
			if r.Case.WantErr {
				want = "<any error> (JVM threw " + r.Case.ErrKind + ")"
			}
			fmt.Fprintf(w, "        want:   %s\n", want)
			fmt.Fprintf(w, "        got:    %s\n", r.Got)
		}
	}
	fmt.Fprintf(w, "\n%d/%d passed, %d failed\n", rep.Passed, len(rep.Results), rep.Failed)
	return rep.Failed
}
