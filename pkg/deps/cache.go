// Package deps implements ADR 0048 dependency resolution for cljgo: a global
// content-addressed cache (keyed by identity, verified by content), a
// deterministic committed lockfile (build.lock.edn), a resolver that reads the
// lock and a dependency's declarative manifest as DATA (never evaluating a
// dependency's build fn), a hard-error version-conflict merge for Go modules
// (no MVS, no solver), and a default-deny purity gate with separate :ffi/:cgo
// switches.
//
// This package is re-authored from the frozen spikes S30–S33 (ADR 0027 forbids
// merging spike code verbatim). It imports only pkg/reader (+ its pkg/lang data
// types) and the standard library; it must not import pkg/eval, pkg/build,
// pkg/emit, or pkg/analyzer, to keep those free to import deps without a cycle.
package deps

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CacheRoot resolves the global cache root, in precedence order:
//
//	$CLJGO_CACHE           explicit override (also S33's "different machine" proxy)
//	$XDG_CACHE_HOME/cljgo   ADR 0048 §1
//	~/.cache/cljgo          fallback
//
// It errors only when no override is set and the user's home directory cannot
// be determined.
func CacheRoot() (string, error) {
	if v := os.Getenv("CLJGO_CACHE"); v != "" {
		return v, nil
	}
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		return filepath.Join(v, "cljgo"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine cache root: %w", err)
	}
	return filepath.Join(home, ".cache", "cljgo"), nil
}

// IdentityKey is the cache-slot key for a dependency: sha256 hex of
// url‖sha‖subdir, computable BEFORE the fetch (ADR 0048 §1 "key by identity").
// The content is what VERIFIES the entry (TreeHash); the key only locates it.
func IdentityKey(url, sha, subdir string) string {
	h := sha256.Sum256([]byte(url + "\x00" + sha + "\x00" + subdir))
	return hex.EncodeToString(h[:])
}

// srcDir is the immutable source-tree slot for an identity.
func srcDir(root, url, sha, subdir string) string {
	return filepath.Join(root, "src", IdentityKey(url, sha, subdir))
}

// mirrorDir is the bare git mirror slot for a URL.
func mirrorDir(root, url string) string {
	h := sha256.Sum256([]byte(url))
	return filepath.Join(root, "dl", hex.EncodeToString(h[:])+".git")
}

// TreeHash is a merkle hash over the materialized tree: for every file in
// sorted relative-path order, its slash-path, mode (regular / executable /
// symlink), and the sha256 of its bytes (symlinks hash their target). `.git` is
// excluded so a tree hashes identically with or without a repo. It is
// recomputed on EVERY read (ADR 0048 §1 "verify by content"): a git SHA alone
// is not a content guarantee.
func TreeHash(dir string) (string, error) {
	h := sha256.New()
	var entries []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		entries = append(entries, rel)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(entries)
	for _, rel := range entries {
		p := filepath.Join(dir, rel)
		fi, err := os.Lstat(p)
		if err != nil {
			return "", err
		}
		mode := "100644"
		var payload []byte
		switch {
		case fi.Mode()&os.ModeSymlink != 0:
			mode = "120000"
			t, err := os.Readlink(p)
			if err != nil {
				return "", err
			}
			payload = []byte(t)
		default:
			if fi.Mode().Perm()&0o111 != 0 {
				mode = "100755"
			}
			payload, err = os.ReadFile(p)
			if err != nil {
				return "", err
			}
		}
		ch := sha256.Sum256(payload)
		fmt.Fprintf(h, "%s\x00%s\x00%s\n", filepath.ToSlash(rel), mode, hex.EncodeToString(ch[:]))
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// publishAtomically renames tmp into dst. If dst already exists (a racer won)
// tmp is discarded and the existing entry wins — entries are immutable, so both
// are the same bytes. No temporary directory is left behind on either path.
func publishAtomically(tmp, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		if _, statErr := os.Stat(dst); statErr == nil {
			return os.RemoveAll(tmp)
		}
		return err
	}
	return nil
}

// markReadOnly strips write bits from a published entry (0555 dirs / 0444
// files). Not security — the tree hash is integrity; this is the "you are
// editing the cache" tripwire.
func markReadOnly(dir string) error {
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.Chmod(p, 0o555)
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		return os.Chmod(p, fi.Mode().Perm()&^0o222)
	})
}

// makeWritable restores write bits so an immutable (0555) tree can be removed.
func makeWritable(dir string) error {
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		return os.Chmod(p, fi.Mode().Perm()|0o200)
	})
}

// CacheClean removes the global cache, first restoring write bits so the 0555
// immutable trees delete cleanly (a plain `rm -rf` cannot). Absent cache is a
// no-op. Required by ADR 0048 §1: a user cannot rm a read-only tree cleanly.
func CacheClean() error {
	root, err := CacheRoot()
	if err != nil {
		return err
	}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil
	}
	if err := makeWritable(root); err != nil {
		return err
	}
	return os.RemoveAll(root)
}

func shortSHA(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

func shortHash(s string) string {
	s = strings.TrimPrefix(s, "sha256:")
	if len(s) > 12 {
		return "sha256:" + s[:12] + "…"
	}
	return "sha256:" + s
}
