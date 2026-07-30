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
