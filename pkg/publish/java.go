// Package publish holds the ADR 0054 publish-side diagnostics. This file is
// `certain-java?` (ADR 0054 dec 4): a best-effort COURTESY diagnostic over the
// SELF-IDENTIFYING JVM surfaces only — never a gate, never a guess.
//
// It is certain-only and zero-false-positive by construction (S35, precision
// 10/10): it flags import/new heads, java.*/javax.*/clojure.java.* in
// call-namespace position, and a fixed table of bare JVM classes in call-ns
// position. It deliberately does NOT flag the undecidable bare instance
// dot-form `(.method obj)` — that AST node is Go-valid or Java depending only
// on the runtime receiver (design/05 M4+), so guessing it would reject good Go
// code. The false negatives that leaves (all bare dot-forms) are safe: the Go
// compiler, `cljgo run` (ADR 0053, never silent nil), and the JVM itself each
// catch a missed Java form loudly downstream.
package publish

import (
	"bufio"
	"os"
	"strings"

	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// Diag is one certain-Java finding with the position a courtesy message cites.
type Diag struct {
	File   string
	Line   int
	Detail string // e.g. "Java static call (System/getProperty)"
}

// jvmBareClassNS is the fixed table of bare JVM classes whose static-member
// surface appears as a call namespace `Class/member` (e.g. System/getProperty,
// Math/sqrt). It deliberately excludes the ADR 0036 class-ref value vocabulary
// used by instance?/catch/bare-value positions — those are pure and must NOT be
// flagged. This is the exact table S35 validated (proto/main.go:105-111).
var jvmBareClassNS = map[string]bool{
	"System": true, "Math": true, "Thread": true, "Integer": true, "Long": true,
	"Double": true, "Float": true, "Boolean": true, "Character": true, "Byte": true,
	"Short": true, "String": true, "Object": true, "Runtime": true, "Class": true,
	"Number": true, "StringBuilder": true, "StringBuffer": true, "Arrays": true,
	"Collections": true, "Objects": true,
}

// CertainJava scans reader forms for the self-identifying JVM surfaces only and
// returns a diagnostic per certain-Java form, in source order. It is
// certain-only and zero-FP: it MUST NOT flag bare dot-forms (.method obj),
// (instance? String x), (catch Exception e), or class-ref values. It is a
// diagnostic, never a gate.
func CertainJava(forms []any) []Diag {
	var out []Diag
	for _, f := range forms {
		out = append(out, certainJavaForm(f)...)
	}
	return out
}

// certainJavaForm collects the certain-Java findings in one top-level form's
// tree. import/new are recognized at the head; the rest are symbol-position
// surfaces found anywhere in the tree.
func certainJavaForm(form any) []Diag {
	var out []Diag
	line := formLine(form)

	// import / new special forms — JVM-only, self-identifying at the head.
	if head := headSym(form); head != nil {
		switch head.Name() {
		case "import":
			out = append(out, Diag{Line: line, Detail: "(import …) — JVM-only special form"})
		case "new":
			// (new java.io.File "x") — the JVM new special form.
			out = append(out, Diag{Line: line, Detail: "(new …) — JVM interop special form"})
		}
	}

	walk(form, func(v any) {
		s, ok := v.(*lang.Symbol)
		if !ok {
			return
		}
		// java.*/javax.* in CALL-namespace position (java.util.UUID/randomUUID):
		// an interop EXECUTION. A bare java.* VALUE with no namespace is an ADR
		// 0036 ClassRef (pure opaque constant) and is deliberately NOT flagged —
		// position-awareness removes the class-ref-value false positive.
		if s.HasNamespace() && (hasPkgPrefix(s.Namespace(), "java") || hasPkgPrefix(s.Namespace(), "javax")) {
			out = append(out, Diag{Line: line, Detail: "Java package call (" + s.Namespace() + "/" + s.Name() + ")"})
			return
		}
		// clojure.java.* namespace target (clojure.java.io/file).
		if s.HasNamespace() && strings.HasPrefix(s.Namespace(), "clojure.java.") {
			out = append(out, Diag{Line: line, Detail: "clojure.java.* namespace (" + s.Namespace() + "/" + s.Name() + ")"})
			return
		}
		if !s.HasNamespace() && strings.HasPrefix(s.Name(), "clojure.java.") {
			out = append(out, Diag{Line: line, Detail: "clojure.java.* reference (" + s.Name() + ")"})
			return
		}
		// bare JVM class as a CALL namespace (System/currentTimeMillis).
		if s.HasNamespace() && jvmBareClassNS[s.Namespace()] {
			out = append(out, Diag{Line: line, Detail: "Java static call (" + s.Namespace() + "/" + s.Name() + ")"})
			return
		}
	})
	return out
}

// CertainJavaFile reads path with pkg/reader and runs CertainJava, tagging each
// Diag's File with path.
func CertainJavaFile(path string) ([]Diag, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := reader.New(bufio.NewReader(f), reader.WithFilename(path))
	forms, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	diags := CertainJava(forms)
	for i := range diags {
		diags[i].File = path
	}
	return diags, nil
}

// ---- reader helpers -------------------------------------------------------

// hasPkgPrefix reports whether ns is the package pkg or a subpackage of it
// (java == "java" or "java.*"), so a namespace merely starting with "java"
// (e.g. "javafoo") is not matched.
func hasPkgPrefix(ns, pkg string) bool {
	return ns == pkg || strings.HasPrefix(ns, pkg+".")
}

// headSym returns the head symbol of a seq form, or nil.
func headSym(form any) *lang.Symbol {
	seq, ok := form.(lang.ISeq)
	if !ok || seq == nil {
		return nil
	}
	s, _ := seq.First().(*lang.Symbol)
	return s
}

// formLine reads the :line meta a form carries, or 0.
func formLine(form any) int {
	im, ok := form.(lang.IMeta)
	if !ok || im.Meta() == nil {
		return 0
	}
	if l, ok := lang.AsInt(lang.Get(im.Meta(), lang.KWLine)); ok {
		return l
	}
	return 0
}

// walk visits every node (symbols, seqs, vectors) depth-first.
func walk(form any, visit func(any)) {
	visit(form)
	switch v := form.(type) {
	case lang.ISeq:
		for s := v; s != nil; s = s.Next() {
			walk(s.First(), visit)
		}
	case lang.IPersistentVector:
		for i := 0; i < v.Count(); i++ {
			walk(v.Nth(i), visit)
		}
	}
}
