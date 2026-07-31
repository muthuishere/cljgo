package lang

import (
	"errors"
	"syscall"
)

// isDirNotEmpty recognises Windows' own "the directory is not empty" status.
//
// Windows does not report ENOTEMPTY. It reports ERROR_DIR_NOT_EMPTY (145),
// which Go's Errno.Is maps to fs.ErrExist — so without this, deleting a
// populated directory classified as :already-exists on Windows and
// :directory-not-empty everywhere else. A :reason keyword that means
// different things per platform is worse than no keyword at all: the whole
// point of the ex-data contract (ADR 0114) is that a caller can branch on it
// portably.
//
// Caught by CI on windows-latest; POSIX hid it on both other platforms.
func isDirNotEmpty(err error) bool {
	return errors.Is(err, syscall.ERROR_DIR_NOT_EMPTY)
}
