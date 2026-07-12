package eval

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// recurSignal is the LoopID-tagged sentinel returned (as an error) by
// OpRecur after evaluating its args. The owning loop* eval or fn-method
// Invoke matches loopID — match → rebind and loop (plain Go loop,
// constant stack); no match → propagate outward (design/03 §5). Analysis
// guarantees a signal never escapes its owner: recur only targets the
// innermost frame and fn* bodies get their own frame.
type recurSignal struct {
	loopID string
	vals   []any
}

func (r *recurSignal) Error() string {
	return fmt.Sprintf("internal error: recur signal for %s escaped its loop", r.loopID)
}

// pushThreadBindings wraps lang.PushThreadBindings, converting its panic
// ("cannot dynamically bind non-dynamic var: ...") into an error. lang
// appends the new frame before validating entries, so on failure the
// partial frame is popped to keep the stack balanced.
func pushThreadBindings(bindings lang.IPersistentMap) (err error) {
	defer func() {
		if r := recover(); r != nil {
			lang.PopThreadBindings()
			if e2, ok := r.(error); ok {
				err = e2
				return
			}
			err = fmt.Errorf("%v", r)
		}
	}()
	lang.PushThreadBindings(bindings)
	return nil
}
