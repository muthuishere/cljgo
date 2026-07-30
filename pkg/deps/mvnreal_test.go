package deps

// Tests for the six defects an adversarial verifier found by taking the
// implementation to the REAL Clojars/Maven Central. Every fixture here mirrors
// an artifact that actually exists; none of these tests touches the network
// (the guard client in mvnhelper_test.go enforces that).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/diag"
)

// --- defect 1: <parent> POM inheritance ------------------------------------

// TestContribParentPOMResolves is the test the original fixture could not have
// failed, because the fixture called "the tools.cli shape" did not have
// tools.cli's shape. The real artifact inherits its groupId from
// org.clojure/pom.contrib, takes ${clojure.version} from the parent's
// <properties>, inherits the parent's dependency on org.clojure/clojure, and
// carries a build-only <profile>. Every one of those was, until now, a G5011
// that made the ENTIRE org.clojure contrib set unconsumable.
func TestContribParentPOMResolves(t *testing.T) {
	r := newMvnRepo(t)
	r.putContribParent("")
	c := Coord{Group: "org.clojure", Artifact: "tools.cli", Version: "1.1.230"}
	r.putContribPOM(c, contribParent, "")
	r.put(c.artifactPath(".jar"), buildJar(t, pureToolsCLI()))

	res := mustResolve(t, r, []Dep{{Name: "org.clojure/tools.cli", MvnVersion: "1.1.230"}}, nil)

	// The clojure edge came from the PARENT at ${clojure.version} and is
	// pruned, so nothing else is fetched.
	if len(res.Lock.Deps) != 1 {
		t.Fatalf("want exactly the one coordinate, got %d: %+v", len(res.Lock.Deps), res.Lock.Deps)
	}
	lk := res.Lock.find("org.clojure/tools.cli")
	if lk == nil || len(lk.MvnPureNS) != 1 || lk.MvnPureNS[0] != "clojure.tools.cli" {
		t.Fatalf("tools.cli did not resolve to one usable namespace: %+v", lk)
	}
	joined := strings.Join(res.MavenReport, "\n")
	// Inheritance is REPORTED, never silent — the same rule as the prune.
	if !strings.Contains(joined, "inherited from the <parent> POM org.clojure/pom.contrib 1.2.0") {
		t.Errorf("the parent merge was silent:\n%s", joined)
	}
	if !strings.Contains(joined, "pruned org.clojure/clojure") {
		t.Errorf("the inherited clojure edge was not pruned+reported:\n%s", joined)
	}
}

// TestParentSuppliesPropertyAndManagedVersion proves the two things a parent
// is actually FOR: a <properties> entry that interpolates a child's ${…}
// version, and a <dependencyManagement> entry that supplies a version the
// child omits.
func TestParentSuppliesPropertyAndManagedVersion(t *testing.T) {
	r := newMvnRepo(t)
	r.putContribParent(`<dependencyManagement><dependencies>` +
		depXML("managed", "managed", "3.0.0") + `</dependencies></dependencyManagement>`)
	c := Coord{Group: "org.clojure", Artifact: "child", Version: "1.0.0"}
	// ${clojure.version} is defined only by the PARENT's <properties>; the
	// child's own block redefines it, and the child must win.
	r.putContribPOM(c, contribParent, depsXML(
		depXML("lib", "lib", "${clojure.version}"),
		"<dependency><groupId>managed</groupId><artifactId>managed</artifactId></dependency>",
	))
	r.put(c.artifactPath(".jar"), buildJar(t, map[string]string{"child/core.clj": "(ns child.core)\n"}))
	r.publish(Coord{"lib", "lib", "1.9.0"}, "", map[string]string{"lib/core.clj": "(ns lib.core)\n"})
	r.publish(Coord{"managed", "managed", "3.0.0"}, "", map[string]string{"managed/core.clj": "(ns managed.core)\n"})

	res := mustResolve(t, r, []Dep{{Name: "org.clojure/child", MvnVersion: "1.0.0"}}, nil)
	if len(res.Lock.Deps) != 3 {
		t.Fatalf("want child + lib + managed, got %d: %+v", len(res.Lock.Deps), res.Lock.Deps)
	}
	if lk := res.Lock.find("lib/lib"); lk == nil || lk.MvnVersion != "1.9.0" {
		t.Errorf("the property did not interpolate the child's version: %+v", lk)
	}
	if lk := res.Lock.find("managed/managed"); lk == nil || lk.MvnVersion != "3.0.0" {
		t.Errorf("<dependencyManagement> did not supply the missing version: %+v", lk)
	}
}

// TestBuildOnlyProfileIsNotRefused: pom.contrib's gpg-signing profile touches
// only <build>. Refusing on it would again exclude every contrib artifact, and
// a build-plugin profile provably cannot change what we resolve.
func TestBuildOnlyProfileIsNotRefused(t *testing.T) {
	r := newMvnRepo(t)
	r.putContribParent("")
	c := Coord{Group: "org.clojure", Artifact: "data.csv", Version: "1.1.0"}
	r.putContribPOM(c, contribParent, "")
	r.put(c.artifactPath(".jar"), buildJar(t, map[string]string{"clojure/data/csv.clj": "(ns clojure.data.csv)\n"}))

	mustResolve(t, r, []Dep{{Name: "org.clojure/data.csv", MvnVersion: "1.1.0"}}, nil)
}

// TestUnresolvableParentNamesTheChild: a parent is a fetched artifact like any
// other, so a missing one is G5010 — but it must say WHOSE parent it is, or
// the user sees a 404 for a coordinate they never wrote down.
func TestUnresolvableParentNamesTheChild(t *testing.T) {
	r := newMvnRepo(t)
	c := Coord{Group: "org.clojure", Artifact: "tools.cli", Version: "1.1.230"}
	r.putContribPOM(c, contribParent, "") // the parent is NOT published
	r.put(c.artifactPath(".jar"), buildJar(t, pureToolsCLI()))

	err := resolveErr(t, r, []Dep{{Name: "org.clojure/tools.cli", MvnVersion: "1.1.230"}}, nil)
	wantCode(t, err, "G5010")
	for _, want := range []string{"org.clojure/pom.contrib 1.2.0", "<parent> POM of", "org.clojure/tools.cli 1.1.230"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the parent-fetch failure does not say %q:\n%s", want, err)
		}
	}
}

// TestParentChainIsBounded: a self-referential parent must be a named error,
// never an unbounded fetch loop.
func TestParentChainIsBounded(t *testing.T) {
	r := newMvnRepo(t)
	c := Coord{Group: "p", Artifact: "p", Version: "1.0.0"}
	r.putRawPOM(c, `<project><parent><groupId>p</groupId><artifactId>p</artifactId><version>1.0.0</version></parent>
	  <groupId>p</groupId><artifactId>p</artifactId><version>1.0.0</version></project>`)
	r.put(c.artifactPath(".jar"), buildJar(t, map[string]string{"p/core.clj": "(ns p.core)\n"}))

	err := resolveErr(t, r, []Dep{{Name: "p", MvnVersion: "1.0.0"}}, nil)
	wantCode(t, err, "G5011")
	if !strings.Contains(err.Error(), "cyclic <parent> chain") {
		t.Errorf("a cyclic parent chain was not named:\n%s", err)
	}
}

// TestRealToolsCLIReaderConditionalsStayUsable — the shapes the live run
// tripped on, which the original one-namespace fixture had none of: a
// top-level `#?(:cljs …)` helper (elided by the JVM too) and a starved `#?@`
// SPLICE inside a `let` binding vector (contributes zero elements, so the
// vector stays even). Neither makes the namespace unusable, and both used to.
func TestRealToolsCLIReaderConditionalsStayUsable(t *testing.T) {
	r := newMvnRepo(t)
	r.putContribParent("")
	c := Coord{Group: "org.clojure", Artifact: "tools.cli", Version: "1.1.230"}
	r.putContribPOM(c, contribParent, "")
	r.put(c.artifactPath(".jar"), buildJar(t, realToolsCLIShape()))

	res := mustResolve(t, r, []Dep{{Name: "org.clojure/tools.cli", MvnVersion: "1.1.230"}}, nil)
	lk := res.Lock.find("org.clojure/tools.cli")
	if lk == nil || len(lk.MvnPureNS) != 1 {
		t.Fatalf("real tools.cli's reader conditionals made it unusable: %+v", lk)
	}
	SetMavenIndex(res.MavenVerdicts)
	t.Cleanup(func() { SetMavenIndex(nil) })
	if err := CheckMavenLoadable(filepath.Join(res.Roots[0], "clojure", "tools", "cli.cljc")); err != nil {
		t.Fatalf("real tools.cli was gated: %v", err)
	}
	// The elision is REPORTED, not glossed: "it loads" and "it is the same
	// namespace you get on the JVM" are different claims.
	if !strings.Contains(strings.Join(res.MavenReport, "\n"), "elided a top-level form") {
		t.Errorf("the elision was silent:\n%v", res.MavenReport)
	}
}

// TestShapeBreakingElisionIsNotUsable — the other half: a NON-splicing starved
// conditional inside a binding vector (real camel-snake-kebab 0.4.3
// internals/string_separator.cljc:44, real medley 1.8.1 core.cljc:456) silently
// changes the vector's length. Calling it usable handed the user a compiler
// error naming a library they never wrote.
func TestShapeBreakingElisionIsNotUsable(t *testing.T) {
	r := newMvnRepo(t)
	c := Coord{Group: "csk", Artifact: "csk", Version: "0.4.3"}
	r.publish(c, "", map[string]string{
		"csk/sep.cljc": `(ns csk.sep)

(defn generic-split [ss]
  (let [cs (mapv identity ss)
        ss-length #?(:clj (.length ss) :cljs (.-length ss))]
    [cs ss-length]))
`})

	res := mustResolve(t, r, []Dep{{Name: "csk", MvnVersion: "0.4.3"}}, nil)
	lk := res.Lock.find("csk")
	if lk == nil || len(lk.MvnPureNS) != 0 {
		t.Fatalf("a shape-breaking elision was called usable: %+v", lk)
	}
	SetMavenIndex(res.MavenVerdicts)
	t.Cleanup(func() { SetMavenIndex(nil) })
	err := CheckMavenLoadable(filepath.Join(res.Roots[0], "csk", "sep.cljc"))
	if err == nil {
		t.Fatal("a structurally-broken namespace was not gated")
	}
	wantCode(t, err, "R1012")
	if !strings.Contains(err.Error(), "would change the shape of the enclosing vector") {
		t.Errorf("R1012 does not say what went wrong:\n%s", err)
	}
}

// --- defect 4: the USER's declared version syntax --------------------------

// TestDeclaredVersionSyntaxIsValidated: these used to produce G5010 "not found
// in any repository", blaming the repository for the user's syntax, because
// validateEdgeVersion was called only from pomChildren.
func TestDeclaredVersionSyntaxIsValidated(t *testing.T) {
	for _, tc := range []struct{ version, want string }{
		{"1.0-SNAPSHOT", "-SNAPSHOT version"},
		{"[1.0,2.0)", "version range"},
		{"LATEST", "floating meta-version LATEST"},
		{"RELEASE", "floating meta-version RELEASE"},
		{"${lib.version}", "${property} placeholder"},
	} {
		t.Run(tc.version, func(t *testing.T) {
			r := newMvnRepo(t)
			err := resolveErr(t, r, []Dep{{Name: "some/lib", MvnVersion: tc.version}}, nil)
			wantCode(t, err, "G5016")
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("G5016 does not name the syntax %q:\n%s", tc.want, err)
			}
			if !strings.Contains(err.Error(), "some/lib") {
				t.Errorf("G5016 does not name the dependency:\n%s", err)
			}
			if strings.Contains(err.Error(), "not found in any repository") {
				t.Errorf("the repository was blamed for the user's syntax:\n%s", err)
			}
		})
	}
}

// --- defect 2: the JVM-only false positive ---------------------------------

// TestAutoResolvedKeywordsAreNotJavaInterop is the regression test for the
// worst defect found: a 100%-pure library (com.stuartsierra/dependency) was
// reported as "requires Java interop … it is a JVM-only library" purely
// because classifyFile built its reader with no resolver, so every ::kw was a
// read error and every read error became I4002.
func TestAutoResolvedKeywordsAreNotJavaInterop(t *testing.T) {
	r := newMvnRepo(t)
	c := Coord{Group: "com.stuartsierra", Artifact: "dependency", Version: "1.0.0"}
	r.publish(c, "", pureAutoResolvedKeywords())

	res := mustResolve(t, r, []Dep{{Name: "com.stuartsierra/dependency", MvnVersion: "1.0.0"}}, nil)
	lk := res.Lock.find("com.stuartsierra/dependency")
	if lk == nil || len(lk.MvnJavaNS) != 0 {
		t.Fatalf("a pure library was flagged as needing Java: %+v", lk)
	}
	if len(lk.MvnPureNS) != 1 || lk.MvnPureNS[0] != "com.stuartsierra.dependency" {
		t.Fatalf("want the namespace usable, got %v", lk.MvnPureNS)
	}
	SetMavenIndex(res.MavenVerdicts)
	t.Cleanup(func() { SetMavenIndex(nil) })
	if err := CheckMavenLoadable(filepath.Join(res.Roots[0], "com", "stuartsierra", "dependency.clj")); err != nil {
		t.Fatalf("a pure namespace was gated: %v", err)
	}
	if strings.Contains(strings.Join(res.MavenReport, "\n"), "JVM-only") {
		t.Error("the report still calls a pure library JVM-only")
	}
}

// TestUnreadableFileIsNotAJavaVerdict: when cljgo genuinely cannot parse a
// file, that is a statement about cljgo's reader, not about the library. It
// gets its own code and says so.
func TestUnreadableFileIsNotAJavaVerdict(t *testing.T) {
	r := newMvnRepo(t)
	c := Coord{Group: "weird", Artifact: "weird", Version: "1.0.0"}
	// An unterminated string: unambiguously a read failure, no Java in sight.
	r.publish(c, "", map[string]string{"weird/core.clj": "(ns weird.core)\n(def s \"unterminated\n"})

	res := mustResolve(t, r, []Dep{{Name: "weird", MvnVersion: "1.0.0"}}, nil)
	SetMavenIndex(res.MavenVerdicts)
	t.Cleanup(func() { SetMavenIndex(nil) })

	err := CheckMavenLoadable(filepath.Join(res.Roots[0], "weird", "core.clj"))
	if err == nil {
		t.Fatal("an unreadable file was not gated")
	}
	wantCode(t, err, "G5017")
	if !strings.Contains(err.Error(), "not a statement about the library") {
		t.Errorf("a parse failure did not disclaim a Java verdict:\n%s", err)
	}
	if strings.Contains(err.Error(), "requires Java interop") || strings.Contains(err.Error(), "JVM-only") {
		t.Errorf("a parse failure was reported as a Java verdict:\n%s", err)
	}
}

// --- defect 5: jar-root build scripts --------------------------------------

// TestJarRootProjectCljIsNotANamespace: hiccup ships Leiningen's project.clj
// at the jar root. Counting it inflated the usable-namespace count and put a
// bogus `project` in :mvn/pure — the same false-positive class as the
// META-INF/…/project.clj case, only partially fixed before.
func TestJarRootProjectCljIsNotANamespace(t *testing.T) {
	r := newMvnRepo(t)
	c := Coord{Group: "hiccup", Artifact: "hiccup", Version: "1.0.5"}
	r.publish(c, "", mixedHiccup())

	res := mustResolve(t, r, []Dep{{Name: "hiccup/hiccup", MvnVersion: "1.0.5"}}, nil)
	lk := res.Lock.find("hiccup/hiccup")
	for _, ns := range lk.MvnPureNS {
		if ns == "project" {
			t.Fatalf("the Leiningen build script counted as a usable namespace: %v", lk.MvnPureNS)
		}
	}
	if _, ok := lk.MvnJavaNS["project"]; ok {
		t.Fatal("the Leiningen build script was classified at all")
	}
	// It is not even on the load path.
	if _, err := os.Stat(filepath.Join(res.Roots[0], "project.clj")); !os.IsNotExist(err) {
		t.Error("project.clj was extracted onto the load path")
	}
}

// TestDeclaredVersionEdgeShapes covers the two holes the adversarial verifier
// found in the declared-version rule. Both used to escape it entirely:
//
//   - {:mvn/version ""} left isMvn() false, so the dep fell through to the git
//     path and the user got `git ls-remote  HEAD: exit status 128: fatal: bad
//     repository ”` — an uncoded subprocess error naming a tool they never
//     invoked.
//   - {:mvn/version "../../etc/passwd"} was URL-joined straight into the
//     repository fetch. Not a local traversal (the cache path is a sha256), but
//     a version is a path SEGMENT and a separator has no business reaching the
//     URL.
func TestDeclaredVersionEdgeShapes(t *testing.T) {
	for _, tc := range []struct {
		name, version, want string
	}{
		{"empty", "", "an empty version"},
		{"traversal", "../../etc/passwd", "a path separator in a version"},
		{"slash", "1.0/2.0", "a path separator in a version"},
		{"backslash", `1.0\2.0`, "a path separator in a version"},
		{"dotdot", "1..2", "a path separator in a version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateDeclaredVersion("org.clojure/tools.cli", tc.version)
			if err == nil {
				t.Fatalf("version %q was accepted; want it named at the declaration", tc.version)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want it to name %q", err, tc.want)
			}
			// It must be the coded diagnostic, never a raw error.
			if !strings.Contains(err.Error(), "G5016") && !hasDiagCode(err, "G5016") {
				t.Fatalf("error = %v, want the registered G5016 code", err)
			}
		})
	}
}

// hasDiagCode reports whether err carries the given registered code.
func hasDiagCode(err error, code string) bool {
	type carrier interface {
		Diagnostic() (diag.Diagnostic, bool)
	}
	if c, ok := err.(carrier); ok {
		if d, ok := c.Diagnostic(); ok {
			return d.ErrorCode == code
		}
	}
	return false
}
