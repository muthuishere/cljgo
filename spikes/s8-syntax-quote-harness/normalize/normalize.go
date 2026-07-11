// Package normalize makes gensym-bearing reader output comparable across runs.
//
// Clojure's auto-gensym mints names from a global monotonic counter
// (x__153__auto__, y__154__auto__, ...), and `#()` arg hygiene mints p1__449#.
// The numbers differ on every JVM run and will differ again in our Go reader,
// so raw string comparison is meaningless. Gensyms renumbers the numeric ids
// within ONE case's output by order of first appearance (1, 2, 3, ...):
//
//	(quote x__153__auto__) (quote x__153__auto__)  ->  __1__ __1__   (same gensym stays same)
//	[(quote x__158__auto__) (quote x__159__auto__)] -> __1__ __2__   (distinct stays distinct)
//
// Identical ids map to identical replacements and distinct ids stay distinct,
// so hygiene properties survive normalization. Apply it to BOTH the golden
// output and the candidate reader's output before comparing.
//
// This package is the piece pkg/reader's CI will import (or vendor verbatim).
package normalize

import (
	"regexp"
	"strconv"
)

// gensymRe matches the numeric id of Clojure-style generated symbols:
//
//	name__NNN__auto__   (syntax-quote auto-gensym `x#`)
//	pN__NNN#            (#() arg hygiene, e.g. p1__449#)
//
// Group 1 is the id digits; group 2 is the trailing marker.
var gensymRe = regexp.MustCompile(`__([0-9]+)(__auto__|#)`)

// Gensyms rewrites every gensym id in s to a stable id numbered by order of
// first appearance, starting at 1. Idempotent: normalizing normalized output
// is a no-op. Call it once per test case — the numbering is scoped to s.
func Gensyms(s string) string {
	seen := map[string]int{}
	next := 1
	return gensymRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := gensymRe.FindStringSubmatch(m)
		id, ok := seen[sub[1]]
		if !ok {
			id = next
			seen[sub[1]] = id
			next++
		}
		return "__" + strconv.Itoa(id) + sub[2]
	})
}
