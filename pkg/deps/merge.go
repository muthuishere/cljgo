package deps

// Go-module version conflict handling (ADR 0048 decision 4). cljgo detects a
// duplicate module required at two different versions and HARD-ERRORS naming
// both, in its OWN layer, BEFORE the go.mod write — never delegating to
// `go mod tidy`, which silently applies MVS (exit 0, higher version wins). A
// consumer-side accept override pins one version. There is NO solver and NO
// MVS at the cljgo layer.

import (
	"fmt"
	"sort"
	"strings"
)

// provGoReq is a Go-module requirement tagged with the dependency that asked
// for it — the provenance a flattened go.mod cannot reconstruct, which makes
// the conflict error nameable (decision 4 depends on decision 3's :requires).
type provGoReq struct {
	GoReq
	From string // requiring dependency name; "" when unknown
}

// MergeGoRequires merges require-sets, hard-erroring on a duplicate module at
// two different versions (naming both), unless accept[module] pins one. It
// never applies MVS. The result is deduplicated and path-sorted.
func MergeGoRequires(sets [][]GoReq, accept map[string]string) ([]GoReq, error) {
	var in []provGoReq
	for _, s := range sets {
		for _, r := range s {
			in = append(in, provGoReq{GoReq: r})
		}
	}
	return mergeGoReqProv(in, accept)
}

// mergeGoReqProv is the provenance-aware core used by Resolve, so the error can
// name both requirers as well as both versions.
func mergeGoReqProv(in []provGoReq, accept map[string]string) ([]GoReq, error) {
	byPath := map[string][]provGoReq{}
	var order []string
	for _, r := range in {
		if r.Path == "" {
			continue
		}
		if _, ok := byPath[r.Path]; !ok {
			order = append(order, r.Path)
		}
		byPath[r.Path] = append(byPath[r.Path], r)
	}

	var out []GoReq
	for _, path := range order {
		reqs := byPath[path]
		if v, ok := accept[path]; ok {
			// Consumer override: accept this version, ignore the conflict.
			out = append(out, GoReq{Path: path, Version: v})
			continue
		}
		v0 := reqs[0].Version
		for _, r := range reqs[1:] {
			if r.Version != v0 {
				return nil, conflictErr(path, reqs)
			}
		}
		out = append(out, GoReq{Path: path, Version: v0})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// conflictErr names every distinct version pinned for a module and, where
// known, the requirer that pinned it — plus the fix (an accept override).
func conflictErr(path string, reqs []provGoReq) error {
	seen := map[string]bool{}
	var lines []string
	for _, r := range reqs {
		key := r.Version + "\x00" + r.From
		if seen[key] {
			continue
		}
		seen[key] = true
		if r.From != "" {
			lines = append(lines, fmt.Sprintf("  %s pins %s", r.From, r.Version))
		} else {
			lines = append(lines, fmt.Sprintf("  pinned at %s", r.Version))
		}
	}
	sort.Strings(lines)
	return fmt.Errorf(
		"conflicting go-require for %s at two different versions:\n%s\n"+
			"cljgo does not silently pick one (no MVS). Resolve it with an explicit "+
			"accepted version for %s in build.cljgo.",
		path, strings.Join(lines, "\n"), path)
}
