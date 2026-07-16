package s19

// Q2 candidate 1: dynamic alts! on reflect.Select — the S10 mechanism,
// re-stated minimally here so both candidates run in ONE benchmark file
// on the same shapes. Works over GoBacked (its raw chan) and over ANY
// foreign Go `chan T` (the interop constraint).

import "reflect"

// AltsR: ports are *GoBacked (read), [2]any{*GoBacked, val} (write), or a
// raw Go channel value of any element type (read) — the interop case.
func AltsR(ports []any, hasDefault bool, defVal any) (any, any, bool) {
	cases := make([]reflect.SelectCase, 0, len(ports)+1)
	ids := make([]any, 0, len(ports))
	for _, p := range ports {
		switch port := p.(type) {
		case *GoBacked:
			cases = append(cases, reflect.SelectCase{
				Dir: reflect.SelectRecv, Chan: reflect.ValueOf(port.ch)})
			ids = append(ids, port)
		case [2]any:
			ch := port[0].(*GoBacked)
			cases = append(cases, reflect.SelectCase{
				Dir: reflect.SelectSend, Chan: reflect.ValueOf(ch.ch),
				Send: reflect.ValueOf(port[1])})
			ids = append(ids, ch)
		default:
			// foreign Go chan T from interop, read side
			cases = append(cases, reflect.SelectCase{
				Dir: reflect.SelectRecv, Chan: reflect.ValueOf(p)})
			ids = append(ids, p)
		}
	}
	if hasDefault {
		cases = append(cases, reflect.SelectCase{Dir: reflect.SelectDefault})
	}
	chosen, recv, recvOK := reflect.Select(cases)
	if hasDefault && chosen == len(ports) {
		return defVal, "default", false
	}
	if cases[chosen].Dir == reflect.SelectSend {
		return true, ids[chosen], true
	}
	if !recvOK {
		return nil, ids[chosen], false
	}
	return recv.Interface(), ids[chosen], true
}
