//go:build windows

package deps

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// ERROR_SHARING_VIOLATION is Windows error 32 — returned when a file is open
// elsewhere without FILE_SHARE_DELETE. Not a named const in package syscall, so
// spelled out here.
const errSharingViolation = syscall.Errno(32)

// contended reports whether an O_CREATE|O_EXCL failure means "another racer
// holds (or is mid-deleting) the lockfile" — i.e. spin and retry — rather than a
// genuine error. On Windows a lockfile caught mid-deletion (a racer's
// `defer os.Remove`) makes the exclusive create fail with ERROR_ALREADY_EXISTS,
// ERROR_ACCESS_DENIED, OR ERROR_SHARING_VIOLATION; all three are contention.
func contended(err error) bool {
	if os.IsExist(err) || os.IsPermission(err) { // EEXIST, ERROR_ACCESS_DENIED
		return true
	}
	var errno syscall.Errno
	return errors.As(err, &errno) && errno == errSharingViolation
}

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
	// Bound the spin so a genuinely stuck lockfile (not a transient racer)
	// eventually errors instead of hanging forever — the POSIX flock blocks, but
	// there the kernel guarantees progress; here we poll, so we need a ceiling.
	deadline := time.Now().Add(2 * staleLock)
	for {
		f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o644)
		if err == nil {
			f.Close()
			break
		}
		if !contended(err) {
			return err // a real error (bad path, disk full), not lock contention
		}
		// Contention (or a lockfile mid-deletion): reclaim if stale, else spin.
		if fi, statErr := os.Stat(p); statErr == nil && time.Since(fi.ModTime()) > staleLock {
			_ = os.Remove(p) // reclaim a lock left by a crashed holder
			continue
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
	defer os.Remove(p)
	return fn()
}
