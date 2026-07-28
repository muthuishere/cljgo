package corelib

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/diag"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// javaStaticHints maps the self-identifying Java static call a JVM-trained
// programmer (or an LLM trained on JVM Clojure) types FIRST onto the cljgo
// namespace that actually provides it. cljgo has no JVM, so `Thread/sleep`
// and `System/nanoTime` can only ever be errors (ADR 0054 decision 4: a
// static Java surface fails LOUD, never a silent nil) — but the error can
// name the replacement instead of leaving the reader to guess. Each entry is
// a REAL equivalent, verified against the cljg.* fn's own docstring and
// conformance file; speculative "close enough" mappings are deliberately
// absent (a wrong did-you-mean is worse than none).
var javaStaticHints = map[string]string{
	"Thread/sleep":             "cljg.system/sleep",
	"System/nanoTime":          "cljg.date/nano-time",
	"System/currentTimeMillis": "cljg.date/now",
	"System/getenv":            "cljg.system/getenv",
	"System/exit":              "cljg.system/exit",
}

// javaStaticNamespaces are the class names whose statics are common enough
// that a generic "cljgo has no Java host" note beats a bare "no such
// namespace" even when the specific member has no cljgo twin.
var javaStaticNamespaces = map[string]string{
	"System": "cljg.system / cljg.date",
	"Thread": "cljg.system",
}

// noSuchNamespaceError is the resolution failure for a namespaced symbol
// whose namespace does not exist. Error() OPENS with the byte-identical
// clause it replaced ("no such namespace: X") — conformance and the publish
// gate freeze that prefix — while Diagnostic() (diag.Carrier) adds the
// registered code and, for a Java static, the did-you-mean Fix the error
// doctrine asks for (CLAUDE.md: suggestions are Fixes, not prose).
//
// Reconciled 2026-07-28 from two independent fixes that replaced the SAME
// bare fmt.Errorf: the gaps batch (known-issue #10) contributed this carrier
// — the A2009 code plus did-you-mean Fixes — and the interop batch
// (known-issue #7, Integer/parseInt "unresolved AND misdiagnosed")
// contributed the broad jvmStaticClass table and the class-not-a-namespace
// clause. Both are kept: the clause lands in Error() because
// conformance/tests/java-static-class-not-namespace.clj freezes it there,
// and java-static-loud-error.clj still matches on the leading clause.
type noSuchNamespaceError struct {
	ns   string // the unresolved namespace ("System")
	full string // the full symbol as written ("System/nanoTime")
}

func newNoSuchNamespaceError(sym *lang.Symbol) error {
	return &noSuchNamespaceError{ns: sym.Namespace(), full: sym.FullName()}
}

// classNotNamespace reports whether this error should spell out that the
// namespace is really a Java class. It does so ONLY when we have no concrete
// cljgo replacement to point at: if javaStaticHints knows one, the
// did-you-mean Fix already says everything the prose would, and the doctrine
// prefers a Fix over prose. That split is also what keeps the message
// byte-stable for the hinted statics (pkg/corelib TestJavaStaticDidYouMean
// and the publish gate freeze "no such namespace: Thread") while still
// diagnosing Integer/parseInt, which has no twin
// (conformance/tests/java-static-class-not-namespace.clj).
func (e *noSuchNamespaceError) classNotNamespace() bool {
	if _, hinted := javaStaticHints[e.full]; hinted {
		return false
	}
	return jvmStaticClass[e.ns]
}

func (e *noSuchNamespaceError) Error() string {
	if e.classNotNamespace() {
		// Naming what the thing actually IS beats "no such namespace" —
		// the user wrote a Java class, not a mistyped namespace.
		return fmt.Sprintf("no such namespace: %s (%s is a Java class, not a namespace: cljgo hosts Clojure on Go, so the Java static %s is unavailable)",
			e.ns, e.ns, e.full)
	}
	return fmt.Sprintf("no such namespace: %s", e.ns)
}

// Diagnostic implements diag.Carrier. The location is left empty: this error
// is raised without a position and the analyzer wraps it in a
// lang.CompilerError that has one, which diag.FromError reads back.
func (e *noSuchNamespaceError) Diagnostic() (diag.Diagnostic, bool) {
	// Code follows the band (pkg/diag/registry.go): a Java class used as a
	// namespace is an INTEROP failure (I4001), not a generic unresolved
	// namespace (A2009). Both are registered and both have explain pages;
	// routing by case is what keeps `cljgo explain` useful.
	code := "A2009"
	if e.classNotNamespace() {
		code = "I4001"
	}
	d := diag.Diagnostic{
		Severity:   diag.SeverityError,
		Message:    e.Error(),
		ErrorCode:  code,
		ExplainURL: diag.ExplainURL(code),
	}
	if repl, ok := javaStaticHints[e.full]; ok {
		d.Fixes = append(d.Fixes, diag.Fix{
			Title:       fmt.Sprintf("cljgo has no Java host — did you mean %s? (require '[%s])", repl, nsOf(repl)),
			Replacement: repl,
		})
		return d, true
	}
	if where, ok := javaStaticNamespaces[e.ns]; ok {
		d.Fixes = append(d.Fixes, diag.Fix{
			Title: fmt.Sprintf("%s is a Java class; cljgo runs on Go and has no Java host — look in %s", e.ns, where),
		})
	}
	return d, true
}

// nsOf splits "cljg.date/nano-time" into its namespace part.
func nsOf(qualified string) string {
	for i := 0; i < len(qualified); i++ {
		if qualified[i] == '/' {
			return qualified[:i]
		}
	}
	return qualified
}
