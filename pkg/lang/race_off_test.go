//go:build !race

package lang

// raceEnabled is false in a normal (non -race) test binary.
const raceEnabled = false
