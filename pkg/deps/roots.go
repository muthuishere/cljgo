package deps

import "sync"

// A process-scoped resolved-roots handle. The build/run bootstrap resolves
// dependencies once and calls SetResolvedRoots; ResolveLibPath (integration
// wiring, outside this package) reads them via ResolvedRoots for load-path slot
// 3 (ADR 0048 decision 2). It is a single handle set once at bootstrap — both
// execution legs read the same handle, so interpreter and emitter cannot
// diverge (the dual-mode parity guarantee).
var (
	resolvedRootsMu sync.RWMutex
	resolvedRoots   []string
)

// SetResolvedRoots publishes the resolved dependency roots, in lock order.
func SetResolvedRoots(roots []string) {
	resolvedRootsMu.Lock()
	defer resolvedRootsMu.Unlock()
	resolvedRoots = append([]string(nil), roots...)
}

// ResolvedRoots returns the resolved dependency roots set at bootstrap.
func ResolvedRoots() []string {
	resolvedRootsMu.RLock()
	defer resolvedRootsMu.RUnlock()
	return append([]string(nil), resolvedRoots...)
}
