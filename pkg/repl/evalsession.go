// Session is the shared eval-session helper of ADR 0031: the binding
// frame every interactive session pushes (*ns* plus the *1 *2 *3 *e
// result/error vars) and the per-eval bookkeeping on it. The terminal
// Driver and pkg/nrepl are both thin frontends of this one helper — the
// smallest export that keeps their session semantics identical (spike
// S15 found Driver itself is not reusable: Run is wired to line-oriented
// stdin and EvalString deliberately skips *1).
package repl

import (
	"github.com/muthuishere/cljgo/pkg/eval"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// Session holds the vars of one eval session's binding frame. It is NOT
// goroutine-safe: dynamic bindings are goroutine-keyed in pkg/lang, so a
// session lives on exactly one goroutine — push Bindings() there and
// call RecordResult/RecordError only from it.
type Session struct {
	ev             *eval.Evaluator
	v1, v2, v3, ve *lang.Var
}

// NewSession returns a session over ev. The *1 *2 *3 *e vars are the
// interned clojure.core ones; their nil roots are untouched — per-session
// values live only in the pushed frame.
func NewSession(ev *eval.Evaluator) *Session {
	find := func(name string) *lang.Var {
		return lang.NSCore.FindInternedVar(lang.NewSymbol(name))
	}
	return &Session{ev: ev, v1: find("*1"), v2: find("*2"), v3: find("*3"), ve: find("*e")}
}

// Evaluator exposes the session's evaluator.
func (s *Session) Evaluator() *eval.Evaluator { return s.ev }

// Bindings is the session frame (design/03 §7b): *ns* seeded from the
// current namespace, *1 *2 *3 *e starting nil. Push it with
// lang.PushThreadBindings on the session's goroutine (and pop on exit);
// callers may Assoc extra vars (pkg/nrepl adds *out*) before pushing.
func (s *Session) Bindings() lang.IPersistentMap {
	return lang.NewMap(
		lang.VarCurrentNS, s.ev.CurrentNS(),
		s.v1, nil, s.v2, nil, s.v3, nil, s.ve, nil,
	)
}

// RecordResult shifts the result history after a successful eval:
// *3 ← *2 ← *1 ← res. Only valid under the session frame.
func (s *Session) RecordResult(res any) {
	s.v3.Set(s.v2.Deref())
	s.v2.Set(s.v1.Deref())
	s.v1.Set(res)
}

// RecordError binds *e to the eval error. Only valid under the session
// frame.
func (s *Session) RecordError(err error) { s.ve.Set(err) }
