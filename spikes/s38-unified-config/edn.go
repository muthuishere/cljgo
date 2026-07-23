package main

import (
	"fmt"
	"os"

	"olympos.io/encoding/edn"
)

// readEDN parses an EDN file into the canonical Map. Missing file => (nil,nil):
// a layer that isn't present contributes nothing, exactly like bri.config's
// -read-file returning nil. Keyword keys become their kebab-case name string.
//
// EDN is the canonical Clojure surface (ADR: Clojure-native). This spike keeps
// it canonical; .properties is a convenience skin that normalizes into the
// identical shape (see properties.go).
func readEDN(path string) (Map, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var raw any
	if err := edn.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	norm, ok := normalizeEDN(raw).(Map)
	if !ok {
		return nil, fmt.Errorf("%s: top-level EDN must be a map, got %T", path, raw)
	}
	return norm, nil
}

// normalizeEDN converts go-edn's decoded tree into canonical values: keyword
// keys -> name strings, nested maps -> Map, integers -> int64. Non-keyword map
// keys are rejected (config keys are keywords in Clojure).
func normalizeEDN(v any) any {
	switch x := v.(type) {
	case map[any]any:
		out := make(Map, len(x))
		for k, val := range x {
			kw, ok := k.(edn.Keyword)
			if !ok {
				panic(fmt.Sprintf("config keys must be keywords, got %T (%v)", k, k))
			}
			out[string(kw)] = normalizeEDN(val)
		}
		return out
	case int:
		return int64(x)
	default:
		return v
	}
}
