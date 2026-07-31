package deps

// Declaration hashing — the staleness mechanism ADR 0112 finishes.
//
// The lock already carried a BuildHash field that three sites read, wrote and
// propagated and that NOTHING compared. This is the comparison, plus the
// per-dependency half that makes a re-resolve minimal.
//
// WHAT IS HASHED, and what deliberately is not. The input is the NORMALISED
// DECLARED SET: for each dependency, its name, its kind, and the thing that
// identifies the version it asked for. Nothing else. In particular the bytes
// of build.cljgo are NOT an input — hashing those would make a reformat, an
// added comment, or an artifact rename re-resolve the whole graph, which is a
// surprise upgrade caused by an edit that declared nothing.
//
// A path dep contributes its declaration but no version: it is a named hole
// (ADR 0052 decision 3) whose contents are never hashed.

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// declLine renders ONE declaration canonically. Two declarations produce the
// same line exactly when they ask for the same thing.
func declLine(d Dep) string {
	switch {
	case d.isMvn() || d.MvnDeclared:
		return "mvn\x00" + d.Name + "\x00" + d.MvnVersion
	case d.isPath():
		return "path\x00" + d.Name + "\x00" + d.Path
	default:
		return "git\x00" + d.Name + "\x00" + d.GitURL + "\x00" + d.GitRef + "\x00" + d.Subdir
	}
}

// DeclHash is one declaration's hash, recorded per locked dep so a re-resolve
// can tell WHICH declarations moved rather than assuming all of them did.
func DeclHash(d Dep) string {
	sum := sha256.Sum256([]byte(declLine(d)))
	return hex.EncodeToString(sum[:8])
}

// DeclaredSetHash is the hash of the whole declared set, order-independent.
// It is the fast "did anything at all move" check; when it matches, no
// coordinate is reconsidered.
func DeclaredSetHash(deps []Dep) string {
	lines := make([]string, 0, len(deps))
	for _, d := range deps {
		lines = append(lines, declLine(d))
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:16])
}

// pinIsCurrent reports whether rd's lock entry may still be used.
//
// This is the whole of "re-resolve minimally", and it is a set difference
// rather than an engine: a declaration whose hash is unchanged keeps its pin
// even during an update, so bumping one root cannot drift every other
// transitive to whatever is newest today — which would be the lockfile
// failing at its one job.
//
// A lock written before ADR 0112 has no DeclHash. Those entries are treated as
// stale under an update (we cannot prove they did not move) and as usable when
// not updating, which is exactly the pre-0112 behaviour.
func pinIsCurrent(lk *LockedDep, rd *rdep, opts ResolveOptions) bool {
	if lk == nil {
		return false
	}
	if !opts.Update {
		return true // not updating: the lock is authoritative, as before
	}
	if lk.DeclHash == "" {
		return false
	}
	return lk.DeclHash == DeclHash(rd.Dep)
}
