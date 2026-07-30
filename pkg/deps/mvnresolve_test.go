package deps

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mustResolve resolves against the repo double with a throwaway cache.
func mustResolve(t *testing.T, r *mvnRepoDouble, deps []Dep, tweak func(*ResolveOptions)) *Resolved {
	t.Helper()
	newCache(t)
	o := r.opts(t)
	o.Update = true
	if tweak != nil {
		tweak(&o)
	}
	res, err := Resolve(deps, o)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return res
}

func resolveErr(t *testing.T, r *mvnRepoDouble, deps []Dep, tweak func(*ResolveOptions)) error {
	t.Helper()
	newCache(t)
	o := r.opts(t)
	o.Update = true
	if tweak != nil {
		tweak(&o)
	}
	_, err := Resolve(deps, o)
	if err == nil {
		t.Fatal("expected an error, got none")
	}
	return err
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	if got := ErrCode(err); got != code {
		t.Fatalf("want code %s, got %q\nmessage: %v", code, got, err)
	}
	if !strings.Contains(err.Error(), "cljgo explain "+code) {
		t.Errorf("rendered error carries no explain pointer for %s:\n%s", code, err)
	}
}

// --- END TO END: a real pure library resolves and its namespace loads ------

// TestPureLibraryResolvesEndToEnd is the headline proof: a pure-Clojure
// library (the tools.cli shape, one of the two s50 rated FULLY consumable) is
// declared by coordinate, resolved with no JVM and no mvn, extracted, mounted
// as a load-path root, classified pure, and its namespace is loadable.
func TestPureLibraryResolvesEndToEnd(t *testing.T) {
	r := newMvnRepo(t)
	c := Coord{Group: "org.clojure", Artifact: "tools.cli", Version: "1.1.230"}
	r.publish(c, "", pureToolsCLI())

	res := mustResolve(t, r, []Dep{{Name: "org.clojure/tools.cli", MvnVersion: "1.1.230"}}, nil)

	if len(res.Roots) != 1 {
		t.Fatalf("want 1 root, got %v", res.Roots)
	}
	src := filepath.Join(res.Roots[0], "clojure", "tools", "cli.clj")
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("extracted source missing: %v", err)
	}
	// META-INF/ is dropped — the clj-http false positive s50 hit.
	if _, err := os.Stat(filepath.Join(res.Roots[0], "META-INF")); !os.IsNotExist(err) {
		t.Error("META-INF/ was not dropped on extraction")
	}

	lk := res.Lock.find("org.clojure/tools.cli")
	if lk == nil || !lk.IsMvn() {
		t.Fatal("no maven lock entry")
	}
	if lk.MvnVersion != "1.1.230" || lk.MvnRepo != r.URL() {
		t.Errorf("lock records %q @ %q", lk.MvnVersion, lk.MvnRepo)
	}
	if lk.MvnSHA256 == "" || lk.MvnPomSHA == "" || lk.TreeHash == "" {
		t.Errorf("lock is missing an integrity hash: %+v", lk)
	}
	if len(lk.Paths) != 1 || lk.Paths[0] != "" {
		t.Errorf("want :paths [\"\"] (the jar root IS the source root), got %v", lk.Paths)
	}
	if len(lk.MvnPureNS) != 1 || lk.MvnPureNS[0] != "clojure.tools.cli" {
		t.Errorf("want the namespace classified pure, got %v / java %v", lk.MvnPureNS, lk.MvnJavaNS)
	}

	// The require-time gate lets it through.
	SetMavenIndex(res.MavenVerdicts)
	t.Cleanup(func() { SetMavenIndex(nil) })
	if err := CheckMavenLoadable(src); err != nil {
		t.Fatalf("a pure namespace was gated: %v", err)
	}
}

// TestFencedLibraryIsFullyPure proves the reader-based classifier, not a text
// scan: medley's java.util forms are inside #?(:clj …) branches cljgo never
// reads, so the namespace is honestly pure.
func TestFencedLibraryIsFullyPure(t *testing.T) {
	r := newMvnRepo(t)
	c := Coord{Group: "medley", Artifact: "medley", Version: "1.4.0"}
	r.publish(c, "", fencedMedley())

	// A single-segment name means group == artifact.
	res := mustResolve(t, r, []Dep{{Name: "medley", MvnVersion: "1.4.0"}}, nil)
	lk := res.Lock.find("medley")
	if lk == nil {
		t.Fatal("no lock entry")
	}
	if len(lk.MvnJavaNS) != 0 {
		t.Fatalf("a #?(:clj …)-fenced namespace was flagged Java: %v", lk.MvnJavaNS)
	}
	if len(lk.MvnPureNS) != 1 || lk.MvnPureNS[0] != "medley.core" {
		t.Fatalf("want medley.core pure, got %v", lk.MvnPureNS)
	}
}

// --- PER-NAMESPACE, not per-library ---------------------------------------

// TestPerNamespaceGate is the binding s50 finding 3: one jar mixes pure and
// Java namespaces, the pure ones stay usable, and only the USE of a Java
// namespace fails — at require, not at resolve.
func TestPerNamespaceGate(t *testing.T) {
	r := newMvnRepo(t)
	c := Coord{Group: "hiccup", Artifact: "hiccup", Version: "1.0.5"}
	r.publish(c, "", mixedHiccup())

	res := mustResolve(t, r, []Dep{{Name: "hiccup/hiccup", MvnVersion: "1.0.5"}}, nil)

	lk := res.Lock.find("hiccup/hiccup")
	if len(lk.MvnPureNS) != 8 {
		t.Errorf("want 8 pure namespaces, got %d: %v", len(lk.MvnPureNS), lk.MvnPureNS)
	}
	if len(lk.MvnJavaNS) != 2 {
		t.Errorf("want 2 Java namespaces, got %d: %v", len(lk.MvnJavaNS), lk.MvnJavaNS)
	}
	for _, ns := range []string{"hiccup.compiler", "hiccup.util"} {
		if _, ok := lk.MvnJavaNS[ns]; !ok {
			t.Errorf("%s was not classified Java", ns)
		}
	}

	SetMavenIndex(res.MavenVerdicts)
	t.Cleanup(func() { SetMavenIndex(nil) })

	root := res.Roots[0]
	// The pure ones load.
	if err := CheckMavenLoadable(filepath.Join(root, "hiccup", "core.clj")); err != nil {
		t.Errorf("hiccup.core was gated: %v", err)
	}
	// The Java one fails LOUD, naming the namespace, the coordinate, the
	// offending form, and how many others are still usable.
	err := CheckMavenLoadable(filepath.Join(root, "hiccup", "compiler.clj"))
	if err == nil {
		t.Fatal("hiccup.compiler was NOT gated")
	}
	wantCode(t, err, "I4002")
	for _, want := range []string{"hiccup.compiler", "hiccup/hiccup 1.0.5", ":import", "8 other namespaces"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("I4002 message is missing %q:\n%s", want, err)
		}
	}
}

// TestAllJavaLibraryWarnsButResolves — decision §7.7: a dependency
// contributing zero usable namespaces still resolves and locks (it may be an
// unrequired transitive edge), but the report names it loudly.
func TestAllJavaLibraryWarnsButResolves(t *testing.T) {
	r := newMvnRepo(t)
	c := Coord{Group: "org.clojure", Artifact: "data.json", Version: "2.5.0"}
	r.publish(c, "", allJavaDataJSON())

	res := mustResolve(t, r, []Dep{{Name: "org.clojure/data.json", MvnVersion: "2.5.0"}}, nil)
	if len(res.MavenReport) != 1 || !strings.Contains(res.MavenReport[0], "no usable namespaces") {
		t.Fatalf("want a loud zero-usable warning, got %v", res.MavenReport)
	}
	if lk := res.Lock.find("org.clojure/data.json"); lk == nil || len(lk.MvnPureNS) != 0 {
		t.Fatalf("expected it to resolve and lock with zero pure namespaces: %+v", lk)
	}
}

// TestStarvedConditionalFailsLoud — the required s50 case: a .cljc whose real
// body is :clj-only gives cljgo NOTHING loadable and must fail loud, never
// silently install an empty namespace.
func TestStarvedConditionalFailsLoud(t *testing.T) {
	r := newMvnRepo(t)
	c := Coord{Group: "jvmonly", Artifact: "jvmonly", Version: "0.1.0"}
	r.publish(c, "", jvmOnlyCljc())

	res := mustResolve(t, r, []Dep{{Name: "jvmonly", MvnVersion: "0.1.0"}}, nil)
	lk := res.Lock.find("jvmonly")
	if len(lk.MvnPureNS) != 0 {
		t.Fatalf("a starved .cljc was classified usable: %v", lk.MvnPureNS)
	}

	SetMavenIndex(res.MavenVerdicts)
	t.Cleanup(func() { SetMavenIndex(nil) })
	err := CheckMavenLoadable(filepath.Join(res.Roots[0], "jvmonly", "core.cljc"))
	if err == nil {
		t.Fatal("a :clj-only .cljc loaded silently — the exact failure mode s50 forbids")
	}
	wantCode(t, err, "R1012")
	for _, want := range []string{"jvmonly.core", ":cljgo", ":clj", ":cljs"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("R1012 message is missing %q:\n%s", want, err)
		}
	}
}

// --- the transitive graph --------------------------------------------------

func TestTransitiveGraph(t *testing.T) {
	r := newMvnRepo(t)
	r.publish(Coord{"a", "a", "1.0.0"}, depsXML(depXML("b", "b", "2.0.0")),
		map[string]string{"a/core.clj": "(ns a.core)\n"})
	r.publish(Coord{"b", "b", "2.0.0"}, depsXML(depXML("c", "c", "3.0.0")),
		map[string]string{"b/core.clj": "(ns b.core)\n"})
	r.publish(Coord{"c", "c", "3.0.0"}, depsXML(depXML("d", "d", "4.0.0")),
		map[string]string{"c/core.clj": "(ns c.core)\n"})
	r.publish(Coord{"d", "d", "4.0.0"}, "", map[string]string{"d/core.clj": "(ns d.core)\n"})

	res := mustResolve(t, r, []Dep{{Name: "a", MvnVersion: "1.0.0"}}, nil)
	if len(res.Lock.Deps) != 4 {
		t.Fatalf("want 4 locked coordinates, got %d", len(res.Lock.Deps))
	}
	if len(res.Roots) != 4 {
		t.Fatalf("want 4 roots, got %v", res.Roots)
	}
}

func TestScopeOptionalAndExclusionsAreHonoured(t *testing.T) {
	r := newMvnRepo(t)
	r.publish(Coord{"a", "a", "1.0.0"}, depsXML(
		depXML("t", "t", "1.0.0", "<scope>test</scope>"),
		depXML("p", "p", "1.0.0", "<scope>provided</scope>"),
		depXML("o", "o", "1.0.0", "<optional>true</optional>"),
		depXML("b", "b", "1.0.0", "<exclusions><exclusion><groupId>x</groupId><artifactId>*</artifactId></exclusion></exclusions>"),
	), map[string]string{"a/core.clj": "(ns a.core)\n"})
	r.publish(Coord{"b", "b", "1.0.0"}, depsXML(depXML("x", "x", "9.9.9")),
		map[string]string{"b/core.clj": "(ns b.core)\n"})
	// t/p/o/x are deliberately NOT published: following any of them 404s.

	res := mustResolve(t, r, []Dep{{Name: "a", MvnVersion: "1.0.0"}}, nil)
	if len(res.Lock.Deps) != 2 {
		t.Fatalf("want only a and b, got %d entries", len(res.Lock.Deps))
	}
}

func TestClojureItselfIsPrunedAndReported(t *testing.T) {
	r := newMvnRepo(t)
	// The clojure edge carries the exact ${clojure.version} shape s50 hit; it
	// is pruned BEFORE validation, which is what makes that 404 disappear.
	r.publish(Coord{"a", "a", "1.0.0"}, depsXML(
		depXML("org.clojure", "clojure", "${clojure.version}"),
		depXML("org.clojure", "spec.alpha", "0.3.218"),
	), map[string]string{"a/core.clj": "(ns a.core)\n"})

	res := mustResolve(t, r, []Dep{{Name: "a", MvnVersion: "1.0.0"}}, nil)
	if len(res.Lock.Deps) != 1 {
		t.Fatalf("clojure itself was not pruned: %d entries", len(res.Lock.Deps))
	}
	if !strings.Contains(strings.Join(res.MavenReport, "\n"), "pruned org.clojure/clojure") {
		t.Errorf("the prune was silent; it must be reported:\n%v", res.MavenReport)
	}
}

func TestVersionConflictNamesBothRequirers(t *testing.T) {
	r := newMvnRepo(t)
	r.publish(Coord{"a", "a", "1.0.0"}, depsXML(depXML("shared", "shared", "1.0.0")),
		map[string]string{"a/core.clj": "(ns a.core)\n"})
	r.publish(Coord{"b", "b", "1.0.0"}, depsXML(depXML("shared", "shared", "2.0.0")),
		map[string]string{"b/core.clj": "(ns b.core)\n"})
	r.publish(Coord{"shared", "shared", "1.0.0"}, "", map[string]string{"s/core.clj": "(ns s.core)\n"})
	r.publish(Coord{"shared", "shared", "2.0.0"}, "", map[string]string{"s/core.clj": "(ns s.core)\n"})

	err := resolveErr(t, r, []Dep{
		{Name: "a", MvnVersion: "1.0.0"},
		{Name: "b", MvnVersion: "1.0.0"},
	}, nil)
	wantCode(t, err, "G5013")
	for _, want := range []string{"shared/shared", "1.0.0", "2.0.0", "required by a", "required by b", "accept-version"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("G5013 is missing %q:\n%s", want, err)
		}
	}

	// accept-version, keyed on the coordinate string, is the override.
	mustResolve(t, r, []Dep{
		{Name: "a", MvnVersion: "1.0.0"},
		{Name: "b", MvnVersion: "1.0.0"},
	}, func(o *ResolveOptions) {
		o.AcceptVersions = map[string]string{"shared/shared": "1.0.0"}
	})
}

// --- the name-error surface (s50 finding 1, binding) -----------------------

func TestUnsupportedPOMFeaturesNameError(t *testing.T) {
	cases := []struct {
		name string
		pom  string
		want string
	}{
		{"property interpolation",
			`<project><groupId>a</groupId><artifactId>a</artifactId><version>1.0.0</version>
			 <dependencies>` + depXML("z", "z", "${z.version}") + `</dependencies></project>`,
			"${property} interpolation"},
		{"dependencyManagement version supply",
			`<project><groupId>a</groupId><artifactId>a</artifactId><version>1.0.0</version>
			 <dependencyManagement><dependencies>` + depXML("z", "z", "1.0.0") + `</dependencies></dependencyManagement>
			 <dependencies><dependency><groupId>z</groupId><artifactId>z</artifactId></dependency></dependencies></project>`,
			"<dependencyManagement>"},
		{"parent inheritance",
			`<project><parent><groupId>p</groupId><artifactId>p</artifactId><version>1</version></parent>
			 <groupId>a</groupId><artifactId>a</artifactId><version>1.0.0</version></project>`,
			"<parent>"},
		{"version range",
			`<project><groupId>a</groupId><artifactId>a</artifactId><version>1.0.0</version>
			 <dependencies>` + depXML("z", "z", "[1.0,2.0)") + `</dependencies></project>`,
			"version range"},
		{"snapshot",
			`<project><groupId>a</groupId><artifactId>a</artifactId><version>1.0.0</version>
			 <dependencies>` + depXML("z", "z", "1.0.0-SNAPSHOT") + `</dependencies></project>`,
			"-SNAPSHOT"},
		{"profiles",
			`<project><groupId>a</groupId><artifactId>a</artifactId><version>1.0.0</version>
			 <profiles><profile><id>dev</id></profile></profiles></project>`,
			"<profiles>"},
		{"classifier",
			`<project><groupId>a</groupId><artifactId>a</artifactId><version>1.0.0</version>
			 <dependencies>` + depXML("z", "z", "1.0.0", "<classifier>sources</classifier>") + `</dependencies></project>`,
			"<classifier>"},
		{"non-jar packaging",
			`<project><groupId>a</groupId><artifactId>a</artifactId><version>1.0.0</version>
			 <packaging>pom</packaging></project>`,
			"<packaging>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newMvnRepo(t)
			c := Coord{"a", "a", "1.0.0"}
			r.putRawPOM(c, tc.pom)
			r.put(c.artifactPath(".jar"), buildJar(t, map[string]string{"a/core.clj": "(ns a.core)\n"}))

			err := resolveErr(t, r, []Dep{{Name: "a", MvnVersion: "1.0.0"}}, nil)
			wantCode(t, err, "G5011")
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("G5011 does not name the feature %q:\n%s", tc.want, err)
			}
			// It names the coordinate that needs it, not "a pom failed".
			if !strings.Contains(err.Error(), "a/a 1.0.0") {
				t.Errorf("G5011 does not name the coordinate:\n%s", err)
			}
		})
	}
}

func TestCoordinateNotFoundNamesEveryRepo(t *testing.T) {
	r := newMvnRepo(t)
	err := resolveErr(t, r, []Dep{{Name: "no.such/thing", MvnVersion: "1.0.0"}}, nil)
	wantCode(t, err, "G5010")
	if !strings.Contains(err.Error(), r.URL()) || !strings.Contains(err.Error(), "404") {
		t.Errorf("G5010 must name every repo tried with its status:\n%s", err)
	}
}

func TestConflictingCoordinateKinds(t *testing.T) {
	r := newMvnRepo(t)
	err := resolveErr(t, r, []Dep{{Name: "x", MvnVersion: "1.0.0", GitURL: "https://example.invalid/x"}}, nil)
	wantCode(t, err, "G5015")
	if !strings.Contains(err.Error(), ":mvn/version") || !strings.Contains(err.Error(), ":git") {
		t.Errorf("G5015 must name both coordinate kinds:\n%s", err)
	}
}

func TestChecksumMismatchAgainstTheLock(t *testing.T) {
	r := newMvnRepo(t)
	c := Coord{"a", "a", "1.0.0"}
	r.publish(c, "", map[string]string{"a/core.clj": "(ns a.core)\n"})

	newCache(t)
	o := r.opts(t)
	o.Update = true
	res, err := Resolve([]Dep{{Name: "a", MvnVersion: "1.0.0"}}, o)
	if err != nil {
		t.Fatal(err)
	}
	// A lock that disagrees with the bytes on disk is a hard failure.
	lock := res.Lock
	lock.find("a").MvnSHA256 = "sha256:deadbeef"
	o2 := r.opts(t)
	o2.Lock = lock
	// Force a re-extraction so the jar is re-hashed.
	if err := makeWritable(filepath.Join(os.Getenv("CLJGO_CACHE"), "src")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(res.Roots[0]); err != nil {
		t.Fatal(err)
	}
	_, err = Resolve([]Dep{{Name: "a", MvnVersion: "1.0.0"}}, o2)
	if err == nil {
		t.Fatal("a lock/bytes disagreement was accepted")
	}
	wantCode(t, err, "G5012")
}

// --- offline ---------------------------------------------------------------

func TestOfflineMatrix(t *testing.T) {
	r := newMvnRepo(t)
	c := Coord{"a", "a", "1.0.0"}
	r.publish(c, "", map[string]string{"a/core.clj": "(ns a.core)\n"})

	cache := newCache(t)
	o := r.opts(t)
	o.Update = true
	res, err := Resolve([]Dep{{Name: "a", MvnVersion: "1.0.0"}}, o)
	if err != nil {
		t.Fatal(err)
	}

	// WARM cache + lock, offline: resolves with the network hard-blocked.
	warm := ResolveOptions{
		Lock:          res.Lock,
		Offline:       true,
		MvnRepos:      []string{r.URL()},
		MvnHTTPClient: &http.Client{Transport: refuseAll{}},
	}
	if _, err := Resolve([]Dep{{Name: "a", MvnVersion: "1.0.0"}}, warm); err != nil {
		t.Fatalf("a warm cache must resolve offline: %v", err)
	}

	// COLD cache, offline: G5014 naming the coordinate and the cache path.
	if err := makeWritable(cache); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(cache); err != nil {
		t.Fatal(err)
	}
	_, err = Resolve([]Dep{{Name: "a", MvnVersion: "1.0.0"}}, warm)
	if err == nil {
		t.Fatal("a cold cache resolved offline")
	}
	wantCode(t, err, "G5014")
	if !strings.Contains(err.Error(), "a/a 1.0.0") {
		t.Errorf("G5014 must name the coordinate:\n%s", err)
	}
}

type refuseAll struct{}

func (refuseAll) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, errNoNetwork
}

var errNoNetwork = &netBlocked{}

type netBlocked struct{}

func (*netBlocked) Error() string { return "network blocked by the offline test" }

// --- the no-network guard --------------------------------------------------

// TestNoNetworkInDepsTests is the structural guard: with the default client
// swapped for one that refuses everything, a Maven resolve of an unlocked,
// uncached coordinate must FAIL rather than silently reaching the internet. A
// future test that "works" by hitting live Clojars cannot slip past this.
func TestNoNetworkInDepsTests(t *testing.T) {
	newCache(t)
	_, err := Resolve([]Dep{{Name: "org.clojure/tools.cli", MvnVersion: "1.1.230"}}, ResolveOptions{
		Update:        true,
		MvnHTTPClient: &http.Client{Transport: refuseAll{}},
	})
	if err == nil {
		t.Fatal("a deps test reached the network")
	}
	wantCode(t, err, "G5010")
	// It must have tried the real default repositories and been refused —
	// proving the defaults are wired, without a byte leaving the machine.
	for _, repo := range DefaultMvnRepos {
		if !strings.Contains(err.Error(), repo) {
			t.Errorf("the default repository %s was not consulted:\n%s", repo, err)
		}
	}
}
