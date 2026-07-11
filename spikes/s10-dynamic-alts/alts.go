// Package alts — spike S10: dynamic alts! (runtime channel list) on
// reflect.Select, per design/00-architecture.md §4.7 and
// design/05-interop-concurrency.md §4.
//
// Contract (core.async parity, doc 05):
//   - read from closed channel  -> (nil, ch, false)   [nil normalization,
//     even for typed channels whose zero value isn't nil]
//   - write to closed channel   -> (false, ch, false) [recover shim, no panic]
//   - write success             -> (true, ch, true)
//   - :default taken only when no op is ready -> (defaultVal, DefaultPort, false)
//   - timeout composed as an extra recv case  -> (nil, TimeoutPort, false)
//   - nil puts rejected (panic, as core.async throws)
//   - fairness: RANDOM by default (reflect.Select is pseudo-random, like
//     select); :priority tries ops in order via a non-blocking pass first.
package alts

import (
	"reflect"
	"time"
)

// AltOp is one alts! port: a read (Chan only) or a write (Chan + Value).
// Chan is `any` because dynamic alts! receives channels as runtime values —
// possibly typed Go channels (chan int) obtained via interop, not just
// chan any. reflect handles element-type assignability for us.
type AltOp struct {
	Chan    any
	Value   any
	IsWrite bool
}

// AltOpts carries core.async's alts! options.
type AltOpts struct {
	HasDefault bool
	Default    any  // value returned when :default fires
	Priority   bool // :priority true — try ops in listed order
	HasTimeout bool // composed timeout (an extra recv case)
	Timeout    time.Duration
}

type sentinel string

// DefaultPort is the "port" returned when the :default clause fires
// (core.async returns the keyword :default; the compiler maps that
// keyword to this sentinel).
var DefaultPort any = sentinel("default")

// TimeoutPort is the "port" returned when the composed timeout fires.
var TimeoutPort any = sentinel("timeout")

// Alts performs one core.async alts! over a runtime list of ops.
// Returns (val, ch, ok):
//
//	read  ready   -> (v, ch, true)
//	read  closed  -> (nil, ch, false)
//	write ready   -> (true, ch, true)
//	write closed  -> (false, ch, false)
//	default taken -> (opts.Default, DefaultPort, false)
//	timeout fired -> (nil, TimeoutPort, false)
func Alts(ops []AltOp, opts AltOpts) (val any, ch any, ok bool) {
	if len(ops) == 0 && !opts.HasDefault && !opts.HasTimeout {
		panic("alts: no ops, no default, no timeout — would block forever")
	}

	cases := make([]reflect.SelectCase, len(ops))
	for i, op := range ops {
		cv := reflect.ValueOf(op.Chan)
		if op.IsWrite {
			if op.Value == nil {
				panic("alts: nil puts are not allowed") // core.async parity
			}
			cases[i] = reflect.SelectCase{
				Dir:  reflect.SelectSend,
				Chan: cv,
				Send: reflect.ValueOf(op.Value),
			}
		} else {
			cases[i] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: cv}
		}
	}

	// :priority — a non-blocking pass over ops in listed order. Only the
	// "more than one op ready" case is order-sensitive; if nothing is ready
	// we fall through to a normal blocking select (whichever wakes first
	// wins, same as core.async).
	if opts.Priority {
		for i := range cases {
			if v, c, k, taken := tryCase(cases[i], ops[i]); taken {
				return v, c, k
			}
		}
		if opts.HasDefault {
			return opts.Default, DefaultPort, false
		}
	}

	all := cases
	timeoutIdx, defaultIdx := -1, -1
	if opts.HasTimeout {
		timeoutIdx = len(all)
		all = append(all, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(time.After(opts.Timeout)),
		})
	}
	if opts.HasDefault && !opts.Priority {
		defaultIdx = len(all)
		all = append(all, reflect.SelectCase{Dir: reflect.SelectDefault})
	}

	chosen, recv, recvOK, panicked := doSelect(all)
	if panicked {
		// A send case hit a closed channel mid-select. reflect.Select does
		// not tell us which case panicked, so probe ops in listed order
		// non-blockingly: any successful op is a legitimate alts! outcome
		// (the panicked select completed nothing), and the closed send is
		// guaranteed to be "taken" (as val=false, ok=false) when reached.
		for i := range cases {
			if v, c, k, taken := tryCase(cases[i], ops[i]); taken {
				return v, c, k
			}
		}
		// Unreachable: the closed send always registers as taken above.
		panic("alts: unreachable — closed-send probe found nothing")
	}

	switch chosen {
	case timeoutIdx:
		return nil, TimeoutPort, false
	case defaultIdx:
		return opts.Default, DefaultPort, false
	}
	op := ops[chosen]
	if op.IsWrite {
		return true, op.Chan, true // send succeeded (closed send panics -> handled above)
	}
	if !recvOK {
		return nil, op.Chan, false // closed: normalize zero value -> nil
	}
	return recv.Interface(), op.Chan, true
}

// doSelect runs reflect.Select, converting the "send on closed channel"
// runtime panic into a flag (doc 05: closed put must return false, not panic).
func doSelect(cases []reflect.SelectCase) (chosen int, recv reflect.Value, recvOK bool, panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
		}
	}()
	chosen, recv, recvOK = reflect.Select(cases)
	return
}

// tryCase attempts a single case non-blockingly. taken=true when the op
// completed — including a send on a closed channel, which reports
// (false, ch, false) per the closed-put contract.
func tryCase(cs reflect.SelectCase, op AltOp) (val any, ch any, ok bool, taken bool) {
	defer func() {
		if r := recover(); r != nil { // send on closed channel
			val, ch, ok, taken = false, op.Chan, false, true
		}
	}()
	chosen, recv, recvOK := reflect.Select([]reflect.SelectCase{cs, {Dir: reflect.SelectDefault}})
	if chosen == 1 {
		return nil, nil, false, false // not ready
	}
	if op.IsWrite {
		return true, op.Chan, true, true
	}
	if !recvOK {
		return nil, op.Chan, false, true
	}
	return recv.Interface(), op.Chan, true, true
}
