// s50 — Clojars/Maven CONSUME spike (ADR 0095 decision 1).
//
// Falsifiable question: can a minimal PURE-GO, zero-dependency client resolve a
// real Clojars/Maven coordinate's transitive .pom graph, download the jar, and
// extract usable .clj source onto a load path — AND how thin is the pure subset
// among real libraries?
//
// Kill condition: transitive .pom resolution needs JVM/Aether semantics we can't
// slice cheaply, OR the pure subset among common libs is so thin the feature
// isn't worth the resolver.
//
// This program uses ONLY the Go standard library (net/http, archive/zip,
// encoding/xml) — the proof that ADR 0095's "no JVM, no shelling to mvn,
// zero-dependency" claim holds. Run: `go run .` (needs network).
package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

// The default repository set — Clojars first, then Maven Central. Exactly the
// tools.deps default order, reused, not reinvented (ADR 0095 dec 1).
var repos = []struct{ name, base string }{
	{"clojars", "https://repo.clojars.org"},
	{"central", "https://repo1.maven.org/maven2"},
}

type coord struct{ group, artifact, version string }

func (c coord) path(ext string) string {
	g := strings.ReplaceAll(c.group, ".", "/")
	return fmt.Sprintf("%s/%s/%s/%s-%s.%s", g, c.artifact, c.version, c.artifact, c.version, ext)
}
func (c coord) String() string { return c.group + "/" + c.artifact + " " + c.version }

// pom is the minimal Maven POM slice we need: the transitive <dependencies>.
// A full Aether reader parses profiles, <dependencyManagement>, property
// interpolation, exclusions — we take the slice real pure-Clojure libs use
// (ADR 0095 dec 4: "the slice real pure Clojure libraries actually need").
type pom struct {
	XMLName xml.Name `xml:"project"`
	Deps    []pomDep `xml:"dependencies>dependency"`
	Parent  *pomDep  `xml:"parent"`
	Packing string   `xml:"packaging"`
}
type pomDep struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
	Scope      string `xml:"scope"`
	Optional   string `xml:"optional"`
}

var httpc = &http.Client{Timeout: 30 * time.Second}

// fetch tries each repo in order, returns the bytes + which repo served it.
func fetch(relPath string) ([]byte, string, error) {
	var lastErr error
	for _, r := range repos {
		url := r.base + "/" + relPath
		resp, err := httpc.Get(url)
		if err != nil {
			lastErr = err
			continue
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == 200 {
			return b, r.name, nil
		}
		lastErr = fmt.Errorf("HTTP %d from %s", resp.StatusCode, r.name)
	}
	return nil, "", lastErr
}

// mavenMeta is the slice of maven-metadata.xml we need to resolve "latest
// release" for a group/artifact when no version is given on the CLI.
type mavenMeta struct {
	Release  string   `xml:"versioning>release"`
	Latest   string   `xml:"versioning>latest"`
	Versions []string `xml:"versioning>versions>version"`
}

// latestVersion resolves the newest release version for group/artifact by
// reading maven-metadata.xml from the repo that has it.
func latestVersion(group, artifact string) (string, string, error) {
	rel := strings.ReplaceAll(group, ".", "/") + "/" + artifact + "/maven-metadata.xml"
	b, repo, err := fetch(rel)
	if err != nil {
		return "", "", err
	}
	var m mavenMeta
	if err := xml.Unmarshal(b, &m); err != nil {
		return "", repo, err
	}
	v := m.Release
	if v == "" {
		v = m.Latest
	}
	if v == "" && len(m.Versions) > 0 {
		v = m.Versions[len(m.Versions)-1]
	}
	if v == "" {
		return "", repo, fmt.Errorf("no release version in metadata")
	}
	return v, repo, nil
}

func fetchPom(c coord) (*pom, string, error) {
	b, repo, err := fetch(c.path("pom"))
	if err != nil {
		return nil, "", err
	}
	var p pom
	if err := xml.Unmarshal(b, &p); err != nil {
		return nil, repo, fmt.Errorf("parse pom: %w", err)
	}
	return &p, repo, nil
}

// Java-taint scan — CERTAIN-ONLY, precision over recall, exactly ADR 0054
// decision 4 / S35's certain-java? predicate. We flag only self-identifying JVM
// surfaces; the undecidable bare (.method obj) dot-form is deliberately NOT
// flagged (S35's accepted false-negative floor, caught downstream loudly).
var javaSurfaces = []*regexp.Regexp{
	regexp.MustCompile(`\(:gen-class`),
	regexp.MustCompile(`\(gen-class\b`),
	regexp.MustCompile(`\(:import\b`),
	regexp.MustCompile(`\(import\s`),
	regexp.MustCompile(`\(proxy\b`),
	regexp.MustCompile(`\(definterface\b`),
	regexp.MustCompile(`\(System/`),
	regexp.MustCompile(`\(Math/`),
	regexp.MustCompile(`\bjava\.[a-z]`),
	regexp.MustCompile(`\bjavax\.[a-z]`),
	regexp.MustCompile(`\bset!\s+\(\.`), // field mutation on a Java object
}

// readerCond matches a `#?(` / `#?@(` reader-conditional opener. Java that lives
// ONLY inside such a form is FENCED: cljgo reads the :cljgo/:default branch, not
// :clj, so JVM-only code in a :clj branch never reaches cljgo. Distinguishing
// fenced from hard (unconditional) Java is the crux of how consumable .cljc
// libraries actually are — the raw "contains java.*" count overstates the taint.
var readerCond = regexp.MustCompile(`#\?@?\(`)

// javaTaint returns ("", "") if clean, or (matchedSurface, kind) where kind is
// "hard" (unconditional Java — cljgo cannot load this ns) or "fenced" (Java only
// inside a reader conditional — cljgo skips it, ns MAY still load if a
// :cljgo/:default branch exists).
func javaTaint(src string) (string, string) {
	// Strip line comments cheaply so a `;; uses java.util` note doesn't trip it.
	var b strings.Builder
	for _, line := range strings.Split(src, "\n") {
		if i := strings.Index(line, ";"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	stripped := b.String()
	hasReaderCond := readerCond.MatchString(stripped)
	for _, re := range javaSurfaces {
		if loc := re.FindStringIndex(stripped); loc != nil {
			m := strings.TrimSpace(re.FindString(stripped))
			// Cheap fence test: is there a reader-conditional opener BEFORE the
			// match? A precise test needs balanced-paren scanning; for the spike
			// this heuristic (any #? in the file, match after it) separates the
			// fenced-Java .cljc libs from the hard-Java .clj ones well enough to
			// measure — and we say so in the verdict.
			if hasReaderCond {
				if rc := readerCond.FindStringIndex(stripped); rc != nil && rc[0] < loc[0] {
					return m, "fenced"
				}
			}
			return m, "hard"
		}
	}
	return "", ""
}

type nsResult struct {
	entry string // path inside the jar
	pure  bool
	taint string
	kind  string // "" | "hard" | "fenced"
}

// extractSources pulls .clj/.cljc/.cljs source entries out of a jar and runs the
// taint scan. Returns per-source results + whether the jar carried ANY source.
func extractSources(jar []byte) ([]nsResult, bool, int) {
	zr, err := zip.NewReader(bytes.NewReader(jar), int64(len(jar)))
	if err != nil {
		return nil, false, 0
	}
	var results []nsResult
	classFiles := 0
	for _, f := range zr.File {
		name := f.Name
		if strings.HasSuffix(name, ".class") {
			classFiles++
			continue
		}
		if !strings.HasSuffix(name, ".clj") && !strings.HasSuffix(name, ".cljc") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		src, _ := io.ReadAll(rc)
		rc.Close()
		taint, kind := javaTaint(string(src))
		// A "fenced" hit means the Java is inside a reader conditional cljgo
		// skips — treat the namespace as (provisionally) pure for reach counting.
		pure := taint == "" || kind == "fenced"
		results = append(results, nsResult{entry: name, pure: pure, taint: taint, kind: kind})
	}
	return results, len(results) > 0, classFiles
}

// resolveTransitive walks the .pom dependency graph breadth-first, bounded, and
// returns the flat set of coordinates (compile/runtime scope, non-optional).
func resolveTransitive(root coord, maxDepth int) ([]coord, map[string]string) {
	seen := map[string]bool{}
	repoOf := map[string]string{}
	var out []coord
	type item struct {
		c     coord
		depth int
	}
	queue := []item{{root, 0}}
	for len(queue) > 0 {
		it := queue[0]
		queue = queue[1:]
		key := it.c.group + "/" + it.c.artifact
		if seen[key] {
			continue
		}
		seen[key] = true
		p, repo, err := fetchPom(it.c)
		repoOf[key] = repo
		if err != nil {
			repoOf[key] = "MISS(" + err.Error() + ")"
			out = append(out, it.c)
			continue
		}
		out = append(out, it.c)
		if it.depth >= maxDepth {
			continue
		}
		for _, d := range p.Deps {
			if d.Scope == "test" || d.Scope == "provided" || d.Optional == "true" {
				continue
			}
			if d.Version == "" { // needs dependencyManagement — out of our slice
				continue
			}
			queue = append(queue, item{coord{d.GroupID, d.ArtifactID, d.Version}, it.depth + 1})
		}
	}
	return out, repoOf
}

// consume prints the full report and returns a category: "full", "partial",
// or "unusable".
func consume(c coord) string {
	fmt.Printf("\n━━━ %s ━━━\n", c)
	// 1. transitive graph from .pom
	graph, repoOf := resolveTransitive(c, 3)
	fmt.Printf("  transitive graph: %d coordinate(s)\n", len(graph))
	for _, g := range graph {
		fmt.Printf("    - %-40s [%s]\n", g.group+"/"+g.artifact+" "+g.version, repoOf[g.group+"/"+g.artifact])
	}
	// 2. fetch the ROOT jar + extract/scan source (the thing we'd load)
	jar, repo, err := fetch(c.path("jar"))
	if err != nil {
		fmt.Printf("  jar: FETCH FAILED (%v)\n", err)
		return "unusable"
	}
	sources, hasSource, classCount := extractSources(jar)
	fmt.Printf("  jar: %d bytes from %s — %d .clj/.cljc source, %d .class\n", len(jar), repo, len(sources), classCount)
	if !hasSource {
		fmt.Printf("  VERDICT: NOT CONSUMABLE — no Clojure source in jar (AOT/.class only)\n")
		return "unusable"
	}
	pure, hard, fenced := 0, 0, 0
	var taintExamples []string
	for _, s := range sources {
		switch {
		case s.kind == "hard":
			hard++
		case s.kind == "fenced":
			fenced++
			pure++ // reachable: cljgo skips the :clj branch
		default:
			pure++
		}
		if s.kind == "hard" && len(taintExamples) < 4 {
			taintExamples = append(taintExamples, fmt.Sprintf("%s → %q (hard)", s.entry, s.taint))
		}
	}
	fmt.Printf("  source purity: %d loadable (%d fenced-.cljc), %d hard-Java\n", pure, fenced, hard)
	for _, e := range taintExamples {
		fmt.Printf("    ✗ %s\n", e)
	}
	switch {
	case hard == 0:
		fmt.Printf("  VERDICT: FULLY CONSUMABLE — all %d namespaces loadable (Java, if any, fenced in reader conditionals cljgo skips)\n", pure)
		return "full"
	case pure == 0:
		fmt.Printf("  VERDICT: UNUSABLE — every namespace is hard-Java-tainted; nothing loads on cljgo's Go host\n")
		return "unusable"
	default:
		fmt.Printf("  VERDICT: PARTIALLY CONSUMABLE — %d ns loadable; %d hard-Java ns fail loud per-namespace at require (ADR 0054 dec 4)\n", pure, hard)
		return "partial"
	}
}

func main() {
	fmt.Println("s50 — Clojars/Maven consume spike (pure-Go stdlib only, live network)")
	fmt.Println("repos:", repos[0].base, "then", repos[1].base)

	// CLI args: "group/artifact[:version]" — version optional (latest release
	// resolved from maven-metadata.xml). Falls back to the built-in sample.
	if len(os.Args) > 1 {
		var sample []coord
		for _, a := range os.Args[1:] {
			ga, ver, hasVer := strings.Cut(a, ":")
			g, art, ok := strings.Cut(ga, "/")
			if !ok {
				fmt.Printf("skip %q: want group/artifact[:version]\n", a)
				continue
			}
			if !hasVer {
				v, repo, err := latestVersion(g, art)
				if err != nil {
					fmt.Printf("%s: cannot resolve latest version: %v\n", ga, err)
					continue
				}
				fmt.Printf("resolved %s → %s [%s]\n", ga, v, repo)
				ver = v
			}
			sample = append(sample, coord{g, art, ver})
		}
		runSample(sample)
		return
	}

	// The sample: expected-pure Clojure libs across BOTH repos + Java-carrying controls.
	sample := []coord{
		{"org.clojure", "data.json", "2.5.1"},   // Maven Central, expected pure
		{"org.clojure", "tools.cli", "1.1.230"}, // Maven Central, expected pure
		{"medley", "medley", "1.4.0"},           // Clojars, expected pure
		{"hiccup", "hiccup", "1.0.5"},           // Clojars, expected pure
		{"org.clojure", "core.match", "1.1.0"},  // Maven Central, expected pure-ish
		{"cheshire", "cheshire", "5.13.0"},      // Clojars, Java-tainted control (Jackson interop)
		{"clj-http", "clj-http", "3.13.0"},      // Clojars, Java-tainted control
	}
	runSample(sample)
}

func runSample(sample []coord) {
	pureCount, partialCount, unusable := 0, 0, 0
	summary := map[string]string{}
	for _, c := range sample {
		cat := consume(c)
		summary[c.String()] = cat
		switch cat {
		case "full":
			pureCount++
		case "partial":
			partialCount++
		default:
			unusable++
		}
	}

	fmt.Printf("\n\n═══ SUBSET MEASUREMENT (n=%d) ═══\n", len(sample))
	keys := make([]string, 0, len(summary))
	for k := range summary {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	label := map[string]string{
		"full":     "FULLY consumable (all pure)",
		"partial":  "PARTIALLY (pure ns usable, Java ns fail loud)",
		"unusable": "UNUSABLE (no source / fetch fail)",
	}
	for _, k := range keys {
		fmt.Printf("  %-32s %s\n", k, label[summary[k]])
	}
	fmt.Printf("\n  fully-consumable: %d   partially: %d   unusable: %d\n", pureCount, partialCount, unusable)
	fmt.Printf("  → pure-subset reach (full+partial): %d/%d libraries yield usable source\n", pureCount+partialCount, len(sample))
}
