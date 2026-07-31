package eval_test

// clojure.core parity with the JVM, as a RATCHET.
//
// WHY THIS EXISTS, and it is not tidiness. cljgo's pitch is that a Clojure
// developer brings their code and their habits across unchanged. Every name in
// cljgo's `clojure.core` that JVM Clojure does not have is a small tax on that
// promise, and issue #171 is the proof the tax is already being paid: a user
// who legally writes `(defn ok [x] …)` gets a shadow warning that could not
// happen on the JVM. Every JVM name cljgo is MISSING is the same tax from the
// other direction — code that compiles there and not here.
//
// So core parity is an ADOPTION property, and adoption properties that nobody
// measures drift. This test measures both directions against the real thing.
//
// THE ORACLE is testdata/jvm-clojure-core-publics.txt: the 679 public vars of
// clojure.core on the JVM, captured from the real `clojure` CLI at 1.12.5
// (2026-07-31) with
//
//	(sort (map str (keys (ns-publics 'clojure.core))))
//
// Regenerate it the same way when the pinned Clojure version moves, and say so
// in the commit — a silently-refreshed oracle proves nothing.
//
// HOW IT RATCHETS. Two frozen lists record today's divergence:
//
//	testdata/core-parity-extra.txt    names cljgo has and the JVM does not
//	testdata/core-parity-missing.txt  names the JVM has and cljgo does not
//
// A NEW divergence in either direction FAILS. Closing one is reported, with
// instructions, and the frozen file is then updated to lock the improvement in.
// The lists may only ever shrink, so parity cannot quietly erode while
// everyone is looking somewhere else.
//
// These are DEBT, not a specification. Nothing in either file is endorsed by
// being there.

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/eval"
	"github.com/muthuishere/cljgo/pkg/lang"
)

func readNameSet(t *testing.T, name string) map[string]bool {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	defer f.Close()
	out := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" && !strings.HasPrefix(line, ";") {
			out[line] = true
		}
	}
	return out
}

// cljgoCorePublics is the set of PUBLIC var names interned in cljgo's
// clojure.core, read straight from the namespace after a real boot — the same
// thing (ns-publics 'clojure.core) reports.
func cljgoCorePublics(t *testing.T) map[string]bool {
	t.Helper()
	eval.New() // boots builtins + defmacro + core.clj
	ns := lang.FindNamespace(lang.NewSymbol("clojure.core"))
	if ns == nil {
		t.Fatal("clojure.core does not exist after boot")
	}
	out := map[string]bool{}
	for s := lang.Seq(ns.Mappings()); s != nil; s = lang.Next(s) {
		entry := lang.First(s)
		sym, ok := lang.Get(entry, int64(0)).(*lang.Symbol)
		if !ok {
			continue
		}
		v, ok := lang.Get(entry, int64(1)).(*lang.Var)
		if !ok {
			continue // a referred class or alias, not an interned var
		}
		// Only vars this namespace OWNS, and only public ones — a referred
		// var from elsewhere is not part of core's surface.
		if !ns.OwnsInternedVar(sym, v) || !v.IsPublic() {
			continue
		}
		out[sym.Name()] = true
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestCoreHasNoNewNamesTheJVMLacks — the pollution direction. A cljgo-only
// name in clojure.core makes legal user code warn (or break) here and not on
// the JVM. New ones are refused; the frozen list is existing debt.
func TestCoreHasNoNewNamesTheJVMLacks(t *testing.T) {
	jvm := readNameSet(t, "jvm-clojure-core-publics.txt")
	known := readNameSet(t, "core-parity-extra.txt")
	ours := cljgoCorePublics(t)

	var added, fixed []string
	for _, name := range sortedKeys(ours) {
		if !jvm[name] && !known[name] {
			added = append(added, name)
		}
	}
	for _, name := range sortedKeys(known) {
		if !ours[name] {
			fixed = append(fixed, name)
		}
	}

	t.Logf("clojure.core: cljgo %d publics, JVM %d, recorded extras %d",
		len(ours), len(jvm), len(known))

	if len(added) > 0 {
		t.Errorf("%d NEW name(s) in cljgo's clojure.core that JVM Clojure 1.12.5 does not have:\n  %s\n\n"+
			"Every one of these makes legal user code behave differently here than on the JVM —\n"+
			"a user who defines this name gets a shadow warning that cannot happen on the JVM.\n"+
			"Put the addition in its own namespace instead (the precedence principle: when a new\n"+
			"feature's name collides with Clojure, the NEW feature moves). If it genuinely belongs\n"+
			"in core, add it to testdata/core-parity-extra.txt with a reason in the commit message.",
			len(added), strings.Join(added, "\n  "))
	}
	if len(fixed) > 0 {
		t.Errorf("%d recorded extra(s) are no longer in clojure.core — parity improved:\n  %s\n\n"+
			"Remove them from testdata/core-parity-extra.txt so the improvement is locked in\n"+
			"and cannot silently regress.", len(fixed), strings.Join(fixed, "\n  "))
	}
}

// TestCoreDoesNotLoseJVMNames — the coverage direction. A name cljgo lacks is
// code that compiles on the JVM and not here, which is the same adoption tax
// pointing the other way.
func TestCoreDoesNotLoseJVMNames(t *testing.T) {
	jvm := readNameSet(t, "jvm-clojure-core-publics.txt")
	known := readNameSet(t, "core-parity-missing.txt")
	ours := cljgoCorePublics(t)

	var lost, gained []string
	for _, name := range sortedKeys(jvm) {
		if !ours[name] && !known[name] {
			lost = append(lost, name)
		}
	}
	for _, name := range sortedKeys(known) {
		if ours[name] {
			gained = append(gained, name)
		}
	}

	t.Logf("clojure.core: %d JVM names recorded as missing", len(known))

	if len(lost) > 0 {
		t.Errorf("%d JVM clojure.core name(s) newly absent from cljgo:\n  %s\n\n"+
			"Code that compiles on the JVM will not compile here. Either implement it, or record\n"+
			"it in testdata/core-parity-missing.txt with the reason in the commit message.",
			len(lost), strings.Join(lost, "\n  "))
	}
	if len(gained) > 0 {
		t.Errorf("%d recorded-missing name(s) now exist — parity improved:\n  %s\n\n"+
			"Remove them from testdata/core-parity-missing.txt to lock the improvement in.",
			len(gained), strings.Join(gained, "\n  "))
	}
}

// TestCoreHasNoLowercaseAliasOfAJVMName — a rename in disguise. cljgo ships
// both `NaN?` (the JVM's spelling) and `nan?`; an alias that differs only in
// case reads as a helpful convenience and is exactly what the precedence
// principle forbids, because it teaches a spelling that does not port.
//
// This is a NARROW check on purpose: same name, different case, both present.
func TestCoreHasNoLowercaseAliasOfAJVMName(t *testing.T) {
	jvm := readNameSet(t, "jvm-clojure-core-publics.txt")
	ours := cljgoCorePublics(t)

	lowerJVM := map[string]string{} // lowercased -> the JVM's own spelling
	for name := range jvm {
		lowerJVM[strings.ToLower(name)] = name
	}

	var aliases []string
	for _, name := range sortedKeys(ours) {
		if jvm[name] {
			continue // it IS the JVM name
		}
		if canonical, ok := lowerJVM[strings.ToLower(name)]; ok && ours[canonical] {
			aliases = append(aliases, name+" (the JVM spells it "+canonical+")")
		}
	}
	if len(aliases) > 0 {
		t.Errorf("%d case-variant alias(es) of a JVM clojure.core name:\n  %s\n\n"+
			"Both spellings work here and only one works on the JVM, so this teaches a spelling\n"+
			"that does not port. Drop the alias and keep the JVM's spelling.",
			len(aliases), strings.Join(aliases, "\n  "))
	}
}
