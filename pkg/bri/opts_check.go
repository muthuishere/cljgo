// opts_check.go — the ONE unknown-option check every cljg.* opts map runs
// through (ADR 0121, closing the class ADR 0120 left open).
//
// The defect it removes: `(:timeout opts default)` means a MISSPELLED option
// has no symptom. The call succeeds, the caller's intent evaporates, and on
// fast hardware every test still passes — which is exactly how koine's dead
// HTTP timeout survived a green suite on both hosts (issue #192).
//
// One function, one shim, one code. `-check-opts` is interned as a :private
// var into EVERY bri/cljg namespace (bri.go InstallShimsInto), so a
// pure-Clojure namespace can call it without growing a Go half of its own,
// and both loaders (interpreter + AOT) get it from the same place. The
// allowed key set is written in the .cljg file next to the docstring that
// documents it — the two cannot drift apart across a file boundary.
//
// The check is deliberately ONE LEVEL DEEP: it validates the opts map itself
// and never descends into a value. Nested maps in a cljg.* opts map are DATA
// (:headers, :query, :form, :env) whose keys belong to the caller, and a
// recursive check would reject them.
package bri

import (
	"fmt"
	"sort"
	"strings"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// checkOptsShim is (-check-opts "ns/fn" opts [:a :b …]) — nil when every key
// in opts is known, a panicking G5027 otherwise.
func checkOptsShim(args ...any) any {
	if len(args) != 3 {
		panic(fmt.Errorf("-check-opts expects 3 args (fn-name opts allowed-keys), got %d", len(args)))
	}
	m, ok := args[1].(lang.IPersistentMap)
	if !ok || m == nil {
		// nil (no opts) or a non-map: not this check's business. A non-map
		// opts is already the callee's own error (cljg.io/write-bytes raises
		// G5017 for it); silently accepting one here would be a second,
		// worse-located diagnosis of the same mistake.
		return nil
	}
	checkOpts(asString(args[0]), m, args[2])
	return nil
}

// checkOpts panics with a coded G5027 error naming every key in opts that
// `allowed` does not list. The message is byte-stable and carries the known
// set inline, which is what lets the render layer (pkg/diag applyUnknownOpt)
// compute the did-you-mean Fix without pkg/bri importing pkg/diag — an import
// that would drag pkg/reader into every compiled bri binary for the sake of
// one error string.
func checkOpts(fnName string, m lang.IPersistentMap, allowed any) {
	known := make([]string, 0, 12)
	for s := lang.Seq(allowed); s != nil; s = lang.Next(s) {
		known = append(known, optToken(lang.First(s)))
	}

	var unknown []string
	for s := lang.Seq(m); s != nil; s = lang.Next(s) {
		k := optToken(lang.First(lang.First(s)))
		found := false
		for _, a := range known {
			if a == k {
				found = true
				break
			}
		}
		if !found {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 0 {
		return
	}
	// Clojure map iteration order is not specified, so sort: the message a
	// user reads (and a conformance test freezes) must not depend on which
	// map implementation the caller happened to build.
	sort.Strings(unknown)
	word := "option"
	if len(unknown) > 1 {
		word = "options"
	}
	panic(lang.NewCodedError("G5027", fmt.Sprintf("%s: unknown %s %s (known: %s)",
		fnName, word, strings.Join(unknown, ", "), strings.Join(known, " "))))
}

// optToken renders a map key as the text the message shows and compares on.
// Keywords — every documented cljg.* option is one — print as `:name`.
func optToken(k any) string { return fmt.Sprintf("%v", k) }
