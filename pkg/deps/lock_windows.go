//go:build windows

package deps

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"time"
)

// withLock is the Windows fallback for the POSIX flock in lock_unix.go. Windows
// has no flock; ADR 0048 §1 flags a Windows equivalent as owed. This is an
// advisory create-exclusive lockfile spin: O_CREATE|O_EXCL is atomic on
// Windows, so at most one process holds the lock at a time. Correctness of the
// cache does not depend on the lock alone — materialization publishes via
// atomic rename into an immutable slot, so a losing racer only ever discards
// duplicate work.
//
// TODO(deps, ADR 0048): a robust Windows equivalent (LockFileEx) if contention
// or crash-during-hold ever proves this spin insufficient. A stale lockfile
// from a crashed holder is reclaimed after staleLock.
func withLock(root, key string, fn func() error) error {
	lockDir := filepath.Join(root, "lock")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return err
	}
	h := sha256.Sum256([]byte(key))
	p := filepath.Join(lockDir, hex.EncodeToString(h[:])[:32]+".lock")

	const staleLock = 2 * time.Minute
	for {
		f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o644)
		if err == nil {
			f.Close()
			break
		}
		if !os.IsExist(err) {
			return err
		}
		if fi, statErr := os.Stat(p); statErr == nil && time.Since(fi.ModTime()) > staleLock {
			_ = os.Remove(p) // reclaim a lock left by a crashed holder
			continue
		}
		time.Sleep(10 * time.Millisecond)
	}
	defer os.Remove(p)
	return fn()
}
