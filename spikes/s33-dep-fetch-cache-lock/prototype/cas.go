package main

// Content-addressed cache: layout, tree hashing, cross-process locking,
// atomic materialization.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

// CacheRoot resolves the global cache root.
//
//	$CLJGO_CACHE            (explicit override; the spike uses it as the
//	                         "different machine" proxy)
//	$XDG_CACHE_HOME/cljgo   (ADR 0052 §1)
//	~/.cache/cljgo          (fallback)
func CacheRoot() string {
	if v := os.Getenv("CLJGO_CACHE"); v != "" {
		return v
	}
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		return filepath.Join(v, "cljgo")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".cache", "cljgo")
}

func srcDir(root, url, sha, subdir string) string {
	// Keyed by IDENTITY (url+sha+subdir), not by content: the key must be
	// computable before the fetch. Content is what VERIFIES the entry.
	h := sha256.Sum256([]byte(url + "\x00" + sha + "\x00" + subdir))
	return filepath.Join(root, "src", hex.EncodeToString(h[:])[:32])
}

func mirrorDir(root, url string) string {
	h := sha256.Sum256([]byte(url))
	return filepath.Join(root, "dl", hex.EncodeToString(h[:])[:32]+".git")
}

// TreeHash is a merkle hash over the materialized tree: for every file in
// sorted relative-path order, the path, the executable bit, and the sha256
// of its bytes. Symlinks hash their target. `.git` is excluded — a checkout
// must hash to the same value whether or not a repo came with it.
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

// withLock takes an exclusive advisory lock on a per-key lockfile for the
// duration of fn. flock is released by the OS on process death, so a
// crashed resolver cannot wedge the cache.
func withLock(root, key string, fn func() error) error {
	lockDir := filepath.Join(root, "lock")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return err
	}
	h := sha256.Sum256([]byte(key))
	p := filepath.Join(lockDir, hex.EncodeToString(h[:])[:32]+".lock")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}

// publishAtomically renames tmp into dst. If dst already exists (another
// process won the race) tmp is discarded and the existing entry wins —
// cache entries are immutable, so both are the same bytes by construction.
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

// markReadOnly strips write bits from a published entry. Not security — it
// is the "you are editing the cache" tripwire; integrity is the tree hash.
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

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
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
	return s
}
