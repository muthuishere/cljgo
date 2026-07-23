package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Map is the canonical nested representation every layer normalizes into.
// Keys are keyword *names* (lowercase, no leading colon) so `.edn`,
// `.properties`, and `APP_*` env all land in one shape. Mirrors the plain
// Clojure map bri.config already produces (kebab-case keyword keys).
type Map = map[string]any

// Path is a resolved key location, e.g. []string{"db", "pool-size"} — the
// Go stand-in for the [:db :pool-size] vector the .cljg API takes.
type Path = []string

// deepMerge overlays b onto a (b wins), recursing into nested maps. Non-map
// values replace wholesale. Nil b is a no-op (keeps a). This is the same
// deep-merge bri.config.cljg uses, transcribed to Go.
func deepMerge(a, b Map) Map {
	if a == nil {
		a = Map{}
	}
	out := cloneMap(a)
	for k, bv := range b {
		if bm, ok := bv.(Map); ok {
			if am, ok := out[k].(Map); ok {
				out[k] = deepMerge(am, bm)
				continue
			}
		}
		out[k] = bv
	}
	return out
}

func cloneMap(m Map) Map {
	out := make(Map, len(m))
	for k, v := range m {
		if vm, ok := v.(Map); ok {
			out[k] = cloneMap(vm)
		} else {
			out[k] = v
		}
	}
	return out
}

// leafPaths returns every scalar leaf's path, sorted for deterministic dumps.
func leafPaths(m Map) []Path {
	var out []Path
	var walk func(prefix Path, m Map)
	walk = func(prefix Path, m Map) {
		for k, v := range m {
			p := append(append(Path{}, prefix...), k)
			if vm, ok := v.(Map); ok {
				walk(p, vm)
			} else {
				out = append(out, p)
			}
		}
	}
	walk(nil, m)
	sort.Slice(out, func(i, j int) bool {
		return strings.Join(out[i], ".") < strings.Join(out[j], ".")
	})
	return out
}

// getIn / setIn are the get-in / assoc-in analogues.
func getIn(m Map, path Path) (any, bool) {
	cur := any(m)
	for _, seg := range path {
		cm, ok := cur.(Map)
		if !ok {
			return nil, false
		}
		v, ok := cm[seg]
		if !ok {
			return nil, false
		}
		cur = v
	}
	return cur, true
}

func setIn(m Map, path Path, val any) Map {
	if len(path) == 0 {
		return m
	}
	if m == nil {
		m = Map{}
	}
	out := cloneMap(m)
	if len(path) == 1 {
		out[path[0]] = val
		return out
	}
	child, _ := out[path[0]].(Map)
	out[path[0]] = setIn(child, path[1:], val)
	return out
}

// coerceScalar turns a stringly value (from env or .properties) into a typed
// scalar: bool, int64, float64, else the trimmed string. Durations/sizes are
// expressed as NUMBERS (seconds, bytes) upstream — never "5m" strings — so a
// bare integer is the expected shape here (criterion 5).
func coerceScalar(s string) any {
	t := strings.TrimSpace(s)
	switch t {
	case "true":
		return true
	case "false":
		return false
	}
	if i, err := strconv.ParseInt(t, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(t, 64); err == nil {
		return f
	}
	return t
}

// prettyScalar renders a resolved value the way pr-str would (numbers bare,
// strings quoted), for the provenance dump.
func prettyScalar(v any) string {
	switch x := v.(type) {
	case string:
		return strconv.Quote(x)
	case bool:
		return strconv.FormatBool(x)
	default:
		return fmt.Sprintf("%v", x)
	}
}
