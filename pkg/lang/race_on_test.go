//go:build race

package lang

// raceEnabled reports that this test binary was built with -race: perf
// budget measurements are meaningless under race instrumentation (its
// per-op overhead is asymmetric between raw channel ops and
// mutex-guarded wrapper ops), so budget tests skip themselves.
const raceEnabled = true
