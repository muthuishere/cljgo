// REPL ergonomics (ADR 0018): graceful exit words, help, and
// did-you-mean suggestions. All affordances are fallback-only — a
// user-defined var always wins (the precedence principle) — and the
// exit/help words additionally gate on interactive use (d.Prompts), so
// piped scripts see exactly the historical semantics.
package repl

import (
	"fmt"
	"sort"
	"strings"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// affordanceWord returns "exit", "quit" or "help" when form is that
// bare symbol or the zero-arg call form ((exit), (quit), (help)) AND
// the symbol does not resolve in the current namespace. A user-defined
// exit/quit/help var (or any other mapping) always wins.
func (d *Driver) affordanceWord(form any) (string, bool) {
	sym, ok := form.(*lang.Symbol)
	if !ok {
		// (exit) — a one-element list whose head is a bare symbol.
		seq, isSeq := form.(lang.ISeq)
		if !isSeq || lang.Seq(seq) == nil || seq.Next() != nil {
			return "", false
		}
		sym, ok = seq.First().(*lang.Symbol)
		if !ok {
			return "", false
		}
	}
	if sym.HasNamespace() {
		return "", false
	}
	name := sym.Name()
	if name != "exit" && name != "quit" && name != "help" {
		return "", false
	}
	if d.ev.CurrentNS().GetMapping(sym) != nil {
		return "", false // resolvable: the user's definition wins
	}
	return name, true
}

// farewell prints the friendly session ending (ADR 0018 §1).
func (d *Driver) farewell() {
	d.outMu.Lock()
	defer d.outMu.Unlock()
	if d.journalFile != nil {
		fmt.Fprintf(d.out, "Goodbye! Resume this session with :resume %s\n", d.sessionID)
		return
	}
	fmt.Fprintln(d.out, "Goodbye!")
}

// printHelp prints the REPL affordances (ADR 0018 §2).
func (d *Driver) printHelp() {
	d.outMu.Lock()
	defer d.outMu.Unlock()
	fmt.Fprint(d.out, `REPL affordances:
  exit, quit     end the session ((exit)/(quit) work too; so does Ctrl-D)
  help           this message
  *1 *2 *3       the last three results
  *e             the last error
  Ctrl-C         discard the input in progress (never exits the session)
  :sessions      list saved sessions
  :resume <id>   replay a saved session, then continue journaling to it
`)
	if d.journalOn {
		fmt.Fprintf(d.out, "  session %s (journal: %s)\n", d.sessionID, d.journalPath(d.sessionID))
	} else {
		fmt.Fprintln(d.out, "  session journaling is off (interactive tty or CLJGO_SESSION=1 turns it on)")
	}
}

// reportEvalError reports an eval error and, for unresolved-symbol
// errors, appends a did-you-mean line with the nearest candidates from
// the current namespace's mappings (ADR 0018 §3).
func (d *Driver) reportEvalError(err error) {
	d.reportError(err)
	name, ok := unresolvedName(err)
	if !ok {
		return
	}
	cands := d.nearbySymbols(name)
	if len(cands) == 0 {
		return
	}
	d.outMu.Lock()
	fmt.Fprintf(d.errOut, "did you mean %s?\n", strings.Join(cands, ", "))
	d.outMu.Unlock()
}

// unresolvedName extracts the symbol name from an "unable to resolve
// symbol: X in this context" error (which may carry a position prefix).
func unresolvedName(err error) (string, bool) {
	const pre = "unable to resolve symbol: "
	const suf = " in this context"
	msg := err.Error()
	i := strings.Index(msg, pre)
	if i < 0 {
		return "", false
	}
	msg = msg[i+len(pre):]
	j := strings.Index(msg, suf)
	if j <= 0 {
		return "", false
	}
	name := msg[:j]
	if strings.ContainsAny(name, " \n") {
		return "", false
	}
	return name, true
}

// nearbySymbols scans the current namespace's mappings (interned AND
// referred vars — refer targets live in the same map) for names within
// edit distance 2 of name, returning at most 3, nearest first (ties
// alphabetical, for determinism).
func (d *Driver) nearbySymbols(name string) []string {
	type cand struct {
		name string
		dist int
	}
	var cands []cand
	for s := lang.Seq(d.ev.CurrentNS().Mappings()); s != nil; s = s.Next() {
		entry, ok := s.First().(lang.IMapEntry)
		if !ok {
			continue
		}
		sym, ok := entry.Key().(*lang.Symbol)
		if !ok {
			continue
		}
		if _, ok := entry.Val().(*lang.Var); !ok {
			continue // class imports etc. are not eval suggestions
		}
		n := sym.Name()
		if n == name {
			continue
		}
		if dist := editDistance(name, n, 2); dist <= 2 {
			cands = append(cands, cand{n, dist})
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].dist != cands[j].dist {
			return cands[i].dist < cands[j].dist
		}
		return cands[i].name < cands[j].name
	})
	if len(cands) > 3 {
		cands = cands[:3]
	}
	out := make([]string, len(cands))
	for i, c := range cands {
		out[i] = c.name
	}
	return out
}

// editDistance is the Levenshtein distance between a and b, cut short
// (returning max+1) as soon as it must exceed max.
func editDistance(a, b string, max int) int {
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
			m := prev[j-1] + cost // substitute
			if v := prev[j] + 1; v < m {
				m = v // delete
			}
			if v := cur[j-1] + 1; v < m {
				m = v // insert
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
