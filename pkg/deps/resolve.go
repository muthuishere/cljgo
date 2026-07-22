package deps

// The resolver: turn declared (dep …) forms into resolved roots + a merged,
// conflict-checked Go-require set + an updated lock. It reads the lock and each
// dependency's manifest as DATA (decision 5), materializes git deps into the
// content-addressed cache (decision 1), verifies content against the lock
// (decision 1/3), merges Go-requires with a hard conflict error (decision 4),
// and gates impurity default-deny with separate ffi/cgo switches (decision 6).

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Dep is a DECLARED dependency from build.cljgo, before resolution. Git* and
// Path are mutually exclusive.
type Dep struct {
	Name   string
	GitURL string
	GitRef string
	Subdir string
	Path   string // local path dep
}

func (d Dep) isPath() bool { return d.Path != "" }

// ResolveOptions configures a resolution pass.
type ResolveOptions struct {
	ProjectDir     string
	Lock           *Lock
	Update         bool              // allow lock (re)generation from remotes
	Offline        bool              // never touch the network
	AllowCaps      map[string]bool   // consumer-acknowledged capabilities
	AcceptVersions map[string]string // module path -> accepted version
	CrossTargets   []string          // declared cross targets (cgo refusal)
	VendorDir      string            // project vendor/ base ("" = none)
}

// Resolved is the output of a resolution pass.
type Resolved struct {
	Roots      []string // dependency source roots, lock order (load-path slot 3)
	GoRequires []GoReq  // merged + conflict-checked, for the consumer go.mod
	Lock       *Lock    // possibly updated (when Update)
}

// rdep is a dependency in flight during resolution.
type rdep struct {
	Dep
	sha   string
	tree  string
	paths []string
	reqs  []string
	imp   *Impurity
	local bool
	base  string // where its tree actually lives (cache/vendor/path)
}

// Resolve resolves the declared dependency graph.
func Resolve(deps []Dep, opts ResolveOptions) (*Resolved, error) {
	root, err := CacheRoot()
	if err != nil {
		return nil, err
	}

	seen := map[string]*rdep{}
	var order []*rdep
	queue := append([]Dep(nil), deps...)

	for len(queue) > 0 {
		decl := queue[0]
		queue = queue[1:]

		if prev, ok := seen[decl.Name]; ok {
			if !prev.isPath() && !decl.isPath() && decl.GitRef != "" && prev.GitRef != "" && prev.GitRef != decl.GitRef {
				return nil, fmt.Errorf("dependency %q required at two refs: %q and %q", decl.Name, prev.GitRef, decl.GitRef)
			}
			continue
		}

		rd := &rdep{Dep: decl}
		kids, err := resolveOne(rd, root, opts)
		if err != nil {
			return nil, err
		}
		seen[decl.Name] = rd
		order = append(order, rd)
		queue = append(queue, kids...)
	}

	// Roots in lock order (slot 3).
	var roots []string
	for _, rd := range order {
		for _, p := range rd.paths {
			roots = append(roots, filepath.Join(rd.base, p))
		}
	}

	// Merge Go-requires across the graph, hard-erroring on a version conflict
	// (decision 4) unless the consumer accepted a version.
	var prov []provGoReq
	for _, rd := range order {
		if rd.imp == nil {
			continue
		}
		for _, g := range rd.imp.GoRequire {
			prov = append(prov, provGoReq{GoReq: g, From: rd.Name})
		}
	}
	goReqs, err := mergeGoReqProv(prov, opts.AcceptVersions)
	if err != nil {
		return nil, err
	}

	return &Resolved{
		Roots:      roots,
		GoRequires: goReqs,
		Lock:       buildLock(order, opts.Lock),
	}, nil
}

// resolveOne resolves a single dependency in place and returns its transitive
// children (declared deps to enqueue).
func resolveOne(rd *rdep, root string, opts ResolveOptions) ([]Dep, error) {
	if rd.isPath() {
		return resolvePath(rd, opts)
	}
	return resolveGit(rd, root, opts)
}

// resolvePath handles a local :path dependency — a named hole (decision 3):
// never hashed, load-path position and transitive deps preserved.
func resolvePath(rd *rdep, opts ResolveOptions) ([]Dep, error) {
	abs := rd.Path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(opts.ProjectDir, rd.Path)
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, fmt.Errorf(":path dep %q: %w", rd.Name, err)
	}
	man, err := readManifest(abs)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", rd.Name, err)
	}
	if err := checkPurity(rd.Name, man.Impure, opts); err != nil {
		return nil, err
	}
	rd.local = true
	rd.base = abs
	rd.paths = man.Paths
	rd.reqs = man.ReqNames
	rd.imp = man.Impure
	return man.Children, nil
}

// resolveGit handles a git dependency: pin (lock unless -update), materialize
// into the cache (or a vendor override), verify content, read its manifest.
func resolveGit(rd *rdep, root string, opts ResolveOptions) ([]Dep, error) {
	lk := opts.Lock.find(rd.Name)

	// A vendor/<name>/ override wins over the cache entirely (air-gap hatch),
	// under the same lock hash (decision 1).
	if opts.VendorDir != "" {
		vd := filepath.Join(opts.VendorDir, rd.Name)
		if fi, err := os.Stat(vd); err == nil && fi.IsDir() {
			th, err := TreeHash(vd)
			if err != nil {
				return nil, err
			}
			if lk != nil && lk.TreeHash != "" && lk.TreeHash != th {
				return nil, fmt.Errorf(
					"vendor/%s does not match the lock:\n  expected tree/hash %s\n  got      tree/hash %s\n"+
						"(vendor was populated from a different commit, or edited in place)",
					rd.Name, lk.TreeHash, th)
			}
			if lk != nil {
				rd.sha = lk.GitSHA
			}
			if rd.sha == "" {
				rd.sha = "vendored"
			}
			rd.tree = th
			rd.base = vd
			return finishGit(rd, vd, opts)
		}
	}

	// Pin the identity.
	switch {
	case lk != nil && !opts.Update:
		if rd.GitURL != "" && lk.GitURL != "" && rd.GitURL != lk.GitURL {
			return nil, fmt.Errorf(
				"lock/build divergence for %q:\n  build.cljgo :git %s\n  build.lock.edn  %s\n  run resolve with -update to re-pin",
				rd.Name, rd.GitURL, lk.GitURL)
		}
		if rd.GitRef != "" && lk.GitRef != rd.GitRef {
			// A disagreeing ref is an error naming both (decision 3): the lock
			// is authoritative and never silently re-pinned. Name SHAs when the
			// declared ref is itself a SHA.
			decl := rd.GitRef
			if len(decl) == 40 && isHex(decl) {
				return nil, fmt.Errorf(
					"lock/build divergence for %q:\n  build.cljgo pins sha %s\n  build.lock.edn pins sha %s (ref %q)\n  run resolve with -update to re-pin",
					rd.Name, decl, lk.GitSHA, lk.GitRef)
			}
			return nil, fmt.Errorf(
				"lock/build divergence for %q:\n  build.cljgo asks for ref %q\n  build.lock.edn pins ref %q (sha %s)\n  run resolve with -update to re-pin",
				rd.Name, rd.GitRef, lk.GitRef, shortSHA(lk.GitSHA))
		}
		rd.sha, rd.GitURL, rd.GitRef = lk.GitSHA, lk.GitURL, lk.GitRef
		// Purity is readable from the lock alone — gate BEFORE any fetch.
		if err := checkPurity(rd.Name, lk.Impure, opts); err != nil {
			return nil, err
		}
	case opts.Offline:
		return nil, fmt.Errorf("offline: %q is not in build.lock.edn, so its ref %q cannot be resolved without a remote", rd.Name, rd.GitRef)
	default:
		if rd.GitRef == "" {
			rd.GitRef = "HEAD"
		}
		sha, err := resolveRef(rd.GitURL, rd.GitRef)
		if err != nil {
			return nil, err
		}
		rd.sha = sha
	}

	// Materialize + VERIFY before trusting a single byte.
	dir, err := materialize(rd, root, opts.Offline)
	if err != nil {
		return nil, err
	}
	th, err := TreeHash(dir)
	if err != nil {
		return nil, err
	}
	if lk != nil && lk.TreeHash != "" && lk.GitSHA == rd.sha && lk.TreeHash != th {
		return nil, fmt.Errorf(
			"integrity failure for %q @ %s:\n  expected tree/hash %s\n  got      tree/hash %s\n  cache entry: %s\n"+
				"the cache entry does not match the lock — it was modified after it was written. "+
				"Run `cljgo cache clean` and re-resolve.",
			rd.Name, shortSHA(rd.sha), lk.TreeHash, th, dir)
	}
	rd.tree = th
	rd.base = dir
	return finishGit(rd, dir, opts)
}

// finishGit reads the resolved dep's manifest and gates its impurity.
func finishGit(rd *rdep, dir string, opts ResolveOptions) ([]Dep, error) {
	man, err := readManifest(dir)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", rd.Name, err)
	}
	if err := checkPurity(rd.Name, man.Impure, opts); err != nil {
		return nil, err
	}
	rd.paths = man.Paths
	rd.reqs = man.ReqNames
	rd.imp = man.Impure
	return man.Children, nil
}

// materialize ensures the cache holds the tree for (url, sha, subdir) and
// returns its directory. Concurrency-safe: flock around the fetch, atomic
// rename to publish, immutable once published (decision 1).
func materialize(rd *rdep, root string, offline bool) (string, error) {
	dst := srcDir(root, rd.GitURL, rd.sha, rd.Subdir)
	if fi, err := os.Stat(dst); err == nil && fi.IsDir() {
		return dst, nil
	}
	if offline {
		return "", fmt.Errorf("offline: %s@%s is not in the cache (%s)", rd.Name, shortSHA(rd.sha), dst)
	}

	key := rd.GitURL + "\x00" + rd.sha + "\x00" + rd.Subdir
	err := withLock(root, key, func() error {
		if fi, err := os.Stat(dst); err == nil && fi.IsDir() {
			return nil // a racer published while we waited
		}
		mirror := mirrorDir(root, rd.GitURL)
		if _, err := os.Stat(mirror); os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(mirror), 0o755); err != nil {
				return err
			}
			if _, err := git("", "clone", "--bare", "--quiet", rd.GitURL, mirror); err != nil {
				return err
			}
		} else if _, err := git(mirror, "cat-file", "-e", rd.sha+"^{commit}"); err != nil {
			if _, err := git(mirror, "fetch", "--quiet", "origin", "+refs/*:refs/*"); err != nil {
				return err
			}
		}

		srcBase := filepath.Join(root, "src")
		if err := os.MkdirAll(srcBase, 0o755); err != nil {
			return err
		}
		tmp, err := os.MkdirTemp(srcBase, ".tmp-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)

		// git archive materializes the tree at SHA with no .git and no mtimes
		// — deterministic, unlike a worktree checkout.
		spec := rd.sha
		if rd.Subdir != "" {
			spec = rd.sha + ":" + rd.Subdir
		}
		tarDir, err := os.MkdirTemp(srcBase, ".tar-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tarDir)
		tarPath := filepath.Join(tarDir, "t.tar")
		if _, err := git(mirror, "archive", "--format=tar", "-o", tarPath, spec); err != nil {
			return err
		}
		if err := untar(tarPath, tmp); err != nil {
			return err
		}
		if err := markReadOnly(tmp); err != nil {
			return err
		}
		return publishAtomically(tmp, dst)
	})
	if err != nil {
		return "", err
	}
	return dst, nil
}

// checkPurity enforces default-deny capability gating (decision 6). :ffi and
// :cgo are separate switches; :cgo is refused (not warned) under declared
// cross-targets; unacknowledged impurity is refused naming the capability.
func checkPurity(name string, imp *Impurity, opts ResolveOptions) error {
	if imp == nil {
		return nil
	}
	for _, cap := range imp.caps() {
		if cap == "cgo" && len(opts.CrossTargets) > 0 {
			return fmt.Errorf(
				"dependency %q requires :cgo (c-link %v), refused: the project declares cross-compile target(s) %v — "+
					"cgo against a third-party system library cannot cross-compile (ADR 0048 decision 6). "+
					"Drop the dependency, use an :ffi equivalent, or drop the cross-target.",
				name, imp.CLink, opts.CrossTargets)
		}
		if !opts.AllowCaps[cap] {
			return fmt.Errorf(
				"dependency %q declares impure capability :%s which the consumer has not acknowledged; "+
					"acknowledge :%s in the allowed capability set to permit it (default deny, ADR 0048 decision 6)",
				name, cap, cap)
		}
	}
	return nil
}

// buildLock produces the lock reflecting the resolved order. When resolving
// warm it mirrors the input lock; when updating it reflects fresh resolution.
func buildLock(order []*rdep, prev *Lock) *Lock {
	l := &Lock{Version: LockVersion}
	if prev != nil {
		l.BuildHash = prev.BuildHash
	}
	for _, rd := range order {
		d := LockedDep{
			Name:     rd.Name,
			Paths:    rd.paths,
			Requires: rd.reqs,
			Impure:   rd.imp,
			Pure:     rd.imp == nil,
		}
		if rd.local {
			d.LocalUnlocked = true
		} else {
			d.GitURL = rd.GitURL
			d.GitRef = rd.GitRef
			d.GitSHA = rd.sha
			d.TreeHash = rd.tree
		}
		l.Deps = append(l.Deps, d)
	}
	sort.Slice(l.Deps, func(i, j int) bool { return l.Deps[i].Name < l.Deps[j].Name })
	return l
}
