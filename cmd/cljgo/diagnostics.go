// Diagnostics CLI surface (ADR 0015): `cljgo check` and `cljgo explain`
// expose the structured pkg/diag data model so editors, CI, and LLM
// agents consume compiler output as data instead of parsing prose. The
// verbs are debug/tooling capacity only — repl/run/build output is
// unchanged; JSON appears solely under --json here.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/muthuishere/cljgo/pkg/diag"
	"github.com/muthuishere/cljgo/pkg/eval"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// runCheck implements `cljgo check <file.clj> [--json]`. It reads and
// analyzes the file WITHOUT executing it, emitting any diagnostics. With
// --json it writes a cljgo-diag envelope (or {"ok":true} on success) to
// stdout; without it, human-readable positioned errors to stderr. Exit
// is non-zero when any diagnostic is produced.
func runCheck(args []string, stdout, stderr io.Writer) int {
	file, asJSON, ok := parseDiagArgs(args)
	if !ok {
		fmt.Fprintln(stderr, "usage: cljgo check <file.clj> [--json]")
		return 2
	}
	src, err := os.ReadFile(file)
	if err != nil {
		if asJSON {
			writeJSON(stdout, diag.NewEnvelope([]diag.Diagnostic{{
				ErrorCode: "G5000",
				Severity:  diag.SeverityError,
				Message:   err.Error(),
			}}))
		} else {
			fmt.Fprintln(stderr, "error:", err)
		}
		return 1
	}

	diags := CheckSource(string(src), file)

	if len(diags) == 0 {
		if asJSON {
			fmt.Fprintln(stdout, `{"ok":true}`)
		}
		return 0
	}
	if asJSON {
		writeJSON(stdout, diag.NewEnvelope(diags))
	} else {
		for _, d := range diags {
			fmt.Fprintln(stderr, "error:", humanLine(d))
		}
	}
	return 1
}

// CheckSource reads every form from src and analyzes it (macroexpansion
// happens; no evaluation), returning one diagnostic per failing form.
// Reader errors stop the stream (the input can no longer be reliably
// re-synchronized); analyzer errors are collected and reading continues.
func CheckSource(src, filename string) []diag.Diagnostic {
	ev := eval.New()
	lang.PushThreadBindings(lang.NewMap(
		lang.VarCurrentNS, ev.CurrentNS(),
		lang.VarFile, filename,
	))
	defer lang.PopThreadBindings()

	rd := reader.New(bufio.NewReader(strings.NewReader(src)),
		reader.WithFilename(filename),
		reader.WithResolver(ev.ReaderResolver()))
	an := ev.Analyzer()

	var diags []diag.Diagnostic
	for {
		form, err := rd.ReadOne()
		if errors.Is(err, reader.ErrEOF) {
			break
		}
		if err != nil {
			diags = append(diags, diag.FromError(err))
			break
		}
		if _, err := an.Analyze(form); err != nil {
			diags = append(diags, diag.FromError(err))
		}
	}
	return diags
}

// runExplain implements `cljgo explain <error-code> [--json]`. Human
// form prints the code's explain page; --json returns the structured
// registry entry plus the doc. Unknown codes exit non-zero.
func runExplain(args []string, stdout, stderr io.Writer) int {
	code, asJSON, ok := parseDiagArgs(args)
	if !ok {
		fmt.Fprintln(stderr, "usage: cljgo explain <error-code> [--json]")
		return 2
	}
	code = strings.ToUpper(code)

	if asJSON {
		page, err := diag.ExplainStructured(code)
		if err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		writeJSON(stdout, page)
		return 0
	}

	doc, err := diag.Explain(code)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	fmt.Fprint(stdout, doc)
	if !strings.HasSuffix(doc, "\n") {
		fmt.Fprintln(stdout)
	}
	return 0
}

// parseDiagArgs pulls a single positional argument and an optional
// --json flag (in any order) out of args.
func parseDiagArgs(args []string) (arg string, asJSON, ok bool) {
	var positional []string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		default:
			positional = append(positional, a)
		}
	}
	if len(positional) != 1 {
		return "", asJSON, false
	}
	return positional[0], asJSON, true
}

// humanLine renders a diagnostic as a positioned one-liner, matching the
// reader/analyzer text style (file:line:col: message).
func humanLine(d diag.Diagnostic) string {
	if d.Location.File != "" || d.Location.Line != 0 {
		return fmt.Sprintf("%s:%d:%d: %s [%s]",
			d.Location.File, d.Location.Line, d.Location.Column, d.Message, d.ErrorCode)
	}
	return fmt.Sprintf("%s [%s]", d.Message, d.ErrorCode)
}

// writeJSON marshals v as deterministic, indented JSON with a trailing
// newline. Key order follows struct field order; no addresses or
// timestamps enter the output, so it is stable for golden diffing.
func writeJSON(w io.Writer, v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintln(w, `{"error":"marshal failed"}`)
		return
	}
	w.Write(b)
	io.WriteString(w, "\n")
}
