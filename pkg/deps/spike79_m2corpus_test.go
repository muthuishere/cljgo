package deps

// Spike s79 harness — would cljgo's Maven resolver agree with tools.deps on a
// REAL dependency graph?
//
// Drives the PRODUCTION POM walker (parsePOM / effectivePOM / pomChildren,
// mvnpom.go) over poms already cached in the local ~/.m2 repository, with a
// parentFetcher that reads from disk. No network: every pom must already be
// cached, and an uncached parent/edge is COUNTED, not fetched.
//
// The roots are read from a file of `group/artifact/version` lines produced by
// `clojure -Spath` on a corpus of real deps.edn projects — so the graph being
// walked is the one tools.deps actually produced, and the question is only
// whether cljgo's walker accepts it.
//
// Not a correctness test: it asserts nothing and SKIPS unless
// CLJGO_S79_M2=<roots-file> is set. See
// spikes/s79-deps-edn-deps-translation/RESULTS.md.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func s79ReadPOM(m2 string, c Coord) (*pomXML, error) {
	b, err := os.ReadFile(filepath.Join(m2, filepath.FromSlash(c.artifactPath(".pom"))))
	if err != nil {
		return nil, err
	}
	return parsePOM(b, c)
}

// s79Walk expands one root the way resolve.go does: BFS, first-wins on the
// coordinate Key(), clojure-itself pruned, exclusions inherited.
func s79Walk(m2 string, root Coord) (nodes int, uncached int, conflicts []string, refusals []string) {
	type item struct {
		c    Coord
		excl []string
		from string
	}
	seen := map[string]Coord{}
	from := map[string]string{}
	q := []item{{c: root}}
	for len(q) > 0 {
		it := q[0]
		q = q[1:]
		if clojureItself[it.c.Key()] {
			continue
		}
		if prev, ok := seen[it.c.Key()]; ok {
			if prev.Version != it.c.Version {
				conflicts = append(conflicts, fmt.Sprintf("%s: %s (from %s) vs %s (from %s)",
					it.c.Key(), prev.Version, from[it.c.Key()], it.c.Version, it.from))
			}
			continue
		}
		seen[it.c.Key()] = it.c
		from[it.c.Key()] = it.from
		p, err := s79ReadPOM(m2, it.c)
		if err != nil {
			uncached++
			continue
		}
		eff, err := effectivePOM(p, it.c, func(pc Coord) (*pomXML, error) { return s79ReadPOM(m2, pc) })
		if err != nil {
			refusals = append(refusals, fmt.Sprintf("%s: %v", it.c, err))
			continue
		}
		edges, _, err := pomChildren(eff, it.c, it.excl)
		if err != nil {
			refusals = append(refusals, fmt.Sprintf("%s: %v", it.c, err))
			continue
		}
		for _, e := range edges {
			q = append(q, item{c: e.Coord, excl: e.Exclusions, from: it.c.Key()})
		}
	}
	return len(seen), uncached, conflicts, refusals
}

func TestSpike79M2Corpus(t *testing.T) {
	rootsFile := os.Getenv("CLJGO_S79_M2")
	if rootsFile == "" {
		t.Skip("set CLJGO_S79_M2=<file of group/artifact/version lines> to run the s79 corpus walk")
	}
	m2 := os.Getenv("CLJGO_S79_M2_REPO")
	if m2 == "" {
		m2 = filepath.Join(os.Getenv("HOME"), ".m2", "repository")
	}
	b, err := os.ReadFile(rootsFile)
	if err != nil {
		t.Fatal(err)
	}
	var okN, refusedN, conflictN, uncachedRoots int
	var lines []string
	for _, ln := range strings.Split(string(b), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		parts := strings.Split(ln, "/")
		if len(parts) != 3 {
			t.Fatalf("bad root line %q (want group/artifact/version)", ln)
		}
		c := Coord{Group: parts[0], Artifact: parts[1], Version: parts[2]}
		n, unc, confl, ref := s79Walk(m2, c)
		status := "OK"
		switch {
		case len(ref) > 0:
			status = "REFUSED"
			refusedN++
		case len(confl) > 0:
			status = "CONFLICT"
			conflictN++
		default:
			okN++
		}
		if unc > 0 {
			uncachedRoots++
		}
		lines = append(lines, fmt.Sprintf("%-9s %-46s nodes=%-4d uncached=%-3d %s",
			status, c.String(), n, unc, strings.Join(append(ref, confl...), " | ")))
	}
	sort.Strings(lines)
	for _, l := range lines {
		t.Log(l)
	}
	t.Logf("roots=%d clean=%d refused=%d conflict=%d roots-with-uncached-edges=%d",
		okN+refusedN+conflictN, okN, refusedN, conflictN, uncachedRoots)
}
