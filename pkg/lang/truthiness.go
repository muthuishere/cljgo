package lang

import "reflect"

// IsTruthy returns true if the value is truthy.
func IsTruthy(v interface{}) bool {
	switch v := v.(type) {
	case nil:
		return false
	case bool:
		return v
	default:
		return !IsNil(v)
	}
}

func IsNil(v interface{}) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	// A typed nil (e.g. a nil func()/chan/map/slice boxed in the `any`
	// param) is != nil at the interface level — the classic Go "typed nil"
	// trap — so every reflect.Kind that supports IsNil() must be checked,
	// not just Ptr. Concretely hit by LazySeq.IsRealized (design/08):
	// s.fn is a `func() interface{}`, and (IsNil s.fn) only ever checked
	// the Ptr case, so it returned false even after realize() set
	// s.fn = nil — (realized? lazy-seq) was permanently false post-force.
	switch rv.Kind() {
	case reflect.Ptr, reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Slice, reflect.UnsafePointer:
		return rv.IsNil()
	}
	return false
}
