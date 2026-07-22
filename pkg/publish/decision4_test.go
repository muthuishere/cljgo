package publish

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/emit"
)

// TestJavaStaticFailsLoudPerNamespace — ADR 0054 decision 4: purity is a
// per-namespace property. A namespace using a static Java surface
// (System/currentTimeMillis) hard-errors LOUD at analysis with file:line and
// never a silent nil; a pure namespace stays independently usable (compiles).
// Granularity is per-namespace, exactly as decision 4 requires.
func TestJavaStaticFailsLoudPerNamespace(t *testing.T) {
	// The Java-static namespace fails loudly at compile — never a silent nil.
	_, err := emit.CompileProgram(filepath.FromSlash("testdata/javastatic/js/core.clj"))
	if err == nil {
		t.Fatalf("a Java-static namespace must fail LOUD, not compile to a silent nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "no such namespace: System") {
		t.Errorf("error should name the unresolved Java surface System: %s", msg)
	}
	if !strings.Contains(filepath.ToSlash(msg), "js/core.clj:") {
		t.Errorf("error should cite the offending file:line (js/core.clj:…): %s", msg)
	}

	// The pure sibling is unaffected — it compiles independently.
	if _, err := emit.CompileProgram(filepath.FromSlash("testdata/javafree/jf/core.clj")); err != nil {
		t.Errorf("a pure sibling must stay usable (per-namespace granularity): %v", err)
	}
}

// TestCertainJavaFlagsStaticNotAmbiguous — the certain-java? courtesy diagnostic
// flags the self-identifying static Java surface (System/…) but NOT the pure,
// Java-flavored surfaces (instance?/catch/class-ref) — zero false positives
// (ADR 0054 decision 4; S35). It only ever upgrades an error message; it is
// never a gate.
func TestCertainJavaFlagsStaticNotAmbiguous(t *testing.T) {
	static, err := CertainJavaFile(filepath.FromSlash("testdata/javastatic/js/core.clj"))
	if err != nil {
		t.Fatalf("CertainJavaFile: %v", err)
	}
	if len(static) != 1 {
		t.Fatalf("want 1 certain-Java diag for System/currentTimeMillis, got %d: %+v", len(static), static)
	}
	if !strings.Contains(static[0].Detail, "System") {
		t.Errorf("diag should name System: %+v", static[0])
	}

	// The Java-flavored-but-pure library must produce ZERO diagnostics.
	free, err := CertainJavaFile(filepath.FromSlash("testdata/javafree/jf/core.clj"))
	if err != nil {
		t.Fatalf("CertainJavaFile(javafree): %v", err)
	}
	if len(free) != 0 {
		t.Errorf("instance?/catch/class-ref must NOT be flagged (zero-FP), got %+v", free)
	}
}
