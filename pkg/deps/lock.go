package deps

// The committed lockfile, build.lock.edn (ADR 0052 decision 3). It is the only
// dependency artifact that can be READ rather than RUN (build.cljgo is code),
// and it is where transitivity lives (decision 5). Deterministic: deps are
// name-sorted and map keys sorted, so the file is byte-identical across
// machines.

import (
	"fmt"
	"os"
	"sort"
)

// LockVersion is the schema version this cljgo speaks.
const LockVersion = 1

// GoReq is one Go-module requirement (ADR 0021 go-require / ADR 0044 ffi).
type GoReq struct {
	Path    string
	Version string
}

// Impurity is a dependency's non-pure-Clojure surface, read at resolve time to
// gate capabilities (decision 6). GoRequire flows into the consumer's go.mod;
// CLink (cgo/c-link library names) and FFI (purego library names) drive the
// ffi/cgo capability switches.
type Impurity struct {
	GoRequire []GoReq
	CLink     []string
	FFI       []string
}

// caps returns the capability names this impurity requires the consumer to
// acknowledge. :ffi and :cgo are deliberately separate switches (decision 6);
// third-party go-require is its own impurity (§6a).
func (imp *Impurity) caps() []string {
	if imp == nil {
		return nil
	}
	var c []string
	if len(imp.FFI) > 0 {
		c = append(c, "ffi")
	}
	if len(imp.CLink) > 0 {
		c = append(c, "cgo")
	}
	if len(imp.GoRequire) > 0 {
		c = append(c, "go-require")
	}
	return c
}

func (imp *Impurity) empty() bool {
	return imp == nil || (len(imp.GoRequire) == 0 && len(imp.CLink) == 0 && len(imp.FFI) == 0)
}

// LockedDep is one dependency's pinned record.
type LockedDep struct {
	Name     string
	GitURL   string
	GitRef   string // provenance — a moving human label
	GitSHA   string // identity — what actually pins
	TreeHash string
	Paths    []string
	Requires []string // transitive dependency NAMES
	Pure     bool
	Impure   *Impurity // nil when Pure
	// LocalUnlocked marks a :path (local) dep: a NAMED HOLE recorded with
	// :local/unlocked? true, never hashed (decision 3).
	LocalUnlocked bool
}

// Lock is the whole build.lock.edn.
type Lock struct {
	Version   int
	BuildHash string
	Deps      []LockedDep
}

// find returns the locked dep of the given name, or nil.
func (l *Lock) find(name string) *LockedDep {
	if l == nil {
		return nil
	}
	for i := range l.Deps {
		if l.Deps[i].Name == name {
			return &l.Deps[i]
		}
	}
	return nil
}

// LoadLock reads build.lock.edn. It returns (nil, nil) when the file is absent
// — an unlocked project is not an error.
func LoadLock(path string) (*Lock, error) {
	form, err := readEDNFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	l := &Lock{
		Version:   ednInt(ednGet(form, "lock/version")),
		BuildHash: ednStr(ednGet(form, "build/hash")),
	}
	if l.Version != LockVersion {
		return nil, fmt.Errorf("%s: lock/version %d, this cljgo speaks %d", path, l.Version, LockVersion)
	}
	for _, e := range ednSlice(ednGet(form, "deps")) {
		d := LockedDep{
			Name:     ednStr(ednGet(e, "name")),
			GitURL:   ednStr(ednGet(e, "git/url")),
			GitRef:   ednStr(ednGet(e, "git/ref")),
			GitSHA:   ednStr(ednGet(e, "git/sha")),
			TreeHash: ednStr(ednGet(e, "tree/hash")),
			Paths:    ednStrs(ednGet(e, "paths")),
			Requires: ednStrs(ednGet(e, "requires")),
		}
		if v := ednGet(e, "local/unlocked?"); v == true {
			d.LocalUnlocked = true
		}
		if imp := ednGet(e, "impure"); imp != nil {
			d.Impure = readImpurity(imp)
		}
		d.Pure = d.Impure == nil
		l.Deps = append(l.Deps, d)
	}
	return l, nil
}

// readImpurity reads an :impure {…} block from the lock.
func readImpurity(v any) *Impurity {
	imp := &Impurity{
		CLink: ednStrs(ednGet(v, "c-link")),
		FFI:   ednStrs(ednGet(v, "ffi")),
	}
	for _, g := range ednSlice(ednGet(v, "go-require")) {
		imp.GoRequire = append(imp.GoRequire, GoReq{
			Path:    ednStr(ednGet(g, "path")),
			Version: ednStr(ednGet(g, "version")),
		})
	}
	if imp.empty() {
		return nil
	}
	return imp
}

// WriteLock writes build.lock.edn deterministically: deps name-sorted, map keys
// sorted (via the emitter), so two independent writes of the same graph are
// byte-identical. The lock is authoritative on :git/sha (decision 3).
func WriteLock(path string, l *Lock) error {
	deps := append([]LockedDep(nil), l.Deps...)
	sort.Slice(deps, func(i, j int) bool { return deps[i].Name < deps[j].Name })

	out := make([]ednVal, 0, len(deps))
	for _, d := range deps {
		m := map[kw]ednVal{"name": d.Name}
		if d.LocalUnlocked {
			// A :path dep is a NAMED HOLE — its name and contributed roots,
			// never a hash (decision 3). It is not reproducible across machines
			// and must not pretend to be.
			m["local/unlocked?"] = true
		} else {
			m["git/url"] = d.GitURL
			m["git/ref"] = d.GitRef
			m["git/sha"] = d.GitSHA
			m["tree/hash"] = d.TreeHash
		}
		m["paths"] = strVals(d.Paths)
		m["requires"] = strVals(d.Requires)
		if d.Impure != nil && !d.Impure.empty() {
			m["impure"] = emitImpurity(d.Impure)
		} else {
			m["pure?"] = true
		}
		out = append(out, m)
	}

	version := l.Version
	if version == 0 {
		version = LockVersion
	}
	top := map[kw]ednVal{
		"lock/version": version,
		"build/hash":   l.BuildHash,
		"deps":         out,
	}
	body := ";; GENERATED by cljgo — commit this file, do not hand-edit.\n" +
		emitEDN(top, "") + "\n"
	return os.WriteFile(path, []byte(body), 0o644)
}

func emitImpurity(imp *Impurity) map[kw]ednVal {
	m := map[kw]ednVal{}
	if len(imp.GoRequire) > 0 {
		gr := make([]ednVal, 0, len(imp.GoRequire))
		for _, g := range imp.GoRequire {
			gr = append(gr, map[kw]ednVal{"path": g.Path, "version": g.Version})
		}
		m["go-require"] = gr
	}
	if len(imp.CLink) > 0 {
		m["c-link"] = strVals(imp.CLink)
	}
	if len(imp.FFI) > 0 {
		m["ffi"] = strVals(imp.FFI)
	}
	return m
}
