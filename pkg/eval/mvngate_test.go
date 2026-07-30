package eval_test

// The END-TO-END proof for ADR 0095: a Maven-origin dependency root is mounted
// on the load path (slot 3), a PURE namespace out of it is actually required
// and its vars are callable, and the Java / starved-conditional namespaces
// beside it in the SAME library fail loud at the require with their registered
// diagnostic codes.
//
// This is the per-NAMESPACE granularity ADR 0054 decision 4 mandates, exercised
// through the real loader — not a unit test of the classifier.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/deps"
	"github.com/muthuishere/cljgo/pkg/diag"
	"github.com/muthuishere/cljgo/pkg/eval"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// mountMavenLib writes a fake extracted Maven tree, mounts it as a resolved
// root, classifies it exactly as the resolver does, and publishes the index.
func mountMavenLib(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	deps.SetResolvedRoots([]string{dir})
	verdicts, err := deps.ClassifyMavenTree(dir, deps.Coord{Group: "demo", Artifact: "demo", Version: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	deps.SetMavenIndex(verdicts)
	t.Cleanup(func() {
		deps.SetResolvedRoots(nil)
		deps.SetMavenIndex(nil)
	})
	return dir
}

const demoLibPure = `(ns demo.pure)

(defn greet [who] (str "hello, " who))
`

const demoLibJava = `(ns demo.javaish
  (:import [java.io StringWriter]))

(defn render [x] (str x))
`

const demoLibStarved = `(ns demo.starved)

#?(:clj  (def impl :jvm)
   :cljs (def impl :browser))
`

func mavenDemoLib() map[string]string {
	return map[string]string{
		"demo/pure.clj":     demoLibPure,
		"demo/javaish.clj":  demoLibJava,
		"demo/starved.cljc": demoLibStarved,
	}
}

// TestMavenPureNamespaceLoadsEndToEnd — the headline: a pure-Clojure namespace
// from a Maven dependency is required and its function runs.
func TestMavenPureNamespaceLoadsEndToEnd(t *testing.T) {
	mountMavenLib(t, mavenDemoLib())
	e := eval.New()

	evalAll(t, e, list(sym("require"), list(sym("quote"), sym("demo.pure"))))
	got := evalAll(t, e, list(sym("demo.pure/greet"), "world"))
	if got != "hello, world" {
		t.Fatalf("want %q, got %#v", "hello, world", got)
	}
}

// TestMavenJavaNamespaceFailsLoudAtRequire — the gate fires at REQUIRE (not at
// resolve), names the namespace, the coordinate and the offending form, and
// says how many other namespaces in the same library are still usable.
func TestMavenJavaNamespaceFailsLoudAtRequire(t *testing.T) {
	mountMavenLib(t, mavenDemoLib())
	e := eval.New()

	err := mustErr(t, e, list(sym("require"), list(sym("quote"), sym("demo.javaish"))))
	rendered := diag.RenderError(err)
	if !strings.Contains(rendered, "I4002") {
		t.Fatalf("want the I4002 diagnostic, got:\n%s", rendered)
	}
	for _, want := range []string{"demo.javaish", "demo/demo 1.0.0", ":import", "other namespace"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("the require-time error is missing %q:\n%s", want, rendered)
		}
	}
	// It did NOT install a broken namespace.
	if ns := lang.FindNamespace(lang.NewSymbol("demo.javaish")); ns != nil {
		t.Error("a Java-tainted namespace was installed anyway")
	}
}

// TestMavenStarvedNamespaceFailsLoudAtRequire — a .cljc whose real body is
// :clj-only must fail, never install an empty namespace (s50 finding 4).
func TestMavenStarvedNamespaceFailsLoudAtRequire(t *testing.T) {
	mountMavenLib(t, mavenDemoLib())
	e := eval.New()

	err := mustErr(t, e, list(sym("require"), list(sym("quote"), sym("demo.starved"))))
	rendered := diag.RenderError(err)
	if !strings.Contains(rendered, "R1012") {
		t.Fatalf("want the R1012 diagnostic, got:\n%s", rendered)
	}
	if ns := lang.FindNamespace(lang.NewSymbol("demo.starved")); ns != nil {
		t.Error("a starved namespace was installed with no vars — the exact trap s50 forbids")
	}
}

// TestMavenGateIsNotBypassedByUse — the gate must be unbypassable by
// CONSTRUCTION, not by inspection. `(use 'demo.javaish)` and
// `(ns … (:use demo.javaish))` used to sail straight past it: the `ns` macro
// dropped every clause that was not :require, so a namespace the resolver had
// classified :java reached a compiled binary having never met the gate, while
// the equivalent :require correctly raised I4002. That is the real shape
// hiccup uses (hiccup.core is `(:use hiccup.compiler hiccup.util)`).
func TestMavenGateIsNotBypassedByUse(t *testing.T) {
	for _, tc := range []struct {
		name string
		form any
	}{
		{"use", list(sym("use"), list(sym("quote"), sym("demo.javaish")))},
		{"ns :use", list(sym("ns"), sym("app.viause"), list(kw("use"), sym("demo.javaish")))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mountMavenLib(t, mavenDemoLib())
			e := eval.New()

			err := mustErr(t, e, tc.form)
			rendered := diag.RenderError(err)
			if !strings.Contains(rendered, "I4002") {
				t.Fatalf("a :java namespace reached the program through %s ungated:\n%s", tc.name, rendered)
			}
			if !strings.Contains(rendered, "demo.javaish") {
				t.Errorf("the error does not name the namespace:\n%s", rendered)
			}
			if ns := lang.FindNamespace(lang.NewSymbol("demo.javaish")); ns != nil {
				t.Error("a Java-tainted namespace was installed anyway")
			}
		})
	}
}

// TestNsUseLoadsAndRefersAPureNamespace — the gate fix must not have made
// :use a no-op in the other direction: a PURE namespace pulled in with :use
// still refers its publics, as JVM Clojure does.
func TestNsUseLoadsAndRefersAPureNamespace(t *testing.T) {
	mountMavenLib(t, mavenDemoLib())
	e := eval.New()

	evalAll(t, e, list(sym("ns"), sym("app.viausepure"), list(kw("use"), sym("demo.pure"))))
	got := evalAll(t, e, list(sym("greet"), "world"))
	if got != "hello, world" {
		t.Fatalf(":use did not refer the pure namespace's vars: %#v", got)
	}
}

// TestNonMavenSourceIsNeverGated — project code and git deps must be entirely
// unaffected, even when they contain the very forms the gate flags.
func TestNonMavenSourceIsNeverGated(t *testing.T) {
	// Mount the library WITHOUT publishing a maven index: the same files, now
	// ordinary source roots.
	dir := t.TempDir()
	p := filepath.Join(dir, "plain", "ns.clj")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("(ns plain.ns)\n\n(defn f [] 1)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deps.SetResolvedRoots([]string{dir})
	deps.SetMavenIndex(nil)
	t.Cleanup(func() { deps.SetResolvedRoots(nil) })

	e := eval.New()
	evalAll(t, e, list(sym("require"), list(sym("quote"), sym("plain.ns"))))
	if got := evalAll(t, e, list(sym("plain.ns/f"))); got != int64(1) {
		t.Fatalf("want 1, got %#v", got)
	}
}

// demoLibGapped is the case real Clojars exposed and httptest fixtures never
// could: a namespace with NO Java interop, which reads cleanly, and which
// cljgo still cannot compile. (Live it was medley.core's ordinary
// `(defn name "doc" {:attr-map} ...)`; here it is an unresolvable symbol, so
// the test does not depend on which cljgo gap is currently open.)
const demoLibGapped = `(ns demo.gapped)

(defn f [x] (a-symbol-cljgo-cannot-resolve x))
`

// TestMavenInteropFreeButUncompilableRaisesG5020 — the report says "N
// namespace(s) with no Java interop"; when one of those N fails anyway, the
// user must not get a bare compile error with nothing joining the two
// statements. The require raises G5020, which names the measurement that
// passed, what actually failed, and that the gap is cljgo's.
func TestMavenInteropFreeButUncompilableRaisesG5020(t *testing.T) {
	files := mavenDemoLib()
	files["demo/gapped.clj"] = demoLibGapped
	dir := mountMavenLib(t, files)

	// It really did pass classification — that is the whole point.
	v, ok := deps.MavenVerdictFor(filepath.Join(dir, "demo", "gapped.clj"))
	if !ok || !v.InteropFree() {
		t.Fatalf("the fixture was supposed to be classified interop-free: %+v (found=%v)", v, ok)
	}

	e := eval.New()
	err := mustErr(t, e, list(sym("require"), list(sym("quote"), sym("demo.gapped"))))
	rendered := diag.RenderError(err)
	for _, want := range []string{
		"G5020",
		"demo.gapped",
		"demo/demo 1.0.0",
		"interop-free",
		"does not compile on cljgo",
		"READ-time measurement",
		"gap in cljgo",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("the failure does not connect back to the resolve report, missing %q:\n%s", want, rendered)
		}
	}
	// It must NOT be reported as a Java-interop verdict: that is a claim about
	// the library, and this is a claim about cljgo.
	for _, forbidden := range []string{"I4002", "requires Java interop"} {
		if strings.Contains(rendered, forbidden) {
			t.Errorf("a cljgo gap was reported as %q — a false statement about the library:\n%s", forbidden, rendered)
		}
	}
}

// TestPureMavenNamespaceIsNotWrappedByG5020 — the wrapper must not fire for a
// namespace that loads fine, and must not change the healthy path's behaviour.
func TestPureMavenNamespaceIsNotWrappedByG5020(t *testing.T) {
	files := mavenDemoLib()
	files["demo/gapped.clj"] = demoLibGapped
	mountMavenLib(t, files)
	e := eval.New()

	evalAll(t, e, list(sym("require"), list(sym("quote"), sym("demo.pure"))))
	if got := evalAll(t, e, list(sym("demo.pure/greet"), "world")); got != "hello, world" {
		t.Fatalf("the G5020 wrapper broke a healthy maven namespace: %#v", got)
	}
}
