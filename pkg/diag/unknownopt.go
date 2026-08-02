package diag

// The did-you-mean half of G5027 (ADR 0121). The raise site — pkg/bri's
// -check-opts — owns the WORDS and the registered code; this layer owns the
// Fix, exactly like applyJavaStatic above. That split is not stylistic: it
// keeps pkg/bri importing only pkg/lang. Computing the Fix at the raise site
// would mean pkg/bri importing pkg/diag, which imports pkg/reader, which
// would then link into every compiled bri binary for the sake of one
// suggestion string.

import (
	"regexp"
	"sort"
	"strings"
)

// unknownOptRe pulls the offending keys and the known set back out of the
// G5027 message, which carries both inline:
//
//	cljg.net.http/request: unknown option :time-out (known: :as :body … :url)
//	cljg.io/exec: unknown options :dirr, :tmout (known: :dir :env :in :timeout-ms)
var unknownOptRe = regexp.MustCompile(`unknown options? ([^()]+) \(known: ([^)]*)\)`)

// applyUnknownOpt attaches one did-you-mean Fix per unknown option, when a
// known key is within edit distance 2 of it. A no-op for every other code, so
// the regexp runs once per G5027 and never on the hot path of ordinary errors.
//
// No near match means NO Fix: the message already names the key and lists the
// whole known set, and a wrong suggestion is worse than none (the rule
// javaStaticFix follows for statics with no clojure.core twin).
func applyUnknownOpt(d *Diagnostic) {
	if d.ErrorCode != "G5027" {
		return
	}
	m := unknownOptRe.FindStringSubmatch(d.Message)
	if m == nil {
		return
	}
	known := strings.Fields(m[2])
	for _, bad := range strings.Split(m[1], ", ") {
		bad = strings.TrimSpace(bad)
		if bad == "" {
			continue
		}
		if best := nearestOpt(bad, known); best != "" {
			d.Fixes = append(d.Fixes, Fix{
				Title:       "did you mean " + best + "?",
				Replacement: best,
			})
		}
	}
}

// nearestOpt returns the known key closest to bad within edit distance 2, or
// "" when nothing is that close. Ties break on the lexicographically smaller
// name so the suggestion is deterministic.
func nearestOpt(bad string, known []string) string {
	const maxDist = 2
	best, bestDist := "", maxDist+1
	sorted := append([]string(nil), known...)
	sort.Strings(sorted)
	for _, k := range sorted {
		if dp := optEditDistance(bad, k, maxDist); dp < bestDist {
			best, bestDist = k, dp
		}
	}
	return best
}

// optEditDistance is the Levenshtein distance between a and b, abandoned
// (returning max+1) as soon as it must exceed max. A copy of the REPL's
// editDistance (pkg/repl/ergonomics.go) rather than a shared dependency:
// pkg/repl is on the interpreter side of the AOT fence and pkg/diag must not
// import it.
func optEditDistance(a, b string, max int) int {
	ra, rb := []rune(a), []rune(b)
	if diff := len(ra) - len(rb); diff > max || -diff > max {
		return max + 1
	}
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		rowMin := cur[0]
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			m := prev[j-1] + cost
			if v := prev[j] + 1; v < m {
				m = v
			}
			if v := cur[j-1] + 1; v < m {
				m = v
			}
			cur[j] = m
			if m < rowMin {
				rowMin = m
			}
		}
		if rowMin > max {
			return max + 1
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
