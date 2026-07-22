package publish

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/reader"
)

type jlabel int

const (
	lPure jlabel = iota
	lGo
	lJava      // certain Java surface — MUST be flagged
	lJavaAmbig // Java but the accepted undecidable dot-form tail — MUST NOT be flagged
)

// corpus is S30's 30-form labeled oracle (proto/main.go:60-96), each java
// entry oracle-confirmed on Clojure 1.12.5. `lJavaAmbig` marks the four bare
// dot-forms S30 accepts as permanent, safe false negatives.
var corpus = []struct {
	src   string
	label jlabel
	note  string
}{
	// ---- pure Clojure (incl. the JVM-class-name traps) ----
	{`(reduce + (map inc [1 2 3]))`, lPure, "arithmetic"},
	{`(clojure.string/upper-case "hi")`, lPure, "clojure.string"},
	{`(let [m {:a 1}] (get m :a))`, lPure, "map access"},
	{`(defn f [x] (* x x))`, lPure, "defn"},
	{`(filter even? (range 10))`, lPure, "seq lib"},
	{`(str "a" "b" "c")`, lPure, "str"},
	{`(instance? String "x")`, lPure, "TRAP: instance? class-syntax"},
	{`(try 1 (catch Exception e 2))`, lPure, "TRAP: catch class name"},
	{`(def x String)`, lPure, "TRAP: bare class-ref value"},
	{`(pr-str java.util.UUID)`, lPure, "TRAP: java.* class-ref value"},

	// ---- Go interop (require-go; not Java) ----
	{`(require-go '[strings :as strs]) (strs/ToUpper "hi")`, lGo, "go ns call"},
	{`(require-go '[strconv :as sc]) (sc/Itoa 42)`, lGo, "go ns call"},
	{`(require-go '[strings]) (def r (strings/NewReplacer "a" "1")) (.Replace r "abc")`, lGo, "go dot-method"},
	{`(require-go '[os]) (os/Getpid)`, lGo, "go ns call"},
	{`(require-go '[math :as m]) (m/Sqrt 2.0)`, lGo, "go Math analog overlap"},

	// ---- Java: certain surfaces (MUST flag) ----
	{`(java.util.UUID/randomUUID)`, lJava, "java.* static call"},
	{`(java.time.Instant/now)`, lJava, "java.* static call"},
	{`(System/currentTimeMillis)`, lJava, "bare JVM class System"},
	{`(Math/sqrt 2)`, lJava, "bare JVM class Math"},
	{`(Thread/sleep 1)`, lJava, "bare JVM class Thread"},
	{`(Integer/parseInt "42")`, lJava, "bare JVM class Integer"},
	{`(String/valueOf 5)`, lJava, "bare JVM class String call-ns"},
	{`(new java.io.File "x")`, lJava, "new + java.* class"},
	{`(import '[java.util Date]) (Date.)`, lJava, "import + ctor"},
	{`(clojure.java.io/file "x")`, lJava, "clojure.java.* ns"},

	// ---- Java: the accepted undecidable dot-form tail (MUST NOT flag) ----
	{`(.toUpperCase "hello")`, lJavaAmbig, "AMBIG dot-method"},
	{`(.getBytes "x")`, lJavaAmbig, "AMBIG dot-method"},
	{`(.length "hello")`, lJavaAmbig, "AMBIG dot-method"},
	{`(defn up [s] (.toUpperCase s))`, lJavaAmbig, "AMBIG dot-method, uncalled"},
}

func readForms(t *testing.T, src string) []any {
	t.Helper()
	forms, err := reader.New(strings.NewReader(src)).ReadAll()
	if err != nil {
		t.Fatalf("read %q: %v", src, err)
	}
	return forms
}

// TestCertainJavaCorpus verifies precision 10/10 (zero false positives) and
// recall 10/14 on the certain surfaces, with the four dot-forms unflagged.
func TestCertainJavaCorpus(t *testing.T) {
	var tp, fp, fn int
	certainTotal := 0

	for _, c := range corpus {
		flagged := len(CertainJava(readForms(t, c.src))) > 0
		switch c.label {
		case lJava:
			certainTotal++
			if flagged {
				tp++
			} else {
				fn++
				t.Errorf("recall miss: certain Java NOT flagged: %q [%s]", c.src, c.note)
			}
		case lJavaAmbig:
			if flagged {
				fp++
				t.Errorf("false positive: ambiguous dot-form flagged: %q [%s]", c.src, c.note)
			}
		case lPure, lGo:
			if flagged {
				fp++
				t.Errorf("false positive: non-Java flagged: %q [%s]", c.src, c.note)
			}
		}
	}

	precDenom := tp + fp
	t.Logf("certain-java? corpus: precision %d/%d, recall %d/%d (of certain), FP=%d FN=%d",
		tp, precDenom, tp, certainTotal, fp, fn)

	if fp != 0 {
		t.Errorf("precision must be perfect (zero FP), got %d false positives", fp)
	}
	if tp != 10 || precDenom != 10 {
		t.Errorf("precision = %d/%d, want 10/10", tp, precDenom)
	}
	if certainTotal != 10 {
		t.Errorf("expected 10 certain-Java forms in corpus, got %d", certainTotal)
	}
}

// TestCertainJavaDetails spot-checks the detail wording for each surface class.
func TestCertainJavaDetails(t *testing.T) {
	cases := []struct{ src, want string }{
		{`(System/getProperty "x")`, "Java static call (System/getProperty)"},
		{`(java.util.UUID/randomUUID)`, "Java package call (java.util.UUID/randomUUID)"},
		{`(clojure.java.io/file "x")`, "clojure.java.* namespace (clojure.java.io/file)"},
		{`(import '[java.util Date])`, "(import …) — JVM-only special form"},
		{`(new java.io.File "x")`, "(new …) — JVM interop special form"},
	}
	for _, c := range cases {
		diags := CertainJava(readForms(t, c.src))
		if len(diags) == 0 {
			t.Errorf("%q: expected a diagnostic, got none", c.src)
			continue
		}
		if diags[0].Detail != c.want {
			t.Errorf("%q: detail = %q, want %q", c.src, diags[0].Detail, c.want)
		}
	}
}

// TestCertainJavaFile checks File+Line tagging via the file entry point.
func TestCertainJavaFile(t *testing.T) {
	path := filepath.FromSlash("testdata/mixed.clj")
	diags, err := CertainJavaFile(path)
	if err != nil {
		t.Fatalf("CertainJavaFile: %v", err)
	}
	// Two certain surfaces: System/currentTimeMillis (line 2) and
	// java.util.UUID/randomUUID (line 5). The (.toUpperCase s) on line 4 is
	// the accepted miss — not flagged.
	if len(diags) != 2 {
		t.Fatalf("want 2 diags, got %d: %+v", len(diags), diags)
	}
	for _, d := range diags {
		if d.File != path {
			t.Errorf("diag File = %q, want %q", d.File, path)
		}
	}
	if diags[0].Line != 2 {
		t.Errorf("first diag line = %d, want 2", diags[0].Line)
	}
	if diags[1].Line != 5 {
		t.Errorf("second diag line = %d, want 5", diags[1].Line)
	}
	// The dot-form on line 4 must be absent.
	for _, d := range diags {
		if d.Line == 4 {
			t.Errorf("line-4 dot-form must NOT be flagged, got %+v", d)
		}
	}
}
