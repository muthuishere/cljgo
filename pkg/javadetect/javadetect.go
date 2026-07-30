// Package javadetect is the ONE certain-Java classifier, shared by both
// directions of ADR 0054 decision 4: the publish side (a courtesy diagnostic
// on a library we are about to ship) and the consume side (the per-NAMESPACE
// gate on a Clojars jar, ADR 0095). It was lifted verbatim out of
// pkg/publish/java.go so the two directions can never drift.
//
// CertainJava is the publish-side surface, byte-identical to what pkg/publish
// shipped. ConsumeJava is a STRICT SUPERSET used by the consume gate: it adds
// the ns-form surfaces (:import / :gen-class) and the class-defining special
// forms that a jar's Java namespaces announce themselves with. Both remain
// certain-only and zero-false-positive: neither flags a bare (.method obj),
// (instance? String x), (catch Exception e), or a class-ref value.
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
package javadetect

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

// ---- consume side (ADR 0095) ----------------------------------------------

// ConsumeJava is the classifier the Clojars consume gate runs (ADR 0095,
// spike s50 finding 3): CertainJava plus the surfaces a JVM-only namespace in
// a published jar announces itself with, which the publish side never needed
// because a cljgo-authored library cannot contain them.
//
// It is run on the READER'S OUTPUT, after reader conditionals have been
// resolved for the :cljgo platform — which is what makes a library like medley
// honestly pure: its java.util forms live in a #?(:clj …) branch cljgo never
// reads, so they are not in the forms and cannot be flagged.
//
// The added surfaces, all self-identifying and undeniably JVM:
//
//   - (ns … (:import …)) / (ns … (:gen-class)) — the dominant real signal
//     (s50: this is what flagged cheshire, clj-http, hiccup.compiler,
//     hiccup.util);
//   - the class-defining special forms gen-class, definterface, proxy,
//     proxy-super, and reify/extend-type/extend-protocol naming a java.*
//     or javax.* type;
//   - (set! Class/field …) is already covered by the Class/member table.
//
// It still MUST NOT flag a bare (.method obj) — undecidable, Go-valid.
func ConsumeJava(forms []any) []Diag {
	var out []Diag
	for _, f := range forms {
		out = append(out, certainJavaForm(f)...)
		out = append(out, consumeOnlyForm(f)...)
	}
	return out
}

// classFormHeads are the special forms that DEFINE or EXTEND a host class.
// gen-class/definterface/proxy/proxy-super are JVM-only outright; reify,
// extend-type and extend-protocol are flagged only when a java.*/javax.* or
// tabled bare JVM class appears among their arguments.
var classFormHeads = map[string]bool{
	"gen-class": true, "definterface": true, "proxy": true, "proxy-super": true,
}

var hostTypeFormHeads = map[string]bool{
	"reify": true, "extend-type": true, "extend-protocol": true, "extend": true,
}

func consumeOnlyForm(form any) []Diag {
	var out []Diag
	walkSeq(form, func(node any) {
		head := headSym(node)
		if head == nil || head.HasNamespace() {
			return
		}
		line := formLine(node)
		if line == 0 {
			line = formLine(form)
		}
		switch {
		case head.Name() == "ns":
			out = append(out, nsFormDiags(node, line)...)
		case classFormHeads[head.Name()]:
			out = append(out, Diag{Line: line, Detail: "(" + head.Name() + " …) — JVM class-definition special form"})
		case hostTypeFormHeads[head.Name()]:
			if t := firstJavaType(node); t != "" {
				out = append(out, Diag{Line: line, Detail: "(" + head.Name() + " …) on JVM type " + t})
			}
		}
	})
	return out
}

// nsFormDiags flags (:import …) and (:gen-class) clauses inside an ns form.
// Note the classifier runs AFTER reader conditionals, so
// (ns x (:import #?(:clj [java.util Date]))) is pure on cljgo: the clause is
// simply not there.
func nsFormDiags(nsForm any, line int) []Diag {
	var out []Diag
	seq, ok := nsForm.(lang.ISeq)
	if !ok || seq == nil {
		return nil
	}
	for s := seq.Next(); s != nil; s = s.Next() {
		clause, ok := s.First().(lang.ISeq)
		if !ok || clause == nil {
			continue
		}
		var name string
		switch k := clause.First().(type) {
		case lang.Keyword:
			name = k.Name()
		case *lang.Symbol:
			name = k.Name()
		default:
			continue
		}
		cl := formLine(clause)
		if cl == 0 {
			cl = line
		}
		switch name {
		case "import":
			// An EMPTY (:import) imports nothing. That is not a hypothetical:
			// it is what `(ns x (:import #?(:clj [java.util Date])))` reduces
			// to on cljgo once the conditional elides, and flagging it would
			// reject exactly the portable libraries this gate exists to admit.
			if clause.Next() == nil {
				continue
			}
			out = append(out, Diag{Line: cl, Detail: "(ns … (:import …)) — JVM-only ns clause"})
		case "gen-class":
			out = append(out, Diag{Line: cl, Detail: "(ns … (:gen-class)) — JVM-only ns clause"})
		}
	}
	return out
}

// firstJavaType returns the first java.*/javax.* or tabled bare JVM class
// symbol appearing in the form, or "".
func firstJavaType(form any) string {
	found := ""
	walk(form, func(v any) {
		if found != "" {
			return
		}
		s, ok := v.(*lang.Symbol)
		if !ok || s.HasNamespace() {
			return
		}
		n := s.Name()
		if hasPkgPrefix(n, "java") || hasPkgPrefix(n, "javax") || jvmBareClassNS[n] {
			found = n
		}
	})
	return found
}

// walkSeq visits every SEQ node (a form that could have a head symbol),
// depth-first, so nested (ns …)/(proxy …) inside a top-level form are seen.
func walkSeq(form any, visit func(any)) {
	switch v := form.(type) {
	case lang.ISeq:
		visit(form)
		for s := v; s != nil; s = s.Next() {
			walkSeq(s.First(), visit)
		}
	case lang.IPersistentVector:
		for i := 0; i < v.Count(); i++ {
			walkSeq(v.Nth(i), visit)
		}
	}
}
