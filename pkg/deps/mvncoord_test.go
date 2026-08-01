package deps

import "testing"

// TestDefaultReposMatchToolsDeps pins the repository ORDER against tools.deps,
// because getting it wrong is silent.
//
// tools.deps returns "central, then clojars, then other repos"
// unconditionally (0.22.1492, clojure/tools/deps/util/maven.clj:161-165).
// cljgo fetches from the first repository that answers, so if these two
// disagree, a coordinate published to BOTH resolves to a DIFFERENT ARTIFACT
// on cljgo than on the JVM — with no conflict, no diagnostic, and nothing in
// the project files to hint at it.
//
// That is the one failure a dual-host `.cljc` project cannot tolerate: its
// whole promise is identical behaviour on both hosts, and this would break it
// below the level anyone would think to look. This list was Clojars-first,
// with a comment claiming that WAS the tools.deps default (spike s79).
func TestDefaultReposMatchToolsDeps(t *testing.T) {
	const (
		central = "https://repo1.maven.org/maven2"
		clojars = "https://repo.clojars.org"
	)
	if len(DefaultMvnRepos) != 2 {
		t.Fatalf("DefaultMvnRepos = %v, want exactly [central clojars]", DefaultMvnRepos)
	}
	if DefaultMvnRepos[0] != central {
		t.Errorf("first repo = %q, want Maven Central %q — tools.deps returns central FIRST",
			DefaultMvnRepos[0], central)
	}
	if DefaultMvnRepos[1] != clojars {
		t.Errorf("second repo = %q, want Clojars %q", DefaultMvnRepos[1], clojars)
	}
}
