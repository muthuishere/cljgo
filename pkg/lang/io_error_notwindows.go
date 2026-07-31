//go:build !windows

package lang

// isDirNotEmpty is the platform half of the directory-not-empty check. On
// POSIX, ENOTEMPTY already carries it and classifyIOReason tests that
// directly, so there is nothing extra to recognise here.
func isDirNotEmpty(error) bool { return false }
