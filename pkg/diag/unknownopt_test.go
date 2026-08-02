package diag

import "testing"

// TestNearestOptSuggestsUnitSuffixedKeys pins the suggestion rule to the case
// the diagnostic exists for.
//
// ADR 0121 was written because the koine port passed `:timeout` to
// cljg.net.http, which reads `:timeout-ms` — the majority spelling across
// cljg.socket, cljg.io and cljg.process. That pair is edit distance 3, so a
// pure-Levenshtein suggestion scores nothing and the user is told their key
// is unknown without being told the one they meant. The whole value of the
// diagnostic is in that second half.
func TestNearestOptSuggestsUnitSuffixedKeys(t *testing.T) {
	httpKeys := []string{
		"method", "url", "headers", "query", "body", "json", "edn", "form",
		"timeout-ms", "timeout", "retry", "as",
	}
	execKeys := []string{"in", "env", "dir", "timeout-ms"}

	for _, tc := range []struct {
		name  string
		bad   string
		known []string
		want  string
	}{
		// The motivating case: a unit suffix is a guessable variation, not a
		// typo, and distance alone never catches it.
		{"unit suffix added", "timeout", execKeys, "timeout-ms"},
		// The boundary, pinned deliberately. A DIFFERENT unit spelling is
		// neither within distance 2 nor a prefix, so it gets no suggestion.
		// Widening the rule to reach it (longest-common-prefix scoring) buys
		// one speculative case and costs a heuristic nobody can predict; the
		// error still names every known key, which is the reliable half.
		{"a different unit word is out of reach", "timeout-millis", execKeys, ""},
		// Ordinary typos must still work, and must still WIN over a
		// coincidental prefix — `retry` is distance 1 from `retr`.
		{"typo beats prefix", "retr", httpKeys, "retry"},
		{"transposition", "haeders", httpKeys, "headers"},
		// Nothing plausible: silence beats a misleading suggestion.
		{"no suggestion", "completely-different", execKeys, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := nearestOpt(tc.bad, tc.known); got != tc.want {
				t.Errorf("nearestOpt(%q) = %q, want %q", tc.bad, got, tc.want)
			}
		})
	}
}
