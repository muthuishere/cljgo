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
	t     testing.TB
	files map[string][]byte // repo-relative path -> bytes
	srv   *httptest.Server
	hits  map[string]int
}

func newMvnRepo(t testing.TB) *mvnRepoDouble {
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
func (r *mvnRepoDouble) opts(t testing.TB) ResolveOptions {
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

// ---- the REAL org.clojure contrib shape ----------------------------------
//
// This is the fixture correction that mattered. putPOM above synthesizes a
// standalone POM with no <parent>, so the fixture called "the tools.cli
// shape" DID NOT HAVE tools.cli's shape, and the green test suite hid the
// fact that the entire target set was unresolvable. The bodies below are
// transcribed from the artifacts actually served by Maven Central on
// 2026-07-30:
//
//   https://repo1.maven.org/maven2/org/clojure/tools.cli/1.1.230/tools.cli-1.1.230.pom
//   https://repo1.maven.org/maven2/org/clojure/pom.contrib/1.2.0/pom.contrib-1.2.0.pom
//
// Note every feature they carry that a synthesized POM does not: a <parent>
// with the child inheriting its groupId, a <properties> block, a parent
// <dependencies> on org.clojure/clojure at ${clojure.version}, and a parent
// <profile> that touches only <build>.

// putContribPOM publishes a POM in the org.clojure contrib shape: NO
// <groupId> of its own (inherited), a <parent> pointing at pom.contrib, and a
// <properties> block. contribParentCoord must be published too.
func (r *mvnRepoDouble) putContribPOM(c Coord, parent Coord, depsXML string) {
	pom := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <modelVersion>4.0.0</modelVersion>
  <artifactId>%s</artifactId>
  <version>%s</version>
  <name>%s</name>

  <parent>
    <groupId>%s</groupId>
    <artifactId>%s</artifactId>
    <version>%s</version>
  </parent>

  <properties>
    <clojure.version>1.9.0</clojure.version>
  </properties>
  %s
</project>
`, c.Artifact, c.Version, c.Artifact, parent.Group, parent.Artifact, parent.Version, depsXML)
	r.put(c.artifactPath(".pom"), []byte(pom))
}

// contribParent is the coordinate of the shared parent POM.
var contribParent = Coord{Group: "org.clojure", Artifact: "pom.contrib", Version: "1.2.0"}

// putContribParent publishes org.clojure/pom.contrib: <packaging>pom</packaging>,
// a <dependencies> on clojure at ${clojure.version} (which the child inherits
// and cljgo prunes), and a build-only <profile> (which must NOT name-error).
func (r *mvnRepoDouble) putContribParent(managedXML string) {
	pom := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
  <modelVersion>4.0.0</modelVersion>
  <groupId>%s</groupId>
  <artifactId>%s</artifactId>
  <packaging>pom</packaging>
  <version>%s</version>
  <name>pom.contrib</name>

  <properties>
    <clojure.version>1.9.0</clojure.version>
    <clojure.warnOnReflection>false</clojure.warnOnReflection>
    <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
  </properties>

  <dependencies>
    <dependency>
      <groupId>org.clojure</groupId>
      <artifactId>clojure</artifactId>
      <version>${clojure.version}</version>
    </dependency>
  </dependencies>
  %s
  <build>
    <plugins>
      <plugin>
        <groupId>org.apache.maven.plugins</groupId>
        <artifactId>maven-source-plugin</artifactId>
        <version>3.3.1</version>
      </plugin>
    </plugins>
  </build>

  <profiles>
    <profile>
      <id>sign</id>
      <build>
        <plugins>
          <plugin>
            <groupId>org.apache.maven.plugins</groupId>
            <artifactId>maven-gpg-plugin</artifactId>
            <version>1.5</version>
          </plugin>
        </plugins>
      </build>
    </profile>
  </profiles>
</project>
`, contribParent.Group, contribParent.Artifact, contribParent.Version, managedXML)
	r.put(contribParent.artifactPath(".pom"), []byte(pom))
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
func buildJar(t testing.TB, files map[string]string) []byte {
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
func mvnGuardClient(t testing.TB, allowed string) *http.Client {
	t.Helper()
	return &http.Client{Transport: &guardTransport{t: t, allowed: allowed, base: http.DefaultTransport}}
}

type guardTransport struct {
	t       testing.TB
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

// pureAutoResolvedKeywords is the com.stuartsierra/dependency 1.0.0 shape: a
// 100%-pure library that uses ::auto-resolved-keywords, including the
// ::alias/qualified form. The classifier read this WITHOUT a resolver, so
// every ::kw was a read error and the library was reported as JVM-only. The
// fixture now has the shape the real artifact has, so that cannot recur
// silently. Transcribed from
// https://repo.clojars.org/com/stuartsierra/dependency/1.0.0/dependency-1.0.0.jar
// (com/stuartsierra/dependency.clj uses ::circular-dependency and friends).
func pureAutoResolvedKeywords() map[string]string {
	return map[string]string{
		"com/stuartsierra/dependency.clj": `(ns com.stuartsierra.dependency
  (:require [clojure.set :as set]))

(defn- throw-circular [node dep]
  (throw (ex-info "Circular dependency"
                  {:reason ::circular-dependency
                   :node node
                   :dependency dep})))

(def ^:private kinds #{::dependency ::dependent})

(defn transitive [neighbors x]
  (loop [acc #{} pending [x]]
    (if-let [n (first pending)]
      (recur (conj acc n) (concat (rest pending) (neighbors n)))
      (disj acc x))))

(defn kind-of [k]
  (get {::dependency :fwd ::dependent :rev} k ::set/unknown))
`,
	}
}

// realToolsCLIShape is real tools.cli 1.1.230's reader-conditional surface,
// which the original one-namespace fixture had none of, and which is what a
// live run tripped on. Two shapes, both of which JVM Clojure elides and
// neither of which makes the namespace unusable:
//
//   - cli.cljc:74  `#?(:cljs (defn- format …))` — a TOP-LEVEL cljs-only helper.
//     Treating any top-level starve as fatal reported a fully consumable
//     library as unusable.
//   - cli.cljc:108 `#?@(:cljr (req …))` inside a `let` binding vector — a
//     starved SPLICE contributes zero elements, so the vector stays even.
func realToolsCLIShape() map[string]string {
	return map[string]string{
		"clojure/tools/cli.cljc": `(ns clojure.tools.cli)

#?(:cljs
   (defn- format
     [fmt & args]
     (apply goog.string.format fmt args)))

(defn- compile-spec [spec]
  (let [long-opt (first spec)
        #?@(:cljr (req (if (= long-opt "") nil long-opt)))
        id (keyword (str long-opt))]
    {:id id :long-opt long-opt}))

(defn parse-opts [args specs]
  {:arguments (vec args) :options {} :specs (mapv compile-spec specs)})
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
	// hiccup.core is the REAL shape: it pulls the Java namespaces in with
	// (:use …), not (:require …). Dropping :use on the floor let exactly this
	// reach a compiled binary ungated.
	m["hiccup/core.clj"] = `(ns hiccup.core
  (:use hiccup.compiler hiccup.util))

(defn html [& content] (render content))
`
	// ...and a pure namespace that uses ::auto-resolved keywords, which the
	// resolverless classifier used to call Java interop.
	m["hiccup/page.clj"] = `(ns hiccup.page)

(def modes #{::all-literal ::html ::xhtml})

(defn doctype [m] (get {::html "<!DOCTYPE html>"} m ""))
`
	for _, n := range []string{"form", "element", "def", "middleware", "util2", "misc"} {
		m["hiccup/"+n+".clj"] = "(ns hiccup." + n + ")\n\n(defn f [x] (str x))\n"
	}
	// Leiningen packages its build script at the JAR ROOT. Counting it made
	// the resolve report say "7 namespaces usable" for a 6-namespace library
	// and put a bogus `project` in :mvn/pure. It must be dropped like
	// META-INF/, not classified.
	m["project.clj"] = `(defproject hiccup "1.0.5"
  :description "A fast library for rendering HTML"
  :dependencies [[org.clojure/clojure "1.2.1"]])
`
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

// interopFreeButUncompilable is the shape the real-Clojars run exposed: a
// namespace with NO Java interop that cljgo nonetheless cannot compile. In the
// live case it was medley.core's perfectly ordinary
// `(defn name "doc" {:attr-map} ...)`, which cljgo's `defn` could not parse;
// the fixture stands in for any such cljgo gap with a form that fails at load
// regardless of which gap is currently open (an unresolvable symbol).
//
// Resolve-time classification PASSES it — correctly, because what it measures
// is "reads on cljgo, no Java interop" — so the report must say exactly that
// and no more. The require-time failure is G5020, which names the difference.
func interopFreeButUncompilable() map[string]string {
	return map[string]string{
		"gapped/core.clj": `(ns gapped.core)

(defn f [x] (a-symbol-cljgo-cannot-resolve x))
`,
	}
}
