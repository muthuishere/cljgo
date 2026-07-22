// Spike S35 prototype — is `uses-java?` a sound, low-false-positive predicate
// on a Go host with no Java? Throwaway (ADR 0027). READS pkg/reader + pkg/lang
// only; never modifies pkg/.
//
// It scores THREE predicates over a labeled corpus (pure / go-interop /
// java-interop, each true label oracle-confirmed against the `clojure` CLI):
//
//	P1 javaSyntactic   — reader-level JVM-marker scan. The RECOMMENDED shape:
//	                     certain-only, position-aware, zero false positives,
//	                     NEVER guesses the ambiguous bare (.method obj).
//	P2 resolutionOutcome — what cljgo actually does today (analysis error /
//	                       runtime method miss / ok), i.e. the resolution signal
//	P3 usesHostInterop — the sound decidable BOUNDARY: ANY host-interop shape
//	                     (Java OR Go), reader-level + resolution union. Flags
//	                     bare dot-forms by guessing — the guess §5 forbids.
//
// and prints a confusion table + false positives/negatives with causes.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

type label int

const (
	pure label = iota
	goInterop
	javaInterop
)

func (l label) String() string {
	switch l {
	case pure:
		return "pure"
	case goInterop:
		return "go-interop"
	default:
		return "java-interop"
	}
}

type sample struct {
	src   string
	label label // oracle-confirmed truth (JVM meaning)
	note  string
}

// Labeled corpus. Every java-interop entry was run on the real `clojure` CLI
// 1.12.5 and confirmed to be genuine, working JVM interop (see VERDICT §oracle).
// Every go-interop entry uses cljgo-only require-go (no JVM meaning by
// construction). Every pure entry runs identically on the JVM.
var corpus = []sample{
	// ---- pure Clojure ----
	{`(reduce + (map inc [1 2 3]))`, pure, "arithmetic"},
	{`(clojure.string/upper-case "hi")`, pure, "clojure.string (pure ns)"},
	{`(let [m {:a 1}] (get m :a))`, pure, "map access"},
	{`(defn f [x] (* x x))`, pure, "defn"},
	{`(filter even? (range 10))`, pure, "seq lib"},
	{`(str "a" "b" "c")`, pure, "str"},
	// pure BUT uses JVM class NAMES in cljgo-native positions — the overlap trap
	{`(instance? String "x")`, pure, "TRAP: instance? class-syntax (ADR 0026)"},
	{`(try 1 (catch Exception e 2))`, pure, "TRAP: catch class name (host-neutral)"},
	{`(def x String)`, pure, "TRAP: bare class ref value (ADR 0036 ClassRef)"},
	{`(pr-str java.util.UUID)`, pure, "TRAP: java.* ClassRef as value (ADR 0036)"},

	// ---- Go interop (cljgo-only; impure by ADR 0048 §6, but NOT java) ----
	{`(require-go '[strings :as strs]) (strs/ToUpper "hi")`, goInterop, "go ns call"},
	{`(require-go '[strconv :as sc]) (sc/Itoa 42)`, goInterop, "go ns call"},
	{`(require-go '[strings]) (def r (strings/NewReplacer "a" "1")) (.Replace r "abc")`, goInterop, "go dot-method (host-neutral shape)"},
	{`(require-go '[os]) (os/Getpid)`, goInterop, "go ns call"},
	{`(require-go '[math :as m]) (m/Sqrt 2.0)`, goInterop, "go Math analog (overlap w/ JVM Math)"},

	// ---- Java interop (real JVM; unsupported on cljgo) ----
	{`(java.util.UUID/randomUUID)`, javaInterop, "java.* static call"},
	{`(java.time.Instant/now)`, javaInterop, "java.* static call"},
	{`(System/currentTimeMillis)`, javaInterop, "bare JVM class System"},
	{`(Math/sqrt 2)`, javaInterop, "bare JVM class Math (overlap trap)"},
	{`(Thread/sleep 1)`, javaInterop, "bare JVM class Thread"},
	{`(Integer/parseInt "42")`, javaInterop, "bare JVM class Integer"},
	{`(String/valueOf 5)`, javaInterop, "bare JVM class String (call ns)"},
	{`(new java.io.File "x")`, javaInterop, "new + java.* class"},
	{`(import '[java.util Date]) (Date.)`, javaInterop, "import + ctor"},
	{`(clojure.java.io/file "x")`, javaInterop, "clojure.java.* ns"},
	{`(.toUpperCase "hello")`, javaInterop, "AMBIG: java dot-method (host-neutral shape)"},
	{`(.getBytes "x")`, javaInterop, "AMBIG: java dot-method"},
	{`(.length "hello")`, javaInterop, "AMBIG: java dot-method"},
	{`(defn up [s] (.toUpperCase s))`, javaInterop, "AMBIG: java dot-method, uncalled"},
}

// ---------- P1: syntactic JVM-marker scan (reader level, no resolution) ----------

var javaPkgRe = regexp.MustCompile(`^(java|javax)\.`)

// Bare JVM classes with a static-member surface that appear as a call
// namespace `Class/member`. Deliberately does NOT include the ADR 0036
// class-ref value vocabulary used by instance?/catch — those are pure.
var jvmBareClassNS = map[string]bool{
	"System": true, "Math": true, "Thread": true, "Integer": true, "Long": true,
	"Double": true, "Float": true, "Boolean": true, "Character": true, "Byte": true,
	"Short": true, "String": true, "Object": true, "Runtime": true, "Class": true,
	"Number": true, "StringBuilder": true, "StringBuffer": true, "Arrays": true,
	"Collections": true, "Objects": true,
}

// javaSyntactic walks a form tree flagging explicit JVM lexical markers.
// Returns (flagged, reason). It does NOT flag bare dot-methods (host-neutral).
func javaSyntactic(form any) (bool, string) {
	head := headSym(form)
	if head != nil {
		switch head.Name() {
		case "import":
			return true, "(import ...) — JVM-only special form"
		case "new":
			return true, "(new ...) — JVM interop special form"
		}
	}
	found := ""
	walk(form, func(v any) {
		if found != "" {
			return
		}
		s, ok := v.(*lang.Symbol)
		if !ok {
			return
		}
		// java.*/javax.* as a CALL namespace (java.util.UUID/randomUUID) — an
		// interop EXECUTION. A bare java.* VALUE with no namespace is an ADR
		// 0036 ClassRef (a pure opaque constant), so it is deliberately NOT
		// flagged here — position-awareness is what removes the ClassRef-value
		// false positive (see VERDICT §refinement).
		if s.HasNamespace() && javaPkgRe.MatchString(s.Namespace()) {
			found = "java/javax package call-ns: " + s.Namespace()
			return
		}
		// clojure.java.* require target (clojure.java.io/file)
		if s.HasNamespace() && strings.HasPrefix(s.Namespace(), "clojure.java.") {
			found = "clojure.java.* ns: " + s.Namespace()
			return
		}
		if !s.HasNamespace() && strings.HasPrefix(s.Name(), "clojure.java.") {
			found = "clojure.java.* ref: " + s.Name()
			return
		}
		// bare JVM class as a call namespace (System/currentTimeMillis)
		if s.HasNamespace() && jvmBareClassNS[s.Namespace()] {
			found = "bare JVM class call-ns: " + s.Namespace()
			return
		}
	})
	if found != "" {
		return true, found
	}
	return false, ""
}

// ---------- P3: host-interop shape detection (Java OR Go) ----------

// hostInteropShape flags any reader-visible host-interop SHAPE: dot-method,
// dot-field, ctor, require-go, plus everything javaSyntactic flags. It is the
// reader-level half of the recommended purity predicate; the resolution half
// (unresolved `Ns/member`) is supplied by cljgo's analysis outcome (P2).
func hostInteropShape(form any) (bool, string) {
	if ok, why := javaSyntactic(form); ok {
		return true, why
	}
	head := headSym(form)
	if head != nil {
		n := head.Name()
		if n == "require-go" {
			return true, "require-go (Go host capability)"
		}
	}
	found := ""
	walk(form, func(v any) {
		if found != "" {
			return
		}
		seq, ok := v.(lang.ISeq)
		if !ok || seq == nil {
			return
		}
		h, ok := seq.First().(*lang.Symbol)
		if !ok || h.HasNamespace() {
			return
		}
		n := h.Name()
		switch {
		case n == "require-go":
			found = "require-go"
		case strings.HasPrefix(n, ".-") && len(n) > 2:
			found = "dot-field access " + n + " (host, provenance unknown)"
		case len(n) >= 2 && n[0] == '.' && n[1] != '-' && n != "..":
			found = "dot-method call " + n + " (host, provenance unknown)"
		case len(n) > 1 && strings.HasSuffix(n, ".") && !strings.HasSuffix(n, ".."):
			found = "ctor " + n + " (host, provenance unknown)"
		}
	})
	if found != "" {
		return true, found
	}
	return false, ""
}

// ---------- P2: what cljgo does today (the resolution signal) ----------

type outcome int

const (
	ok                 outcome = iota
	analysisResolveErr         // "no such namespace" / "unable to resolve symbol import/new"
	runtimeMethodErr           // "no method X on Y" — dot-form host miss at runtime
	otherErr
)

func (o outcome) String() string {
	switch o {
	case ok:
		return "ok"
	case analysisResolveErr:
		return "ANALYSIS-resolve-err"
	case runtimeMethodErr:
		return "RUNTIME-method-err"
	default:
		return "other-err"
	}
}

var noMethodRe = regexp.MustCompile(`no method \S+ on `)
var resolveRe = regexp.MustCompile(`no such namespace|unable to resolve symbol|could not locate namespace`)

func runCljgo(src string) (outcome, string) {
	cmd := exec.Command("cljgo", "run", "/dev/stdin")
	cmd.Stdin = strings.NewReader(src)
	out, err := cmd.CombinedOutput()
	s := string(out)
	if err == nil {
		return ok, strings.TrimSpace(s)
	}
	switch {
	case noMethodRe.MatchString(s):
		return runtimeMethodErr, strings.TrimSpace(s)
	case resolveRe.MatchString(s):
		return analysisResolveErr, strings.TrimSpace(s)
	default:
		return otherErr, strings.TrimSpace(s)
	}
}

// ---------- reader helpers ----------

func readForms(src string) []any {
	r := reader.New(strings.NewReader(src))
	forms, _ := r.ReadAll()
	return forms
}

func headSym(form any) *lang.Symbol {
	seq, ok := form.(lang.ISeq)
	if !ok || seq == nil {
		return nil
	}
	s, _ := seq.First().(*lang.Symbol)
	return s
}

// walk visits every node (symbols, seqs, vectors, maps) depth-first.
func walk(form any, visit func(any)) {
	visit(form)
	switch v := form.(type) {
	case lang.ISeq:
		for s := v; s != nil; s = s.Next() {
			if s.First() == form {
				break
			}
			walk(s.First(), visit)
		}
	case lang.IPersistentVector:
		for i := 0; i < v.Count(); i++ {
			walk(v.Nth(i), visit)
		}
	}
}

func anyForm(src string, pred func(any) (bool, string)) (bool, string) {
	for _, f := range readForms(src) {
		if ok, why := pred(f); ok {
			return true, why
		}
	}
	return false, ""
}

// ---------- scoring ----------

func main() {
	fmt.Println("S35 — uses-java? predicate evaluation over labeled corpus")
	fmt.Println(strings.Repeat("=", 78))

	// Predicate scores. Positive class for P1 = java-interop.
	var p1tp, p1fp, p1fn int
	// P3 positive class = host-interop (go OR java). pure = negative.
	var p3fp, p3fn int

	type row struct {
		s      sample
		p1     bool
		p1why  string
		p3     bool
		p3why  string
		out    outcome
		outmsg string
	}
	var rows []row

	for _, s := range corpus {
		p1, p1why := anyForm(s.src, javaSyntactic)
		// P3 host-interop = reader shape OR resolution says unresolved host call.
		p3shape, p3why := anyForm(s.src, hostInteropShape)
		out, outmsg := runCljgo(s.src)
		p3 := p3shape || out == analysisResolveErr || out == runtimeMethodErr
		if !p3shape && (out == analysisResolveErr || out == runtimeMethodErr) {
			p3why = "resolution: " + out.String()
		}
		rows = append(rows, row{s, p1, p1why, p3, p3why, out, outmsg})

		// P1 scoring (positive = java-interop)
		isJava := s.label == javaInterop
		switch {
		case p1 && isJava:
			p1tp++
		case p1 && !isJava:
			p1fp++
		case !p1 && isJava:
			p1fn++
		}
		// P3 scoring (positive = host-interop i.e. NOT pure)
		isHost := s.label != pure
		switch {
		case p3 && !isHost:
			p3fp++
		case !p3 && isHost:
			p3fn++
		}
	}

	fmt.Printf("\n%-46s %-5s %-4s %-4s %s\n", "snippet (truncated)", "truth", "P1", "P3", "cljgo-outcome")
	fmt.Println(strings.Repeat("-", 78))
	for _, r := range rows {
		src := strings.ReplaceAll(r.s.src, "\n", " ")
		if len(src) > 44 {
			src = src[:41] + "..."
		}
		mark := func(b bool) string {
			if b {
				return "Y"
			}
			return "."
		}
		fmt.Printf("%-46s %-5s %-4s %-4s %s\n", src,
			abbr(r.s.label), mark(r.p1), mark(r.p3), r.out)
	}

	fmt.Println("\n" + strings.Repeat("=", 78))
	fmt.Println("P1  javaSyntactic  (positive class = java-interop)")
	fmt.Printf("    true-pos=%d  false-pos=%d  false-neg=%d\n", p1tp, p1fp, p1fn)
	fmt.Printf("    recall (java caught) = %d/%d ;  precision = %d/%d\n",
		p1tp, p1tp+p1fn, p1tp, p1tp+p1fp)
	fmt.Println("\n  P1 false negatives (java NOT caught by syntax):")
	for _, r := range rows {
		if r.s.label == javaInterop && !r.p1 {
			fmt.Printf("    - %q  [%s]\n", oneLine(r.s.src), r.s.note)
		}
	}
	fmt.Println("  P1 false positives (non-java flagged as java):")
	for _, r := range rows {
		if r.s.label != javaInterop && r.p1 {
			fmt.Printf("    - %q  reason: %s\n", oneLine(r.s.src), r.p1why)
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 78))
	fmt.Println("P3  usesHostInterop  (positive class = host-interop, Java OR Go)")
	fmt.Printf("    false-pos(pure flagged)=%d  false-neg(host missed)=%d\n", p3fp, p3fn)
	fmt.Println("\n  P3 false negatives (host interop missed):")
	n := 0
	for _, r := range rows {
		if r.s.label != pure && !r.p3 {
			fmt.Printf("    - %q\n", oneLine(r.s.src))
			n++
		}
	}
	if n == 0 {
		fmt.Println("    (none)")
	}
	fmt.Println("  P3 false positives (pure flagged as host):")
	n = 0
	for _, r := range rows {
		if r.s.label == pure && r.p3 {
			fmt.Printf("    - %q  reason: %s\n", oneLine(r.s.src), r.p3why)
			n++
		}
	}
	if n == 0 {
		fmt.Println("    (none)")
	}

	os.Exit(0)
}

func abbr(l label) string {
	switch l {
	case pure:
		return "pure"
	case goInterop:
		return "go"
	default:
		return "java"
	}
}
func oneLine(s string) string { return strings.ReplaceAll(s, "\n", " ") }
