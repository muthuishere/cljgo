package eval

import (
	"strings"
	"testing"
)

// TestFormatGoFlagsNeverLeaksUnfiltered is the invariant VERDICT.md calls
// out explicitly for translate-then-delegate: no flag/verb combination may
// reach fmt.Sprintf unless it is in formatGoFlags' per-verb allow-list —
// otherwise Go's fmt silently emits "%!d(BADFLAG)"-style noise into real
// output instead of failing loudly. This is a whitebox test on the
// unexported allow-list itself (not just corpus-driven), per ADR 0030's
// stated discipline requirement.
func TestFormatGoFlagsNeverLeaksUnfiltered(t *testing.T) {
	// ',' and '(' have NO Go fmt equivalent for any verb — formatBuildVerb
	// must never pass them through, for every verb we delegate to fmt.Sprintf.
	for _, conv := range []string{"d", "x", "o", "s", "c", "f", "e"} {
		allowed := formatGoFlags[conv]
		for _, banned := range []byte{',', '('} {
			if strings.IndexByte(allowed, banned) >= 0 {
				t.Fatalf("formatGoFlags[%q] allows %q, which Go's fmt does not support — must be hand-rendered, never delegated", conv, banned)
			}
		}
	}
	// buildVerb itself must filter: an input flags string containing a
	// banned char must not appear in the built verb.
	d := formatDirective{flags: "-+0, (", conv: 'd'}
	verb := formatBuildVerb(d, "d")
	for _, banned := range []byte{',', '('} {
		if strings.IndexByte(verb, banned) >= 0 {
			t.Fatalf("formatBuildVerb leaked banned flag %q into verb %q", banned, verb)
		}
	}
}
