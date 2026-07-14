package diag

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/muthuishere/cljgo/pkg/reader"
)

// compilerErrRe parses the stable text rendered by lang.CompilerError:
// "compiler error at <file>:<line>:<col>: <message>". lang keeps that
// type's fields unexported and offers no accessor, so the CLI wiring of
// ADR 0015 reads the position back out of the rendered string. The file
// group is greedy but backtracks so the two trailing :<digits> groups
// still bind (a path may itself contain colons).
var compilerErrRe = regexp.MustCompile(`^compiler error at (.*):(\d+):(\d+): ([\s\S]*)$`)

// FromError maps an error produced by the reader or analyzer into a
// structured Diagnostic (ADR 0015). Errors that carry a position
// (reader.Error, lang.CompilerError) yield a located diagnostic; the
// message is classified to a registered error code when it matches a
// known pattern, otherwise it falls back to the general G5000 code so a
// check never fails to produce a record. Registered codes get an
// ExplainURL pointing at their explain page.
func FromError(err error) Diagnostic {
	if err == nil {
		return Diagnostic{}
	}

	// Reader errors expose position and inner cause via exported fields.
	var re *reader.Error
	if errors.As(err, &re) {
		d := Diagnostic{
			Severity: SeverityError,
			Message:  re.Err.Error(),
			Location: Location{File: re.Pos.File, Line: re.Pos.Line, Column: re.Pos.Col},
		}
		if re.Start != nil {
			d.Related = append(d.Related, Related{
				Message:  "form starts here",
				Location: Location{File: re.Start.File, Line: re.Start.Line, Column: re.Start.Col},
			})
		}
		assignCode(&d, BandReader)
		return d
	}

	// Analyzer errors render as CompilerError text; parse position back.
	if m := compilerErrRe.FindStringSubmatch(err.Error()); m != nil {
		line, _ := strconv.Atoi(m[2])
		col, _ := strconv.Atoi(m[3])
		d := Diagnostic{
			Severity: SeverityError,
			Message:  m[4],
			Location: Location{File: m[1], Line: line, Column: col},
		}
		assignCode(&d, BandAnalyzer)
		return d
	}

	// Anything else: an unlocated, uncategorized diagnostic.
	d := Diagnostic{Severity: SeverityError, Message: err.Error(), ErrorCode: "G5000"}
	setExplainURL(&d)
	return d
}

// assignCode classifies d.Message within band, falling back to the
// general G5000 code when nothing matches, then attaches the explain URL.
func assignCode(d *Diagnostic, band Band) {
	if code := classify(band, d.Message); code != "" {
		d.ErrorCode = code
	} else {
		d.ErrorCode = "G5000"
	}
	setExplainURL(d)
}

// setExplainURL points a diagnostic at its explain page when the code is
// registered (unregistered codes never happen here but stay URL-less).
func setExplainURL(d *Diagnostic) {
	if _, ok := Lookup(d.ErrorCode); ok {
		d.ExplainURL = ExplainURL(d.ErrorCode)
	}
}

// ExplainURL is the repo-relative path of a code's explain page.
func ExplainURL(code string) string { return "docs/diagnostics/" + code + ".md" }

// classify maps a message to a registered error code within its band by
// matching the message-pattern vocabulary of the reader and analyzer.
// It returns "" when nothing matches (caller falls back to G5000). Match
// order is significant where patterns overlap (escape before character).
func classify(band Band, msg string) string {
	m := strings.ToLower(msg)
	has := func(sub string) bool { return strings.Contains(m, strings.ToLower(sub)) }

	switch band {
	case BandReader:
		switch {
		case has("eof while reading"):
			return "R1001"
		case has("unmatched delimiter"):
			return "R1002"
		case has("even number of forms") && has("map literal"):
			return "R1003"
		case has("duplicate key"):
			return "R1004"
		case has("invalid token"):
			return "R1005"
		case has("invalid number") || has("invalid digit"):
			return "R1006"
		case has("escape"):
			return "R1007"
		case has("invalid character") || has("invalid unicode"):
			return "R1008"
		case has("metadata"):
			return "R1009"
		}
	case BandAnalyzer:
		switch {
		case has("unable to resolve symbol") || has("unable to resolve var"):
			return "A2001"
		case has("recur") && has("tail"):
			return "A2002"
		case has("recur") && (has("argument count") || has("cannot recur across")):
			return "A2003"
		case has("binding vector") || has("vector for its bindings"):
			return "A2006"
		case has("wrong number of args") || has("too many arguments to") ||
			has("too few arguments to"):
			return "A2004"
		case has("first argument to def must be a symbol"):
			return "A2005"
		case has("bad binding form") || has("can't let qualified") ||
			has("can't bind name"):
			return "A2007"
		case has("variadic overload") || has("overloads with same arity") ||
			has("fixed arity function"):
			return "A2008"
		}
	}
	return ""
}

// ExplainPage is the structured form of an explain lookup: the registry
// entry plus its long-form doc (ADR 0015 compiler.explain).
type ExplainPage struct {
	Code       string `json:"code"`
	Title      string `json:"title"`
	Since      string `json:"since"`
	Band       string `json:"band"`
	ExplainURL string `json:"explain_url"`
	Doc        string `json:"doc"`
}

// bandName is the human label for a band letter.
func bandName(b Band) string {
	switch b {
	case BandReader:
		return "reader"
	case BandAnalyzer:
		return "analyzer"
	case BandEmitter:
		return "emitter"
	case BandInterop:
		return "interop"
	case BandGeneral:
		return "general"
	}
	return "unknown"
}

// ExplainStructured returns the registry entry and long-form doc for a
// code as a single value, for `cljgo explain <code> --json`.
func ExplainStructured(code string) (ExplainPage, error) {
	e, ok := Lookup(code)
	if !ok {
		return ExplainPage{}, fmt.Errorf("diag: unknown error code %q", code)
	}
	doc, err := Explain(code)
	if err != nil {
		return ExplainPage{}, err
	}
	return ExplainPage{
		Code:       e.Code,
		Title:      e.Title,
		Since:      e.Since,
		Band:       bandName(e.Band()),
		ExplainURL: ExplainURL(e.Code),
		Doc:        doc,
	}, nil
}
