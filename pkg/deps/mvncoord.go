package deps

// Maven/Clojars coordinates (ADR 0095 decision 1, evidence spike s50).
//
// The dependency NAME *is* the coordinate: "org.clojure/tools.cli". A
// single-segment name means group == artifact ("medley" => medley/medley), the
// Maven/Leiningen convention. Because the name is also the load-path and lock
// identity, a Maven dep and a git dep can never silently collide — they are
// necessarily different names.

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
)

// Coord is a resolved Maven coordinate.
type Coord struct {
	Group    string
	Artifact string
	Version  string
}

// Key is the coordinate's identity without a version: "group/artifact". It is
// the dep name, the accept-version key, and the graph's first-wins key.
func (c Coord) Key() string { return c.Group + "/" + c.Artifact }

// String renders the coordinate the way every diagnostic names it.
func (c Coord) String() string { return c.Key() + " " + c.Version }

// splitCoordName splits a dep name into (group, artifact). A single-segment
// name means group == artifact. Returns ok=false for a name that is not a
// coordinate (empty, or more than one slash).
func splitCoordName(name string) (group, artifact string, ok bool) {
	if name == "" {
		return "", "", false
	}
	switch parts := strings.Split(name, "/"); len(parts) {
	case 1:
		return parts[0], parts[0], parts[0] != ""
	case 2:
		if parts[0] == "" || parts[1] == "" {
			return "", "", false
		}
		return parts[0], parts[1], true
	default:
		return "", "", false
	}
}

// artifactPath is the repository-relative path of one artifact file:
//
//	org/clojure/tools.cli/1.1.230/tools.cli-1.1.230.pom
//
// Only the GROUP has its dots turned into path separators; the artifact id
// keeps its dots (tools.cli is one directory).
func (c Coord) artifactPath(ext string) string {
	return strings.ReplaceAll(c.Group, ".", "/") + "/" + c.Artifact + "/" + c.Version +
		"/" + c.Artifact + "-" + c.Version + ext
}

// MvnIdentityKey is the cache-slot key for a Maven artifact: sha256 hex of
// repo‖group‖artifact‖version‖ext, computable BEFORE the fetch. It mirrors
// IdentityKey: identity LOCATES the entry, content (sha256 / TreeHash)
// VERIFIES it.
func MvnIdentityKey(repo string, c Coord, ext string) string {
	h := sha256.Sum256([]byte(repo + "\x00" + c.Group + "\x00" + c.Artifact + "\x00" + c.Version + "\x00" + ext))
	return hex.EncodeToString(h[:])
}

// mvnBlobPath is the raw-bytes slot for one downloaded artifact file (.pom /
// .jar). Raw poms are kept because they are what a WARM, OFFLINE resolve walks
// the transitive graph from — the extracted source tree contains no manifest.
func mvnBlobPath(root, repo string, c Coord, ext string) string {
	return filepath.Join(root, "mvn", MvnIdentityKey(repo, c, ext))
}

// mvnSrcDir is the extracted-source slot for a Maven coordinate.
func mvnSrcDir(root, repo string, c Coord) string {
	return filepath.Join(root, "src", MvnIdentityKey(repo, c, ".src"))
}

// DefaultMvnRepos is the repository list used when a project declares none:
// **Maven Central first, then Clojars** — matching tools.deps, which returns
// "central, then clojars, then other repos" unconditionally
// (org.clojure/tools.deps 0.22.1492, clojure/tools/deps/util/maven.clj:161-165).
// (mvn-repo …) PREPENDS to this list.
//
// The order is not cosmetic and this list used to be the other way round,
// with a comment claiming Clojars-first WAS the tools.deps default. It is
// not. Fetching takes the first repository that answers, so a coordinate
// published to BOTH resolved to a different artifact on cljgo than on the
// JVM — silently, with no conflict, no diagnostic, and nothing in the project
// files to hint at it. That is the one failure mode a dual-host `.cljc`
// project cannot tolerate: the whole promise is identical behaviour on both
// hosts, and this broke it below the level anyone would think to look.
//
// Found by spike s79 while surveying what `deps.edn` `:deps` translation
// would cost. Reading `:deps` does not cause this — it only exposes it.
var DefaultMvnRepos = []string{
	"https://repo1.maven.org/maven2",
	"https://repo.clojars.org",
}

// clojureItself is the set of coordinates pruned from every transitive graph
// (design §2.3). cljgo IS the Clojure implementation — its clojure.core is
// embedded (ADR 0043) — so fetching the JVM's own clojure.jar would mount a
// second, Java-riddled clojure/core.clj on the load path. Pruning is REPORTED,
// never silent.
var clojureItself = map[string]bool{
	"org.clojure/clojure":          true,
	"org.clojure/spec.alpha":       true,
	"org.clojure/core.specs.alpha": true,
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}
