package lang

// cljgo addition (ADR 0022, design/08 §5): a raw PersistentArrayMap
// constructor for the `array-map` builtin. NewMap/NewPersistentArrayMap
// promote to a hash map once len(keyVals) >= hashmapThreshold; `array-map`
// must NOT — construction always yields a *Map, however many pairs are
// given (a later `assoc` past the threshold still promotes normally via
// Map.Assoc, matching the JVM: (class (array-map ...)) is always
// PersistentArrayMap, but (class (assoc big-array-map k v)) becomes
// PersistentHashMap once past the threshold).
//
// Not vendored from Glojure — new file, no surgery to log.

// NewArrayMapForce builds a PersistentArrayMap from keyVals, never
// promoting to a hash map regardless of size. Duplicate keys keep the
// first key's position with the last value, matching real Clojure:
// (array-map :a 1 :b 2 :a 3) => {:a 3, :b 2}.
func NewArrayMapForce(keyVals ...any) IPersistentMap {
	if len(keyVals) == 0 {
		return emptyMap
	}
	if len(keyVals)%2 != 0 {
		panic("invalid map. must have even number of inputs")
	}

	kv := make([]any, 0, len(keyVals))
	for i := 0; i < len(keyVals); i += 2 {
		k, v := keyVals[i], keyVals[i+1]
		found := false
		for j := 0; j < len(kv); j += 2 {
			if Equiv(kv[j], k) {
				kv[j+1] = v
				found = true
				break
			}
		}
		if !found {
			kv = append(kv, k, v)
		}
	}
	return &Map{keyVals: kv}
}
