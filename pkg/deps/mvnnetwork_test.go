package deps

// The ONE test file in this package that talks to the real internet.
//
// Everything else under pkg/deps runs against an httptest Maven double
// (mvnhelper_test.go) whose fixtures encode the shapes spike s50 observed.
// Fixtures are fast, hermetic and CI-safe — and they are also written by the
// same person who wrote the parser, so they can only ever confirm what that
// person believed Clojars looks like. Two release-blocking defects (the
// <parent> POM inheritance chain, the root-level project.clj misread as a
// namespace) existed precisely because no fixture disagreed with its author.
//
// So this file exists to disagree. It is OFF by default and gated on
//
//	CLJGO_CLOJARS_IT=1 go test ./pkg/deps/ -run TestClojarsIT -v
//
// Run it before cutting a release, and any time mvnpom.go / mvnresolve.go /
// mvnclassify.go changes. It is deliberately NOT wired into the default gate:
// a green build must never depend on Clojars being up.
//
// It asserts BOTH directions, because a gate that only ever says yes is not a
// gate:
//   - a pure-Clojure library resolves, extracts and classifies clean, and
//   - a library that genuinely needs the JVM is REFUSED with a coded, located
//     diagnostic rather than silently half-loading.

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// itOpts returns resolve options pointed at the REAL repositories (empty
// MvnRepos = Clojars then Maven Central) with a real, bounded HTTP client.
func itOpts(t *testing.T) ResolveOptions {
	t.Helper()
	newCache(t)
	return ResolveOptions{
		Update:        true,
		MvnHTTPClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func requireIT(t *testing.T) {
	t.Helper()
	if os.Getenv("CLJGO_CLOJARS_IT") != "1" {
		t.Skip("network test; set CLJGO_CLOJARS_IT=1 to run")
	}
}

// verdictFor returns the verdict for a namespace, or nil.
func verdictFor(res *Resolved, ns string) *NSVerdict {
	for i := range res.MavenVerdicts {
		if res.MavenVerdicts[i].NS == ns {
			return &res.MavenVerdicts[i]
		}
	}
	return nil
}

// --- direction 1: a real pure-Clojure library resolves and is consumable ----

// TestClojarsITPureLibraryResolves pulls com.stuartsierra/dependency 1.0.0
// from the real Clojars. It is the library the end-to-end probe actually ran
// (topological sort over a real graph), it is pure Clojure with no Java
// interop and no dependencies, and — unlike tools.cli or data.json — cljgo
// ships NO native port of it, so a success here can only have come from the
// jar.
func TestClojarsITPureLibraryResolves(t *testing.T) {
	requireIT(t)

	res, err := Resolve([]Dep{{
		Name:        "com.stuartsierra/dependency",
		MvnVersion:  "1.0.0",
		MvnDeclared: true,
	}}, itOpts(t))
	if err != nil {
		t.Fatalf("Resolve from real Clojars: %v", err)
	}

	lk := res.Lock.find("com.stuartsierra/dependency")
	if lk == nil {
		t.Fatalf("coordinate missing from the lock: %+v", res.Lock.Deps)
	}
	if len(res.Roots) == 0 {
		t.Fatal("no source root: the jar was never extracted onto the load path")
	}

	// The library is one namespace and it must classify CLEAN. A non-empty
	// Code here means the require-time gate would refuse a library that the
	// probe demonstrably ran.
	v := verdictFor(res, "com.stuartsierra.dependency")
	if v == nil {
		var got []string
		for _, x := range res.MavenVerdicts {
			got = append(got, x.NS)
		}
		t.Fatalf("namespace com.stuartsierra.dependency not classified; got %v", got)
	}
	if v.Code != "" {
		t.Fatalf("pure library was refused: %s %s", v.Code, v.Reason)
	}
}

// --- direction 2: a real JVM-dependent library is refused, with a code ------

// TestClojarsITJavaLibraryIsRefused pulls hiccup 1.0.5. hiccup is mostly pure
// Clojure, but hiccup.compiler imports java.net.URI — so the honest verdict is
// a REFUSAL naming that namespace, not a quiet success. This is the assertion
// that stops "cljgo consumes Clojars" from drifting into a claim the runtime
// cannot keep.
func TestClojarsITJavaLibraryIsRefused(t *testing.T) {
	requireIT(t)

	res, err := Resolve([]Dep{{
		Name:        "hiccup/hiccup",
		MvnVersion:  "1.0.5",
		MvnDeclared: true,
	}}, itOpts(t))
	if err != nil {
		t.Fatalf("Resolve from real Clojars: %v", err)
	}

	// Resolution itself SUCCEEDS — the gate is per namespace at require time,
	// not per artifact at resolve time (ADR 0095). What must hold is that at
	// least one namespace carries a real code, and that the pure ones don't.
	var refused []NSVerdict
	for _, v := range res.MavenVerdicts {
		if v.Code != "" {
			refused = append(refused, v)
		}
	}
	if len(refused) == 0 {
		t.Fatal("hiccup classified fully pure; hiccup.compiler imports java.net.URI, so the Java gate is not firing")
	}

	// Every refusal must be coded and located — an uncoded, unlocated refusal
	// is exactly the raw-panic failure mode the error doctrine forbids.
	for _, v := range refused {
		switch v.Code {
		case "I4002", "R1012", "G5019":
		default:
			t.Errorf("%s: unregistered refusal code %q", v.NS, v.Code)
		}
		if v.Reason == "" {
			t.Errorf("%s: refused with no reason", v.NS)
		}
		if v.File == "" {
			t.Errorf("%s: refused with no file", v.NS)
		}
	}

	// And the specific known offender must be among them, with the real
	// Java-interop code. If hiccup.compiler ever passes, the classifier has
	// gone blind rather than hiccup having changed: 1.0.5 is immutable.
	if v := verdictFor(res, "hiccup.compiler"); v == nil {
		t.Error("hiccup.compiler was not classified at all")
	} else if v.Code != "I4002" {
		t.Errorf("hiccup.compiler: want I4002 (real Java interop), got %q (%s)", v.Code, v.Reason)
	}

	// hiccup.core has no Java interop; if the classifier smears the artifact's
	// verdict across every namespace, this catches it.
	if v := verdictFor(res, "hiccup.core"); v != nil && v.Code != "" {
		t.Errorf("hiccup.core should be pure, got %s: %s", v.Code, v.Reason)
	}
}

// --- direction 3: the transitive walk survives a real POM chain -------------

// TestClojarsITParentPOMChain resolves org.clojure/tools.cli from Maven
// Central. Its POM inherits from org.clojure/pom.contrib, takes its groupId
// and ${clojure.version} from that parent, and carries a build-only profile.
// The fixture for this shape was wrong once already (mvnreal_test.go defect
// 1); this is the version of that test that cannot be wrong, because the POM
// comes from Central rather than from a belief about Central.
//
// cljgo ships a NATIVE clojure.tools.cli, so this asserts resolution and POM
// handling only — it deliberately makes no claim about which implementation a
// program would end up requiring.
func TestClojarsITParentPOMChain(t *testing.T) {
	requireIT(t)

	res, err := Resolve([]Dep{{
		Name:        "org.clojure/tools.cli",
		MvnVersion:  "1.1.230",
		MvnDeclared: true,
	}}, itOpts(t))
	if err != nil {
		t.Fatalf("Resolve org.clojure/tools.cli from Maven Central: %v", err)
	}
	if res.Lock.find("org.clojure/tools.cli") == nil {
		t.Fatalf("coordinate missing from the lock: %+v", res.Lock.Deps)
	}

	// The parent contributes a dependency on org.clojure/clojure at
	// ${clojure.version}. cljgo prunes the Clojure runtime itself — it IS the
	// runtime — so it must not appear as a source root.
	for _, d := range res.Lock.Deps {
		if strings.HasPrefix(d.Name, "org.clojure/clojure") {
			t.Errorf("org.clojure/clojure was not pruned: %+v", d)
		}
	}
}
