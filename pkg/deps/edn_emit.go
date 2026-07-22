package deps

// A deterministic, sorted-key EDN emitter for build.lock.edn.
//
// This is the ONLY hand-written EDN code in the package: reading EDN (the lock
// and dependency manifests) goes through cljgo's own pkg/reader (see
// edn_read.go), per ADR 0048's mandate not to hand-roll a second parser. The
// emitter exists so two machines resolving the same graph produce byte-identical
// lockfiles — maps emit with keys sorted by printed form, vectors in order.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// kw is an EDN keyword name in the emitter model (e.g. "git/sha" -> :git/sha).
type kw string

// ednVal is any value the emitter accepts: nil, bool, string, int, kw,
// []ednVal, or map[kw]ednVal.
type ednVal any

// emitEDN renders v deterministically. Map keys are sorted by their printed
// form; that ordering is what makes the lockfile byte-comparable across
// machines.
func emitEDN(v ednVal, indent string) string {
	switch t := v.(type) {
	case nil:
		return "nil"
	case bool:
		return strconv.FormatBool(t)
	case kw:
		return ":" + string(t)
	case string:
		return strconv.Quote(t)
	case int:
		return strconv.Itoa(t)
	case []ednVal:
		if len(t) == 0 {
			return "[]"
		}
		inner := indent + " "
		parts := make([]string, len(t))
		for i, e := range t {
			parts[i] = emitEDN(e, inner)
		}
		return "[" + strings.Join(parts, "\n"+inner) + "]"
	case map[kw]ednVal:
		if len(t) == 0 {
			return "{}"
		}
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, string(k))
		}
		sort.Strings(keys)
		inner := indent + " "
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, ":"+k+" "+emitEDN(t[kw(k)], inner+strings.Repeat(" ", len(k)+2)))
		}
		return "{" + strings.Join(parts, "\n"+inner) + "}"
	}
	panic(fmt.Sprintf("emitEDN: unsupported %T", v))
}

func strVals(ss []string) []ednVal {
	out := make([]ednVal, 0, len(ss))
	for _, s := range ss {
		out = append(out, s)
	}
	return out
}
