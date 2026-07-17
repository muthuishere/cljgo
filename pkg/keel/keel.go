// Package keel is the Go half of the keel application framework (ADR
// 0041, openspec app-framework T1): the net/http adapter and the small
// host primitives (JSON, HMAC, env, files) the keel.* namespaces lean
// on. The Clojure half lives in core/keel/*.cljg, embedded via the core
// package.
//
// Loading is LAZY, through the lib-provider registry
// (pkg/corelib/require.go): Register wires a provider per keel namespace,
// and the first (require 'keel.http) interns the Go shims into the
// namespace and evaluates its embedded source. Nothing loads at boot —
// the boot budget (ADR 0024) is untouched — and nothing is scanned:
// the app requires keel; keel never requires the app.
//
// S20's honesty note said `cljgo new --template web && cljgo dev` boots NOTHING
// because the seed registry lacks net/http; this package IS the T1
// closure of that gap — a thin Go shim rather than reflect-seeding all
// of net/http, per the spike's own prototype (its main.go is the
// sketch this adapter grew from).
package keel

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/muthuishere/cljgo/core"
	"github.com/muthuishere/cljgo/pkg/corelib"
	"github.com/muthuishere/cljgo/pkg/eval"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// current is the evaluator keel sources load through. Namespaces and
// vars are process-global (pkg/lang's registry), so which live
// evaluator performs the load does not matter semantically; Register
// keeps the most recent one (frontends create one driver per process).
var (
	mu      sync.Mutex
	current *eval.Evaluator
	loaded  = map[string]bool{}
	wired   sync.Once
)

// nsSpec is one keel namespace: its embedded source and the Go shims
// interned (as :private vars) before the source evaluates.
type nsSpec struct {
	name    string
	file    string
	source  *string
	install func(def func(name string, fn func(args ...any) any))
}

func specs() []nsSpec {
	return []nsSpec{
		{name: "keel.http", file: "keel/http.cljg", source: &core.KeelHTTPSource, install: installHTTPShims},
		{name: "keel.html", file: "keel/html.cljg", source: &core.KeelHTMLSource, install: nil},
		{name: "keel.config", file: "keel/config.cljg", source: &core.KeelConfigSource, install: installConfigShims},
	}
}

// Register makes the keel.* namespaces requireable through e. Frontends
// (pkg/repl.New, pkg/nrepl.NewServer) call it when they boot an
// evaluator; the provider wiring itself happens once per process.
func Register(e *eval.Evaluator) {
	mu.Lock()
	current = e
	mu.Unlock()
	wired.Do(func() {
		for _, s := range specs() {
			s := s
			corelib.RegisterLibProvider(s.name, func() { load(s) })
		}
	})
}

// load evaluates one keel namespace's embedded source (once per
// process). The loaded flag is set BEFORE evaluating so the
// keel.html → keel.http require chain re-enters cleanly.
func load(s nsSpec) {
	mu.Lock()
	if loaded[s.name] {
		mu.Unlock()
		return
	}
	loaded[s.name] = true
	e := current
	mu.Unlock()
	if e == nil {
		panic(fmt.Errorf("keel: no evaluator registered (frontend did not call keel.Register)"))
	}

	ns := lang.FindOrCreateNamespace(lang.NewSymbol(s.name))
	if s.install != nil {
		s.install(func(name string, fn func(args ...any) any) {
			v := ns.Intern(lang.NewSymbol(name))
			v.BindRoot(lang.NewFnFunc(fn))
			v.SetMeta(v.Meta().Assoc(lang.KWPrivate, true).(lang.IPersistentMap))
		})
	}

	lang.PushThreadBindings(lang.NewMap(
		lang.VarCurrentNS, ns,
		lang.VarFile, s.file,
	))
	defer lang.PopThreadBindings()

	r := reader.New(strings.NewReader(*s.source),
		reader.WithFilename(s.file),
		reader.WithResolver(e.ReaderResolver()))
	for {
		form, err := r.ReadOne()
		if errors.Is(err, reader.ErrEOF) {
			return
		}
		if err != nil {
			panic(fmt.Errorf("keel: reading %s: %w", s.file, err))
		}
		if _, err := e.EvalForm(form); err != nil {
			panic(fmt.Errorf("keel: evaluating %s: %w", s.file, err))
		}
	}
}
