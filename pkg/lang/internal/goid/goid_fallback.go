//go:build !((amd64 || arm64) && go1.26 && !go1.27)

package goid

// Get returns the current goroutine's ID via the runtime.Stack()
// text-parse — the portable fallback for arches or toolchains the fast
// path (goid_fast.go) has not been vetted on. Slow but always correct.
func Get() int64 {
	return getSlow()
}
