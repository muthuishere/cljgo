package lang

// cljgo S4 surgery: rewritten on the stdlib testing package only; the
// original used github.com/stretchr/testify/assert (an external dep we
// dropped). Assertions are 1:1.
import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestPersistentHashMap(t *testing.T) {
	var m Associative = NewPersistentHashMap()
	m = m.Assoc(nil, 1)
	if got := m.ValAt(nil); got != 1 {
		t.Errorf("ValAt(nil) = %v; want 1", got)
	}

	if m.Seq() == nil {
		t.Error("Seq() = nil; want non-nil")
	}

	m = NewPersistentHashMap()
	for i := 0; i < 1000; i++ {
		m = m.Assoc(i, i)
	}
	for i := 0; i < 1000; i++ {
		if got := m.ValAt(i); got != i {
			t.Errorf("ValAt(%d) = %v; want %d", i, got, i)
		}
	}
}

func FuzzPersistentHashMap(f *testing.F) {
	f.Add([]byte(`[
      42,
      "a",
      ["symbol", "foo"],
      ["symbol", "foo/bar"],
			["symbol", "fn"]
    ]`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var jsVals []interface{}
		if err := json.Unmarshal(data, &jsVals); err != nil {
			t.Skip()
		}
		var vals []any
		for _, jsVal := range jsVals {
			v, err := jsonValToVal(jsVal)
			if err != nil {
				t.Errorf("jsonValToVal(%v) error: %v", jsVal, err)
			}
			vals = append(vals, v)
		}

		var m Associative = NewPersistentHashMap()

		for _, val := range vals {
			m = m.Assoc(val, val)
		}

		// Test that all values are present.

		for _, val := range vals {
			if got := m.ValAt(val); !Equals(got, val) {
				t.Errorf("%v (%T) not in %v (got %v)", val, val, m, got)
			}
		}
	})
}

func jsonValToVal(v any) (res any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	switch v := v.(type) {
	case float64:
		if v == float64(int64(v)) {
			return int64(v), nil
		}
		return v, nil
	case string:
		return v, nil
	case []interface{}:
		if len(v) == 0 {
			return NewList(), nil
		}
		if v[0] == "symbol" && len(v) <= 3 {
			strs := make([]string, len(v)-1)
			for i := 1; i < len(v); i++ {
				strs[i-1] = v[i].(string)
			}
			return NewSymbol(strings.Join(strs, "/")), nil
		}
		switch v[0] {
		case "list":
			return NewList(mapJSONVals(v[1:])...), nil
		case "vector":
			return NewVector(mapJSONVals(v[1:])...), nil
		case "map":
			if len(v)%2 != 1 {
				return nil, fmt.Errorf("map must have odd number of elements")
			}
			return NewMap(mapJSONVals(v[1:])...), nil
		}
		return NewList(mapJSONVals(v)...), nil
	}
	panic(fmt.Errorf("unknown type %T", v))
}

func mapJSONVals(arr []any) []any {
	var els []any
	for _, el := range arr {
		el, err := jsonValToVal(el)
		if err != nil {
			panic(err)
		}
		els = append(els, el)
	}
	return els
}
