// Package briloader is the INTERPRETER's half of bri loading (ADR 0041,
// ADR 0071). It reads and evaluates the embedded bri.* sources through a
// live evaluator, registered lazily via the lib-provider registry, so a
// `cljgo dev` / REPL session can (require 'bri.http) and get the shims +
// framework fns on demand — the boot budget (ADR 0024) untouched.
//
// It is split from pkg/bri (which stays pure Go, no interpreter) so the
// COMPILED path (pkg/briaot) can link the shims without dragging the
// tree-walk evaluator into a user binary (ADR 0071 dec 2). pkg/bri owns
// the shims and the Spec descriptors; this package owns the read+eval
// loading; pkg/briaot owns the AOT-compiled equivalent. All three share
// one shim implementation — the parity guarantee (ADR 0071 dec 6).
package briloader

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/muthuishere/cljgo/pkg/bri"
	// Blank-import every OptIn namespace's isolated shim package (ADR 0074,
	// ADR 0076) so its init() registers the shim installer before an
	// interpreted (require 'bri.otel)/(require 'bri.db) interns the private
	// vars. briloader is the REPL / `cljgo dev` half — it already links the
	// whole interpreter, so linking the heavy deps (the OpenTelemetry SDK, the
	// SQLite + pgx drivers) here does not touch the AOT user-binary zero-cost
	// guarantee (that is enforced on pkg/briaot's sub-packages).
	_ "github.com/muthuishere/cljgo/pkg/bri/db"
	_ "github.com/muthuishere/cljgo/pkg/bri/otel"
	"github.com/muthuishere/cljgo/pkg/corelib"
	"github.com/muthuishere/cljgo/pkg/eval"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// current is the evaluator bri sources load through. Namespaces and vars
// are process-global (pkg/lang's registry), so which live evaluator
// performs the load does not matter semantically; Register keeps the most
// recent one (frontends create one driver per process).
var (
	mu      sync.Mutex
	current *eval.Evaluator
	loaded  = map[string]bool{}
)

// Register makes the bri.* namespaces requireable through e. Frontends
// (pkg/repl.New, pkg/nrepl.NewServer) call it when they boot an
// evaluator. It (re)registers the providers on every call — cheap map
// writes — so that after a `cljgo build` in the same process (which
// installs its own bri providers, ADR 0071) a fresh REPL driver restores
// providers bound to the live evaluator rather than a stale build one.
// The providers resolve `current` (the most recent evaluator) at load
// time, and namespaces/vars are process-global, so this is coherent.
func Register(e *eval.Evaluator) {
	mu.Lock()
	current = e
	mu.Unlock()
	for _, s := range bri.Specs() {
		s := s
		corelib.RegisterLibProvider(s.Name, func() { providerLoad(s) })
	}
}

// providerLoad evaluates one bri namespace's embedded source once per
// process (the loaded flag is set BEFORE evaluating so the
// bri.html → bri.http require chain re-enters cleanly).
func providerLoad(s bri.Spec) {
	mu.Lock()
	if loaded[s.Name] {
		mu.Unlock()
		return
	}
	loaded[s.Name] = true
	e := current
	mu.Unlock()
	if e == nil {
		panic(fmt.Errorf("bri: no evaluator registered (frontend did not call briloader.Register)"))
	}
	LoadSpec(e, s)
}

// LoadSpec interns s's Go shims then reads and evaluates its embedded
// source through e, under a pushed *ns*/*file* frame (the interpreter's
// load frame). It is unguarded — callers dedupe (providerLoad's loaded
// map for the REPL, the build discovery pass's own map for pkg/emit) —
// so it is also the seam cmd/genbri and the emitter's bri-provider reuse.
func LoadSpec(e *eval.Evaluator, s bri.Spec) {
	bri.InstallShimsInto(s)
	ns := lang.FindOrCreateNamespace(lang.NewSymbol(s.Name))
	lang.PushThreadBindings(lang.NewMap(
		lang.VarCurrentNS, ns,
		lang.VarFile, s.File,
	))
	defer lang.PopThreadBindings()

	r := reader.New(strings.NewReader(*s.Source),
		reader.WithFilename(s.File),
		reader.WithResolver(e.ReaderResolver()))
	for {
		form, err := r.ReadOne()
		if errors.Is(err, reader.ErrEOF) {
			return
		}
		if err != nil {
			panic(fmt.Errorf("bri: reading %s: %w", s.File, err))
		}
		if _, err := e.EvalForm(form); err != nil {
			panic(fmt.Errorf("bri: evaluating %s: %w", s.File, err))
		}
	}
}
