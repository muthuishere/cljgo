package deps

// The httptest Maven repository double, and the in-process jar builder.
//
// NO COMMITTED TEST TOUCHES THE NETWORK. The repository is an httptest.Server
// serving a Maven-layout tree assembled in memory; jars are built with
// archive/zip at test time, so no binary fixture is committed. The repository
// list and the HTTP client are injected through ResolveOptions, so the code
// path under test IS the production one — not a parallel test-only path.
//
// The fixture LIBRARIES encode the spike-s50 shapes verbatim: a 1-namespace
// pure library (tools.cli), a mixed 8-pure/2-Java library (hiccup), a fully
// Java library (data.json), a reader-conditional-fenced library (medley), a
// ${property} library (core.match), and a 4-deep transitive graph.

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mvnRepoDouble is a Maven repository served over httptest.
type mvnRepoDouble struct {
	t     *testing.T
	files map[string][]byte // repo-relative path -> bytes
	srv   *httptest.Server
	hits  map[string]int
}

func newMvnRepo(t *testing.T) *mvnRepoDouble {
	t.Helper()
	r := &mvnRepoDouble{t: t, files: map[string][]byte{}, hits: map[string]int{}}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		p := strings.TrimPrefix(req.URL.Path, "/")
		r.hits[p]++
		b, ok := r.files[p]
		if !ok {
			http.NotFound(w, req)
			return
		}
		w.Write(b)
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *mvnRepoDouble) URL() string { return r.srv.URL }

// opts returns ResolveOptions wired to this repository double, with a client
// that can reach nothing else (see mvnGuardClient).
func (r *mvnRepoDouble) opts(t *testing.T) ResolveOptions {
	return ResolveOptions{
		MvnRepos:      []string{r.URL()},
		MvnHTTPClient: mvnGuardClient(t, r.URL()),
	}
}

// put stores a file and its .sha1 sidecar, exactly as a real repository does.
func (r *mvnRepoDouble) put(path string, body []byte) {
	r.files[path] = body
	sum := sha1.Sum(body)
	r.files[path+".sha1"] = []byte(hex.EncodeToString(sum[:]) + "\n")
}

// publish adds a coordinate's .pom and .jar. srcs maps jar-relative paths to
// file bodies; deps is the raw <dependencies> XML body (may be "").
func (r *mvnRepoDouble) publish(c Coord, depsXML string, srcs map[string]string) {
	r.putPOM(c, depsXML)
	r.put(c.artifactPath(".jar"), buildJar(r.t, srcs))
}

func (r *mvnRepoDouble) putPOM(c Coord, depsXML string) {
	pom := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
  <modelVersion>4.0.0</modelVersion>
  <groupId>%s</groupId>
  <artifactId>%s</artifactId>
  <version>%s</version>
  <packaging>jar</packaging>
  %s
</project>
`, c.Group, c.Artifact, c.Version, depsXML)
	r.put(c.artifactPath(".pom"), []byte(pom))
}

// putRawPOM stores an arbitrary POM body (for the name-error cases).
func (r *mvnRepoDouble) putRawPOM(c Coord, pom string) {
	r.put(c.artifactPath(".pom"), []byte(pom))
}

// depXML renders one <dependency> element.
func depXML(group, artifact, version string, extra ...string) string {
	return fmt.Sprintf("<dependency><groupId>%s</groupId><artifactId>%s</artifactId><version>%s</version>%s</dependency>",
		group, artifact, version, strings.Join(extra, ""))
}

func depsXML(deps ...string) string {
	if len(deps) == 0 {
		return ""
	}
	return "<dependencies>" + strings.Join(deps, "") + "</dependencies>"
}

// buildJar builds a jar (a zip) in memory. No binary fixture is committed.
func buildJar(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	// A real jar always ships a manifest; ours does too, so the tests prove
	// META-INF/ is dropped on extraction (the clj-http false positive s50 hit).
	files = withDefault(files, "META-INF/MANIFEST.MF", "Manifest-Version: 1.0\n")
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func withDefault(m map[string]string, k, v string) map[string]string {
	out := map[string]string{k: v}
	for kk, vv := range m {
		out[kk] = vv
	}
	return out
}

// mvnGuardClient returns an http.Client whose transport refuses every request
// not aimed at allowed. It is what makes "a committed test that needs the
// internet is a broken test" enforceable rather than aspirational.
func mvnGuardClient(t *testing.T, allowed string) *http.Client {
	t.Helper()
	return &http.Client{Transport: &guardTransport{t: t, allowed: allowed, base: http.DefaultTransport}}
}

type guardTransport struct {
	t       *testing.T
	allowed string
	base    http.RoundTripper
}

func (g *guardTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !strings.HasPrefix(req.URL.String(), g.allowed) {
		g.t.Errorf("test tried to reach the network: %s (allowed: %s)", req.URL, g.allowed)
		return nil, fmt.Errorf("blocked by the no-network test guard: %s", req.URL)
	}
	return g.base.RoundTrip(req)
}

// ---- the s50 fixture libraries -------------------------------------------

// pureToolsCLI is the FULLY consumable shape: one namespace, no Java, no
// reader conditionals.
func pureToolsCLI() map[string]string {
	return map[string]string{
		"clojure/tools/cli.clj": `(ns clojure.tools.cli)

(defn parse-opts
  "A pure-Clojure argument parser."
  [args specs]
  {:arguments (vec args) :options {} :specs (vec specs)})

(defn summarize [specs]
  (clojure.string/join "\n" (map str specs)))
`,
	}
}

// fencedMedley is the FULLY consumable shape that only a reader-aware
// classifier can see: its java.util form lives in a #?(:clj …) branch cljgo
// never reads, so it is not in the forms and cannot be flagged. A text scan
// would call this library Java-tainted; the reader does not.
func fencedMedley() map[string]string {
	return map[string]string{
		"medley/core.cljc": `(ns medley.core)

(defn now []
  #?(:clj (java.util.Date.)
     :default :now))

(defn random-uuid* []
  #?(:clj (java.util.UUID/randomUUID)
     :default "uuid"))

(defn map-vals [f m]
  (reduce-kv (fn [acc k v] (assoc acc k (f v))) {} m))
`,
	}
}

// mixedHiccup is the shape that PROVES per-namespace granularity: pure and
// Java namespaces in ONE jar (s50: hiccup ships 8 pure + 2 Java).
func mixedHiccup() map[string]string {
	m := map[string]string{
		"hiccup/compiler.clj": `(ns hiccup.compiler
  (:import [java.io StringWriter]))

(defn render [x] (str x))
`,
		"hiccup/util.clj": `(ns hiccup.util
  (:import java.net.URI))

(defn to-uri [s] (URI. s))
`,
	}
	for _, n := range []string{"core", "page", "form", "element", "def", "middleware", "util2", "misc"} {
		m["hiccup/"+n+".clj"] = "(ns hiccup." + n + ")\n\n(defn f [x] (str x))\n"
	}
	return m
}

// allJavaDataJSON contributes ZERO usable namespaces — it resolves and locks
// (it may be an unrequired transitive edge) but resolve warns loudly.
func allJavaDataJSON() map[string]string {
	return map[string]string{
		"clojure/data/json.clj": `(ns clojure.data.json
  (:import (java.io PrintWriter StringWriter)))

(defn write-str [x] (str x))
`,
	}
}

// jvmOnlyCljc is the starved-conditional trap: a .cljc whose REAL body is
// :clj-only. cljgo must fail loud (R1012), never load an empty namespace.
func jvmOnlyCljc() map[string]string {
	return map[string]string{
		// Every top-level form of the real body is fenced to :clj/:cljs. On
		// cljgo they ALL vanish, leaving a namespace with no vars — the exact
		// trap s50 finding 4 names.
		"jvmonly/core.cljc": `(ns jvmonly.core)

#?(:clj  (def impl :jvm)
   :cljs (def impl :browser))

#?(:clj  (defn f [] impl)
   :cljs (defn f [] impl))
`,
	}
}
