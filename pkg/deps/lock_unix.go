//go:build !windows

package deps

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"syscall"
)

// withLock takes an exclusive advisory lock on a per-key lockfile for the
// duration of fn (POSIX flock). flock is released by the OS on process death,
// so a crashed resolver cannot wedge the cache. Concurrent cold-cache
// resolvers serialize here; losing racers discard via publishAtomically.
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
