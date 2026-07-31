package deps

// resolveMvn — the THIRD arm of resolveOne, beside resolvePath and resolveGit
// (ADR 0095 decision 1). Not a subsystem: a Maven dependency is an ordinary
// resolved dep whose tree happens to arrive by HTTP + unzip instead of by git,
// mounted as an ordinary load-path root and locked with the same tree hash.
//
// The honest scope, stated once: cljgo consumes PURE-CLOJURE libraries.
// Everything else fails loud, per namespace. It does not "consume the Clojure
// ecosystem" — spike s50 sampled seven real libraries and found the reachable
// set is utility/algorithm shaped (2 fully consumable, 4 partial, 1 unusable).
//
// And the second half of that honesty, added after the report was caught
// over-claiming against real Clojars: resolve-time classification measures
// "reads on cljgo, no Java interop" — nothing more. It does not compile the
// namespace, so it cannot promise the namespace compiles. The report says
// exactly what it measured, and a namespace that passes and still fails
// raises G5020 naming that gap as cljgo's, not the library's.

import (
	"os"
	"path/filepath"
)

// resolveMvn resolves one Maven coordinate: pin it, fetch and validate its
// POM, fetch and extract its jar, verify the content against the lock,
// classify its namespaces, and return the transitive edges to enqueue.
func resolveMvn(rd *rdep, root string, opts ResolveOptions) ([]Dep, error) {
	c := Coord{Group: rd.mvnGroup, Artifact: rd.mvnArtifact, Version: rd.MvnVersion}
	lk := opts.Lock.find(rd.Name)

	// Lock/build divergence is a hard error naming both versions — the lock is
	// authoritative and is never silently re-pinned (same shape as git).
	pinnedRepo := ""
	if pinIsCurrent(lk, rd, opts) {
		if lk.MvnVersion != "" && lk.MvnVersion != c.Version {
			return nil, codedf("G5013", "lock/build divergence for %q", rd.Name).
				withExpectedFound("build.lock.edn pins "+lk.MvnVersion, "build.cljgo asks for "+c.Version).
				withFix("edit build.cljgo to agree, or rebuild without --locked to re-pin")
		}
		pinnedRepo = lk.MvnRepo
	}

	// A vendor/<name>/ override wins over the cache entirely (air-gap hatch),
	// under the same lock hash — exactly as it does for a git dep. Its POM is
	// still needed for the transitive graph, so vendoring a dep with children
	// still consults the cached pom.
	if opts.VendorDir != "" {
		vd := filepath.Join(opts.VendorDir, filepath.FromSlash(rd.Name))
		if fi, err := os.Stat(vd); err == nil && fi.IsDir() {
			th, err := TreeHash(vd)
			if err != nil {
				return nil, err
			}
			if lk != nil && lk.TreeHash != "" && lk.TreeHash != th {
				return nil, codedf("G5012", "vendor/%s does not match the lock", rd.Name).
					withExpectedFound(lk.TreeHash, th).
					withFix("vendor was populated from a different artifact, or edited in place — re-populate it")
			}
			rd.tree = th
			rd.base = vd
			rd.mvnRepo = pinnedRepo
			return finishMvn(rd, vd, c, nil, opts)
		}
	}

	pomBytes, repo, err := fetchArtifact(root, c, ".pom", pinnedRepo, opts)
	if err != nil {
		return nil, err
	}
	rd.mvnRepo = repo
	rd.mvnPomSHA = sha256Hex(pomBytes)
	if lk != nil && lk.MvnPomSHA != "" && lk.MvnVersion == c.Version && lk.MvnPomSHA != rd.mvnPomSHA {
		return nil, codedf("G5012", "maven artifact checksum mismatch for %s (pom)", c).
			withExpectedFound(lk.MvnPomSHA+" (build.lock.edn)", rd.mvnPomSHA).
			withFix("run `cljgo cache clean` and resolve again")
	}

	pom, err := parsePOM(pomBytes, c)
	if err != nil {
		return nil, err
	}
	eff, err := effectivePOM(pom, c, parentFetcherFor(root, opts))
	if err != nil {
		return nil, err
	}
	rd.mvnParents = eff.chain
	edges, pruned, err := pomChildren(eff, c, rd.mvnExcl)
	if err != nil {
		return nil, err
	}
	rd.mvnPruned = pruned

	dst := mvnSrcDir(root, repo, c)
	if _, statErr := os.Stat(dst); statErr != nil {
		jar, _, err := fetchArtifact(root, c, ".jar", repo, opts)
		if err != nil {
			return nil, err
		}
		rd.mvnJarSHA = sha256Hex(jar)
		if lk != nil && lk.MvnSHA256 != "" && lk.MvnVersion == c.Version && lk.MvnSHA256 != rd.mvnJarSHA {
			return nil, codedf("G5012", "maven artifact checksum mismatch for %s (jar)", c).
				withExpectedFound(lk.MvnSHA256+" (build.lock.edn)", rd.mvnJarSHA).
				withFix("run `cljgo cache clean` and resolve again")
		}
		if err := publishMvnTree(root, dst, jar, c); err != nil {
			return nil, err
		}
	} else if lk != nil {
		rd.mvnJarSHA = lk.MvnSHA256
	}
	if rd.mvnJarSHA == "" {
		// The tree was already cached but the lock had no sha (a fresh
		// -update against a warm cache): re-read the cached jar bytes rather
		// than recording an empty hash.
		if jar, _, err := fetchArtifact(root, c, ".jar", repo, opts); err == nil {
			rd.mvnJarSHA = sha256Hex(jar)
		}
	}

	th, err := TreeHash(dst)
	if err != nil {
		return nil, err
	}
	if lk != nil && lk.TreeHash != "" && lk.MvnVersion == c.Version && lk.TreeHash != th {
		return nil, codedf("G5012", "integrity failure for %s: the cache entry does not match the lock", c).
			withExpectedFound(lk.TreeHash, th).
			withFix("run `cljgo cache clean` and re-resolve")
	}
	rd.tree = th
	rd.base = dst

	return finishMvn(rd, dst, c, edges, opts)
}

// parentFetcherFor returns the <parent>-POM fetcher effectivePOM walks the
// chain with. A parent is fetched exactly like any other artifact — same
// cache, same repository list, same G5010 when it is missing — but with NO
// pinned repository: a Clojars-hosted child routinely inherits from a POM that
// only exists on Maven Central (every org.clojure contrib artifact does).
//
// Parent POMs are cached as ordinary blobs, so a warm cache resolves them
// offline. They are NOT hashed into build.lock.edn: the lock records the
// child's own pom sha and the extracted tree hash, and a parent can only
// influence the RESULT through the edges it contributes, which are themselves
// locked coordinates. (Known gap, stated rather than hidden: a mutated parent
// POM in a repository that also allows re-publishing a child would not be
// caught by a checksum, only by the resulting edge set changing.)
func parentFetcherFor(root string, opts ResolveOptions) parentFetcher {
	return func(pc Coord) (*pomXML, error) {
		b, _, err := fetchArtifact(root, pc, ".pom", "", opts)
		if err != nil {
			return nil, err
		}
		return parsePOM(b, pc)
	}
}

// publishMvnTree extracts the jar into a temp dir and publishes it atomically,
// read-only, into the content-addressed cache — the same discipline as a git
// dep's materialize.
func publishMvnTree(root, dst string, jar []byte, c Coord) error {
	srcBase := filepath.Join(root, "src")
	if err := os.MkdirAll(srcBase, 0o755); err != nil {
		return err
	}
	return withLock(root, dst, func() error {
		if fi, err := os.Stat(dst); err == nil && fi.IsDir() {
			return nil // a racer published while we waited
		}
		tmp, err := os.MkdirTemp(srcBase, ".mvn-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		if err := extractJar(jar, tmp, c); err != nil {
			return err
		}
		if err := markReadOnly(tmp); err != nil {
			return err
		}
		return publishAtomically(tmp, dst)
	})
}

// finishMvn classifies the extracted tree and shapes the rdep. A Maven dep has
// no cljgo.manifest.edn, so :paths is [""] — the jar root IS the source root —
// and it declares no ffi/cgo/go-require impurity (ADR 0052 purity is a
// different notion from Java taint and both are kept, separately).
func finishMvn(rd *rdep, dir string, c Coord, edges []pomEdge, opts ResolveOptions) ([]Dep, error) {
	sum, verdicts, err := classifyTree(dir, c)
	if err != nil {
		return nil, err
	}
	rd.mvnNS = sum
	rd.mvnVerdicts = verdicts
	rd.paths = []string{""}
	rd.imp = nil

	var kids []Dep
	for _, e := range edges {
		kids = append(kids, Dep{
			Name:       e.Coord.Key(),
			MvnVersion: e.Coord.Version,
			mvnExcl:    e.Exclusions,
			mvnFrom:    rd.Name,
		})
		rd.reqs = append(rd.reqs, e.Coord.Key())
	}
	rd.reqs = uniqSorted(rd.reqs)
	return kids, nil
}
