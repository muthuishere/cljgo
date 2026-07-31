package lang

import (
	"errors"
	"io/fs"
	"syscall"
)

// io_error.go — ADR 0114: cljg.io/cljg.process errors read as Clojure, with
// the Go detail preserved in ex-data. A cljgo user has not signed up to read
// Go idioms (`open x: no such file or directory`); NewIOError builds the
// ex-info an operation should throw instead: a message that names the
// operation and a stable, closed-set :reason a caller can branch on
// portably, plus the original Go error text verbatim under :go/error so
// nothing is lost.
//
// Scope (ADR 0114): this is the raise-site helper for the FIRST slice —
// slurp, cljg.io/read-bytes, cljg.io/mkdirs, cljg.io/delete! — not a
// general translation layer. Every call site still writes its own message;
// this only supplies the shared, closed reason classification so it is not
// duplicated per call site.

// IOReason is the stable, closed set of failure kinds a cljgo caller can
// branch on without matching host-specific prose. It is deliberately small
// and append-only in spirit (ADR 0114) — new members require an ADR note,
// not a per-call-site judgment call.
const (
	IOReasonNotFound          = "not-found"
	IOReasonPermissionDenied  = "permission-denied"
	IOReasonNotADirectory     = "not-a-directory"
	IOReasonDirectoryNotEmpty = "directory-not-empty"
	IOReasonAlreadyExists     = "already-exists"
	IOReasonLoop              = "loop"
	IOReasonUnknown           = "unknown"
)

// classifyIOReason maps a Go filesystem/process error to one member of
// IOReason via errors.Is against the stdlib sentinel errors and, where the
// platform's errno maps cleanly, the POSIX errno values the issue named
// (ENOTDIR/ENOTEMPTY/ELOOP). fs.ErrNotExist/ErrPermission/ErrExist are the
// portable, cross-platform sentinels Go's os package guarantees on every
// host; the errno checks are best-effort and fall back to :unknown on hosts
// (notably Windows) where the underlying error does not carry a matching
// POSIX errno — a deliberate, documented gap rather than a fabricated match.
func classifyIOReason(err error) (reason string, phrase string) {
	switch {
	// ENOTDIR/ENOTEMPTY/ELOOP first: the stdlib's own syscall.Errno.Is maps
	// ENOTEMPTY to fs.ErrExist too (removing requires the target be both
	// "exists" and "empty"), so checking fs.ErrExist first would swallow
	// directory-not-empty into :already-exists. Most-specific-first avoids
	// that, verified against os.Remove on a populated directory.
	case errors.Is(err, syscall.ENOTDIR):
		return IOReasonNotADirectory, "not a directory"
	case errors.Is(err, syscall.ENOTEMPTY), isDirNotEmpty(err):
		return IOReasonDirectoryNotEmpty, "directory is not empty"
	case errors.Is(err, syscall.ELOOP):
		return IOReasonLoop, "too many levels of symbolic links"
	case errors.Is(err, fs.ErrNotExist):
		return IOReasonNotFound, "no such file or directory"
	case errors.Is(err, fs.ErrPermission):
		return IOReasonPermissionDenied, "permission denied"
	case errors.Is(err, fs.ErrExist):
		return IOReasonAlreadyExists, "already exists"
	default:
		return IOReasonUnknown, err.Error()
	}
}

// NewIOError builds the ex-info a cljg.io/cljg.process raise site should
// throw for a failed host operation (ADR 0114). op is the PUBLIC symbol the
// user called (e.g. "cljg.io/delete!"); opKind is a namespaced keyword
// identifying the operation kind for ex-data (e.g. :fs/delete); path is the
// path/argument the operation acted on (empty string omits :path); err is
// the underlying Go error, preserved verbatim under :go/error.
func NewIOError(op string, opKind Keyword, path string, err error) *ExceptionInfo {
	reason, phrase := classifyIOReason(err)
	kvs := []any{
		NewKeyword("op"), opKind,
		NewKeyword("reason"), NewKeyword(reason),
		NewKeyword("go/error"), err.Error(),
	}
	if path != "" {
		kvs = append(kvs, NewKeyword("path"), path)
	}
	return NewExceptionInfo(op+": "+phrase, NewMap(kvs...))
}
